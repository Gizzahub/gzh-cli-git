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
