package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-git/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-git/pkg/merge"
	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

var (
	mergeStrategy     string
	mergeFastForward  bool
	mergeNoCommit     bool
	mergeSquash       bool
	mergeMessage      string
)

// doCmd represents the merge do command
var doCmd = &cobra.Command{
	Use:   "do <source-branch>",
	Short: "Execute a merge operation",
	Long: `Merge a source branch into the current branch.

Supports various merge strategies and options:
- Fast-forward merge (if possible)
- Three-way merge with auto-merge
- Squash merge (combine all commits)
- No-commit merge (stage without committing)`,
	Example: `  # Merge feature branch
  gzh-git merge do feature/new-feature

  # Fast-forward only
  gzh-git merge do feature/new-feature --ff-only

  # Squash merge
  gzh-git merge do feature/new-feature --squash

  # Merge without committing
  gzh-git merge do feature/new-feature --no-commit

  # Merge with custom message
  gzh-git merge do feature/new-feature --message "Merge feature X"`,
	Args: cobra.ExactArgs(1),
	RunE: runMergeDo,
}

func init() {
	mergeCmd.AddCommand(doCmd)

	doCmd.Flags().StringVar(&mergeStrategy, "strategy", "auto", "merge strategy (auto|ours|theirs|recursive)")
	doCmd.Flags().BoolVar(&mergeFastForward, "ff-only", false, "only allow fast-forward merge")
	doCmd.Flags().BoolVar(&mergeNoCommit, "no-commit", false, "perform merge but don't commit")
	doCmd.Flags().BoolVar(&mergeSquash, "squash", false, "squash all commits into one")
	doCmd.Flags().StringVarP(&mergeMessage, "message", "m", "", "custom merge commit message")
}

func runMergeDo(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	sourceBranch := args[0]

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

	// Create executor
	executor := gitcmd.NewExecutor()

	// Get current branch
	branchResult, err := executor.Run(ctx, repo.Path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}
	currentBranch := strings.TrimSpace(branchResult.Stdout)

	// Create merge manager and detector
	detector := merge.NewConflictDetector(executor)
	mgr := merge.NewMergeManager(executor, detector)

	// Parse merge strategy
	strategy, err := parseMergeStrategy(mergeStrategy)
	if err != nil {
		return err
	}

	// Build options
	opts := merge.MergeOptions{
		Source:           sourceBranch,
		Target:           currentBranch,
		Strategy:         strategy,
		AllowFastForward: !mergeFastForward || strategy == merge.StrategyFastForward,
		NoCommit:         mergeNoCommit,
		Squash:           mergeSquash,
		CommitMessage:    mergeMessage,
	}

	if !quiet {
		fmt.Printf("Merging '%s' into '%s'...\n", sourceBranch, currentBranch)
		if mergeFastForward {
			fmt.Println("(fast-forward only)")
		}
		if mergeSquash {
			fmt.Println("(squash mode)")
		}
	}

	// Perform merge
	result, err := mgr.Merge(ctx, repo, opts)
	if err != nil {
		return fmt.Errorf("failed to merge: %w", err)
	}

	// Display result
	if !quiet {
		fmt.Println()
		if result.Success {
			fmt.Println("✅ Merge successful!")
			if result.Strategy == merge.StrategyFastForward {
				fmt.Println("   Mode: Fast-forward")
			} else {
				fmt.Println("   Mode: Three-way merge")
			}
			if result.CommitHash != "" {
				fmt.Printf("   Commit: %s\n", result.CommitHash[:8])
			}
		} else {
			fmt.Println("⚠️  Merge completed with conflicts")
			fmt.Printf("   Conflicts: %d files\n", len(result.Conflicts))
			fmt.Println()
			fmt.Println("Conflicting files:")
			for _, conflict := range result.Conflicts {
				fmt.Printf("  - %s\n", conflict.FilePath)
			}
			fmt.Println()
			fmt.Println("Resolve conflicts and commit, or run 'gzh-git merge abort' to cancel.")
		}
	}

	if !result.Success {
		return fmt.Errorf("merge has conflicts that need resolution")
	}

	return nil
}

// parseMergeStrategy converts string to MergeStrategy enum
func parseMergeStrategy(strategy string) (merge.MergeStrategy, error) {
	switch strategy {
	case "auto", "recursive":
		return merge.StrategyRecursive, nil
	case "ours":
		return merge.StrategyOurs, nil
	case "theirs":
		return merge.StrategyTheirs, nil
	case "ff", "fast-forward":
		return merge.StrategyFastForward, nil
	default:
		return merge.StrategyRecursive, fmt.Errorf("unknown merge strategy: %s (valid: auto, ours, theirs, recursive, fast-forward)", strategy)
	}
}
