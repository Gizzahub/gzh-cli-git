// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// nonInteractiveEnv contains environment variables to disable git credential prompts.
// This prevents bulk operations from blocking on authentication requests.
// Repositories requiring credentials will fail with an error instead of waiting.
var nonInteractiveEnv = []string{
	"GIT_TERMINAL_PROMPT=0",
}

// authErrorPatterns contains common patterns that indicate authentication failures.
var authErrorPatterns = []string{
	"could not read Username",
	"Authentication failed",
	"terminal prompts disabled",
	"could not read Password",
	"Invalid username or password",
	"remote: HTTP Basic: Access denied",
}

// isAuthenticationError checks if the error output indicates an authentication failure.
// Returns true if the stderr contains any authentication error patterns.
func isAuthenticationError(stderr string) bool {
	for _, pattern := range authErrorPatterns {
		if strings.Contains(stderr, pattern) {
			return true
		}
	}
	return false
}

// ErrAuthRequired is returned when a git operation fails due to missing credentials.
var ErrAuthRequired = fmt.Errorf("authentication required (credential helper not configured)")

// BulkFetchOptions configures bulk repository fetch operations.
type BulkFetchOptions struct {
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

	// AllRemotes fetches from all remotes (default: origin only)
	AllRemotes bool

	// Prune removes remote-tracking branches that no longer exist
	Prune bool

	// Tags fetches all tags from remote
	Tags bool

	// IncludeSubmodules includes git submodules in the scan (default: false)
	// When false, only scans for independent nested repositories
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

// BulkFetchResult contains the results of a bulk fetch operation.
type BulkFetchResult struct {
	// TotalScanned is the number of repositories found
	TotalScanned int

	// TotalProcessed is the number of repositories processed
	TotalProcessed int

	// Repositories contains individual repository results
	Repositories []RepositoryFetchResult

	// Duration is the total operation time
	Duration time.Duration

	// Summary contains status counts
	Summary map[string]int
}

// RepositoryFetchResult represents the result for a single repository fetch.
type RepositoryFetchResult struct {
	// Path is the repository path
	Path string

	// RelativePath is the path relative to scan root
	RelativePath string

	// Status is the operation status (success, skipped, error, etc.)
	Status string

	// Message is a human-readable status message
	Message string

	// Error if the operation failed
	Error error

	// Duration is how long this repository took to process
	Duration time.Duration

	// Branch is the current branch name
	Branch string

	// RemoteURL is the remote origin URL
	RemoteURL string

	// Remote is the remote name (e.g., "origin")
	Remote string

	// FetchedRefs is the number of refs fetched
	FetchedRefs int

	// FetchedObjects is the number of objects fetched
	FetchedObjects int

	// CommitsBehind is the number of commits behind remote after fetch
	CommitsBehind int

	// CommitsAhead is the number of commits ahead of remote after fetch
	CommitsAhead int

	// Deprecated: use TrackedChangedFiles. The name promised "uncommitted files"
	// but the value was len(StagedFiles)+len(ModifiedFiles), which counted a path
	// that is both staged and modified twice and missed working-tree-only
	// deletions entirely. It now carries the TrackedChangedFiles value.
	UncommittedFiles int

	// TrackedChangedFiles is the number of distinct tracked paths with
	// uncommitted changes, staged or not - checked after fetch
	TrackedChangedFiles int

	// StagedFiles is the number of paths whose index differs from HEAD - checked after fetch
	StagedFiles int

	// UnstagedFiles is the number of tracked paths whose working tree differs
	// from the index - checked after fetch
	UnstagedFiles int

	// UntrackedFiles is the number of untracked files - checked after fetch
	UntrackedFiles int
}

// GetStatus returns the status for summary calculation.
func (r RepositoryFetchResult) GetStatus() string { return r.Status }

// BulkPullOptions configures bulk repository pull operations.
type BulkPullOptions struct {
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

	// Strategy defines how to merge changes (merge, rebase, ff-only)
	Strategy string

	// Prune removes remote-tracking branches that no longer exist
	Prune bool

	// Tags fetches all tags from remote
	Tags bool

	// Stash automatically stashes local changes before pull
	Stash bool

	// IncludeSubmodules includes git submodules in the scan (default: false)
	// When false, only scans for independent nested repositories
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

// BulkPullResult contains the results of a bulk pull operation.
type BulkPullResult struct {
	// TotalScanned is the number of repositories found
	TotalScanned int

	// TotalProcessed is the number of repositories processed
	TotalProcessed int

	// Repositories contains individual repository results
	Repositories []RepositoryPullResult

	// Duration is the total operation time
	Duration time.Duration

	// Summary contains status counts
	Summary map[string]int
}

// RepositoryPullResult represents the result for a single repository pull.
type RepositoryPullResult struct {
	// Path is the repository path
	Path string

	// RelativePath is the path relative to scan root
	RelativePath string

	// Status is the operation status (success, skipped, error, etc.)
	Status string

	// Message is a human-readable status message
	Message string

	// Error if the operation failed
	Error error

	// Duration is how long this repository took to process
	Duration time.Duration

	// Branch is the current branch name
	Branch string

	// RemoteURL is the remote origin URL
	RemoteURL string

	// Remote is the remote name (e.g., "origin")
	Remote string

	// CommitsBehind is the number of commits behind remote before pull
	CommitsBehind int

	// CommitsAhead is the number of commits ahead of remote before pull
	CommitsAhead int

	// UpdatedFiles is the number of files changed
	UpdatedFiles int

	// Stashed indicates if local changes were stashed
	Stashed bool

	// Deprecated: use TrackedChangedFiles. See RepositoryFetchResult for why the
	// old value was wrong; it now carries the TrackedChangedFiles value.
	UncommittedFiles int

	// TrackedChangedFiles is the number of distinct tracked paths with
	// uncommitted changes, staged or not - checked after pull
	TrackedChangedFiles int

	// StagedFiles is the number of paths whose index differs from HEAD - checked after pull
	StagedFiles int

	// UnstagedFiles is the number of tracked paths whose working tree differs
	// from the index - checked after pull
	UnstagedFiles int

	// UntrackedFiles is the number of untracked files - checked after pull
	UntrackedFiles int
}

// GetStatus returns the status for summary calculation.
func (r RepositoryPullResult) GetStatus() string { return r.Status }

// BulkPushOptions configures bulk repository push operations.
type BulkPushOptions struct {
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

	// Force forces the push (use with caution)
	Force bool

	// SetUpstream sets upstream for new branches
	SetUpstream bool

	// Tags pushes all tags
	Tags bool

	// Refspec is a custom refspec for branch mapping (e.g., "develop:master")
	Refspec string

	// Remotes is a list of remotes to push to (empty = use origin)
	Remotes []string

	// AllRemotes pushes to all configured remotes
	AllRemotes bool

	// IgnoreDirty skips dirty status check after push (useful for CI/CD)
	IgnoreDirty bool

	// IncludeSubmodules includes git submodules in the scan (default: false)
	// When false, only scans for independent nested repositories
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

// BulkPushResult contains the results of a bulk push operation.
type BulkPushResult struct {
	// TotalScanned is the number of repositories found
	TotalScanned int

	// TotalProcessed is the number of repositories processed
	TotalProcessed int

	// Repositories contains individual repository results
	Repositories []RepositoryPushResult

	// Duration is the total operation time
	Duration time.Duration

	// Summary contains status counts
	Summary map[string]int
}

// RepositoryPushResult represents the result for a single repository push.
type RepositoryPushResult struct {
	// Path is the repository path
	Path string

	// RelativePath is the path relative to scan root
	RelativePath string

	// Status is the operation status (success, skipped, error, etc.)
	Status string

	// Message is a human-readable status message
	Message string

	// Error if the operation failed
	Error error

	// Duration is how long this repository took to process
	Duration time.Duration

	// Branch is the current branch name
	Branch string

	// RemoteURL is the remote origin URL
	RemoteURL string

	// Remote is the remote name (e.g., "origin")
	Remote string

	// CommitsAhead is the number of commits ahead of remote before push
	CommitsAhead int

	// PushedCommits is the number of commits pushed
	PushedCommits int

	// Deprecated: use TrackedChangedFiles. See RepositoryFetchResult for why the
	// old value was wrong; it now carries the TrackedChangedFiles value.
	UncommittedFiles int

	// TrackedChangedFiles is the number of distinct tracked paths with
	// uncommitted changes, staged or not - checked after push
	TrackedChangedFiles int

	// StagedFiles is the number of paths whose index differs from HEAD - checked after push
	StagedFiles int

	// UnstagedFiles is the number of tracked paths whose working tree differs
	// from the index - checked after push
	UnstagedFiles int

	// UntrackedFiles is the number of untracked files - checked after push
	UntrackedFiles int
}

// GetStatus returns the status for summary calculation.
func (r RepositoryPushResult) GetStatus() string { return r.Status }

// BulkStatusOptions configures bulk repository status check operations.
type BulkStatusOptions struct {
	// Directory is the root directory to scan for repositories
	Directory string

	// Parallel is the number of concurrent workers (default: 10)
	Parallel int

	// MaxDepth is the maximum directory depth to scan (default: 10)
	MaxDepth int

	// Verbose enables detailed logging for all repositories (including clean ones)
	Verbose bool

	// IncludeSubmodules includes git submodules in the scan (default: false)
	// When false, only scans for independent nested repositories
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

// BulkStatusResult contains the results of a bulk status operation.
type BulkStatusResult struct {
	// TotalScanned is the number of repositories found
	TotalScanned int

	// TotalProcessed is the number of repositories processed
	TotalProcessed int

	// Repositories contains individual repository results
	Repositories []RepositoryStatusResult

	// Duration is the total operation time
	Duration time.Duration

	// Summary contains status counts
	Summary map[string]int
}

// RepositoryStatusResult represents the status result for a single repository.
type RepositoryStatusResult struct {
	// Path is the repository path
	Path string

	// RelativePath is the path relative to scan root
	RelativePath string

	// Status is the operation status (clean, dirty, conflict, error, etc.)
	Status string

	// Message is a human-readable status message
	Message string

	// Error if the operation failed
	Error error

	// Duration is how long this repository took to process
	Duration time.Duration

	// Branch is the current branch name
	Branch string

	// RemoteURL is the remote origin URL
	RemoteURL string

	// Remote is the remote name (e.g., "origin")
	Remote string

	// Remotes contains all configured remotes (name -> url).
	Remotes map[string]string

	// Metadata
	HeadSHA          string
	Describe         string
	LastCommitMsg    string
	LastCommitDate   string
	LastCommitAuthor string
	LocalBranches    []string
	StashCount       int

	// CommitsBehind is how many commits behind remote
	CommitsBehind int

	// CommitsAhead is how many commits ahead of remote
	CommitsAhead int

	// Deprecated: use TrackedChangedFiles. Unlike the other three results, this
	// one was assigned the raw porcelain line count, so it also included every
	// untracked entry — which UntrackedFiles below then reported a second time.
	// It now carries the TrackedChangedFiles value and untracked paths appear
	// once, in UntrackedFiles.
	UncommittedFiles int

	// TrackedChangedFiles is the number of distinct tracked paths with
	// uncommitted changes, staged or not
	TrackedChangedFiles int

	// StagedFiles is the number of paths whose index differs from HEAD
	StagedFiles int

	// UnstagedFiles is the number of tracked paths whose working tree differs
	// from the index
	UnstagedFiles int

	// UntrackedFiles is the number of untracked files
	UntrackedFiles int

	// ConflictFiles is the list of files with conflicts
	ConflictFiles []string

	// RebaseInProgress indicates if repository is in rebase state
	RebaseInProgress bool

	// MergeInProgress indicates if repository is in merge state
	MergeInProgress bool
}

// GetStatus returns the status for summary calculation.
func (r RepositoryStatusResult) GetStatus() string { return r.Status }

// BulkUpdateOptions configures bulk repository update operations.
type BulkUpdateOptions struct {
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

	// NoFetch skips fetching from remote
	NoFetch bool

	// IncludeSubmodules includes git submodules in the scan (default: false)
	// When false, only scans for independent nested repositories
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

// BulkUpdateResult contains the results of a bulk update operation.
type BulkUpdateResult struct {
	// TotalScanned is the number of repositories found
	TotalScanned int

	// TotalProcessed is the number of repositories processed
	TotalProcessed int

	// Repositories contains individual repository results
	Repositories []RepositoryUpdateResult

	// Duration is the total operation time
	Duration time.Duration

	// Summary contains status counts
	Summary map[string]int
}

// RepositoryUpdateResult represents the result for a single repository.
type RepositoryUpdateResult struct {
	// Path is the repository path
	Path string

	// RelativePath is the path relative to scan root
	RelativePath string

	// Status is the operation status (success, skipped, error, etc.)
	Status string

	// Message is a human-readable status message
	Message string

	// Error if the operation failed
	Error error

	// Duration is how long this repository took to process
	Duration time.Duration

	// Branch is the current branch name
	Branch string

	// RemoteURL is the remote origin URL
	RemoteURL string

	// Remote is the remote name (e.g., "origin")
	Remote string

	// CommitsBehind is how many commits behind remote
	CommitsBehind int

	// CommitsAhead is how many commits ahead of remote
	CommitsAhead int

	// HasStash indicates if there are stashed changes
	HasStash bool

	// InMergeState indicates if repository is in merge state
	InMergeState bool

	// HasUncommittedChanges indicates if there are local changes
	HasUncommittedChanges bool
}

// GetStatus returns the status for summary calculation.
func (r RepositoryUpdateResult) GetStatus() string { return r.Status }

// BulkSwitchOptions configures bulk repository branch switch operations.
type BulkSwitchOptions struct {
	// Directory is the root directory to scan for repositories
	Directory string

	// Branch is the target branch to switch to (required)
	Branch string

	// Parallel is the number of concurrent workers (default: 10)
	Parallel int

	// MaxDepth is the maximum directory depth to scan (default: 1)
	MaxDepth int

	// DryRun performs simulation without actual changes
	DryRun bool

	// Verbose enables detailed logging
	Verbose bool

	// Create creates the branch if it doesn't exist
	Create bool

	// Force forces the switch even with uncommitted changes (dangerous)
	Force bool

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

// BulkSwitchResult contains the results of a bulk switch operation.
type BulkSwitchResult struct {
	// TotalScanned is the number of repositories found
	TotalScanned int

	// TotalProcessed is the number of repositories processed
	TotalProcessed int

	// Repositories contains individual repository results
	Repositories []RepositorySwitchResult

	// Duration is the total operation time
	Duration time.Duration

	// Summary contains status counts
	Summary map[string]int

	// TargetBranch is the branch that was switched to
	TargetBranch string
}

// RepositorySwitchResult represents the result for a single repository switch.
type RepositorySwitchResult struct {
	// Path is the repository path
	Path string

	// RelativePath is the path relative to scan root
	RelativePath string

	// Status is the operation status (switched, already-on-branch, dirty, error, etc.)
	Status string

	// Message is a human-readable status message
	Message string

	// Error if the operation failed
	Error error

	// Duration is how long this repository took to process
	Duration time.Duration

	// PreviousBranch is the branch before switching
	PreviousBranch string

	// CurrentBranch is the branch after switching (or current if not switched)
	CurrentBranch string

	// RemoteURL is the remote origin URL
	RemoteURL string

	// Remote is the remote name (e.g., "origin")
	Remote string

	// HasUncommittedChanges indicates if there were local changes preventing switch
	HasUncommittedChanges bool
}

// GetStatus returns the status for summary calculation.
func (r RepositorySwitchResult) GetStatus() string { return r.Status }

// BulkUpdate scans for repositories and updates them in parallel.
func (c *client) BulkUpdate(ctx context.Context, opts BulkUpdateOptions) (*BulkUpdateResult, error) {
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
		return &BulkUpdateResult{
			TotalScanned:   totalScanned,
			TotalProcessed: 0,
			Repositories:   []RepositoryUpdateResult{},
			Duration:       time.Since(startTime),
			Summary:        map[string]int{},
		}, nil
	}

	// Process repositories in parallel
	results, err := c.processRepositories(ctx, opts.Directory, filteredRepos, opts, common.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to process repositories: %w", err)
	}

	// Calculate summary
	summary := calculateSummary(results)

	return &BulkUpdateResult{
		TotalScanned:   totalScanned,
		TotalProcessed: len(filteredRepos),
		Repositories:   results,
		Duration:       time.Since(startTime),
		Summary:        summary,
	}, nil
}

// scanRepositoriesWithConfig recursively finds Git repositories with configuration.
func (c *client) scanRepositoriesWithConfig(ctx context.Context, dir string, maxDepth int, logger Logger, config walkDirectoryConfig) ([]string, error) {
	var repos []string
	var mu sync.Mutex

	// Start depth at 0 (root directory is depth 0)
	// maxDepth=1 means scan only direct children of root directory (depth 0 -> depth 1)
	// maxDepth=2 means scan root's children + their children (depth 0 -> depth 1 -> depth 2)
	err := c.walkDirectoryWithConfig(ctx, dir, 0, maxDepth, &repos, &mu, logger, config)
	if err != nil {
		return nil, err
	}

	// Sort for consistent ordering
	sort.Strings(repos)

	return repos, nil
}

// isSubmodule checks if a directory is a git submodule.
//
// A git submodule is identified by having a .git FILE (not directory) that points
// to the parent repository's .git/modules/ directory. This is the definitive way
// to distinguish submodules from independent nested repositories.
//
// Independent nested repositories have their own .git DIRECTORY with a complete
// git object database, whereas submodules only have a .git file containing a
// reference like "gitdir: ../.git/modules/submodule-name".
//
// This distinction is important for scanning strategies:
//   - Submodules should only be scanned when explicitly requested (--include-submodules)
//   - Independent nested repos should always be found during recursive scans
//
// Returns true if dir is a git submodule, false otherwise.
func isSubmodule(dir string) bool {
	// Primary check: If .git is a file (not directory), it's a submodule
	gitPath := filepath.Join(dir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return false
	}

	// If .git is a file, it's definitely a submodule
	if !info.IsDir() {
		return true
	}

	// If .git is a directory, it's an independent repository
	// (Even if parent has .gitmodules, this repo has its own .git directory)
	return false
}

// walkDirectoryConfig holds configuration for walkDirectoryWithConfig.
type walkDirectoryConfig struct {
	includeSubmodules bool
}

// walkDirectoryWithConfig walks directories with configuration options to find git repositories.
//
// Scanning Strategy:
//   - Always scans the root directory (depth 0) and its children
//   - For repositories found at depth > 0:
//   - If it's a submodule and config.includeSubmodules==false: skip its children
//   - If it's a submodule and config.includeSubmodules==true: scan its children
//   - If it's an independent nested repo: scan its children to find more nested repos
//   - Respects maxDepth limit to prevent infinite recursion
//   - Skips hidden directories (.git, node_modules, etc.) to improve performance
//
// This strategy ensures:
//   - Independent nested repositories are always discovered
//   - Submodules are only scanned when explicitly requested
//   - Deeply nested repository structures are properly handled
func (c *client) walkDirectoryWithConfig(ctx context.Context, dir string, depth, maxDepth int, repos *[]string, mu *sync.Mutex, logger Logger, config walkDirectoryConfig) error {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Check if this directory is a Git repository
	if c.IsRepository(ctx, dir) {
		mu.Lock()
		*repos = append(*repos, dir)
		mu.Unlock()
		logger.Debug("found repository", "path", dir)

		// Determine if we should continue scanning subdirectories
		// Skip scanning children in these cases:
		// 1. This is a submodule AND IncludeSubmodules is false
		// 2. This is depth > 0 AND is an independent nested repo (to avoid recursing into nested repo's children)

		switch {
		case isSubmodule(dir):
			if !config.includeSubmodules {
				// Skip submodule and its children
				logger.Debug("skipping submodule", "path", dir)
				return nil
			}
			// Include submodules, continue scanning
			logger.Debug("including submodule", "path", dir)
		case depth > 0:
			// This is an independent nested repository at depth > 0
			// Continue scanning its children to find more nested repos
			logger.Debug("found independent nested repository, continuing scan", "path", dir)
		default:
			// This is the root directory (depth 0), always scan its children
			logger.Debug("scanning children of root repository", "path", dir)
		}
	}

	// Check depth limit before scanning subdirectories
	// depth starts at 0 for the root directory
	// maxDepth=1 means scan only direct children of root (depth 0 -> depth 1)
	// maxDepth=2 means scan 2 levels (depth 0 -> depth 1 -> depth 2)
	// We continue scanning subdirectories only if depth < maxDepth
	if depth >= maxDepth {
		return nil
	}

	// Read directory entries
	entries, err := os.ReadDir(dir)
	if err != nil {
		// Log but don't fail on permission errors
		logger.Debug("cannot read directory", "path", dir, "error", err)
		return nil
	}

	// Scan subdirectories
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Skip hidden directories and common ignore patterns
		if shouldIgnoreDirectory(name) {
			continue
		}

		subDir := filepath.Join(dir, name)
		if err := c.walkDirectoryWithConfig(ctx, subDir, depth+1, maxDepth, repos, mu, logger, config); err != nil {
			return err
		}
	}

	return nil
}

// shouldIgnoreDirectory checks if a directory should be skipped.
func shouldIgnoreDirectory(name string) bool {
	// Skip hidden directories
	if name != "" && name[0] == '.' {
		return true
	}

	// Skip common ignore patterns
	ignorePatterns := []string{
		"node_modules",
		"vendor",
		"target",
		"build",
		"dist",
		"__pycache__",
		".cache",
		".tmp",
	}

	return slices.Contains(ignorePatterns, name)
}

// filterRepositories filters repositories based on include/exclude patterns.
func filterRepositories(repos []string, includePattern, excludePattern string, logger Logger) ([]string, error) {
	if includePattern == "" && excludePattern == "" {
		return repos, nil
	}

	var includeRegex, excludeRegex *regexp.Regexp
	var err error

	if includePattern != "" {
		includeRegex, err = regexp.Compile(includePattern)
		if err != nil {
			return nil, fmt.Errorf("invalid include pattern: %w", err)
		}
	}

	if excludePattern != "" {
		excludeRegex, err = regexp.Compile(excludePattern)
		if err != nil {
			return nil, fmt.Errorf("invalid exclude pattern: %w", err)
		}
	}

	var filtered []string
	for _, repo := range repos {
		// Check exclude first
		if excludeRegex != nil && excludeRegex.MatchString(repo) {
			logger.Debug("excluded repository", "path", repo)
			continue
		}

		// Check include
		if includeRegex != nil && !includeRegex.MatchString(repo) {
			logger.Debug("not included repository", "path", repo)
			continue
		}

		filtered = append(filtered, repo)
	}

	return filtered, nil
}

// processRepositories processes repositories in parallel.
func (c *client) processRepositories(ctx context.Context, rootDir string, repos []string, opts BulkUpdateOptions, logger Logger) ([]RepositoryUpdateResult, error) {
	results := make([]RepositoryUpdateResult, len(repos))

	// Create error group with concurrency limit
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(opts.Parallel)

	for i, repoPath := range repos {
		g.Go(func() error {
			// Call progress callback
			if opts.ProgressCallback != nil {
				opts.ProgressCallback(i+1, len(repos), repoPath)
			}

			result := c.processRepository(gctx, rootDir, repoPath, opts, logger)
			results[i] = result

			return nil // Don't fail entire operation on single repo error
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

// processRepository processes a single repository.
func (c *client) processRepository(ctx context.Context, rootDir, repoPath string, opts BulkUpdateOptions, logger Logger) RepositoryUpdateResult {
	startTime := time.Now()

	result := RepositoryUpdateResult{
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
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to get repository info"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	result.Branch = info.Branch
	result.RemoteURL = info.RemoteURL
	result.Remote = info.Remote
	result.CommitsBehind = info.BehindBy
	result.CommitsAhead = info.AheadBy

	// Get status
	status, err := c.GetStatus(ctx, repo)
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to get repository status"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	result.HasUncommittedChanges = !status.IsClean

	// Check if update is safe
	if result.HasUncommittedChanges {
		result.Status = StatusSkipped
		result.Message = "Has uncommitted changes - manual intervention required"
		result.Duration = time.Since(startTime)
		return result
	}

	if info.BehindBy == 0 {
		result.Status = StatusUpToDate
		result.Message = "Already up to date"
		result.Duration = time.Since(startTime)
		return result
	}

	if info.Upstream == "" {
		result.Status = StatusNoUpstream
		result.Message = "No upstream branch configured"
		result.Duration = time.Since(startTime)
		return result
	}

	// Dry run - don't actually update
	if opts.DryRun {
		result.Status = StatusWouldUpdate
		result.Message = fmt.Sprintf("Would pull %d commits", info.BehindBy)
		result.Duration = time.Since(startTime)
		return result
	}

	// Perform pull --rebase with non-interactive mode to prevent credential prompts
	pullResult, err := c.executor.RunWithEnv(ctx, repoPath, nonInteractiveEnv, "pull", "--rebase")
	if err != nil || pullResult.ExitCode != 0 {
		// Check for authentication errors
		if pullResult != nil && isAuthenticationError(pullResult.Stderr) {
			result.Status = StatusAuthRequired
			result.Message = "Authentication required"
			result.Error = ErrAuthRequired
		} else {
			result.Status = StatusError
			result.Message = "Pull failed"
			if err != nil {
				result.Error = err
			} else {
				result.Error = fmt.Errorf("pull exited with code %d: %w", pullResult.ExitCode, pullResult.Error)
			}
		}
		result.Duration = time.Since(startTime)
		return result
	}

	result.Status = StatusPulled
	result.Message = fmt.Sprintf("Successfully pulled %d commits", info.BehindBy)
	result.Duration = time.Since(startTime)

	logger.Info("repository pulled", "path", result.RelativePath, "commits", info.BehindBy)

	return result
}

// getRelativePath returns the relative path from root to target.
func getRelativePath(root, target string) string {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return target
	}
	return rel
}

// calculateSummary creates a summary of results by status.
func calculateSummary(results []RepositoryUpdateResult) map[string]int {
	return calculateSummaryGeneric(results)
}

// BulkFetch scans for repositories and fetches them in parallel.
func (c *client) BulkFetch(ctx context.Context, opts BulkFetchOptions) (*BulkFetchResult, error) {
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
		return &BulkFetchResult{
			TotalScanned:   totalScanned,
			TotalProcessed: 0,
			Repositories:   []RepositoryFetchResult{},
			Duration:       time.Since(startTime),
			Summary:        map[string]int{},
		}, nil
	}

	// Process repositories in parallel
	results, err := c.processFetchRepositories(ctx, opts.Directory, filteredRepos, opts, common.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to process repositories: %w", err)
	}

	// Calculate summary
	summary := calculateFetchSummary(results)

	return &BulkFetchResult{
		TotalScanned:   totalScanned,
		TotalProcessed: len(filteredRepos),
		Repositories:   results,
		Duration:       time.Since(startTime),
		Summary:        summary,
	}, nil
}

// processFetchRepositories processes repositories in parallel for fetch.
func (c *client) processFetchRepositories(ctx context.Context, rootDir string, repos []string, opts BulkFetchOptions, logger Logger) ([]RepositoryFetchResult, error) {
	results := make([]RepositoryFetchResult, len(repos))

	// Create error group with concurrency limit
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(opts.Parallel)

	for i, repoPath := range repos {
		g.Go(func() error {
			// Call progress callback
			if opts.ProgressCallback != nil {
				opts.ProgressCallback(i+1, len(repos), repoPath)
			}

			result := c.processFetchRepository(gctx, rootDir, repoPath, opts, logger)
			results[i] = result

			return nil // Don't fail entire operation on single repo error
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

// processFetchRepository processes a single repository fetch.
func (c *client) processFetchRepository(ctx context.Context, rootDir, repoPath string, opts BulkFetchOptions, logger Logger) RepositoryFetchResult {
	startTime := time.Now()

	result := RepositoryFetchResult{
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
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to get repository info"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	result.Branch = info.Branch
	result.RemoteURL = info.RemoteURL
	result.Remote = info.Remote

	// Check if repository has remote
	if info.RemoteURL == "" {
		result.Status = StatusNoRemote
		result.Message = "No remote configured"
		result.Duration = time.Since(startTime)
		return result
	}

	// Dry run - don't actually fetch
	if opts.DryRun {
		result.Status = StatusWouldFetch
		result.Message = "Would fetch from remote"
		result.Duration = time.Since(startTime)
		return result
	}

	// Build fetch command
	fetchArgs := []string{"fetch"}

	if opts.AllRemotes {
		fetchArgs = append(fetchArgs, "--all")
	}

	if opts.Prune {
		fetchArgs = append(fetchArgs, "--prune")
	}

	if opts.Tags {
		fetchArgs = append(fetchArgs, "--tags")
	}

	if opts.Verbose {
		fetchArgs = append(fetchArgs, "--verbose")
	} else {
		fetchArgs = append(fetchArgs, "--quiet")
	}

	// Perform fetch with non-interactive mode to prevent credential prompts
	fetchResult, err := c.executor.RunWithEnv(ctx, repoPath, nonInteractiveEnv, fetchArgs...)
	if err != nil || fetchResult.ExitCode != 0 {
		// Check for authentication errors
		if fetchResult != nil && isAuthenticationError(fetchResult.Stderr) {
			result.Status = StatusAuthRequired
			result.Message = "Authentication required"
			result.Error = ErrAuthRequired
		} else {
			result.Status = StatusError
			result.Message = "Fetch failed"
			if err != nil {
				result.Error = err
			} else {
				result.Error = fmt.Errorf("fetch exited with code %d: %w", fetchResult.ExitCode, fetchResult.Error)
			}
		}
		result.Duration = time.Since(startTime)
		return result
	}

	// Get updated info after fetch to check behind/ahead status
	updatedInfo, err := c.GetInfo(ctx, repo)
	if err == nil {
		result.CommitsBehind = updatedInfo.BehindBy
		result.CommitsAhead = updatedInfo.AheadBy

		// Update status based on behind/ahead state
		// Use StatusFetched when changes were fetched, StatusUpToDate when no changes
		switch {
		case result.CommitsBehind > 0:
			result.Status = StatusFetched
			if result.CommitsAhead > 0 {
				result.Message = fmt.Sprintf("Fetched updates: %d behind, %d ahead", result.CommitsBehind, result.CommitsAhead)
			} else {
				result.Message = fmt.Sprintf("Fetched updates: %d behind", result.CommitsBehind)
			}
		case result.CommitsAhead > 0:
			result.Status = StatusUpToDate
			result.Message = fmt.Sprintf("Up to date: %d ahead", result.CommitsAhead)
		default:
			result.Status = StatusUpToDate
			result.Message = "Already up to date"
		}
	} else {
		// Fallback if GetInfo fails - assume fetched successfully
		result.Status = StatusFetched
		result.Message = "Successfully fetched from remote"
	}

	// Check dirty status after fetch (for user awareness)
	c.populateFetchDirtyStatus(ctx, repo, &result)

	result.Duration = time.Since(startTime)

	logger.Info("repository fetched", "path", result.RelativePath, "branch", result.Branch, "behind", result.CommitsBehind, "ahead", result.CommitsAhead)

	return result
}

// populateFetchDirtyStatus checks and populates the dirty status fields in fetch result.
func (c *client) populateFetchDirtyStatus(ctx context.Context, repo *Repository, result *RepositoryFetchResult) {
	status, err := c.GetStatus(ctx, repo)
	if err != nil {
		return
	}
	result.TrackedChangedFiles = status.TrackedChangedCount
	result.UncommittedFiles = status.TrackedChangedCount
	result.StagedFiles = status.StagedCount
	result.UnstagedFiles = status.UnstagedCount
	result.UntrackedFiles = len(status.UntrackedFiles)
}

// calculateFetchSummary creates a summary of fetch results by status.
func calculateFetchSummary(results []RepositoryFetchResult) map[string]int {
	return calculateSummaryGeneric(results)
}

// BulkPull scans for repositories and pulls them in parallel.
func (c *client) BulkPull(ctx context.Context, opts BulkPullOptions) (*BulkPullResult, error) {
	startTime := time.Now()

	// Set strategy default
	if opts.Strategy == "" {
		opts.Strategy = "merge"
	}

	// Validate strategy
	validStrategies := map[string]bool{
		"merge":   true,
		"rebase":  true,
		"ff-only": true,
	}
	if !validStrategies[opts.Strategy] {
		return nil, fmt.Errorf("invalid strategy: %s (valid: merge, rebase, ff-only)", opts.Strategy)
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
		return &BulkPullResult{
			TotalScanned:   totalScanned,
			TotalProcessed: 0,
			Repositories:   []RepositoryPullResult{},
			Duration:       time.Since(startTime),
			Summary:        map[string]int{},
		}, nil
	}

	// Process repositories in parallel
	results, err := c.processPullRepositories(ctx, opts.Directory, filteredRepos, opts, common.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to process repositories: %w", err)
	}

	// Calculate summary
	summary := calculatePullSummary(results)

	return &BulkPullResult{
		TotalScanned:   totalScanned,
		TotalProcessed: len(filteredRepos),
		Repositories:   results,
		Duration:       time.Since(startTime),
		Summary:        summary,
	}, nil
}

// processPullRepositories processes repositories in parallel for pull.
func (c *client) processPullRepositories(ctx context.Context, rootDir string, repos []string, opts BulkPullOptions, logger Logger) ([]RepositoryPullResult, error) {
	results := make([]RepositoryPullResult, len(repos))

	// Create error group with concurrency limit
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(opts.Parallel)

	for i, repoPath := range repos {
		g.Go(func() error {
			// Call progress callback
			if opts.ProgressCallback != nil {
				opts.ProgressCallback(i+1, len(repos), repoPath)
			}

			result := c.processPullRepository(gctx, rootDir, repoPath, opts, logger)
			results[i] = result

			return nil // Don't fail entire operation on single repo error
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

// processPullRepository processes a single repository pull.
//
//nolint:gocognit,gocyclo // TODO: Refactor into smaller helper functions (validateRepo, handleState, executePull, handleResult)
func (c *client) processPullRepository(ctx context.Context, rootDir, repoPath string, opts BulkPullOptions, logger Logger) RepositoryPullResult {
	startTime := time.Now()

	result := RepositoryPullResult{
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
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to get repository info"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	result.Branch = info.Branch
	result.RemoteURL = info.RemoteURL
	result.Remote = info.Remote
	result.CommitsBehind = info.BehindBy
	result.CommitsAhead = info.AheadBy

	// Check if repository has remote
	if info.RemoteURL == "" {
		result.Status = StatusNoRemote
		result.Message = "No remote configured"
		result.Duration = time.Since(startTime)
		return result
	}

	// Check if repository has upstream
	if info.Upstream == "" {
		result.Status = StatusNoUpstream
		result.Message = "No upstream branch configured"
		result.Duration = time.Since(startTime)
		return result
	}

	// Check repository status
	status, err := c.GetStatus(ctx, repo)
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to get repository status"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	// Check detailed repository state
	repoState, err := c.checkRepositoryState(ctx, repoPath)
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to check repository state"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	// Handle repositories with conflicts
	if repoState.HasConflicts {
		result.Status = StatusConflict
		result.Message = fmt.Sprintf("Repository has conflicts in %d file(s): %s",
			len(repoState.ConflictedFiles),
			strings.Join(repoState.ConflictedFiles, ", "))
		result.Error = fmt.Errorf("cannot pull: repository has unresolved conflicts")
		result.Duration = time.Since(startTime)
		logger.Warn("repository has conflicts", "path", result.RelativePath, "files", repoState.ConflictedFiles)
		return result
	}

	// Handle repositories with ongoing rebase
	if repoState.RebaseInProgress {
		result.Status = StatusRebaseInProgress
		result.Message = "Repository has rebase in progress - skipping"
		result.Error = fmt.Errorf("rebase in progress, run 'git rebase --continue' or 'git rebase --abort'")
		result.Duration = time.Since(startTime)
		logger.Warn("rebase in progress", "path", result.RelativePath)
		return result
	}

	// Handle repositories with ongoing merge
	if repoState.MergeInProgress {
		result.Status = StatusMergeInProgress
		result.Message = "Repository has merge in progress - skipping"
		result.Error = fmt.Errorf("merge in progress, resolve conflicts and commit")
		result.Duration = time.Since(startTime)
		logger.Warn("merge in progress", "path", result.RelativePath)
		return result
	}

	// Handle local changes with stash
	if !status.IsClean && opts.Stash {
		stashArgs := []string{"stash", "push", "-m", "Auto-stash by gz-git pull"}
		stashResult, err := c.executor.Run(ctx, repoPath, stashArgs...)
		if err != nil || stashResult.ExitCode != 0 {
			result.Status = StatusError
			result.Message = "Failed to stash local changes"
			if err != nil {
				result.Error = err
			} else {
				result.Error = fmt.Errorf("stash exited with code %d: %w", stashResult.ExitCode, stashResult.Error)
			}
			result.Duration = time.Since(startTime)
			return result
		}
		result.Stashed = true
		logger.Info("stashed local changes", "path", result.RelativePath)
	}

	// Dry run - don't actually pull
	if opts.DryRun {
		if result.CommitsBehind > 0 {
			result.Status = StatusWouldPull
			result.Message = fmt.Sprintf("Would pull %d commit(s) from remote", result.CommitsBehind)
		} else {
			result.Status = StatusUpToDate
			result.Message = "Already up to date"
		}
		result.Duration = time.Since(startTime)
		return result
	}

	// Capture HEAD commit before pull to detect actual changes
	// This is more reliable than CommitsBehind which uses stale local remote-tracking branches
	headBeforeResult, err := c.executor.Run(ctx, repoPath, "rev-parse", "HEAD")
	var headBefore string
	if err == nil && headBeforeResult.ExitCode == 0 {
		headBefore = strings.TrimSpace(headBeforeResult.Stdout)
	}

	// Build pull command based on strategy
	// IMPORTANT: Always pass explicit strategy flag to override user's git config (pull.rebase=true, etc.)
	// Without this, user's global pull.rebase=true causes merge strategy to silently use rebase,
	// which fails on dirty repos with exit code 128 ("cannot pull with rebase: You have unstaged changes")
	pullArgs := []string{"pull"}

	switch opts.Strategy {
	case "rebase":
		pullArgs = append(pullArgs, "--rebase")
	case "ff-only":
		pullArgs = append(pullArgs, "--ff-only")
	default:
		// Explicit --no-rebase ensures merge strategy regardless of user's git config
		pullArgs = append(pullArgs, "--no-rebase")
	}

	if opts.Prune {
		pullArgs = append(pullArgs, "--prune")
	}

	if opts.Tags {
		pullArgs = append(pullArgs, "--tags")
	}

	if opts.Verbose {
		pullArgs = append(pullArgs, "--verbose")
	} else {
		pullArgs = append(pullArgs, "--quiet")
	}

	// Perform pull with non-interactive mode to prevent credential prompts
	pullResult, err := c.executor.RunWithEnv(ctx, repoPath, nonInteractiveEnv, pullArgs...)
	if err != nil || pullResult.ExitCode != 0 {
		// Check for authentication errors first
		if pullResult != nil && isAuthenticationError(pullResult.Stderr) {
			result.Status = StatusAuthRequired
			result.Message = "Authentication required"
			result.Error = ErrAuthRequired
			result.Duration = time.Since(startTime)
			return result
		}
		// Check if pull failed due to conflicts
		postPullState, stateErr := c.checkRepositoryState(ctx, repoPath)
		if stateErr == nil {
			if postPullState.HasConflicts {
				// Conflicts detected - abort rebase/merge to restore clean state
				if opts.Strategy == "rebase" && postPullState.RebaseInProgress {
					logger.Warn("rebase created conflicts, aborting", "path", result.RelativePath)
					if abortErr := c.abortRebaseIfNeeded(ctx, repoPath); abortErr != nil {
						logger.Error("failed to abort rebase", "path", result.RelativePath, "error", abortErr)
					} else {
						logger.Info("rebase aborted, repository restored to clean state", "path", result.RelativePath)
					}
				}
				result.Status = StatusConflict
				result.Message = fmt.Sprintf("Pull failed: conflicts in %d file(s) - %s",
					len(postPullState.ConflictedFiles),
					strings.Join(postPullState.ConflictedFiles, ", "))
				result.Error = fmt.Errorf("pull created conflicts, repository restored to previous state")
			} else {
				// Non-conflict error - include stderr for diagnostics
				result.Status = StatusError
				result.Message = "Pull failed"
				result.Error = buildPullError(err, pullResult.ExitCode, pullResult.Stderr, pullResult.Error)
			}
		} else {
			// Couldn't check state, report general error
			result.Status = StatusError
			result.Message = "Pull failed"
			result.Error = buildPullError(err, pullResult.ExitCode, pullResult.Stderr, pullResult.Error)
		}
		result.Duration = time.Since(startTime)

		// Try to pop stash if we stashed earlier
		if result.Stashed {
			popArgs := []string{"stash", "pop"}
			_, _ = c.executor.Run(ctx, repoPath, popArgs...) //nolint:errcheck // Best effort
		}

		return result
	}

	// Determine if actual changes were pulled by comparing HEAD before and after
	// This is more reliable than checking CommitsBehind (which uses stale local tracking branches)
	// or parsing git output (which may be empty with --quiet flag)
	headAfterResult, headAfterErr := c.executor.Run(ctx, repoPath, "rev-parse", "HEAD")
	var headAfter string
	if headAfterErr == nil && headAfterResult.ExitCode == 0 {
		headAfter = strings.TrimSpace(headAfterResult.Stdout)
	}

	// Compare HEAD before and after to determine if changes were actually pulled
	if headBefore != "" && headAfter != "" && headBefore != headAfter {
		// HEAD changed - actual changes were pulled
		// Count commits pulled using rev-list
		countResult, countErr := c.executor.Run(ctx, repoPath, "rev-list", "--count", headBefore+".."+headAfter)
		pulledCount := 0
		if countErr == nil && countResult.ExitCode == 0 {
			if n, parseErr := fmt.Sscanf(strings.TrimSpace(countResult.Stdout), "%d", &pulledCount); parseErr != nil || n != 1 {
				pulledCount = 1 // Fallback to 1 if parsing fails
			}
		} else {
			pulledCount = 1 // Fallback to 1 if command fails
		}
		result.CommitsBehind = pulledCount // Update with actual pulled count
		result.Status = StatusPulled
		result.Message = fmt.Sprintf("Successfully pulled %d commit(s) from remote", pulledCount)
	} else {
		// HEAD unchanged - no changes pulled
		result.Status = StatusUpToDate
		result.Message = "Already up to date"
	}
	result.Duration = time.Since(startTime)

	// Pop stash if we stashed earlier
	if result.Stashed {
		popArgs := []string{"stash", "pop"}
		popResult, err := c.executor.Run(ctx, repoPath, popArgs...)
		if err != nil || popResult.ExitCode != 0 {
			logger.Warn("failed to pop stash", "path", result.RelativePath)
			// Don't fail the pull operation, just warn
		} else {
			logger.Info("popped stashed changes", "path", result.RelativePath)
		}
	}

	// Check dirty status after pull (for user awareness)
	c.populatePullDirtyStatus(ctx, repo, &result)

	logger.Info("repository pulled", "path", result.RelativePath)

	return result
}

// populatePullDirtyStatus checks and populates the dirty status fields in pull result.
func (c *client) populatePullDirtyStatus(ctx context.Context, repo *Repository, result *RepositoryPullResult) {
	status, err := c.GetStatus(ctx, repo)
	if err != nil {
		return
	}
	result.TrackedChangedFiles = status.TrackedChangedCount
	result.UncommittedFiles = status.TrackedChangedCount
	result.StagedFiles = status.StagedCount
	result.UnstagedFiles = status.UnstagedCount
	result.UntrackedFiles = len(status.UntrackedFiles)
}

// buildPullError constructs an informative error from a failed pull command.
// Includes stderr content when available to help diagnose issues like
// "cannot pull with rebase: You have unstaged changes" (exit code 128).
func buildPullError(execErr error, exitCode int, stderr string, cmdErr error) error {
	if execErr != nil {
		return execErr
	}
	stderrTrimmed := strings.TrimSpace(stderr)
	if stderrTrimmed != "" {
		return fmt.Errorf("pull exited with code %d: %s", exitCode, stderrTrimmed)
	}
	if cmdErr != nil {
		return fmt.Errorf("pull exited with code %d: %w", exitCode, cmdErr)
	}
	return fmt.Errorf("pull exited with code %d", exitCode)
}

// calculatePullSummary creates a summary of pull results by status.
func calculatePullSummary(results []RepositoryPullResult) map[string]int {
	return calculateSummaryGeneric(results)
}

// repositoryState represents the current state of a git repository.
//
// It carries no file counts. It used to hold UncommittedFiles (the raw porcelain
// line count, untracked entries included) and IsDirty, but the sole caller also
// holds a *Status and was adding the untracked paths a second time from there.
// Counts belong to Status, which is the one value that saw the XY codes; this
// type answers only "what operation is this repository in the middle of".
type repositoryState struct {
	HasConflicts     bool
	RebaseInProgress bool
	MergeInProgress  bool
	ConflictedFiles  []string
}

// checkRepositoryState checks the detailed state of a repository.
func (c *client) checkRepositoryState(ctx context.Context, repoPath string) (*repositoryState, error) {
	state := &repositoryState{
		RebaseInProgress: IsRebaseInProgress(repoPath),
		MergeInProgress:  IsMergeInProgress(repoPath),
	}

	// -z through runGit, for the three reasons porcelain.Parse documents plus a
	// fourth specific to this caller: runGit surfaces a non-zero exit, which the
	// previous `err != nil` check could not. Executor.Run reports a failed git
	// through Result.ExitCode and returns a nil error, so a git that died here
	// read as a conflict-free, clean repository — and the push guard built on
	// HasConflicts opened.
	//
	// -uno, not -uall: this function reads only ConflictFiles, and a conflicted
	// path is by definition tracked. -uall would force a full recursive walk of
	// every untracked directory to produce entries nothing here looks at, and
	// processStatusRepository already pays for one such walk in GetStatus. If a
	// future field here needs untracked paths, take them from that Status rather
	// than widening this call back out.
	stdout, err := c.runGit(ctx, repoPath, "status", "--porcelain", "-z", "-uno")
	if err != nil {
		return nil, fmt.Errorf("failed to get repository status: %w", err)
	}

	status, err := parseStatusZ(stdout)
	if err != nil {
		return nil, fmt.Errorf("failed to parse repository status: %w", err)
	}

	state.HasConflicts = len(status.ConflictFiles) > 0

	if state.HasConflicts {
		// Detection comes from porcelain, the path list from the index: ls-files
		// reads stages 1-3 directly and does not depend on how the working tree
		// currently looks. Fall back to the porcelain paths when it yields
		// nothing, so a detected conflict is never reported with no files.
		state.ConflictedFiles = c.collectConflictedPaths(ctx, repoPath)
		if len(state.ConflictedFiles) == 0 {
			state.ConflictedFiles = status.ConflictFiles
		}
	}

	return state, nil
}

// abortRebaseIfNeeded aborts an ongoing rebase operation.
func (c *client) abortRebaseIfNeeded(ctx context.Context, repoPath string) error {
	result, err := c.executor.Run(ctx, repoPath, "rebase", "--abort")
	if err != nil {
		return fmt.Errorf("failed to abort rebase: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("rebase abort failed: %w", result.Error)
	}
	return nil
}

// BulkPush scans for repositories and pushes them in parallel.
func (c *client) BulkPush(ctx context.Context, opts BulkPushOptions) (*BulkPushResult, error) {
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
		return &BulkPushResult{
			TotalScanned:   totalScanned,
			TotalProcessed: 0,
			Repositories:   []RepositoryPushResult{},
			Duration:       time.Since(startTime),
			Summary:        map[string]int{},
		}, nil
	}

	// Process repositories in parallel
	results, err := c.processPushRepositories(ctx, opts.Directory, filteredRepos, opts, common.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to process repositories: %w", err)
	}

	// Calculate summary
	summary := calculatePushSummary(results)

	return &BulkPushResult{
		TotalScanned:   totalScanned,
		TotalProcessed: len(filteredRepos),
		Repositories:   results,
		Duration:       time.Since(startTime),
		Summary:        summary,
	}, nil
}

// processPushRepositories processes repositories in parallel for push.
func (c *client) processPushRepositories(ctx context.Context, rootDir string, repos []string, opts BulkPushOptions, logger Logger) ([]RepositoryPushResult, error) {
	results := make([]RepositoryPushResult, len(repos))

	// Create error group with concurrency limit
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(opts.Parallel)

	for i, repoPath := range repos {
		g.Go(func() error {
			// Call progress callback
			if opts.ProgressCallback != nil {
				opts.ProgressCallback(i+1, len(repos), repoPath)
			}

			result := c.processPushRepository(gctx, rootDir, repoPath, opts, logger)
			results[i] = result

			return nil // Don't fail entire operation on single repo error
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

// processPushRepository processes a single repository push.
//
//nolint:gocognit,gocyclo // TODO: Refactor into smaller helper functions (similar to processPullRepository)
func (c *client) processPushRepository(ctx context.Context, rootDir, repoPath string, opts BulkPushOptions, logger Logger) RepositoryPushResult {
	startTime := time.Now()

	result := RepositoryPushResult{
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
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to get repository info"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	result.Branch = info.Branch
	result.RemoteURL = info.RemoteURL
	result.Remote = info.Remote
	result.CommitsAhead = info.AheadBy

	// CRITICAL: Check refspec validity BEFORE checking remote/state
	// This ensures we give clear error messages for missing source branches
	// rather than generic "no remote" errors
	if opts.Refspec != "" {
		// Parse refspec to get source branch
		parsed, err := ValidateRefspec(opts.Refspec)
		if err != nil {
			result.Status = StatusError
			result.Message = "Invalid refspec"
			result.Error = fmt.Errorf("failed to parse refspec: %w", err)
			result.Duration = time.Since(startTime)
			return result
		}

		// Check if source branch exists locally BEFORE checking remote
		sourceBranch := parsed.GetSourceBranch()
		sourceCheckResult, err := c.executor.Run(ctx, repoPath, "rev-parse", "--verify", sourceBranch)
		if err != nil || sourceCheckResult.ExitCode != 0 {
			result.Status = StatusError
			result.Message = fmt.Sprintf("Source branch '%s' does not exist", sourceBranch)
			result.Error = fmt.Errorf("refspec source branch '%s' not found in repository (current branch: %s)", sourceBranch, info.Branch)
			result.Duration = time.Since(startTime)
			logger.Warn("source branch not found", "path", result.RelativePath, "source", sourceBranch, "current", info.Branch)
			return result
		}
	}

	// Check if repository has remote
	if info.RemoteURL == "" {
		result.Status = StatusNoRemote
		result.Message = "No remote configured"
		result.Duration = time.Since(startTime)
		return result
	}

	// Check detailed repository state before pushing
	repoState, err := c.checkRepositoryState(ctx, repoPath)
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to check repository state"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	// Handle repositories with conflicts
	if repoState.HasConflicts {
		result.Status = StatusConflict
		result.Message = fmt.Sprintf("Repository has conflicts in %d file(s): %s",
			len(repoState.ConflictedFiles),
			strings.Join(repoState.ConflictedFiles, ", "))
		result.Error = fmt.Errorf("cannot push: repository has unresolved conflicts")
		result.Duration = time.Since(startTime)
		logger.Warn("repository has conflicts", "path", result.RelativePath, "files", repoState.ConflictedFiles)
		return result
	}

	// Handle repositories with ongoing rebase
	if repoState.RebaseInProgress {
		result.Status = StatusRebaseInProgress
		result.Message = "Repository has rebase in progress - skipping"
		result.Error = fmt.Errorf("rebase in progress, run 'git rebase --continue' or 'git rebase --abort'")
		result.Duration = time.Since(startTime)
		logger.Warn("rebase in progress", "path", result.RelativePath)
		return result
	}

	// Handle repositories with ongoing merge
	if repoState.MergeInProgress {
		result.Status = StatusMergeInProgress
		result.Message = "Repository has merge in progress - skipping"
		result.Error = fmt.Errorf("merge in progress, resolve conflicts and commit")
		result.Duration = time.Since(startTime)
		logger.Warn("merge in progress", "path", result.RelativePath)
		return result
	}

	// Determine whether we should set upstream for this repo.
	// Guardrail: only set upstream when it is missing; do not override existing tracking.
	setUpstreamMissing := opts.SetUpstream && info.Upstream == ""

	// Check if repository has upstream (unless we're setting it)
	if info.Upstream == "" && !setUpstreamMissing {
		result.Status = StatusNoUpstream
		result.Message = "No upstream branch configured (use --set-upstream to set)"
		result.Duration = time.Since(startTime)
		return result
	}

	// Check if there are commits to push
	// Skip this check when refspec is provided (e.g., develop:master)
	// because AheadBy is calculated against the current branch's upstream,
	// not the refspec target branch
	if info.AheadBy == 0 && !opts.Tags && opts.Refspec == "" && !setUpstreamMissing {
		result.Status = StatusUpToDate
		result.Message = "Nothing to push"
		result.Duration = time.Since(startTime)
		return result
	}

	// Dry run - don't actually push
	if opts.DryRun {
		result.Status = StatusWouldPush
		switch {
		case info.AheadBy > 0:
			result.Message = fmt.Sprintf("Would push %d commit(s) to remote", info.AheadBy)
		case opts.Tags:
			result.Message = "Would push tags to remote"
		case setUpstreamMissing:
			result.Message = fmt.Sprintf("Would set upstream for branch '%s'", info.Branch)
		default:
			result.Message = "Would push"
		}
		result.Duration = time.Since(startTime)
		return result
	}

	// Determine target remotes
	remotes := opts.Remotes
	if len(remotes) == 0 {
		if opts.AllRemotes {
			// Get all configured remotes
			remotesResult, err := c.executor.Run(ctx, repoPath, "remote")
			if err != nil || remotesResult.ExitCode != 0 {
				result.Status = StatusError
				result.Message = "Failed to get remotes"
				result.Error = fmt.Errorf("failed to get remotes: %w", err)
				result.Duration = time.Since(startTime)
				return result
			}
			for line := range strings.SplitSeq(strings.TrimSpace(remotesResult.Stdout), "\n") {
				if remote := strings.TrimSpace(line); remote != "" {
					remotes = append(remotes, remote)
				}
			}
			if len(remotes) == 0 {
				result.Status = StatusNoRemote
				result.Message = "No remotes configured"
				result.Duration = time.Since(startTime)
				return result
			}
		} else {
			remotes = []string{"origin"}
		}
	}

	// Calculate commits to be pushed if using refspec
	var actualCommitsToPush int
	if opts.Refspec != "" {
		// Parse refspec - already validated and checked earlier, so this should not fail
		parsed, _ := ValidateRefspec(opts.Refspec) //nolint:errcheck // refspec already validated upstream; error here is impossible in practice

		// Calculate commits between local source branch and remote destination branch
		// This gives us the actual commit count for refspec push
		sourceBranch := parsed.GetSourceBranch()
		destBranch := parsed.GetDestinationBranch()

		// Check if remote branch exists first
		for _, remote := range remotes {
			remoteBranch := fmt.Sprintf("%s/%s", remote, destBranch)
			checkResult, err := c.executor.Run(ctx, repoPath, "rev-parse", "--verify", remoteBranch)
			if err == nil && checkResult.ExitCode == 0 {
				// Remote branch exists, calculate commits ahead
				countResult, err := c.executor.Run(ctx, repoPath, "rev-list", "--count", remoteBranch+".."+sourceBranch)
				if err == nil && countResult.ExitCode == 0 {
					var count int
					if n, parseErr := fmt.Sscanf(strings.TrimSpace(countResult.Stdout), "%d", &count); parseErr == nil && n == 1 {
						actualCommitsToPush = count
						break
					}
				}
			}
			// If remote branch doesn't exist, count all commits in source branch
			// This will be a new branch on remote
			if actualCommitsToPush == 0 {
				countResult, err := c.executor.Run(ctx, repoPath, "rev-list", "--count", sourceBranch)
				if err == nil && countResult.ExitCode == 0 {
					var count int
					if n, parseErr := fmt.Sscanf(strings.TrimSpace(countResult.Stdout), "%d", &count); parseErr == nil && n == 1 {
						actualCommitsToPush = count
						break
					}
				}
			}
		}
	} else {
		actualCommitsToPush = info.AheadBy
	}

	// If we are setting upstream and multiple remotes are targeted, avoid upstream flapping.
	// Choose a deterministic primary remote and set upstream only against it.
	primaryRemote := ""
	if setUpstreamMissing && len(remotes) > 0 {
		primaryRemote = remotes[0]
		if slices.Contains(remotes, "origin") {
			primaryRemote = "origin"
		}
	}

	// Push to each remote
	var pushErrors []string
	var hasAuthError bool
	for _, remote := range remotes {
		remoteOpts := opts
		// Only set upstream for the deterministic primary remote.
		remoteOpts.SetUpstream = setUpstreamMissing && remote == primaryRemote

		if err := c.pushToRemote(ctx, repoPath, remote, info.Branch, remoteOpts); err != nil {
			pushErrors = append(pushErrors, fmt.Sprintf("%s: %v", remote, err))
			if errors.Is(err, ErrAuthRequired) {
				hasAuthError = true
			}
		}
	}

	if len(pushErrors) > 0 {
		// Use auth-required status if any remote had auth errors
		if hasAuthError {
			result.Status = StatusAuthRequired
			result.Message = "Authentication required"
		} else {
			result.Status = StatusError
			result.Message = "Push failed for some remotes"
		}
		result.Error = fmt.Errorf("%s", strings.Join(pushErrors, "; "))
		result.Duration = time.Since(startTime)
		return result
	}

	// Use specific statuses: StatusPushed (changes pushed) vs StatusUpToDate (no changes)
	result.PushedCommits = actualCommitsToPush
	if actualCommitsToPush > 0 {
		result.Status = StatusPushed
		if opts.Refspec != "" {
			result.Message = fmt.Sprintf("Successfully pushed %d commit(s) to %d remote(s) via refspec", actualCommitsToPush, len(remotes))
		} else {
			result.Message = fmt.Sprintf("Successfully pushed %d commit(s) to %d remote(s)", actualCommitsToPush, len(remotes))
		}
	} else {
		result.Status = StatusUpToDate
		result.Message = "Already up to date"
	}

	// Check dirty status after push (for user awareness)
	// Skip if --ignore-dirty flag is set (useful for CI/CD)
	if !opts.IgnoreDirty {
		c.populateDirtyStatus(ctx, repo, &result)
	}

	result.Duration = time.Since(startTime)

	logger.Info("repository pushed", "path", result.RelativePath, "commits", result.PushedCommits, "remotes", len(remotes))

	return result
}

// pushToRemote performs the actual push to a single remote.
func (c *client) pushToRemote(ctx context.Context, repoPath, remote, branch string, opts BulkPushOptions) error {
	// Build push command
	pushArgs := []string{"push"}

	if opts.Force {
		pushArgs = append(pushArgs, "--force-with-lease")
	}

	if opts.SetUpstream {
		pushArgs = append(pushArgs, "--set-upstream")
	}

	if opts.Tags {
		pushArgs = append(pushArgs, "--tags")
	}

	if opts.Verbose {
		pushArgs = append(pushArgs, "--verbose")
	} else {
		pushArgs = append(pushArgs, "--quiet")
	}

	pushArgs = append(pushArgs, remote)

	// Add refspec or branch
	if opts.Refspec != "" {
		pushArgs = append(pushArgs, opts.Refspec)
	} else {
		pushArgs = append(pushArgs, branch)
	}

	// Perform push with non-interactive mode to prevent credential prompts
	pushResult, err := c.executor.RunWithEnv(ctx, repoPath, nonInteractiveEnv, pushArgs...)
	if err != nil || pushResult.ExitCode != 0 {
		if err != nil {
			return err
		}
		// Check for authentication errors
		if isAuthenticationError(pushResult.Stderr) {
			return ErrAuthRequired
		}
		return fmt.Errorf("push exited with code %d: %w", pushResult.ExitCode, pushResult.Error)
	}

	return nil
}

// calculatePushSummary creates a summary of push results by status.
func calculatePushSummary(results []RepositoryPushResult) map[string]int {
	return calculateSummaryGeneric(results)
}

// populateDirtyStatus checks and populates the dirty status fields in push result.
// This is called after push to inform users about uncommitted/untracked files.
func (c *client) populateDirtyStatus(ctx context.Context, repo *Repository, result *RepositoryPushResult) {
	status, err := c.GetStatus(ctx, repo)
	if err != nil {
		// Don't fail the push result, just skip dirty status
		return
	}

	result.TrackedChangedFiles = status.TrackedChangedCount
	result.UncommittedFiles = status.TrackedChangedCount
	result.StagedFiles = status.StagedCount
	result.UnstagedFiles = status.UnstagedCount
	result.UntrackedFiles = len(status.UntrackedFiles)
}

// BulkStatus scans for repositories and checks their status in parallel.
func (c *client) BulkStatus(ctx context.Context, opts BulkStatusOptions) (*BulkStatusResult, error) {
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
		return &BulkStatusResult{
			TotalScanned:   totalScanned,
			TotalProcessed: 0,
			Repositories:   []RepositoryStatusResult{},
			Duration:       time.Since(startTime),
			Summary:        map[string]int{},
		}, nil
	}

	// Process repositories in parallel
	results, err := c.processStatusRepositories(ctx, opts.Directory, filteredRepos, opts, common.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to process repositories: %w", err)
	}

	// Calculate summary
	summary := calculateStatusSummary(results)

	return &BulkStatusResult{
		TotalScanned:   totalScanned,
		TotalProcessed: len(filteredRepos),
		Repositories:   results,
		Duration:       time.Since(startTime),
		Summary:        summary,
	}, nil
}

// processStatusRepositories processes repositories in parallel for status check.
func (c *client) processStatusRepositories(ctx context.Context, rootDir string, repos []string, opts BulkStatusOptions, logger Logger) ([]RepositoryStatusResult, error) {
	results := make([]RepositoryStatusResult, len(repos))

	// Create error group with concurrency limit
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(opts.Parallel)

	for i, repoPath := range repos {
		g.Go(func() error {
			// Call progress callback
			if opts.ProgressCallback != nil {
				opts.ProgressCallback(i+1, len(repos), repoPath)
			}

			result := c.processStatusRepository(gctx, rootDir, repoPath, opts, logger)
			results[i] = result

			return nil // Don't fail entire operation on single repo error
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

// processStatusRepository processes a single repository status check.
func (c *client) processStatusRepository(ctx context.Context, rootDir, repoPath string, _ BulkStatusOptions, logger Logger) RepositoryStatusResult {
	startTime := time.Now()

	result := RepositoryStatusResult{
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
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to get repository info"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	result.Branch = info.Branch
	result.RemoteURL = info.RemoteURL
	result.Remote = info.Remote
	result.Remotes = info.Remotes
	result.HeadSHA = info.HeadSHA
	result.Describe = info.Describe
	result.LastCommitMsg = info.LastCommitMsg
	result.LastCommitDate = info.LastCommitDate
	result.LastCommitAuthor = info.LastCommitAuthor
	result.LocalBranches = info.LocalBranches
	result.StashCount = info.StashCount
	result.CommitsBehind = info.BehindBy
	result.CommitsAhead = info.AheadBy

	// Get status
	status, err := c.GetStatus(ctx, repo)
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to get repository status"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	// Check detailed repository state
	repoState, err := c.checkRepositoryState(ctx, repoPath)
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to check repository state"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	result.RebaseInProgress = repoState.RebaseInProgress
	result.MergeInProgress = repoState.MergeInProgress
	result.ConflictFiles = repoState.ConflictedFiles
	result.TrackedChangedFiles = status.TrackedChangedCount
	result.UncommittedFiles = status.TrackedChangedCount
	result.StagedFiles = status.StagedCount
	result.UnstagedFiles = status.UnstagedCount
	result.UntrackedFiles = len(status.UntrackedFiles)

	// Determine status
	switch {
	case repoState.HasConflicts:
		result.Status = StatusConflict
		result.Message = fmt.Sprintf("Repository has conflicts in %d file(s)", len(repoState.ConflictedFiles))
	case repoState.RebaseInProgress:
		result.Status = StatusRebaseInProgress
		result.Message = "Rebase in progress"
	case repoState.MergeInProgress:
		result.Status = StatusMergeInProgress
		result.Message = "Merge in progress"
	case info.RemoteURL == "":
		if status.IsClean {
			result.Status = StatusNoRemote
			result.Message = "No remote configured (clean)"
		} else {
			result.Status = StatusDirty
			result.Message = "Working tree has uncommitted changes (no remote)"
		}
	case info.Upstream == "":
		if status.IsClean {
			result.Status = StatusNoUpstream
			result.Message = "No upstream branch configured (clean)"
		} else {
			result.Status = StatusDirty
			result.Message = "Working tree has uncommitted changes (no upstream)"
		}
	case !status.IsClean:
		result.Status = StatusDirty
		result.Message = fmt.Sprintf("Working tree has %d uncommitted file(s), %d untracked file(s)",
			result.TrackedChangedFiles, result.UntrackedFiles)
	default:
		result.Status = StatusClean
		switch {
		case info.AheadBy > 0 && info.BehindBy > 0:
			result.Message = fmt.Sprintf("Clean (%d ahead, %d behind)", info.AheadBy, info.BehindBy)
		case info.AheadBy > 0:
			result.Message = fmt.Sprintf("Clean (%d ahead)", info.AheadBy)
		case info.BehindBy > 0:
			result.Message = fmt.Sprintf("Clean (%d behind)", info.BehindBy)
		default:
			result.Message = "Clean and up to date"
		}
	}

	result.Duration = time.Since(startTime)

	logger.Info("repository status checked", "path", result.RelativePath, "status", result.Status)

	return result
}

// calculateStatusSummary creates a summary of status results by status.
func calculateStatusSummary(results []RepositoryStatusResult) map[string]int {
	return calculateSummaryGeneric(results)
}

// ============================================================================
// Common Bulk Operation Helpers
// ============================================================================

// bulkOperationCommon holds common configuration for bulk operations.
type bulkOperationCommon struct {
	Directory         string
	Parallel          int
	MaxDepth          int
	IncludeSubmodules bool
	IncludePattern    string
	ExcludePattern    string
	Logger            Logger
}

// initializeBulkOperation initializes common bulk operation settings
// Returns initialized common config and absolute directory path.
func initializeBulkOperation(
	directory string,
	parallel int,
	maxDepth int,
	includeSubmodules bool,
	includePattern string,
	excludePattern string,
	logger Logger,
) (*bulkOperationCommon, error) {
	// Set directory default
	if directory == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
		directory = cwd
	}

	// Set parallel default
	if parallel <= 0 {
		parallel = DefaultBulkParallel
	}

	// Set maxDepth default
	if maxDepth <= 0 {
		maxDepth = DefaultBulkMaxDepth
	}

	// Set logger default
	if logger == nil {
		logger = &noopLogger{}
	}

	// Resolve absolute path
	absPath, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	return &bulkOperationCommon{
		Directory:         absPath,
		Parallel:          parallel,
		MaxDepth:          maxDepth,
		IncludeSubmodules: includeSubmodules,
		IncludePattern:    includePattern,
		ExcludePattern:    excludePattern,
		Logger:            logger,
	}, nil
}

// ScanOptions configures repository scanning without processing.
type ScanOptions struct {
	// Directory is the root directory to scan for repositories
	Directory string

	// MaxDepth is the maximum directory depth to scan (default: 1)
	MaxDepth int

	// IncludeSubmodules includes git submodules in the scan (default: false)
	IncludeSubmodules bool

	// IncludePattern is a regex pattern for repositories to include
	IncludePattern string

	// ExcludePattern is a regex pattern for repositories to exclude
	ExcludePattern string

	// Logger for operation feedback
	Logger Logger
}

// ScanResult contains the results of a repository scan.
type ScanResult struct {
	// Paths contains the absolute paths of discovered repositories.
	Paths []string

	// TotalScanned is the number of repositories found before filtering.
	TotalScanned int

	// Directory is the resolved absolute root directory.
	Directory string
}

// ScanRepositories scans a directory for Git repositories and returns their paths.
// This is a lightweight scan-only operation (no GetInfo/GetStatus).
func (c *client) ScanRepositories(ctx context.Context, opts ScanOptions) (*ScanResult, error) {
	common, err := initializeBulkOperation(
		opts.Directory,
		1, // parallel not used for scan
		opts.MaxDepth,
		opts.IncludeSubmodules,
		opts.IncludePattern,
		opts.ExcludePattern,
		opts.Logger,
	)
	if err != nil {
		return nil, err
	}

	filteredRepos, totalScanned, err := c.scanAndFilterRepositories(ctx, common)
	if err != nil {
		return nil, err
	}

	return &ScanResult{
		Paths:        filteredRepos,
		TotalScanned: totalScanned,
		Directory:    common.Directory,
	}, nil
}

// scanAndFilterRepositories scans for repositories and applies filters.
func (c *client) scanAndFilterRepositories(
	ctx context.Context,
	common *bulkOperationCommon,
) (paths []string, totalScanned int, err error) {
	common.Logger.Info("scanning for repositories", "directory", common.Directory, "maxDepth", common.MaxDepth)

	// Scan for repositories
	repos, err := c.scanRepositoriesWithConfig(ctx, common.Directory, common.MaxDepth, common.Logger, walkDirectoryConfig{
		includeSubmodules: common.IncludeSubmodules,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to scan repositories: %w", err)
	}

	common.Logger.Info("scan complete", "found", len(repos))
	totalScanned = len(repos)

	// Filter repositories
	filteredRepos, err := filterRepositories(repos, common.IncludePattern, common.ExcludePattern, common.Logger)
	if err != nil {
		return nil, totalScanned, fmt.Errorf("failed to filter repositories: %w", err)
	}

	if len(filteredRepos) < len(repos) {
		common.Logger.Info("filtered repositories", "total", len(repos), "selected", len(filteredRepos))
	}

	return filteredRepos, totalScanned, nil
}
