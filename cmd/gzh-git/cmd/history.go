package cmd

import (
	"github.com/spf13/cobra"
)

// historyCmd represents the history command group
var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "History analysis commands",
	Long: `Analyze Git history including commit statistics, contributors, and file changes.

This command provides subcommands for:
  - Commit statistics and trends
  - Contributor analysis and rankings
  - File history tracking
  - Line-by-line authorship (blame)`,
	Example: `  # Show commit statistics
  gzh-git history stats

  # List top contributors
  gzh-git history contributors --top 10

  # View file history
  gzh-git history file src/main.go

  # Show line-by-line authorship
  gzh-git history blame src/main.go`,
}

func init() {
	rootCmd.AddCommand(historyCmd)
}
