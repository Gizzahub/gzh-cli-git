package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/reposync"
)

// WatchModeHelpText provides consistent documentation for watch mode across commands.
//
// This constant centralizes watch mode documentation to ensure consistency
// across all commands that support the --watch flag (status, fetch, pull, push).
const WatchModeHelpText = `
Watch Mode:
  Use --watch to continuously execute the operation at regular intervals.

  Interval Format (Go duration syntax):
    30s   - 30 seconds
    5m    - 5 minutes
    2h    - 2 hours
    1h30m - 1 hour 30 minutes

  Behavior:
    - Clears screen between iterations
    - Shows timestamp for each run
    - Ctrl+C to stop gracefully
    - Errors don't stop watch mode (continues on next interval)

  Use Cases:
    - Monitor repositories for incoming changes
    - Auto-fetch every 5 minutes to stay updated
    - Track sync status in real-time
    - CI/CD polling for git state changes`

// BulkCommandFlags holds common flags for bulk operations (fetch, pull, push).
type BulkCommandFlags struct {
	Depth             int
	Parallel          int
	DryRun            bool
	IncludeSubmodules bool
	Include           string
	Exclude           string
	Format            string
	Watch             bool
	Interval          time.Duration
	SkipFetch         bool
}

// BulkFlagOptions allows customizing which bulk flags are registered.
type BulkFlagOptions struct {
	SkipDryRun    bool
	SkipFetch     bool
	SkipFormat    bool
	SkipWatch     bool
	SkipRecursive bool
	SkipScanDepth bool
	SkipInclude   bool
	SkipExclude   bool
}

// addBulkFlagsWithOpts registers common bulk operation flags to a command, skipping those specified in opts.
func addBulkFlagsWithOpts(cmd *cobra.Command, flags *BulkCommandFlags, opts BulkFlagOptions) {
	if !opts.SkipScanDepth {
		cmd.Flags().IntVarP(&flags.Depth, "scan-depth", "d", repository.DefaultLocalScanDepth, "directory depth to scan for repositories")
	}
	cmd.Flags().IntVarP(&flags.Parallel, "parallel", "j", repository.DefaultLocalParallel, "number of parallel operations")
	if !opts.SkipRecursive {
		cmd.Flags().BoolVarP(&flags.IncludeSubmodules, "recursive", "r", false, "recursively include nested repositories and submodules")
	}
	if !opts.SkipInclude {
		cmd.Flags().StringVar(&flags.Include, "include", "", "regex pattern to include repositories")
	}
	if !opts.SkipExclude {
		cmd.Flags().StringVar(&flags.Exclude, "exclude", "", "regex pattern to exclude repositories")
	}

	if !opts.SkipFormat {
		cmd.Flags().StringVar(&flags.Format, "format", "default", "output format: default, compact, json, llm")
	}
	if !opts.SkipDryRun {
		cmd.Flags().BoolVarP(&flags.DryRun, "dry-run", "n", false, "show what would be done without doing it")
	}
	if !opts.SkipWatch {
		cmd.Flags().BoolVar(&flags.Watch, "watch", false, "continuously run at intervals")
		cmd.Flags().DurationVar(&flags.Interval, "interval", 5*time.Minute, "interval when watching")
	}
	if !opts.SkipFetch {
		cmd.Flags().BoolVar(&flags.SkipFetch, "skip-fetch", false, "skip fetching from remote (use local state only)")
	}
}

// addBulkFlags registers common bulk operation flags to a command.
func addBulkFlags(cmd *cobra.Command, flags *BulkCommandFlags) {
	addBulkFlagsWithOpts(cmd, flags, BulkFlagOptions{})
}

// validateBulkDirectory parses and validates the directory argument
// Returns the directory path (defaults to ".") or an error.
func validateBulkDirectory(args []string) (string, error) {
	directory := "."
	if len(args) > 0 {
		directory = args[0]
	}

	// Validate directory exists
	if _, err := os.Stat(directory); err != nil {
		return "", fmt.Errorf("directory does not exist: %s", directory)
	}

	return directory, nil
}

