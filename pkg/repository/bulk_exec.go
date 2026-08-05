// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/sync/errgroup"
)

// Status constants for bulk exec.
const (
	StatusExecOK     = "success"
	StatusExecFailed = "error"
	StatusWouldExec  = "would-exec"
)

// BulkExecOptions configures bulk arbitrary-command execution across repositories.
type BulkExecOptions struct {
	Directory         string
	Parallel          int
	MaxDepth          int
	DryRun            bool
	IncludeSubmodules bool
	IncludePattern    string
	ExcludePattern    string
	Logger            Logger
	ProgressCallback  func(current, total int, repo string)

	// Command is argv[0]; Args are remaining argv elements. Never passed through a shell.
	Command string
	Args    []string

	// Env extra environment variables (merged with process env). Values for
	// GZ_REPO_NAME / GZ_REPO_PATH are set per-repo automatically.
	// Timeout is the per-repository command deadline (0 = no limit).
	Timeout time.Duration

	// FailFast cancels remaining work after the first non-zero exit.
	FailFast bool

	// OutputTailMax is max bytes of combined stdout/stderr retained per repo (default 4KiB).
	OutputTailMax int
}

// BulkExecResult aggregates bulk exec results.
type BulkExecResult struct {
	TotalScanned   int
	TotalProcessed int
	Repositories   []RepositoryExecResult
	Duration       time.Duration
	Summary        map[string]int
	Command        string
	Args           []string
}

// RepositoryExecResult is the per-repository exec outcome.
type RepositoryExecResult struct {
	Path         string
	RelativePath string
	Status       string
	Message      string
	Error        error
	Duration     time.Duration
	ExitCode     int
	Output       string
}

// GetStatus returns the status for summary calculation.
func (r RepositoryExecResult) GetStatus() string { return r.Status }

// BulkExec scans for git repositories and runs an arbitrary command in each
// working tree in parallel. The command is executed via exec.CommandContext
// (no shell) for security.
func (c *client) BulkExec(ctx context.Context, opts BulkExecOptions) (*BulkExecResult, error) {
	startTime := time.Now()

	if opts.Command == "" {
		return nil, fmt.Errorf("command is required")
	}

	common, err := initializeBulkOperation(
		opts.Directory,
		opts.Parallel,
		opts.MaxDepth,
		opts.IncludeSubmodules,
		opts.IncludePattern,
		opts.ExcludePattern,
		opts.Logger,
	)
	if err != nil {
		return nil, err
	}
	opts.Directory = common.Directory
	opts.Parallel = common.Parallel
	opts.MaxDepth = common.MaxDepth
	opts.Logger = common.Logger
	if opts.OutputTailMax <= 0 {
		opts.OutputTailMax = 4096
	}

	filteredRepos, totalScanned, err := c.scanAndFilterRepositories(ctx, common)
	if err != nil {
		return nil, err
	}

	if len(filteredRepos) == 0 {
		return &BulkExecResult{
			TotalScanned:   totalScanned,
			TotalProcessed: 0,
			Repositories:   []RepositoryExecResult{},
			Duration:       time.Since(startTime),
			Summary:        map[string]int{},
			Command:        opts.Command,
			Args:           opts.Args,
		}, nil
	}

	results, err := c.processExecRepositories(ctx, opts.Directory, filteredRepos, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to process repositories: %w", err)
	}

	return &BulkExecResult{
		TotalScanned:   totalScanned,
		TotalProcessed: len(filteredRepos),
		Repositories:   results,
		Duration:       time.Since(startTime),
		Summary:        calculateSummaryGeneric(results),
		Command:        opts.Command,
		Args:           opts.Args,
	}, nil
}

func (c *client) processExecRepositories(ctx context.Context, rootDir string, repos []string, opts BulkExecOptions) ([]RepositoryExecResult, error) {
	results := make([]RepositoryExecResult, len(repos))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(opts.Parallel)

	for i, repoPath := range repos {
		i, repoPath := i, repoPath
		g.Go(func() error {
			if opts.ProgressCallback != nil {
				opts.ProgressCallback(i+1, len(repos), repoPath)
			}
			result := c.processExecRepository(gctx, rootDir, repoPath, opts)
			results[i] = result
			if opts.FailFast && result.Status == StatusExecFailed {
				return fmt.Errorf("fail-fast: %s: %s", result.RelativePath, result.Message)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		if opts.FailFast {
			return results, nil
		}
		return nil, err
	}
	return results, nil
}

func (c *client) processExecRepository(ctx context.Context, rootDir, repoPath string, opts BulkExecOptions) RepositoryExecResult {
	start := time.Now()
	rel := getRelativePath(rootDir, repoPath)
	result := RepositoryExecResult{
		Path:         repoPath,
		RelativePath: rel,
	}

	if opts.DryRun {
		result.Status = StatusWouldExec
		result.Message = fmt.Sprintf("would run: %s", formatArgv(opts.Command, opts.Args))
		result.Duration = time.Since(start)
		return result
	}

	runCtx := ctx
	var cancel context.CancelFunc
	if opts.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, opts.Command, opts.Args...) //nolint:gosec // intentional user-supplied argv, no shell
	cmd.Dir = repoPath
	cmd.Env = append(
		os.Environ(),
		"GZ_REPO_NAME="+filepath.Base(repoPath),
		"GZ_REPO_PATH="+repoPath,
	)

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	result.Duration = time.Since(start)
	result.Output = tailBytes(buf.String(), opts.OutputTailMax)

	if err != nil {
		exitCode := 1
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
		if runCtx.Err() == context.DeadlineExceeded {
			result.Status = StatusExecFailed
			result.Message = fmt.Sprintf("timeout after %s", opts.Timeout)
			result.ExitCode = -1
			result.Error = runCtx.Err()
			return result
		}
		if runCtx.Err() == context.Canceled {
			result.Status = StatusExecFailed
			result.Message = "canceled"
			result.ExitCode = -1
			result.Error = runCtx.Err()
			return result
		}
		result.Status = StatusExecFailed
		result.ExitCode = exitCode
		result.Error = err
		if result.Output != "" {
			result.Message = fmt.Sprintf("exit %d: %s", exitCode, compactOneLine(result.Output))
		} else {
			result.Message = fmt.Sprintf("exit %d", exitCode)
		}
		return result
	}

	result.Status = StatusExecOK
	result.ExitCode = 0
	result.Message = "ok"
	return result
}

func formatArgv(command string, args []string) string {
	if len(args) == 0 {
		return command
	}
	out := command
	for _, a := range args {
		out += " " + a
	}
	return out
}

func tailBytes(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}

func compactOneLine(s string) string {
	// collapse newlines for summary line
	b := make([]byte, 0, len(s))
	prevSpace := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\n' || c == '\r' || c == '\t' {
			if !prevSpace {
				b = append(b, ' ')
				prevSpace = true
			}
			continue
		}
		b = append(b, c)
		prevSpace = false
	}
	out := string(b)
	if len(out) > 120 {
		return out[:117] + "..."
	}
	return out
}
