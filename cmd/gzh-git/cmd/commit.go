package cmd

import (
	"github.com/spf13/cobra"
)

// commitCmd represents the commit command group
var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Commit automation commands",
	Long: `Automate commit message creation, validation, and template management.

This command provides subcommands for:
  - Automatic commit message generation from changes
  - Commit message validation against templates
  - Template management (list, show, validate)`,
	Example: `  # Auto-generate and commit with conventional commits
  gzh-git commit auto

  # Validate a commit message
  gzh-git commit validate "feat(auth): add login endpoint"

  # List available templates
  gzh-git commit template list`,
}

func init() {
	rootCmd.AddCommand(commitCmd)
}