// validateBulkDepth validates the scan-depth flag
// Returns an error if depth is explicitly set to 0.
func validateBulkDepth(cmd *cobra.Command, depth int) error {
	if cmd.Flags().Changed("scan-depth") && depth == 0 {
		return fmt.Errorf("scan-depth must be at least 1 (use --scan-depth 1 to scan current directory and immediate subdirectories)")
	}
	return nil
}

// errPartialFailure returns an exit-code-2 error when some repositories failed,
// so the process exit code distinguishes partial failure (2) from tool/config
// errors (1) in scripts and CI. Returns nil when nothing failed.
func errPartialFailure(failed, total int) error {
	if failed > 0 {
		return cliutil.NewExitError(cliutil.ExitPartialFailed,
			fmt.Errorf("%d of %d repositories failed", failed, total))
	}
	return nil
}

// CoreFormats contains formats supported by all commands.
var CoreFormats = cliutil.CoreFormats

// ValidBulkFormats contains valid output formats for bulk operations
// Core formats + compact (bulk-specific).
var ValidBulkFormats = cliutil.CoreFormats

// ValidHistoryFormats contains valid output formats for history commands
// Core formats + table, csv, markdown (history-specific).
var ValidHistoryFormats = cliutil.TabularFormats

// validateBulkFormat validates the format flag for bulk operations
// Returns an error if the format is not supported.
func validateBulkFormat(format string) error {
	return cliutil.ValidateFormat(format, ValidBulkFormats)
}

// validateHistoryFormat validates the format flag for history commands
// Returns an error if the format is not supported.
func validateHistoryFormat(format string) error {
	return cliutil.ValidateFormat(format, ValidHistoryFormats)
}

// createBulkLogger creates a logger for bulk operations
// Returns a logger if verbose mode is enabled, nil otherwise.
func createBulkLogger(verbose bool) repository.Logger {
	if verbose {
		return repository.NewWriterLogger(os.Stdout)
	}
	return nil
}

// createProgressCallback creates a progress callback function for bulk operations
// The callback is used to display progress during bulk operations.
func createProgressCallback(operationName, format string, quiet bool) func(int, int, string) {
	return func(current, total int, repo string) {
		if !quiet && format != "compact" && !cliutil.IsMachineFormat(format) {
			fmt.Printf("[%d/%d] %s %s...\n", current, total, operationName, repo)
		}
	}
}

// shouldShowProgress returns true if progress messages should be displayed.
func shouldShowProgress(format string, quiet bool) bool {
	return !quiet && !cliutil.IsMachineFormat(format)
}

// getBulkStatusIcon returns the appropriate icon for bulk operation status.
// The changesCount parameter indicates actual changes (commits behind/ahead/pushed).
// Icons: ✓ (changes occurred/clean), = (no changes), ✗ (error), ⚠ (warning), ⊘ (skipped).
func getBulkStatusIcon(status string, changesCount int) string {
	switch status {
	// Clean state (for status command)
	case "clean":
		return "✓"

	// Success states - show ✓ only if changes occurred
	case "success", "fetched", "pulled", "pushed", "updated":
		if changesCount > 0 {
			return "✓"
		}
		return "=" // No changes = up-to-date

	// Up-to-date states
	case "nothing-to-push", "up-to-date":
		return "="

	// Error states
	case "error":
		return "✗"

	// Conflict/in-progress states
	case "conflict":
		return "⚡"
	case "rebase-in-progress":
		return "↻"
	case "merge-in-progress":
		return "⇄"
	case "dirty":
		return "⚠"

	// Clean states
	case "cleaned":
		return "✓"
	case "nothing-to-clean":
		return "="

	// Skipped/dry-run states
	case "skipped":
		return "⊘"
	case "would-fetch", "would-pull", "would-push", "would-update", "would-clean":
		return "→"

	// Base-ref repair states
	case "base-synced":
		return "✓"

	// Warning states
	case "no-remote", "no-upstream", "base-blocked":
		return "⚠"

	// Authentication required
	case "auth-required":
		return "🔐"

	default:
		return "•"
	}
}

