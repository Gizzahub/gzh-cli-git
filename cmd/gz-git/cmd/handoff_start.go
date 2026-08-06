// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/handoff"
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
	DryRun  bool              `json:"dry_run"`
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
	if err := validateBulkDepth(cmd, handoffStartFlags.Depth); err != nil {
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

	if !handoffStartFlags.DryRun {
		if err := updateRepositories(ctx, directory, plan, report); err != nil {
			return cliutil.NewExitError(2, err)
		}
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

// updateRepositories rebases what it safely can and refreshes the rest.
func updateRepositories(ctx context.Context, directory string, plan handoff.StartPlan, report *handoffStartReport) error {
	client := repository.NewClient()

	// Repositories that cannot be rebased are still fetched. Their remote refs
	// then reflect reality, so the status that follows is worth reading.
	if len(plan.Skipped) > 0 {
		if _, err := client.BulkFetch(ctx, repository.BulkFetchOptions{
			Directory:         directory,
			Parallel:          handoffStartFlags.Parallel,
			MaxDepth:          handoffStartFlags.Depth,
			IncludeSubmodules: handoffStartFlags.IncludeSubmodules,
			IncludePattern:    repoPathPattern(handoff.Paths(plan.Skipped)),
			Prune:             true,
			Verbose:           verbose,
			Logger:            createBulkLogger(verbose),
		}); err != nil {
			return fmt.Errorf("failed to fetch: %w", err)
		}
	}

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

	fmt.Println()
	printHandoffStartVerdict(report)
	fmt.Println()
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
