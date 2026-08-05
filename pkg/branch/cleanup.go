// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package branch

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// CleanupService analyzes and cleans up branches.
type CleanupService interface {
	// Analyze analyzes branches for cleanup.
	Analyze(ctx context.Context, repo *repository.Repository, opts AnalyzeOptions) (*CleanupReport, error)

	// Execute performs cleanup based on report. The result names which branches
	// were deleted and which were not; a non-nil error means the run never
	// started, not that nothing was deleted.
	Execute(ctx context.Context, repo *repository.Repository, report *CleanupReport, opts ExecuteOptions) (*ExecuteResult, error)
}

// cleanupService implements CleanupService.
type cleanupService struct {
	executor      *gitcmd.Executor
	branchManager BranchManager
}

// NewCleanupService creates a new CleanupService.
func NewCleanupService() CleanupService {
	return &cleanupService{
		executor:      gitcmd.NewExecutor(),
		branchManager: NewManager(),
	}
}

// NewCleanupServiceWithDeps creates a new CleanupService with custom dependencies.
func NewCleanupServiceWithDeps(executor *gitcmd.Executor, branchManager BranchManager) CleanupService {
	return &cleanupService{
		executor:      executor,
		branchManager: branchManager,
	}
}

// run executes a git command and turns a non-zero exit into an error.
//
// Every git read in this file feeds a decision about deleting a branch, and each
// one answers "no evidence" the same way it answers "the read failed" — so a
// failure that stays silent becomes a verdict.
func (c *cleanupService) run(ctx context.Context, dir string, args ...string) (*gitcmd.Result, error) {
	return runGit(ctx, c.executor, dir, args...)
}

// Analyze analyzes branches for cleanup.
func (c *cleanupService) Analyze(ctx context.Context, repo *repository.Repository, opts AnalyzeOptions) (*CleanupReport, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository cannot be nil")
	}

	// Set defaults
	if opts.StaleThreshold == 0 {
		opts.StaleThreshold = 30 * 24 * time.Hour // 30 days
	}

	if opts.BaseBranch == "" {
		// Try to detect base branch
		baseBranch, err := c.detectBaseBranch(ctx, repo)
		if err == nil {
			opts.BaseBranch = baseBranch
		} else {
			opts.BaseBranch = "main" // Default fallback
		}
	}

	// Get all branches
	branches, err := c.branchManager.List(ctx, repo, ListOptions{
		All: opts.IncludeRemote,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}

	report := &CleanupReport{
		Merged:    make([]*Branch, 0),
		Stale:     make([]*Branch, 0),
		Orphaned:  make([]*Branch, 0),
		Protected: make([]*Branch, 0),
		Total:     len(branches),
	}

	// Ask git which branches it marks [gone] before the loop: the answer comes
	// from one for-each-ref over all of refs/heads, not from a per-branch query.
	gone := map[string]bool{}

	if opts.IncludeGone {
		gone, err = c.findGoneBranches(ctx, repo)
		if err != nil {
			return nil, fmt.Errorf("failed to find gone branches: %w", err)
		}
	}

	// Analyze each branch
	for _, branch := range branches {
		// Skip current branch
		if branch.IsHead {
			continue
		}

		// Check if protected
		if c.isProtectedBranch(branch.Name, opts.Exclude) {
			report.Protected = append(report.Protected, branch)
			continue
		}

		// Check if merged
		if opts.IncludeMerged {
			if merged, err := c.isBranchMerged(ctx, repo, branch.Name, opts.BaseBranch); err == nil && merged {
				report.Merged = append(report.Merged, branch)
				continue
			}
		}

		// Check if stale
		if opts.IncludeStale {
			if stale, err := c.isBranchStale(ctx, repo, branch.Name, opts.StaleThreshold); err == nil && stale {
				report.Stale = append(report.Stale, branch)
				continue
			}
		}

		// Check if the upstream this branch tracked is gone
		if gone[branch.Name] {
			report.Orphaned = append(report.Orphaned, branch)
		}
	}

	return report, nil
}

// Execute performs cleanup based on report.
//
// A failure on one branch does not stop the others — that policy is deliberate
// and unchanged. What it returns is the account of which ones succeeded, because
// without it the caller has no way to tell a run that deleted everything from one
// that deleted nothing.
func (c *cleanupService) Execute(ctx context.Context, repo *repository.Repository, report *CleanupReport, opts ExecuteOptions) (*ExecuteResult, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository cannot be nil")
	}

	if report == nil {
		return nil, fmt.Errorf("cleanup report cannot be nil")
	}

	// Collect all branches to delete
	toDelete := make([]*Branch, 0)
	toDelete = append(toDelete, report.Merged...)
	toDelete = append(toDelete, report.Stale...)
	toDelete = append(toDelete, report.Orphaned...)

	// Filter out excluded branches
	if len(opts.Exclude) > 0 {
		filtered := make([]*Branch, 0)
		for _, branch := range toDelete {
			if !c.isProtectedBranch(branch.Name, opts.Exclude) {
				filtered = append(filtered, branch)
			}
		}
		toDelete = filtered
	}

	result := &ExecuteResult{}

	// Dry run - just return
	if opts.DryRun {
		return result, nil
	}

	// Delete branches
	for _, branch := range toDelete {
		deleteOpts := DeleteOptions{
			Name:    branch.Name,
			Force:   opts.Force,
			Remote:  opts.Remote && branch.IsRemote,
			Confirm: opts.Confirm,
		}

		// Carry the failure rather than dropping it. Continuing past a branch that
		// could not be deleted is the right policy; doing so silently meant the
		// caller counted the report and announced that many deletions.
		if err := c.branchManager.Delete(ctx, repo, deleteOpts); err != nil {
			result.Failed = append(result.Failed, DeleteFailure{Branch: branch.Name, Err: err})
			continue
		}

		result.Deleted = append(result.Deleted, branch.Name)
	}

	return result, nil
}