// getBulkStatusIconSimple returns the icon without considering changes count.
func getBulkStatusIconSimple(status string) string {
	return getBulkStatusIcon(status, 0)
}

// summaryDisplayOrder defines the preferred display order for summary line items.
var summaryDisplayOrder = []string{
	"up-to-date", "nothing-to-push", "nothing-to-clean",
	"success", "fetched", "pulled", "pushed", "updated", "cleaned",
	"would-fetch", "would-pull", "would-push", "would-update", "would-clean",
	"skipped",
	"dirty",
	"no-remote", "no-upstream",
	"auth-required",
	"conflict", "rebase-in-progress", "merge-in-progress",
	"error",
}

// getSummaryIcon returns an icon for summary line display.
// Uses directional arrows for operations (↓ for fetch/pull, ↑ for push).
func getSummaryIcon(status string) string {
	switch status {
	case "up-to-date", "nothing-to-push", "nothing-to-clean":
		return "="
	case "fetched", "pulled", "updated":
		return "↓"
	case "pushed":
		return "↑"
	case "success", "cleaned":
		return "✓"
	case "would-fetch", "would-pull", "would-push", "would-update", "would-clean":
		return "→"
	case "skipped":
		return "⊘"
	case "dirty":
		return "⚠"
	case "error":
		return "✗"
	case "no-remote", "no-upstream":
		return "⚠"
	case "auth-required":
		return "🔐"
	case "conflict":
		return "⚡"
	case "rebase-in-progress":
		return "↻"
	case "merge-in-progress":
		return "⇄"
	default:
		return "•"
	}
}

// WriteSummaryLine prints a one-line summary for bulk operations.
// Example: "Fetched 6 repos  [=4 up-to-date  ↓2 fetched]  1.2s".
func WriteSummaryLine(w io.Writer, verb string, total int, summary map[string]int, duration time.Duration) {
	var parts []string
	seen := make(map[string]bool)
	for _, status := range summaryDisplayOrder {
		count, ok := summary[status]
		if !ok || count == 0 {
			continue
		}
		seen[status] = true
		icon := getSummaryIcon(status)
		parts = append(parts, fmt.Sprintf("%s%d %s", icon, count, status))
	}
	for status, count := range summary {
		if count == 0 || seen[status] {
			continue
		}
		icon := getSummaryIcon(status)
		parts = append(parts, fmt.Sprintf("%s%d %s", icon, count, status))
	}

	bracket := ""
	if len(parts) > 0 {
		bracket = "  [" + strings.Join(parts, "  ") + "]"
	}

	durationStr := duration.Round(100 * time.Millisecond).String()
	fmt.Fprintf(w, "%s %d repos%s  %s\n", verb, total, bracket, durationStr)
}

// WriteHealthSummaryLine prints a one-line health summary for diagnostic status.
// Example: "Status 6 repos  [✓4 healthy  ⚠1 warning  ✗1 error]  1.2s".
func WriteHealthSummaryLine(w io.Writer, total int, summary reposync.HealthSummary, duration time.Duration) {
	var parts []string
	if summary.Healthy > 0 {
		parts = append(parts, fmt.Sprintf("✓%d healthy", summary.Healthy))
	}
	if summary.Warning > 0 {
		parts = append(parts, fmt.Sprintf("⚠%d warning", summary.Warning))
	}
	if summary.Error > 0 {
		parts = append(parts, fmt.Sprintf("✗%d error", summary.Error))
	}
	if summary.Unreachable > 0 {
		parts = append(parts, fmt.Sprintf("⊘%d unreachable", summary.Unreachable))
	}

	bracket := ""
	if len(parts) > 0 {
		bracket = "  [" + strings.Join(parts, "  ") + "]"
	}

	durationStr := duration.Round(100 * time.Millisecond).String()
	fmt.Fprintf(w, "Status %d repos%s  %s\n", total, bracket, durationStr)
}

