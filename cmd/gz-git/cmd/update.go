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
	updateFlags    BulkCommandFlags
	updateNoFetch  bool
	updateSyncBase bool
)

// updateCmd represents the update command for multi-repository operations.
var updateCmd = &cobra.Command{
	Use:   "update [directory]",
	Short: "Update multiple repositories in parallel",
	Long: cliutil.QuickStartHelp(`  # Update all repositories in current directory
  gz-git update

  # Update all repositories up to 2 levels deep
  gz-git update -d 2 .

  # Skip fetching (only update already fetched repos)
  gz-git update --skip-fetch ~/workspace

  # Also fast-forward each repo's base branch, which nothing checks out
  gz-git update --sync-base

  # Detailed output
  gz-git update --verbose`) + cliutil.ExitCodesBulkHelp(),
	Args: cobra.MaximumNArgs(1),
	RunE: runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)

	addBulkFlags(updateCmd, &updateFlags)

	updateCmd.Flags().BoolVar(&updateSyncBase, "sync-base", false,
		"also fast-forward each repository's base branch ref, even when it is not checked out")

	updateCmd.Flags().BoolVar(&updateNoFetch, "no-fetch", false, "deprecated: use --skip-fetch")
	if err := updateCmd.Flags().MarkDeprecated("no-fetch", "use --skip-fetch instead"); err != nil {
		panic(err)
	}
}

func runUpdate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	var baseCandidates []string

	effective, _ := LoadEffectiveConfig(cmd, nil)
	if effective != nil {
		if !cmd.Flags().Changed("parallel") && effective.Parallel > 0 {
			updateFlags.Parallel = effective.Parallel
		}
		// Same list `gz-git info` resolves its BASE column with. Repairing a
		// different branch than the one the report names would leave the report
		// unchanged and read as a broken command.
		baseCandidates = effective.Branch.DefaultBranch
		if verbose {
			PrintConfigSources(cmd, effective)
		}
	}

	directory, err := validateBulkDirectory(args)
	if err != nil {
		return err
	}

	if err := validateBulkDepth(cmd, updateFlags.Depth); err != nil {
		return err
	}

	if err := validateBulkFormat(updateFlags.Format); err != nil {
		return err
	}

	skipFetch := updateFlags.SkipFetch || updateNoFetch

	client := repository.NewClient()
	logger := createBulkLogger(verbose)

	opts := repository.BulkUpdateOptions{
		Directory:         directory,
		Parallel:          updateFlags.Parallel,
		MaxDepth:          updateFlags.Depth,
		DryRun:            updateFlags.DryRun,
		Verbose:           verbose,
		NoFetch:           skipFetch,
		IncludeSubmodules: updateFlags.IncludeSubmodules,
		IncludePattern:    updateFlags.Include,
		ExcludePattern:    updateFlags.Exclude,
		Logger:            logger,
		ProgressCallback:  createProgressCallback("Updating", updateFlags.Format, quiet),
		SyncBase:          updateSyncBase,
		BaseCandidates:    baseCandidates,
	}

	// Watch mode: continuously update at intervals
	if updateFlags.Watch {
		return runUpdateWatch(ctx, client, opts)
	}

	// One-time update
	if shouldShowProgress(updateFlags.Format, quiet) {
		printScanningMessage(directory, updateFlags.Depth, updateFlags.Parallel, updateFlags.DryRun)
	}

	result, err := client.BulkUpdate(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk update failed: %w", err)
	}

	// Display scan completion message
	if shouldShowProgress(updateFlags.Format, quiet) && result.TotalScanned == 0 {
		fmt.Printf("Scan complete: no repositories found\n")
	}

	// Display results (always output for JSON format, otherwise respect quiet flag)
	if updateFlags.Format == "json" || !quiet {
		displayUpdateResults(result)
	}

	return errPartialFailure(result.Summary[repository.StatusError], result.TotalProcessed)
}

func runUpdateWatch(ctx context.Context, client repository.Client, opts repository.BulkUpdateOptions) error {
	cfg := WatchConfig{
		Interval:      updateFlags.Interval,
		Format:        updateFlags.Format,
		Quiet:         quiet,
		OperationName: "update",
		Directory:     opts.Directory,
		MaxDepth:      opts.MaxDepth,
		Parallel:      opts.Parallel,
	}

	return RunBulkWatch(cfg, func() error {
		return executeUpdate(ctx, client, opts)
	})
}

func executeUpdate(ctx context.Context, client repository.Client, opts repository.BulkUpdateOptions) error {
	result, err := client.BulkUpdate(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk update failed: %w", err)
	}

	// Display results
	if !quiet {
		displayUpdateResults(result)
	}

	return nil
}

