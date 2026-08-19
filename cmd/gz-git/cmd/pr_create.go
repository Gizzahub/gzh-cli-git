// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/provider"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/reposynccli"
)

var (
	prCreateFlags     BulkCommandFlags
	prCreateTitle     string
	prCreateBody      string
	prCreateBase      string
	prCreateDraft     bool
	prCreateReviewers []string
	prCreateLabels    []string
	prCreateProvider  string
	prCreateToken     string
)

var prCreateCmd = &cobra.Command{
	Use:   "create [directory]",
	Short: "Create a pull request / merge request from the current branch",
	Long: cliutil.QuickStartHelp(`  # Current repo, current branch → default branch
  gz-git pr create

  # Bulk across a workspace
  gz-git pr create -d 2 --parallel 4 ~/work

  # Existing PRs are skipped and their URLs printed`) + cliutil.ExitCodesBulkHelp(),
	Args: cobra.MaximumNArgs(1),
	RunE: runPRCreate,
}

func init() {
	prCmd.AddCommand(prCreateCmd)
	addBulkFlagsWithOpts(prCreateCmd, &prCreateFlags, BulkFlagOptions{
		SkipWatch: true,
		SkipFetch: true,
	})
	prCreateCmd.Flags().StringVar(&prCreateTitle, "title", "", "PR title (default: derived from the branch name)")
	prCreateCmd.Flags().StringVar(&prCreateBody, "body", "", "PR body")
	prCreateCmd.Flags().StringVar(&prCreateBase, "base", "", "base branch (default: forge default branch)")
	prCreateCmd.Flags().BoolVar(&prCreateDraft, "draft", false, "open as draft")
	prCreateCmd.Flags().StringSliceVar(&prCreateReviewers, "reviewer", nil, "reviewer usernames")
	prCreateCmd.Flags().StringSliceVar(&prCreateLabels, "label", nil, "labels")
	prCreateCmd.Flags().StringVar(&prCreateProvider, "provider", "", "force provider: github, gitlab, or gitea")
	prCreateCmd.Flags().StringVar(&prCreateToken, "token", "", "forge API token")
}

type prCreateOutcome struct {
	Path    string
	Status  string
	Message string
	URL     string
	Err     error
}

func runPRCreate(cmd *cobra.Command, args []string) error {
	ctx := cmdContext(cmd)
	directory, err := validateBulkDirectory(args)
	if err != nil {
		return cliutil.NewExitError(cliutil.ExitToolError, err)
	}
	if err := validateBulkDepth(cmd, prCreateFlags.Depth); err != nil {
		return cliutil.NewExitError(cliutil.ExitToolError, err)
	}

	effective, _ := LoadEffectiveConfig(cmd, map[string]any{
		"provider": prCreateProvider,
		"token":    prCreateToken,
	})
	if effective != nil {
		if prCreateProvider == "" {
			prCreateProvider = effective.Provider
		}
		if prCreateToken == "" {
			prCreateToken = effective.Token
		}
	}

	client := repository.NewClient()
	scan, err := client.ScanRepositories(ctx, repository.ScanOptions{
		Directory:         directory,
		MaxDepth:          prCreateFlags.Depth,
		IncludeSubmodules: prCreateFlags.IncludeSubmodules,
		IncludePattern:    prCreateFlags.Include,
		ExcludePattern:    prCreateFlags.Exclude,
	})
	if err != nil {
		return cliutil.NewExitError(cliutil.ExitToolError, err)
	}

	parallel := prCreateFlags.Parallel
	if parallel < 1 {
		parallel = 1
	}
	outcomes := make([]prCreateOutcome, len(scan.Paths))
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for i, path := range scan.Paths {
		wg.Add(1)
		go func(i int, path string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			outcomes[i] = createPRForRepo(ctx, client, path, effective)
		}(i, path)
	}
	wg.Wait()

	failed := 0
	for _, out := range outcomes {
		switch out.Status {
		case "created", "exists", "dry-run":
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", out.Status, out.Path, out.Message)
		default:
			failed++
			fmt.Fprintf(cmd.ErrOrStderr(), "failed\t%s\t%s\n", out.Path, out.Message)
		}
	}
	if failed > 0 {
		return cliutil.NewExitError(cliutil.ExitPartialFailed, fmt.Errorf("%d of %d repositories failed", failed, len(outcomes)))
	}
	return nil
}

