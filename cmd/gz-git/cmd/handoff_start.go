// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/handoff"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/identity"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

var handoffStartFlags BulkCommandFlags

var handoffStartCmd = &cobra.Command{
	Use:   "start [directory]",
	Short: "Bring every repository up to date with the remote",
	Long: cliutil.QuickStartHelp(`  # After arriving at this machine
  gz-git handoff start

  # See what would be pulled first
  gz-git handoff start --dry-run

Each repository is pulled with a rebase, so any commits that are still only
here are replayed on top of what the remote gained meanwhile instead of
producing a merge commit. Deleted remote branches are pruned at the same time.

Repositories with uncommitted work are fetched but not rebased: replaying
commits over an unfinished edit risks losing it. The same applies to unresolved
conflicts, an interrupted rebase, and a detached HEAD.

Branches whose remote gained commits signed by another device or agent are
named at the end. Nothing there is unsafe — a rebase replays over them and
loses nothing — but two writers on one branch is the point to give each of them
their own. A --dry-run reads the remote refs already on disk, so it only sees
what the last fetch brought in.

Exit Codes:
  0  every repository is up to date
  1  at least one repository needs attention
  2  the operation itself could not run`),
	Args: cobra.MaximumNArgs(1),
	RunE: runHandoffStart,
}

func init() {
	handoffCmd.AddCommand(handoffStartCmd)

	addBulkFlagsWithOpts(handoffStartCmd, &handoffStartFlags, BulkFlagOptions{
		SkipFetch: true,
		SkipWatch: true,
	})
}

// handoffStartReport is the machine-readable account of one arrival.
type handoffStartReport struct {
	Plan    handoff.StartPlan `json:"plan"`
	Updated []string          `json:"updated,omitempty"`
	Current []string          `json:"already_current,omitempty"`
	Failed  []string          `json:"failed,omitempty"`
	Shared  []sharedBranch    `json:"shared_branches,omitempty"`
	DryRun  bool              `json:"dry_run"`
}

// sharedBranch is a branch this machine is not alone on: the remote gained
// commits signed by someone else while this machine was away.
type sharedBranch struct {
	Path    string                     `json:"path"`
	Branch  string                     `json:"branch,omitempty"`
	Commits []repository.ForeignCommit `json:"commits"`
	Writers []string                   `json:"writers"`
}

// Ready reports whether every repository ended up aligned with its remote.
func (r *handoffStartReport) Ready() bool {
	return len(r.Failed) == 0 && len(r.Plan.Skipped) == 0
}

func runHandoffStart(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	directory, err := validateBulkDirectory(args)
	if err != nil {
		return err
	}
	if err := resolveBulkDepth(cmd, directory, &handoffStartFlags.Depth); err != nil {
		return err
	}
	if err := validateBulkFormat(handoffStartFlags.Format); err != nil {
		return err
	}

	assessment, err := assessHandoff(ctx, directory, handoffStartFlags)
	if err != nil {
		return cliutil.NewExitError(2, err)
	}

	plan := handoff.PlanStart(assessment)
	report := &handoffStartReport{Plan: plan, DryRun: handoffStartFlags.DryRun}

	effective, _ := LoadEffectiveConfig(cmd, nil)
	mine := pushIdentity(effective)

	if handoffStartFlags.DryRun {
		// Nothing was fetched, so this reads whatever remote refs are already on
		// disk — the same refs the rest of the preview is built from.
		report.Shared = findSharedBranches(ctx, plan.Update, mine)
	} else if err := updateRepositories(ctx, directory, plan, mine, report); err != nil {
		return cliutil.NewExitError(2, err)
	}

	if handoffStartFlags.Format == "json" {
		if err := encodeJSON(report); err != nil {
			return cliutil.NewExitError(2, err)
		}
	} else if !quiet {
		printHandoffStart(report)
	}

	// A dry run that found nothing to skip is a clean bill of health, so the
	// same rule decides the exit code in both modes.
	if report.Ready() {
		return nil
	}

	needsAttention := len(report.Failed) + len(plan.Skipped)
	if needsAttention == 1 {
		return cliutil.NewExitError(1, fmt.Errorf("1 repository needs attention"))
	}
	return cliutil.NewExitError(1, fmt.Errorf("%d repositories need attention", needsAttention))
}

