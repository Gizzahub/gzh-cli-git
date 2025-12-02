// Package cli provides CLI binary integration tests for gzh-git.
package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func getBinaryPath() string {
	// Get the module root directory
	return filepath.Join("..", "..", "gzh-git")
}

// TestCLIVersion tests the version command.
func TestCLIVersion(t *testing.T) {
	cmd := exec.Command(getBinaryPath(), "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run version command: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "gzh-git version") {
		t.Errorf("Expected version output to contain 'gzh-git version', got: %s", outputStr)
	}
}

// TestCLIHelp tests the help command.
func TestCLIHelp(t *testing.T) {
	cmd := exec.Command(getBinaryPath(), "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run help command: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	expectedStrings := []string{
		"gzh-git",
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
	cmd := exec.Command(getBinaryPath(), "status", repoRoot)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run status command: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	// Status output can show either full status or changes
	if !strings.Contains(outputStr, "Repository:") && !strings.Contains(outputStr, "Changes to be committed") && !strings.Contains(outputStr, "Working tree is clean") {
		t.Errorf("Expected status output to contain repository status information, got: %s", outputStr)
	}
}

// TestCLIInfo tests the info command on current repository.
func TestCLIInfo(t *testing.T) {
	// Change to repository root
	repoRoot := filepath.Join("..", "..")
	cmd := exec.Command(getBinaryPath(), "info", repoRoot)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run info command: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	expectedStrings := []string{
		"Repository:",
		"Branch:",
		"Remote URL:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(outputStr, expected) {
			t.Errorf("Expected info output to contain '%s', got: %s", expected, outputStr)
		}
	}
}

// TestCLIClone tests the clone command with a small repository.
func TestCLIClone(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()
	destination := filepath.Join(tmpDir, "test-clone")

	// Clone a small, well-known repository
	cmd := exec.Command(getBinaryPath(), "clone",
		"--depth", "1",
		"--single-branch",
		"https://github.com/golang/example.git",
		destination)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run clone command: %v\nOutput: %s", err, output)
	}

	// Verify the repository was cloned
	gitDir := filepath.Join(destination, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Errorf("Expected .git directory to exist at %s", gitDir)
	}

	// Verify output contains success message
	outputStr := string(output)
	if !strings.Contains(outputStr, "Successfully cloned") {
		t.Errorf("Expected clone output to contain 'Successfully cloned', got: %s", outputStr)
	}
}

// TestCLIStatusQuietClean tests the --quiet flag with clean repository.
func TestCLIStatusQuietClean(t *testing.T) {
	// Create a temporary directory and initialize a clean git repository
	tmpDir := t.TempDir()

	// Initialize git repository
	initCmd := exec.Command("git", "init")
	initCmd.Dir = tmpDir
	if output, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to initialize git repository: %v\nOutput: %s", err, output)
	}

	// Configure git user
	configUserCmd := exec.Command("git", "config", "user.name", "Test User")
	configUserCmd.Dir = tmpDir
	if output, err := configUserCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to configure git user: %v\nOutput: %s", err, output)
	}

	configEmailCmd := exec.Command("git", "config", "user.email", "test@example.com")
	configEmailCmd.Dir = tmpDir
	if output, err := configEmailCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to configure git email: %v\nOutput: %s", err, output)
	}

	// Create and commit a file to have a clean state
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	addCmd := exec.Command("git", "add", "test.txt")
	addCmd.Dir = tmpDir
	if output, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to add test file: %v\nOutput: %s", err, output)
	}

	commitCmd := exec.Command("git", "commit", "-m", "Initial commit")
	commitCmd.Dir = tmpDir
	if output, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to commit test file: %v\nOutput: %s", err, output)
	}

	// Run status command with --quiet flag
	cmd := exec.Command(getBinaryPath(), "status", "--quiet", tmpDir)
	output, err := cmd.CombinedOutput()

	// For a clean repository, exit code should be 0
	if err != nil {
		t.Errorf("Expected exit code 0 for clean repository, got error: %v\nOutput: %s", err, output)
	}

	// In quiet mode, output should be minimal or empty
	outputStr := strings.TrimSpace(string(output))
	if len(outputStr) > 0 {
		// Some output is acceptable as long as it's not verbose
		t.Logf("Quiet mode output: %s", outputStr)
	}
}

// TestCLIStatusQuietDirty tests the --quiet flag with dirty repository.
func TestCLIStatusQuietDirty(t *testing.T) {
	// Create a temporary directory and initialize a git repository
	tmpDir := t.TempDir()

	// Initialize git repository
	initCmd := exec.Command("git", "init")
	initCmd.Dir = tmpDir
	if output, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to initialize git repository: %v\nOutput: %s", err, output)
	}

	// Create an untracked file (dirty state)
	testFile := filepath.Join(tmpDir, "untracked.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Run status command with --quiet flag
	cmd := exec.Command(getBinaryPath(), "status", "--quiet", tmpDir)
	output, err := cmd.CombinedOutput()

	// For a dirty repository, exit code should be 1
	if err == nil {
		t.Errorf("Expected non-zero exit code for dirty repository, got success\nOutput: %s", output)
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 1 {
		t.Errorf("Expected exit code 1 for dirty repository, got: %v", err)
	}
}

// TestCLIInvalidCommand tests behavior with invalid command.
func TestCLIInvalidCommand(t *testing.T) {
	cmd := exec.Command(getBinaryPath(), "invalid-command")
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
func TestCLICloneInvalidURL(t *testing.T) {
	tmpDir := t.TempDir()
	destination := filepath.Join(tmpDir, "invalid-clone")

	cmd := exec.Command(getBinaryPath(), "clone", "not-a-valid-url", destination)
	output, err := cmd.CombinedOutput()

	// Should fail with non-zero exit code
	if err == nil {
		t.Errorf("Expected error for invalid URL, got success\nOutput: %s", output)
	}
}