func displayUpdateResults(result *repository.BulkUpdateResult) {
	rows := make([]BulkRenderRow, 0, len(result.Repositories))
	for _, repo := range result.Repositories {
		rows = append(rows, BulkRenderRow{
			Path:                  repo.GetPath(),
			Branch:                repo.Branch,
			Status:                repo.GetStatus(),
			Message:               withBaseSyncNote(repo.GetMessage(), repo.BaseSync),
			Note:                  baseSyncNote(repo.BaseSync),
			Remote:                repo.Remote,
			Err:                   repo.GetError(),
			Duration:              repo.Duration,
			CommitsAhead:          repo.CommitsAhead,
			CommitsBehind:         repo.CommitsBehind,
			HasUncommittedChanges: repo.HasUncommittedChanges,
		})
	}

	issueStatuses := issueStatusSet("error", "dirty", "conflict", "base-blocked", "base-synced")
	if updateFlags.Format != "compact" {
		issueStatuses = issueStatusSet(
			"error", "dirty", "conflict", "base-blocked", "base-synced", "no-remote", "no-upstream",
			"auth-required", "rebase-in-progress", "merge-in-progress",
		)
	}

	RenderBulkResults(os.Stdout, BulkRenderConfig{
		Title:          "=== Update Results ===",
		Verb:           "Updated",
		Format:         updateFlags.Format,
		Verbose:        verbose,
		IssueStatuses:  issueStatuses,
		FormatStatus:   formatUpdateStatus,
		ChangesCount:   func(row BulkRenderRow) int { return row.CommitsBehind },
		SuccessMessage: "✓ All repositories updated successfully",
		ShowFooters:    false,
	}, BulkRenderInput{
		TotalScanned:   result.TotalScanned,
		TotalProcessed: result.TotalProcessed,
		Duration:       result.Duration,
		Summary:        result.Summary,
		Rows:           rows,
	})
}

func formatUpdateStatus(row BulkRenderRow) string {
	switch row.Status {
	case "success", "pulled", "updated":
		if row.CommitsBehind > 0 && row.CommitsAhead > 0 {
			return fmt.Sprintf("%d↓ %d↑ updated", row.CommitsBehind, row.CommitsAhead)
		}
		if row.CommitsBehind > 0 {
			return fmt.Sprintf("%d↓ updated", row.CommitsBehind)
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
	case "dirty":
		return "has changes"
	case "no-remote":
		return "no remote"
	case "no-upstream":
		return "no upstream"
	case "would-update":
		if row.CommitsBehind > 0 {
			return fmt.Sprintf("would update %d↓", row.CommitsBehind)
		}
		return "would update"
	case "skipped":
		if row.HasUncommittedChanges {
			return "skipped (dirty)"
		}
		return "skipped"
	case "base-synced", "base-blocked":
		// The note names the ref and the distance, which is the whole point of
		// the row. Fall back only if it is somehow empty, so the row is never
		// blank about why it was shown.
		if row.Note != "" {
			return row.Note
		}
		return "base diverged"
	case "conflict":
		return "conflict"
	case "rebase-in-progress":
		return "rebase in progress"
	case "merge-in-progress":
		return "merge in progress"
	default:
		return row.Status
	}
}

// withBaseSyncNote appends the base-ref outcome to a repository's message.
//
// It is a suffix on the existing message rather than its own column because the
// base ref is a second, independent answer about the same repository: the pull
// describes the branch you are on, the base sync describes one you are not.
// Collapsing them into a single verdict would make one of the two invisible.
//
// A nil result means --sync-base was off, which is not the same as "nothing to
// do" and must not print.
func withBaseSyncNote(message string, sync *repository.BaseSyncResult) string {
	note := baseSyncNote(sync)
	switch {
	case note == "":
		return message
	case message == "":
		return note
	default:
		return message + "; " + note
	}
}

// baseSyncNote renders the base-ref outcome on its own, for the status column.
//
// The note has to exist separately from the message because the shared bulk
// renderer passes Message to JSON output only — a note that lives solely in the
// message is invisible to everyone reading the terminal, which is everyone.
//
// A nil result means --sync-base was off, which is not the same as "nothing to
// do" and must not print.
func baseSyncNote(sync *repository.BaseSyncResult) string {
	if sync == nil {
		return ""
	}

	switch sync.Action {
	case repository.BaseSyncFastForward:
		return fmt.Sprintf("base %s +%d", sync.Base, sync.Advanced)
	case repository.BaseSyncAdopted:
		return fmt.Sprintf("base %s +%d (adopted)", sync.Base, sync.Advanced)
	case repository.BaseSyncBlocked:
		return fmt.Sprintf("base %s blocked: %s", sync.Base, sync.Reason)
	case repository.BaseSyncUpToDate, repository.BaseSyncSkipped:
		// Silent by design. A run over thirty repositories where twenty-eight
		// bases are already current should not print twenty-eight rows saying
		// so; the finding is the exception, not the census.
		return ""
	}
	return ""
}
