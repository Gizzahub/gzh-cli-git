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
	pullFlags    BulkCommandFlags
	pullStrategy string
	pullPrune    bool
	pullTags     bool
	pullStash    bool
)

// pullCmd represents the pull command
var pullCmd = &cobra.Command{
	Use:   "pull [directory]",
	Short: "Pull updates from remote repositories",
	Long: `Scan for Git repositories and pull updates from remote in parallel.

This command recursively scans the specified directory (or current directory)
for Git repositories and pulls updates (fetch + merge/rebase) from their remotes
in parallel.

By default:
  - Scans 1 directory level deep
  - Processes 5 repositories in parallel
  - Uses merge strategy (can use rebase or ff-only)
  - Skips repositories without remotes or upstreams

The command updates your working tree with changes from the remote.`,
	Example: `  # Pull all repositories in current directory (1-depth scan)
  gz-git pull -d 1

  # Pull all repositories up to 2 levels deep
  gz-git pull -d 2 ~/projects

  # Pull with custom parallelism
  gz-git pull --parallel 10 ~/workspace

  # Pull with rebase strategy
  gz-git pull --strategy rebase ~/projects

  # Pull with fast-forward only strategy
  gz-git pull --strategy ff-only ~/repos

  # Pull and prune deleted remote branches
  gz-git pull --prune ~/projects

  # Dry run to see what would be pulled
  gz-git pull --dry-run ~/projects

  # Automatically stash local changes before pull
  gz-git pull --stash ~/projects

  # Filter by pattern
  gz-git pull --include "myproject.*" ~/workspace

  # Exclude pattern
  gz-git pull --exclude "test.*" ~/projects

  # Compact output format
  gz-git pull --format compact ~/projects

  # Continuously pull at intervals (watch mode)
  gz-git pull -d 2 --watch --interval 10m ~/projects

  # Watch with shorter interval
  gz-git pull --watch --interval 5m ~/work`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPull,
}

func init() {
	rootCmd.AddCommand(pullCmd)

	// Common bulk operation flags
	addBulkFlags(pullCmd, &pullFlags)

	// Pull-specific flags
	pullCmd.Flags().StringVarP(&pullStrategy, "strategy", "s", "merge", "pull strategy: merge, rebase, ff-only")
	pullCmd.Flags().BoolVarP(&pullPrune, "prune", "p", false, "prune remote-tracking branches that no longer exist")
	pullCmd.Flags().BoolVarP(&pullTags, "tags", "t", false, "fetch all tags from remote")
	pullCmd.Flags().BoolVar(&pullStash, "stash", false, "automatically stash local changes before pull")
}

func runPull(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Validate and parse directory
	directory, err := validateBulkDirectory(args)
	if err != nil {
		return err
	}

	// Validate depth
	if err := validateBulkDepth(cmd, pullFlags.Depth); err != nil {
		return err
	}

	// Create client
	client := repository.NewClient()

	// Create logger for verbose mode
	logger := createBulkLogger(verbose)

	// Build options
	opts := repository.BulkPullOptions{
		Directory:         directory,
		Parallel:          pullFlags.Parallel,
		MaxDepth:          pullFlags.Depth,
		DryRun:            pullFlags.DryRun,
		Verbose:           verbose,
		Strategy:          pullStrategy,
		Prune:             pullPrune,
		Tags:              pullTags,
		Stash:             pullStash,
		IncludeSubmodules: pullFlags.IncludeSubmodules,
		IncludePattern:    pullFlags.Include,
		ExcludePattern:    pullFlags.Exclude,
		Logger:            logger,
		ProgressCallback:  createProgressCallback("Pulling", pullFlags.Format, quiet),
	}

	// Watch mode: continuously pull at intervals
	if pullFlags.Watch {
		return runPullWatch(ctx, client, opts)
	}

	// One-time pull
	if !quiet {
		fmt.Printf("Scanning for repositories in %s (depth: %d)...\n", directory, pullFlags.Depth)
	}

	result, err := client.BulkPull(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk pull failed: %w", err)
	}

	// Display scan completion message
	if !quiet && result.TotalScanned == 0 {
		fmt.Printf("Scan complete: no repositories found\n")
	}

	// Display results
	if !quiet {
		displayPullResults(result)
	}

	return nil
}

