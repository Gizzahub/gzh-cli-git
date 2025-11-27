package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-git/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-git/pkg/history"
	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

var (
	fileHistorySince  string
	fileHistoryUntil  string
	fileHistoryMax    int
	fileHistoryFollow bool
	fileHistoryAuthor string
	fileHistoryFormat string
)

// fileCmd represents the history file command
var fileCmd = &cobra.Command{
	Use:   "file <path>",
	Short: "Show file change history",
	Long: `Show the commit history for a specific file.

Displays all commits that modified the file, including commit hash, author,
date, and commit message.`,
	Example: `  # Show file history
  gzh-git history file src/main.go

  # Follow renames
  gzh-git history file --follow src/main.go

  # Limit to 10 commits
  gzh-git history file --max 10 src/main.go

  # Export as JSON
  gzh-git history file --format json src/main.go > history.json`,
	Args: cobra.ExactArgs(1),
	RunE: runHistoryFile,
}

// blameCmd represents the history blame command
var blameCmd = &cobra.Command{
	Use:   "blame <file>",
	Short: "Show line-by-line authorship",
	Long: `Show who last modified each line of a file (git blame).

Displays the commit hash, author, date, and line number for each line,
showing who is responsible for the current content.`,
	Example: `  # Show blame for a file
  gzh-git history blame src/main.go

  # Export as JSON
  gzh-git history blame --format json src/main.go`,
	Args: cobra.ExactArgs(1),
	RunE: runHistoryBlame,
}

func init() {
	historyCmd.AddCommand(fileCmd)
	historyCmd.AddCommand(blameCmd)

	fileCmd.Flags().StringVar(&fileHistorySince, "since", "", "start date (e.g., '2024-01-01')")
	fileCmd.Flags().StringVar(&fileHistoryUntil, "until", "", "end date (e.g., '2024-12-31')")
	fileCmd.Flags().IntVar(&fileHistoryMax, "max", 0, "maximum number of commits")
	fileCmd.Flags().BoolVar(&fileHistoryFollow, "follow", false, "follow file renames")
	fileCmd.Flags().StringVar(&fileHistoryAuthor, "author", "", "filter by author")
	fileCmd.Flags().StringVarP(&fileHistoryFormat, "format", "f", "table", "output format (table|json|csv|markdown)")
}

func runHistoryFile(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	filePath := args[0]

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

	// Create tracker
	tracker := history.NewFileHistoryTracker(gitcmd.NewExecutor())

	// Parse dates
	sinceTime, err := parseDate(fileHistorySince)
	if err != nil {
		return fmt.Errorf("invalid --since date: %w", err)
	}

	untilTime, err := parseDate(fileHistoryUntil)
	if err != nil {
		return fmt.Errorf("invalid --until date: %w", err)
	}

	// Build options
	opts := history.HistoryOptions{
		MaxCount: fileHistoryMax,
		Since:    sinceTime,
		Until:    untilTime,
		Follow:   fileHistoryFollow,
		Author:   fileHistoryAuthor,
	}

	if !quiet {
		fmt.Printf("Analyzing file history for '%s'...\n", filePath)
	}

	// Get history
	fileHistory, err := tracker.GetHistory(ctx, repo, filePath, opts)
	if err != nil {
		return fmt.Errorf("failed to get file history: %w", err)
	}

	// Format output
	format, err := parseOutputFormat(fileHistoryFormat)
	if err != nil {
		return err
	}

	formatter := history.NewFormatter(format)
	output, err := formatter.FormatFileHistory(fileHistory)
	if err != nil {
		return fmt.Errorf("failed to format output: %w", err)
	}

	fmt.Println(string(output))
	return nil
}

func runHistoryBlame(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	filePath := args[0]

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

	// Create tracker
	tracker := history.NewFileHistoryTracker(gitcmd.NewExecutor())

	if !quiet {
		fmt.Printf("Getting blame information for '%s'...\n", filePath)
	}

	// Get blame
	blameInfo, err := tracker.GetBlame(ctx, repo, filePath)
	if err != nil {
		return fmt.Errorf("failed to get blame: %w", err)
	}

	// Display blame information
	// Note: Blame output is typically shown in a special format
	// Show lines with author and commit info
	for _, line := range blameInfo.Lines {
		fmt.Printf("%s (%s %s) %s\n",
			line.Hash[:8],
			line.Author,
			line.Date.Format("2006-01-02"),
			line.Content,
		)
	}

	return nil
}
