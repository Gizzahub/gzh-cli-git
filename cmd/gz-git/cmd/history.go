package cmd

import (
	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
)

// historyCmd represents the history command group.
var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "History analysis commands",
	Long: cliutil.QuickStartHelp(`  # Show commit statistics
  gz-git history stats

  # List top contributors
  gz-git history contributors --top 10

  # View file history
  gz-git history file src/main.go`),
	Example: ``,
	Args:    cobra.NoArgs,
}

func init() {
	rootCmd.AddCommand(historyCmd)
}
