package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/reposync"
)

var statusFlags BulkCommandFlags

// statusCmd represents the status command.
var statusCmd = &cobra.Command{
	Use:   "status [directory]",
	Short: "Check status of multiple repositories",
	Long: cliutil.QuickStartHelp(`  # Check status of current directory (default)
  gz-git status

  # Check specific directory with filter
  gz-git status --include "myproject.*" ~/workspace

  # Exclude pattern
  gz-git status --exclude "test.*" ~/projects

  # Compact output format
  gz-git status --format compact ~/projects

  # Quick check (skip network fetch)
  gz-git status --skip-fetch

  # Continuously check at intervals (watch mode)
  gz-git status --scan-depth 2 --watch --interval 30s ~/projects`) + cliutil.ExitCodesBulkHelp(),
	Args: cobra.MaximumNArgs(1),
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)

	// Common bulk operation flags
	addBulkFlags(statusCmd, &statusFlags)
}

func runStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Load config with profile support
	effective, _ := LoadEffectiveConfig(cmd, nil)
	if effective != nil {
		// Apply config if flag not explicitly set
		if !cmd.Flags().Changed("parallel") && effective.Parallel > 0 {
			statusFlags.Parallel = effective.Parallel
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
	if err := resolveBulkDepth(cmd, directory, &statusFlags.Depth); err != nil {
		return err
	}

	// Validate format
	if err := validateBulkFormat(statusFlags.Format); err != nil {
		return err
	}

	// Create client
	client := repository.NewClient()

	// Create logger for verbose mode
	logger := createBulkLogger(verbose)

	// Watch mode: continuously check at intervals
	if statusFlags.Watch {
		return runStatusWatch(ctx, client, directory, logger)
	}

	// One-time status check with diagnostic
	if shouldShowProgress(statusFlags.Format, quiet) {
		printScanningMessage(directory, statusFlags.Depth, statusFlags.Parallel, false)
	}

	result, err := runDiagnosticStatus(ctx, client, directory, logger)
	if err != nil {
		return fmt.Errorf("status check failed: %w", err)
	}

	// Handle no repositories found
	if len(result.Results) == 0 {
		if statusFlags.Format == "json" || statusFlags.Format == "llm" {
			displayDiagnosticResultsStructured(result, statusFlags.Format)
		} else if !quiet {
			fmt.Printf("No repositories found in %s (depth: %d)\n", directory, statusFlags.Depth)
		}
		return nil
	}

	// Display results (always output for JSON format, otherwise respect quiet flag)
	if statusFlags.Format == "json" || !quiet {
		displayDiagnosticResults(result)
	}

	// Exit 2 only when the health check itself failed for some repositories;
	// warning/unreachable are health states, not operation failures.
	return errPartialFailure(result.Summary.Error, result.Summary.Total)
}

func runDiagnosticStatus(ctx context.Context, client repository.Client, directory string, logger repository.Logger) (*reposync.HealthReport, error) {
	// Scan for repositories (lightweight, no GetInfo/GetStatus)
	scanResult, err := client.ScanRepositories(ctx, repository.ScanOptions{
		Directory:         directory,
		MaxDepth:          statusFlags.Depth,
		IncludeSubmodules: statusFlags.IncludeSubmodules,
		IncludePattern:    statusFlags.Include,
		ExcludePattern:    resolveScanExclude(directory, statusFlags.Exclude),
		Logger:            logger,
	})
	if err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}

	// Convert scanned paths to RepoSpec for diagnostic
	var repoSpecs []reposync.RepoSpec
	for _, repoPath := range scanResult.Paths {
		repoSpecs = append(repoSpecs, reposync.RepoSpec{
			TargetPath: repoPath,
		})
	}

	// Handle empty result
	if len(repoSpecs) == 0 {
		return &reposync.HealthReport{
			Results:   []reposync.RepoHealth{},
			Summary:   reposync.HealthSummary{},
			CheckedAt: time.Now(),
		}, nil
	}

	// Run diagnostic health check (single pass: fetch + info + status)
	executor := reposync.DiagnosticExecutor{
		Client: client,
		Logger: logger,
	}

	opts := reposync.DefaultDiagnosticOptions()
	opts.Parallel = statusFlags.Parallel
	opts.SkipFetch = statusFlags.SkipFetch

	return executor.CheckHealth(ctx, repoSpecs, opts)
}

func runStatusWatch(ctx context.Context, client repository.Client, directory string, logger repository.Logger) error {
	cfg := WatchConfig{
		Interval:      statusFlags.Interval,
		Format:        statusFlags.Format,
		Quiet:         quiet,
		OperationName: "status check",
		Directory:     directory,
		MaxDepth:      statusFlags.Depth,
		Parallel:      statusFlags.Parallel,
	}

	return RunBulkWatch(cfg, func() error {
		return executeStatusDiagnostic(ctx, client, directory, logger)
	})
}

