// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

// BulkStashOptions configures bulk stash operations.
type BulkStashOptions struct {
	// Directory is the root directory to scan for repositories
	Directory string

	// Parallel is the number of concurrent workers (default: 10)
	Parallel int

	// MaxDepth is the maximum directory depth to scan (default: 10)
	MaxDepth int

	// DryRun performs simulation without actual changes
	DryRun bool

	// Operation is the stash operation: "save", "list", "pop", "apply"
	Operation string

	// Message is the stash message (for save operation)
	Message string

	// IncludeUntracked includes untracked files (for save operation)
	IncludeUntracked bool

	// IncludeSubmodules includes git submodules in the scan
	IncludeSubmodules bool

	// IncludePattern is a regex pattern for repositories to include
	IncludePattern string

	// ExcludePattern is a regex pattern for repositories to exclude
	ExcludePattern string

	// OnlyDirty only processes repositories with uncommitted changes (for save)
	OnlyDirty bool

	// OnlyWithStash only processes repositories with existing stashes (for pop/list)
	OnlyWithStash bool

	// Logger for operation feedback
	Logger Logger

	// ProgressCallback is called for each processed repository
	ProgressCallback func(current, total int, repo string)
}

// BulkStashResult contains the results of a bulk stash operation.
type BulkStashResult struct {
	// TotalScanned is the number of repositories found
	TotalScanned int

	// TotalProcessed is the number of repositories processed
	TotalProcessed int

	// Repositories contains individual repository results
	Repositories []RepositoryStashResult

	// Duration is the total operation time
	Duration time.Duration

	// Summary contains status counts
	Summary map[string]int

	// TotalStashCount is the total number of stashes affected
	TotalStashCount int
}

// RepositoryStashResult represents the result for a single repository stash operation.
type RepositoryStashResult struct {
	// Path is the repository path
	Path string

	// RelativePath is the path relative to scan root
	RelativePath string

	// Status is the operation status
	Status string

	// Message is a human-readable status message
	Message string

	// Error if the operation failed
	Error error

	// Duration is how long this repository took to process
	Duration time.Duration

	// Branch is the current branch name
	Branch string

	// StashCount is the number of stashes in the repository
	StashCount int

	// StashMessage is the stash message (for save operation)
	StashMessage string
}

// GetStatus returns the status for summary calculation.
func (r RepositoryStashResult) GetStatus() string { return r.Status }

// Status constants for stash operations.
const (
	StatusStashed     = "stashed"
	StatusPopped      = "popped"
	StatusWouldStash  = "would-stash"
	StatusWouldPop    = "would-pop"
	StatusNoChanges   = "no-changes"
	StatusNoStash     = "no-stash"
	StatusHasStash    = "has-stash"
	StatusListedStash = "listed"
)

// BulkStash scans for repositories and performs stash operations in parallel.
func (c *client) BulkStash(ctx context.Context, opts BulkStashOptions) (*BulkStashResult, error) {
	startTime := time.Now()

	// Initialize common settings
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

	// Update opts with initialized values
	opts.Directory = common.Directory
	opts.Parallel = common.Parallel
	opts.MaxDepth = common.MaxDepth
	opts.Logger = common.Logger

	// Scan and filter repositories
	filteredRepos, totalScanned, err := c.scanAndFilterRepositories(ctx, common)
	if err != nil {
		return nil, err
	}

	// Handle empty result
	if len(filteredRepos) == 0 {
		return &BulkStashResult{
			TotalScanned:   totalScanned,
			TotalProcessed: 0,
			Repositories:   []RepositoryStashResult{},
			Duration:       time.Since(startTime),
			Summary:        map[string]int{},
		}, nil
	}

	// Process repositories in parallel
	results, err := c.processStashRepositories(ctx, opts.Directory, filteredRepos, opts, common.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to process repositories: %w", err)
	}

	// Calculate summary and totals
	summary := calculateStashSummary(results)
	totalStashCount := 0
	for _, r := range results {
		totalStashCount += r.StashCount
	}

	return &BulkStashResult{
		TotalScanned:    totalScanned,
		TotalProcessed:  len(filteredRepos),
		Repositories:    results,
		Duration:        time.Since(startTime),
		Summary:         summary,
		TotalStashCount: totalStashCount,
	}, nil
}

