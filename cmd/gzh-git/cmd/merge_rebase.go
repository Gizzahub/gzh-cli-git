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

var (
	rebaseOnto      string
	rebaseContinue  bool
	rebaseSkip      bool
	rebaseAbort     bool
)

// rebaseCmd represents the merge rebase command
var rebaseCmd = &cobra.Command{
	Use:   "rebase [branch]",
	Short: "Rebase current branch",
	Long: `Rebase the current branch onto another branch.

Rebasing rewrites commit history by replaying commits on top of another branch.
This creates a linear history without merge commits.`,
	Example: `  # Rebase onto main
  gzh-git merge rebase main

  # Rebase onto specific commit
  gzh-git merge rebase --onto abc123 main

  # Continue after resolving conflicts
  gzh-git merge rebase --continue

  # Skip current commit
  gzh-git merge rebase --skip

  # Abort rebase
  gzh-git merge rebase --abort`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMergeRebase,
}

func init() {
	mergeCmd.AddCommand(rebaseCmd)

	rebaseCmd.Flags().StringVar(&rebaseOnto, "onto", "", "rebase onto specific commit")
	rebaseCmd.Flags().BoolVar(&rebaseContinue, "continue", false, "continue rebase after resolving conflicts")
	rebaseCmd.Flags().BoolVar(&rebaseSkip, "skip", false, "skip current commit and continue")
	rebaseCmd.Flags().BoolVar(&rebaseAbort, "abort", false, "abort rebase and return to original state")
}

func runMergeRebase(cmd *cobra.Command, args []string) error {
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

	// Create rebase manager
	mgr := merge.NewRebaseManager(gitcmd.NewExecutor())

	// Handle rebase operations
	if rebaseAbort {
		if !quiet {
			fmt.Println("Aborting rebase...")
		}
		if err := mgr.Abort(ctx, repo); err != nil {
			return fmt.Errorf("failed to abort rebase: %w", err)
		}
		if !quiet {
			fmt.Println("✅ Rebase aborted successfully")
		}
		return nil
	}

	if rebaseContinue {
		if !quiet {
			fmt.Println("Continuing rebase...")
		}
		result, err := mgr.Continue(ctx, repo)
		if err != nil {
			return fmt.Errorf("failed to continue rebase: %w", err)
		}
		displayRebaseResult(result)
		return nil
	}

	if rebaseSkip {
		if !quiet {
			fmt.Println("Skipping current commit...")
		}
		result, err := mgr.Skip(ctx, repo)
		if err != nil {
			return fmt.Errorf("failed to skip commit: %w", err)
		}
		displayRebaseResult(result)
		return nil
	}

	// Regular rebase
	if len(args) == 0 {
		return fmt.Errorf("branch name required (or use --continue, --skip, --abort)")
	}

	branch := args[0]

	// Build options
	opts := merge.RebaseOptions{
		Branch: branch,
		Onto:   rebaseOnto,
	}

	if !quiet {
		if rebaseOnto != "" {
			fmt.Printf("Rebasing onto %s (from %s)...\n", rebaseOnto, branch)
		} else {
			fmt.Printf("Rebasing onto %s...\n", branch)
		}
	}

	// Perform rebase
	result, err := mgr.Rebase(ctx, repo, opts)
	if err != nil {
		return fmt.Errorf("failed to rebase: %w", err)
	}

	displayRebaseResult(result)
	return nil
}

func displayRebaseResult(result *merge.RebaseResult) {
	if quiet {
		return
	}

	fmt.Println()
	if result.Success {
		fmt.Println("✅ Rebase successful!")
		if result.CommitsRebased > 0 {
			fmt.Printf("   Commits rebased: %d\n", result.CommitsRebased)
		}
	} else {
		fmt.Println("⚠️  Rebase stopped due to conflicts")
		if result.ConflictsFound > 0 {
			fmt.Printf("   Conflicts: %d\n", result.ConflictsFound)
			fmt.Println()
			fmt.Println("Resolve conflicts, stage changes, then run:")
			fmt.Println("  gzh-git merge rebase --continue")
			fmt.Println()
			fmt.Println("Or skip this commit:")
			fmt.Println("  gzh-git merge rebase --skip")
			fmt.Println()
			fmt.Println("Or abort the rebase:")
			fmt.Println("  gzh-git merge rebase --abort")
		}
	}
}
