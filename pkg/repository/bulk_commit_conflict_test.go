// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// createConflictedRepo builds a repository stopped mid-merge with one unmerged
// path ("UU conflict.txt") and .git/MERGE_HEAD present.
func createConflictedRepo(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := initGitRepo(path); err != nil {
		t.Fatalf("git init: %v", err)
	}

	git := func(wantSuccess bool, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...) //nolint:noctx // test helper, no context available
		cmd.Dir = path
		out, err := cmd.CombinedOutput()
		if wantSuccess && err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(path, "conflict.txt"), []byte(content), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	write("BASE\n")
	git(true, "add", "conflict.txt")
	git(true, "commit", "-m", "base")
	git(true, "branch", "other")

	write("MAIN\n")
	git(true, "commit", "-am", "main side")

	git(true, "checkout", "other")
	write("OTHER\n")
	git(true, "commit", "-am", "other side")

	git(true, "checkout", "-")
	// Expected to fail: this is what leaves the repo with an unmerged index.
	git(false, "merge", "other")

	if _, err := os.Stat(filepath.Join(path, ".git", "MERGE_HEAD")); err != nil {
		t.Fatalf("fixture did not produce a merge in progress: %v", err)
	}
}

// headParentCount returns the number of parents of HEAD.
func headParentCount(t *testing.T, path string) int {
	t.Helper()
	cmd := exec.Command("git", "rev-list", "--parents", "-n", "1", "HEAD") //nolint:noctx // test helper, no context available
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rev-list: %v\n%s", err, out)
	}
	return len(strings.Fields(string(out))) - 1
}

// TestBulkCommitRefusesUnmergedRepository is the regression guard for the
// irreversible case: before the conflict gate, `commit --yes` staged the
// conflict markers with `git add -A` (which git reads as "resolved"), produced a
// two-parent merge commit, and left the repo reporting clean.
func TestBulkCommitRefusesUnmergedRepository(t *testing.T) {
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "conflicted")
	createConflictedRepo(t, repoPath)

	c := NewClient()
	result, err := c.BulkCommit(context.Background(), BulkCommitOptions{
		Directory: tmpDir,
		Parallel:  1,
		MaxDepth:  1,
		Yes:       true,
	})
	if err != nil {
		t.Fatalf("BulkCommit: %v", err)
	}

	if result.TotalConflicted != 1 {
		t.Errorf("TotalConflicted = %d, want 1", result.TotalConflicted)
	}
	if result.TotalCommitted != 0 {
		t.Errorf("TotalCommitted = %d, want 0 (conflicted repo must not be committed)", result.TotalCommitted)
	}

	if len(result.Repositories) != 1 {
		t.Fatalf("got %d repositories, want 1", len(result.Repositories))
	}
	repo := result.Repositories[0]

	if repo.Status != "conflicted" {
		t.Errorf("Status = %q, want %q", repo.Status, "conflicted")
	}
	if len(repo.ConflictedFiles) != 1 || repo.ConflictedFiles[0] != "conflict.txt" {
		t.Errorf("ConflictedFiles = %v, want [conflict.txt]", repo.ConflictedFiles)
	}
	if repo.Error == nil {
		t.Error("Error is nil; the refusal must carry a reason")
	}

	// The merge must still be resumable by hand.
	if _, err := os.Stat(filepath.Join(repoPath, ".git", "MERGE_HEAD")); err != nil {
		t.Errorf("MERGE_HEAD was removed: %v", err)
	}
	if n := headParentCount(t, repoPath); n != 1 {
		t.Errorf("HEAD has %d parents, want 1 (no merge commit should have been created)", n)
	}
}

// TestBulkCommitAllowConflictedOptIn verifies the escape hatch still works, so
// the guard is a default and not a hard block.
func TestBulkCommitAllowConflictedOptIn(t *testing.T) {
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "conflicted")
	createConflictedRepo(t, repoPath)

	c := NewClient()
	result, err := c.BulkCommit(context.Background(), BulkCommitOptions{
		Directory:       tmpDir,
		Parallel:        1,
		MaxDepth:        1,
		Yes:             true,
		AllowConflicted: true,
		Message:         "chore: force merge",
	})
	if err != nil {
		t.Fatalf("BulkCommit: %v", err)
	}

	if result.TotalConflicted != 0 {
		t.Errorf("TotalConflicted = %d, want 0 when AllowConflicted is set", result.TotalConflicted)
	}
	if result.TotalCommitted != 1 {
		t.Errorf("TotalCommitted = %d, want 1", result.TotalCommitted)
	}
	if n := headParentCount(t, repoPath); n != 2 {
		t.Errorf("HEAD has %d parents, want 2 (opt-in should complete the merge)", n)
	}
}

// TestBulkCommitDryRunReportsConflict ensures the preview names the refusal
// instead of promising "would-commit".
func TestBulkCommitDryRunReportsConflict(t *testing.T) {
	tmpDir := t.TempDir()
	createConflictedRepo(t, filepath.Join(tmpDir, "conflicted"))

	c := NewClient()
	result, err := c.BulkCommit(context.Background(), BulkCommitOptions{
		Directory: tmpDir,
		Parallel:  1,
		MaxDepth:  1,
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("BulkCommit: %v", err)
	}

	if len(result.Repositories) != 1 {
		t.Fatalf("got %d repositories, want 1", len(result.Repositories))
	}
	if got := result.Repositories[0].Status; got != "conflicted" {
		t.Errorf("dry-run Status = %q, want %q", got, "conflicted")
	}
	if result.Summary["would-commit"] != 0 {
		t.Errorf("summary reports %d would-commit, want 0", result.Summary["would-commit"])
	}
}

func TestIsUnmergedCode(t *testing.T) {
	// The seven combinations git defines as unmerged, plus near-misses that
	// must not trip the guard.
	unmerged := []string{"DD", "AU", "UD", "UA", "DU", "AA", "UU"}
	for _, code := range unmerged {
		if !isUnmergedCode(code) {
			t.Errorf("isUnmergedCode(%q) = false, want true", code)
		}
	}

	merged := []string{"M ", " M", "MM", "A ", " D", "??", "R ", "AM", "AD", "", "U"}
	for _, code := range merged {
		if isUnmergedCode(code) {
			t.Errorf("isUnmergedCode(%q) = true, want false", code)
		}
	}
}
