// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package reposync

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/porcelain"
	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)

// gitOK runs a git command in dir and fails the test unless it succeeds.
func gitOK(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...) //nolint:noctx // test helper; no context.Context available in *testing.T API
	cmd.Dir = dir

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// gitAny runs a git command in dir and ignores its exit status. Merges that
// conflict exit non-zero, which is the point of the fixtures below.
func gitAny(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...) //nolint:noctx // test helper; no context.Context available in *testing.T API
	cmd.Dir = dir
	_, _ = cmd.CombinedOutput()
}

// writeFile creates a file under dir.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// statusCodes returns the porcelain status codes present in the repository, so
// a fixture can assert it actually produced the conflict shape it is named for
// rather than some other conflict that happens to also be detected.
func statusCodes(t *testing.T, dir string) []string {
	t.Helper()

	cmd := exec.Command("git", "status", "--porcelain", "-z") //nolint:noctx // test helper; no context.Context available in *testing.T API
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status failed: %v", err)
	}

	records, err := porcelain.Parse(string(out))
	if err != nil {
		t.Fatalf("porcelain.Parse: %v", err)
	}

	codes := make([]string, 0, len(records))
	for _, rec := range records {
		codes = append(codes, rec.Code)
	}

	return codes
}

// TestCollectPostSyncStatusDetectsAAConflict covers the conflict code that the
// previous hand-rolled scan could not see.
//
// That scan tested each status letter against 'U' alone. Five of git's seven
// unmerged codes contain one, so the miss was invisible in ordinary use — but
// "both added" contains no U at all, and a repository sitting in that state was
// reported as merely dirty. The badge then reads "dirty" where it should read
// "conflict", and the next sync runs against a half-merged tree.
func TestCollectPostSyncStatusDetectsAAConflict(t *testing.T) {
	repoPath := testutil.TempGitRepoWithCommit(t)

	// Two branches independently create the same path with different content.
	gitOK(t, repoPath, "checkout", "-b", "feature")
	writeFile(t, repoPath, "both.txt", "from feature\n")
	gitOK(t, repoPath, "add", "both.txt")
	gitOK(t, repoPath, "commit", "-m", "add both.txt on feature")

	gitOK(t, repoPath, "checkout", "-")
	writeFile(t, repoPath, "both.txt", "from base\n")
	gitOK(t, repoPath, "add", "both.txt")
	gitOK(t, repoPath, "commit", "-m", "add both.txt on base")

	gitAny(t, repoPath, "merge", "feature")

	// Confirm the fixture is what it claims to be: an AA record and no code
	// containing a U, so passing this test cannot be explained by the old logic.
	codes := statusCodes(t, repoPath)
	if !slices.Contains(codes, "AA") {
		t.Fatalf("fixture did not produce an AA record; codes = %q", codes)
	}

	for _, c := range codes {
		if strings.Contains(c, "U") {
			t.Fatalf("fixture also produced %q, which the old U-only scan would have caught; codes = %q", c, codes)
		}
	}

	ps := collectPostSyncStatus(context.Background(), repoPath)
	if ps.StatusErr != nil {
		t.Fatalf("StatusErr = %v, want nil", ps.StatusErr)
	}

	if !ps.HasConflicts {
		t.Errorf("HasConflicts = false for codes %q, want true", codes)
	}

	if got, want := FormatCompactStatus(ps), "conflict"; !strings.Contains(got, want) {
		t.Errorf("FormatCompactStatus() = %q, want it to contain %q", got, want)
	}
}

// TestCollectPostSyncStatusDetectsDDConflict covers "both deleted", the other
// unmerged code with no U in it.
//
// It arises from a rename/rename conflict: each side moved the same file
// somewhere else, so the original path is deleted on both sides.
func TestCollectPostSyncStatusDetectsDDConflict(t *testing.T) {
	repoPath := testutil.TempGitRepoWithCommit(t)

	writeFile(t, repoPath, "orig.txt", "content\n")
	gitOK(t, repoPath, "add", "orig.txt")
	gitOK(t, repoPath, "commit", "-m", "add orig.txt")

	gitOK(t, repoPath, "checkout", "-b", "feature")
	gitOK(t, repoPath, "mv", "orig.txt", "a.txt")
	gitOK(t, repoPath, "commit", "-m", "rename to a.txt")

	gitOK(t, repoPath, "checkout", "-")
	gitOK(t, repoPath, "mv", "orig.txt", "b.txt")
	gitOK(t, repoPath, "commit", "-m", "rename to b.txt")

	gitAny(t, repoPath, "merge", "feature")

	codes := statusCodes(t, repoPath)
	if !slices.Contains(codes, "DD") {
		t.Fatalf("fixture did not produce a DD record; codes = %q", codes)
	}

	ps := collectPostSyncStatus(context.Background(), repoPath)
	if ps.StatusErr != nil {
		t.Fatalf("StatusErr = %v, want nil", ps.StatusErr)
	}

	if !ps.HasConflicts {
		t.Errorf("HasConflicts = false for codes %q, want true", codes)
	}
}

// TestCollectPostSyncStatusReportsUnreadableStatus verifies that a status which
// could not be read is not reported as a clean working tree.
//
// IsDirty and HasConflicts both default to false, so swallowing the error left
// a PostSyncStatus indistinguishable from a healthy repository — the badge
// rendered nothing at all, which a user reads as "no problems here".
func TestCollectPostSyncStatusReportsUnreadableStatus(t *testing.T) {
	notARepo := t.TempDir()

	ps := collectPostSyncStatus(context.Background(), notARepo)
	if ps == nil {
		t.Fatal("collectPostSyncStatus() = nil, want a status")
	}

	if ps.StatusErr == nil {
		t.Fatal("StatusErr = nil for a path that is not a repository, want an error")
	}

	if got := FormatCompactStatus(ps); got != "unknown" {
		t.Errorf("FormatCompactStatus() = %q, want %q", got, "unknown")
	}
}
