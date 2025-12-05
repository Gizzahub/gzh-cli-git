// Package testutil provides Git-specific testing utilities.
// Common test utilities are available from gzh-cli-core/testutil.
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

	// Initialize git repo.
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Configure git user for commits.
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = dir
	cmd.Run()

	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = dir
	cmd.Run()

	return dir
}

// TempGitRepoWithCommit creates a temp git repo with an initial commit.
func TempGitRepoWithCommit(t *testing.T) string {
	t.Helper()
	dir := TempGitRepo(t)

	// Create a file and commit.
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test"), 0o644); err != nil {
		t.Fatalf("failed to create README: %v", err)
	}

	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = dir
	cmd.Run()

	return dir
}

// TempGitRepoWithBranch creates a temp git repo with an initial commit and a branch.
func TempGitRepoWithBranch(t *testing.T, branchName string) string {
	t.Helper()
	dir := TempGitRepoWithCommit(t)

	cmd := exec.Command("git", "checkout", "-b", branchName)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to create branch %s: %v", branchName, err)
	}

	return dir
}
