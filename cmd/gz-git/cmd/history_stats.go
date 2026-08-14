package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/history"
)

var (
	statsSince  string
	statsUntil  string
	statsBranch string
	statsAuthor string
	statsFormat string
)

// statsCmd represents the history stats command.
var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show commit statistics",
	Long: cliutil.QuickStartHelp(`  # Show overall statistics
  gz-git history stats

  # Statistics for last month
  gz-git history stats --since "1 month ago"

  # Statistics for specific branch
  gz-git history stats --branch feature/new-feature

  # Export as JSON
  gz-git history stats --format json > stats.json`),
	Example: ``,
	RunE:    runHistoryStats,
}

func init() {
	historyCmd.AddCommand(statsCmd)

	statsCmd.Flags().StringVar(&statsSince, "since", "", "start date (e.g., '2024-01-01', '1 month ago')")
	statsCmd.Flags().StringVar(&statsUntil, "until", "", "end date (e.g., '2024-12-31', 'yesterday')")
	statsCmd.Flags().StringVarP(&statsBranch, "branch", "b", "", "specific branch (default: current)")
	statsCmd.Flags().StringVar(&statsAuthor, "author", "", "filter by author")
	statsCmd.Flags().StringVar(&statsFormat, "format", "table", "output format: table, json, csv, markdown, llm")
}

func runHistoryStats(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Validate format
	if err := validateHistoryFormat(statsFormat); err != nil {
		return err
	}

	repo, err := openCurrentRepo(ctx)
	if err != nil {
		return err
	}

	analyzer := history.NewHistoryAnalyzer(gitcmd.NewExecutor())

	// Parse dates
	sinceTime, err := parseDate(statsSince)
	if err != nil {
		return fmt.Errorf("invalid --since date: %w", err)
	}

	untilTime, err := parseDate(statsUntil)
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

// parseOutputFormat converts string format to OutputFormat enum.
// It must accept every format that validateHistoryFormat allows.
func parseOutputFormat(format string) (history.OutputFormat, error) {
	switch format {
	case "table", "default", "compact":
		return history.FormatTable, nil
	case "json":
		return history.FormatJSON, nil
	case "csv":
		return history.FormatCSV, nil
	case "markdown", "md":
		return history.FormatMarkdown, nil
	case "llm":
		return history.FormatLLM, nil
	default:
		return history.FormatTable, fmt.Errorf("unknown format: %s (valid: %s)", format, strings.Join(ValidHistoryFormats, ", "))
	}
}
