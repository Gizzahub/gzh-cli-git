package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var integrationBinaryPath string

// TestRepo represents a temporary test repository.
type TestRepo struct {
	Path string
	T    *testing.T
}

// NewTestRepo creates a temporary Git repository for testing.
func NewTestRepo(t *testing.T) *TestRepo {
	t.Helper()

	tmpDir := t.TempDir()

	// Initialize Git repository
	cmd := exec.Command("git", "init") //nolint:noctx // test helper, no context needed
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure Git user (required for commits)
	configCmds := [][]string{
		{"config", "user.name", "Test User"},
		{"config", "user.email", "test@example.com"},
	}

	for _, args := range configCmds {
		cmd := exec.Command("git", args...) //nolint:noctx // test helper, no context needed
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to configure git: %v", err)
		}
	}

	return &TestRepo{
		Path: tmpDir,
		T:    t,
	}
}

// WriteFile writes content to a file in the repository.
func (r *TestRepo) WriteFile(path, content string) {
	r.T.Helper()

	fullPath := filepath.Join(r.Path, path)
	dir := filepath.Dir(fullPath)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.T.Fatalf("Failed to create directory: %v", err)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		r.T.Fatalf("Failed to write file: %v", err)
	}
}

// GitAdd stages files.
func (r *TestRepo) GitAdd(files ...string) {
	r.T.Helper()

	args := append([]string{"add"}, files...)
	cmd := exec.Command("git", args...) //nolint:noctx // test helper, no context needed
	cmd.Dir = r.Path
	if err := cmd.Run(); err != nil {
		r.T.Fatalf("Failed to git add: %v", err)
	}
}

// GitCommit creates a commit.
func (r *TestRepo) GitCommit(message string) {
	r.T.Helper()

	cmd := exec.Command("git", "commit", "-m", message) //nolint:noctx // test helper, no context needed
	cmd.Dir = r.Path
	if err := cmd.Run(); err != nil {
		r.T.Fatalf("Failed to git commit: %v", err)
	}
}

// GitBranch creates a branch.
func (r *TestRepo) GitBranch(name string) {
	r.T.Helper()

	cmd := exec.Command("git", "branch", name) //nolint:noctx // test helper, no context needed
	cmd.Dir = r.Path
	if err := cmd.Run(); err != nil {
		r.T.Fatalf("Failed to create branch: %v", err)
	}
}

// GitCheckout checks out a branch.
func (r *TestRepo) GitCheckout(ref string) {
	r.T.Helper()

	cmd := exec.Command("git", "checkout", ref) //nolint:noctx // test helper, no context needed
	cmd.Dir = r.Path
	output, err := cmd.CombinedOutput()
	if err != nil {
		r.T.Fatalf("Failed to checkout %s: %v\n%s", ref, err, output)
	}
}

// SetupWithCommits creates a repository with initial commits.
func (r *TestRepo) SetupWithCommits() {
	r.T.Helper()

	// Create initial commit
	r.WriteFile("README.md", "# Test Repository\n")
	r.GitAdd("README.md")
	r.GitCommit("Initial commit")

	// Ensure we're on master branch (not detached HEAD)
	cmd := exec.Command("git", "checkout", "-B", "master") //nolint:noctx // test helper, no context needed
	cmd.Dir = r.Path
	cmd.Run() // Ignore error as we might already be on master

	// Create second commit
	r.WriteFile("main.go", "package main\n\nfunc main() {}\n")
	r.GitAdd("main.go")
	r.GitCommit("Add main.go")

	// Create third commit
	r.WriteFile("README.md", "# Test Repository\n\nUpdated content\n")
	r.GitAdd("README.md")
	r.GitCommit("Update README")
}

// RunGzhGit executes gz-git command in the repository.
func (r *TestRepo) RunGzhGit(args ...string) (string, error) {
	r.T.Helper()

	// Find gz-git binary
	binary := findGzhGitBinary(r.T)

	cmd := exec.Command(binary, args...) //nolint:noctx // test helper, no context needed
	cmd.Dir = r.Path
	output, err := cmd.CombinedOutput()

	return string(output), err
}

// RunGzhGitSuccess runs gz-git and expects success.
func (r *TestRepo) RunGzhGitSuccess(args ...string) string {
	r.T.Helper()

	output, err := r.RunGzhGit(args...)
	if err != nil {
		r.T.Fatalf("Command failed: gz-git %v\nError: %v\nOutput: %s",
			args, err, output)
	}

	return output
}

// RunGzhGitExpectError runs gz-git and expects an error.
func (r *TestRepo) RunGzhGitExpectError(args ...string) string {
	r.T.Helper()

	output, err := r.RunGzhGit(args...)
	if err == nil {
		r.T.Fatalf("Expected command to fail but it succeeded: gz-git %v\nOutput: %s",
			args, output)
	}

	return output
}

// AssertContains checks if output contains expected string.
func AssertContains(t *testing.T, output, expected string) {
	t.Helper()

	if !strings.Contains(output, expected) {
		t.Errorf("Output does not contain expected string\nExpected: %q\nGot: %s",
			expected, output)
	}
}

// AssertNotContains checks if output does not contain a string.
func AssertNotContains(t *testing.T, output, unexpected string) {
	t.Helper()

	if strings.Contains(output, unexpected) {
		t.Errorf("Output contains unexpected string\nUnexpected: %q\nGot: %s",
			unexpected, output)
	}
}

// findGzhGitBinary returns the package-scoped binary prepared by TestMain.
func findGzhGitBinary(t *testing.T) string {
	t.Helper()

	if integrationBinaryPath == "" {
		t.Fatal("gz-git integration test binary was not initialized")
	}
	return integrationBinaryPath
}

// SkipIfNoBinary skips test if gz-git binary is not available.
func SkipIfNoBinary(t *testing.T) {
	t.Helper()

	defer func() {
		if r := recover(); r != nil {
			t.Skipf("gz-git binary not available: %v", r)
		}
	}()

	findGzhGitBinary(t)
}

// TestMain ensures binary is built before running tests.
func TestMain(m *testing.M) {
	testDir, err := os.MkdirTemp("", "gzh-cli-gitforge-integration-tests-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create integration test directory: %v\n", err)
		os.Exit(1)
	}

	goExe, err := goExecutableSuffix()
	if err != nil {
		_ = os.RemoveAll(testDir)
		fmt.Fprintf(os.Stderr, "failed to determine executable suffix: %v\n", err)
		os.Exit(1)
	}

	integrationBinaryPath = filepath.Join(testDir, "gz-git"+goExe)
	moduleRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		_ = os.RemoveAll(testDir)
		fmt.Fprintf(os.Stderr, "failed to determine module root: %v\n", err)
		os.Exit(1)
	}

	buildCmd := exec.Command("go", "build", "-o", integrationBinaryPath, "./cmd/gz-git") //nolint:noctx // package setup
	buildCmd.Dir = moduleRoot
	if output, err := buildCmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(testDir)
		fmt.Fprintf(os.Stderr, "failed to build gz-git: %v\n%s", err, output)
		os.Exit(1)
	}

	code := m.Run()
	if err := os.RemoveAll(testDir); err != nil {
		fmt.Fprintf(os.Stderr, "failed to remove integration test directory: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

func goExecutableSuffix() (string, error) {
	cmd := exec.Command("go", "env", "GOEXE") //nolint:noctx // package setup
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