func createPRForRepo(ctx context.Context, client repository.Client, path string, effective *config.EffectiveConfig) prCreateOutcome {
	repo, err := client.Open(ctx, path)
	if err != nil {
		return prCreateOutcome{Path: path, Status: "failed", Message: err.Error(), Err: err}
	}
	info, err := client.GetInfo(ctx, repo)
	if err != nil {
		return prCreateOutcome{Path: path, Status: "failed", Message: err.Error(), Err: err}
	}
	if info.Branch == "" {
		return prCreateOutcome{Path: path, Status: "failed", Message: "detached HEAD"}
	}
	if info.RemoteURL == "" {
		return prCreateOutcome{Path: path, Status: "failed", Message: "no origin remote"}
	}
	remote, err := provider.ParseForgeRemote(info.RemoteURL)
	if err != nil {
		return prCreateOutcome{Path: path, Status: "failed", Message: err.Error(), Err: err}
	}
	provName := remote.Provider
	if prCreateProvider != "" {
		provName = prCreateProvider
	}
	if provName == "" {
		return prCreateOutcome{Path: path, Status: "failed", Message: "unknown forge host; pass --provider"}
	}
	baseURL := remote.BaseURL
	if effective != nil && effective.BaseURL != "" && remote.Host != "github.com" && remote.Host != "gitlab.com" {
		baseURL = effective.BaseURL
	}
	token := resolveForgeToken(provName, prCreateToken)
	if token == "" && !prCreateFlags.DryRun {
		return prCreateOutcome{Path: path, Status: "failed", Message: "missing " + provName + " token"}
	}

	title := prCreateTitle
	if title == "" {
		title = defaultPRTitle(info.Branch)
	}
	body := prCreateBody
	if body == "" {
		body = "Opened from `" + info.Branch + "`."
	}
	base := prCreateBase
	if base != "" && base == info.Branch {
		return prCreateOutcome{Path: path, Status: "failed", Message: "head and base are the same branch"}
	}

	if prCreateFlags.DryRun {
		msg := fmt.Sprintf("would create %s %s:%s → %s", provName, remote.Owner+"/"+remote.Repo, info.Branch, orDefault(base, "default"))
		return prCreateOutcome{Path: path, Status: "dry-run", Message: msg}
	}

	requester, err := newPullRequester(provName, token, baseURL)
	if err != nil {
		return prCreateOutcome{Path: path, Status: "failed", Message: err.Error(), Err: err}
	}
	existing, err := requester.FindPullRequest(ctx, remote.Owner, remote.Repo, info.Branch, base)
	switch {
	case err == nil:
		return prCreateOutcome{Path: path, Status: "exists", Message: existing.URL, URL: existing.URL}
	case errors.Is(err, provider.ErrPullRequestNotFound):
		// continue to create
	default:
		return prCreateOutcome{Path: path, Status: "failed", Message: err.Error(), Err: err}
	}

	created, err := requester.CreatePullRequest(ctx, provider.CreatePullRequestInput{
		Owner:     remote.Owner,
		Repo:      remote.Repo,
		Title:     title,
		Body:      body,
		Head:      info.Branch,
		Base:      base,
		Draft:     prCreateDraft,
		Reviewers: prCreateReviewers,
		Labels:    prCreateLabels,
	})
	if err != nil {
		return prCreateOutcome{Path: path, Status: "failed", Message: err.Error(), Err: err}
	}
	return prCreateOutcome{Path: path, Status: "created", Message: created.URL, URL: created.URL}
}

func newPullRequester(name, token, baseURL string) (provider.PullRequester, error) {
	p, err := reposynccli.NewForgeProviderWithAuth(name, token, baseURL, 0)
	if err != nil {
		return nil, err
	}
	requester, ok := p.(provider.PullRequester)
	if !ok {
		return nil, fmt.Errorf("provider %s does not implement pull requests", name)
	}
	return requester, nil
}

func resolveForgeToken(providerName, flagToken string) string {
	if flagToken != "" {
		return flagToken
	}
	if tok, _ := config.ResolveTokenFromEnv(providerName); tok != "" {
		return tok
	}
	if tok, err := config.DefaultTokenStore.Get(providerName); err == nil && tok != "" {
		return tok
	}
	return ""
}

func defaultPRTitle(branch string) string {
	parts := strings.Split(strings.Trim(branch, "/"), "/")
	slug := parts[len(parts)-1]
	if slug == "" {
		return branch
	}
	return slug
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