func executeStatusDiagnostic(ctx context.Context, client repository.Client, directory string, logger repository.Logger) error {
	result, err := runDiagnosticStatus(ctx, client, directory, logger)
	if err != nil {
		return fmt.Errorf("diagnostic status failed: %w", err)
	}

	// Display results
	if !quiet {
		displayDiagnosticResults(result)
	}

	return nil
}

func FormatUpstreamFixHint(branch, remote string) string {
	if branch == "" {
		return ""
	}
	if remote == "" {
		remote = "origin"
	}
	return fmt.Sprintf("    → Fix: git branch --set-upstream-to=%s/%s %s\n", remote, branch, branch)
}

func displayDiagnosticResults(report *reposync.HealthReport) {
	// JSON or LLM output mode
	if statusFlags.Format == "json" || statusFlags.Format == "llm" {
		displayDiagnosticResultsStructured(report, statusFlags.Format)
		return
	}

	// Compact output mode: unchanged
	if statusFlags.Format == "compact" {
		displayDiagnosticResultsCompact(report)
		return
	}

	fmt.Println()

	if verbose {
		// Verbose: full detailed output (old default behavior)
		fmt.Println("=== Repository Health Status ===")
		fmt.Printf("Total repositories: %d\n", len(report.Results))
		fmt.Printf("Duration:           %s\n", report.TotalDuration.Round(100_000_000))
		fmt.Println()

		if report.Summary.Total > 0 {
			fmt.Println("Summary by health:")
			if report.Summary.Healthy > 0 {
				icon := getHealthIcon(reposync.HealthHealthy)
				fmt.Printf("  %s %-15s %d\n", icon, "healthy:", report.Summary.Healthy)
			}
			if report.Summary.Warning > 0 {
				icon := getHealthIcon(reposync.HealthWarning)
				fmt.Printf("  %s %-15s %d\n", icon, "warning:", report.Summary.Warning)
			}
			if report.Summary.Error > 0 {
				icon := getHealthIcon(reposync.HealthError)
				fmt.Printf("  %s %-15s %d\n", icon, "error:", report.Summary.Error)
			}
			if report.Summary.Unreachable > 0 {
				icon := getHealthIcon(reposync.HealthUnreachable)
				fmt.Printf("  %s %-15s %d\n", icon, "unreachable:", report.Summary.Unreachable)
			}
			fmt.Println()
		}

		if len(report.Results) > 0 {
			fmt.Println("Repository details:")
			for _, repo := range report.Results {
				displayHealthRepositoryResult(repo)
			}
		}
	} else {
		// Default: summary line + issues only
		WriteHealthSummaryLine(os.Stdout, len(report.Results), report.Summary, report.TotalDuration)

		hasIssues := false
		for _, repo := range report.Results {
			if repo.HealthStatus != reposync.HealthHealthy {
				if !hasIssues {
					hasIssues = true
				}
				displayHealthRepositoryResult(repo)
			}
		}
		if !hasIssues {
			fmt.Printf("✓ All %d repositories are healthy\n", len(report.Results))
		}
	}
}

func displayDiagnosticResultsStructured(report *reposync.HealthReport, format string) {
	// Field names unified with BulkStatus JSON (bulk_common.go)
	type RepoHealthJSON struct {
		Path             string `json:"path"`
		HealthStatus     string `json:"health_status"`
		NetworkStatus    string `json:"network_status,omitempty"`
		DivergenceType   string `json:"divergence_type,omitempty"`
		Branch           string `json:"branch,omitempty"`
		Upstream         string `json:"upstream,omitempty"`
		CommitsAhead     int    `json:"commits_ahead,omitempty"`
		CommitsBehind    int    `json:"commits_behind,omitempty"`
		UncommittedFiles int    `json:"uncommitted_files,omitempty"`
		UntrackedFiles   int    `json:"untracked_files,omitempty"`
		ConflictFiles    int    `json:"conflict_files,omitempty"`
		Recommendation   string `json:"recommendation,omitempty"`
		Error            string `json:"error,omitempty"`
		DurationMs       int64  `json:"duration_ms"`
		FetchDurationMs  int64  `json:"fetch_duration_ms,omitempty"`
	}

	output := struct {
		TotalRepositories int   `json:"total_repositories"`
		DurationMs        int64 `json:"duration_ms"`
		Summary           struct {
			Healthy     int `json:"healthy"`
			Warning     int `json:"warning"`
			Error       int `json:"error"`
			Unreachable int `json:"unreachable"`
			Total       int `json:"total"`
		} `json:"summary"`
		Repositories []RepoHealthJSON `json:"repositories"`
	}{
		TotalRepositories: len(report.Results),
		DurationMs:        report.TotalDuration.Milliseconds(),
		Repositories:      make([]RepoHealthJSON, 0, len(report.Results)),
	}

	// Convert summary
	output.Summary.Healthy = report.Summary.Healthy
	output.Summary.Warning = report.Summary.Warning
	output.Summary.Error = report.Summary.Error
	output.Summary.Unreachable = report.Summary.Unreachable
	output.Summary.Total = report.Summary.Total

	// Convert repositories
	for _, repo := range report.Results {
		repoJSON := RepoHealthJSON{
			Path:             repo.Repo.TargetPath,
			HealthStatus:     string(repo.HealthStatus),
			NetworkStatus:    string(repo.NetworkStatus),
			DivergenceType:   string(repo.DivergenceType),
			Branch:           repo.CurrentBranch,
			Upstream:         repo.UpstreamBranch,
			CommitsAhead:     repo.AheadBy,
			CommitsBehind:    repo.BehindBy,
			UncommittedFiles: repo.ModifiedFiles,
			UntrackedFiles:   repo.UntrackedFiles,
			ConflictFiles:    repo.ConflictFiles,
			Recommendation:   repo.Recommendation,
			DurationMs:       repo.Duration.Milliseconds(),
			FetchDurationMs:  repo.FetchDuration.Milliseconds(),
		}
		if repo.Error != nil {
			repoJSON.Error = repo.Error.Error()
		}
		output.Repositories = append(output.Repositories, repoJSON)
	}

	writeBulkOutput(format, output)
}