// findSharedBranches names the repositories whose remote branch advanced under
// someone else's device or agent.
//
// It reports rather than blocks. Rebasing onto another writer's commits is
// safe; what matters is knowing the branch has two writers on it, because that
// is the point at which it should be split instead of shared.
//
// A failure to read one repository is not worth interrupting an arrival over,
// so it is dropped: the result is a hint, and a missing hint costs nothing.
func findSharedBranches(ctx context.Context, repos []handoff.RepoAssessment, mine identity.Identity) []sharedBranch {
	if !mine.Known() || len(repos) == 0 {
		return nil
	}

	client := repository.NewClient()

	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		shared []sharedBranch
	)

	// The same worker count the rest of the command runs at; each check is one
	// cheap "git log" against refs that are already local.
	limit := make(chan struct{}, max(1, handoffStartFlags.Parallel))

	for _, repo := range repos {
		wg.Go(func() {
			limit <- struct{}{}
			defer func() { <-limit }()

			commits, err := client.IncomingForeignWork(ctx, repo.Path, mine)
			if err != nil || len(commits) == 0 {
				return
			}

			mu.Lock()
			shared = append(shared, sharedBranch{
				Path:    repo.RelativePath,
				Branch:  repo.Branch,
				Commits: commits,
				Writers: foreignWriters(commits),
			})
			mu.Unlock()
		})
	}
	wg.Wait()

	sort.Slice(shared, func(i, j int) bool { return shared[i].Path < shared[j].Path })
	return shared
}

// foreignWriters lists each distinct writer once, in the order first seen, so
// twenty commits from one machine read as one name.
func foreignWriters(commits []repository.ForeignCommit) []string {
	seen := make(map[string]bool, len(commits))
	writers := make([]string, 0, len(commits))

	for _, c := range commits {
		name := c.Identity.Name()
		if seen[name] {
			continue
		}
		seen[name] = true
		writers = append(writers, name)
	}

	return writers
}

// updateRepositories rebases what it safely can and refreshes the rest.
func updateRepositories(ctx context.Context, directory string, plan handoff.StartPlan, mine identity.Identity, report *handoffStartReport) error {
	client := repository.NewClient()

	// Fetch everything first, including the repositories that cannot be rebased:
	// their remote refs then reflect reality, so the status that follows is worth
	// reading. The pull below would fetch its own share anyway, so this costs
	// nothing extra and gives the shared-branch check refs to read.
	all := append(handoff.Paths(plan.Update), handoff.Paths(plan.Skipped)...)
	if len(all) > 0 {
		if _, err := client.BulkFetch(ctx, repository.BulkFetchOptions{
			Directory:         directory,
			Parallel:          handoffStartFlags.Parallel,
			MaxDepth:          handoffStartFlags.Depth,
			IncludeSubmodules: handoffStartFlags.IncludeSubmodules,
			IncludePattern:    repoPathPattern(all),
			Prune:             true,
			Verbose:           verbose,
			Logger:            createBulkLogger(verbose),
		}); err != nil {
			return fmt.Errorf("failed to fetch: %w", err)
		}
	}

	// Ask before the rebase runs: afterwards the incoming commits are part of
	// the local branch and there is no range left to ask about.
	report.Shared = findSharedBranches(ctx, plan.Update, mine)

	if len(plan.Update) == 0 {
		return nil
	}

	result, err := client.BulkPull(ctx, repository.BulkPullOptions{
		Directory:         directory,
		Parallel:          handoffStartFlags.Parallel,
		MaxDepth:          handoffStartFlags.Depth,
		IncludeSubmodules: handoffStartFlags.IncludeSubmodules,
		IncludePattern:    repoPathPattern(handoff.Paths(plan.Update)),
		// Rebase keeps a work branch linear across machines; a merge here would
		// record that the branch was touched in two places, which is noise.
		Strategy: "rebase",
		Prune:    true,
		Verbose:  verbose,
		Logger:   createBulkLogger(verbose),
	})
	if err != nil {
		return fmt.Errorf("failed to pull: %w", err)
	}

	for _, r := range result.Repositories {
		switch r.Status {
		case repository.StatusPulled:
			report.Updated = append(report.Updated, r.RelativePath)
		case repository.StatusUpToDate, repository.StatusSkipped:
			report.Current = append(report.Current, r.RelativePath)
		default:
			report.Failed = append(report.Failed, r.RelativePath)
		}
	}

	return nil
}

