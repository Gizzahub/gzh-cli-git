// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/integrate"
)

var (
	integrateRunTarget          string
	integrateRunDirectToDefault bool
	integrateRunRelease         bool
	integrateRunAllowSkipped    bool
)

var integrateRunCmd = &cobra.Command{
	Use:   "run [branch]",
	Short: "Fast-forward integrate and reclaim the task branch",
	Long: cliutil.QuickStartHelp(`  # Integrate the current branch
  gz-git integrate run

  # Required when no integration branch can be resolved
  gz-git integrate run --target origin/main --direct-to-default

Reclaim only runs for names matching the repo-root taskPattern.
No declaration means reclaim nothing.

Exit Codes:
  0  integrated (reclaim finished or intentionally skipped)
  1  not ready, or the integrate itself failed
  2  usage or execution error
  3  integrated, but reclaim did not finish`),
	Args: cobra.MaximumNArgs(1),
	RunE: runIntegrateRun,
}

func init() {
	integrateCmd.AddCommand(integrateRunCmd)
	integrateRunCmd.Flags().StringVar(&integrateRunTarget, "target", "", "integration target (required when none can be resolved)")
	integrateRunCmd.Flags().BoolVar(&integrateRunDirectToDefault, "direct-to-default", false, "allow targeting the default branch when no integration branch exists")
	integrateRunCmd.Flags().BoolVar(&integrateRunRelease, "release", false, "promote the integration branch onto the default branch")
	integrateRunCmd.Flags().BoolVar(&integrateRunAllowSkipped, "allow-skipped-checks", false, "downgrade SKIPPED CHECK banners to warnings")
}

func runIntegrateRun(cmd *cobra.Command, args []string) error {
	ctx := cmdContext(cmd)
	dir, err := os.Getwd()
	if err != nil {
		return cliutil.NewExitError(cliutil.ExitToolError, err)
	}
	branch := ""
	if len(args) == 1 {
		branch = args[0]
	}

	report, err := integrate.Run(ctx, gitcmd.NewExecutor(), integrate.RunOptions{
		CheckOptions: integrate.CheckOptions{
			RepoPath:           dir,
			Branch:             branch,
			Target:             integrateRunTarget,
			DirectToDefault:    integrateRunDirectToDefault,
			Release:            integrateRunRelease,
			AllowSkippedChecks: integrateRunAllowSkipped,
		},
	})
	if report != nil && !quiet {
		fmt.Fprint(cmd.OutOrStdout(), integrate.FormatRun(report))
	}
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "not ready") || strings.Contains(msg, "--target") || strings.Contains(msg, "integration branch") {
			return cliutil.NewExitError(1, err)
		}
		return cliutil.NewExitError(2, err)
	}
	if report != nil && report.Integrated && report.Reclaim.Incomplete() {
		return cliutil.NewExitError(cliutil.ExitReclaimIncomplete, fmt.Errorf("reclaim incomplete"))
	}
	return nil
}
