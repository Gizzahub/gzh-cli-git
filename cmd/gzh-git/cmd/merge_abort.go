package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-git/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-git/pkg/merge"
	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

// abortCmd represents the merge abort command
var abortCmd = &cobra.Command{
	Use:   "abort",
	Short: "Abort an in-progress merge",
	Long: `Abort the current merge operation and return to pre-merge state.

This command is useful when a merge results in conflicts and you decide
not to resolve them. It will restore your working directory to the state
before the merge began.`,
	Example: `  # Abort current merge
  gzh-git merge abort`,
	Args: cobra.NoArgs,
	RunE: runMergeAbort,
}

func init() {
	mergeCmd.AddCommand(abortCmd)
}

func runMergeAbort(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

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

	// Create merge manager and detector
	executor := gitcmd.NewExecutor()
	detector := merge.NewConflictDetector(executor)
	mgr := merge.NewMergeManager(executor, detector)

	if !quiet {
		fmt.Println("Aborting merge...")
	}

	// Abort merge
	if err := mgr.AbortMerge(ctx, repo); err != nil {
		return fmt.Errorf("failed to abort merge: %w", err)
	}

	if !quiet {
		fmt.Println("✅ Merge aborted successfully")
		fmt.Println("   Working directory restored to pre-merge state")
	}

	return nil
}
