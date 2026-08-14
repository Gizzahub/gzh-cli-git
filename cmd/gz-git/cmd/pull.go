package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

var (
	pullFlags    BulkCommandFlags
	pullStrategy string
	pullPrune    bool
	pullTags     bool
	pullStash    bool
)

// pullCmd represents the pull command for multi-repository operations.
var pullCmd = &cobra.Command{
	Use:   "pull [directory]",
	Short: "Pull updates from multiple repositories in parallel",
	Long: cliutil.QuickStartHelp(`  # Pull all repositories in current directory
  gz-git pull

  # Pull all repositories up to 2 levels deep
  gz-git pull -d 2 ~/projects

  # Pull with custom parallelism
  gz-git pull --parallel 10 ~/workspace

  # Pull with rebase strategy
  gz-git pull --merge-strategy rebase ~/projects

  # Pull with fast-forward only strategy
  gz-git pull --merge-strategy ff-only ~/repos

  # Pull and prune deleted remote branches
  gz-git pull --prune ~/projects

  # Automatically stash local changes before pull
  gz-git pull --stash ~/projects

  # Filter by pattern
  gz-git pull --include "myproject.*" ~/workspace`) + cliutil.ExitCodesBulkHelp(),
	Args: cobra.MaximumNArgs(1),
	RunE: runPull,
}

func init() {
	rootCmd.AddCommand(pullCmd)

	// Common bulk operation flags
	addBulkFlags(pullCmd, &pullFlags)

	// Pull-specific flags
	pullCmd.Flags().StringVarP(&pullStrategy, "merge-strategy", "s", "merge", "merge strategy: merge, rebase, ff-only")
	pullCmd.Flags().BoolVarP(&pullPrune, "prune", "p", false, "prune remote-tracking branches that no longer exist")
	pullCmd.Flags().BoolVarP(&pullTags, "tags", "t", false, "fetch all tags from remote")
	pullCmd.Flags().BoolVar(&pullStash, "stash", false, "automatically stash local changes before pull")
}

func runPull(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Load config with profile support
	effective, _ := LoadEffectiveConfig(cmd, nil)
	if effective != nil {
		// Apply config if flag not explicitly set
		if !cmd.Flags().Changed("parallel") && effective.Parallel > 0 {
			pullFlags.Parallel = effective.Parallel
		}
		// Apply pull strategy from config (rebase or ff-only)
		if !cmd.Flags().Changed("merge-strategy") && !cmd.Flags().Changed("strategy") {
			if effective.Pull.FFOnly {
				pullStrategy = "ff-only"
			} else if effective.Pull.Rebase {
				pullStrategy = "rebase"
			}
		}
		if verbose {
			PrintConfigSources(cmd, effective)
		}
	}

	// Validate and parse directory
	directory, err := validateBulkDirectory(args)
	if err != nil {
		return err
	}

	// Validate depth
	if err := validateBulkDepth(cmd, pullFlags.Depth); err != nil {
		return err
	}

	// Validate format
	if err := validateBulkFormat(pullFlags.Format); err != nil {
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
	if shouldShowProgress(pullFlags.Format, quiet) {
		printScanningMessage(directory, pullFlags.Depth, pullFlags.Parallel, pullFlags.DryRun)
	}

	result, err := client.BulkPull(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk pull failed: %w", err)
	}

	// Display scan completion message
	if shouldShowProgress(pullFlags.Format, quiet) && result.TotalScanned == 0 {
		fmt.Printf("Scan complete: no repositories found\n")
	}

	// Display results (always output for JSON format, otherwise respect quiet flag)
	if pullFlags.Format == "json" || !quiet {
		displayPullResults(result)
	}

	return errPartialFailure(result.Summary[repository.StatusError], result.TotalProcessed)
}

func runPullWatch(ctx context.Context, client repository.Client, opts repository.BulkPullOptions) error {
	cfg := WatchConfig{
		Interval:      pullFlags.Interval,
		Format:        pullFlags.Format,
		Quiet:         quiet,
		OperationName: "pull",
		Directory:     opts.Directory,
		MaxDepth:      opts.MaxDepth,
		Parallel:      opts.Parallel,
	}

	return RunBulkWatch(cfg, func() error {
		return executePull(ctx, client, opts)
	})
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
	rows := make([]BulkRenderRow, 0, len(result.Repositories))
	for _, repo := range result.Repositories {
		rows = append(rows, BulkRenderRow{
			Path:             repo.GetPath(),
			Branch:           repo.Branch,
			Status:           repo.GetStatus(),
			Message:          repo.GetMessage(),
			Remote:           repo.Remote,
			Err:              repo.GetError(),
			Duration:         repo.Duration,
			CommitsAhead:     repo.CommitsAhead,
			CommitsBehind:    repo.CommitsBehind,
			UncommittedFiles: repo.TrackedChangedFiles,
			UntrackedFiles:   repo.UntrackedFiles,
			Stashed:          repo.Stashed,
		})
	}

	issueStatuses := issueStatusSet(
		"error", "no-remote", "no-upstream", "conflict",
		"rebase-in-progress", "merge-in-progress", "dirty",
	)
	if pullFlags.Format != "compact" {
		issueStatuses["auth-required"] = true
	}

	RenderBulkResults(os.Stdout, BulkRenderConfig{
		Title:          "=== Pull Results ===",
		Verb:           "Pulled",
		Format:         pullFlags.Format,
		Verbose:        verbose,
		IssueStatuses:  issueStatuses,
		FormatStatus:   formatPullStatus,
		ChangesCount:   func(row BulkRenderRow) int { return row.CommitsBehind },
		SuccessMessage: "✓ All repositories pulled successfully",
		ShowFooters:    true,
	}, BulkRenderInput{
		TotalScanned:   result.TotalScanned,
		TotalProcessed: result.TotalProcessed,
		Duration:       result.Duration,
		Summary:        result.Summary,
		Rows:           rows,
	})
}

func formatPullStatus(row BulkRenderRow) string {
	statusStr := ""
	switch row.Status {
	case "success", "pulled":
		if row.CommitsBehind > 0 {
			statusStr = fmt.Sprintf("%d↓ pulled", row.CommitsBehind)
		} else {
			statusStr = "up-to-date"
		}
	case "up-to-date":
		if row.CommitsAhead > 0 {
			statusStr = fmt.Sprintf("up-to-date %d↑", row.CommitsAhead)
		} else {
			statusStr = "up-to-date"
		}
	case "would-pull":
		if row.CommitsBehind > 0 {
			statusStr = fmt.Sprintf("would pull %d↓", row.CommitsBehind)
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
		statusStr = row.Status
	}
	if row.Stashed {
		statusStr += " [stash]"
	}
	return statusStr
}
