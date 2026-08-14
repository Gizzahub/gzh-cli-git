package cmd

import (
	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
)

// cleanupCmd represents the cleanup command group.
var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Clean up various Git resources",
	Long: cliutil.QuickStartHelp(`  # Clean up merged branches (dry-run)
  gz-git cleanup branch --merged

  # Clean up stale branches (>30 days)
  gz-git cleanup branch --stale

  # BULK: Clean up all repos in directory
  gz-git cleanup branch --merged --force .`),
	Example: ``,
	Args:    cobra.NoArgs,
}

func init() {
	rootCmd.AddCommand(cleanupCmd)
}
