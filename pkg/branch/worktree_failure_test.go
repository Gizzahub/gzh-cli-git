// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package branch

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// Every test in this file drives a git command that fails, and asserts the
// failure is reported.
//
// gitcmd.Executor.Run signals a failed git through Result.ExitCode and returns a
// nil error unless the process could not be started, so `if _, err := Run(...);
// err != nil` accepts every git failure as a success. In this file that turned a
// failed `worktree list` into a repository with no worktrees, a failed `worktree
// remove` into a completed removal, a failed `worktree prune` into a completed
// prune, and a failed `status` into a worktree with nothing to lose — each one
// the answer a caller acts on by doing nothing further.

func TestWorktreeManager_ListFailsOnNonRepository(t *testing.T) {
	mgr := NewWorktreeManager()
	repo := &repository.Repository{Path: t.TempDir()}

	worktrees, err := mgr.List(context.Background(), repo)
	if err == nil {
		t.Fatalf("List() on a non-repository returned %d worktrees and a nil error, want failure", len(worktrees))
	}
}

func TestWorktreeManager_PruneFailsOnNonRepository(t *testing.T) {
	mgr := NewWorktreeManager()
	repo := &repository.Repository{Path: t.TempDir()}

	if err := mgr.Prune(context.Background(), repo); err == nil {
		t.Error("Prune() on a non-repository returned nil, want failure")
	}
}

func TestWorktreeManager_RemoveFailsOnNonRepository(t *testing.T) {
	mgr := NewWorktreeManager()
	repo := &repository.Repository{Path: t.TempDir()}

	err := mgr.Remove(context.Background(), repo, RemoveOptions{Path: filepath.Join(t.TempDir(), "wt")})
	if err == nil {
		t.Error("Remove() on a non-repository returned nil, want failure")
	}
}

// TestWorktreeManager_RemoveRefusesDirtyWorktree exercises the guard end to end
// against a real worktree, since it is the reason Remove reads status at all.
func TestWorktreeManager_RemoveRefusesDirtyWorktree(t *testing.T) {
	repoPath := testutil.TempGitRepoWithCommit(t)

	// Resolve symlinks in the temp root: git reports worktree paths resolved,
	// while Get compares them with filepath.Abs, which does not resolve. On macOS
	// /var is a symlink to /private/var, so an unresolved path never matches.
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	worktreePath := filepath.Join(parent, "feature-wt")

	mgr := NewWorktreeManager()
	repo := &repository.Repository{Path: repoPath}
	ctx := context.Background()

	if _, err := mgr.Add(ctx, repo, AddOptions{
		Path:         worktreePath,
		Branch:       "feature",
		CreateBranch: true,
	}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	write(t, worktreePath, "wip.txt", "unsaved work\n")

	err = mgr.Remove(ctx, repo, RemoveOptions{Path: worktreePath})
	if !errors.Is(err, ErrWorktreeDirty) {
		t.Fatalf("Remove() error = %v, want %v", err, ErrWorktreeDirty)
	}

	// --force is the documented way past the guard, so it must still work.
	if err := mgr.Remove(ctx, repo, RemoveOptions{Path: worktreePath, Force: true}); err != nil {
		t.Errorf("Remove() with Force error = %v, want nil", err)
	}
}
