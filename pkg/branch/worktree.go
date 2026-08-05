// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package branch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// WorktreeManager manages Git worktree operations.
type WorktreeManager interface {
	// Add adds a new worktree.
	Add(ctx context.Context, repo *repository.Repository, opts AddOptions) (*Worktree, error)

	// Remove removes a worktree.
	Remove(ctx context.Context, repo *repository.Repository, opts RemoveOptions) error

	// List lists all worktrees.
	List(ctx context.Context, repo *repository.Repository) ([]*Worktree, error)

	// Prune removes orphaned worktree metadata.
	Prune(ctx context.Context, repo *repository.Repository) error

	// Get retrieves a specific worktree by path.
	Get(ctx context.Context, repo *repository.Repository, path string) (*Worktree, error)

	// Exists checks if a worktree exists at the given path.
	Exists(ctx context.Context, repo *repository.Repository, path string) (bool, error)
}

// worktreeManager implements WorktreeManager.
type worktreeManager struct {
	executor *gitcmd.Executor
}

// NewWorktreeManager creates a new WorktreeManager.
func NewWorktreeManager() WorktreeManager {
	return &worktreeManager{
		executor: gitcmd.NewExecutor(),
	}
}

// NewWorktreeManagerWithExecutor creates a new WorktreeManager with custom executor.
func NewWorktreeManagerWithExecutor(executor *gitcmd.Executor) WorktreeManager {
	return &worktreeManager{
		executor: executor,
	}
}

// run executes a git command and turns a non-zero exit into an error.
//
// Every command in this file asks git to do something rather than to answer a
// yes/no question, so a non-zero exit always means the work did not happen: a
// failed `worktree remove` would otherwise be reported as a removal, a failed
// `worktree list` as a repository with no worktrees, and a failed `status` as a
// worktree with nothing to lose. See runGit for why the distinction exists.
func (w *worktreeManager) run(ctx context.Context, dir string, args ...string) (*gitcmd.Result, error) {
	return runGit(ctx, w.executor, dir, args...)
}

// Add adds a new worktree.
func (w *worktreeManager) Add(ctx context.Context, repo *repository.Repository, opts AddOptions) (*Worktree, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository cannot be nil")
	}

	if opts.Path == "" {
		return nil, fmt.Errorf("worktree path is required")
	}

	if opts.Branch == "" && !opts.Detach {
		return nil, fmt.Errorf("branch name is required (or use --detach)")
	}

	// Validate path
	if err := validateWorktreePath(opts.Path); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidPath, err)
	}

	// Check if path already exists
	exists, err := w.pathExists(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to check path existence: %w", err)
	}

	if exists && !opts.Force {
		return nil, fmt.Errorf("%w: %s (use --force to overwrite)", ErrWorktreeExists, opts.Path)
	}

	// Check if branch is already checked out in another worktree
	if opts.Branch != "" && !opts.CreateBranch && !opts.Detach {
		inUse, err := w.isBranchInUse(ctx, repo, opts.Branch)
		if err != nil {
			return nil, fmt.Errorf("failed to check branch usage: %w", err)
		}

		if inUse {
			return nil, fmt.Errorf("%w: %s", ErrBranchInUse, opts.Branch)
		}
	}

	// Build git worktree add command
	args := []string{"worktree", "add"}

	if opts.Force {
		args = append(args, "--force")
	}

	if opts.Detach {
		args = append(args, "--detach")
	}

	if opts.CreateBranch {
		args = append(args, "-b", opts.Branch)
	}

	args = append(args, opts.Path)

	if !opts.CreateBranch && opts.Branch != "" {
		args = append(args, opts.Branch)
	} else if opts.Checkout != "" {
		args = append(args, opts.Checkout)
	}

	// Add worktree
	if _, err := w.run(ctx, repo.Path, args...); err != nil {
		return nil, fmt.Errorf("failed to add worktree: %w", err)
	}

	// Get worktree info
	worktree, err := w.Get(ctx, repo, opts.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree info: %w", err)
	}

	return worktree, nil
}