// WatchConfig holds configuration for watch mode operations.
type WatchConfig struct {
	Interval      time.Duration
	Format        string
	Quiet         bool
	OperationName string // e.g., "fetch", "pull", "push", "status check"
	Directory     string
	MaxDepth      int
	Parallel      int
}

// printScanningMessage prints the standard scanning message for bulk operations.
// This centralizes the message format for consistency across all bulk commands.
func printScanningMessage(directory string, depth, parallel int, dryRun bool) {
	suffix := ""
	if dryRun {
		suffix = " [DRY-RUN]"
	}
	fmt.Printf("Scanning for repositories in %s (depth: %d, parallel: %d)%s...\n", directory, depth, parallel, suffix)
}

// WatchExecutor is a function that executes the bulk operation once.
// Returns an error if the operation fails.
type WatchExecutor func() error

// StatusJSONOutput represents the JSON output structure for status command.
type StatusJSONOutput struct {
	TotalScanned   int                          `json:"total_scanned"`
	TotalProcessed int                          `json:"total_processed"`
	DurationMs     int64                        `json:"duration_ms"`
	Summary        map[string]int               `json:"summary"`
	Repositories   []StatusRepositoryJSONOutput `json:"repositories"`
}

// StatusRepositoryJSONOutput represents a single repository in JSON output.
type StatusRepositoryJSONOutput struct {
	Path             string   `json:"path"`
	Branch           string   `json:"branch,omitempty"`
	Status           string   `json:"status"`
	UncommittedFiles int      `json:"uncommitted_files,omitempty"`
	UntrackedFiles   int      `json:"untracked_files,omitempty"`
	CommitsAhead     int      `json:"commits_ahead,omitempty"`
	CommitsBehind    int      `json:"commits_behind,omitempty"`
	ConflictFiles    []string `json:"conflict_files,omitempty"`
	DurationMs       int64    `json:"duration_ms,omitempty"`
	Error            string   `json:"error,omitempty"`
}

// displayStatusResultsStructured displays BulkStatusResult in structured format (JSON/LLM).
// Shared by status and info commands.
func displayStatusResultsStructured(result *repository.BulkStatusResult, format string) {
	output := StatusJSONOutput{
		TotalScanned:   result.TotalScanned,
		TotalProcessed: result.TotalProcessed,
		DurationMs:     result.Duration.Milliseconds(),
		Summary:        result.Summary,
		Repositories:   make([]StatusRepositoryJSONOutput, 0, len(result.Repositories)),
	}

	for _, repo := range result.Repositories {
		repoOutput := StatusRepositoryJSONOutput{
			Path:             repo.RelativePath,
			Branch:           repo.Branch,
			Status:           repo.Status,
			UncommittedFiles: repo.TrackedChangedFiles,
			UntrackedFiles:   repo.UntrackedFiles,
			CommitsAhead:     repo.CommitsAhead,
			CommitsBehind:    repo.CommitsBehind,
			ConflictFiles:    repo.ConflictFiles,
			DurationMs:       repo.Duration.Milliseconds(),
		}
		if repo.Error != nil {
			repoOutput.Error = repo.Error.Error()
		}
		output.Repositories = append(output.Repositories, repoOutput)
	}

	writeBulkOutput(format, output)
}

