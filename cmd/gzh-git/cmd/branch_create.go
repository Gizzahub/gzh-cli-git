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
	createBase     string
	createWorktree string
	createTrack    bool
)

// createCmd represents the branch create command
var createCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new branch",
	Long: `Create a new Git branch, optionally with a worktree.

The branch is created from the current HEAD or a specified base branch.
Optionally create a worktree for parallel development.`,
	Example: `  # Create a new branch
  gzh-git branch create feature/new-feature

  # Create from specific base
  gzh-git branch create feature/auth --base main

  # Create with worktree
  gzh-git branch create feature/auth --worktree ./worktrees/auth

  # Create and track upstream
  gzh-git branch create feature/api --track`,
	Args: cobra.ExactArgs(1),
	RunE: runBranchCreate,
}

func init() {
	branchCmd.AddCommand(createCmd)

	createCmd.Flags().StringVarP(&createBase, "base", "b", "", "base branch (default: current branch)")
	createCmd.Flags().StringVar(&createWorktree, "worktree", "", "create worktree at path")
	createCmd.Flags().BoolVar(&createTrack, "track", false, "set up tracking with upstream")
}

func runBranchCreate(cmd *cobra.Command, args []string) error {
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

	// Create branch
	opts := branch.CreateOptions{
		Name:     branchName,
		StartRef: createBase,
		Track:    createTrack,
	}

	if !quiet {
		fmt.Printf("Creating branch '%s'...\n", branchName)
	}

	if err := mgr.Create(ctx, repo, opts); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	if !quiet {
		fmt.Printf("✅ Branch '%s' created successfully\n", branchName)
	}

	// Create worktree if requested
	if createWorktree != "" {
		worktreePath, err := filepath.Abs(createWorktree)
		if err != nil {
			return fmt.Errorf("failed to resolve worktree path: %w", err)
		}

		if !quiet {
			fmt.Printf("\nCreating worktree at '%s'...\n", worktreePath)
		}

		wtMgr := branch.NewWorktreeManager()
		wtOpts := branch.AddOptions{
			Path:   worktreePath,
			Branch: branchName,
			Force:  false,
		}

		wt, err := wtMgr.Add(ctx, repo, wtOpts)
		if err != nil {
			return fmt.Errorf("failed to create worktree: %w", err)
		}
		_ = wt // Worktree created successfully

		if !quiet {
			fmt.Printf("✅ Worktree created at '%s'\n", worktreePath)
		}
	}

	return nil
}
