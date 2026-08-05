// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// TestGetStatusExpandsUntrackedDirectories exercises the flags rather than the
// parser.
//
// TestParseStatus feeds records in directly, so it can prove how a record is
// classified but never that the call site asked for the right records. Only a
// real repository shows whether -uall reached git: without it, the three files
// under docs/ and tasks/ collapse into two "docs/" and "tasks/" entries and a
// caller counting untracked files is off by three.
func TestGetStatusExpandsUntrackedDirectories(t *testing.T) {
	repoPath := changeSetFixture(t)
	c := testClient(t)

	status, err := c.GetStatus(context.Background(), &Repository{Path: repoPath})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	got := slices.Clone(status.UntrackedFiles)
	sort.Strings(got)

	want := []string{
		"docs/adr/0005.md",
		"docs/adr/0006.md",
		"has space.txt",
		"tasks/todo.md",
		"한글경로.md",
	}

	if !slices.Equal(got, want) {
		t.Errorf("UntrackedFiles = %q, want %q", got, want)
	}

	// tracked1.txt is staged, tracked2.txt is modified in the worktree; neither
	// overlaps, so the three counts are unambiguous.
	if status.StagedCount != 1 {
		t.Errorf("StagedCount = %d, want 1", status.StagedCount)
	}
	if status.UnstagedCount != 1 {
		t.Errorf("UnstagedCount = %d, want 1", status.UnstagedCount)
	}
	if status.TrackedChangedCount != 2 {
		t.Errorf("TrackedChangedCount = %d, want 2", status.TrackedChangedCount)
	}
}

// worktreeRenameFixture produces the two codes that carry the rename or the
// addition on the *worktree* side rather than the index side.
//
// `git mv` stages the rename and yields "R ". Renaming on disk and only
// intent-to-adding the destination leaves the deletion of the source unstaged,
// which is what moves the R to the second column. Both forms need git >= 2.16
// for `git add -N`; the caller asserts the raw bytes so a future git that
// classified this differently fails loudly instead of silently covering nothing.
func worktreeRenameFixture(t *testing.T) string {
	t.Helper()

	repoPath := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoPath, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := initGitRepoWithCommit(repoPath); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	writeIn(t, repoPath, "handler.go", "package main\n\nfunc Handle() {}\n")
	writeIn(t, repoPath, "keep.txt", "unchanged\n")
	gitIn(t, repoPath, "add", "handler.go", "keep.txt")
	gitIn(t, repoPath, "commit", "-m", "add handler")

	if err := os.Rename(
		filepath.Join(repoPath, "handler.go"),
		filepath.Join(repoPath, "handler_v2.go"),
	); err != nil {
		t.Fatalf("rename: %v", err)
	}
	gitIn(t, repoPath, "add", "-N", "handler_v2.go")

	// Content deliberately unlike handler.go so rename detection cannot pair the
	// deletion with this file instead.
	writeIn(t, repoPath, "brand_new.go", "package other\n\nvar Answer = 42\n")
	gitIn(t, repoPath, "add", "-N", "brand_new.go")

	return repoPath
}

// TestGetStatusPairsWorktreeSideRename pins the pairing rule that took a whole
// status read down.
//
// -z moves a rename's source path into the *next* record. Consuming it only
// when the R sits in the index column left the source loose in the stream for
// " R", where the next iteration read it as a status line and took its first two
// bytes as an XY code: for "handler.go" that is "ha", so applyStatusCode
// returned `unknown index status code: h` and GetStatus failed outright. What
// made that a silent wrong answer rather than a loud one is the caller —
// reposync's health check assumed a clean tree on error — so this test is only
// half the guard; TestCheckHealthDoesNotCallUnreadableTreeHealthy is the rest.
func TestGetStatusPairsWorktreeSideRename(t *testing.T) {
	repoPath := worktreeRenameFixture(t)
	c := testClient(t)

	// Assert the fixture's own premise. Without this the test would keep passing
	// on a git that stopped reporting these codes, while covering nothing.
	raw := gitIn(t, repoPath, "status", "--porcelain", "-z", "-uall")
	if !strings.Contains(raw, " R handler_v2.go\x00handler.go\x00") {
		t.Fatalf("fixture yields no worktree-side rename; porcelain = %q", raw)
	}
	if !strings.Contains(raw, " A brand_new.go\x00") {
		t.Fatalf("fixture yields no intent-to-add; porcelain = %q", raw)
	}

	status, err := c.GetStatus(context.Background(), &Repository{Path: repoPath})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	if len(status.RenamedFiles) != 1 {
		t.Fatalf("RenamedFiles = %+v, want exactly one entry", status.RenamedFiles)
	}
	if got := status.RenamedFiles[0]; got.OldPath != "handler.go" || got.NewPath != "handler_v2.go" {
		t.Errorf("RenamedFiles[0] = %+v, want {handler.go -> handler_v2.go}", got)
	}

	// The source path must have been consumed as a source. If it leaks back into
	// the record stream it shows up here as a file in its own right.
	if len(status.UntrackedFiles) != 0 {
		t.Errorf("UntrackedFiles = %q, want none", status.UntrackedFiles)
	}

	// Two entries, both unstaged: the rename and the intent-to-add. The counts
	// and the lists have to agree — an entry that raises a count while appearing
	// in no list is how the intent-to-add case hid.
	if status.TrackedChangedCount != 2 {
		t.Errorf("TrackedChangedCount = %d, want 2", status.TrackedChangedCount)
	}
	if status.UnstagedCount != 2 {
		t.Errorf("UnstagedCount = %d, want 2", status.UnstagedCount)
	}
	if status.StagedCount != 0 {
		t.Errorf("StagedCount = %d, want 0", status.StagedCount)
	}

	gotModified := slices.Clone(status.ModifiedFiles)
	sort.Strings(gotModified)
	wantModified := []string{"brand_new.go", "handler_v2.go"}
	if !slices.Equal(gotModified, wantModified) {
		t.Errorf("ModifiedFiles = %q, want %q", gotModified, wantModified)
	}

	if status.IsClean {
		t.Error("IsClean = true for a tree holding a rename and a new file")
	}
}

