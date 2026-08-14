// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package reposynccli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/provider"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/reposync"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/templates"
)

// ConfigGenerateOptions holds options for config generate command.
type ConfigGenerateOptions struct {
	Provider         string
	Organization     string
	Path             string
	Token            string
	BaseURL          string
	CloneProto       string
	SSHPort          int
	Strategy         string
	Parallel         int
	MaxRetries       int
	Output           string
	IncludeArchived  bool
	IncludeForks     bool
	IncludePrivate   bool
	IsUser           bool
	IncludeSubgroups bool
	SubgroupMode     string
	FullOutput       bool

	// Metadata filters
	FilterLanguage string // Comma-separated languages (e.g., "go,rust")
	FilterMinStars int    // Minimum star count
	FilterMaxStars int    // Maximum star count (0 = unlimited)
	FilterLastPush string // Activity filter (e.g., "30d", "6M", "1y")
	FilterInclude  []string
	FilterExclude  []string
}

func (f CommandFactory) newConfigGenerateCmd() *cobra.Command {
	opts := &ConfigGenerateOptions{
		Strategy:     "reset",
		Parallel:     4,
		MaxRetries:   3,
		CloneProto:   "ssh",
		SubgroupMode: "flat",
		Output:       "sync.yaml",
	}

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate config from Git forge (GitHub, GitLab, Gitea)",
		Long: cliutil.QuickStartHelp(`  # Generate config from GitLab
  gz-git forge config generate --provider gitlab --org devbox -o sync.yaml \
    --token $GITLAB_TOKEN --path ~/repos

  # Include subgroups with flat naming
  gz-git forge config generate --provider gitlab --org parent-group \
    --include-subgroups --subgroup-mode flat -o sync.yaml \
    --token $GITLAB_TOKEN --path ~/repos

  # Generate from GitHub with HTTPS clone
  gz-git forge config generate --provider github --org myorg \
    --clone-proto https -o sync.yaml \
    --token $GITHUB_TOKEN --path ~/repos

  # Self-hosted GitLab with custom SSH port
  gz-git forge config generate --provider gitlab --org mygroup \
    --base-url https://gitlab.company.com --ssh-port 2224 -o sync.yaml \
    --token $GITLAB_TOKEN --path ~/repos

  # Generate config for selected repositories by name or full path
  gz-git forge config generate --provider github --org myorg \
    --include "api|web" --exclude "archive" -o sync.yaml \
    --token $GITHUB_TOKEN --path ~/repos`),
		RunE: func(cmd *cobra.Command, args []string) error {
			return f.runConfigGenerate(cmd, opts)
		},
	}

	// Provider and path (required)
	cmd.Flags().StringVar(&opts.Provider, "provider", "", "Git forge provider: github, gitlab, gitea [required]")
	cmd.Flags().StringVar(&opts.Organization, "org", "", "Organization/group name [required]")
	cmd.Flags().StringVar(&opts.Path, "path", "", "Directory for cloned repositories [required]")
	cmd.Flags().BoolVar(&opts.IsUser, "user", false, "Treat --org as a user instead of organization")

	// Authentication
	cmd.Flags().StringVar(&opts.Token, "token", "", "API token for authentication")
	cmd.Flags().StringVar(&opts.BaseURL, "base-url", "", "Base URL for self-hosted instances (API endpoint)")

	// Clone options
	cmd.Flags().StringVar(&opts.CloneProto, "clone-proto", opts.CloneProto, "Clone protocol: ssh, https")
	cmd.Flags().IntVar(&opts.SSHPort, "ssh-port", opts.SSHPort, "Custom SSH port (0 = default 22)")

	// Strategy and execution
	cmd.Flags().StringVar(&opts.Strategy, "sync-strategy", opts.Strategy, "Sync strategy (reset, pull, rebase, fetch)")
	cmd.Flags().IntVar(&opts.Parallel, "parallel", opts.Parallel, "Number of parallel workers")
	cmd.Flags().IntVar(&opts.MaxRetries, "max-retries", opts.MaxRetries, "Max retry attempts")

	// Output
	cmd.Flags().StringVarP(&opts.Output, "output", "o", opts.Output, "Output file path")

	// Filters
	cmd.Flags().BoolVar(&opts.IncludeArchived, "include-archived", false, "Include archived repositories")
	cmd.Flags().BoolVar(&opts.IncludeForks, "include-forks", false, "Include forked repositories")
	cmd.Flags().BoolVar(&opts.IncludePrivate, "include-private", true, "Include private repositories")

	// GitLab subgroups
	cmd.Flags().BoolVar(&opts.IncludeSubgroups, "include-subgroups", false, "Include subgroups (GitLab only)")
	cmd.Flags().StringVar(&opts.SubgroupMode, "subgroup-mode", opts.SubgroupMode, "Subgroup mode: flat (dash-separated) or nested (directories)")

	// Output format
	cmd.Flags().BoolVar(&opts.FullOutput, "full", false, "Output all fields (name, path) even if redundant")

	// Metadata filters
	cmd.Flags().StringVar(&opts.FilterLanguage, "language", "", "Filter by language (comma-separated, e.g., go,rust)")
	cmd.Flags().IntVar(&opts.FilterMinStars, "min-stars", 0, "Minimum star count")
	cmd.Flags().IntVar(&opts.FilterMaxStars, "max-stars", 0, "Maximum star count (0 = unlimited)")
	cmd.Flags().StringVar(&opts.FilterLastPush, "last-push-within", "", "Filter by recent activity (e.g., 7d, 30d, 6M, 1y)")
	cmd.Flags().StringSliceVar(&opts.FilterInclude, "include", nil, "Include repos matching regex (name or full path; can be repeated)")
	cmd.Flags().StringSliceVar(&opts.FilterExclude, "exclude", nil, "Exclude repos matching regex (name or full path; can be repeated)")

	// Mark required flags. These names are registered immediately above, so an
	// error here indicates a programming/configuration mistake and must not be
	// silently discarded.
	for _, name := range []string{"provider", "org", "path"} {
		if err := cmd.MarkFlagRequired(name); err != nil {
			panic(fmt.Errorf("mark --%s required: %w", name, err))
		}
	}

	return cmd
}