// Remove removes a worktree.
func (w *worktreeManager) Remove(ctx context.Context, repo *repository.Repository, opts RemoveOptions) error {
	if repo == nil {
		return fmt.Errorf("repository cannot be nil")
	}

	if opts.Path == "" {
		return fmt.Errorf("worktree path is required")
	}

	// Get worktree info
	worktree, err := w.Get(ctx, repo, opts.Path)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("%w: %s", ErrWorktreeNotFound, opts.Path)
		}
		return fmt.Errorf("failed to get worktree info: %w", err)
	}

	// Safety checks
	if !opts.Force {
		// Cannot remove main worktree
		if worktree.IsMain {
			return fmt.Errorf("%w: %s", ErrWorktreeMain, opts.Path)
		}

		// Check for uncommitted changes. A check that could not run is not a
		// check that passed — this is the guard standing between an unforced
		// remove and a worktree holding work that exists nowhere else.
		dirty, dirtyErr := w.isWorktreeDirty(ctx, opts.Path)
		if dirtyErr != nil {
			return fmt.Errorf("cannot verify %s has no uncommitted changes: %w (use --force to remove anyway)", opts.Path, dirtyErr)
		}

		if dirty {
			return fmt.Errorf("%w: %s (use --force to remove anyway)", ErrWorktreeDirty, opts.Path)
		}

		// Check if locked
		if worktree.IsLocked {
			return fmt.Errorf("%w: %s (use --force to remove anyway)", ErrWorktreeLocked, opts.Path)
		}
	}

	// Build git worktree remove command
	args := []string{"worktree", "remove"}

	if opts.Force {
		args = append(args, "--force")
	}

	args = append(args, opts.Path)

	// Remove worktree
	if _, err := w.run(ctx, repo.Path, args...); err != nil {
		return fmt.Errorf("failed to remove worktree: %w", err)
	}

	return nil
}

// List lists all worktrees.
func (w *worktreeManager) List(ctx context.Context, repo *repository.Repository) ([]*Worktree, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository cannot be nil")
	}

	// Run git worktree list --porcelain
	result, err := w.run(ctx, repo.Path, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	// Parse output
	worktrees, err := w.parseWorktreeList(result.Stdout)
	if err != nil {
		return nil, fmt.Errorf("failed to parse worktree list: %w", err)
	}

	return worktrees, nil
}

// Prune removes orphaned worktree metadata.
func (w *worktreeManager) Prune(ctx context.Context, repo *repository.Repository) error {
	if repo == nil {
		return fmt.Errorf("repository cannot be nil")
	}

	// Run git worktree prune
	if _, err := w.run(ctx, repo.Path, "worktree", "prune"); err != nil {
		return fmt.Errorf("failed to prune worktrees: %w", err)
	}

	return nil
}

// Get retrieves a specific worktree by path.
func (w *worktreeManager) Get(ctx context.Context, repo *repository.Repository, path string) (*Worktree, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository cannot be nil")
	}

	if path == "" {
		return nil, fmt.Errorf("worktree path is required")
	}

	// Normalize path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// List all worktrees
	worktrees, err := w.List(ctx, repo)
	if err != nil {
		return nil, err
	}

	// Find matching worktree
	for _, wt := range worktrees {
		if samePath(absPath, wt.Path) {
			return wt, nil
		}
	}

	return nil, fmt.Errorf("worktree not found: %s", path)
}

