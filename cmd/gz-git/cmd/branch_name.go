// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/branch"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
)

var branchNameKind string

// branchNameCmd builds a branch name and prints it. It creates nothing: the
// name is meant to be handed to `gz-git switch --create` or to plain git.
var branchNameCmd = &cobra.Command{
	Use:   "name <task>",
	Short: "Build a conventional branch name for a task",
	Long: cliutil.QuickStartHelp(`  # The shared branch for a task
  gz-git branch name task-001-product-unit
  feat/task-001-product-unit

  # This machine's own branch for it
  gz-git branch name task-001-product-unit --kind device
  feat/task-001-product-unit/dave-office

  # This agent's own branch
  gz-git branch name task-001-product-unit --kind agent
  agent/task-001-product-unit/hermes-01

  # Create it everywhere the task spans
  gz-git switch "$(gz-git branch name task-001 --kind device)" --create`) + `
This prints a name and creates nothing, so it composes with 'gz-git switch
--create' for bulk work and with plain git for one repository.

Kinds
  work     the branch a task lives on when it has one writer   feat/{task}
  device   one machine's slice of a shared task       feat/{task}/{device}
  agent    one agent's slice, kept out of a person's  agent/{task}/{agent}

The device and agent segments come from the resolved identity — 'identity.device'
and 'identity.agent' in global config, or GZ_GIT_DEVICE and GZ_GIT_AGENT. A
device or agent branch is refused when its segment is unnamed, since the result
would be the shared branch again under a longer name.

Every substituted value is slugified: lowercased, with runs of anything outside
[a-z0-9] collapsed to a dash. A hostname like 'Daves-MacBook.local' is not a
legal branch name, and it is the default device name, so the alternative is a
template that works on one machine and fails on the next.

Override the templates per kind under 'branch.naming' in any config layer:

  branch:
    naming:
      device: wip/{device}/{task}

Placeholders are {task}, {device} and {agent}. A misspelled one is reported
rather than left in the name.

Exit codes:
  0  the name was printed
  2  the kind, the task or a template could not produce a valid branch name
`,
	Args: cobra.ExactArgs(1),
	RunE: runBranchName,
}

func init() {
	branchCmd.AddCommand(branchNameCmd)

	branchNameCmd.Flags().StringVar(&branchNameKind, "kind", "work",
		"branch role: work, device or agent")
}

func runBranchName(cmd *cobra.Command, args []string) error {
	kind, err := branch.ParseKind(branchNameKind)
	if err != nil {
		return cliutil.NewExitError(2, err)
	}

	// A missing config is not an error here: the defaults spell the convention,
	// and the identity still resolves from the environment and the hostname.
	effective, _ := LoadEffectiveConfig(cmd, nil)

	var naming *branch.Naming
	if effective != nil {
		naming = effective.Branch.Naming
	}

	name, err := naming.Resolve(kind, args[0], pushIdentity(effective))
	if err != nil {
		return cliutil.NewExitError(2, err)
	}

	fmt.Println(name)

	return nil
}
