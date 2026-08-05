package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

var (
	diffFlags          BulkCommandFlags
	diffStaged         bool
	diffIncludeUntrack bool
	diffContextLines   int
	diffMaxSize        int
	diffNoDiffContent  bool
)

// diffCmd represents the diff command
var diffCmd = &cobra.Command{
	Use:   "diff [directory]",
	Short: "Show diffs across multiple repositories",
	Long: cliutil.QuickStartHelp(`  # Show diffs for all repositories in current directory
  gz-git diff --scan-depth 1

  # Show only staged changes
  gz-git diff --staged ~/projects

  # Include untracked files
  gz-git diff --include-untracked ~/projects

  # Summary only (no diff content)
  gz-git diff --no-content ~/projects

  # JSON output (for scripting/LLM)
  gz-git diff --format json ~/projects`) + cliutil.ExitCodesBulkHelp(),
	Example: ``,
	Args:    cobra.MaximumNArgs(1),
	RunE:    runDiff,
}

func init() {
	rootCmd.AddCommand(diffCmd)

	// Common bulk operation flags (excluding --dry-run: diff is read-only)
	addBulkFlagsWithOpts(diffCmd, &diffFlags, BulkFlagOptions{SkipDryRun: true, SkipFetch: true})

	// Diff-specific flags
	diffCmd.Flags().BoolVar(&diffStaged, "staged", false, "show only staged changes (--cached)")
	diffCmd.Flags().BoolVar(&diffIncludeUntrack, "include-untracked", false, "include untracked files in output")
	diffCmd.Flags().IntVarP(&diffContextLines, "context", "U", 3, "number of context lines around changes")
	diffCmd.Flags().IntVar(&diffMaxSize, "max-size", 100, "max diff size per repository in KB")
	diffCmd.Flags().BoolVar(&diffNoDiffContent, "no-content", false, "suppress diff content in verbose/json/llm output modes")
}

func runDiff(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		<-sigChan
		if !quiet {
			fmt.Println("\nInterrupted, cancelling...")
		}
		cancel()
	}()

	// Validate and parse directory
	directory, err := validateBulkDirectory(args)
	if err != nil {
		return err
	}

	// Validate depth
	if err := validateBulkDepth(cmd, diffFlags.Depth); err != nil {
		return err
	}

	// Validate format
	if err := validateBulkFormat(diffFlags.Format); err != nil {
		return err
	}

	// Create client
	client := repository.NewClient()

	// Create logger for verbose mode
	logger := createBulkLogger(verbose)

	// Build options
	opts := repository.BulkDiffOptions{
		Directory:         directory,
		Parallel:          diffFlags.Parallel,
		MaxDepth:          diffFlags.Depth,
		Staged:            diffStaged,
		IncludeUntracked:  diffIncludeUntrack,
		ContextLines:      diffContextLines,
		MaxDiffSize:       diffMaxSize * 1024, // Convert KB to bytes
		Verbose:           verbose,
		IncludeSubmodules: diffFlags.IncludeSubmodules,
		IncludePattern:    diffFlags.Include,
		ExcludePattern:    diffFlags.Exclude,
		Logger:            logger,
		ProgressCallback:  createProgressCallback("Scanning", diffFlags.Format, quiet),
	}

	// Watch mode
	if diffFlags.Watch {
		return runDiffWatch(ctx, client, opts)
	}

	return executeDiff(ctx, client, opts)
}

func runDiffWatch(_ context.Context, client repository.Client, opts repository.BulkDiffOptions) error {
	cfg := WatchConfig{
		Interval:      diffFlags.Interval,
		Format:        diffFlags.Format,
		Quiet:         quiet,
		OperationName: "diff",
		Directory:     opts.Directory,
		MaxDepth:      opts.MaxDepth,
		Parallel:      opts.Parallel,
	}

	return RunBulkWatch(cfg, func() error {
		return executeDiff(context.Background(), client, opts)
	})
}

func executeDiff(ctx context.Context, client repository.Client, opts repository.BulkDiffOptions) error {
	// Scanning phase
	if shouldShowProgress(diffFlags.Format, quiet) {
		printScanningMessage(opts.Directory, opts.MaxDepth, opts.Parallel, false)
	}

	// Execute bulk diff
	result, err := client.BulkDiff(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk diff failed: %w", err)
	}

	// Display results
	if diffFlags.Format == "json" || !quiet {
		displayDiffResults(result)
	}

	return errPartialFailure(result.Summary[repository.StatusError], result.TotalScanned)
}