// RunConfigGenerate executes the config generation logic.
func RunConfigGenerate(cmd *cobra.Command, opts *ConfigGenerateOptions) error {
	ctx := cmd.Context()

	// Validate clone protocol
	if opts.CloneProto != "ssh" && opts.CloneProto != "https" {
		return fmt.Errorf("invalid --clone-proto: %s (must be ssh or https)", opts.CloneProto)
	}

	// Validate subgroup mode
	if opts.SubgroupMode != "flat" && opts.SubgroupMode != "nested" {
		return fmt.Errorf("invalid --subgroup-mode: %s (must be flat or nested)", opts.SubgroupMode)
	}

	// Build metadata filter
	metadataFilter, err := BuildFilterFromOptions(
		opts.FilterLanguage,
		opts.FilterMinStars,
		opts.FilterMaxStars,
		opts.FilterLastPush,
	)
	if err != nil {
		return err
	}

	if _, err := reposync.NewRepositoryPatternFilter(opts.FilterInclude, opts.FilterExclude); err != nil {
		return err
	}

	// Print warnings for provider-specific filter limitations
	PrintWarningsForProvider(opts.Provider, metadataFilter, cmd.OutOrStdout())

	// Create provider
	forgeProvider, err := createConfigGenerateProvider(opts)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	// Fetch repositories from forge
	repos, err := fetchRepositoriesFromForge(ctx, forgeProvider, opts, metadataFilter)
	if err != nil {
		return fmt.Errorf("failed to fetch repositories: %w", err)
	}

	// Build repository entries for template
	repoEntries := make([]templates.ForgeRepoData, 0, len(repos))
	for _, repo := range repos {
		targetPath := buildTargetPath(repo, opts)
		// Omit path if the directory name equals repo name (compact output)
		// Path defaults to Name when loading config, so redundant paths can be omitted
		pathOutput := targetPath
		if !opts.FullOutput && filepath.Base(targetPath) == repo.Name {
			pathOutput = ""
		}

		repoEntries = append(repoEntries, templates.ForgeRepoData{
			Name: repo.Name,
			URL:  repo.CloneURL,
			Path: pathOutput,
		})
	}

	// Render template
	data := templates.ForgeGeneratedData{
		GeneratedAt:  time.Now().Format(time.RFC3339),
		Provider:     opts.Provider,
		Organization: opts.Organization,
		Strategy:     opts.Strategy,
		Parallel:     opts.Parallel,
		MaxRetries:   opts.MaxRetries,
		CloneProto:   opts.CloneProto,
		SSHPort:      opts.SSHPort,
		Repositories: repoEntries,
	}

	content, err := templates.Render(templates.RepositoriesForge, data)
	if err != nil {
		return fmt.Errorf("failed to render template: %w", err)
	}

	if err := os.WriteFile(opts.Output, []byte(content), 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "✓ Generated configuration: %s (%d repositories)\n", opts.Output, len(repos))
	fmt.Fprintf(cmd.OutOrStdout(), "\nReview the file and run:\n  gz-git workspace sync -c %s\n", opts.Output)

	return nil
}

