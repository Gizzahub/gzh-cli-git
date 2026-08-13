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

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)

// TestProcessSwitchRepositorySkipsDirty is the data-protection gate for bulk
// switch: a dirty working tree must not be force-switched without Force.
// Mutation: remove the !status.IsClean check and this fails with Status switched.
func TestProcessSwitchRepositorySkipsDirty(t *testing.T) {
	repoPath := testutil.TempGitRepoWithCommit(t)

	// Create a second branch, then return to main so the switch has work to do.
	createBranch(t, repoPath, "feature")
	runGitIn(t, repoPath, "checkout", "main")

	// Dirty the tree on main — the gate must refuse the switch.
	if err := os.WriteFile(filepath.Join(repoPath, "dirty.txt"), []byte("wip"), 0o600); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}

	ctx := context.Background()
	c, ok := NewClient().(*client)
	if !ok {
		t.Fatal("NewClient did not return *client")
	}

	result := c.processSwitchRepository(ctx, filepath.Dir(repoPath), repoPath, BulkSwitchOptions{
		Branch: "feature",
		Force:  false,
		Logger: NewNoopLogger(),
	}, NewNoopLogger())

	if result.Status != StatusDirty {
		t.Fatalf("status = %q, want %q (message=%q err=%v)", result.Status, StatusDirty, result.Message, result.Error)
	}
	if !result.HasUncommittedChanges {
		t.Error("HasUncommittedChanges should be true for a dirty tree")
	}

	// Working tree must still be on the original branch.
	branch := currentBranch(t, repoPath)
	if branch == "feature" {
		t.Fatalf("dirty repo was switched to feature; gate failed")
	}
}

// TestProcessSwitchRepositorySwitchesClean proves the happy path so the dirty
// test is not the only fixture covering processSwitchRepository.
func TestProcessSwitchRepositorySwitchesClean(t *testing.T) {
	repoPath := testutil.TempGitRepoWithCommit(t)
	createBranch(t, repoPath, "feature")
	// Return to main so switch has work to do.
	runGitIn(t, repoPath, "checkout", "main")

	ctx := context.Background()
	c, ok := NewClient().(*client)
	if !ok {
		t.Fatal("NewClient did not return *client")
	}

	result := c.processSwitchRepository(ctx, filepath.Dir(repoPath), repoPath, BulkSwitchOptions{
		Branch: "feature",
		Force:  false,
		Logger: NewNoopLogger(),
	}, NewNoopLogger())

	if result.Status == StatusDirty {
		t.Fatalf("clean repo reported dirty: %q", result.Message)
	}
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if currentBranch(t, repoPath) != "feature" {
		t.Fatalf("expected switch to feature, on %q (status=%q msg=%q)",
			currentBranch(t, repoPath), result.Status, result.Message)
	}
}

// TestProcessStatusRepositoryReportsDirty covers the user-facing status path.
// Mutation: force IsClean=true after GetStatus and this fails with StatusClean.
func TestProcessStatusRepositoryReportsDirty(t *testing.T) {
	repoPath := testutil.TempGitRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(repoPath, "extra.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	ctx := context.Background()
	c, ok := NewClient().(*client)
	if !ok {
		t.Fatal("NewClient did not return *client")
	}

	result := c.processStatusRepository(ctx, filepath.Dir(repoPath), repoPath, BulkStatusOptions{
		Logger: NewNoopLogger(),
	}, NewNoopLogger())

	if result.Status != StatusDirty && result.Status != StatusNoRemote {
		// No remote is fine if the dirty message is still attached; prefer dirty.
		// Temp fixtures have no remote, so StatusDirty (no remote branch) or
		// StatusNoRemote with dirty message are both acceptable classifications
		// as long as clean is not reported.
		if result.Status == StatusClean {
			t.Fatalf("dirty repo reported clean: status=%q message=%q", result.Status, result.Message)
		}
	}
	if result.UntrackedFiles < 1 && result.TrackedChangedFiles < 1 {
		t.Fatalf("expected dirty counts populated, got untracked=%d tracked=%d status=%q msg=%q",
			result.UntrackedFiles, result.TrackedChangedFiles, result.Status, result.Message)
	}
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
}

// TestProcessStatusRepositoryReportsClean is the inverse fixture.
func TestProcessStatusRepositoryReportsClean(t *testing.T) {
	repoPath := testutil.TempGitRepoWithCommit(t)

	ctx := context.Background()
	c, ok := NewClient().(*client)
	if !ok {
		t.Fatal("NewClient did not return *client")
	}

	result := c.processStatusRepository(ctx, filepath.Dir(repoPath), repoPath, BulkStatusOptions{
		Logger: NewNoopLogger(),
	}, NewNoopLogger())

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.Status == StatusDirty {
		t.Fatalf("clean repo reported dirty: %q", result.Message)
	}
	if result.UntrackedFiles != 0 || result.TrackedChangedFiles != 0 {
		t.Fatalf("expected zero dirty counts, got untracked=%d tracked=%d",
			result.UntrackedFiles, result.TrackedChangedFiles)
	}
}

func createBranch(t *testing.T, repoPath, name string) {
	t.Helper()
	runGitIn(t, repoPath, "checkout", "-b", name)
}

func currentBranch(t *testing.T, repoPath string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD") //nolint:noctx // test helper
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func runGitIn(t *testing.T, repoPath string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:noctx // test helper
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
