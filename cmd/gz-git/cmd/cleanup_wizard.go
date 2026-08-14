// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/wizard"
)

// cleanupWizardCmd represents the cleanup wizard command.
var cleanupWizardCmd = &cobra.Command{
	Use:   "wizard [directory]",
	Short: "Interactive wizard for branch cleanup",
	Long: cliutil.QuickStartHelp(`  # Start wizard in current directory
  gz-git cleanup wizard

  # Start wizard in specific directory
  gz-git cleanup wizard ~/projects`),
	Args: cobra.MaximumNArgs(1),
	RunE: runCleanupWizard,
}

func init() {
	cleanupCmd.AddCommand(cleanupWizardCmd)
}

func runCleanupWizard(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Get directory from args or use current
	directory := "."
	if len(args) > 0 {
		directory = args[0]
	}

	// Run the wizard
	w := wizard.NewBranchCleanupWizard(directory)
	result, err := w.Run(ctx)
	if err != nil {
		return fmt.Errorf("wizard failed: %w", err)
	}

	// Print final message
	if result.BranchesDeleted > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\nCleanup completed successfully!\n")
	}

	return nil
}