func runPullWatch(ctx context.Context, client repository.Client, opts repository.BulkPullOptions) error {
	if !quiet {
		fmt.Printf("Starting watch mode: pulling every %s\n", pullFlags.Interval)
		fmt.Printf("Scanning for repositories in %s (depth: %d)...\n", opts.Directory, opts.MaxDepth)
		fmt.Println("Press Ctrl+C to stop...")
		fmt.Println()
	}

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create ticker for periodic pulling
	ticker := time.NewTicker(pullFlags.Interval)
	defer ticker.Stop()

	// Perform initial pull immediately
	if err := executePull(ctx, client, opts); err != nil {
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
			if !quiet && pullFlags.Format != "compact" {
				fmt.Printf("\n[%s] Running scheduled pull...\n", time.Now().Format("15:04:05"))
			}
			if err := executePull(ctx, client, opts); err != nil {
				if !quiet {
					fmt.Fprintf(os.Stderr, "Pull error: %v\n", err)
				}
				// Continue watching even on error
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func executePull(ctx context.Context, client repository.Client, opts repository.BulkPullOptions) error {
	result, err := client.BulkPull(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk pull failed: %w", err)
	}

	// Display results
	if !quiet {
		displayPullResults(result)
	}

	return nil
}

func displayPullResults(result *repository.BulkPullResult) {
	fmt.Println()
	fmt.Println("=== Bulk Pull Results ===")
	fmt.Printf("Total scanned:   %d repositories\n", result.TotalScanned)
	fmt.Printf("Total processed: %d repositories\n", result.TotalProcessed)
	fmt.Printf("Duration:        %s\n", result.Duration.Round(100_000_000)) // Round to 0.1s
	fmt.Println()

	// Display summary
	if len(result.Summary) > 0 {
		fmt.Println("Summary by status:")
		for status, count := range result.Summary {
			icon := getPullStatusIcon(status)
			fmt.Printf("  %s %-15s %d\n", icon, status+":", count)
		}
		fmt.Println()
	}

	// Display individual results if not compact
	if pullFlags.Format != "compact" && len(result.Repositories) > 0 {
		fmt.Println("Repository details:")
		for _, repo := range result.Repositories {
			displayPullRepositoryResult(repo)
		}
	}

	// Display only errors/warnings in compact mode
	if pullFlags.Format == "compact" {
		hasIssues := false
		for _, repo := range result.Repositories {
			if repo.Status == "error" || repo.Status == "no-remote" || repo.Status == "no-upstream" ||
				repo.Status == "conflict" || repo.Status == "rebase-in-progress" || repo.Status == "merge-in-progress" ||
				repo.Status == "dirty" {
				if !hasIssues {
					fmt.Println("Issues found:")
					hasIssues = true
				}
				displayPullRepositoryResult(repo)
			}
		}
		if !hasIssues {
			fmt.Println("✓ All repositories pulled successfully")
		}
	}
}

func displayPullRepositoryResult(repo repository.RepositoryPullResult) {
	icon := getPullStatusIcon(repo.Status)

	// Build compact one-line format: icon path (branch) status duration
	parts := []string{icon}

	// Path with branch
	pathPart := repo.RelativePath
	if repo.Branch != "" {
		pathPart += fmt.Sprintf(" (%s)", repo.Branch)
	}
	parts = append(parts, fmt.Sprintf("%-50s", pathPart))

	// Show status compactly
	statusStr := ""
	switch repo.Status {
	case "success":
		if repo.CommitsBehind > 0 {
			statusStr = fmt.Sprintf("%d↓ pulled", repo.CommitsBehind)
		} else {
			statusStr = "pulled"
		}
	case "up-to-date":
		if repo.CommitsAhead > 0 {
			statusStr = fmt.Sprintf("up-to-date %d↑", repo.CommitsAhead)
		} else {
			statusStr = "up-to-date"
		}
	case "would-pull":
		if repo.CommitsBehind > 0 {
			statusStr = fmt.Sprintf("would pull %d↓", repo.CommitsBehind)
		} else {
			statusStr = "would pull"
		}
	case "error":
		statusStr = "failed"
	case "no-remote":
		statusStr = "no remote"
	case "no-upstream":
		statusStr = "no upstream"
	case "conflict":
		statusStr = "CONFLICT"
	case "rebase-in-progress":
		statusStr = "REBASE"
	case "merge-in-progress":
		statusStr = "MERGE"
	case "dirty":
		statusStr = "dirty"
	case "skipped":
		statusStr = "skipped"
	default:
		statusStr = repo.Status
	}

	// Add stash indicator
	if repo.Stashed {
		statusStr += " [stash]"
	}

	parts = append(parts, fmt.Sprintf("%-18s", statusStr))

	// Duration
	if repo.Duration > 0 {
		parts = append(parts, fmt.Sprintf("%6s", repo.Duration.Round(10_000_000)))
	}

	// Build output line safely
	line := "  " + parts[0] + " " + parts[1] + " " + parts[2]
	if len(parts) > 3 {
		line += " " + parts[3]
	}
	fmt.Println(line)

	// Show error details if present
	if repo.Error != nil && verbose {
		fmt.Printf("    Error: %v\n", repo.Error)
	}
}

func getPullStatusIcon(status string) string {
	switch status {
	case "success":
		return "✓"
	case "up-to-date":
		return "="
	case "error":
		return "✗"
	case "conflict":
		return "⚡"
	case "rebase-in-progress":
		return "↻"
	case "merge-in-progress":
		return "⇄"
	case "dirty":
		return "⚠"
	case "skipped":
		return "⊘"
	case "would-pull":
		return "→"
	case "no-remote":
		return "⚠"
	case "no-upstream":
		return "⚠"
	default:
		return "•"
	}
}
