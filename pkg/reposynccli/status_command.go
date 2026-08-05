// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package reposynccli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/reposync"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/tui"
)

// StatusOptions holds options for the status command.
type StatusOptions struct {
	ConfigFile  string
	Path        string
	ScanDepth   int
	SkipFetch   bool
	Timeout     time.Duration
	Parallel    int
	Verbose     bool
	Format      string // default, json, compact, tui
	UseTUI      bool   // Use interactive TUI mode
	SaveHistory bool   // Save snapshot to history
	HistoryDir  string // History directory (default: ~/.gz-git/history)
}

// newStatusCmd creates the 'sync status' command.
func (f CommandFactory) newStatusCmd() *cobra.Command {
	opts := &StatusOptions{
		Timeout:   30 * time.Second,
		Parallel:  4,
		ScanDepth: 1,
	}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check repository health and sync status",
		Long: cliutil.QuickStartHelp(`  # Check all repositories from config
  gz-git forge status -c sync.yaml

  # Check repositories in a directory
  gz-git forge status --path ~/repos --scan-depth 2

  # Quick check (skip remote fetch)
  gz-git forge status -c sync.yaml --skip-fetch

  # Check with custom timeout
  gz-git forge status -c sync.yaml --timeout 60s

  # Detailed output
  gz-git forge status -c sync.yaml --verbose

  # Auto-detect config in current directory
  gz-git forge status`),
		RunE: func(cmd *cobra.Command, args []string) error {
			return f.runStatus(cmd, opts)
		},
	}

	// Config or path (mutually exclusive in practice)
	cmd.Flags().StringVarP(&opts.ConfigFile, "config", "c", "", "Sync config file")
	cmd.Flags().StringVar(&opts.Path, "path", "", "Directory to scan")
	cmd.Flags().IntVarP(&opts.ScanDepth, "scan-depth", "d", opts.ScanDepth, "Directory scan depth (when using --path)")

	// Diagnostic options
	cmd.Flags().BoolVar(&opts.SkipFetch, "skip-fetch", false, "Skip remote fetch (faster but may show stale data)")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", opts.Timeout, "Timeout for remote fetch per repository")
	cmd.Flags().IntVarP(&opts.Parallel, "parallel", "j", opts.Parallel, "Number of parallel health checks")

	// Output options
	cmd.Flags().BoolVarP(&opts.Verbose, "verbose", "v", false, "Verbose output (show detailed diagnostics)")
	cmd.Flags().StringVar(&opts.Format, "format", "default", "Output format: default, json, compact, llm")
	cmd.Flags().BoolVar(&opts.UseTUI, "tui", false, "Interactive TUI mode")

	// History options
	cmd.Flags().BoolVar(&opts.SaveHistory, "save-history", false, "Save health snapshot to history")
	cmd.Flags().StringVar(&opts.HistoryDir, "history-dir", "", "History directory (default: ~/.gz-git/history)")

	return cmd
}

