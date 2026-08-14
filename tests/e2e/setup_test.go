package e2e

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var e2eBinaryPath string

// E2ERepo represents an E2E test repository.
type E2ERepo struct {
	t          *testing.T
	rootDir    string // Original working directory
	repoDir    string // Test repository directory
	binaryPath string
}

// TestMain builds one binary for this package in a private temporary
// directory. The E2E tests all share this immutable binary.
func TestMain(m *testing.M) {
	testDir, err := os.MkdirTemp("", "gzh-cli-gitforge-e2e-tests-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create E2E test directory: %v\n", err)
		os.Exit(1)
	}

	goExe, err := goExecutableSuffix()
	if err != nil {
		_ = os.RemoveAll(testDir)
		fmt.Fprintf(os.Stderr, "failed to determine executable suffix: %v\n", err)
		os.Exit(1)
	}

	e2eBinaryPath = filepath.Join(testDir, "gz-git"+goExe)
	moduleRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		_ = os.RemoveAll(testDir)
		fmt.Fprintf(os.Stderr, "failed to determine module root: %v\n", err)
		os.Exit(1)
	}

	buildCmd := exec.Command("go", "build", "-o", e2eBinaryPath, "./cmd/gz-git") //nolint:noctx // package setup
	buildCmd.Dir = moduleRoot
	if output, err := buildCmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(testDir)
		fmt.Fprintf(os.Stderr, "failed to build gz-git: %v\n%s", err, output)
		os.Exit(1)
	}

	code := m.Run()
	if err := os.RemoveAll(testDir); err != nil {
		fmt.Fprintf(os.Stderr, "failed to remove E2E test directory: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

// NewE2ERepo creates a new E2E test repository.
func NewE2ERepo(t *testing.T) *E2ERepo {
	t.Helper()

	// Get current working directory
	rootDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// Create temp directory for test repo
	repoDir := t.TempDir()

	// Find or build gz-git binary
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

// findOrBuildBinary returns the package-scoped binary prepared by TestMain.
func findOrBuildBinary(t *testing.T) string {
	t.Helper()

	if e2eBinaryPath == "" {
		t.Fatal("gz-git E2E test binary was not initialized")
	}
	return e2eBinaryPath
}

func goExecutableSuffix() (string, error) {
	cmd := exec.Command("go", "env", "GOEXE") //nolint:noctx // package setup
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// runCommand runs a command in the specified directory.
//
//nolint:unparam // generic command runner; name kept for clarity
func (r *E2ERepo) runCommand(dir, name string, args ...string) string {
	r.t.Helper()

	cmd := exec.Command(name, args...) //nolint:noctx // test helper, no context needed
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("Command failed: %s %v\nError: %v\nOutput: %s",
			name, args, err, output)
	}
	return string(output)
}

// RunGzhGit runs gz-git command and expects success.
func (r *E2ERepo) RunGzhGit(args ...string) string {
	r.t.Helper()

	cmd := exec.Command(r.binaryPath, args...) //nolint:noctx // test helper, no context needed
	cmd.Dir = r.repoDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("gz-git command failed: %v %v\nError: %v\nOutput: %s",
			r.binaryPath, args, err, output)
	}
	return string(output)
}

// RunGzhGitExpectError runs gz-git command and expects failure.
func (r *E2ERepo) RunGzhGitExpectError(args ...string) string {
	r.t.Helper()

	cmd := exec.Command(r.binaryPath, args...) //nolint:noctx // test helper, no context needed
	cmd.Dir = r.repoDir
	output, err := cmd.CombinedOutput()
	if err == nil {
		r.t.Fatalf("Expected gz-git to fail but it succeeded: %v %v\nOutput: %s",
			r.binaryPath, args, output)
	}
	return string(output)
}

// Git runs a git command.
func (r *E2ERepo) Git(args ...string) string {
	r.t.Helper()
	return r.runCommand(r.repoDir, "git", args...)
}

// WriteFile writes content to a file in the repository.
func (r *E2ERepo) WriteFile(path, content string) {
	r.t.Helper()

	fullPath := filepath.Join(r.repoDir, path)
	dir := filepath.Dir(fullPath)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.t.Fatalf("Failed to create directory %s: %v", dir, err)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		r.t.Fatalf("Failed to write file %s: %v", path, err)
	}
}

// ReadFile reads content from a file in the repository.
func (r *E2ERepo) ReadFile(path string) string {
	r.t.Helper()

	fullPath := filepath.Join(r.repoDir, path)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		r.t.Fatalf("Failed to read file %s: %v", path, err)
	}
	return string(content)
}

// FileExists checks if a file exists.
func (r *E2ERepo) FileExists(path string) bool {
	r.t.Helper()

	fullPath := filepath.Join(r.repoDir, path)
	_, err := os.Stat(fullPath)
	return err == nil
}

// CommitExists checks if a commit with given message exists.
func (r *E2ERepo) CommitExists(message string) bool {
	r.t.Helper()

	output := r.Git("log", "--all", "--oneline", "--grep", message)
	return strings.Contains(output, message)
}

// GetCurrentBranch returns the current branch name.
func (r *E2ERepo) GetCurrentBranch() string {
	r.t.Helper()

	output := r.Git("rev-parse", "--abbrev-ref", "HEAD")
	return strings.TrimSpace(output)
}

// BranchExists checks if a branch exists.
func (r *E2ERepo) BranchExists(name string) bool {
	r.t.Helper()

	output := r.Git("branch", "--list", name)
	return strings.Contains(output, name)
}

// AssertContains checks if output contains expected string.
func AssertContains(t *testing.T, output, expected string) {
	t.Helper()

	if !strings.Contains(output, expected) {
		t.Errorf("Expected output to contain %q, but it didn't.\nOutput: %s",
			expected, output)
	}
}

// AssertNotContains checks if output does not contain expected string.
func AssertNotContains(t *testing.T, output, expected string) {
	t.Helper()

	if strings.Contains(output, expected) {
		t.Errorf("Expected output NOT to contain %q, but it did.\nOutput: %s",
			expected, output)
	}
}

// AssertExitCode checks if command exited with expected code.
func AssertExitCode(t *testing.T, err error, expectedCode int) {
	t.Helper()

	exitErr := &exec.ExitError{}
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() != expectedCode {
			t.Errorf("Expected exit code %d, got %d", expectedCode, exitErr.ExitCode())
		}
	} else if err == nil && expectedCode != 0 {
		t.Errorf("Expected exit code %d, got 0 (success)", expectedCode)
	}
}