func displayDiffResults(result *repository.BulkDiffResult) {
	// JSON or LLM output mode
	if diffFlags.Format == "json" || diffFlags.Format == "llm" {
		displayDiffResultsStructured(result, diffFlags.Format)
		return
	}

	// Compact mode: unchanged
	if diffFlags.Format == "compact" {
		displayDiffResultsCompact(result)
		return
	}

	if verbose {
		// Verbose: full diff output (old default behavior)
		displayDiffResultsDefault(result)
	} else {
		// Default: summary line + changed repos brief (no diff content)
		fmt.Println()
		durationStr := result.Duration.Round(100 * time.Millisecond).String()
		fmt.Printf("Diff %d repos  [⚠%d changed  ✓%d clean]  %s\n",
			result.TotalScanned, result.TotalWithChanges, result.TotalClean, durationStr)

		for _, repo := range result.Repositories {
			if repo.Status == "clean" {
				continue
			}
			icon := getDiffStatusIcon(repo.Status)
			changes := fmt.Sprintf("+%d/-%d", repo.Additions, repo.Deletions)
			fmt.Printf("  %s %-40s (%s)  %-24s %-10s %s\n",
				icon, repo.RelativePath, repo.Branch, formatDiffFileCount(repo), changes, repo.DiffSummary)

			if note := omittedNote(repo); note != "" {
				fmt.Printf("      %s (--verbose for reasons)\n", note)
			}
		}

		if result.TotalWithChanges == 0 {
			fmt.Println("✓ All repositories are clean")
		}
	}
}

func displayDiffResultsDefault(result *repository.BulkDiffResult) {
	fmt.Println()
	fmt.Println("=== Bulk Diff Results ===")
	fmt.Printf("Total scanned:     %d repositories\n", result.TotalScanned)
	fmt.Printf("With changes:      %d repositories\n", result.TotalWithChanges)
	fmt.Printf("Clean:             %d repositories\n", result.TotalClean)
	fmt.Printf("Duration:          %s\n", result.Duration.Round(100_000_000))
	fmt.Println()

	// Show each repository's diff
	for _, repo := range result.Repositories {
		if repo.Status == "clean" && !verbose {
			continue
		}

		displayDiffRepositoryResult(repo)
	}

	if result.TotalWithChanges == 0 {
		fmt.Println("✓ All repositories are clean")
	}
}

func displayDiffResultsCompact(result *repository.BulkDiffResult) {
	fmt.Println()
	fmt.Println("=== Bulk Diff Summary ===")
	fmt.Printf("Total: %d | With changes: %d | Clean: %d\n",
		result.TotalScanned, result.TotalWithChanges, result.TotalClean)
	fmt.Println()

	if result.TotalWithChanges == 0 {
		fmt.Println("✓ All repositories are clean")
		return
	}

	// The untracked column only appears when some repository has untracked
	// files. A column of zeroes would be pure noise on the common case, and the
	// "Files" heading only becomes ambiguous once a second file count sits next
	// to it — which is exactly when it is renamed to "Tracked".
	showUntracked := anyUntracked(result)
	if showUntracked {
		fmt.Printf("%-40s %-12s %-8s %-10s %-8s %s\n",
			"Repository", "Branch", "Tracked", "Untracked", "+/-", "Summary")
		fmt.Println(strings.Repeat("-", 100))
	} else {
		fmt.Printf("%-40s %-12s %-8s %-8s %s\n", "Repository", "Branch", "Files", "+/-", "Summary")
		fmt.Println(strings.Repeat("-", 90))
	}

	for _, repo := range result.Repositories {
		if repo.Status == "clean" {
			continue
		}

		icon := getDiffStatusIcon(repo.Status)
		path := repo.RelativePath
		if len(path) > 38 {
			path = "..." + path[len(path)-35:]
		}

		changes := fmt.Sprintf("+%d/-%d", repo.Additions, repo.Deletions)
		summary := repo.DiffSummary
		if len(summary) > 30 {
			summary = summary[:27] + "..."
		}
		if repo.Truncated {
			summary += " [truncated]"
		}
		if n := len(repo.OmittedFiles); n > 0 {
			summary += fmt.Sprintf(" [%d omitted]", n)
		}

		if showUntracked {
			fmt.Printf("%s %-38s %-12s %-8d %-10d %-8s %s\n",
				icon, path, repo.Branch, repo.FilesChanged, repo.UntrackedFilesChanged, changes, summary)

			continue
		}

		fmt.Printf("%s %-38s %-12s %-8d %-8s %s\n",
			icon, path, repo.Branch, repo.FilesChanged, changes, summary)
	}
}

