package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestTempGitRepo(t *testing.T) {
	dir := TempGitRepo(t)

	// Check .git directory exists.
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Error(".git directory should exist in TempGitRepo")
	}

	// Check git config is set.
	cmd := exec.Command("git", "config", "user.email") //nolint:noctx // test verification helper; no context.Context available in *testing.T API
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		t.Errorf("git config user.email should be set: %v", err)
	}
	if string(output) != "test@test.com\n" {
		t.Errorf("git config user.email = %q, want %q", string(output), "test@test.com\n")
	}
}

func TestTempGitRepoWithCommit(t *testing.T) {
	dir := TempGitRepoWithCommit(t)

	// Check README exists.
	readme := filepath.Join(dir, "README.md")
	if _, err := os.Stat(readme); os.IsNotExist(err) {
		t.Error("README.md should exist in TempGitRepoWithCommit")
	}

	// Check .git directory exists.
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Error(".git directory should exist")
	}

	// Check commit exists — helper must not swallow commit failures.
	cmd := exec.Command("git", "log", "--oneline", "-1") //nolint:noctx // test verification helper; no context.Context available
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		t.Errorf("git log should work: %v", err)
	}
	if len(output) == 0 {
		t.Error("should have at least one commit")
	}

	// Local commit.gpgsign must be off so developer global signing cannot
	// turn TempGitRepoWithCommit into a silent HEAD-less fixture.
	cmd = exec.Command("git", "config", "--get", "commit.gpgsign") //nolint:noctx // test verification helper
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("commit.gpgsign should be set locally: %v", err)
	}
	if string(out) != "false\n" {
		t.Errorf("commit.gpgsign = %q, want false", string(out))
	}
}

func TestTempGitRepoWithBranch(t *testing.T) {
	branchName := "feature/test"
	dir := TempGitRepoWithBranch(t, branchName)

	// Check branch exists and is current.
	cmd := exec.Command("git", "branch", "--show-current") //nolint:noctx // test verification helper; no context.Context available
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		t.Errorf("git branch should work: %v", err)
	}
	if string(output) != branchName+"\n" {
		t.Errorf("current branch = %q, want %q", string(output), branchName+"\n")
	}
}
