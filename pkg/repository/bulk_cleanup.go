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

// BulkCleanupOptions configures bulk branch cleanup operations.
type BulkCleanupOptions struct {
	// Directory is the root directory to scan for repositories
	Directory string

	// Parallel is the number of concurrent workers (default: 10)
	Parallel int

	// MaxDepth is the maximum directory depth to scan (default: 10)
	MaxDepth int

	// DryRun performs simulation without actual changes
	DryRun bool

	// Verbose enables detailed logging
	Verbose bool

	// IncludeMerged includes fully merged branches
	IncludeMerged bool

	// IncludeStale includes stale branches (no recent activity)
	IncludeStale bool

	// IncludeGone includes gone branches (remote deleted)
	IncludeGone bool

	// IncludeSuperseded includes unmerged bot remotes whose version target
	// is already satisfied on the base. Only bot names are considered.
	IncludeSuperseded bool

	// StaleThreshold is the threshold for stale branches (default: 30 days)
	StaleThreshold time.Duration

	// BaseBranch is the base branch for merge detection (default: auto-detect)
	BaseBranch string

	// DeleteRemote also deletes remote branches
	DeleteRemote bool

	// BotsOnly restricts candidates to Dependabot/Renovate/github-actions
	// prefixes. It is a filter, not a cleanup type.
	BotsOnly bool

	// ProtectPatterns are additional patterns to protect from deletion
	ProtectPatterns []string

	// IncludeSubmodules includes git submodules in the scan (default: false)
	IncludeSubmodules bool

	// IncludePattern is a regex pattern for repositories to include
	IncludePattern string

	// ExcludePattern is a regex pattern for repositories to exclude
	ExcludePattern string

	// Logger for operation feedback
	Logger Logger

	// ProgressCallback is called for each processed repository
	ProgressCallback func(current, total int, repo string)
}

// BulkCleanupResult contains the results of a bulk cleanup operation.
type BulkCleanupResult struct {
	// TotalScanned is the number of repositories found
	TotalScanned int

	// TotalProcessed is the number of repositories processed
	TotalProcessed int

	// Repositories contains individual repository results
	Repositories []RepositoryCleanupResult

	// Duration is the total operation time
	Duration time.Duration

	// Summary contains status counts
	Summary map[string]int

	// TotalBranchesDeleted is the total number of branches deleted across all repos
	TotalBranchesDeleted int

	// TotalBranchesAnalyzed is the total number of branches analyzed
	TotalBranchesAnalyzed int
}

// RepositoryCleanupResult represents the result for a single repository cleanup.
type RepositoryCleanupResult struct {
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

	// MergedCount is the number of merged branches found/deleted
	MergedCount int

	// StaleCount is the number of stale branches found/deleted
	StaleCount int

	// GoneCount is the number of gone branches found/deleted
	GoneCount int

	// SupersededCount is the number of superseded bot remotes found/deleted
	SupersededCount int

	// ProtectedCount is the number of protected branches skipped
	ProtectedCount int

	// TotalAnalyzed is the total number of branches analyzed
	TotalAnalyzed int

	// DeletedBranches is the list of deleted branch names
	DeletedBranches []string

	// Branches is the per-branch account (name, reason, location, kind)
	// used by machine-readable printers. DeletedBranches stays a flat name
	// list for existing human output.
	Branches []CleanupBranchEntry
}

// CleanupBranchEntry is one branch a cleanup run would delete (dry-run) or did.
type CleanupBranchEntry struct {
	Name     string `json:"name"`
	Reason   string `json:"reason"`
	Location string `json:"location"`
	Kind     string `json:"kind,omitempty"`
}

// GetStatus returns the status for summary calculation.
func (r RepositoryCleanupResult) GetStatus() string { return r.Status }

// Status constants for cleanup operations.
const (
	StatusCleanedUp    = "cleaned-up"
	StatusNothingToDo  = "nothing-to-do"
	StatusWouldCleanup = "would-cleanup"
)

