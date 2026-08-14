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

func TestTempWorktreeWithBareOrigin(t *testing.T) {
	fx := TempWorktreeWithBareOrigin(t)

	if fx.Remote != "origin" {
		t.Errorf("Remote = %q, want origin", fx.Remote)
	}

	assertGitOutput(t, fx.Origin, []string{"rev-parse", "--is-bare-repository"}, "true\n")
	assertGitOutput(t, fx.Clone, []string{"rev-parse", "--is-bare-repository"}, "false\n")

	if _, err := os.Stat(filepath.Join(fx.Worktree, ".git")); err != nil {
		t.Fatalf("worktree .git missing: %v", err)
	}

	assertGitOutput(t, fx.Clone, []string{"remote"}, fx.Remote+"\n")
	assertGitOutput(t, fx.Clone, []string{"config", "--get", "commit.gpgsign"}, "false\n")
	assertGitOutput(t, fx.Worktree, []string{"config", "--get", "commit.gpgsign"}, "false\n")
	assertGitOutput(t, fx.Worktree, []string{"branch", "--show-current"}, "feature/worktree\n")

	// Linked worktree: git-dir is not the common dir.
	gitDir := gitOutput(t, fx.Worktree, "rev-parse", "--git-dir")
	commonDir := gitOutput(t, fx.Worktree, "rev-parse", "--git-common-dir")
	if gitDir == commonDir {
		t.Errorf("worktree git-dir = common-dir %q, want a linked worktree", gitDir)
	}

	readme := filepath.Join(fx.Worktree, "from-worktree.md")
	if err := os.WriteFile(readme, []byte("wt\n"), 0o600); err != nil {
		t.Fatalf("write worktree file: %v", err)
	}
	runGit(t, fx.Worktree, "add", "from-worktree.md")
	runGit(t, fx.Worktree, "commit", "-m", "worktree commit")
}

func TestTempWorktreeWithBareOriginRemote(t *testing.T) {
	fx := TempWorktreeWithBareOriginRemote(t, "upstream")
	if fx.Remote != "upstream" {
		t.Fatalf("Remote = %q, want upstream", fx.Remote)
	}
	assertGitOutput(t, fx.Clone, []string{"remote"}, "upstream\n")
	if got := gitOutput(t, fx.Clone, "remote", "get-url", "upstream"); got == "" {
		t.Fatal("upstream remote URL is empty")
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:noctx // test verification helper; no context.Context available
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}

func assertGitOutput(t *testing.T, dir string, args []string, want string) {
	t.Helper()
	got := gitOutput(t, dir, args...)
	if got != want {
		t.Errorf("git %v = %q, want %q", args, got, want)
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