// RunStatus executes the status checks.
func RunStatus(cmd *cobra.Command, opts *StatusOptions, loader SpecLoader) error { //nolint:gocognit // status command orchestrates multiple code paths; complexity is inherent to its scope
	ctx := cmd.Context()

	// Load repositories from config or scan directory
	var repos []reposync.RepoSpec

	// Auto-detect config if neither --config nor --path specified
	if opts.ConfigFile == "" && opts.Path == "" {
		detected, detectErr := detectConfigFile(".")
		if detectErr == nil {
			opts.ConfigFile = detected
			fmt.Fprintf(cmd.OutOrStdout(), "Using config: %s\n", detected)
		} else {
			// Fall back to scanning current directory
			opts.Path = "."
		}
	}

	if opts.ConfigFile != "" {
		// Load from config file
		configData, loadErr := loader.Load(ctx, opts.ConfigFile)
		if loadErr != nil {
			return fmt.Errorf("failed to load config: %w", loadErr)
		}

		// Extract repos from plan
		repos = configData.Plan.Input.Repos
	} else if opts.Path != "" {
		// Scan directory
		planner := reposync.FSPlanner{}
		plan, planErr := planner.Plan(ctx, reposync.PlanRequest{
			Options: reposync.PlanOptions{
				Roots:           []string{opts.Path},
				DefaultStrategy: reposync.StrategyFetch, // Doesn't matter for status
			},
		})
		if planErr != nil {
			return fmt.Errorf("failed to scan directory: %w", planErr)
		}

		// Extract repo descriptors from plan
		for _, action := range plan.Actions {
			repos = append(repos, action.Repo)
		}
	}

	if len(repos) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No repositories found")
		return nil
	}

	// Prepare diagnostic options
	diagOpts := reposync.DiagnosticOptions{
		SkipFetch:              opts.SkipFetch,
		FetchTimeout:           opts.Timeout,
		Parallel:               opts.Parallel,
		CheckWorkTree:          true,
		IncludeRecommendations: true,
	}

	// Add progress indicator for non-JSON formats
	if opts.Format != "json" && opts.Format != "compact" {
		progress := NewStatusProgressIndicator(cmd.OutOrStdout(), len(repos), false)
		diagOpts.Progress = &DiagnosticProgressAdapter{indicator: progress}
		defer progress.Done()
	}

	// Execute health check
	executor := reposync.DiagnosticExecutor{}
	report, err := executor.CheckHealth(ctx, repos, diagOpts)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	// Save to history if requested
	if opts.SaveHistory {
		historyDir := opts.HistoryDir
		if historyDir == "" {
			home, err := os.UserHomeDir()
			if err == nil {
				historyDir = filepath.Join(home, ".gz-git", "history")
			}
		}

		if historyDir != "" {
			store := reposync.NewFileHistoryStore(historyDir)
			if err := store.Save(ctx, report); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to save history: %v\n", err)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "✓ Snapshot saved to %s\n", historyDir)
			}
		}
	}

	// Display results
	if opts.UseTUI {
		return runTUI(report)
	}

	switch opts.Format {
	case "json":
		return printHealthReportJSON(cmd, report, opts.Verbose)
	case "compact":
		printHealthReportCompact(cmd, report)
	default:
		printHealthReport(cmd, report, opts.Verbose)
	}

	return nil
}

func (f CommandFactory) runStatus(cmd *cobra.Command, opts *StatusOptions) error {
	return RunStatus(cmd, opts, f.SpecLoader)
}

func printHealthReport(cmd *cobra.Command, report *reposync.HealthReport, verbose bool) {
	out := cmd.OutOrStdout()

	fmt.Fprintf(out, "\nChecking repository health...\n\n")

	// Print per-repo status
	for _, health := range report.Results {
		icon := getHealthIcon(health.HealthStatus)
		statusStr := getStatusString(health)

		fmt.Fprintf(
			out, "%s %-30s  %s\n",
			icon,
			formatRepoName(health.Repo),
			statusStr,
		)

		// Show recommendation if not healthy
		if health.HealthStatus != reposync.HealthHealthy && health.Recommendation != "" {
			fmt.Fprintf(out, "  → %s\n", health.Recommendation)
		}

		// Verbose mode: show detailed diagnostics
		if verbose {
			printVerboseHealth(out, health)
		}

		if health.Error != nil {
			fmt.Fprintf(out, "  ⚠  Error: %v\n", health.Error)
		}
	}

	// Print summary
	fmt.Fprintf(
		out, "\nSummary: %d healthy, %d warnings, %d errors, %d unreachable (%d total)\n",
		report.Summary.Healthy,
		report.Summary.Warning,
		report.Summary.Error,
		report.Summary.Unreachable,
		report.Summary.Total,
	)

	fmt.Fprintf(out, "Total time: %v\n", report.TotalDuration.Round(time.Millisecond))
}

