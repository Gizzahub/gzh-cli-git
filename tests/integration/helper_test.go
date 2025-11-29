package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRepo represents a temporary test repository
type TestRepo struct {
	Path string
	T    *testing.T
}

// NewTestRepo creates a temporary Git repository for testing
func NewTestRepo(t *testing.T) *TestRepo {
	t.Helper()

	tmpDir := t.TempDir()

	// Initialize Git repository
	cmd := exec.Command("git", "init")
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
		cmd := exec.Command("git", args...)
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

// WriteFile writes content to a file in the repository
func (r *TestRepo) WriteFile(path, content string) {
	r.T.Helper()

	fullPath := filepath.Join(r.Path, path)
	dir := filepath.Dir(fullPath)

	if err := os.MkdirAll(dir, 0755); err != nil {
		r.T.Fatalf("Failed to create directory: %v", err)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		r.T.Fatalf("Failed to write file: %v", err)
	}
}

// GitAdd stages files
func (r *TestRepo) GitAdd(files ...string) {
	r.T.Helper()

	args := append([]string{"add"}, files...)
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Path
	if err := cmd.Run(); err != nil {
		r.T.Fatalf("Failed to git add: %v", err)
	}
}

// GitCommit creates a commit
func (r *TestRepo) GitCommit(message string) {
	r.T.Helper()

	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = r.Path
	if err := cmd.Run(); err != nil {
		r.T.Fatalf("Failed to git commit: %v", err)
	}
}

// GitBranch creates a branch
func (r *TestRepo) GitBranch(name string) {
	r.T.Helper()

	cmd := exec.Command("git", "branch", name)
	cmd.Dir = r.Path
	if err := cmd.Run(); err != nil {
		r.T.Fatalf("Failed to create branch: %v", err)
	}
}

// GitCheckout checks out a branch
func (r *TestRepo) GitCheckout(ref string) {
	r.T.Helper()

	cmd := exec.Command("git", "checkout", ref)
	cmd.Dir = r.Path
	output, err := cmd.CombinedOutput()
	if err != nil {
		r.T.Fatalf("Failed to checkout %s: %v\n%s", ref, err, output)
	}
}

// SetupWithCommits creates a repository with initial commits
func (r *TestRepo) SetupWithCommits() {
	r.T.Helper()

	// Create initial commit
	r.WriteFile("README.md", "# Test Repository\n")
	r.GitAdd("README.md")
	r.GitCommit("Initial commit")

	// Ensure we're on master branch (not detached HEAD)
	cmd := exec.Command("git", "checkout", "-B", "master")
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

// RunGzhGit executes gzh-git command in the repository
func (r *TestRepo) RunGzhGit(args ...string) (string, error) {
	r.T.Helper()

	// Find gzh-git binary
	binary := findGzhGitBinary(r.T)

	cmd := exec.Command(binary, args...)
	cmd.Dir = r.Path
	output, err := cmd.CombinedOutput()

	return string(output), err
}

// RunGzhGitSuccess runs gzh-git and expects success
func (r *TestRepo) RunGzhGitSuccess(args ...string) string {
	r.T.Helper()

	output, err := r.RunGzhGit(args...)
	if err != nil {
		r.T.Fatalf("Command failed: gzh-git %v\nError: %v\nOutput: %s",
			args, err, output)
	}

	return output
}

// RunGzhGitExpectError runs gzh-git and expects an error
func (r *TestRepo) RunGzhGitExpectError(args ...string) string {
	r.T.Helper()

	output, err := r.RunGzhGit(args...)
	if err == nil {
		r.T.Fatalf("Expected command to fail but it succeeded: gzh-git %v\nOutput: %s",
			args, output)
	}

	return output
}

// AssertContains checks if output contains expected string
func AssertContains(t *testing.T, output, expected string) {
	t.Helper()

	if !strings.Contains(output, expected) {
		t.Errorf("Output does not contain expected string\nExpected: %q\nGot: %s",
			expected, output)
	}
}

// AssertNotContains checks if output does not contain a string
func AssertNotContains(t *testing.T, output, unexpected string) {
	t.Helper()

	if strings.Contains(output, unexpected) {
		t.Errorf("Output contains unexpected string\nUnexpected: %q\nGot: %s",
			unexpected, output)
	}
}

// findGzhGitBinary locates the gzh-git binary
func findGzhGitBinary(t *testing.T) string {
	t.Helper()

	// Check if binary exists in various locations
	candidates := []string{
		"../../gzh-git",        // Root directory (where make build creates it)
		"../../build/gzh-git",  // Build directory (alternative location)
		"../../tmp/gzh-git",    // Tmp directory (alternative location)
		"gzh-git",              // In PATH
	}

	for _, candidate := range candidates {
		// For relative paths, use os.Stat
		if _, err := os.Stat(candidate); err == nil {
			abs, _ := filepath.Abs(candidate)
			return abs
		}
		// For PATH lookup, use exec.LookPath
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate
		}
	}

	// Build if not found
	t.Log("Binary not found, building...")
	buildCmd := exec.Command("make", "build")
	buildCmd.Dir = "../.."
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build gzh-git: %v", err)
	}

	return "../../gzh-git"
}

// SkipIfNoBinary skips test if gzh-git binary is not available
func SkipIfNoBinary(t *testing.T) {
	t.Helper()

	defer func() {
		if r := recover(); r != nil {
			t.Skipf("gzh-git binary not available: %v", r)
		}
	}()

	findGzhGitBinary(t)
}

// TestMain ensures binary is built before running tests
func TestMain(m *testing.M) {
	// Build the binary before running tests
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "make", "build")
	cmd.Dir = "../.."
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build gzh-git: %v\n", err)
		os.Exit(1)
	}

	// Run tests
	code := m.Run()
	os.Exit(code)
}
