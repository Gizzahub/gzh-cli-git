// Package cmd implements the CLI commands for gzh-git.
package cmd

import (
	"github.com/spf13/cobra"
)

// multiCmd represents the multi command group for bulk operations
var multiCmd = &cobra.Command{
	Use:   "multi",
	Short: "Bulk operations across multiple repositories",
	Long: `Multi-repository operations that scan and process multiple Git repositories.

These commands recursively find Git repositories in the current directory
and perform operations on all of them in parallel.

Examples:
  gz-git multi switch develop    # Switch all repos to develop branch
  gz-git multi switch main --dry-run  # Preview branch switch

Use "gz-git multi [command] --help" for more information about a command.`,
}

func init() {
	rootCmd.AddCommand(multiCmd)
}
