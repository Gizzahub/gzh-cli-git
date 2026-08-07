// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
)

// handoffCmd groups the commands that move work between machines and agents.
var handoffCmd = &cobra.Command{
	Use:   "handoff",
	Short: "Move work safely between machines and agents",
	Long: cliutil.QuickStartHelp(`  # Can I walk away from this workspace?
  gz-git handoff check

  # Before leaving: commit outstanding work and push it
  gz-git handoff end

  # After arriving: bring every repository up to date
  gz-git handoff start

handoff answers a different question than status. status reports how healthy a
repository is; handoff reports whether anything in it exists only on this
machine — uncommitted files, unpushed commits, stash entries — because that is
what another device or agent can never see.

Unlike sync, which aligns the set of repositories against a config, handoff
operates on the work state of the repositories already present.`),
}

func init() {
	rootCmd.AddCommand(handoffCmd)
}
