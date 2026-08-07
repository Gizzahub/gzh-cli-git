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

// TestCleanupService_ExecuteSkipsProtectedEvenWhenExcludeEmpty pins the safety
// net that must not depend on Analyze: a hand-built report that lists main under
// Merged, with empty Exclude and Force+Confirm, must not delete main. Built-in
// IsProtected is applied even when Exclude is empty; Skipped surfaces the name.
//
// Manager.Delete only refuses protected branches when Force is false, so Force
// would otherwise delete main — Execute must screen first.
func TestCleanupService_ExecuteSkipsProtectedEvenWhenExcludeEmpty(t *testing.T) {
	repoPath := testutil.TempGitRepoWithCommit(t)
	repo := &repository.Repository{Path: repoPath}
	ctx := context.Background()

	// Non-protected candidate that Force+Confirm is allowed to remove.
	gitCommit(t, repoPath, "branch", "feature/safe-to-delete")
	// Ensure main exists even when init.defaultBranch is master, then leave it
	// so neither candidate is HEAD (git refuses to delete the current branch).
	gitCommit(t, repoPath, "branch", "-f", "main")
	gitCommit(t, repoPath, "checkout", "-b", "work")

	report := &CleanupReport{
		Merged: []*Branch{
			{Name: "main"},
			{Name: "feature/safe-to-delete"},
		},
	}

	svc := NewCleanupService()

	result, err := svc.Execute(ctx, repo, report, ExecuteOptions{
		Force:   true,
		Confirm: true,
		// Exclude deliberately empty — built-in IsProtected must still apply.
	})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if len(result.Deleted) != 1 || result.Deleted[0] != "feature/safe-to-delete" {
		t.Errorf("Deleted = %v, want [feature/safe-to-delete]", result.Deleted)
	}

	if len(result.Skipped) != 1 || result.Skipped[0] != "main" {
		t.Errorf("Skipped = %v, want [main]", result.Skipped)
	}

	if len(result.Failed) != 0 {
		t.Errorf("Failed = %+v, want none", result.Failed)
	}

	mgr := NewManager()

	exists, err := mgr.Exists(ctx, repo, "main")
	if err != nil {
		t.Fatalf("Exists(main) error = %v", err)
	}

	if !exists {
		t.Error("main was deleted despite built-in protection with empty Exclude")
	}

	gone, err := mgr.Exists(ctx, repo, "feature/safe-to-delete")
	if err != nil {
		t.Fatalf("Exists(feature/safe-to-delete) error = %v", err)
	}

	if gone {
		t.Error("feature/safe-to-delete should have been deleted")
	}
}

// TestCleanupService_ExecuteSeparatesDeletedFromFailed builds the one case that
// tells the two counts apart: a report of two branches where git deletes one and
// refuses the other.
//
// Before this, Execute dropped the failure and the CLI printed
// report.CountBranches() — the number of candidates — so this run announced two
// deletions. The assertions below pin the difference: Deleted is 1, Failed is 1,
// and CountBranches() is 2.
func TestCleanupService_ExecuteSeparatesDeletedFromFailed(t *testing.T) {
	repoPath := testutil.TempGitRepoWithCommit(t)
	repo := &repository.Repository{Path: repoPath}
	ctx := context.Background()

	// Merged: points at the same commit as the default branch, so `git branch -d`
	// accepts it.
	gitCommit(t, repoPath, "branch", "feature/merged")

	// Unmerged: carries a commit the default branch does not have, so `git branch
	// -d` refuses it. Confirm below skips this package's own unmerged guard, which
	// leaves git as the one saying no.
	gitCommit(t, repoPath, "checkout", "-b", "feature/unmerged")

	if err := os.WriteFile(filepath.Join(repoPath, "extra.txt"), []byte("x\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	gitCommit(t, repoPath, "add", "extra.txt")
	gitCommit(t, repoPath, "commit", "-m", "unmerged work")
	gitCommit(t, repoPath, "checkout", "-")

	report := &CleanupReport{
		Merged: []*Branch{
			{Name: "feature/merged"},
			{Name: "feature/unmerged"},
		},
	}

	svc := NewCleanupService()

	result, err := svc.Execute(ctx, repo, report, ExecuteOptions{Confirm: true})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil — one branch failing must not end the run", err)
	}

	if len(result.Deleted) != 1 || result.Deleted[0] != "feature/merged" {
		t.Errorf("Deleted = %v, want [feature/merged]", result.Deleted)
	}

	if len(result.Failed) != 1 || result.Failed[0].Branch != "feature/unmerged" {
		t.Fatalf("Failed = %+v, want one entry for feature/unmerged", result.Failed)
	}

	if result.Failed[0].Err == nil {
		t.Error("Failed entry carries a nil error, so nothing can be printed for it")
	}

	// The count the CLI used to print.
	if report.CountBranches() != 2 {
		t.Fatalf("CountBranches() = %d, want 2", report.CountBranches())
	}

	if len(result.Deleted) == report.CountBranches() {
		t.Error("deletion count still equals the candidate count, so the two are indistinguishable")
	}

	// The refused branch is still there.
	mgr := NewManager()

	exists, err := mgr.Exists(ctx, repo, "feature/unmerged")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}

	if !exists {
		t.Error("feature/unmerged is gone, but Execute reported it as failed")
	}
}
