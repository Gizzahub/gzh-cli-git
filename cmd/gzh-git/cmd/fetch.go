package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

var (
	fetchDepth            int
	fetchParallel         int
	fetchDryRun           bool
	fetchAllRemotes       bool
	fetchPrune            bool
	fetchTags             bool
	fetchIncludeSubmodules bool
	fetchInclude          string
	fetchExclude          string
	fetchFormat           string
	fetchWatch            bool
	fetchInterval         time.Duration
)

// fetchCmd represents the fetch command
var fetchCmd = &cobra.Command{
	Use:   "fetch [directory]",
	Short: "Fetch updates from remote repositories",
	Long: `Scan for Git repositories and fetch updates from remote in parallel.

This command recursively scans the specified directory (or current directory)
for Git repositories and fetches updates from their remotes in parallel.

By default:
  - Scans up to 5 directory levels deep
  - Processes 5 repositories in parallel
  - Fetches from origin remote only
  - Skips repositories without remotes

The command is safe to run and will not modify your working tree.
It only updates remote-tracking branches.`,
	Example: `  # Fetch all repositories in current directory (1-depth scan)
  gz-git fetch -d 1

  # Fetch all repositories up to 2 levels deep
  gz-git fetch -d 2 .

  # Fetch with custom parallelism
  gz-git fetch --parallel 10 ~/projects

  # Fetch from all remotes
  gz-git fetch --all ~/workspace

  # Fetch and prune deleted remote branches
  gz-git fetch --prune ~/projects

  # Fetch all tags
  gz-git fetch --tags ~/repos

  # Dry run to see what would be fetched
  gz-git fetch --dry-run ~/projects

  # Filter by pattern
  gz-git fetch --include "myproject.*" ~/workspace

  # Exclude pattern
  gz-git fetch --exclude "test.*" ~/projects

  # Compact output format
  gz-git fetch --format compact ~/projects

  # Continuously fetch at intervals (watch mode)
  gz-git fetch -d 2 --watch --interval 5m ~/projects

  # Watch with shorter interval
  gz-git fetch --watch --interval 1m ~/work`,
	Args: cobra.MaximumNArgs(1),
	RunE: runFetch,
}

func init() {
	rootCmd.AddCommand(fetchCmd)

	// Flags
	fetchCmd.Flags().IntVarP(&fetchDepth, "depth", "d", 5, "directory depth to scan")
	fetchCmd.Flags().IntVarP(&fetchParallel, "parallel", "j", 5, "number of parallel fetch operations")
	fetchCmd.Flags().BoolVarP(&fetchDryRun, "dry-run", "n", false, "show what would be fetched without fetching")
	fetchCmd.Flags().BoolVar(&fetchAllRemotes, "all", false, "fetch from all remotes (not just origin)")
	fetchCmd.Flags().BoolVar(&fetchPrune, "prune", false, "prune remote-tracking branches that no longer exist")
	fetchCmd.Flags().BoolVarP(&fetchTags, "tags", "t", false, "fetch all tags from remote")
	fetchCmd.Flags().BoolVarP(&fetchIncludeSubmodules, "recursive", "r", false, "recursively include nested repositories and submodules")
	fetchCmd.Flags().StringVar(&fetchInclude, "include", "", "regex pattern to include repositories")
	fetchCmd.Flags().StringVar(&fetchExclude, "exclude", "", "regex pattern to exclude repositories")
	fetchCmd.Flags().StringVar(&fetchFormat, "format", "default", "output format: default, compact")
	fetchCmd.Flags().BoolVar(&fetchWatch, "watch", false, "continuously fetch at intervals")
	fetchCmd.Flags().DurationVar(&fetchInterval, "interval", 5*time.Minute, "fetch interval when watching")
}

func runFetch(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Parse directory argument
	directory := "."
	if len(args) > 0 {
		directory = args[0]
	}

	// Validate directory exists
	if _, err := os.Stat(directory); err != nil {
		return fmt.Errorf("directory does not exist: %s", directory)
	}

	// Create client
	client := repository.NewClient()

	// Create logger for verbose mode
	var logger repository.Logger
	if verbose {
		logger = repository.NewWriterLogger(os.Stdout)
	}

	// Build options
	opts := repository.BulkFetchOptions{
		Directory:         directory,
		Parallel:          fetchParallel,
		MaxDepth:          fetchDepth,
		DryRun:            fetchDryRun,
		Verbose:           verbose,
		AllRemotes:        fetchAllRemotes,
		Prune:             fetchPrune,
		Tags:              fetchTags,
		IncludeSubmodules: fetchIncludeSubmodules,
		IncludePattern:    fetchInclude,
		ExcludePattern:    fetchExclude,
		Logger:            logger,
		ProgressCallback: func(current, total int, repo string) {
			if !quiet && fetchFormat != "compact" {
				fmt.Printf("[%d/%d] Fetching %s...\n", current, total, repo)
			}
		},
	}

	// Watch mode: continuously fetch at intervals
	if fetchWatch {
		return runFetchWatch(ctx, client, opts)
	}

	// One-time fetch
	if !quiet {
		fmt.Printf("Scanning for repositories in %s (depth: %d)...\n", directory, fetchDepth)
	}

	result, err := client.BulkFetch(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk fetch failed: %w", err)
	}

	// Display results
	if !quiet {
		displayFetchResults(result)
	}

	return nil
}

