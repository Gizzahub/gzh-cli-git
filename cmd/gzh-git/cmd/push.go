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
	pushDepth             int
	pushParallel          int
	pushDryRun            bool
	pushForce             bool
	pushSetUpstream       bool
	pushTags              bool
	pushIncludeSubmodules bool
	pushInclude           string
	pushExclude           string
	pushFormat            string
	pushWatch             bool
	pushInterval          time.Duration
)

// pushCmd represents the push command
var pushCmd = &cobra.Command{
	Use:   "push [directory]",
	Short: "Push commits to remote repositories",
	Long: `Scan for Git repositories and push local commits to remote in parallel.

This command recursively scans the specified directory (or current directory)
for Git repositories and pushes local commits to their remotes in parallel.

By default:
  - Scans 1 directory level deep
  - Processes 5 repositories in parallel
  - Pushes to origin remote only
  - Skips repositories without remotes or upstreams

The command pushes your local commits to remote repositories.`,
	Example: `  # Push all repositories in current directory (1-depth scan)
  gz-git push -d 1

  # Push all repositories up to 2 levels deep
  gz-git push -d 2 ~/projects

  # Push with custom parallelism
  gz-git push --parallel 10 ~/workspace

  # Force push (use with caution!)
  gz-git push --force ~/projects

  # Push and set upstream for new branches
  gz-git push --set-upstream ~/projects

  # Push all tags
  gz-git push --tags ~/repos

  # Dry run to see what would be pushed
  gz-git push --dry-run ~/projects

  # Filter by pattern
  gz-git push --include "myproject.*" ~/workspace

  # Exclude pattern
  gz-git push --exclude "test.*" ~/projects

  # Compact output format
  gz-git push --format compact ~/projects

  # Continuously push at intervals (watch mode)
  gz-git push -d 2 --watch --interval 10m ~/projects

  # Watch with shorter interval
  gz-git push --watch --interval 5m ~/work`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPush,
}

func init() {
	rootCmd.AddCommand(pushCmd)

	// Flags
	pushCmd.Flags().IntVarP(&pushDepth, "depth", "d", repository.DefaultBulkMaxDepth, "directory depth to scan")
	pushCmd.Flags().IntVarP(&pushParallel, "parallel", "j", repository.DefaultBulkParallel, "number of parallel push operations")
	pushCmd.Flags().BoolVarP(&pushDryRun, "dry-run", "n", false, "show what would be pushed without pushing")
	pushCmd.Flags().BoolVarP(&pushForce, "force", "f", false, "force push (use with caution!)")
	pushCmd.Flags().BoolVarP(&pushSetUpstream, "set-upstream", "u", false, "set upstream for new branches")
	pushCmd.Flags().BoolVarP(&pushTags, "tags", "t", false, "push all tags to remote")
	pushCmd.Flags().BoolVarP(&pushIncludeSubmodules, "recursive", "r", false, "recursively include nested repositories and submodules")
	pushCmd.Flags().StringVar(&pushInclude, "include", "", "regex pattern to include repositories")
	pushCmd.Flags().StringVar(&pushExclude, "exclude", "", "regex pattern to exclude repositories")
	pushCmd.Flags().StringVar(&pushFormat, "format", "default", "output format: default, compact")
	pushCmd.Flags().BoolVar(&pushWatch, "watch", false, "continuously push at intervals")
	pushCmd.Flags().DurationVar(&pushInterval, "interval", 5*time.Minute, "push interval when watching")
}

func runPush(cmd *cobra.Command, args []string) error {
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

	// Validate depth
	if cmd.Flags().Changed("depth") && pushDepth == 0 {
		return fmt.Errorf("depth must be at least 1 (use --depth 1 to scan current directory and immediate subdirectories)")
	}

	// Create client
	client := repository.NewClient()

	// Create logger for verbose mode
	var logger repository.Logger
	if verbose {
		logger = repository.NewWriterLogger(os.Stdout)
	}

	// Build options
	opts := repository.BulkPushOptions{
		Directory:         directory,
		Parallel:          pushParallel,
		MaxDepth:          pushDepth,
		DryRun:            pushDryRun,
		Verbose:           verbose,
		Force:             pushForce,
		SetUpstream:       pushSetUpstream,
		Tags:              pushTags,
		IncludeSubmodules: pushIncludeSubmodules,
		IncludePattern:    pushInclude,
		ExcludePattern:    pushExclude,
		Logger:            logger,
		ProgressCallback: func(current, total int, repo string) {
			if !quiet && pushFormat != "compact" {
				fmt.Printf("[%d/%d] Pushing %s...\n", current, total, repo)
			}
		},
	}

	// Watch mode: continuously push at intervals
	if pushWatch {
		return runPushWatch(ctx, client, opts)
	}

	// One-time push
	if !quiet {
		fmt.Printf("Scanning for repositories in %s (depth: %d)...\n", directory, pushDepth)
	}

	result, err := client.BulkPush(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk push failed: %w", err)
	}

	// Display scan completion message
	if !quiet && result.TotalScanned == 0 {
		fmt.Printf("Scan complete: no repositories found\n")
	}

	// Display results
	if !quiet {
		displayPushResults(result)
	}

	return nil
}

