package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-git/pkg/branch"
	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

var (
	deleteForce  bool
	deleteRemote bool
)

// deleteCmd represents the branch delete command
var deleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a branch",
	Long: `Delete a Git branch (local or remote).

Protected branches (main, master, develop, release/*, hotfix/*) cannot be deleted
unless --force is used.`,
	Example: `  # Delete a local branch
  gzh-git branch delete feature/old-feature

  # Force delete (even if not merged)
  gzh-git branch delete feature/experimental --force

  # Delete remote branch
  gzh-git branch delete feature/done --remote`,
	Args: cobra.ExactArgs(1),
	RunE: runBranchDelete,
}

func init() {
	branchCmd.AddCommand(deleteCmd)

	deleteCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "force delete even if not merged")
	deleteCmd.Flags().BoolVarP(&deleteRemote, "remote", "r", false, "delete remote branch")
}

func runBranchDelete(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	branchName := args[0]

	// Get repository path
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Create client
	client := repository.NewClient()

	// Check if it's a repository
	if !client.IsRepository(ctx, absPath) {
		return fmt.Errorf("not a git repository: %s", absPath)
	}

	// Open repository
	repo, err := client.Open(ctx, absPath)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Create branch manager
	mgr := branch.NewManager()

	// Delete branch
	opts := branch.DeleteOptions{
		Name:   branchName,
		Force:  deleteForce,
		Remote: deleteRemote,
	}

	if !quiet {
		target := "local"
		if deleteRemote {
			target = "remote"
		}
		fmt.Printf("Deleting %s branch '%s'...\n", target, branchName)
	}

	if err := mgr.Delete(ctx, repo, opts); err != nil {
		return fmt.Errorf("failed to delete branch: %w", err)
	}

	if !quiet {
		fmt.Printf("✅ Branch '%s' deleted successfully\n", branchName)
	}

	return nil
}