func displayDiagnosticResultsCompact(report *reposync.HealthReport) {
	// Summary line
	WriteHealthSummaryLine(os.Stdout, len(report.Results), report.Summary, report.TotalDuration)

	// Show only repos with issues
	hasIssues := false
	for _, repo := range report.Results {
		if repo.HealthStatus != reposync.HealthHealthy {
			hasIssues = true
			icon := getHealthIcon(repo.HealthStatus)
			repoName := filepath.Base(repo.Repo.TargetPath)
			fmt.Printf(
				"%s %s (%s) - %s\n",
				icon,
				repoName,
				repo.CurrentBranch,
				repo.Recommendation,
			)
		}
	}

	if !hasIssues {
		fmt.Println("✓ All repositories are healthy")
	}
}

func displayHealthRepositoryResult(health reposync.RepoHealth) {
	icon := getHealthIcon(health.HealthStatus)
	repoName := filepath.Base(health.Repo.TargetPath)

	// Build status line
	statusParts := []string{icon, repoName}
	if health.CurrentBranch != "" {
		statusParts = append(statusParts, fmt.Sprintf("(%s)", health.CurrentBranch))
	}

	// Add divergence info
	if health.DivergenceType != reposync.DivergenceNone {
		divergenceStr := ""
		switch health.DivergenceType {
		case reposync.DivergenceFastForward:
			divergenceStr = fmt.Sprintf("↓%d", health.BehindBy)
		case reposync.DivergenceAhead:
			divergenceStr = fmt.Sprintf("↑%d", health.AheadBy)
		case reposync.DivergenceDiverged:
			divergenceStr = fmt.Sprintf("↓%d ↑%d", health.BehindBy, health.AheadBy)
		case reposync.DivergenceConflict:
			divergenceStr = fmt.Sprintf("CONFLICT: %d files", health.ConflictFiles)
		case reposync.DivergenceNoUpstream:
			divergenceStr = "no upstream"
		}
		if divergenceStr != "" {
			statusParts = append(statusParts, divergenceStr)
		}
	}

	// Add work tree status
	if health.WorkTreeStatus == reposync.WorkTreeDirty {
		dirtyDetails := []string{}
		if health.ModifiedFiles > 0 {
			dirtyDetails = append(dirtyDetails, fmt.Sprintf("%d modified", health.ModifiedFiles))
		}
		if health.UntrackedFiles > 0 {
			dirtyDetails = append(dirtyDetails, fmt.Sprintf("%d untracked", health.UntrackedFiles))
		}
		if len(dirtyDetails) > 0 {
			statusParts = append(statusParts, fmt.Sprintf("[%s]", dirtyDetails[0]))
		}
	}

	// Print status line
	fmt.Printf("  %s\n", statusParts[0]+" "+statusParts[1])
	for i := 2; i < len(statusParts); i++ {
		fmt.Printf("     %s\n", statusParts[i])
	}

	// Print recommendation
	if health.Recommendation != "" {
		fmt.Printf("     → %s\n", health.Recommendation)
	}

	// Print error if present and verbose
	if health.Error != nil && verbose {
		fmt.Printf("     ⚠  Error: %v\n", health.Error)
	}

	// Print timing if verbose
	if verbose {
		fmt.Printf(
			"     ⏱  Check: %s, Fetch: %s\n",
			health.Duration.Round(10*time.Millisecond),
			health.FetchDuration.Round(10*time.Millisecond),
		)
	}

	fmt.Println()
}

func getHealthIcon(status reposync.HealthStatus) string {
	switch status {
	case reposync.HealthHealthy:
		return "✓"
	case reposync.HealthWarning:
		return "⚠"
	case reposync.HealthError:
		return "✗"
	case reposync.HealthUnreachable:
		return "⊘"
	default:
		return "?"
	}
}
