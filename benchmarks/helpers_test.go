package benchmarks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setupGitRepo initializes a git repository
func setupGitRepo(b *testing.B, path string) {
	b.Helper()

	if err := os.MkdirAll(path, 0755); err != nil {
		b.Fatalf("Failed to create directory: %v", err)
	}

	runCmd(b, path, "git", "init")
	runCmd(b, path, "git", "config", "user.name", "Benchmark User")
	runCmd(b, path, "git", "config", "user.email", "benchmark@example.com")
}

// setupGitRepoWithCommit creates a git repo with initial commit
func setupGitRepoWithCommit(b *testing.B, path string) {
	b.Helper()

	setupGitRepo(b, path)

	// Create and commit initial file
	writeFile(b, filepath.Join(path, "README.md"), "# Benchmark Repo\n")
	runCmd(b, path, "git", "add", "README.md")
	runCmd(b, path, "git", "commit", "-m", "Initial commit")
}

// setupLargeRepo creates a repository with many commits
func setupLargeRepo(b *testing.B, path string, commits int) {
	b.Helper()

	setupGitRepo(b, path)

	for i := 0; i < commits; i++ {
		filename := filepath.Join(path, fmt.Sprintf("file%d.txt", i))
		writeFile(b, filename, fmt.Sprintf("Content %d\n", i))
		runCmd(b, path, "git", "add", filepath.Base(filename))
		runCmd(b, path, "git", "commit", "-m", fmt.Sprintf("Commit %d", i))
	}
}

// setupRepoWithManyFiles creates a repository with many files
func setupRepoWithManyFiles(b *testing.B, path string, fileCount int) {
	b.Helper()

	setupGitRepo(b, path)

	// Create many files in one commit
	for i := 0; i < fileCount; i++ {
		filename := filepath.Join(path, fmt.Sprintf("file%d.txt", i))
		writeFile(b, filename, fmt.Sprintf("Content %d\n", i))
	}

	runCmd(b, path, "git", "add", ".")
	runCmd(b, path, "git", "commit", "-m", "Add many files")
}

// runCmd executes a command in the specified directory
func runCmd(b *testing.B, dir string, name string, args ...string) {
	b.Helper()

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		b.Fatalf("Command %s %v failed: %v\nOutput: %s", name, args, err, output)
	}
}

// writeFile writes content to a file
func writeFile(b *testing.B, path, content string) {
	b.Helper()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		b.Fatalf("Failed to create directory %s: %v", dir, err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		b.Fatalf("Failed to write file %s: %v", path, err)
	}
}