func printHandoffStart(report *handoffStartReport) {
	fmt.Println()

	if report.DryRun {
		for _, repo := range report.Plan.Update {
			fmt.Printf("  %s→%s %s%s %swould pull with a rebase%s\n",
				cliutil.ColorCyan, cliutil.ColorReset, repo.RelativePath,
				handoffBranchSuffix(repo), cliutil.ColorGray, cliutil.ColorReset)
		}
	} else {
		for _, path := range report.Updated {
			fmt.Printf("  %s✓%s %s %supdated%s\n",
				cliutil.ColorGreen, cliutil.ColorReset, path, cliutil.ColorGray, cliutil.ColorReset)
		}
		for _, path := range report.Failed {
			fmt.Printf("  %s✗%s %s %spull failed%s\n",
				cliutil.ColorRed, cliutil.ColorReset, path, cliutil.ColorGray, cliutil.ColorReset)
		}
	}

	for _, repo := range report.Plan.Skipped {
		blocker, _ := handoff.FirstStartBlocker(repo)
		fmt.Printf("  %s✗%s %s%s\n", cliutil.ColorRed, cliutil.ColorReset,
			repo.RelativePath, handoffBranchSuffix(repo))
		fmt.Printf("      %snot rebased — %s%s\n", cliutil.ColorGray, blocker.Detail, cliutil.ColorReset)
	}

	printSharedBranches(report.Shared)

	fmt.Println()
	printHandoffStartVerdict(report)
	fmt.Println()
}

// printSharedBranches notes the branches someone else is also writing to. It is
// a remark rather than a warning: nothing here failed, and nothing is at risk
// until one of the two writers force pushes.
func printSharedBranches(shared []sharedBranch) {
	if len(shared) == 0 {
		return
	}

	fmt.Println()
	for _, s := range shared {
		branch := ""
		if s.Branch != "" {
			branch = " (" + s.Branch + ")"
		}
		fmt.Printf("  %s·%s %s%s %salso written by %s — %d commit(s)%s\n",
			cliutil.ColorCyan, cliutil.ColorReset, s.Path, branch,
			cliutil.ColorGray, joinWriters(s.Writers), len(s.Commits), cliutil.ColorReset)
	}
	fmt.Printf("\n%sTwo writers on one branch is the point to give each of them their own.%s\n",
		cliutil.ColorGray, cliutil.ColorReset)
}

// joinWriters renders a writer list as prose.
func joinWriters(writers []string) string {
	switch len(writers) {
	case 0:
		return "someone else"
	case 1:
		return writers[0]
	default:
		return strings.Join(writers[:len(writers)-1], ", ") + " and " + writers[len(writers)-1]
	}
}

func printHandoffStartVerdict(report *handoffStartReport) {
	total := len(report.Plan.Update) + len(report.Plan.Skipped)

	if report.DryRun {
		fmt.Printf("%s%d of %d repositories would be rebased onto their remote.%s\n",
			cliutil.ColorGray, len(report.Plan.Update), total, cliutil.ColorReset)
		return
	}

	if report.Ready() {
		fmt.Printf("%sREADY%s — %d repositories match their remote (%d updated).\n",
			cliutil.ColorGreen, cliutil.ColorReset, total, len(report.Updated))
		return
	}

	needsAttention := len(report.Failed) + len(report.Plan.Skipped)
	fmt.Printf("%sATTENTION%s — %d of %d repositories were not brought up to date.\n",
		cliutil.ColorYellow, cliutil.ColorReset, needsAttention, total)

	if len(report.Plan.Skipped) > 0 {
		fmt.Println("\nRun 'gz-git handoff check' to see what is holding them.")
	}
}
