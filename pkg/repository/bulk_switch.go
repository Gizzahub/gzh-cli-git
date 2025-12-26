package repository

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// BulkSwitch scans for repositories and switches their branches in parallel
func (c *client) BulkSwitch(ctx context.Context, opts BulkSwitchOptions) (*BulkSwitchResult, error) {
	startTime := time.Now()

	// Validate required options
	if opts.Branch == "" {
		return nil, fmt.Errorf("branch name is required")
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
		return &BulkSwitchResult{
			TotalScanned:   totalScanned,
			TotalProcessed: 0,
			Repositories:   []RepositorySwitchResult{},
			Duration:       time.Since(startTime),
			Summary:        map[string]int{},
			TargetBranch:   opts.Branch,
		}, nil
	}

	// Process repositories in parallel
	results, err := c.processSwitchRepositories(ctx, opts.Directory, filteredRepos, opts, common.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to process repositories: %w", err)
	}

	// Calculate summary
	summary := calculateSwitchSummary(results)

	return &BulkSwitchResult{
		TotalScanned:   totalScanned,
		TotalProcessed: len(filteredRepos),
		Repositories:   results,
		Duration:       time.Since(startTime),
		Summary:        summary,
		TargetBranch:   opts.Branch,
	}, nil
}

// processSwitchRepositories processes repositories in parallel for branch switching
func (c *client) processSwitchRepositories(ctx context.Context, rootDir string, repos []string, opts BulkSwitchOptions, logger Logger) ([]RepositorySwitchResult, error) {
	results := make([]RepositorySwitchResult, len(repos))
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

			result := c.processSwitchRepository(gctx, rootDir, repoPath, opts, logger)

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

// processSwitchRepository processes a single repository branch switch
func (c *client) processSwitchRepository(ctx context.Context, rootDir, repoPath string, opts BulkSwitchOptions, logger Logger) RepositorySwitchResult {
	startTime := time.Now()

	result := RepositorySwitchResult{
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

	result.PreviousBranch = info.Branch
	result.CurrentBranch = info.Branch
	result.RemoteURL = info.RemoteURL

	// Check if already on target branch
	if info.Branch == opts.Branch {
		result.Status = StatusAlreadyOnBranch
		result.Message = fmt.Sprintf("Already on branch '%s'", opts.Branch)
		result.Duration = time.Since(startTime)
		logger.Info("already on target branch", "path", result.RelativePath, "branch", opts.Branch)
		return result
	}

	// Get repository status
	status, err := c.GetStatus(ctx, repo)
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to get repository status"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	// Check for uncommitted changes
	if !status.IsClean && !opts.Force {
		result.Status = StatusDirty
		result.HasUncommittedChanges = true
		uncommittedCount := len(status.ModifiedFiles) + len(status.StagedFiles)
		result.Message = fmt.Sprintf("Has uncommitted changes (%d files) - skipping", uncommittedCount)
		result.Duration = time.Since(startTime)
		logger.Warn("skipping dirty repository", "path", result.RelativePath, "files", uncommittedCount)
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

	// Handle repositories with ongoing rebase
	if repoState.RebaseInProgress {
		result.Status = StatusRebaseInProgress
		result.Message = "Repository has rebase in progress - skipping"
		result.Error = fmt.Errorf("rebase in progress")
		result.Duration = time.Since(startTime)
		logger.Warn("rebase in progress", "path", result.RelativePath)
		return result
	}

	// Handle repositories with ongoing merge
	if repoState.MergeInProgress {
		result.Status = StatusMergeInProgress
		result.Message = "Repository has merge in progress - skipping"
		result.Error = fmt.Errorf("merge in progress")
		result.Duration = time.Since(startTime)
		logger.Warn("merge in progress", "path", result.RelativePath)
		return result
	}

	// Check if branch exists locally
	branchExists, err := c.branchExistsLocally(ctx, repoPath, opts.Branch)
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to check branch existence"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	// Check if branch exists on remote (if not local)
	if !branchExists {
		remoteBranchExists, err := c.branchExistsOnRemote(ctx, repoPath, opts.Branch)
		if err != nil {
			// Just log, don't fail - we'll try to create if needed
			logger.Debug("failed to check remote branch", "path", result.RelativePath, "error", err)
		}

		if !remoteBranchExists && !opts.Create {
			result.Status = StatusBranchNotFound
			result.Message = fmt.Sprintf("Branch '%s' not found (use --create to create)", opts.Branch)
			result.Duration = time.Since(startTime)
			logger.Warn("branch not found", "path", result.RelativePath, "branch", opts.Branch)
			return result
		}
	}

	// Dry run - don't actually switch
	if opts.DryRun {
		if branchExists {
			result.Status = StatusWouldSwitch
			result.Message = fmt.Sprintf("Would switch from '%s' to '%s'", info.Branch, opts.Branch)
		} else if opts.Create {
			result.Status = StatusWouldSwitch
			result.Message = fmt.Sprintf("Would create and switch to '%s'", opts.Branch)
		} else {
			result.Status = StatusWouldSwitch
			result.Message = fmt.Sprintf("Would switch to '%s' (tracking remote)", opts.Branch)
		}
		result.Duration = time.Since(startTime)
		return result
	}

	// Perform the switch
	var switchErr error
	var created bool

	if branchExists {
		// Switch to existing local branch
		switchErr = c.switchBranch(ctx, repoPath, opts.Branch)
	} else if opts.Create {
		// Create new branch from current HEAD
		switchErr = c.createAndSwitchBranch(ctx, repoPath, opts.Branch)
		created = switchErr == nil
	} else {
		// Try to checkout remote tracking branch
		switchErr = c.checkoutRemoteTrackingBranch(ctx, repoPath, opts.Branch)
	}

	if switchErr != nil {
		result.Status = StatusError
		result.Message = fmt.Sprintf("Failed to switch to branch '%s'", opts.Branch)
		result.Error = switchErr
		result.Duration = time.Since(startTime)
		logger.Error("switch failed", "path", result.RelativePath, "branch", opts.Branch, "error", switchErr)
		return result
	}

	// Update result
	result.CurrentBranch = opts.Branch
	if created {
		result.Status = StatusBranchCreated
		result.Message = fmt.Sprintf("Created and switched to branch '%s'", opts.Branch)
	} else {
		result.Status = StatusSwitched
		result.Message = fmt.Sprintf("Switched from '%s' to '%s'", info.Branch, opts.Branch)
	}
	result.Duration = time.Since(startTime)

	logger.Info("branch switched", "path", result.RelativePath, "from", info.Branch, "to", opts.Branch)

	return result
}

// branchExistsLocally checks if a branch exists locally
func (c *client) branchExistsLocally(ctx context.Context, repoPath, branch string) (bool, error) {
	result, err := c.executor.Run(ctx, repoPath, "rev-parse", "--verify", fmt.Sprintf("refs/heads/%s", branch))
	if err != nil {
		return false, nil // Branch doesn't exist
	}
	return result.ExitCode == 0, nil
}

// branchExistsOnRemote checks if a branch exists on the remote
func (c *client) branchExistsOnRemote(ctx context.Context, repoPath, branch string) (bool, error) {
	// First try origin
	result, err := c.executor.Run(ctx, repoPath, "rev-parse", "--verify", fmt.Sprintf("refs/remotes/origin/%s", branch))
	if err != nil {
		return false, nil
	}
	return result.ExitCode == 0, nil
}

// switchBranch switches to an existing local branch
func (c *client) switchBranch(ctx context.Context, repoPath, branch string) error {
	result, err := c.executor.Run(ctx, repoPath, "checkout", branch)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("checkout failed: %s", strings.TrimSpace(result.Stderr))
	}
	return nil
}

// createAndSwitchBranch creates a new branch and switches to it
func (c *client) createAndSwitchBranch(ctx context.Context, repoPath, branch string) error {
	result, err := c.executor.Run(ctx, repoPath, "checkout", "-b", branch)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("checkout -b failed: %s", strings.TrimSpace(result.Stderr))
	}
	return nil
}

// checkoutRemoteTrackingBranch checks out a remote tracking branch
func (c *client) checkoutRemoteTrackingBranch(ctx context.Context, repoPath, branch string) error {
	// Try to checkout with tracking
	result, err := c.executor.Run(ctx, repoPath, "checkout", "--track", fmt.Sprintf("origin/%s", branch))
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		// Fallback: try simple checkout (git may auto-detect remote)
		result2, err2 := c.executor.Run(ctx, repoPath, "checkout", branch)
		if err2 != nil {
			return err2
		}
		if result2.ExitCode != 0 {
			return fmt.Errorf("checkout failed: %s", strings.TrimSpace(result2.Stderr))
		}
	}
	return nil
}

// calculateSwitchSummary creates a summary of switch results by status
func calculateSwitchSummary(results []RepositorySwitchResult) map[string]int {
	summary := make(map[string]int)

	for _, result := range results {
		summary[result.Status]++
	}

	return summary
}
