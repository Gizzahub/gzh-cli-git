package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-git/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-git/pkg/history"
	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

var (
	contribSince      string
	contribUntil      string
	contribTop        int
	contribMinCommits int
	contribSortBy     string
	contribFormat     string
)

// contributorsCmd represents the history contributors command
var contributorsCmd = &cobra.Command{
	Use:   "contributors",
	Short: "Analyze repository contributors",
	Long: `Analyze and rank repository contributors by commits, lines changed, and activity.

Shows contributor statistics including total commits, lines added/removed,
first and last commit dates, and activity trends.`,
	Example: `  # List all contributors
  gzh-git history contributors

  # Top 10 contributors
  gzh-git history contributors --top 10

  # Contributors with at least 5 commits
  gzh-git history contributors --min-commits 5

  # Contributors since last month
  gzh-git history contributors --since "2024-10-01"

  # Export as JSON
  gzh-git history contributors --format json > contributors.json`,
	RunE: runHistoryContributors,
}

func init() {
	historyCmd.AddCommand(contributorsCmd)

	contributorsCmd.Flags().StringVar(&contribSince, "since", "", "start date (e.g., '2024-01-01')")
	contributorsCmd.Flags().StringVar(&contribUntil, "until", "", "end date (e.g., '2024-12-31')")
	contributorsCmd.Flags().IntVar(&contribTop, "top", 0, "show only top N contributors")
	contributorsCmd.Flags().IntVar(&contribMinCommits, "min-commits", 0, "minimum commits threshold")
	contributorsCmd.Flags().StringVar(&contribSortBy, "sort", "commits", "sort by (commits|additions|deletions|recent)")
	contributorsCmd.Flags().StringVarP(&contribFormat, "format", "f", "table", "output format (table|json|csv|markdown)")
}

func runHistoryContributors(cmd *cobra.Command, args []string) error {
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

	// Create analyzer
	analyzer := history.NewContributorAnalyzer(gitcmd.NewExecutor())

	// Parse dates
	sinceTime, err := parseDate(contribSince)
	if err != nil {
		return fmt.Errorf("invalid --since date: %w", err)
	}

	untilTime, err := parseDate(contribUntil)
	if err != nil {
		return fmt.Errorf("invalid --until date: %w", err)
	}

	// Parse sort by
	sortBy, err := parseContributorSortBy(contribSortBy)
	if err != nil {
		return err
	}

	// Build options
	opts := history.ContributorOptions{
		Since:      sinceTime,
		Until:      untilTime,
		MinCommits: contribMinCommits,
		SortBy:     sortBy,
	}

	if !quiet {
		fmt.Println("Analyzing contributors...")
	}

	// Analyze
	var contributors []*history.Contributor
	if contribTop > 0 {
		contributors, err = analyzer.GetTopContributors(ctx, repo, contribTop)
	} else {
		contributors, err = analyzer.Analyze(ctx, repo, opts)
	}
	if err != nil {
		return fmt.Errorf("failed to analyze contributors: %w", err)
	}

	// Format output
	format, err := parseOutputFormat(contribFormat)
	if err != nil {
		return err
	}

	formatter := history.NewFormatter(format)
	output, err := formatter.FormatContributors(contributors)
	if err != nil {
		return fmt.Errorf("failed to format output: %w", err)
	}

	fmt.Println(string(output))
	return nil
}

// parseContributorSortBy converts string to ContributorSortBy enum
func parseContributorSortBy(sort string) (history.ContributorSortBy, error) {
	switch sort {
	case "commits":
		return history.SortByCommits, nil
	case "additions", "lines":
		return history.SortByLinesAdded, nil
	case "deletions":
		return history.SortByLinesDeleted, nil
	case "recent":
		return history.SortByRecent, nil
	default:
		return history.SortByCommits, fmt.Errorf("unknown sort option: %s (valid: commits, additions, deletions, recent)", sort)
	}
}

// parseDate parses a date string in common formats
func parseDate(dateStr string) (time.Time, error) {
	if dateStr == "" {
		return time.Time{}, nil
	}

	// Try common date formats
	formats := []string{
		"2006-01-02",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported date format (use YYYY-MM-DD or YYYY-MM-DD HH:MM:SS): %s", dateStr)
}