func printVerboseHealth(out interface{ Write([]byte) (int, error) }, health reposync.RepoHealth) {
	fmt.Fprintf(out, "     Branch: %s", health.CurrentBranch)
	if health.UpstreamBranch != "" {
		fmt.Fprintf(out, " → %s", health.UpstreamBranch)
	}
	fmt.Fprintln(out)

	if health.AheadBy > 0 || health.BehindBy > 0 {
		fmt.Fprintf(out, "     Divergence: %d↑ ahead, %d↓ behind\n", health.AheadBy, health.BehindBy)
	}

	if health.ModifiedFiles > 0 || health.UntrackedFiles > 0 || health.ConflictFiles > 0 {
		fmt.Fprintf(out, "     Working tree: %d modified, %d untracked, %d conflicts\n",
			health.ModifiedFiles, health.UntrackedFiles, health.ConflictFiles)
	}

	if health.FetchDuration > 0 {
		fmt.Fprintf(out, "     Fetch: %v (%s)\n",
			health.FetchDuration.Round(time.Millisecond),
			health.NetworkStatus)
	}
}

func getHealthIcon(status reposync.HealthStatus) string {
	return tui.FormatHealthIcon(status)
}

func getStatusString(health reposync.RepoHealth) string {
	return tui.FormatStatusText(health)
}

func formatRepoName(repo reposync.RepoSpec) string {
	return tui.FormatRepoName(repo)
}

// printHealthReportJSON outputs the health report in JSON format.
func printHealthReportJSON(cmd *cobra.Command, report *reposync.HealthReport, verbose bool) error {
	out := cmd.OutOrStdout()

	// Create JSON-friendly structure
	type JSONRepoHealth struct {
		Name            string  `json:"name"`
		Path            string  `json:"path"`
		HealthStatus    string  `json:"health_status"`
		NetworkStatus   string  `json:"network_status"`
		DivergenceType  string  `json:"divergence_type"`
		WorkTreeStatus  string  `json:"worktree_status"` //nolint:tagliatelle // existing JSON API field name
		CurrentBranch   string  `json:"current_branch"`
		UpstreamBranch  string  `json:"upstream_branch,omitempty"`
		AheadBy         int     `json:"ahead_by"`
		BehindBy        int     `json:"behind_by"`
		ModifiedFiles   int     `json:"modified_files"`
		UntrackedFiles  int     `json:"untracked_files"`
		ConflictFiles   int     `json:"conflict_files"`
		Recommendation  string  `json:"recommendation,omitempty"`
		Error           string  `json:"error,omitempty"`
		DurationMs      float64 `json:"duration_ms"`
		FetchDurationMs float64 `json:"fetch_duration_ms"`
	}

	type JSONHealthReport struct {
		Results   []JSONRepoHealth       `json:"results"`
		Summary   reposync.HealthSummary `json:"summary"`
		TotalMs   float64                `json:"total_duration_ms"` //nolint:tagliatelle // existing JSON API field name
		CheckedAt string                 `json:"checked_at"`
	}

	jsonResults := make([]JSONRepoHealth, len(report.Results))
	for i, health := range report.Results {
		errStr := ""
		if health.Error != nil {
			errStr = health.Error.Error()
		}

		jsonResults[i] = JSONRepoHealth{
			Name:            formatRepoName(health.Repo),
			Path:            health.Repo.TargetPath,
			HealthStatus:    string(health.HealthStatus),
			NetworkStatus:   string(health.NetworkStatus),
			DivergenceType:  string(health.DivergenceType),
			WorkTreeStatus:  string(health.WorkTreeStatus),
			CurrentBranch:   health.CurrentBranch,
			UpstreamBranch:  health.UpstreamBranch,
			AheadBy:         health.AheadBy,
			BehindBy:        health.BehindBy,
			ModifiedFiles:   health.ModifiedFiles,
			UntrackedFiles:  health.UntrackedFiles,
			ConflictFiles:   health.ConflictFiles,
			Recommendation:  health.Recommendation,
			Error:           errStr,
			DurationMs:      float64(health.Duration.Microseconds()) / 1000.0,
			FetchDurationMs: float64(health.FetchDuration.Microseconds()) / 1000.0,
		}
	}

	jsonReport := JSONHealthReport{
		Results:   jsonResults,
		Summary:   report.Summary,
		TotalMs:   float64(report.TotalDuration.Microseconds()) / 1000.0,
		CheckedAt: report.CheckedAt.Format(time.RFC3339),
	}

	return cliutil.WriteJSON(out, jsonReport, verbose)
}

