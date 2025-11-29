package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// E2ERepo represents an E2E test repository
type E2ERepo struct {
	t        *testing.T
	rootDir  string // Original working directory
	repoDir  string // Test repository directory
	binaryPath string
}

// NewE2ERepo creates a new E2E test repository
func NewE2ERepo(t *testing.T) *E2ERepo {
	t.Helper()

	// Get current working directory
	rootDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// Create temp directory for test repo
	repoDir := t.TempDir()

	// Find or build gzh-git binary
	binaryPath := findOrBuildBinary(t)

	repo := &E2ERepo{
		t:          t,
		rootDir:    rootDir,
		repoDir:    repoDir,
		binaryPath: binaryPath,
	}

	// Initialize git repository
	repo.runCommand(repoDir, "git", "init")
	repo.runCommand(repoDir, "git", "config", "user.name", "E2E Test User")
	repo.runCommand(repoDir, "git", "config", "user.email", "e2e@example.com")

	return repo
}

// findOrBuildBinary locates the gzh-git binary or builds it if necessary
func findOrBuildBinary(t *testing.T) string {
	t.Helper()

	// Try to find existing binary
	if _, err := os.Stat("../../gzh-git"); err == nil {
		abs, _ := filepath.Abs("../../gzh-git")
		return abs
	}

	// Build the binary
	t.Log("Building gzh-git binary for E2E tests...")
	cmd := exec.Command("go", "build", "-o", "gzh-git", "./cmd/gzh-git")
	cmd.Dir = "../../"
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build gzh-git: %v\nOutput: %s", err, output)
	}

	abs, _ := filepath.Abs("../../gzh-git")
	return abs
}

// runCommand runs a command in the specified directory
func (r *E2ERepo) runCommand(dir string, name string, args ...string) string {
	r.t.Helper()

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("Command failed: %s %v\nError: %v\nOutput: %s",
			name, args, err, output)
	}
	return string(output)
}

// RunGzhGit runs gzh-git command and expects success
func (r *E2ERepo) RunGzhGit(args ...string) string {
	r.t.Helper()

	cmd := exec.Command(r.binaryPath, args...)
	cmd.Dir = r.repoDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("gzh-git command failed: %v %v\nError: %v\nOutput: %s",
			r.binaryPath, args, err, output)
	}
	return string(output)
}

// RunGzhGitExpectError runs gzh-git command and expects failure
func (r *E2ERepo) RunGzhGitExpectError(args ...string) string {
	r.t.Helper()

	cmd := exec.Command(r.binaryPath, args...)
	cmd.Dir = r.repoDir
	output, err := cmd.CombinedOutput()
	if err == nil {
		r.t.Fatalf("Expected gzh-git to fail but it succeeded: %v %v\nOutput: %s",
			r.binaryPath, args, output)
	}
	return string(output)
}

// Git runs a git command
func (r *E2ERepo) Git(args ...string) string {
	r.t.Helper()
	return r.runCommand(r.repoDir, "git", args...)
}

// WriteFile writes content to a file in the repository
func (r *E2ERepo) WriteFile(path, content string) {
	r.t.Helper()

	fullPath := filepath.Join(r.repoDir, path)
	dir := filepath.Dir(fullPath)

	if err := os.MkdirAll(dir, 0755); err != nil {
		r.t.Fatalf("Failed to create directory %s: %v", dir, err)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		r.t.Fatalf("Failed to write file %s: %v", path, err)
	}
}

// ReadFile reads content from a file in the repository
func (r *E2ERepo) ReadFile(path string) string {
	r.t.Helper()

	fullPath := filepath.Join(r.repoDir, path)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		r.t.Fatalf("Failed to read file %s: %v", path, err)
	}
	return string(content)
}

// FileExists checks if a file exists
func (r *E2ERepo) FileExists(path string) bool {
	r.t.Helper()

	fullPath := filepath.Join(r.repoDir, path)
	_, err := os.Stat(fullPath)
	return err == nil
}

// CommitExists checks if a commit with given message exists
func (r *E2ERepo) CommitExists(message string) bool {
	r.t.Helper()

	output := r.Git("log", "--all", "--oneline", "--grep", message)
	return strings.Contains(output, message)
}

// GetCurrentBranch returns the current branch name
func (r *E2ERepo) GetCurrentBranch() string {
	r.t.Helper()

	output := r.Git("rev-parse", "--abbrev-ref", "HEAD")
	return strings.TrimSpace(output)
}

// BranchExists checks if a branch exists
func (r *E2ERepo) BranchExists(name string) bool {
	r.t.Helper()

	output := r.Git("branch", "--list", name)
	return strings.Contains(output, name)
}

// AssertContains checks if output contains expected string
func AssertContains(t *testing.T, output, expected string) {
	t.Helper()

	if !strings.Contains(output, expected) {
		t.Errorf("Expected output to contain %q, but it didn't.\nOutput: %s",
			expected, output)
	}
}

// AssertNotContains checks if output does not contain expected string
func AssertNotContains(t *testing.T, output, expected string) {
	t.Helper()

	if strings.Contains(output, expected) {
		t.Errorf("Expected output NOT to contain %q, but it did.\nOutput: %s",
			expected, output)
	}
}

// AssertExitCode checks if command exited with expected code
func AssertExitCode(t *testing.T, err error, expectedCode int) {
	t.Helper()

	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != expectedCode {
			t.Errorf("Expected exit code %d, got %d", expectedCode, exitErr.ExitCode())
		}
	} else if err == nil && expectedCode != 0 {
		t.Errorf("Expected exit code %d, got 0 (success)", expectedCode)
	}
}
