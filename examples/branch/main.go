// Package main demonstrates branch management using gzh-cli-git library.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/gizzahub/gzh-cli-git/pkg/branch"
	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

func main() {
	ctx := context.Background()

	// Get repository path from args or use current directory
	repoPath := "."
	if len(os.Args) >= 2 {
		repoPath = os.Args[1]
	}

	// Create clients
	repoClient := repository.NewClient()
	branchManager := branch.NewManager()

	// Open repository
	repo, err := repoClient.Open(ctx, repoPath)
	if err != nil {
		log.Fatalf("Failed to open repository: %v", err)
	}

	fmt.Printf("Repository: %s\n\n", repo.Path)

	// Example 1: List all branches
	fmt.Println("=== Example 1: List All Branches ===")
	branches, err := branchManager.List(ctx, repo, branch.ListOptions{
		All: true, // Include remote branches
	})
	if err != nil {
		log.Fatalf("Failed to list branches: %v", err)
	}

	fmt.Println("Local branches:")
	for _, b := range branches {
		if !b.IsRemote {
			current := ""
			if b.IsCurrent {
				current = " (current)"
			}
			fmt.Printf("  %s%s\n", b.Name, current)
		}
	}
	fmt.Println()

	remoteBranches := []branch.Branch{}
	for _, b := range branches {
		if b.IsRemote {
			remoteBranches = append(remoteBranches, b)
		}
	}

	if len(remoteBranches) > 0 {
		fmt.Println("Remote branches:")
		for _, b := range remoteBranches {
			fmt.Printf("  %s\n", b.Name)
		}
		fmt.Println()
	}

	// Example 2: Get current branch
	fmt.Println("=== Example 2: Current Branch ===")
	current, err := branchManager.GetCurrent(ctx, repo)
	if err != nil {
		log.Fatalf("Failed to get current branch: %v", err)
	}
	fmt.Printf("Current branch: %s\n", current.Name)
	fmt.Println()

	// Example 3: Check if branch exists
	fmt.Println("=== Example 3: Check Branch Existence ===")
	branchName := "main"
	exists, err := branchManager.Exists(ctx, repo, branchName)
	if err != nil {
		log.Printf("Warning: Failed to check branch: %v", err)
	} else {
		if exists {
			fmt.Printf("✓ Branch '%s' exists\n", branchName)
		} else {
			fmt.Printf("✗ Branch '%s' does not exist\n", branchName)
		}
	}
	fmt.Println()

	// Example 4: Create branch (dry-run example)
	fmt.Println("=== Example 4: Create Branch (Example) ===")
	newBranch := "feature/example-branch"
	fmt.Printf("To create a new branch '%s':\n", newBranch)
	fmt.Println()
	fmt.Println("Using gzh-cli-git library:")
	fmt.Println("  err := branchManager.Create(ctx, repo, branch.CreateOptions{")
	fmt.Println("      Name:       \"feature/example-branch\",")
	fmt.Println("      StartPoint: \"main\",")
	fmt.Println("  })")
	fmt.Println()
	fmt.Println("Using CLI:")
	fmt.Printf("  gzh-git branch create %s\n", newBranch)
	fmt.Println()

	// Example 5: List worktrees (if any)
	fmt.Println("=== Example 5: List Worktrees ===")
	worktrees, err := branchManager.ListWorktrees(ctx, repo)
	if err != nil {
		log.Printf("Warning: Failed to list worktrees: %v", err)
	} else if len(worktrees) > 0 {
		fmt.Println("Active worktrees:")
		for _, wt := range worktrees {
			fmt.Printf("  %s -> %s (branch: %s)\n", wt.Path, wt.Branch, wt.Commit)
		}
	} else {
		fmt.Println("No additional worktrees (only main working tree)")
	}
	fmt.Println()

	fmt.Println("Tip: Use worktrees for parallel development:")
	fmt.Println("  gzh-git branch create feature/new --worktree /path/to/worktree")
}
