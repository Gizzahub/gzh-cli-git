package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// BulkFetchOptions configures bulk repository fetch operations
type BulkFetchOptions struct {
	// Directory is the root directory to scan for repositories
	Directory string

	// Parallel is the number of concurrent workers (default: 5)
	Parallel int

	// MaxDepth is the maximum directory depth to scan (default: 5)
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

// BulkFetchResult contains the results of a bulk fetch operation
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

// RepositoryFetchResult represents the result for a single repository fetch
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

	// FetchedRefs is the number of refs fetched
	FetchedRefs int

	// FetchedObjects is the number of objects fetched
	FetchedObjects int
}

// BulkUpdateOptions configures bulk repository update operations
type BulkUpdateOptions struct {
	// Directory is the root directory to scan for repositories
	Directory string

	// Parallel is the number of concurrent workers (default: 5)
	Parallel int

	// MaxDepth is the maximum directory depth to scan (default: 5)
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

// BulkUpdateResult contains the results of a bulk update operation
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

// RepositoryUpdateResult represents the result for a single repository
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

// BulkUpdate scans for repositories and updates them in parallel
func (c *client) BulkUpdate(ctx context.Context, opts BulkUpdateOptions) (*BulkUpdateResult, error) {
	startTime := time.Now()

	// Validate options
	if opts.Directory == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
		opts.Directory = cwd
	}

	// Set defaults
	if opts.Parallel <= 0 {
		opts.Parallel = 5
	}
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = 5
	}

	// Use logger
	logger := opts.Logger
	if logger == nil {
		logger = &noopLogger{}
	}

	// Resolve absolute path
	absPath, err := filepath.Abs(opts.Directory)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path: %w", err)
	}
	opts.Directory = absPath

	logger.Info("scanning for repositories", "directory", opts.Directory, "maxDepth", opts.MaxDepth)

	// Scan for repositories
	repos, err := c.scanRepositoriesWithConfig(ctx, opts.Directory, opts.MaxDepth, logger, walkDirectoryConfig{
		includeSubmodules: opts.IncludeSubmodules,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan repositories: %w", err)
	}

	logger.Info("scan complete", "found", len(repos))

	// Filter repositories
	filteredRepos, err := filterRepositories(repos, opts.IncludePattern, opts.ExcludePattern, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to filter repositories: %w", err)
	}

	if len(filteredRepos) < len(repos) {
		logger.Info("filtered repositories", "total", len(repos), "selected", len(filteredRepos))
	}

	if len(filteredRepos) == 0 {
		return &BulkUpdateResult{
			TotalScanned:   len(repos),
			TotalProcessed: 0,
			Repositories:   []RepositoryUpdateResult{},
			Duration:       time.Since(startTime),
			Summary:        map[string]int{},
		}, nil
	}

	// Process repositories in parallel
	results, err := c.processRepositories(ctx, opts.Directory, filteredRepos, opts, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to process repositories: %w", err)
	}

	// Calculate summary
	summary := calculateSummary(results)

	return &BulkUpdateResult{
		TotalScanned:   len(repos),
		TotalProcessed: len(filteredRepos),
		Repositories:   results,
		Duration:       time.Since(startTime),
		Summary:        summary,
	}, nil
}

// scanRepositories recursively finds Git repositories
func (c *client) scanRepositories(ctx context.Context, dir string, maxDepth int, logger Logger) ([]string, error) {
	return c.scanRepositoriesWithConfig(ctx, dir, maxDepth, logger, walkDirectoryConfig{
		includeSubmodules: false,
	})
}