// detectBaseBranch detects the main/master branch.
func (c *cleanupService) detectBaseBranch(ctx context.Context, repo *repository.Repository) (string, error) {
	// Try common base branches in order
	candidates := []string{"main", "master", "develop", "development"}

	for _, branch := range candidates {
		exists, err := c.branchManager.Exists(ctx, repo, branch)
		if err == nil && exists {
			return branch, nil
		}
	}

	return "", fmt.Errorf("could not detect base branch")
}

// isBranchMerged checks if a branch is fully merged into base.
func (c *cleanupService) isBranchMerged(ctx context.Context, repo *repository.Repository, branch, base string) (bool, error) {
	// Run git branch --merged base
	result, err := c.run(ctx, repo.Path, "branch", "--merged", base)
	if err != nil {
		return false, err
	}

	// Parse output
	lines := strings.SplitSeq(strings.TrimSpace(result.Stdout), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "* ")
		if line == branch {
			return true, nil
		}
	}

	return false, nil
}

// isBranchStale checks if a branch has no recent activity.
func (c *cleanupService) isBranchStale(ctx context.Context, repo *repository.Repository, branch string, threshold time.Duration) (bool, error) {
	// Get last commit date
	result, err := c.run(ctx, repo.Path, "log", "-1", "--format=%ct", branch)
	if err != nil {
		return false, err
	}

	// Parse timestamp
	var timestamp int64
	if _, err := fmt.Sscanf(strings.TrimSpace(result.Stdout), "%d", &timestamp); err != nil {
		return false, err
	}

	// Check if older than threshold
	lastCommit := time.Unix(timestamp, 0)
	age := time.Since(lastCommit)

	return age > threshold, nil
}

// findGoneBranches returns the set of local branches whose upstream is gone.
//
// git answers this directly through %(upstream:track), which reads "[gone]" when
// a branch's configured upstream no longer resolves. The marker only appears once
// the stale remote-tracking ref has been pruned, so the prune runs first; it is
// best-effort because a repository with no reachable remote still has a truthful
// answer for every branch whose upstream was already pruned.
//
// This mirrors the bulk path (pkg/repository, getGoneBranches), which is the
// implementation `gz-git cleanup branch <dir> --gone` has always used.
func (c *cleanupService) findGoneBranches(ctx context.Context, repo *repository.Repository) (map[string]bool, error) {
	_, _ = c.executor.RunWithEnv(ctx, repo.Path, repository.NonInteractiveEnv(), "fetch", "--prune") //nolint:errcheck // best-effort; the for-each-ref below is the read that decides

	result, err := c.run(ctx, repo.Path,
		"for-each-ref", "--format=%(refname:short) %(upstream:track)", "refs/heads/")
	if err != nil {
		return nil, err
	}

	gone := make(map[string]bool)

	for line := range strings.SplitSeq(strings.TrimSpace(result.Stdout), "\n") {
		if !strings.Contains(line, "[gone]") {
			continue
		}

		if name, _, ok := strings.Cut(line, " "); ok && name != "" {
			gone[name] = true
		}
	}

	return gone, nil
}

// isProtectedBranch checks if a branch is protected.
func (c *cleanupService) isProtectedBranch(branch string, additionalPatterns []string) bool {
	// Check built-in protected branches
	if IsProtected(branch) {
		return true
	}

	// Check additional patterns
	for _, pattern := range additionalPatterns {
		if matchPattern(branch, pattern) {
			return true
		}
	}

	return false
}

// CountBranches returns the total number of branches in the report.
func (r *CleanupReport) CountBranches() int {
	return len(r.Merged) + len(r.Stale) + len(r.Orphaned)
}

// IsEmpty checks if the report has no branches to clean up.
func (r *CleanupReport) IsEmpty() bool {
	return r.CountBranches() == 0
}

// GetAllBranches returns all branches eligible for cleanup.
func (r *CleanupReport) GetAllBranches() []*Branch {
	all := make([]*Branch, 0, len(r.Merged)+len(r.Stale)+len(r.Orphaned))
	all = append(all, r.Merged...)
	all = append(all, r.Stale...)
	all = append(all, r.Orphaned...)
	return all
}
