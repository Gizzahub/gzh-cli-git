// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package branch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)

// git runs a git command in dir and fails the test if it does not succeed.
func git(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...) //nolint:noctx // test helper; no context.Context available in *testing.T API
	cmd.Dir = dir

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// write creates a file under dir, making parent directories as needed.
func write(t *testing.T, dir, name, content string) {
	t.Helper()

	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatalf("mkdir for %s: %v", name, err)
	}

	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// TestGetModifiedFiles verifies that every returned path names a file that
// exists.
//
// The values feed a map keyed by path and compared across worktrees to find
// files two branches touch at once, so a path that names nothing on disk cannot
// match anything — the conflict it should have revealed is simply not reported.
// Each case below is a shape that used to produce such a value.
func TestGetModifiedFiles(t *testing.T) {
	repo := testutil.TempGitRepoWithCommit(t)

	// Committed files, so the changes below are tracked ones.
	write(t, repo, "old.txt", "content\n")
	write(t, repo, "plain.go", "package main\n")
	write(t, repo, "my file.go", "package main\n")
	write(t, repo, "한글.md", "# 제목\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "add fixtures")

	// A rename: plain --porcelain renders this as one line reading
	// "R  old.txt -> new.txt", which the previous parser returned whole.
	git(t, repo, "mv", "old.txt", "new.txt")

	// A worktree-only modification. Its status code is " M", whose leading space
	// is significant — an X of ' ' means the index still agrees with HEAD.
	write(t, repo, "plain.go", "package main\n\nfunc main() {}\n")

	// Paths git C-quotes under plain --porcelain: one holding a space, one
	// holding non-ASCII bytes. Both come back with quotes and \nnn escapes
	// baked in, naming no real file.
	write(t, repo, "my file.go", "package main\n\nvar x = 1\n")
	write(t, repo, "한글.md", "# 제목\n\n본문\n")

	// Untracked entries, including a directory. --porcelain without -uall
	// collapses the directory to a single `fresh/` entry, so three new files
	// were reported as one path that is not a file.
	write(t, repo, "loose.txt", "new\n")
	write(t, repo, "fresh/a.txt", "a\n")
	write(t, repo, "fresh/b.txt", "b\n")

	p := &parallelWorkflow{executor: gitcmd.NewExecutor()}

	got, err := p.getModifiedFiles(context.Background(), repo)
	if err != nil {
		t.Fatalf("getModifiedFiles() error = %v", err)
	}

	want := []string{"my file.go", "new.txt", "plain.go", "한글.md"}
	slices.Sort(got)

	if !slices.Equal(got, want) {
		t.Errorf("getModifiedFiles() = %q, want %q", got, want)
	}

	// The point of the paths above is that they exist. A value that names no
	// file can never match another worktree's entry, which is the failure mode
	// this test is guarding, so assert it directly rather than trusting the
	// string comparison to have covered it.
	for _, f := range got {
		if _, err := os.Lstat(filepath.Join(repo, f)); err != nil {
			t.Errorf("returned path %q does not exist: %v", f, err)
		}
	}
}

// TestGetModifiedFilesExcludesUntracked pins the narrower claim separately: the
// function is named for modified files and its result is compared against other
// worktrees, where two branches independently creating a file git does not track
// is not the overlap being detected.
func TestGetModifiedFilesExcludesUntracked(t *testing.T) {
	repo := testutil.TempGitRepoWithCommit(t)

	write(t, repo, "untracked.txt", "new\n")
	write(t, repo, "dir/nested.txt", "new\n")

	p := &parallelWorkflow{executor: gitcmd.NewExecutor()}

	got, err := p.getModifiedFiles(context.Background(), repo)
	if err != nil {
		t.Fatalf("getModifiedFiles() error = %v", err)
	}

	if len(got) != 0 {
		t.Errorf("getModifiedFiles() = %q, want no entries (all changes are untracked)", got)
	}
}

// TestGetModifiedFilesFailsOnBrokenRepo verifies that a git that could not run
// is not read as a worktree with nothing changed.
//
// gitcmd.Executor.Run signals a failed git through Result.ExitCode and returns
// a nil error unless the process itself could not start, so checking err alone
// turned "status failed" into an empty file list — and an empty list means no
// conflicts, which is the answer a caller most wants to be true.
func TestGetModifiedFilesFailsOnBrokenRepo(t *testing.T) {
	notARepo := t.TempDir()

	p := &parallelWorkflow{executor: gitcmd.NewExecutor()}

	if _, err := p.getModifiedFiles(context.Background(), notARepo); err == nil {
		t.Error("getModifiedFiles() on a non-repository returned nil error, want failure")
	}
}