// BulkCleanup scans for repositories and performs branch cleanup in parallel.
func (c *client) BulkCleanup(ctx context.Context, opts BulkCleanupOptions) (*BulkCleanupResult, error) {
	startTime := time.Now()

	// Set defaults
	if opts.StaleThreshold == 0 {
		opts.StaleThreshold = 30 * 24 * time.Hour
	}

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
		return &BulkCleanupResult{
			TotalScanned:   totalScanned,
			TotalProcessed: 0,
			Repositories:   []RepositoryCleanupResult{},
			Duration:       time.Since(startTime),
			Summary:        map[string]int{},
		}, nil
	}

	// Process repositories in parallel
	results, err := c.processCleanupRepositories(ctx, opts.Directory, filteredRepos, opts, common.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to process repositories: %w", err)
	}

	// Calculate summary and totals
	summary := calculateCleanupSummary(results)
	totalDeleted := 0
	totalAnalyzed := 0
	for _, r := range results {
		totalDeleted += r.MergedCount + r.StaleCount + r.GoneCount + r.SupersededCount
		totalAnalyzed += r.TotalAnalyzed
	}

	return &BulkCleanupResult{
		TotalScanned:          totalScanned,
		TotalProcessed:        len(filteredRepos),
		Repositories:          results,
		Duration:              time.Since(startTime),
		Summary:               summary,
		TotalBranchesDeleted:  totalDeleted,
		TotalBranchesAnalyzed: totalAnalyzed,
	}, nil
}

