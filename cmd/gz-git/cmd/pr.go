// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
)

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Create and inspect pull requests / merge requests",
	Long: cliutil.QuickStartHelp(`  # Create PRs from the current branch of every scanned repo
  gz-git pr create

  # Dry-run a workspace
  gz-git pr create -n ~/work`),
}

func init() {
	rootCmd.AddCommand(prCmd)
}