func runFetchWatch(ctx context.Context, client repository.Client, opts repository.BulkFetchOptions) error {
	if !quiet {
		fmt.Printf("Starting watch mode: fetching every %s\n", fetchInterval)
		fmt.Printf("Scanning for repositories in %s (depth: %d)...\n", opts.Directory, opts.MaxDepth)
		fmt.Println("Press Ctrl+C to stop...")
		fmt.Println()
	}

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create ticker for periodic fetching
	ticker := time.NewTicker(fetchInterval)
	defer ticker.Stop()

	// Perform initial fetch immediately
	if err := executeFetch(ctx, client, opts); err != nil {
		return err
	}

	// Watch loop
	for {
		select {
		case <-sigChan:
			if !quiet {
				fmt.Println("\nStopping watch...")
			}
			return nil

		case <-ticker.C:
			if !quiet && fetchFormat != "compact" {
				fmt.Printf("\n[%s] Running scheduled fetch...\n", time.Now().Format("15:04:05"))
			}
			if err := executeFetch(ctx, client, opts); err != nil {
				if !quiet {
					fmt.Fprintf(os.Stderr, "Fetch error: %v\n", err)
				}
				// Continue watching even on error
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func executeFetch(ctx context.Context, client repository.Client, opts repository.BulkFetchOptions) error {
	result, err := client.BulkFetch(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk fetch failed: %w", err)
	}

	// Display results
	if !quiet {
		displayFetchResults(result)
	}

	return nil
}

func displayFetchResults(result *repository.BulkFetchResult) {
	fmt.Println()
	fmt.Println("=== Bulk Fetch Results ===")
	fmt.Printf("Total scanned:   %d repositories\n", result.TotalScanned)
	fmt.Printf("Total processed: %d repositories\n", result.TotalProcessed)
	fmt.Printf("Duration:        %s\n", result.Duration.Round(100_000_000)) // Round to 0.1s
	fmt.Println()

	// Display summary
	if len(result.Summary) > 0 {
		fmt.Println("Summary by status:")
		for status, count := range result.Summary {
			icon := getStatusIcon(status)
			fmt.Printf("  %s %-15s %d\n", icon, status+":", count)
		}
		fmt.Println()
	}

	// Display individual results if not compact
	if fetchFormat != "compact" && len(result.Repositories) > 0 {
		fmt.Println("Repository details:")
		for _, repo := range result.Repositories {
			displayFetchRepositoryResult(repo)
		}
	}

	// Display only errors/warnings in compact mode
	if fetchFormat == "compact" {
		hasIssues := false
		for _, repo := range result.Repositories {
			if repo.Status == "error" || repo.Status == "no-remote" {
				if !hasIssues {
					fmt.Println("Issues found:")
					hasIssues = true
				}
				displayFetchRepositoryResult(repo)
			}
		}
		if !hasIssues {
			fmt.Println("✓ All repositories fetched successfully")
		}
	}
}

func displayFetchRepositoryResult(repo repository.RepositoryFetchResult) {
	icon := getStatusIcon(repo.Status)

	// Format: icon path (branch) - message [duration]
	line := fmt.Sprintf("  %s %s", icon, repo.RelativePath)

	if repo.Branch != "" {
		line += fmt.Sprintf(" (%s)", repo.Branch)
	}

	line += fmt.Sprintf(" - %s", repo.Message)

	if repo.Duration > 0 {
		line += fmt.Sprintf(" [%s]", repo.Duration.Round(10_000_000)) // Round to 0.01s
	}

	fmt.Println(line)

	// Show error details if present
	if repo.Error != nil && verbose {
		fmt.Printf("    Error: %v\n", repo.Error)
	}
}

func getStatusIcon(status string) string {
	switch status {
	case "success":
		return "✓"
	case "error":
		return "✗"
	case "skipped":
		return "⊘"
	case "would-fetch":
		return "→"
	case "no-remote":
		return "⚠"
	case "up-to-date":
		return "="
	case "no-upstream":
		return "⚠"
	case "updated":
		return "↓"
	default:
		return "•"
	}
}