// anyUntracked reports whether any repository in the result has untracked
// changes, which decides whether the compact table grows its extra column.
func anyUntracked(result *repository.BulkDiffResult) bool {
	for _, repo := range result.Repositories {
		if repo.UntrackedFilesChanged > 0 {
			return true
		}
	}

	return false
}

// formatDiffFileCount renders the file count for the default format, naming the
// untracked share explicitly.
//
// FilesChanged counts tracked paths only, because that is what `git diff` can
// compare. `gz-git commit` runs `git add -A` and records the untracked files
// too, so a default-format reader who took this number as "what will be
// committed" was systematically short — the reported case had diff say 4 where
// the commit recorded 7. The suffix is dropped entirely when there are no
// untracked files, so those lines gain padding but no new words.
func formatDiffFileCount(repo repository.RepositoryDiffResult) string {
	if repo.UntrackedFilesChanged == 0 {
		return fmt.Sprintf("%d files", repo.FilesChanged)
	}

	return fmt.Sprintf("%d files (+%d untracked)", repo.FilesChanged, repo.UntrackedFilesChanged)
}

// omittedNote renders the completeness warning shared by the human-readable
// formats, or "" when the diff body is complete. A diff that quietly dropped
// files reads exactly like one that had nothing to drop, so the count is
// surfaced even in the formats that never print a diff body.
func omittedNote(repo repository.RepositoryDiffResult) string {
	n := len(repo.OmittedFiles)
	if n == 0 {
		return ""
	}

	noun := "files"
	if n == 1 {
		noun = "file"
	}

	return fmt.Sprintf("⚠ %d untracked %s omitted from the diff body", n, noun)
}

func displayDiffRepositoryResult(repo repository.RepositoryDiffResult) {
	icon := getDiffStatusIcon(repo.Status)

	// Header
	fmt.Printf("\n%s === %s (%s) ===\n", icon, repo.RelativePath, repo.Branch)

	if repo.Error != nil {
		fmt.Printf("  Error: %v\n", repo.Error)
		return
	}

	if repo.Status == "clean" {
		fmt.Println("  No changes")
		return
	}

	// Summary line
	if repo.DiffSummary != "" {
		fmt.Printf("  %s\n", repo.DiffSummary)
	}

	// Changed files list
	if len(repo.ChangedFiles) > 0 {
		fmt.Println("  Changed files:")
		for _, file := range repo.ChangedFiles {
			statusIcon := getFileStatusIcon(file.Status)
			if file.OldPath != "" {
				fmt.Printf("    %s %s → %s\n", statusIcon, file.OldPath, file.Path)
			} else {
				fmt.Printf("    %s %s\n", statusIcon, file.Path)
			}
		}
	}

	// Untracked files
	if len(repo.UntrackedFiles) > 0 {
		fmt.Println("  Untracked files:")
		for _, file := range repo.UntrackedFiles {
			fmt.Printf("    ? %s\n", file)
		}
	}

	// Files whose content could not be included. Printed unconditionally: a
	// diff that silently dropped files is worse than one that says so. This is
	// the only format that also prints why each one was dropped.
	if note := omittedNote(repo); note != "" {
		fmt.Printf("  %s:\n", note)
		for _, omitted := range repo.OmittedFiles {
			fmt.Printf("    ! %s (%s)\n", omitted.Path, omitted.Reason)
		}
	}

	// Diff content (unless --no-content)
	if !diffNoDiffContent && repo.DiffContent != "" {
		fmt.Println()
		fmt.Println("  --- Diff ---")
		// Indent diff content
		lines := strings.SplitSeq(repo.DiffContent, "\n")
		for line := range lines {
			if line == "" {
				fmt.Println()
			} else {
				fmt.Printf("  %s\n", line)
			}
		}
		if repo.Truncated {
			fmt.Println("  ... [truncated due to size limit] ...")
		}
	}
}

func getDiffStatusIcon(status string) string {
	switch status {
	case "has-changes":
		return "⚠"
	case "clean":
		return "✓"
	case "error":
		return "✗"
	default:
		return "•"
	}
}

func getFileStatusIcon(status string) string {
	switch status {
	case "M":
		return "M" // Modified
	case "A":
		return "A" // Added
	case "D":
		return "D" // Deleted
	case "R":
		return "R" // Renamed
	case "C":
		return "C" // Copied
	default:
		return "?"
	}
}