// samePath reports whether two paths name the same directory.
//
// Comparing the two spellings is not enough. filepath.Abs is string arithmetic —
// it prepends the working directory and folds "." and ".." without ever touching
// the filesystem — while `git worktree list` reports paths with every symlink
// already resolved. On macOS /var is a symlink to /private/var, so a worktree the
// caller knows as /var/folders/… is reported as /private/var/folders/…, and an
// Abs-only comparison never matches it.
func samePath(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)

	if errA != nil || errB != nil {
		return false
	}

	// Identical spellings need no filesystem walk, and this is the only branch
	// that can match a worktree whose directory has been deleted — those are
	// still listed by git, and removing one is a thing callers do.
	if absA == absB {
		return true
	}

	resolvedA, errA := filepath.EvalSymlinks(absA)
	resolvedB, errB := filepath.EvalSymlinks(absB)

	// EvalSymlinks fails on a path that does not exist, which is an ordinary
	// question here — Exists is built on Get. The spelling comparison above has
	// already given the only answer available.
	if errA != nil || errB != nil {
		return false
	}

	return resolvedA == resolvedB
}

// Exists checks if a worktree exists at the given path.
func (w *worktreeManager) Exists(ctx context.Context, repo *repository.Repository, path string) (bool, error) {
	if repo == nil {
		return false, fmt.Errorf("repository cannot be nil")
	}

	if path == "" {
		return false, fmt.Errorf("worktree path is required")
	}

	_, err := w.Get(ctx, repo, path)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// parseWorktreeList parses git worktree list --porcelain output.
func (w *worktreeManager) parseWorktreeList(output string) ([]*Worktree, error) { //nolint:unparam // error return kept for future use and consistent interface
	lines := strings.Split(strings.TrimSpace(output), "\n")
	worktrees := make([]*Worktree, 0)

	var current *Worktree
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "" {
			// Empty line separates worktrees
			if current != nil {
				worktrees = append(worktrees, current)
				current = nil
			}
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			// Start of new worktree
			if current != nil {
				worktrees = append(worktrees, current)
			}
			current = &Worktree{
				Path: strings.TrimPrefix(line, "worktree "),
			}
		} else if current != nil {
			// Parse worktree attributes
			switch {
			case strings.HasPrefix(line, "HEAD "):
				current.Ref = strings.TrimPrefix(line, "HEAD ")
			case strings.HasPrefix(line, "branch "):
				current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
			case line == "bare":
				current.IsBare = true
			case line == "detached":
				current.IsDetached = true
			case strings.HasPrefix(line, "locked"):
				current.IsLocked = true
			case strings.HasPrefix(line, "prunable"):
				current.IsPrunable = true
			}
		}
	}

	// Add last worktree
	if current != nil {
		worktrees = append(worktrees, current)
	}

	// Mark main worktree (first one)
	if len(worktrees) > 0 {
		worktrees[0].IsMain = true
	}

	return worktrees, nil
}

// isBranchInUse checks if a branch is checked out in any worktree.
func (w *worktreeManager) isBranchInUse(ctx context.Context, repo *repository.Repository, branch string) (bool, error) {
	worktrees, err := w.List(ctx, repo)
	if err != nil {
		return false, err
	}

	for _, wt := range worktrees {
		if wt.Branch == branch {
			return true, nil
		}
	}

	return false, nil
}

// isWorktreeDirty checks if a worktree has uncommitted changes.
//
// Plain --porcelain is enough here: the answer is whether git reported anything
// at all, so neither the C-quoting of unusual paths nor the collapsing of
// untracked directories can change it.
func (w *worktreeManager) isWorktreeDirty(ctx context.Context, path string) (bool, error) {
	// Run git status in worktree
	result, err := w.run(ctx, path, "status", "--porcelain")
	if err != nil {
		return false, err
	}

	// If output is not empty, there are changes
	return strings.TrimSpace(result.Stdout) != "", nil
}

// pathExists checks if a path exists.
func (w *worktreeManager) pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// validateWorktreePath validates worktree path.
func validateWorktreePath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	// Path must be absolute or relative
	if !filepath.IsAbs(path) && !strings.HasPrefix(path, ".") && !strings.HasPrefix(path, "~") {
		// Assume relative to current directory
		path = "./" + path
	}

	// Cannot contain null bytes
	if strings.Contains(path, "\x00") {
		return fmt.Errorf("path cannot contain null bytes")
	}

	return nil
}
