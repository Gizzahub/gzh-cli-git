// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"path/filepath"
	"slices"
	"sort"
	"testing"
)

// bulkCommitAll runs BulkCommit over rootDir, filling in the serial defaults the
// tests rely on for deterministic results.
func bulkCommitAll(t *testing.T, opts BulkCommitOptions) *BulkCommitResult {
	t.Helper()

	if opts.Parallel == 0 {
		opts.Parallel = 1
	}
	if opts.MaxDepth == 0 {
		opts.MaxDepth = 1
	}

	result, err := NewClient().BulkCommit(context.Background(), opts)
	if err != nil {
		t.Fatalf("BulkCommit: %v", err)
	}

	return result
}

// commitPreviewOne runs BulkCommit in dry-run mode over the single repository in
// rootDir and returns its result.
func commitPreviewOne(t *testing.T, rootDir string) RepositoryCommitResult {
	t.Helper()

	result := bulkCommitAll(t, BulkCommitOptions{
		Directory: rootDir,
		DryRun:    true,
		Yes:       true,
		Message:   "preview",
	})
	if len(result.Repositories) != 1 {
		t.Fatalf("got %d repositories, want 1", len(result.Repositories))
	}

	return result.Repositories[0]
}

// diffFileSet flattens a diff result back into the single set of paths the user
// sees, which is what has to line up with commit.
func diffFileSet(r RepositoryDiffResult) []string {
	paths := make([]string, 0, len(r.ChangedFiles)+len(r.UntrackedFiles))
	for _, f := range r.ChangedFiles {
		paths = append(paths, f.Path)
	}
	paths = append(paths, r.UntrackedFiles...)
	sort.Strings(paths)

	return paths
}

// TestDiffAndCommitAgreeOnFileSet is the end-to-end form of the reported bug:
// `gz-git diff` reported 4 files where `gz-git commit --dry-run` reported 7 on
// the same repository, so a commit message written from the diff was missing
// most of what the commit actually recorded.
//
// All three answers below must be identical: what diff shows, what commit
// previews, and what git itself writes.
func TestDiffAndCommitAgreeOnFileSet(t *testing.T) {
	repoPath := changeSetFixture(t)
	rootDir := filepath.Dir(repoPath)

	diff := diffOne(t, rootDir, BulkDiffOptions{MaxDiffSize: 1 << 20, ContextLines: 3})
	preview := commitPreviewOne(t, rootDir)

	diffPaths := diffFileSet(diff)

	previewPaths := slices.Clone(preview.ChangedFiles)
	sort.Strings(previewPaths)

	if !slices.Equal(diffPaths, previewPaths) {
		t.Errorf("diff and commit disagree on the file set\n  diff:    %q\n  preview: %q", diffPaths, previewPaths)
	}

	// And now the ground truth: what a real commit records.
	gitIn(t, repoPath, "add", "-A")
	gitIn(t, repoPath, "commit", "-m", "everything")

	committed := showNameOnly(t, repoPath)
	if !slices.Equal(diffPaths, committed) {
		t.Errorf("diff file set != committed file set\n  diff:      %q\n  committed: %q", diffPaths, committed)
	}
}

// TestDiffReportsStagedContent covers diff-omits-staged-changes end to end: with
// everything staged the old worktree↔index comparison produced an empty body and
// zero line counts, so a fully staged repository looked like it had no content.
func TestDiffReportsStagedContent(t *testing.T) {
	repoPath := changeSetFixture(t)
	rootDir := filepath.Dir(repoPath)
	gitIn(t, repoPath, "add", "-A")

	diff := diffOne(t, rootDir, BulkDiffOptions{MaxDiffSize: 1 << 20, ContextLines: 3})

	if diff.DiffContent == "" {
		t.Error("diff_content is empty for a fully staged repository")
	}
	if diff.Additions == 0 {
		t.Errorf("additions = 0 for a fully staged repository (deletions=%d)", diff.Deletions)
	}
	if diff.DiffSummary == "" {
		t.Error("diff_summary is empty for a fully staged repository")
	}
	if diff.Scope != string(ScopeHead) {
		t.Errorf("scope = %q, want %q", diff.Scope, ScopeHead)
	}

	// The breakdown has to add up, or the new keys are worse than none.
	if diff.TrackedFilesChanged+diff.UntrackedFilesChanged != len(diffFileSet(diff)) {
		t.Errorf("tracked(%d) + untracked(%d) != %d files reported",
			diff.TrackedFilesChanged, diff.UntrackedFilesChanged, len(diffFileSet(diff)))
	}
	if diff.StagedFilesChanged != diff.TrackedFilesChanged+diff.UntrackedFilesChanged {
		t.Errorf("staged_files_changed = %d, want %d (everything was staged)",
			diff.StagedFilesChanged, diff.TrackedFilesChanged+diff.UntrackedFilesChanged)
	}
}

// TestCommitPreviewCountsUntrackedFilesNotDirectories covers
// commit-preview-undercounts-untracked-dirs at the reported level: the preview
// count must equal the number of files the commit records, not the number of
// collapsed `dir/` entries.
func TestCommitPreviewCountsUntrackedFilesNotDirectories(t *testing.T) {
	repoPath := changeSetFixture(t)
	rootDir := filepath.Dir(repoPath)

	preview := commitPreviewOne(t, rootDir)

	gitIn(t, repoPath, "add", "-A")
	gitIn(t, repoPath, "commit", "-m", "everything")
	committed := showNameOnly(t, repoPath)

	if preview.FilesChanged != len(committed) {
		t.Errorf("preview files_changed = %d, commit recorded %d files: %q",
			preview.FilesChanged, len(committed), committed)
	}
	if preview.UntrackedFilesChanged != 5 {
		t.Errorf("untracked_files_changed = %d, want 5 (docs/adr/0005.md, docs/adr/0006.md, tasks/todo.md, has space.txt, 한글경로.md)",
			preview.UntrackedFilesChanged)
	}
}
