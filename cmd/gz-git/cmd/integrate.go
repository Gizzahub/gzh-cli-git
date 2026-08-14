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

  # Is this branch ready?
  gz-git integrate check

  # Fast-forward the target and reclaim the task branch
  gz-git integrate run

integrate answers a different question than branch list. queue lists
unfinished task branches, check is the readiness gate, and run
fast-forwards then reclaims.`),
	Args: cobra.NoArgs,
}

func init() {
	rootCmd.AddCommand(integrateCmd)
}
