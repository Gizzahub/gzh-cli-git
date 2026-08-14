// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
)

// integrateCmd groups the read-only queue and the later check/run commands.
var integrateCmd = &cobra.Command{
	Use:   "integrate",
	Short: "Inspect and apply task-branch integration",
	Long: cliutil.QuickStartHelp(`  # What is waiting to land?
  gz-git integrate queue

  # Compare against a specific base
  gz-git integrate queue --base origin/develop

integrate answers a different question than branch list. branch list is a
bulk inventory. integrate queue asks which unfinished task branches are
not the base, the remote HEAD, or the integration branch — and whether
they are stale, conflicting, or expired.`),
	Args: cobra.NoArgs,
}

func init() {
	rootCmd.AddCommand(integrateCmd)
}