// RunBulkWatch runs a bulk operation in watch mode with proper signal handling.
// This centralizes the watch loop logic used by fetch, pull, push, and status commands.
func RunBulkWatch(cfg WatchConfig, executor WatchExecutor) error {
	if !cfg.Quiet {
		fmt.Printf("Starting watch mode: %s every %s\n", cfg.OperationName, cfg.Interval)
		printScanningMessage(cfg.Directory, cfg.MaxDepth, cfg.Parallel, false)
		fmt.Println("Press Ctrl+C to stop...")
		fmt.Println()
	}

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Create ticker for periodic execution
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	// Perform initial execution immediately
	if err := executor(); err != nil {
		return err
	}

	// Watch loop
	for {
		select {
		case <-sigChan:
			if !cfg.Quiet {
				fmt.Println("\nStopping watch...")
			}
			return nil

		case <-ticker.C:
			if shouldShowProgress(cfg.Format, cfg.Quiet) {
				fmt.Printf("\n[%s] Running scheduled %s...\n", time.Now().Format("15:04:05"), cfg.OperationName)
			}
			if err := executor(); err != nil {
				if !cfg.Quiet {
					fmt.Fprintf(os.Stderr, "%s error: %v\n", cfg.OperationName, err)
				}
				// Continue watching even on error
			}
		}
	}
}

// ─── destructive-op safety helpers ─────────────────────────────────────────────
// Shared safety primitives for the destructive-op policy (dry-run default,
// confirm gate on bulk×destructive, SIGINT graceful shutdown).
// See docs/design/DESTRUCTIVE_OP_SAFETY.md.

// withInterruptCancel returns a child context that is canceled when the process
// receives SIGINT or SIGTERM, so long-running bulk operations stop gracefully and
// report partial results instead of being hard-killed by the default handler.
// The caller MUST defer the returned cancel func; it also tears down the signal
// handler and lets the watcher goroutine exit (no leak).
func withInterruptCancel(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		select {
		case <-sigChan:
			if !quiet {
				fmt.Fprintln(os.Stderr, "\nInterrupted, canceling...")
			}
			cancel()
		case <-ctx.Done():
		}
		signal.Stop(sigChan)
	}()

	return ctx, cancel
}

// stdinIsInteractive reports whether stdin is a terminal we can prompt on. The
// confirmation answer is read from stdin, so the gate keys on stdin (not stdout):
// a piped or CI invocation is treated as non-interactive. It shares cliutil's
// TTY detection with the color gate.
func stdinIsInteractive() bool {
	return cliutil.IsTerminal(os.Stdin.Fd())
}

// confirmDestructiveBulk gates a destructive bulk operation. The caller must have
// already printed the deletion summary. On an interactive terminal it asks y/N;
// in a non-interactive environment it refuses unless assumeYes (the --yes flag)
// is set, so piped/CI invocations cannot delete without explicitly opting in.
func confirmDestructiveBulk(assumeYes bool) (bool, error) {
	if assumeYes {
		return true, nil
	}
	if !stdinIsInteractive() {
		return false, errors.New("refusing to run a destructive bulk operation without confirmation; re-run with --yes to proceed in a non-interactive environment")
	}
	fmt.Fprint(os.Stderr, "Proceed with deletion? [y/N]: ")
	return readYesNo(os.Stdin)
}

// readYesNo reads a single line and reports whether it is an affirmative answer.
// Anything other than "y"/"yes" (case-insensitive), including EOF or a bare
// newline, is treated as No — the safe default for destructive operations.
func readYesNo(r io.Reader) (bool, error) {
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("failed to read confirmation: %w", err)
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

// writeBulkOutput serializes the output for bulk operations based on the format.
// It handles "json" and "llm" formats centrally.
func writeBulkOutput(format string, output any) {
	switch format {
	case "json":
		if err := cliutil.WriteJSON(os.Stdout, output, true); err != nil {
			fmt.Fprintf(os.Stderr, "error writing JSON: %v\n", err)
		}
	case "llm":
		if err := cliutil.WriteLLM(os.Stdout, output); err != nil {
			fmt.Fprintf(os.Stderr, "error writing LLM: %v\n", err)
		}
	}
}
