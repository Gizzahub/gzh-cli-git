package cmd

import (
	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
)

// branchCmd represents the branch command group
var branchCmd = &cobra.Command{
	Use:   "branch",
	Short: "Branch management commands",
	Long: cliutil.QuickStartHelp(`  # List branches in current repo
  gz-git branch list

  # List all branches including remote
  gz-git branch list -a

  # BULK: List branches across multiple repos
  gz-git branch list .

  # Build a conventional branch name for a task
  gz-git branch name task-001 --kind device

  # Clean up branches
  gz-git cleanup branch --merged`) + `

Policy: branch create/delete are not exposed — use plain git for single-repo
create, gz-git switch --create for bulk create, and gz-git cleanup branch for
bulk deletion of merged/stale branches.

'branch name' does not create anything either. It prints the name a task's
branch should have on this machine or under this agent, which plain git cannot
work out, and leaves creating it to the commands above.
`,
	Example: ``,
	Args:    cobra.NoArgs,
}

func init() {
	rootCmd.AddCommand(branchCmd)
}
