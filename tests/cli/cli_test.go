// Package cli provides CLI binary integration tests for gz-git.
package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func getBinaryPath() string {
	// Get the module root directory
	return filepath.Join("..", "..", "gz-git")
}

// TestCLIVersion tests the version command.
func TestCLIVersion(t *testing.T) {
	cmd := exec.Command(getBinaryPath(), "--version") //nolint:noctx // test helper, no context needed
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run version command: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "gz-git version") {
		t.Errorf("Expected version output to contain 'gz-git version', got: %s", outputStr)
	}
}

// TestCLIHelp tests the help command.
func TestCLIHelp(t *testing.T) {
	cmd := exec.Command(getBinaryPath(), "--help") //nolint:noctx // test helper, no context needed
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run help command: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	expectedStrings := []string{
		"gz-git",
		"status",
		"info",
		"clone",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(outputStr, expected) {
			t.Errorf("Expected help output to contain '%s', got: %s", expected, outputStr)
		}
	}
}

// TestCLIStatus tests the status command on current repository.
func TestCLIStatus(t *testing.T) {
	// Change to repository root
	repoRoot := filepath.Join("..", "..")
	cmd := exec.Command(getBinaryPath(), "status", repoRoot) //nolint:noctx // test helper, no context needed
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run status command: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	// Status command now outputs bulk format with summary
	expectedStrings := []string{
		"Bulk Status Results",
		"Total scanned:",
		"repositories",
	}

	foundAny := false
	for _, expected := range expectedStrings {
		if strings.Contains(outputStr, expected) {
			foundAny = true
			break
		}
	}
	if !foundAny {
		t.Errorf("Expected status output to contain bulk status information, got: %s", outputStr)
	}
}

// TestCLIInfo tests the info command on current repository.
//
// The default view is a one-line-per-repository table with a fixed column set,
// so every header can be asserted regardless of what state the checkout this
// test runs in happens to be in.
func TestCLIInfo(t *testing.T) {
	// Change to repository root
	repoRoot := filepath.Join("..", "..")
	cmd := exec.Command(getBinaryPath(), "info", repoRoot) //nolint:noctx // test helper, no context needed
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run info command: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	expectedStrings := []string{
		"REPOSITORY", // table header
		"BRANCH",     // always populated: every repo has a branch or is detached
		"UPSTREAM",   // present even when nothing has drifted
		"WT",
		"DIRTY",
		"repositories",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(outputStr, expected) {
			t.Errorf("Expected info output to contain '%s', got: %s", expected, outputStr)
		}
	}
}

// TestCLIInfoCompact tests the shorter line behind --compact. It asserts only
// that the fixed columns survive: which of the optional ones get dropped
// depends on the state of the checkout the test runs in, so asserting their
// absence would make this test fail on a dirty tree rather than on a bug.
func TestCLIInfoCompact(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	cmd := exec.Command(getBinaryPath(), "info", "--compact", repoRoot) //nolint:noctx // test helper, no context needed
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run info --compact command: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	for _, expected := range []string{"REPOSITORY", "BRANCH", "repositories"} {
		if !strings.Contains(outputStr, expected) {
			t.Errorf("Expected info --compact output to contain '%s', got: %s", expected, outputStr)
		}
	}
}

// TestCLIInfoFull tests the per-repository detail view behind --full, which is
// where the prose fields the compact table deliberately omits still live.
func TestCLIInfoFull(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	cmd := exec.Command(getBinaryPath(), "info", "--full", repoRoot) //nolint:noctx // test helper, no context needed
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run info --full command: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	expectedStrings := []string{
		"📦",               // Repository indicator
		"Current Branch:", // Branch info
		"Base:",           // Integration branch and its divergence
		"Remotes:",        // Remote info section
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(outputStr, expected) {
			t.Errorf("Expected info --full output to contain '%s', got: %s", expected, outputStr)
		}
	}
}

