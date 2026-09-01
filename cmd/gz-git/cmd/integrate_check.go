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
	integrateCheckTarget           string
	integrateCheckDirectToDefault  bool
	integrateCheckRelease          bool
	integrateCheckAllowSkipped     bool
	integrateCheckControllerConfig string
)

var integrateCheckCmd = &cobra.Command{
	Use:   "check [branch]",
	Short: "Verify a task branch is ready to integrate",
	Long: cliutil.QuickStartHelp(`  # Check the current branch against the integration branch
  gz-git integrate check

  # Required when no integration branch can be resolved
  gz-git integrate check --target origin/main --direct-to-default

This is read-only. It never pushes and never reclaims.

Exit Codes:
  0  READY
  1  NOT READY, or --target required
  2  the check itself could not run`),
	Args: cobra.MaximumNArgs(1),
	RunE: runIntegrateCheck,
}

func init() {
	integrateCmd.AddCommand(integrateCheckCmd)
	integrateCheckCmd.Flags().StringVar(&integrateCheckTarget, "target", "", "integration target (required when none can be resolved)")
	integrateCheckCmd.Flags().BoolVar(&integrateCheckDirectToDefault, "direct-to-default", false, "allow targeting the default branch when no integration branch exists")
	integrateCheckCmd.Flags().BoolVar(&integrateCheckRelease, "release", false, "promote the integration branch onto the default branch")
	integrateCheckCmd.Flags().BoolVar(&integrateCheckAllowSkipped, "allow-skipped-checks", false, "allow a repo with no check/lint gate, and downgrade SKIPPED CHECK banners to warnings")
	integrateCheckCmd.Flags().StringVar(&integrateCheckControllerConfig, "controller-config", "", "explicit devbox/controller config; never searched automatically")
}

func runIntegrateCheck(cmd *cobra.Command, args []string) error {
	ctx := cmdContext(cmd)
	dir, err := os.Getwd()
	if err != nil {
		return cliutil.NewExitError(cliutil.ExitToolError, err)
	}
	branch := ""
	if len(args) == 1 {
		branch = args[0]
	}

	report, err := integrate.Check(ctx, gitcmd.NewExecutor(), integrate.CheckOptions{
		RepoPath:           dir,
		Branch:             branch,
		Target:             integrateCheckTarget,
		DirectToDefault:    integrateCheckDirectToDefault,
		Release:            integrateCheckRelease,
		AllowSkippedChecks: integrateCheckAllowSkipped,
		ControllerConfig:   integrateCheckControllerConfig,
	})
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "--target") || strings.Contains(msg, "integration branch") {
			if !quiet {
				fmt.Fprintln(cmd.ErrOrStderr(), "integrate check:", msg)
			}
			return cliutil.NewExitError(1, err)
		}
		return cliutil.NewExitError(2, err)
	}

	if !quiet {
		fmt.Fprint(cmd.OutOrStdout(), integrate.FormatCheck(report))
	}
	if !report.Ready {
		return cliutil.NewExitError(1, fmt.Errorf("not ready"))
	}
	return nil
}
