// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

var (
	execFlags    BulkCommandFlags
	execFailFast bool
	execTimeout  time.Duration
)

// execCmd runs an arbitrary command in each discovered git repository.
var execCmd = &cobra.Command{
	Use:   "exec [directory] -- <command> [args...]",
	Short: "Run a command in each repository in parallel",
	Long: cliutil.QuickStartHelp(`  # Run git gc in every repo under the current directory
  gz-git exec -- git gc

  # Target a tree with depth and filters
  gz-git exec ~/mydevbox -d 2 --include "gzh-.*" -- go mod tidy

  # Dry-run (show what would run)
  gz-git exec -n -- make test

  # Fail fast on first non-zero exit; per-repo timeout
  gz-git exec --fail-fast --timeout 30s -- ./scripts/check.sh

Environment injected per repository:
  GZ_REPO_NAME  basename of the repository path
  GZ_REPO_PATH  absolute path to the repository

Security: commands are executed without a shell (no pipes, globs, or variable
expansion). Wrap complex logic in a script and pass the script path.`) + cliutil.ExitCodesBulkHelp(),
	Args: cobra.ArbitraryArgs,
	RunE: runExec,
}

func init() {
	rootCmd.AddCommand(execCmd)

	addBulkFlagsWithOpts(execCmd, &execFlags, BulkFlagOptions{
		SkipWatch: true,
		SkipFetch: true,
	})
	execCmd.Flags().BoolVar(&execFailFast, "fail-fast", false, "stop scheduling remaining repos after the first failure")
	execCmd.Flags().DurationVar(&execTimeout, "timeout", 0, "per-repository command timeout (0 = none)")
}

func runExec(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	dash := cmd.ArgsLenAtDash()
	if dash < 0 {
		return fmt.Errorf("missing '--' separator: use 'gz-git exec [directory] -- <command> [args...]'")
	}
	dirArgs := args[:dash]
	cmdArgs := args[dash:]
	if len(cmdArgs) == 0 {
		return fmt.Errorf("command is required after '--'")
	}

	directory, err := validateBulkDirectory(dirArgs)
	if err != nil {
		return err
	}
	if err := resolveBulkDepth(cmd, directory, &execFlags.Depth); err != nil {
		return err
	}
	if err := validateBulkFormat(execFlags.Format); err != nil {
		return err
	}

	effective, _ := LoadEffectiveConfig(cmd, nil)
	if effective != nil {
		if !cmd.Flags().Changed("parallel") && effective.Parallel > 0 {
			execFlags.Parallel = effective.Parallel
		}
		if verbose {
			PrintConfigSources(cmd, effective)
		}
	}

	client := repository.NewClient()
	logger := createBulkLogger(verbose)

	opts := repository.BulkExecOptions{
		Directory:         directory,
		Parallel:          execFlags.Parallel,
		MaxDepth:          execFlags.Depth,
		DryRun:            execFlags.DryRun,
		IncludeSubmodules: execFlags.IncludeSubmodules,
		IncludePattern:    execFlags.Include,
		ExcludePattern:    resolveScanExclude(directory, execFlags.Exclude),
		Logger:            logger,
		Command:           cmdArgs[0],
		Args:              cmdArgs[1:],
		Timeout:           execTimeout,
		FailFast:          execFailFast,
		ProgressCallback:  createProgressCallback("Exec", execFlags.Format, quiet),
	}

	if shouldShowProgress(execFlags.Format, quiet) {
		printScanningMessage(directory, execFlags.Depth, execFlags.Parallel, execFlags.DryRun)
	}

	result, err := client.BulkExec(ctx, opts)
	if err != nil {
		return fmt.Errorf("exec failed: %w", err)
	}

	if shouldShowProgress(execFlags.Format, quiet) && result.TotalScanned == 0 {
		fmt.Printf("No repositories found in %s (depth: %d)\n", directory, execFlags.Depth)
		return nil
	}

	if execFlags.Format == "json" || execFlags.Format == "llm" || !quiet {
		displayExecResults(result)
	}

	failed := result.Summary[repository.StatusExecFailed]
	return errPartialFailure(failed, result.TotalProcessed)
}

func displayExecResults(result *repository.BulkExecResult) {
	if execFlags.Format == "json" || execFlags.Format == "llm" {
		displayExecResultsStructured(result, execFlags.Format)
		return
	}

	fmt.Println()
	for _, repo := range result.Repositories {
		icon := getBulkStatusIcon(repo.Status, 1)
		if repo.Status == repository.StatusWouldExec {
			icon = "→"
		}
		line := fmt.Sprintf("%s %-40s", icon, repo.RelativePath)
		switch repo.Status {
		case repository.StatusExecOK:
			line += fmt.Sprintf(" (%s)", repo.Duration.Round(time.Millisecond))
			if verbose && repo.Output != "" {
				line += "\n    " + strings.ReplaceAll(strings.TrimSpace(repo.Output), "\n", "\n    ")
			}
		case repository.StatusWouldExec:
			line += " " + repo.Message
		default:
			line += " " + repo.Message
		}
		fmt.Println(line)
	}

	// Re-print failures at the end for quick scanning
	var failures []repository.RepositoryExecResult
	for _, repo := range result.Repositories {
		if repo.Status == repository.StatusExecFailed {
			failures = append(failures, repo)
		}
	}
	if len(failures) > 0 && execFlags.Format != "compact" {
		fmt.Println()
		fmt.Println("Failed repositories:")
		for _, repo := range failures {
			fmt.Printf("  ✗ %s  %s\n", repo.RelativePath, repo.Message)
		}
	}

	ok := result.Summary[repository.StatusExecOK]
	would := result.Summary[repository.StatusWouldExec]
	failed := result.Summary[repository.StatusExecFailed]
	fmt.Println()
	parts := []string{fmt.Sprintf("%d repos", result.TotalProcessed)}
	if ok > 0 {
		parts = append(parts, fmt.Sprintf("%d ok", ok))
	}
	if would > 0 {
		parts = append(parts, fmt.Sprintf("%d would-exec", would))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	fmt.Printf("Summary: %s (%s)\n", strings.Join(parts, ", "), result.Duration.Round(time.Millisecond))
}

func displayExecResultsStructured(result *repository.BulkExecResult, format string) {
	type repoJSON struct {
		Path     string `json:"path"`
		Status   string `json:"status"`
		ExitCode int    `json:"exit_code"`
		Message  string `json:"message,omitempty"`
		Output   string `json:"output,omitempty"`
		Duration int64  `json:"duration_ms"`
	}
	out := struct {
		Command        string         `json:"command"`
		Args           []string       `json:"args"`
		TotalScanned   int            `json:"total_scanned"`
		TotalProcessed int            `json:"total_processed"`
		DurationMs     int64          `json:"duration_ms"`
		Summary        map[string]int `json:"summary"`
		Repositories   []repoJSON     `json:"repositories"`
	}{
		Command:        result.Command,
		Args:           result.Args,
		TotalScanned:   result.TotalScanned,
		TotalProcessed: result.TotalProcessed,
		DurationMs:     result.Duration.Milliseconds(),
		Summary:        result.Summary,
		Repositories:   make([]repoJSON, 0, len(result.Repositories)),
	}
	for _, r := range result.Repositories {
		out.Repositories = append(out.Repositories, repoJSON{
			Path:     r.RelativePath,
			Status:   r.Status,
			ExitCode: r.ExitCode,
			Message:  r.Message,
			Output:   r.Output,
			Duration: r.Duration.Milliseconds(),
		})
	}
	writeBulkOutput(format, out)
}