// TestCLIInfoAudit tests that --audit emits a parseable document on stdout even
// when it exits non-zero. A caller pipes stdout into a parser; findings are the
// command's output, not a failure to produce it.
func TestCLIInfoAudit(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	cmd := exec.Command(getBinaryPath(), "info", "--audit", repoRoot) //nolint:noctx // test helper, no context needed
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	// Exit 1 means findings were reported; only exit 2 is an execution failure.
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			t.Fatalf("info --audit failed: %v\nOutput: %s", err, stdout.String())
		}
	}

	var doc struct {
		Schema       string `json:"schema"`
		Repositories []struct {
			Name     string `json:"name"`
			Complete bool   `json:"audit_complete"`
		} `json:"repositories"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if doc.Schema != "gz-git.info.audit/v1" {
		t.Errorf("schema = %q, want gz-git.info.audit/v1", doc.Schema)
	}
	if len(doc.Repositories) == 0 {
		t.Error("audit reported no repositories")
	}
}

// TestCLIClone tests the clone command with a small repository.
// Note: Clone command uses --url flag pattern (consistent with commit --messages).
// Directory is positional arg, URLs are via --url flag.
func TestCLIClone(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()

	// Clone a small, well-known repository using --url flag
	// Pattern: gz-git clone [directory] --url <url>
	cmd := exec.Command(getBinaryPath(), "clone", //nolint:noctx // test helper, no context needed
		tmpDir, // directory as positional arg
		"--depth", "1",
		"--single-branch",
		"--url", "https://github.com/golang/example.git")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run clone command: %v\nOutput: %s", err, output)
	}

	// Verify the repository was cloned (repo name extracted from URL: "example")
	gitDir := filepath.Join(tmpDir, "example", ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Errorf("Expected .git directory to exist at %s", gitDir)
	}

	// Verify output contains bulk clone result
	outputStr := string(output)
	if !strings.Contains(outputStr, "Bulk Clone Results") {
		t.Errorf("Expected clone output to contain 'Bulk Clone Results', got: %s", outputStr)
	}
}

// TestCLIStatusQuietClean tests the --quiet flag with clean repository.
func TestCLIStatusQuietClean(t *testing.T) {
	// Create a temporary directory and initialize a clean git repository
	tmpDir := t.TempDir()

	// Initialize git repository
	initCmd := exec.Command("git", "init") //nolint:noctx // test helper, no context needed
	initCmd.Dir = tmpDir
	if output, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to initialize git repository: %v\nOutput: %s", err, output)
	}

	// Configure git user
	configUserCmd := exec.Command("git", "config", "user.name", "Test User") //nolint:noctx // test helper, no context needed
	configUserCmd.Dir = tmpDir
	if output, err := configUserCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to configure git user: %v\nOutput: %s", err, output)
	}

	configEmailCmd := exec.Command("git", "config", "user.email", "test@example.com") //nolint:noctx // test helper, no context needed
	configEmailCmd.Dir = tmpDir
	if output, err := configEmailCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to configure git email: %v\nOutput: %s", err, output)
	}

	// Create and commit a file to have a clean state
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	addCmd := exec.Command("git", "add", "test.txt") //nolint:noctx // test helper, no context needed
	addCmd.Dir = tmpDir
	if output, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to add test file: %v\nOutput: %s", err, output)
	}

	commitCmd := exec.Command("git", "commit", "-m", "Initial commit") //nolint:noctx // test helper, no context needed
	commitCmd.Dir = tmpDir
	if output, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to commit test file: %v\nOutput: %s", err, output)
	}

	// Run status command with --quiet flag
	cmd := exec.Command(getBinaryPath(), "status", "--quiet", tmpDir) //nolint:noctx // test helper, no context needed
	output, err := cmd.CombinedOutput()
	// For a clean repository, exit code should be 0
	if err != nil {
		t.Errorf("Expected exit code 0 for clean repository, got error: %v\nOutput: %s", err, output)
	}

	// In quiet mode, output should be minimal or empty
	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" {
		// Some output is acceptable as long as it's not verbose
		t.Logf("Quiet mode output: %s", outputStr)
	}
}

// TestCLIStatusQuietDirty tests the --quiet flag with dirty repository.
func TestCLIStatusQuietDirty(t *testing.T) {
	// Create a temporary directory and initialize a git repository
	tmpDir := t.TempDir()

	// Initialize git repository
	initCmd := exec.Command("git", "init") //nolint:noctx // test helper, no context needed
	initCmd.Dir = tmpDir
	if output, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to initialize git repository: %v\nOutput: %s", err, output)
	}

	// Create an untracked file (dirty state)
	testFile := filepath.Join(tmpDir, "untracked.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Run status command with --quiet flag
	cmd := exec.Command(getBinaryPath(), "status", "--quiet", tmpDir) //nolint:noctx // test helper, no context needed
	output, err := cmd.CombinedOutput()
	// Quiet mode suppresses output but still returns 0 (command success)
	// Similar to git status --porcelain which returns 0 regardless of dirty state
	if err != nil {
		t.Errorf("Expected exit code 0 for status command, got error: %v\nOutput: %s", err, output)
	}

	// In quiet mode, output should be minimal (no verbose messages)
	outputStr := strings.TrimSpace(string(output))
	t.Logf("Quiet mode output for dirty repo: %s", outputStr)
}

// TestCLIInvalidCommand tests behavior with invalid command.
func TestCLIInvalidCommand(t *testing.T) {
	cmd := exec.Command(getBinaryPath(), "invalid-command") //nolint:noctx // test helper, no context needed
	output, err := cmd.CombinedOutput()

	// Should fail with non-zero exit code
	if err == nil {
		t.Errorf("Expected error for invalid command, got success\nOutput: %s", output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "unknown command") && !strings.Contains(outputStr, "Error") {
		t.Logf("Expected error message for invalid command, got: %s", outputStr)
	}
}

// TestCLICloneInvalidURL tests clone with invalid URL.
// Bulk clone reports failures in results AND exits non-zero so scripts/CI
// can detect partial failure.
func TestCLICloneInvalidURL(t *testing.T) {
	tmpDir := t.TempDir()

	// Use --url flag pattern
	cmd := exec.Command(getBinaryPath(), "clone", tmpDir, "--url", "not-a-valid-url") //nolint:noctx // test helper, no context needed
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("Expected non-zero exit for failed clone, got success\nOutput: %s", output)
	}

	outputStr := string(output)
	// Should show error status in results
	if !strings.Contains(outputStr, "error") && !strings.Contains(outputStr, "Total failed") {
		t.Errorf("Expected output to contain error information, got: %s", outputStr)
	}
}
