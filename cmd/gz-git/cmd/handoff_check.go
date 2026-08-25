// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/handoff"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

var handoffCheckFlags BulkCommandFlags

var handoffCheckCmd = &cobra.Command{
	Use:   "check [directory]",
	Short: "Report whether work can safely be left behind",
	Long: cliutil.QuickStartHelp(`  # Check the current directory
  gz-git handoff check

  # Check a workspace two levels deep
  gz-git handoff check -d 2 ~/workspace

  # Machine-readable verdict
  gz-git handoff check --format json

Exit Codes:
  0  ready — nothing exists only on this machine
  1  work is outstanding (fixable by 'handoff end', or blocked)
  2  the check itself could not run

No network access is needed. Unpushed commits are counted against the remote
tracking ref, which only advances when this machine pushes, so a stale ref
cannot hide work that was never sent.`),
	Args: cobra.MaximumNArgs(1),
	RunE: runHandoffCheck,
}

func init() {
	handoffCmd.AddCommand(handoffCheckCmd)

	addBulkFlagsWithOpts(handoffCheckCmd, &handoffCheckFlags, BulkFlagOptions{
		SkipDryRun: true,
		SkipFetch:  true,
		SkipWatch:  true,
	})
}

func runHandoffCheck(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	directory, err := validateBulkDirectory(args)
	if err != nil {
		return err
	}
	if err := resolveBulkDepth(cmd, directory, &handoffCheckFlags.Depth); err != nil {
		return err
	}
	if err := validateBulkFormat(handoffCheckFlags.Format); err != nil {
		return err
	}

	assessment, err := assessHandoff(ctx, directory, handoffCheckFlags)
	if err != nil {
		return cliutil.NewExitError(2, err)
	}

	if handoffCheckFlags.Format == "json" {
		if err := encodeJSON(assessment); err != nil {
			return cliutil.NewExitError(2, err)
		}
	} else if !quiet {
		printHandoffAssessment(assessment)
	}

	if assessment.Verdict == handoff.VerdictReady {
		return nil
	}
	// The rendered report above already says what is outstanding; this error
	// exists to carry exit code 1, so it stays to the verdict alone.
	return cliutil.NewExitError(1, fmt.Errorf("handoff verdict: %s", assessment.Verdict))
}

// assessHandoff scans directory and classifies every repository found.
func assessHandoff(ctx context.Context, directory string, flags BulkCommandFlags) (*handoff.Assessment, error) {
	client := repository.NewClient()

	result, err := client.BulkStatus(ctx, repository.BulkStatusOptions{
		Directory:         directory,
		Parallel:          flags.Parallel,
		MaxDepth:          flags.Depth,
		IncludeSubmodules: flags.IncludeSubmodules,
		IncludePattern:    flags.Include,
		ExcludePattern:    resolveScanExclude(directory, flags.Exclude),
		Verbose:           verbose,
		Logger:            createBulkLogger(verbose),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan repositories: %w", err)
	}

	return handoff.Assess(result.Repositories), nil
}

func printHandoffAssessment(a *handoff.Assessment) {
	fmt.Println()

	notReady := a.NotReady()
	for _, repo := range notReady {
		fmt.Printf("  %s %s%s\n", handoffRepoSymbol(repo), repo.RelativePath, handoffBranchSuffix(repo))
		for _, b := range repo.Blockers {
			fmt.Printf("      %s%s%s\n", cliutil.ColorGray, b.Detail, cliutil.ColorReset)
		}
	}
	if len(notReady) > 0 {
		fmt.Println()
	}

	printHandoffVerdict(a, true)
	fmt.Println()
}

// printHandoffVerdict renders the one-line verdict. advise adds the pointer to
// "handoff end", which only makes sense for a command that has not already run
// it.
func printHandoffVerdict(a *handoff.Assessment, advise bool) {
	switch a.Verdict {
	case handoff.VerdictReady:
		fmt.Printf("%sSAFE TO LEAVE%s — %d repositories, nothing exists only here.\n",
			cliutil.ColorGreen, cliutil.ColorReset, a.TotalScanned)

	case handoff.VerdictFixable:
		fmt.Printf("%sNOT YET%s — %d of %d repositories have work that exists only here (%s).\n",
			cliutil.ColorYellow, cliutil.ColorReset,
			len(a.NotReady()), a.TotalScanned, summarizeHandoffReasons(a))
		if advise {
			fmt.Println("\nRun 'gz-git handoff end' to commit and push all of it.")
		}

	case handoff.VerdictBlocked:
		blocked := a.Blocked()
		fmt.Printf("%sBLOCKED%s — %d of %d repositories need a decision made on this machine (%s).\n",
			cliutil.ColorRed, cliutil.ColorReset,
			len(blocked), a.TotalScanned, summarizeHandoffReasons(a))
		if advise {
			fmt.Println("\n'handoff end' cannot resolve these; they are marked ✗ above.")
		}
	}
}

// handoffRepoSymbol distinguishes what 'handoff end' can resolve on its own
// from what needs a human decision.
func handoffRepoSymbol(r handoff.RepoAssessment) string {
	if r.AutoFixable() {
		return cliutil.ColorYellow + "⚠" + cliutil.ColorReset
	}
	return cliutil.ColorRed + "✗" + cliutil.ColorReset
}

func handoffBranchSuffix(r handoff.RepoAssessment) string {
	if r.Branch == "" {
		return ""
	}
	return fmt.Sprintf(" %s(%s)%s", cliutil.ColorGray, r.Branch, cliutil.ColorReset)
}

// summarizeHandoffReasons renders the blocker tally as "2 uncommitted, 1 stashed"
// in a stable order so the line does not reshuffle between runs.
func summarizeHandoffReasons(a *handoff.Assessment) string {
	counts := a.ReasonCounts()
	reasons := make([]string, 0, len(counts))
	for reason := range counts {
		reasons = append(reasons, string(reason))
	}
	sort.Strings(reasons)

	parts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		parts = append(parts, fmt.Sprintf("%d %s", counts[handoff.Reason(reason)], reason))
	}
	return strings.Join(parts, ", ")
}
