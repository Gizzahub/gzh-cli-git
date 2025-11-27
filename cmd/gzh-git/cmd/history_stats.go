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
	statsSince  string
	statsUntil  string
	statsBranch string
	statsAuthor string
	statsFormat string
)

// statsCmd represents the history stats command
var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show commit statistics",
	Long: `Display commit statistics including total commits, contributors,
lines changed, and activity trends.

Statistics can be filtered by date range, branch, and author.
Output can be formatted as table, JSON, CSV, or markdown.`,
	Example: `  # Show overall statistics
  gzh-git history stats

  # Statistics for last month
  gzh-git history stats --since "1 month ago"

  # Statistics for specific branch
  gzh-git history stats --branch feature/new-feature

  # Export as JSON
  gzh-git history stats --format json > stats.json`,
	RunE: runHistoryStats,
}

func init() {
	historyCmd.AddCommand(statsCmd)

	statsCmd.Flags().StringVar(&statsSince, "since", "", "start date (e.g., '2024-01-01', '1 month ago')")
	statsCmd.Flags().StringVar(&statsUntil, "until", "", "end date (e.g., '2024-12-31', 'yesterday')")
	statsCmd.Flags().StringVar(&statsBranch, "branch", "", "specific branch (default: current)")
	statsCmd.Flags().StringVar(&statsAuthor, "author", "", "filter by author")
	statsCmd.Flags().StringVarP(&statsFormat, "format", "f", "table", "output format (table|json|csv|markdown)")
}

func runHistoryStats(cmd *cobra.Command, args []string) error {
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
	analyzer := history.NewHistoryAnalyzer(gitcmd.NewExecutor())

	// Parse dates
	sinceTime, err := parseStatsDate(statsSince)
	if err != nil {
		return fmt.Errorf("invalid --since date: %w", err)
	}

	untilTime, err := parseStatsDate(statsUntil)
	if err != nil {
		return fmt.Errorf("invalid --until date: %w", err)
	}

	// Build options
	opts := history.AnalyzeOptions{
		Since:  sinceTime,
		Until:  untilTime,
		Branch: statsBranch,
		Author: statsAuthor,
	}

	if !quiet {
		fmt.Println("Analyzing commit history...")
	}

	// Analyze
	stats, err := analyzer.Analyze(ctx, repo, opts)
	if err != nil {
		return fmt.Errorf("failed to analyze history: %w", err)
	}

	// Format output
	format, err := parseOutputFormat(statsFormat)
	if err != nil {
		return err
	}

	formatter := history.NewFormatter(format)
	output, err := formatter.FormatCommitStats(stats)
	if err != nil {
		return fmt.Errorf("failed to format output: %w", err)
	}

	fmt.Println(string(output))
	return nil
}

// parseOutputFormat converts string format to OutputFormat enum
func parseOutputFormat(format string) (history.OutputFormat, error) {
	switch format {
	case "table":
		return history.FormatTable, nil
	case "json":
		return history.FormatJSON, nil
	case "csv":
		return history.FormatCSV, nil
	case "markdown", "md":
		return history.FormatMarkdown, nil
	default:
		return history.FormatTable, fmt.Errorf("unknown format: %s (valid: table, json, csv, markdown)", format)
	}
}

// parseStatsDate parses a date string in common formats
func parseStatsDate(dateStr string) (time.Time, error) {
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
