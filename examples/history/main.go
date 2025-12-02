// Package main demonstrates history analysis using gzh-cli-git library.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gizzahub/gzh-cli-git/internal/gitcmd"
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
	executor := gitcmd.NewExecutor()
	historyAnalyzer := history.NewHistoryAnalyzer(executor)
	contributorAnalyzer := history.NewContributorAnalyzer(executor)
	fileTracker := history.NewFileHistoryTracker(executor)

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

	stats, err := historyAnalyzer.Analyze(ctx, repo, history.AnalyzeOptions{
		Since: since,
	})
	if err != nil {
		log.Printf("Warning: Failed to get stats: %v", err)
	} else {
		fmt.Printf("Commits (last 30 days): %d\n", stats.TotalCommits)
		fmt.Printf("Authors: %d\n", stats.UniqueAuthors)
		fmt.Printf("Insertions: %d\n", stats.TotalAdditions)
		fmt.Printf("Deletions: %d\n", stats.TotalDeletions)
		fmt.Printf("Avg commits/day: %.2f\n", stats.AvgPerDay)
	}
	fmt.Println()

	// Example 2: Analyze contributors
	fmt.Println("=== Example 2: Top Contributors ===")

	contributors, err := contributorAnalyzer.Analyze(ctx, repo, history.ContributorOptions{
		SortBy: history.SortByCommits,
	})
	if err != nil {
		log.Printf("Warning: Failed to get contributors: %v", err)
	} else {
		limit := 5
		if len(contributors) < limit {
			limit = len(contributors)
		}
		for i := 0; i < limit; i++ {
			contrib := contributors[i]
			fmt.Printf("%d. %s <%s>\n", i+1, contrib.Name, contrib.Email)
			fmt.Printf("   Commits: %d\n", contrib.TotalCommits)
			fmt.Printf("   Lines: +%d/-%d\n", contrib.LinesAdded, contrib.LinesDeleted)
		}
	}
	fmt.Println()

	// Example 3: Get file history
	fmt.Println("=== Example 3: File History ===")

	// Analyze README.md if it exists
	filePath := "README.md"
	fileHistory, err := fileTracker.GetHistory(ctx, repo, filePath, history.HistoryOptions{
		MaxCount: 5,
	})
	if err != nil {
		log.Printf("Warning: Failed to get file history: %v", err)
	} else {
		fmt.Printf("Recent commits affecting %s:\n", filePath)
		for i, commit := range fileHistory {
			fmt.Printf("%d. %s\n", i+1, commit.Message)
			fmt.Printf("   Author: %s\n", commit.Author)
			fmt.Printf("   Date: %s\n", commit.Date.Format("2006-01-02"))
			fmt.Printf("   Hash: %s\n", commit.Hash[:8])
		}
	}
	fmt.Println()

	// Example 4: Get commit trends
	fmt.Println("=== Example 4: Commit Trends ===")

	trends, err := historyAnalyzer.GetTrends(ctx, repo, history.TrendOptions{
		Since: since,
	})
	if err != nil {
		log.Printf("Warning: Failed to get trends: %v", err)
	} else {
		fmt.Printf("Daily commits: %d different days\n", len(trends.Daily))
		fmt.Printf("Weekly commits: %d different weeks\n", len(trends.Weekly))
		fmt.Printf("Monthly commits: %d different months\n", len(trends.Monthly))
		fmt.Println("Commits by hour:")
		for hour := 0; hour < 24; hour++ {
			if count, ok := trends.Hourly[hour]; ok && count > 0 {
				fmt.Printf("  %02d:00 - %d commits\n", hour, count)
			}
		}
	}
	fmt.Println()

	fmt.Println("Tip: Use gzh-git for more detailed history analysis:")
	fmt.Println("  gzh-git history stats --since \"1 month ago\"")
	fmt.Println("  gzh-git history contributors --top 10")
	fmt.Println("  gzh-git history file <path>")
}
