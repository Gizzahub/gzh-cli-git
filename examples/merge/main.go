// Package main demonstrates merge and conflict detection using gzh-cli-git library.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/gizzahub/gzh-cli-git/pkg/branch"
	"github.com/gizzahub/gzh-cli-git/pkg/merge"
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
	mergeManager := merge.NewManager()

	// Open repository
	repo, err := repoClient.Open(ctx, repoPath)
	if err != nil {
		log.Fatalf("Failed to open repository: %v", err)
	}

	fmt.Printf("Repository: %s\n\n", repo.Path)

	// Get current branch
	current, err := branchManager.GetCurrent(ctx, repo)
	if err != nil {
		log.Fatalf("Failed to get current branch: %v", err)
	}

	fmt.Printf("Current branch: %s\n\n", current.Name)

	// Example 1: Check merge status
	fmt.Println("=== Example 1: Check Merge Status ===")

	inProgress, err := mergeManager.InProgress(ctx, repo)
	if err != nil {
		log.Printf("Warning: Failed to check merge status: %v", err)
	} else {
		if inProgress {
			fmt.Println("⚠️  Merge in progress")
			fmt.Println("Complete or abort the merge before proceeding")
		} else {
			fmt.Println("✓ No merge in progress")
		}
	}
	fmt.Println()

	// Example 2: Detect conflicts before merging
	fmt.Println("=== Example 2: Pre-Merge Conflict Detection ===")

	// Check if there's a main/master branch to test with
	branches, _ := branchManager.List(ctx, repo, branch.ListOptions{})

	var targetBranch string
	for _, b := range branches {
		if !b.IsRemote && b.Name != current.Name && (b.Name == "main" || b.Name == "master" || b.Name == "develop") {
			targetBranch = b.Name
			break
		}
	}

	if targetBranch != "" {
		fmt.Printf("Checking for potential conflicts with '%s'...\n", targetBranch)

		result, err := mergeManager.DetectConflicts(ctx, repo, merge.DetectOptions{
			Source: current.Name,
			Target: targetBranch,
		})
		if err != nil {
			log.Printf("Warning: Failed to detect conflicts: %v", err)
		} else {
			if result.CanFastForward {
				fmt.Println("✓ Can fast-forward (no merge commit needed)")
			} else if len(result.Conflicts) == 0 {
				fmt.Println("✓ No conflicts detected - safe to merge")
			} else {
				fmt.Printf("⚠️  %d potential conflicts detected:\n", len(result.Conflicts))
				for _, conflict := range result.Conflicts {
					fmt.Printf("  - %s (%s)\n", conflict.Path, conflict.Type)
				}
			}

			if result.Difficulty != "" {
				fmt.Printf("Merge difficulty: %s\n", result.Difficulty)
			}
		}
	} else {
		fmt.Println("Skipping - no other branch found to test merge")
		fmt.Println()
		fmt.Println("Example usage:")
		fmt.Println("  result, err := mergeManager.DetectConflicts(ctx, repo, merge.DetectOptions{")
		fmt.Println("      Source: \"feature/my-branch\",")
		fmt.Println("      Target: \"main\",")
		fmt.Println("  })")
	}
	fmt.Println()

	// Example 3: Merge strategies
	fmt.Println("=== Example 3: Available Merge Strategies ===")
	fmt.Println("gzh-cli-git supports multiple merge strategies:")
	fmt.Println("  - fast-forward: Fast-forward only (no merge commit)")
	fmt.Println("  - recursive: Default 3-way merge")
	fmt.Println("  - ours: Prefer current branch on conflicts")
	fmt.Println("  - theirs: Prefer incoming branch on conflicts")
	fmt.Println("  - octopus: Merge multiple branches")
	fmt.Println()

	// Example 4: Merge execution (demonstration only)
	fmt.Println("=== Example 4: Execute Merge (Example) ===")
	if targetBranch != "" {
		fmt.Printf("To merge '%s' into current branch:\n", targetBranch)
		fmt.Println()
		fmt.Println("Using gzh-cli-git library:")
		fmt.Printf("  err := mergeManager.Execute(ctx, repo, merge.ExecuteOptions{\n")
		fmt.Printf("      Branch:   \"%s\",\n", targetBranch)
		fmt.Println("      Strategy: \"recursive\",")
		fmt.Println("      NoCommit: false,")
		fmt.Println("  })")
		fmt.Println()
		fmt.Println("Using CLI:")
		fmt.Printf("  gzh-git merge do %s\n", targetBranch)
		fmt.Println()
		fmt.Println("⚠️  This example does NOT execute the merge")
	}

	// Example 5: Rebase operations
	fmt.Println("=== Example 5: Rebase Operations ===")
	fmt.Println("Rebase current branch onto another:")
	fmt.Println()
	fmt.Println("Using gzh-cli-git library:")
	fmt.Println("  err := mergeManager.Rebase(ctx, repo, merge.RebaseOptions{")
	fmt.Println("      Onto:        \"main\",")
	fmt.Println("      Interactive: false,")
	fmt.Println("  })")
	fmt.Println()
	fmt.Println("Using CLI:")
	fmt.Println("  gzh-git merge rebase main")
	fmt.Println()

	fmt.Println("Tip: Always detect conflicts before merging:")
	fmt.Println("  gzh-git merge detect <source> <target>")
}