func runPushWatch(ctx context.Context, client repository.Client, opts repository.BulkPushOptions) error {
	if !quiet {
		fmt.Printf("Starting watch mode: pushing every %s\n", pushInterval)
		fmt.Printf("Scanning for repositories in %s (depth: %d)...\n", opts.Directory, opts.MaxDepth)
		fmt.Println("Press Ctrl+C to stop...")
		fmt.Println()
	}

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create ticker for periodic pushing
	ticker := time.NewTicker(pushInterval)
	defer ticker.Stop()

	// Perform initial push immediately
	if err := executePush(ctx, client, opts); err != nil {
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
			if !quiet && pushFormat != "compact" {
				fmt.Printf("\n[%s] Running scheduled push...\n", time.Now().Format("15:04:05"))
			}
			if err := executePush(ctx, client, opts); err != nil {
				if !quiet {
					fmt.Fprintf(os.Stderr, "Push error: %v\n", err)
				}
				// Continue watching even on error
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func executePush(ctx context.Context, client repository.Client, opts repository.BulkPushOptions) error {
	result, err := client.BulkPush(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk push failed: %w", err)
	}

	// Display results
	if !quiet {
		displayPushResults(result)
	}

	return nil
}

func displayPushResults(result *repository.BulkPushResult) {
	fmt.Println()
	fmt.Println("=== Bulk Push Results ===")
	fmt.Printf("Total scanned:   %d repositories\n", result.TotalScanned)
	fmt.Printf("Total processed: %d repositories\n", result.TotalProcessed)
	fmt.Printf("Duration:        %s\n", result.Duration.Round(100_000_000)) // Round to 0.1s
	fmt.Println()

	// Display summary
	if len(result.Summary) > 0 {
		fmt.Println("Summary by status:")
		for status, count := range result.Summary {
			icon := getPushStatusIcon(status)
			fmt.Printf("  %s %-15s %d\n", icon, status+":", count)
		}
		fmt.Println()
	}

	// Display individual results if not compact
	if pushFormat != "compact" && len(result.Repositories) > 0 {
		fmt.Println("Repository details:")
		for _, repo := range result.Repositories {
			displayPushRepositoryResult(repo)
		}
	}

	// Display only errors/warnings in compact mode
	if pushFormat == "compact" {
		hasIssues := false
		for _, repo := range result.Repositories {
			if repo.Status == "error" || repo.Status == "no-remote" || repo.Status == "no-upstream" {
				if !hasIssues {
					fmt.Println("Issues found:")
					hasIssues = true
				}
				displayPushRepositoryResult(repo)
			}
		}
		if !hasIssues {
			fmt.Println("✓ All repositories pushed successfully")
		}
	}
}

func displayPushRepositoryResult(repo repository.RepositoryPushResult) {
	icon := getPushStatusIcon(repo.Status)

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
		if repo.PushedCommits > 0 {
			statusStr = fmt.Sprintf("%d↑ pushed", repo.PushedCommits)
		} else {
			statusStr = "pushed"
		}
	case "nothing-to-push":
		statusStr = "nothing to push"
	case "would-push":
		if repo.CommitsAhead > 0 {
			statusStr = fmt.Sprintf("would push %d↑", repo.CommitsAhead)
		} else {
			statusStr = "would push"
		}
	case "error":
		statusStr = "failed"
	case "no-remote":
		statusStr = "no remote"
	case "no-upstream":
		statusStr = "no upstream"
	case "skipped":
		statusStr = "skipped"
	default:
		statusStr = repo.Status
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

func getPushStatusIcon(status string) string {
	switch status {
	case "success":
		return "✓"
	case "nothing-to-push":
		return "="
	case "error":
		return "✗"
	case "skipped":
		return "⊘"
	case "would-push":
		return "→"
	case "no-remote":
		return "⚠"
	case "no-upstream":
		return "⚠"
	default:
		return "•"
	}
}
