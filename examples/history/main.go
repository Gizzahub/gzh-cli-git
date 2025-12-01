// Package main demonstrates history analysis using gzh-cli-git library.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gizzahub/gzh-cli-git/pkg/history"
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
	historyAnalyzer := history.NewAnalyzer()

	// Open repository
	repo, err := repoClient.Open(ctx, repoPath)
	if err != nil {
		log.Fatalf("Failed to open repository: %v", err)
	}

	fmt.Printf("Repository: %s\n\n", repo.Path)

	// Example 1: Get commit statistics
	fmt.Println("=== Example 1: Commit Statistics ===")

	// Last 30 days
	since := time.Now().AddDate(0, 0, -30)

	stats, err := historyAnalyzer.GetStats(ctx, repo, history.StatsOptions{
		Since: &since,
	})
	if err != nil {
		log.Printf("Warning: Failed to get stats: %v", err)
	} else {
		fmt.Printf("Commits (last 30 days): %d\n", stats.TotalCommits)
		fmt.Printf("Authors: %d\n", stats.TotalAuthors)
		fmt.Printf("Files changed: %d\n", stats.FilesChanged)
		fmt.Printf("Insertions: %d\n", stats.Insertions)
		fmt.Printf("Deletions: %d\n", stats.Deletions)
	}
	fmt.Println()

	// Example 2: Analyze contributors
	fmt.Println("=== Example 2: Top Contributors ===")

	contributors, err := historyAnalyzer.GetContributors(ctx, repo, history.ContributorOptions{
		Since: &since,
		Limit: 5,
	})
	if err != nil {
		log.Printf("Warning: Failed to get contributors: %v", err)
	} else {
		for i, contrib := range contributors {
			fmt.Printf("%d. %s <%s>\n", i+1, contrib.Name, contrib.Email)
			fmt.Printf("   Commits: %d\n", contrib.Commits)
			fmt.Printf("   Lines: +%d/-%d\n", contrib.Insertions, contrib.Deletions)
		}
	}
	fmt.Println()

	// Example 3: Get file history
	fmt.Println("=== Example 3: File History ===")

	// Analyze README.md if it exists
	filePath := "README.md"
	fileHistory, err := historyAnalyzer.GetFileHistory(ctx, repo, history.FileHistoryOptions{
		Path:  filePath,
		Limit: 5,
	})
	if err != nil {
		log.Printf("Warning: Failed to get file history: %v", err)
	} else {
		fmt.Printf("Recent commits affecting %s:\n", filePath)
		for i, commit := range fileHistory {
			fmt.Printf("%d. %s\n", i+1, commit.Subject)
			fmt.Printf("   Author: %s\n", commit.Author)
			fmt.Printf("   Date: %s\n", commit.Date.Format("2006-01-02"))
			fmt.Printf("   Hash: %s\n", commit.Hash[:8])
		}
	}
	fmt.Println()

	// Example 4: Get recent commits
	fmt.Println("=== Example 4: Recent Commits ===")

	commits, err := historyAnalyzer.GetCommits(ctx, repo, history.CommitOptions{
		Limit: 3,
	})
	if err != nil {
		log.Printf("Warning: Failed to get commits: %v", err)
	} else {
		for i, commit := range commits {
			fmt.Printf("%d. %s\n", i+1, commit.Subject)
			fmt.Printf("   %s (%s)\n", commit.Author, commit.Hash[:8])
			fmt.Printf("   %s\n", commit.Date.Format("Mon Jan 2 15:04:05 2006"))
		}
	}
	fmt.Println()

	fmt.Println("Tip: Use gzh-git for more detailed history analysis:")
	fmt.Println("  gzh-git history stats --since \"1 month ago\"")
	fmt.Println("  gzh-git history contributors --top 10")
	fmt.Println("  gzh-git history file <path>")
}