func (f CommandFactory) runConfigGenerate(cmd *cobra.Command, opts *ConfigGenerateOptions) error {
	return RunConfigGenerate(cmd, opts)
}

func createConfigGenerateProvider(opts *ConfigGenerateOptions) (provider.Provider, error) {
	// Keep the command-specific hint before delegating: NewForgeProviderWithAuth
	// would surface the constructor's generic "baseURL is required" message here.
	if opts.Provider == "gitea" && opts.BaseURL == "" {
		return nil, fmt.Errorf("gitea requires --base-url")
	}
	// Delegate to the single owner of provider construction (returns the
	// auth-capable interface, which satisfies provider.Provider).
	return NewForgeProviderWithAuth(opts.Provider, opts.Token, opts.BaseURL, opts.SSHPort)
}

func fetchRepositoriesFromForge(ctx context.Context, forgeProvider provider.Provider, opts *ConfigGenerateOptions, filter *MetadataFilter) ([]*provider.Repository, error) {
	var repos []*provider.Repository
	var err error

	if opts.IsUser {
		repos, err = forgeProvider.ListUserRepos(ctx, opts.Organization)
	} else {
		repos, err = forgeProvider.ListOrganizationRepos(ctx, opts.Organization)
	}

	if err != nil {
		return nil, err
	}

	patternFilter, err := reposync.NewRepositoryPatternFilter(opts.FilterInclude, opts.FilterExclude)
	if err != nil {
		return nil, err
	}

	// Filter based on options
	filtered := make([]*provider.Repository, 0, len(repos))
	for _, repo := range repos {
		// Filter archived
		if repo.Archived && !opts.IncludeArchived {
			continue
		}
		// Filter forks
		if repo.Fork && !opts.IncludeForks {
			continue
		}
		// Filter private (include by default)
		if repo.Private && !opts.IncludePrivate {
			continue
		}

		// Filter subgroups if needed (GitLab only)
		if !opts.IncludeSubgroups && opts.Provider == "gitlab" {
			// Keep only repos directly in the org (no slashes in FullName after org/)
			if repo.FullName != fmt.Sprintf("%s/%s", opts.Organization, repo.Name) {
				continue
			}
		}

		// Apply name/full name regex filter
		if !patternFilter.Match(repo) {
			continue
		}

		// Apply metadata filter
		if filter != nil && !filter.Match(repo) {
			continue
		}

		filtered = append(filtered, repo)
	}

	return filtered, nil
}

func buildTargetPath(repo *provider.Repository, opts *ConfigGenerateOptions) string {
	basePath := opts.Path
	projectPath := repo.FullName

	if !opts.IncludeSubgroups || opts.SubgroupMode == "" {
		return fmt.Sprintf("%s/%s", basePath, repo.Name)
	}

	switch opts.SubgroupMode {
	case "flat":
		// Replace slashes with dashes: parent-group/subgroup/repo → parent-group-subgroup-repo
		flat := projectPath
		for i := 0; i < len(flat); i++ {
			if flat[i] == '/' {
				flat = flat[:i] + "-" + flat[i+1:]
			}
		}
		return fmt.Sprintf("%s/%s", basePath, flat)
	case "nested":
		// Keep directory structure: parent-group/subgroup/repo
		return fmt.Sprintf("%s/%s", basePath, projectPath)
	default:
		return fmt.Sprintf("%s/%s", basePath, repo.Name)
	}
}