// processCleanupRepositories processes repositories in parallel for cleanup.
func (c *client) processCleanupRepositories(ctx context.Context, rootDir string, repos []string, opts BulkCleanupOptions, logger Logger) ([]RepositoryCleanupResult, error) {
	results := make([]RepositoryCleanupResult, len(repos))

	// Create error group with concurrency limit
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(opts.Parallel)

	for i, repoPath := range repos {
		g.Go(func() error {
			// Call progress callback
			if opts.ProgressCallback != nil {
				opts.ProgressCallback(i+1, len(repos), repoPath)
			}

			result := c.processCleanupRepository(gctx, rootDir, repoPath, opts, logger)
			results[i] = result

			return nil // Don't fail entire operation on single repo error
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

// processCleanupRepository processes a single repository cleanup.
func (c *client) processCleanupRepository(ctx context.Context, rootDir, repoPath string, opts BulkCleanupOptions, logger Logger) RepositoryCleanupResult {
	startTime := time.Now()

	result := RepositoryCleanupResult{
		Path:            repoPath,
		RelativePath:    getRelativePath(rootDir, repoPath),
		Duration:        0,
		DeletedBranches: []string{},
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

	// Get repository info for branch name
	info, err := c.GetInfo(ctx, repo)
	if err == nil {
		result.Branch = info.Branch
	}

	// Detect base branch if not specified
	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		baseBranch = c.detectBaseBranch(ctx, repoPath)
	}

	remote := defaultRemoteName
	if info != nil && info.Remote != "" {
		remote = info.Remote
	}

	toDelete := c.collectCleanupCandidates(ctx, repoPath, baseBranch, remote, result.Branch, opts, &result)

	// Display count only: exit≠0 → leave TotalAnalyzed zero. Not a delete guard.
	//nolint:errcheck // intentional: empty list is a valid display answer
	allBranchesResult, _ := c.executor.Run(ctx, repoPath, "branch", "--list")
	if allBranchesResult.ExitCode == 0 {
		lines := strings.Split(strings.TrimSpace(allBranchesResult.Stdout), "\n")
		result.TotalAnalyzed = len(lines)
	}

	if len(toDelete) == 0 {
		result.Status = StatusNothingToDo
		result.Message = "No branches to clean up"
		result.Duration = time.Since(startTime)
		return result
	}

	if opts.DryRun {
		result.Status = StatusWouldCleanup
		result.Message = fmt.Sprintf("Would delete %d branch(es)", len(toDelete))
		recordCleanupBranches(&result, toDelete)
		result.Duration = time.Since(startTime)
		return result
	}

	deleted := c.executeCleanupDeletes(ctx, repoPath, remote, toDelete, logger, result.RelativePath)
	recordCleanupBranches(&result, deleted)

	result.Status = StatusCleanedUp
	result.Message = fmt.Sprintf("Deleted %d branch(es)", len(deleted))
	result.Duration = time.Since(startTime)

	logger.Info("repository cleaned up",
		"path", result.RelativePath,
		"merged", result.MergedCount,
		"stale", result.StaleCount,
		"gone", result.GoneCount,
		"superseded", result.SupersededCount)

	return result
}

// detectBaseBranch detects the main/master branch.
func (c *client) detectBaseBranch(ctx context.Context, repoPath string) string {
	candidates := []string{"main", "master", "develop", "development"}
	for _, branch := range candidates {
		// Existence probe: exit≠0 means this candidate name is not a branch.
		//nolint:errcheck // intentional: exit≠0 means branch missing
		result, _ := c.executor.Run(ctx, repoPath, "rev-parse", "--verify", branch)
		if result.ExitCode == 0 {
			return branch
		}
	}
	return "main" // Default fallback
}

// getMergedBranches returns branches merged into base.
func (c *client) getMergedBranches(ctx context.Context, repoPath, baseBranch string) ([]string, error) {
	result, err := c.executor.Run(ctx, repoPath, "branch", "--merged", baseBranch)
	if err != nil {
		return nil, err
	}

	var branches []string
	lines := strings.SplitSeq(strings.TrimSpace(result.Stdout), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "* ")
		if line != "" && line != baseBranch {
			branches = append(branches, line)
		}
	}
	return branches, nil
}

// getStaleBranches returns branches with no recent activity.
func (c *client) getStaleBranches(ctx context.Context, repoPath string, threshold time.Duration) ([]string, error) {
	// Get all branches with last commit date
	result, err := c.executor.Run(ctx, repoPath, "for-each-ref", "--format=%(refname:short) %(committerdate:unix)", "refs/heads/")
	if err != nil {
		return nil, err
	}

	var stale []string
	lines := strings.SplitSeq(strings.TrimSpace(result.Stdout), "\n")
	for line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			branchName := parts[0]
			var timestamp int64
			if _, err := fmt.Sscanf(parts[1], "%d", &timestamp); err == nil {
				lastCommit := time.Unix(timestamp, 0)
				if time.Since(lastCommit) > threshold {
					stale = append(stale, branchName)
				}
			}
		}
	}
	return stale, nil
}

// getGoneBranches returns tracking branches whose remote branch was deleted.
func (c *client) getGoneBranches(ctx context.Context, repoPath string) ([]string, error) {
	// First prune remote tracking branches (non-interactive to prevent credential prompts)
	_, _ = c.executor.RunWithEnv(ctx, repoPath, nonInteractiveEnv, "fetch", "--prune") //nolint:errcheck // best-effort prune; subsequent for-each-ref handles missing tracking info gracefully

	// Find branches with gone upstream
	result, err := c.executor.Run(ctx, repoPath, "for-each-ref", "--format=%(refname:short) %(upstream:track)", "refs/heads/")
	if err != nil {
		return nil, err
	}

	var gone []string
	lines := strings.SplitSeq(strings.TrimSpace(result.Stdout), "\n")
	for line := range lines {
		if strings.Contains(line, "[gone]") {
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				gone = append(gone, parts[0])
			}
		}
	}
	return gone, nil
}

// isProtectedBranch checks if a branch should not be deleted. Built-in
// protection is resolved through the shared IsProtected source; the current
// branch and any caller-supplied patterns are layered on top.
func (c *client) isProtectedBranch(branchName, currentBranch string, additionalPatterns []string) bool {
	// Never delete current branch
	if branchName == currentBranch {
		return true
	}

	// Built-in protected set (single source of truth)
	if IsProtected(branchName) {
		return true
	}

	// Caller-supplied extra patterns
	for _, pattern := range additionalPatterns {
		if matchBranchPattern(branchName, pattern) {
			return true
		}
	}

	return false
}

// calculateCleanupSummary creates a summary of cleanup results by status.
func calculateCleanupSummary(results []RepositoryCleanupResult) map[string]int {
	return calculateSummaryGeneric(results)
}
