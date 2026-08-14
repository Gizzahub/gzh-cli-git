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
	fetchFlags      BulkCommandFlags
	fetchAllRemotes bool
	fetchPrune      bool
	fetchTags       bool
)

// fetchCmd represents the fetch command for multi-repository operations.
var fetchCmd = &cobra.Command{
	Use:   "fetch [directory]",
	Short: "Fetch updates from multiple repositories in parallel",
	Long: cliutil.QuickStartHelp(`  # Fetch all repositories in current directory
  gz-git fetch

  # Fetch all repositories up to 2 levels deep
  gz-git fetch -d 2 .

  # Fetch from origin only (default is all remotes)
  gz-git fetch --all-remotes=false ~/workspace

  # Fetch and prune deleted remote branches
  gz-git fetch --prune ~/projects

  # Fetch all tags
  gz-git fetch --tags ~/repos

  # Filter by pattern
  gz-git fetch --include "myproject.*" ~/workspace`) + cliutil.ExitCodesBulkHelp(),
	Args: cobra.MaximumNArgs(1),
	RunE: runFetch,
}

func init() {
	rootCmd.AddCommand(fetchCmd)

	// Common bulk operation flags
	addBulkFlags(fetchCmd, &fetchFlags)

	// Fetch-specific flags
	fetchCmd.Flags().BoolVar(&fetchAllRemotes, "all-remotes", true, "fetch from all remotes (default: true, use --no-all-remotes for origin only)")
	fetchCmd.Flags().BoolVarP(&fetchPrune, "prune", "p", false, "prune remote-tracking branches that no longer exist")
	fetchCmd.Flags().BoolVarP(&fetchTags, "tags", "t", false, "fetch all tags from remote")
}

func runFetch(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Load config with profile support
	effective, _ := LoadEffectiveConfig(cmd, nil)
	if effective != nil {
		// Apply config if flag not explicitly set
		if !cmd.Flags().Changed("parallel") && effective.Parallel > 0 {
			fetchFlags.Parallel = effective.Parallel
		}
		if !cmd.Flags().Changed("all-remotes") {
			fetchAllRemotes = effective.Fetch.AllRemotes
		}
		if !cmd.Flags().Changed("prune") {
			fetchPrune = effective.Fetch.Prune
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
	if err := validateBulkDepth(cmd, fetchFlags.Depth); err != nil {
		return err
	}

	// Validate format
	if err := validateBulkFormat(fetchFlags.Format); err != nil {
		return err
	}

	// Create client
	client := repository.NewClient()

	// Create logger for verbose mode
	logger := createBulkLogger(verbose)

	// Build options
	opts := repository.BulkFetchOptions{
		Directory:         directory,
		Parallel:          fetchFlags.Parallel,
		MaxDepth:          fetchFlags.Depth,
		DryRun:            fetchFlags.DryRun,
		Verbose:           verbose,
		AllRemotes:        fetchAllRemotes,
		Prune:             fetchPrune,
		Tags:              fetchTags,
		IncludeSubmodules: fetchFlags.IncludeSubmodules,
		IncludePattern:    fetchFlags.Include,
		ExcludePattern:    fetchFlags.Exclude,
		Logger:            logger,
		ProgressCallback:  createProgressCallback("Fetching", fetchFlags.Format, quiet),
	}

	// Watch mode: continuously fetch at intervals
	if fetchFlags.Watch {
		return runFetchWatch(ctx, client, opts)
	}

	// One-time fetch
	if shouldShowProgress(fetchFlags.Format, quiet) {
		printScanningMessage(directory, fetchFlags.Depth, fetchFlags.Parallel, fetchFlags.DryRun)
	}

	result, err := client.BulkFetch(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk fetch failed: %w", err)
	}

	// Display scan completion message
	if shouldShowProgress(fetchFlags.Format, quiet) && result.TotalScanned == 0 {
		fmt.Printf("Scan complete: no repositories found\n")
	}

	// Display results (always output for JSON format, otherwise respect quiet flag)
	if fetchFlags.Format == "json" || !quiet {
		displayFetchResults(result)
	}

	return errPartialFailure(result.Summary[repository.StatusError], result.TotalProcessed)
}

func runFetchWatch(ctx context.Context, client repository.Client, opts repository.BulkFetchOptions) error {
	cfg := WatchConfig{
		Interval:      fetchFlags.Interval,
		Format:        fetchFlags.Format,
		Quiet:         quiet,
		OperationName: "fetch",
		Directory:     opts.Directory,
		MaxDepth:      opts.MaxDepth,
		Parallel:      opts.Parallel,
	}

	return RunBulkWatch(cfg, func() error {
		return executeFetch(ctx, client, opts)
	})
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
		})
	}

	issueStatuses := issueStatusSet("error", "no-remote")
	if fetchFlags.Format != "compact" {
		issueStatuses = issueStatusSet("error", "no-remote", "no-upstream", "auth-required")
	}

	RenderBulkResults(os.Stdout, BulkRenderConfig{
		Title:          "=== Fetch Results ===",
		Verb:           "Fetched",
		Format:         fetchFlags.Format,
		Verbose:        verbose,
		IssueStatuses:  issueStatuses,
		FormatStatus:   formatFetchStatus,
		ChangesCount:   func(row BulkRenderRow) int { return row.CommitsBehind },
		SuccessMessage: "✓ All repositories fetched successfully",
		ShowFooters:    true,
	}, BulkRenderInput{
		TotalScanned:   result.TotalScanned,
		TotalProcessed: result.TotalProcessed,
		Duration:       result.Duration,
		Summary:        result.Summary,
		Rows:           rows,
	})
}

func formatFetchStatus(row BulkRenderRow) string {
	switch row.Status {
	case "success", "fetched", "updated":
		if row.CommitsBehind > 0 && row.CommitsAhead > 0 {
			return fmt.Sprintf("%d↓ %d↑ fetched", row.CommitsBehind, row.CommitsAhead)
		}
		if row.CommitsBehind > 0 {
			return fmt.Sprintf("%d↓ fetched", row.CommitsBehind)
		}
		if row.CommitsAhead > 0 {
			return fmt.Sprintf("up-to-date %d↑", row.CommitsAhead)
		}
		return "up-to-date"
	case "up-to-date":
		if row.CommitsAhead > 0 {
			return fmt.Sprintf("up-to-date %d↑", row.CommitsAhead)
		}
		return "up-to-date"
	case "error":
		return "failed"
	case "no-remote":
		return "no remote"
	case "no-upstream":
		return "no upstream"
	case "would-fetch":
		if row.CommitsBehind > 0 {
			return fmt.Sprintf("would fetch %d↓", row.CommitsBehind)
		}
		return "would fetch"
	case "skipped":
		return "skipped"
	default:
		return row.Status
	}
}