// printHealthReportCompact outputs a compact one-line-per-repo summary.
func printHealthReportCompact(cmd *cobra.Command, report *reposync.HealthReport) {
	out := cmd.OutOrStdout()

	for _, health := range report.Results {
		icon := getHealthIcon(health.HealthStatus)
		name := formatRepoName(health.Repo)

		// Compact format: icon name (branch) status
		status := ""
		switch {
		case health.AheadBy > 0 && health.BehindBy > 0:
			status = fmt.Sprintf("%d↑%d↓", health.AheadBy, health.BehindBy)
		case health.BehindBy > 0:
			status = fmt.Sprintf("%d↓", health.BehindBy)
		case health.AheadBy > 0:
			status = fmt.Sprintf("%d↑", health.AheadBy)
		default:
			status = "="
		}

		switch health.WorkTreeStatus {
		case reposync.WorkTreeDirty:
			status += " dirty"
		case reposync.WorkTreeConflict:
			status += " CONFLICT"
		case reposync.WorkTreeRebaseInProgress:
			status += " REBASE"
		case reposync.WorkTreeMergeInProgress:
			status += " MERGE"
		case reposync.WorkTreeUnknown:
			status += " UNREADABLE"
		case reposync.WorkTreeClean:
			// clean — no suffix needed
		}

		fmt.Fprintf(out, "%s %s (%s) %s\n", icon, name, health.CurrentBranch, status)
	}

	// Summary line
	fmt.Fprintf(
		out, "\n%d ok, %d warn, %d err, %d unreachable\n",
		report.Summary.Healthy,
		report.Summary.Warning,
		report.Summary.Error,
		report.Summary.Unreachable,
	)
}

// runTUI launches the interactive TUI for repository status.
func runTUI(report *reposync.HealthReport) error {
	// Create TUI model with health results
	model := tui.NewStatusModel(report.Results)

	// Start Bubble Tea program
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// Check if user selected an action
	m, ok := finalModel.(tui.StatusModel)
	if !ok {
		return nil
	}

	action := m.GetAction()
	if action == "" {
		// No action selected, just quit
		return nil
	}

	selectedPaths := m.GetSelectedPaths()
	if len(selectedPaths) == 0 {
		return nil
	}

	// Print selected repositories
	fmt.Printf("\n✓ Selected %d repositories for %s:\n\n",
		len(selectedPaths), action)

	for i, path := range selectedPaths {
		fmt.Printf("  %d. %s\n", i+1, path)
	}

	// Generate command to execute
	fmt.Print("\n" + generateCommandHelp(action, selectedPaths) + "\n")

	return nil
}

// generateCommandHelp generates executable command help text.
func generateCommandHelp(action string, paths []string) string {
	var cmd string
	var description string

	switch action {
	case "sync":
		description = "To sync these repositories, run:"
		cmd = "gz-git workspace sync -c <config-file>"
		// Note: workspace sync doesn't support path filtering yet.
		// Create a filtered config or use pull.

	case "pull":
		description = "To pull these repositories, run:"
		cmd = "gz-git pull"
		// Add each path as argument
		for _, path := range paths {
			cmd += " \\\n    " + path
		}

	case "fetch":
		description = "To fetch these repositories, run:"
		cmd = "gz-git fetch"
		// Add each path as argument
		for _, path := range paths {
			cmd += " \\\n    " + path
		}

	default:
		return ""
	}

	return fmt.Sprintf(`%s

  %s

Or manually run git commands in each directory.`, description, cmd)
}
