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

// testClient returns the concrete client so unexported collectors can be
// exercised directly, without routing every assertion through a bulk operation.
func testClient(t *testing.T) *client {
	t.Helper()

	c, ok := NewClient().(*client)
	if !ok {
		t.Fatal("NewClient did not return *client")
	}

	return c
}

// showNameOnly returns the sorted paths recorded in HEAD. -z keeps paths
// containing spaces in one piece and suppresses C-quoting, so the result is
// directly comparable against a change set.
func showNameOnly(t *testing.T, repoPath string) []string {
	t.Helper()

	var paths []string
	for _, p := range strings.Split(gitIn(t, repoPath, "show", "--pretty=", "--name-only", "-z", "HEAD"), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)

	return paths
}

// changeSetFixture builds the repository shape that made diff and commit
// disagree: tracked files modified, some of them already staged, plus untracked
// files nested inside directories and paths git would C-quote.
func changeSetFixture(t *testing.T) string {
	t.Helper()

	repoPath := filepath.Join(t.TempDir(), "repo")

	if err := os.MkdirAll(repoPath, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := initGitRepoWithCommit(repoPath); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	writeIn(t, repoPath, "tracked1.txt", "one\n")
	writeIn(t, repoPath, "tracked2.txt", "two\n")
	gitIn(t, repoPath, "add", "tracked1.txt", "tracked2.txt")
	gitIn(t, repoPath, "commit", "-m", "add tracked files")

	// Tracked modifications: one staged, one left in the worktree.
	writeIn(t, repoPath, "tracked1.txt", "one modified\nextra\n")
	gitIn(t, repoPath, "add", "tracked1.txt")
	writeIn(t, repoPath, "tracked2.txt", "two modified\n")

	// Untracked files, collapsed to "docs/" and "tasks/" by plain --porcelain.
	writeIn(t, repoPath, "docs/adr/0005.md", "adr five\n")
	writeIn(t, repoPath, "docs/adr/0006.md", "adr six\n")
	writeIn(t, repoPath, "tasks/todo.md", "todo\n")

	// Paths that plain --porcelain would return wrapped in quotes.
	writeIn(t, repoPath, "has space.txt", "spaced\n")
	writeIn(t, repoPath, "한글경로.md", "korean\n")

	return repoPath
}

func writeIn(t *testing.T, repoPath, rel, content string) {
	t.Helper()

	abs := filepath.Join(repoPath, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func gitIn(t *testing.T, repoPath string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...) //nolint:noctx // test helper, no context available
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}

	return string(out)
}

func entryPaths(cs *ChangeSet) []string {
	paths := make([]string, 0, len(cs.Entries))
	for _, e := range cs.Entries {
		paths = append(paths, e.Path)
	}
	sort.Strings(paths)

	return paths
}

// TestChangeSetMatchesAddAllCommit is the criterion the whole task exists for:
// the HEAD-scoped change set must equal what `git add -A && git commit` records.
func TestChangeSetMatchesAddAllCommit(t *testing.T) {
	repoPath := changeSetFixture(t)

	cs, err := testClient(t).collectChangeSet(context.Background(), repoPath, ScopeHead)
	if err != nil {
		t.Fatalf("collectChangeSet: %v", err)
	}

	// Let git answer the same question its own way.
	gitIn(t, repoPath, "add", "-A")
	gitIn(t, repoPath, "commit", "-m", "everything")
	committed := showNameOnly(t, repoPath)

	got := entryPaths(cs)
	if !slices.Equal(got, committed) {
		t.Errorf("change set != committed set\n  change set: %q\n  committed:  %q", got, committed)
	}
}

// TestChangeSetExpandsUntrackedDirectories covers
// commit-preview-undercounts-untracked-dirs: without -uall the three files below
// arrived as two directory entries.
func TestChangeSetExpandsUntrackedDirectories(t *testing.T) {
	repoPath := changeSetFixture(t)

	cs, err := testClient(t).collectChangeSet(context.Background(), repoPath, ScopeHead)
	if err != nil {
		t.Fatalf("collectChangeSet: %v", err)
	}

	for _, want := range []string{"docs/adr/0005.md", "docs/adr/0006.md", "tasks/todo.md"} {
		if !slices.Contains(entryPaths(cs), want) {
			t.Errorf("missing %q: %v", want, entryPaths(cs))
		}
	}
	for _, unwanted := range []string{"docs/", "tasks/"} {
		if slices.Contains(entryPaths(cs), unwanted) {
			t.Errorf("collapsed directory entry %q survived", unwanted)
		}
	}
	// 2 tracked + 5 untracked
	if cs.UntrackedCount != 5 {
		t.Errorf("UntrackedCount = %d, want 5", cs.UntrackedCount)
	}
	if cs.TrackedCount != 2 {
		t.Errorf("TrackedCount = %d, want 2", cs.TrackedCount)
	}
}

// TestChangeSetUnquotesPaths covers porcelain-quoted-paths-never-unquoted for
// both core.quotePath settings, since the space case breaks regardless of it.
func TestChangeSetUnquotesPaths(t *testing.T) {
	for _, quotePath := range []string{"true", "false"} {
		t.Run("quotePath="+quotePath, func(t *testing.T) {
			repoPath := changeSetFixture(t)
			gitIn(t, repoPath, "config", "core.quotePath", quotePath)

			cs, err := testClient(t).collectChangeSet(context.Background(), repoPath, ScopeHead)
			if err != nil {
				t.Fatalf("collectChangeSet: %v", err)
			}

			paths := entryPaths(cs)
			for _, want := range []string{"has space.txt", "한글경로.md"} {
				if !slices.Contains(paths, want) {
					t.Errorf("missing %q: %v", want, paths)
				}
			}
			for _, p := range paths {
				if strings.HasPrefix(p, `"`) {
					t.Errorf("path %q is still C-quoted", p)
				}
				if _, err := os.Lstat(filepath.Join(repoPath, p)); err != nil {
					t.Errorf("path %q does not exist on disk: %v", p, err)
				}
			}
		})
	}
}

// TestChangeSetStagedScopeCountsStagedChanges covers diff-omits-staged-changes:
// the previous worktree↔index comparison reported zero lines for a repository
// whose changes were all staged.
func TestChangeSetStagedScopeCountsStagedChanges(t *testing.T) {
	repoPath := changeSetFixture(t)
	gitIn(t, repoPath, "add", "-A")

	c := testClient(t)
	ctx := context.Background()

	head, err := c.collectChangeSet(ctx, repoPath, ScopeHead)
	if err != nil {
		t.Fatalf("collectChangeSet(head): %v", err)
	}
	if head.Additions == 0 {
		t.Error("HEAD scope reported 0 additions for a fully staged repository")
	}
	if head.StagedCount != len(head.Entries) {
		t.Errorf("StagedCount = %d, want %d (everything is staged)", head.StagedCount, len(head.Entries))
	}

	staged, err := c.collectChangeSet(ctx, repoPath, ScopeStagedOnly)
	if err != nil {
		t.Fatalf("collectChangeSet(staged): %v", err)
	}
	if !slices.Equal(entryPaths(head), entryPaths(staged)) {
		t.Errorf("staged scope != head scope when everything is staged:\n  head:   %q\n  staged: %q",
			entryPaths(head), entryPaths(staged))
	}
	if staged.Additions != head.Additions {
		t.Errorf("staged additions = %d, head additions = %d", staged.Additions, head.Additions)
	}

	// A worktree-only comparison must now be empty, and say so rather than
	// silently standing in for the other two.
	worktree, err := c.collectChangeSet(ctx, repoPath, ScopeWorktreeOnly)
	if err != nil {
		t.Fatalf("collectChangeSet(worktree): %v", err)
	}
	if len(worktree.Entries) != 0 {
		t.Errorf("worktree scope should be empty after add -A, got %q", entryPaths(worktree))
	}
}

// TestChangeSetPartiallyStaged checks the index/worktree distinction that
// TrimSpace used to erase: " M" and "M " both trimmed to "M".
func TestChangeSetPartiallyStaged(t *testing.T) {
	repoPath := changeSetFixture(t)

	cs, err := testClient(t).collectChangeSet(context.Background(), repoPath, ScopeHead)
	if err != nil {
		t.Fatalf("collectChangeSet: %v", err)
	}

	byPath := make(map[string]ChangeEntry, len(cs.Entries))
	for _, e := range cs.Entries {
		byPath[e.Path] = e
	}

	if e := byPath["tracked1.txt"]; !e.Staged {
		t.Error("tracked1.txt was staged with `git add` but Staged is false")
	}
	if e := byPath["tracked2.txt"]; e.Staged {
		t.Error("tracked2.txt was never staged but Staged is true")
	}
	if cs.StagedCount != 1 {
		t.Errorf("StagedCount = %d, want 1", cs.StagedCount)
	}
}

// TestChangeSetRenameKeepsBothPaths checks the -z field-order reversal: git
// drops the " -> " separator and emits destination first, source second.
func TestChangeSetRenameKeepsBothPaths(t *testing.T) {
	rootDir := t.TempDir()
	repoPath := filepath.Join(rootDir, "repo")
	if err := os.MkdirAll(repoPath, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := initGitRepoWithCommit(repoPath); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	writeIn(t, repoPath, "old name.txt", strings.Repeat("content\n", 20))
	gitIn(t, repoPath, "add", "old name.txt")
	gitIn(t, repoPath, "commit", "-m", "add file")
	gitIn(t, repoPath, "mv", "old name.txt", "new name.txt")

	cs, err := testClient(t).collectChangeSet(context.Background(), repoPath, ScopeHead)
	if err != nil {
		t.Fatalf("collectChangeSet: %v", err)
	}

	var renamed *ChangeEntry
	for i := range cs.Entries {
		if cs.Entries[i].Status == "R" {
			renamed = &cs.Entries[i]
		}
	}
	if renamed == nil {
		t.Fatalf("no rename entry found: %+v", cs.Entries)
	}
	if renamed.Path != "new name.txt" {
		t.Errorf("Path = %q, want %q (destination comes first under -z)", renamed.Path, "new name.txt")
	}
	if renamed.OldPath != "old name.txt" {
		t.Errorf("OldPath = %q, want %q", renamed.OldPath, "old name.txt")
	}
	// The source path must not leak in as an entry of its own.
	if len(cs.Entries) != 1 {
		t.Errorf("got %d entries, want 1: %q", len(cs.Entries), entryPaths(cs))
	}
}

// TestChangeSetConflictedIsNotStaged guards the interaction with the conflict
// gate: an unmerged path has index stages, not a staged change, and must not
// pass a staged-only filter.
func TestChangeSetConflictedIsNotStaged(t *testing.T) {
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "conflicted")
	createConflictedRepo(t, repoPath)

	c := testClient(t)
	ctx := context.Background()

	cs, err := c.collectChangeSet(ctx, repoPath, ScopeHead)
	if err != nil {
		t.Fatalf("collectChangeSet: %v", err)
	}
	if cs.ConflictCount != 1 {
		t.Errorf("ConflictCount = %d, want 1", cs.ConflictCount)
	}
	if cs.StagedCount != 0 {
		t.Errorf("StagedCount = %d, want 0 (a conflict is not a staged change)", cs.StagedCount)
	}

	staged, err := c.collectChangeSet(ctx, repoPath, ScopeStagedOnly)
	if err != nil {
		t.Fatalf("collectChangeSet(staged): %v", err)
	}
	if len(staged.Entries) != 0 {
		t.Errorf("conflicted path passed the staged-only filter: %q", entryPaths(staged))
	}
}

// TestChangeSetUnbornHead covers a repository with no commits, where
// `git diff HEAD` has nothing to compare against and fails.
func TestChangeSetUnbornHead(t *testing.T) {
	rootDir := t.TempDir()
	repoPath := filepath.Join(rootDir, "repo")
	if err := os.MkdirAll(repoPath, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := initGitRepo(repoPath); err != nil {
		t.Fatalf("git init: %v", err)
	}

	writeIn(t, repoPath, "first.txt", "a\nb\nc\n")
	gitIn(t, repoPath, "add", "first.txt")

	cs, err := testClient(t).collectChangeSet(context.Background(), repoPath, ScopeHead)
	if err != nil {
		t.Fatalf("collectChangeSet: %v", err)
	}
	if len(cs.Entries) != 1 || cs.Entries[0].Path != "first.txt" {
		t.Fatalf("entries = %q, want [first.txt]", entryPaths(cs))
	}
	if cs.Additions != 3 {
		t.Errorf("Additions = %d, want 3 (fell back to --cached on unborn HEAD)", cs.Additions)
	}
}

func TestParseNumstat(t *testing.T) {
	tests := []struct {
		name       string
		out        string
		wantAdd    int
		wantDelete int
		wantFiles  int
	}{
		{"empty", "", 0, 0, 0},
		{"single file", "3\t1\ta.txt\x00", 3, 1, 1},
		{"two files", "3\t1\ta.txt\x004\t2\tb.txt\x00", 7, 3, 2},
		{
			// Binary files report "-" for both counts: no lines, but still a file
			// that differs, which is why files is counted separately.
			"binary", "-\t-\tblob.bin\x005\t0\ta.txt\x00", 5, 0, 2,
		},
		{
			// A mode-only change nets zero lines and must still count as a file.
			"mode only", "0\t0\tscript.sh\x00", 0, 0, 1,
		},
		{
			// Rename: empty path field, then two extra records that must be
			// consumed rather than parsed as counts.
			"rename", "1\t1\t\x00old.txt\x00new.txt\x002\t0\ta.txt\x00", 3, 1, 2,
		},
		{"spaced path", "1\t0\thas space.txt\x00", 1, 0, 1},
		{
			// The poisoning fixture: under the old --stat text parser the leading
			// digits of these names were read as line counts.
			"numeric filenames",
			"3\t0\t9-changed-deletions.txt\x000\t0\t40-unchanged-insertions.md\x00",
			3, 0, 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			add, del, files := parseNumstat(tt.out)
			if add != tt.wantAdd || del != tt.wantDelete || files != tt.wantFiles {
				t.Errorf("parseNumstat() = (%d, %d, %d), want (%d, %d, %d)",
					add, del, files, tt.wantAdd, tt.wantDelete, tt.wantFiles)
			}
		})
	}
}

func TestScopeDiffArgs(t *testing.T) {
	tests := []struct {
		scope ChangeScope
		want  []string
	}{
		{ScopeHead, []string{"HEAD"}},
		{ScopeStagedOnly, []string{"--cached"}},
		{ScopeWorktreeOnly, nil},
	}

	for _, tt := range tests {
		t.Run(string(tt.scope), func(t *testing.T) {
			if got := scopeDiffArgs(tt.scope); !slices.Equal(got, tt.want) {
				t.Errorf("scopeDiffArgs(%q) = %q, want %q", tt.scope, got, tt.want)
			}
		})
	}
}
