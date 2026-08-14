// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/integrate"
)

var (
	integrateQueueBase       string
	integrateQueueExpiryDays int
	integrateQueueNoFetch    bool
)

var integrateQueueCmd = &cobra.Command{
	Use:   "queue",
	Short: "List unfinished task branches",
	Long: cliutil.QuickStartHelp(`  # Scan the current repository
  gz-git integrate queue

  # Compare against a specific base
  gz-git integrate queue --base origin/develop

  # Skip fetch (offline / already fetched)
  gz-git integrate queue --no-fetch

  # Hook-style: only problem rows, stay quiet when the queue is large
  gz-git integrate queue --quiet

This is a read-only report. It does not create, delete, or check out
branches. An empty queue is success (exit 0), not a grep-style failure.

Exit Codes:
  0  empty queue, or no conflicts/expired entries
  1  conflict or expired entries, or the base ref is missing
  2  the scan itself could not run`),
	Args: cobra.NoArgs,
	RunE: runIntegrateQueue,
}

func init() {
	integrateCmd.AddCommand(integrateQueueCmd)
	integrateQueueCmd.Flags().StringVar(&integrateQueueBase, "base", "", "comparison base (default: remote HEAD)")
	integrateQueueCmd.Flags().IntVar(&integrateQueueExpiryDays, "expiry-days", integrate.DefaultExpiryDays, "age in days after which a branch is expired")
	integrateQueueCmd.Flags().BoolVar(&integrateQueueNoFetch, "no-fetch", false, "do not fetch before scanning")
}

func runIntegrateQueue(cmd *cobra.Command, _ []string) error {
	if integrateQueueExpiryDays < 1 {
		return cliutil.NewExitError(cliutil.ExitToolError, fmt.Errorf("--expiry-days must be >= 1"))
	}

	ctx := cmdContext(cmd)
	dir, err := os.Getwd()
	if err != nil {
		return cliutil.NewExitError(cliutil.ExitToolError, err)
	}

	report, err := integrate.CollectQueue(ctx, gitcmd.NewExecutor(), integrate.QueueOptions{
		RepoPath:   dir,
		Base:       integrateQueueBase,
		ExpiryDays: integrateQueueExpiryDays,
		NoFetch:    integrateQueueNoFetch,
		Quiet:      quiet,
	})
	if err != nil {
		if quiet {
			return nil
		}
		return cliutil.NewExitError(2, err)
	}
	if report.QuietSkipped {
		return nil
	}
	if report.BaseMissing {
		if quiet {
			return nil
		}
		fmt.Fprint(cmd.ErrOrStderr(), integrate.FormatQueue(report))
		return cliutil.NewExitError(1, fmt.Errorf("base ref not found"))
	}

	if quiet {
		if len(report.Entries) == 0 {
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "unfinished branches (base=%s, expiry=%dd):\n", report.Base, report.ExpiryDays)
		fmt.Fprint(cmd.OutOrStdout(), integrate.FormatQueue(report))
	} else {
		fmt.Fprint(cmd.OutOrStdout(), integrate.FormatQueue(report))
	}

	if report.ConflictCount > 0 || report.ExpiredCount > 0 {
		return cliutil.NewExitError(1, fmt.Errorf("queue has %d conflict(s) and %d expired", report.ConflictCount, report.ExpiredCount))
	}
	return nil
}