// scanRepositoriesWithConfig recursively finds Git repositories with configuration
func (c *client) scanRepositoriesWithConfig(ctx context.Context, dir string, maxDepth int, logger Logger, config walkDirectoryConfig) ([]string, error) {
	var repos []string
	var mu sync.Mutex

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

// walkDirectoryConfig holds configuration for walkDirectory
type walkDirectoryConfig struct {
	includeSubmodules bool
}

// walkDirectory recursively walks directories to find Git repositories
func (c *client) walkDirectory(ctx context.Context, dir string, depth, maxDepth int, repos *[]string, mu *sync.Mutex, logger Logger) error {
	return c.walkDirectoryWithConfig(ctx, dir, depth, maxDepth, repos, mu, logger, walkDirectoryConfig{
		includeSubmodules: false,
	})
}

// walkDirectoryWithConfig walks directories with configuration options to find git repositories.
//
// Scanning Strategy:
//   - Always scans the root directory (depth 0) and its children
//   - For repositories found at depth > 0:
//     * If it's a submodule and config.includeSubmodules==false: skip its children
//     * If it's a submodule and config.includeSubmodules==true: scan its children
//     * If it's an independent nested repo: scan its children to find more nested repos
//   - Respects maxDepth limit to prevent infinite recursion
//   - Skips hidden directories (.git, node_modules, etc.) to improve performance
//
// This strategy ensures:
//   - Independent nested repositories are always discovered
//   - Submodules are only scanned when explicitly requested
//   - Deeply nested repository structures are properly handled
//
func (c *client) walkDirectoryWithConfig(ctx context.Context, dir string, depth, maxDepth int, repos *[]string, mu *sync.Mutex, logger Logger, config walkDirectoryConfig) error {
	// Check depth limit
	if depth > maxDepth {
		return nil
	}

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

		if isSubmodule(dir) {
			if !config.includeSubmodules {
				// Skip submodule and its children
				logger.Debug("skipping submodule", "path", dir)
				return nil
			}
			// Include submodules, continue scanning
			logger.Debug("including submodule", "path", dir)
		} else if depth > 0 {
			// This is an independent nested repository at depth > 0
			// Continue scanning its children to find more nested repos
			logger.Debug("found independent nested repository, continuing scan", "path", dir)
		} else {
			// This is the root directory (depth 0), always scan its children
			logger.Debug("scanning children of root repository", "path", dir)
		}
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

// shouldIgnoreDirectory checks if a directory should be skipped
func shouldIgnoreDirectory(name string) bool {
	// Skip hidden directories
	if len(name) > 0 && name[0] == '.' {
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

	for _, pattern := range ignorePatterns {
		if name == pattern {
			return true
		}
	}

	return false
}

// filterRepositories filters repositories based on include/exclude patterns
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

// processRepositories processes repositories in parallel
func (c *client) processRepositories(ctx context.Context, rootDir string, repos []string, opts BulkUpdateOptions, logger Logger) ([]RepositoryUpdateResult, error) {
	results := make([]RepositoryUpdateResult, len(repos))
	var mu sync.Mutex

	// Create error group with concurrency limit
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(opts.Parallel)

	for i, repoPath := range repos {
		i, repoPath := i, repoPath // capture loop variables

		g.Go(func() error {
			// Call progress callback
			if opts.ProgressCallback != nil {
				opts.ProgressCallback(i+1, len(repos), repoPath)
			}

			result := c.processRepository(gctx, rootDir, repoPath, opts, logger)

			mu.Lock()
			results[i] = result
			mu.Unlock()

			return nil // Don't fail entire operation on single repo error
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

// processRepository processes a single repository
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
		result.Status = "error"
		result.Message = "Failed to open repository"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	// Get repository info
	info, err := c.GetInfo(ctx, repo)
	if err != nil {
		result.Status = "error"
		result.Message = "Failed to get repository info"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	result.Branch = info.Branch
	result.RemoteURL = info.RemoteURL
	result.CommitsBehind = info.BehindBy
	result.CommitsAhead = info.AheadBy

	// Get status
	status, err := c.GetStatus(ctx, repo)
	if err != nil {
		result.Status = "error"
		result.Message = "Failed to get repository status"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	result.HasUncommittedChanges = !status.IsClean

	// Check if update is safe
	if result.HasUncommittedChanges {
		result.Status = "skipped"
		result.Message = "Has uncommitted changes - manual intervention required"
		result.Duration = time.Since(startTime)
		return result
	}

	if info.BehindBy == 0 {
		result.Status = "up-to-date"
		result.Message = "Already up to date"
		result.Duration = time.Since(startTime)
		return result
	}

	if info.Upstream == "" {
		result.Status = "no-upstream"
		result.Message = "No upstream branch configured"
		result.Duration = time.Since(startTime)
		return result
	}

	// Dry run - don't actually update
	if opts.DryRun {
		result.Status = "would-update"
		result.Message = fmt.Sprintf("Would pull %d commits", info.BehindBy)
		result.Duration = time.Since(startTime)
		return result
	}

	// Perform pull --rebase
	pullResult, err := c.executor.Run(ctx, repoPath, "pull", "--rebase")
	if err != nil || pullResult.ExitCode != 0 {
		result.Status = "error"
		result.Message = "Pull failed"
		if err != nil {
			result.Error = err
		} else {
			result.Error = fmt.Errorf("pull exited with code %d: %s", pullResult.ExitCode, pullResult.Error)
		}
		result.Duration = time.Since(startTime)
		return result
	}

	result.Status = "updated"
	result.Message = fmt.Sprintf("Successfully pulled %d commits", info.BehindBy)
	result.Duration = time.Since(startTime)

	logger.Info("repository updated", "path", result.RelativePath, "commits", info.BehindBy)

	return result
}

// getRelativePath returns the relative path from root to target
func getRelativePath(root, target string) string {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return target
	}
	return rel
}

// calculateSummary creates a summary of results by status
func calculateSummary(results []RepositoryUpdateResult) map[string]int {
	summary := make(map[string]int)

	for _, result := range results {
		summary[result.Status]++
	}

	return summary
}

// BulkFetch scans for repositories and fetches them in parallel
func (c *client) BulkFetch(ctx context.Context, opts BulkFetchOptions) (*BulkFetchResult, error) {
	startTime := time.Now()

	// Validate options
	if opts.Directory == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
		opts.Directory = cwd
	}

	// Set defaults
	if opts.Parallel <= 0 {
		opts.Parallel = 5
	}
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = 5
	}

	// Use logger
	logger := opts.Logger
	if logger == nil {
		logger = &noopLogger{}
	}

	// Resolve absolute path
	absPath, err := filepath.Abs(opts.Directory)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path: %w", err)
	}
	opts.Directory = absPath

	logger.Info("scanning for repositories", "directory", opts.Directory, "maxDepth", opts.MaxDepth)

	// Scan for repositories
	repos, err := c.scanRepositoriesWithConfig(ctx, opts.Directory, opts.MaxDepth, logger, walkDirectoryConfig{
		includeSubmodules: opts.IncludeSubmodules,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan repositories: %w", err)
	}

	logger.Info("scan complete", "found", len(repos))

	// Filter repositories
	filteredRepos, err := filterRepositories(repos, opts.IncludePattern, opts.ExcludePattern, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to filter repositories: %w", err)
	}

	if len(filteredRepos) < len(repos) {
		logger.Info("filtered repositories", "total", len(repos), "selected", len(filteredRepos))
	}

	if len(filteredRepos) == 0 {
		return &BulkFetchResult{
			TotalScanned:   len(repos),
			TotalProcessed: 0,
			Repositories:   []RepositoryFetchResult{},
			Duration:       time.Since(startTime),
			Summary:        map[string]int{},
		}, nil
	}

	// Process repositories in parallel
	results, err := c.processFetchRepositories(ctx, opts.Directory, filteredRepos, opts, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to process repositories: %w", err)
	}

	// Calculate summary
	summary := calculateFetchSummary(results)

	return &BulkFetchResult{
		TotalScanned:   len(repos),
		TotalProcessed: len(filteredRepos),
		Repositories:   results,
		Duration:       time.Since(startTime),
		Summary:        summary,
	}, nil
}

// processFetchRepositories processes repositories in parallel for fetch
func (c *client) processFetchRepositories(ctx context.Context, rootDir string, repos []string, opts BulkFetchOptions, logger Logger) ([]RepositoryFetchResult, error) {
	results := make([]RepositoryFetchResult, len(repos))
	var mu sync.Mutex

	// Create error group with concurrency limit
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(opts.Parallel)

	for i, repoPath := range repos {
		i, repoPath := i, repoPath // capture loop variables

		g.Go(func() error {
			// Call progress callback
			if opts.ProgressCallback != nil {
				opts.ProgressCallback(i+1, len(repos), repoPath)
			}

			result := c.processFetchRepository(gctx, rootDir, repoPath, opts, logger)

			mu.Lock()
			results[i] = result
			mu.Unlock()

			return nil // Don't fail entire operation on single repo error
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

// processFetchRepository processes a single repository fetch
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
		result.Status = "error"
		result.Message = "Failed to open repository"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	// Get repository info
	info, err := c.GetInfo(ctx, repo)
	if err != nil {
		result.Status = "error"
		result.Message = "Failed to get repository info"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	result.Branch = info.Branch
	result.RemoteURL = info.RemoteURL

	// Check if repository has remote
	if info.RemoteURL == "" {
		result.Status = "no-remote"
		result.Message = "No remote configured"
		result.Duration = time.Since(startTime)
		return result
	}

	// Dry run - don't actually fetch
	if opts.DryRun {
		result.Status = "would-fetch"
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

	// Perform fetch
	fetchResult, err := c.executor.Run(ctx, repoPath, fetchArgs...)
	if err != nil || fetchResult.ExitCode != 0 {
		result.Status = "error"
		result.Message = "Fetch failed"
		if err != nil {
			result.Error = err
		} else {
			result.Error = fmt.Errorf("fetch exited with code %d: %s", fetchResult.ExitCode, fetchResult.Error)
		}
		result.Duration = time.Since(startTime)
		return result
	}

	result.Status = "success"
	result.Message = "Successfully fetched from remote"
	result.Duration = time.Since(startTime)

	logger.Info("repository fetched", "path", result.RelativePath)

	return result
}

// calculateFetchSummary creates a summary of fetch results by status
func calculateFetchSummary(results []RepositoryFetchResult) map[string]int {
	summary := make(map[string]int)

	for _, result := range results {
		summary[result.Status]++
	}

	return summary
}
