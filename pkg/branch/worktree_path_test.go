// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package branch

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// `git worktree list` reports paths with symlinks resolved. Get is handed paths
// that have not been, because they come from a flag, a config file, or a shell
// completion. Every test here drives that mismatch through an explicit symlink so
// it reproduces on any platform, not only on macOS where /var happens to be one.

// symlinkedWorktree creates a repository and a worktree reached through a
// symlink, returning the symlinked path and the resolved path git will report.
func symlinkedWorktree(t *testing.T) (mgr WorktreeManager, repo *repository.Repository, linkPath, realPath string) {
	t.Helper()

	realParent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	linkParent := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(realParent, linkParent); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	mgr = NewWorktreeManager()
	repo = &repository.Repository{Path: testutil.TempGitRepoWithCommit(t)}
	linkPath = filepath.Join(linkParent, "wt")
	realPath = filepath.Join(realParent, "wt")

	if _, err := mgr.Add(context.Background(), repo, AddOptions{
		Path:         linkPath,
		Branch:       "feature",
		CreateBranch: true,
	}); err != nil {
		t.Fatalf("Add() through a symlinked path error = %v", err)
	}

	return mgr, repo, linkPath, realPath
}

// TestWorktreeManager_AddThroughSymlinkedPath is the reproduction that started
// this: Add creates the worktree and then calls Get to describe it, so a Get that
// cannot match the path turns a completed creation into a reported failure — with
// the worktree left on disk.
func TestWorktreeManager_AddThroughSymlinkedPath(t *testing.T) {
	_, _, linkPath, realPath := symlinkedWorktree(t)

	if _, err := os.Stat(realPath); err != nil {
		t.Fatalf("worktree missing at resolved path %s: %v", realPath, err)
	}

	if _, err := os.Stat(linkPath); err != nil {
		t.Fatalf("worktree unreachable through symlink %s: %v", linkPath, err)
	}
}

// TestWorktreeManager_GetMatchesEitherSpelling pins the symmetry: the caller may
// hold either spelling, and neither is more correct than the other.
func TestWorktreeManager_GetMatchesEitherSpelling(t *testing.T) {
	mgr, repo, linkPath, realPath := symlinkedWorktree(t)
	ctx := context.Background()

	for name, path := range map[string]string{
		"symlinked": linkPath,
		"resolved":  realPath,
	} {
		t.Run(name, func(t *testing.T) {
			wt, err := mgr.Get(ctx, repo, path)
			if err != nil {
				t.Fatalf("Get(%s) error = %v, want the worktree", path, err)
			}

			if wt.Branch != "feature" {
				t.Errorf("Get(%s).Branch = %q, want %q", path, wt.Branch, "feature")
			}

			exists, err := mgr.Exists(ctx, repo, path)
			if err != nil || !exists {
				t.Errorf("Exists(%s) = %v, %v; want true, nil", path, exists, err)
			}
		})
	}
}

// TestWorktreeManager_RemoveThroughSymlinkedPath covers the other direction: a
// worktree that cannot be found cannot be removed, which leaves the CLI with no
// way to undo what Add did.
func TestWorktreeManager_RemoveThroughSymlinkedPath(t *testing.T) {
	mgr, repo, linkPath, realPath := symlinkedWorktree(t)

	if err := mgr.Remove(context.Background(), repo, RemoveOptions{Path: linkPath}); err != nil {
		t.Fatalf("Remove(%s) error = %v, want nil", linkPath, err)
	}

	if _, err := os.Stat(realPath); !os.IsNotExist(err) {
		t.Errorf("worktree still present at %s after Remove", realPath)
	}
}

// TestWorktreeManager_ExistsOnAbsentPath guards the fallback: EvalSymlinks fails
// on a path that is not there, and that failure must stay an ordinary "no" rather
// than becoming an error. Exists is built on Get, so this is Get's contract too.
func TestWorktreeManager_ExistsOnAbsentPath(t *testing.T) {
	mgr := NewWorktreeManager()
	repo := &repository.Repository{Path: testutil.TempGitRepoWithCommit(t)}

	absent := filepath.Join(t.TempDir(), "never-created")

	exists, err := mgr.Exists(context.Background(), repo, absent)
	if err != nil {
		t.Fatalf("Exists(absent) error = %v, want nil", err)
	}

	if exists {
		t.Error("Exists(absent) = true, want false")
	}
}
