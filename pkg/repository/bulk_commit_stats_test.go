// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"os"
	"path/filepath"
	"testing"
)

// newRepoIn creates an initialized repository with one commit under rootDir.
func newRepoIn(t *testing.T, rootDir, name string) string {
	t.Helper()

	repoPath := filepath.Join(rootDir, name)
	if err := os.MkdirAll(repoPath, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := initGitRepoWithCommit(repoPath); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	return repoPath
}

// recordedStats commits everything and returns what git actually recorded, which
// is the only authority on what the preview should have said.
func recordedStats(t *testing.T, repoPath string) (additions, deletions, files int) {
	t.Helper()

	gitIn(t, repoPath, "add", "-A")
	gitIn(t, repoPath, "commit", "-m", "actual")

	return parseNumstat(gitIn(t, repoPath, "diff", "--numstat", "-z", "HEAD~1", "HEAD"))
}

// TestCommitNetZeroIsNotPreviewedAsCommittable covers
// commit-stats-sum-not-head-delta. `MM f.txt` — staged, then reverted in the
// worktree — leaves the index differing from HEAD while the worktree matches it.
// The net change against HEAD is empty, so `git add -A && git commit` fails with
// "nothing to commit"; the preview used to promise would-commit anyway, and the
// resulting failure exited 0.
func TestCommitNetZeroIsNotPreviewedAsCommittable(t *testing.T) {
	rootDir := t.TempDir()
	repoPath := newRepoIn(t, rootDir, "repo")

	writeIn(t, repoPath, "f.txt", "one\n")
	gitIn(t, repoPath, "add", "f.txt")
	gitIn(t, repoPath, "commit", "-m", "base")

	writeIn(t, repoPath, "f.txt", "changed\n")
	gitIn(t, repoPath, "add", "f.txt")
	writeIn(t, repoPath, "f.txt", "one\n") // back to exactly HEAD

	if got := gitIn(t, repoPath, "status", "--porcelain"); got == "" {
		t.Fatal("fixture is not dirty; the test would prove nothing")
	}

	preview := commitPreviewOne(t, rootDir)

	if preview.Status == "would-commit" || preview.Status == "dirty" {
		t.Errorf("status = %q, want clean: there is no HEAD delta to commit (additions=%d deletions=%d)",
			preview.Status, preview.Additions, preview.Deletions)
	}
}

// TestCommitStatsMatchRecordedCommit covers
// commit-additions-double-count-staged-plus-unstaged: summing `--stat --cached`
// and `--stat` counted a line edited both in the index and again in the worktree
// twice over.
func TestCommitStatsMatchRecordedCommit(t *testing.T) {
	rootDir := t.TempDir()
	repoPath := newRepoIn(t, rootDir, "repo")

	writeIn(t, repoPath, "f.txt", "a\nb\nc\n")
	gitIn(t, repoPath, "add", "f.txt")
	gitIn(t, repoPath, "commit", "-m", "base")

	// Edit line 1, stage it, then edit line 1 again in the worktree. The old sum
	// reported 2/2; the commit records 1/1.
	writeIn(t, repoPath, "f.txt", "A\nb\nc\n")
	gitIn(t, repoPath, "add", "f.txt")
	writeIn(t, repoPath, "f.txt", "AA\nb\nc\n")

	preview := commitPreviewOne(t, rootDir)

	wantAdd, wantDel, _ := recordedStats(t, repoPath)
	if preview.Additions != wantAdd || preview.Deletions != wantDel {
		t.Errorf("preview reported +%d/-%d, commit recorded +%d/-%d",
			preview.Additions, preview.Deletions, wantAdd, wantDel)
	}
}

// TestCommitCountsUntrackedLines covers commit-untracked-lines-never-counted:
// untracked files appeared in files_changed but never in additions, because
// `git diff` cannot see what is not in the index.
func TestCommitCountsUntrackedLines(t *testing.T) {
	rootDir := t.TempDir()
	repoPath := newRepoIn(t, rootDir, "repo")

	writeIn(t, repoPath, "new1.txt", "1\n2\n3\n4\n5\n")
	writeIn(t, repoPath, "docs/new2.md", "a\nb\n")
	writeIn(t, repoPath, "nonewline.txt", "no trailing newline")

	preview := commitPreviewOne(t, rootDir)

	wantAdd, wantDel, _ := recordedStats(t, repoPath)
	if preview.Additions != wantAdd || preview.Deletions != wantDel {
		t.Errorf("preview reported +%d/-%d, commit recorded +%d/-%d",
			preview.Additions, preview.Deletions, wantAdd, wantDel)
	}
	if wantAdd != 8 {
		t.Fatalf("fixture drift: git recorded %d additions, expected 8 (5 + 2 + 1)", wantAdd)
	}
}

// TestCommitStatsIgnoreFilenameDigits covers parse-diff-stats-filename-poisoning:
// the old parser matched any line containing "changed", which includes file name
// lines, and then read their leading digits as line counts.
func TestCommitStatsIgnoreFilenameDigits(t *testing.T) {
	rootDir := t.TempDir()
	repoPath := newRepoIn(t, rootDir, "repo")

	writeIn(t, repoPath, "9-changed-deletions.txt", "a\n")
	writeIn(t, repoPath, "40-unchanged-insertions.md", "b\n")
	gitIn(t, repoPath, "add", "-A")
	gitIn(t, repoPath, "commit", "-m", "base")

	// Pure addition of three lines to a file whose name starts with 9.
	writeIn(t, repoPath, "9-changed-deletions.txt", "a\nx\ny\nz\n")

	preview := commitPreviewOne(t, rootDir)

	if preview.Additions != 3 || preview.Deletions != 0 {
		t.Errorf("reported +%d/-%d, want +3/-0 (file name digits must not be read as counts)",
			preview.Additions, preview.Deletions)
	}
}

// TestCommitCountsBinaryAsZeroLines pins the counterpart of the file-count fix:
// git records no insertions for binary content, so neither do we — but the file
// still has to be recognized as something to commit.
func TestCommitCountsBinaryAsZeroLines(t *testing.T) {
	rootDir := t.TempDir()
	repoPath := newRepoIn(t, rootDir, "repo")

	if err := os.WriteFile(filepath.Join(repoPath, "blob.bin"), []byte("\x00\x01\x02binary\x00"), 0o600); err != nil {
		t.Fatalf("write blob: %v", err)
	}

	preview := commitPreviewOne(t, rootDir)

	if preview.Status != "would-commit" {
		t.Errorf("status = %q, want would-commit: a binary file is still a change", preview.Status)
	}
	if preview.Additions != 0 {
		t.Errorf("additions = %d, want 0 for binary content", preview.Additions)
	}
	if preview.FilesChanged != 1 {
		t.Errorf("files_changed = %d, want 1", preview.FilesChanged)
	}
}

// TestBulkCommitSkippedCountsFilteredSet covers the two TotalSkipped defects:
// it was measured against the pre-filter scan total, and it was computed after
// the dry-run return so a dry run always reported zero.
func TestBulkCommitSkippedCountsFilteredSet(t *testing.T) {
	rootDir := t.TempDir()

	dirty := newRepoIn(t, rootDir, "dirty-repo")
	writeIn(t, dirty, "change.txt", "x\n")

	newRepoIn(t, rootDir, "clean-one")
	newRepoIn(t, rootDir, "clean-two")

	t.Run("dry run reports skipped", func(t *testing.T) {
		result := bulkCommitAll(t, BulkCommitOptions{Directory: rootDir, DryRun: true})

		if result.TotalScanned != 3 {
			t.Fatalf("fixture drift: scanned %d repositories, want 3", result.TotalScanned)
		}
		if result.TotalDirty != 1 {
			t.Fatalf("dirty = %d, want 1", result.TotalDirty)
		}
		if result.TotalSkipped != 2 {
			t.Errorf("dry-run skipped = %d, want 2 (was always 0: computed after the dry-run return)",
				result.TotalSkipped)
		}
	})

	t.Run("skipped excludes filtered-out repositories", func(t *testing.T) {
		result := bulkCommitAll(t, BulkCommitOptions{
			Directory:      rootDir,
			DryRun:         true,
			IncludePattern: "clean-one",
		})

		if len(result.Repositories) != 1 {
			t.Fatalf("include filter matched %d repositories, want 1", len(result.Repositories))
		}
		if result.TotalSkipped != 1 {
			t.Errorf("skipped = %d, want 1: the two repositories excluded by --include were never candidates",
				result.TotalSkipped)
		}
	})
}