// DiffJSONOutput represents the JSON output structure for diff command
type DiffJSONOutput struct {
	TotalScanned     int                        `json:"total_scanned"`
	TotalWithChanges int                        `json:"total_with_changes"`
	TotalClean       int                        `json:"total_clean"`
	DurationMs       int64                      `json:"duration_ms"`
	Summary          map[string]int             `json:"summary"`
	Repositories     []DiffRepositoryJSONOutput `json:"repositories"`
}

// DiffRepositoryJSONOutput represents a single repository in JSON output
type DiffRepositoryJSONOutput struct {
	Path                  string                  `json:"path"`
	Branch                string                  `json:"branch,omitempty"`
	Status                string                  `json:"status"`
	Scope                 string                  `json:"scope,omitempty"`
	FilesChanged          int                     `json:"files_changed,omitempty"`
	TrackedFilesChanged   int                     `json:"tracked_files_changed,omitempty"`
	UntrackedFilesChanged int                     `json:"untracked_files_changed,omitempty"`
	StagedFilesChanged    int                     `json:"staged_files_changed,omitempty"`
	Additions             int                     `json:"additions,omitempty"`
	Deletions             int                     `json:"deletions,omitempty"`
	DiffSummary           string                  `json:"diff_summary,omitempty"`
	DiffContent           string                  `json:"diff_content,omitempty"`
	ChangedFiles          []ChangedFileJSONOutput `json:"changed_files,omitempty"`
	UntrackedFiles        []string                `json:"untracked_files,omitempty"`
	OmittedFiles          []OmittedFileJSONOutput `json:"omitted_files,omitempty"`
	Truncated             bool                    `json:"truncated,omitempty"`
	DurationMs            int64                   `json:"duration_ms,omitempty"`
	Error                 string                  `json:"error,omitempty"`
}

// OmittedFileJSONOutput reports an untracked file whose content was not
// included in the diff body, so consumers can tell a complete diff from a
// partial one instead of having to assume.
type OmittedFileJSONOutput struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// ChangedFileJSONOutput represents a changed file in JSON output
type ChangedFileJSONOutput struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	OldPath string `json:"old_path,omitempty"`
}

func displayDiffResultsStructured(result *repository.BulkDiffResult, format string) {
	output := DiffJSONOutput{
		TotalScanned:     result.TotalScanned,
		TotalWithChanges: result.TotalWithChanges,
		TotalClean:       result.TotalClean,
		DurationMs:       result.Duration.Milliseconds(),
		Summary:          result.Summary,
		Repositories:     make([]DiffRepositoryJSONOutput, 0, len(result.Repositories)),
	}

	for _, repo := range result.Repositories {
		repoOutput := DiffRepositoryJSONOutput{
			Path:                  repo.RelativePath,
			Branch:                repo.Branch,
			Status:                repo.Status,
			Scope:                 repo.Scope,
			FilesChanged:          repo.FilesChanged,
			TrackedFilesChanged:   repo.TrackedFilesChanged,
			UntrackedFilesChanged: repo.UntrackedFilesChanged,
			StagedFilesChanged:    repo.StagedFilesChanged,
			Additions:             repo.Additions,
			Deletions:             repo.Deletions,
			DiffSummary:           repo.DiffSummary,
			UntrackedFiles:        repo.UntrackedFiles,
			Truncated:             repo.Truncated,
			DurationMs:            repo.Duration.Milliseconds(),
		}

		// Include diff content unless --no-content
		if !diffNoDiffContent {
			repoOutput.DiffContent = repo.DiffContent
		}

		// Omission reasons stay even under --no-content: they are metadata
		// about completeness, not diff body.
		for _, omitted := range repo.OmittedFiles {
			repoOutput.OmittedFiles = append(repoOutput.OmittedFiles, OmittedFileJSONOutput{
				Path:   omitted.Path,
				Reason: omitted.Reason,
			})
		}

		// Convert changed files
		for _, file := range repo.ChangedFiles {
			repoOutput.ChangedFiles = append(repoOutput.ChangedFiles, ChangedFileJSONOutput{
				Path:    file.Path,
				Status:  file.Status,
				OldPath: file.OldPath,
			})
		}

		if repo.Error != nil {
			repoOutput.Error = repo.Error.Error()
		}

		output.Repositories = append(output.Repositories, repoOutput)
	}

	writeBulkOutput(format, output)
}