// mergeConflictFixture leaves the repository mid-merge with one conflicted path,
// one cleanly merged path, and one untracked file.
func mergeConflictFixture(t *testing.T) string {
	t.Helper()

	repoPath := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoPath, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := initGitRepoWithCommit(repoPath); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	writeIn(t, repoPath, "conflict.txt", "base\n")
	writeIn(t, repoPath, "calm.txt", "untouched\n")
	gitIn(t, repoPath, "add", "conflict.txt", "calm.txt")
	gitIn(t, repoPath, "commit", "-m", "base")

	// The default branch name is a git config, not a constant.
	base := strings.TrimSpace(gitIn(t, repoPath, "rev-parse", "--abbrev-ref", "HEAD"))

	gitIn(t, repoPath, "checkout", "-q", "-b", "feature")
	writeIn(t, repoPath, "conflict.txt", "feature side\n")
	gitIn(t, repoPath, "commit", "-am", "feature edit")

	gitIn(t, repoPath, "checkout", "-q", base)
	writeIn(t, repoPath, "conflict.txt", "base side\n")
	gitIn(t, repoPath, "commit", "-am", "base edit")

	// A non-zero exit is the point, so this cannot go through gitIn.
	cmd := exec.Command("git", "merge", "feature") //nolint:noctx // test helper, no context available
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("git merge succeeded; the fixture must conflict\n%s", out)
	}

	writeIn(t, repoPath, "scratch/notes.md", "untracked\n")

	return repoPath
}

// TestCheckRepositoryStateReportsConflicts covers the positive side of the push
// guard, which had no test at all: every existing case asserted HasConflicts
// was false, so a detector wired to return false unconditionally would have
// passed the suite while letting `gz-git push` run on a half-merged tree.
//
// It also pins -uno at this call site. checkRepositoryState reads only
// ConflictFiles and a conflicted path is by definition tracked, so suppressing
// the untracked walk must not change the verdict — scratch/notes.md is here to
// show it does not, and that untracked paths never reach ConflictedFiles.
func TestCheckRepositoryStateReportsConflicts(t *testing.T) {
	repoPath := mergeConflictFixture(t)
	c := testClient(t)

	state, err := c.checkRepositoryState(context.Background(), repoPath)
	if err != nil {
		t.Fatalf("checkRepositoryState: %v", err)
	}

	if !state.HasConflicts {
		t.Fatal("HasConflicts = false during an unresolved merge")
	}
	if !state.MergeInProgress {
		t.Error("MergeInProgress = false during an unresolved merge")
	}

	got := slices.Clone(state.ConflictedFiles)
	sort.Strings(got)
	if want := []string{"conflict.txt"}; !slices.Equal(got, want) {
		t.Errorf("ConflictedFiles = %q, want %q", got, want)
	}
}

// TestCheckRepositoryStateSeparatesDirtyFromConflicted is the false-positive
// half of the pair above: staged edits, worktree edits and untracked trees are
// not conflicts, however many of them there are.
func TestCheckRepositoryStateSeparatesDirtyFromConflicted(t *testing.T) {
	repoPath := changeSetFixture(t)
	c := testClient(t)

	state, err := c.checkRepositoryState(context.Background(), repoPath)
	if err != nil {
		t.Fatalf("checkRepositoryState: %v", err)
	}

	if state.HasConflicts {
		t.Errorf("HasConflicts = true, want false (files are merely dirty)")
	}
	if len(state.ConflictedFiles) != 0 {
		t.Errorf("ConflictedFiles = %q, want none", state.ConflictedFiles)
	}
	if state.MergeInProgress || state.RebaseInProgress {
		t.Errorf("state = %+v, want no operation in progress", state)
	}
}

// TestCheckRepositoryStateFailsOnBrokenGit pins the fail-fast contract.
//
// Executor.Run reports a failed git through Result.ExitCode and returns a nil
// error, so checking only err let a git that died read as a clean, conflict-free
// repository — and the push guard built on HasConflicts opened. A corrupt index
// makes `git status` exit 128 while the repository still looks valid to the
// checks that run before it.
func TestCheckRepositoryStateFailsOnBrokenGit(t *testing.T) {
	repoPath := changeSetFixture(t)
	c := testClient(t)

	indexPath := filepath.Join(repoPath, ".git", "index")
	if err := os.WriteFile(indexPath, []byte("not an index"), 0o600); err != nil {
		t.Fatalf("corrupt index: %v", err)
	}

	state, err := c.checkRepositoryState(context.Background(), repoPath)
	if err == nil {
		t.Fatalf("checkRepositoryState returned no error for a broken git; state = %+v", state)
	}
	if state != nil {
		t.Errorf("state = %+v, want nil alongside the error", state)
	}
}