// processStashRepositories processes repositories in parallel for stash operations.
func (c *client) processStashRepositories(ctx context.Context, rootDir string, repos []string, opts BulkStashOptions, logger Logger) ([]RepositoryStashResult, error) {
	results := make([]RepositoryStashResult, len(repos))

	// Create error group with concurrency limit
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(opts.Parallel)

	for i, repoPath := range repos {
		g.Go(func() error {
			// Call progress callback
			if opts.ProgressCallback != nil {
				opts.ProgressCallback(i+1, len(repos), repoPath)
			}

			result := c.processStashRepository(gctx, rootDir, repoPath, opts, logger)
			results[i] = result

			return nil // Don't fail entire operation on single repo error
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

// processStashRepository processes a single repository stash operation.
func (c *client) processStashRepository(ctx context.Context, rootDir, repoPath string, opts BulkStashOptions, logger Logger) RepositoryStashResult {
	startTime := time.Now()

	result := RepositoryStashResult{
		Path:         repoPath,
		RelativePath: getRelativePath(rootDir, repoPath),
		Duration:     0,
	}

	// Open repository
	repo, err := c.Open(ctx, repoPath)
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to open repository"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	// Get repository info
	info, err := c.GetInfo(ctx, repo)
	if err == nil {
		result.Branch = info.Branch
	}

	// Display metadata only: exit≠0 → report zero stashes. Not a mutation guard.
	//nolint:errcheck // intentional: empty list is a valid display answer
	stashCountResult, _ := c.executor.Run(ctx, repoPath, "stash", "list")
	if stashCountResult.ExitCode == 0 {
		lines := strings.Split(strings.TrimSpace(stashCountResult.Stdout), "\n")
		if lines[0] != "" {
			result.StashCount = len(lines)
		}
	}

	switch opts.Operation {
	case "save":
		result = c.processStashSave(ctx, repoPath, opts, result, logger)
	case "pop":
		result = c.processStashPop(ctx, repoPath, opts, result, logger)
	case "apply":
		result = c.processStashApply(ctx, repoPath, opts, result, logger)
	case "list":
		result = c.processStashList(ctx, repoPath, opts, result, logger)
	default:
		result.Status = StatusError
		result.Message = fmt.Sprintf("Unknown operation: %s", opts.Operation)
	}

	result.Duration = time.Since(startTime)
	return result
}

// processStashSave handles stash save operation.
func (c *client) processStashSave(ctx context.Context, repoPath string, opts BulkStashOptions, result RepositoryStashResult, logger Logger) RepositoryStashResult {
	// status exit≠0 means unreadable, not "has changes". The previous ExitCode-only
	// check fell through to stash on a broken index — stash then also failed, with
	// a worse message. Fail closed so the caller sees the status error.
	statusOut, err := c.runGit(ctx, repoPath, "status", "--porcelain")
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to get repository status"
		result.Error = err
		return result
	}
	if strings.TrimSpace(statusOut) == "" {
		result.Status = StatusNoChanges
		result.Message = "No changes to stash"
		return result
	}

	// Dry run - just report
	if opts.DryRun {
		result.Status = StatusWouldStash
		result.Message = "Would stash changes"
		result.StashMessage = opts.Message
		return result
	}

	// Execute stash save
	args := []string{"stash", "push"}
	if opts.Message != "" {
		args = append(args, "-m", opts.Message)
	}
	if opts.IncludeUntracked {
		args = append(args, "--include-untracked")
	}

	stashResult, err := c.executor.Run(ctx, repoPath, args...)
	if err != nil || stashResult.ExitCode != 0 {
		result.Status = StatusError
		result.Message = "Failed to stash"
		if stashResult.Stderr != "" {
			result.Error = fmt.Errorf("%s", stashResult.Stderr)
		}
		return result
	}

	result.Status = StatusStashed
	result.Message = "Changes stashed"
	result.StashMessage = opts.Message
	result.StashCount++

	logger.Info("changes stashed", "path", result.RelativePath, "message", opts.Message)
	return result
}

// processStashPop handles stash pop operation.
func (c *client) processStashPop(ctx context.Context, repoPath string, opts BulkStashOptions, result RepositoryStashResult, logger Logger) RepositoryStashResult {
	// Check if repository has stashes
	if result.StashCount == 0 {
		result.Status = StatusNoStash
		result.Message = "No stash to pop"
		return result
	}

	// Dry run - just report
	if opts.DryRun {
		result.Status = StatusWouldPop
		result.Message = fmt.Sprintf("Would pop stash (%d available)", result.StashCount)
		return result
	}

	// Execute stash pop
	popResult, err := c.executor.Run(ctx, repoPath, "stash", "pop")
	if err != nil || popResult.ExitCode != 0 {
		result.Status = StatusError
		result.Message = "Failed to pop stash"
		if popResult.Stderr != "" {
			result.Error = fmt.Errorf("%s", popResult.Stderr)
		}
		return result
	}

	result.Status = StatusPopped
	result.Message = "Stash popped"
	result.StashCount--

	logger.Info("stash popped", "path", result.RelativePath)
	return result
}

func (c *client) processStashApply(ctx context.Context, repoPath string, opts BulkStashOptions, result RepositoryStashResult, logger Logger) RepositoryStashResult {
	if result.StashCount == 0 {
		result.Status = StatusNoStash
		result.Message = "No stash to apply"
		return result
	}

	if opts.DryRun {
		result.Status = StatusWouldPop
		result.Message = fmt.Sprintf("Would apply stash (%d available)", result.StashCount)
		return result
	}

	applyResult, err := c.executor.Run(ctx, repoPath, "stash", "apply")
	if err != nil || applyResult.ExitCode != 0 {
		result.Status = StatusError
		result.Message = "Failed to apply stash"
		if applyResult.Stderr != "" {
			result.Error = fmt.Errorf("%s", applyResult.Stderr)
		}
		return result
	}

	result.Status = StatusPopped
	result.Message = "Stash applied"
	logger.Info("stash applied", "path", result.RelativePath)
	return result
}

// processStashList handles stash list operation.
func (c *client) processStashList(_ context.Context, _ string, _ BulkStashOptions, result RepositoryStashResult, _ Logger) RepositoryStashResult {
	if result.StashCount == 0 {
		result.Status = StatusNoStash
		result.Message = "No stashes"
		return result
	}

	result.Status = StatusHasStash
	result.Message = fmt.Sprintf("%d stash(es)", result.StashCount)
	return result
}

// calculateStashSummary creates a summary of stash results by status.
func calculateStashSummary(results []RepositoryStashResult) map[string]int {
	return calculateSummaryGeneric(results)
}
