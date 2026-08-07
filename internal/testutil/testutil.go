// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

// Package testutil provides helpers for creating temporary git repositories in tests.
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TempGitRepo creates a temporary git repository.
// Returns the repository path. Automatically cleaned up.
func TempGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Initialize git repo with an explicit default branch so callers that
	// hard-code branch names are not coupled to the developer's global
	// init.defaultBranch.
	cmd := exec.Command("git", "-c", "init.defaultBranch=main", "init") //nolint:noctx // test helper; no context.Context available in *testing.T API
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Local config only — never depend on the developer's global git settings.
	// commit.gpgsign=false avoids silent commit failure when the user has signing on.
	for _, args := range [][]string{
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd = exec.Command("git", args...) //nolint:noctx // test helper; no context.Context available
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to config git %v: %v", args, err)
		}
	}

	return dir
}

// TempGitRepoWithCommit creates a temp git repo with an initial commit.
// Any setup failure is fatal: a helper named ...WithCommit must not hand back
// a HEAD-less repository and let the test exercise a different state.
func TempGitRepoWithCommit(t *testing.T) string {
	t.Helper()
	dir := TempGitRepo(t)

	// Create a file and commit.
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test"), 0o600); err != nil { //nolint:gosec // 0o600 satisfies G306; test file needs no broader access
		t.Fatalf("failed to create README: %v", err)
	}

	cmd := exec.Command("git", "add", ".") //nolint:noctx // test helper; no context.Context available
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit") //nolint:noctx // test helper; no context.Context available
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	return dir
}

// TempGitRepoWithBranch creates a temp git repo with an initial commit and a branch.
func TempGitRepoWithBranch(t *testing.T, branchName string) string {
	t.Helper()
	dir := TempGitRepoWithCommit(t)

	cmd := exec.Command("git", "checkout", "-b", branchName) //nolint:noctx // test helper; no context.Context available
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to create branch %s: %v", branchName, err)
	}

	return dir
}
