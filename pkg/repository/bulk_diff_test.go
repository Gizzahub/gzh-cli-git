package repository

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// createRepoWithChanges creates a git repo with staged and unstaged changes.
func createRepoWithChanges(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	if err := initGitRepo(path); err != nil {
		return err
	}

	// Create initial commit
	testFile := filepath.Join(path, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test\n"), 0o644); err != nil {
		return err
	}

	cmd := exec.Command("git", "add", ".") //nolint:noctx // test helper, no context available
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit") //nolint:noctx // test helper, no context available
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		return err
	}

	// Make unstaged changes
	if err := os.WriteFile(testFile, []byte("# Test\nUpdated content\n"), 0o644); err != nil {
		return err
	}

	// Create another file and stage it
	newFile := filepath.Join(path, "new.go")
	if err := os.WriteFile(newFile, []byte("package main\n"), 0o644); err != nil {
		return err
	}

	cmd = exec.Command("git", "add", "new.go") //nolint:noctx // test helper, no context available
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func TestBulkDiff(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test repositories
	repo1Path := filepath.Join(tmpDir, "repo1")
	repo2Path := filepath.Join(tmpDir, "repo2")

	if err := createRepoWithChanges(repo1Path); err != nil {
		t.Skipf("Skipping test: git not available or failed: %v", err)
	}
	if err := createRepoWithChanges(repo2Path); err != nil {
		t.Skipf("Skipping test: git not available or failed: %v", err)
	}

	ctx := context.Background()
	client := NewClient()

	opts := BulkDiffOptions{
		Directory: tmpDir,
		Parallel:  2,
		MaxDepth:  2,
		Logger:    NewNoopLogger(),
	}

	result, err := client.BulkDiff(ctx, opts)
	if err != nil {
		t.Fatalf("BulkDiff failed: %v", err)
	}

	// Verify results
	if result.TotalScanned != 2 {
		t.Errorf("Expected 2 scanned repositories, got %d", result.TotalScanned)
	}

	if result.TotalWithChanges != 2 {
		t.Errorf("Expected 2 repositories with changes, got %d", result.TotalWithChanges)
	}

	if len(result.Repositories) != 2 {
		t.Errorf("Expected 2 repository results, got %d", len(result.Repositories))
	}

	// Check that duration was recorded
	if result.Duration == 0 {
		t.Error("Expected non-zero duration")
	}
}

func TestBulkDiffCleanRepo(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a clean repository (no uncommitted changes)
	repoPath := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	if err := initGitRepo(repoPath); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	// Create initial commit
	testFile := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test\n"), 0o644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	cmd := exec.Command("git", "add", ".") //nolint:noctx // test setup, no context available
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit") //nolint:noctx // test setup, no context available
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	ctx := context.Background()
	client := NewClient()

	opts := BulkDiffOptions{
		Directory: tmpDir,
		MaxDepth:  2,
		Logger:    NewNoopLogger(),
	}

	result, err := client.BulkDiff(ctx, opts)
	if err != nil {
		t.Fatalf("BulkDiff failed: %v", err)
	}

	if result.TotalWithChanges != 0 {
		t.Errorf("Expected 0 repositories with changes, got %d", result.TotalWithChanges)
	}

	if result.TotalClean != 1 {
		t.Errorf("Expected 1 clean repository, got %d", result.TotalClean)
	}

	// Check status is clean
	for _, repo := range result.Repositories {
		if repo.Status != "clean" {
			t.Errorf("Expected status clean, got %s", repo.Status)
		}
	}
}

func TestBulkDiffWithFilters(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test repositories with different names
	repo1Path := filepath.Join(tmpDir, "myproject-api")
	repo2Path := filepath.Join(tmpDir, "test-repo")

	if err := createRepoWithChanges(repo1Path); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}
	if err := createRepoWithChanges(repo2Path); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	ctx := context.Background()
	client := NewClient()

	t.Run("Include pattern", func(t *testing.T) {
		opts := BulkDiffOptions{
			Directory:      tmpDir,
			MaxDepth:       2,
			IncludePattern: "myproject.*",
			Logger:         NewNoopLogger(),
		}

		result, err := client.BulkDiff(ctx, opts)
		if err != nil {
			t.Fatalf("BulkDiff failed: %v", err)
		}

		if result.TotalWithChanges != 1 {
			t.Errorf("Expected 1 repository with changes with include pattern, got %d", result.TotalWithChanges)
		}
	})

	t.Run("Exclude pattern", func(t *testing.T) {
		opts := BulkDiffOptions{
			Directory:      tmpDir,
			MaxDepth:       2,
			ExcludePattern: "test.*",
			Logger:         NewNoopLogger(),
		}

		result, err := client.BulkDiff(ctx, opts)
		if err != nil {
			t.Fatalf("BulkDiff failed: %v", err)
		}

		if result.TotalWithChanges != 1 {
			t.Errorf("Expected 1 repository with changes with exclude pattern, got %d", result.TotalWithChanges)
		}
	})
}

func TestBulkDiffProgressCallback(t *testing.T) {
	tmpDir := t.TempDir()

	repoPath := filepath.Join(tmpDir, "repo")
	if err := createRepoWithChanges(repoPath); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	ctx := context.Background()
	client := NewClient()

	callbackCalled := false
	opts := BulkDiffOptions{
		Directory: tmpDir,
		MaxDepth:  2,
		Logger:    NewNoopLogger(),
		ProgressCallback: func(current, total int, repo string) {
			callbackCalled = true
			if current < 1 || current > total {
				t.Errorf("Invalid progress: current=%d, total=%d", current, total)
			}
		},
	}

	_, err := client.BulkDiff(ctx, opts)
	if err != nil {
		t.Fatalf("BulkDiff failed: %v", err)
	}

	if !callbackCalled {
		t.Error("Progress callback was not called")
	}
}

func TestBulkDiffEmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	ctx := context.Background()
	client := NewClient()

	opts := BulkDiffOptions{
		Directory: tmpDir,
		MaxDepth:  2,
		Logger:    NewNoopLogger(),
	}

	result, err := client.BulkDiff(ctx, opts)
	if err != nil {
		t.Fatalf("BulkDiff on empty directory failed: %v", err)
	}

	if result.TotalScanned != 0 {
		t.Errorf("Expected 0 scanned repositories in empty directory, got %d", result.TotalScanned)
	}

	if result.TotalWithChanges != 0 {
		t.Errorf("Expected 0 repositories with changes in empty directory, got %d", result.TotalWithChanges)
	}
}

func TestRepositoryDiffResult(t *testing.T) {
	result := RepositoryDiffResult{
		Path:           "/tmp/test",
		Status:         "has-changes",
		FilesChanged:   3,
		Additions:      10,
		Deletions:      5,
		ChangedFiles:   []ChangedFile{{Path: "a.go", Status: "M"}},
		UntrackedFiles: []string{"new.txt"},
	}

	if result.Path != "/tmp/test" {
		t.Errorf("Expected path /tmp/test, got %s", result.Path)
	}

	if result.Status != "has-changes" {
		t.Errorf("Expected status has-changes, got %s", result.Status)
	}

	if result.FilesChanged != 3 {
		t.Errorf("Expected 3 files changed, got %d", result.FilesChanged)
	}

	if result.Additions != 10 {
		t.Errorf("Expected 10 additions, got %d", result.Additions)
	}

	if result.Deletions != 5 {
		t.Errorf("Expected 5 deletions, got %d", result.Deletions)
	}

	if len(result.ChangedFiles) != 1 {
		t.Errorf("Expected 1 changed file, got %d", len(result.ChangedFiles))
	}

	if len(result.UntrackedFiles) != 1 {
		t.Errorf("Expected 1 untracked file, got %d", len(result.UntrackedFiles))
	}
}

func TestChangedFile(t *testing.T) {
	cf := ChangedFile{
		Path:   "src/main.go",
		Status: "M",
	}

	if cf.Path != "src/main.go" {
		t.Errorf("Expected path src/main.go, got %s", cf.Path)
	}

	if cf.Status != "M" {
		t.Errorf("Expected status M, got %s", cf.Status)
	}

	// Test renamed file
	renamedFile := ChangedFile{
		OldPath: "old-name.go",
	}

	if renamedFile.OldPath != "old-name.go" {
		t.Errorf("Expected old path old-name.go, got %s", renamedFile.OldPath)
	}
}

func TestParseGitStatus(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{
			name:     "Modified in index",
			code:     "M ",
			expected: "M",
		},
		{
			name:     "Added in index",
			code:     "A ",
			expected: "A",
		},
		{
			name:     "Deleted in index",
			code:     "D ",
			expected: "D",
		},
		{
			name:     "Renamed in index",
			code:     "R ",
			expected: "R",
		},
		{
			name:     "Copied in index",
			code:     "C ",
			expected: "C",
		},
		{
			name:     "Modified in worktree",
			code:     " M",
			expected: "M",
		},
		{
			name:     "Deleted in worktree",
			code:     " D",
			expected: "D",
		},
		{
			name:     "Unknown status",
			code:     "  ",
			expected: "?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseGitStatus(tt.code)
			if result != tt.expected {
				t.Errorf("Expected status '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestBulkDiffContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	repoPath := filepath.Join(tmpDir, "repo")
	if err := createRepoWithChanges(repoPath); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	// Create canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	client := NewClient()

	opts := BulkDiffOptions{
		Directory: tmpDir,
		MaxDepth:  2,
		Logger:    NewNoopLogger(),
	}

	_, err := client.BulkDiff(ctx, opts)
	// Should handle canceled context gracefully
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Logf("BulkDiff with canceled context returned: %v", err)
	}
}

func TestBulkDiffStagedOnly(t *testing.T) {
	tmpDir := t.TempDir()

	repoPath := filepath.Join(tmpDir, "repo")
	if err := createRepoWithChanges(repoPath); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	ctx := context.Background()
	client := NewClient()

	opts := BulkDiffOptions{
		Directory: tmpDir,
		MaxDepth:  2,
		Staged:    true,
		Logger:    NewNoopLogger(),
	}

	result, err := client.BulkDiff(ctx, opts)
	if err != nil {
		t.Fatalf("BulkDiff failed: %v", err)
	}

	// Should still find changes (we have staged changes)
	if result.TotalWithChanges == 0 {
		t.Error("Expected to find repositories with staged changes")
	}
}

func TestBulkDiffMaxDiffSize(t *testing.T) {
	tmpDir := t.TempDir()

	repoPath := filepath.Join(tmpDir, "repo")
	if err := createRepoWithChanges(repoPath); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	ctx := context.Background()
	client := NewClient()

	opts := BulkDiffOptions{
		Directory:   tmpDir,
		MaxDepth:    2,
		MaxDiffSize: 10, // Very small limit
		Logger:      NewNoopLogger(),
	}

	result, err := client.BulkDiff(ctx, opts)
	if err != nil {
		t.Fatalf("BulkDiff failed: %v", err)
	}

	// Check if truncation occurred
	for _, repo := range result.Repositories {
		if repo.Status == "has-changes" && len(repo.DiffContent) > 10 {
			if !repo.Truncated {
				t.Error("Expected diff to be truncated with small MaxDiffSize")
			}
		}
	}
}

func TestBulkDiffOptions(t *testing.T) {
	t.Run("Default values", func(t *testing.T) {
		opts := BulkDiffOptions{}

		// ContextLines defaults to 0, will be set to 3 in BulkDiff
		if opts.ContextLines != 0 {
			t.Errorf("Expected ContextLines 0 before processing, got %d", opts.ContextLines)
		}

		// MaxDiffSize defaults to 0, will be set to 100KB in BulkDiff
		if opts.MaxDiffSize != 0 {
			t.Errorf("Expected MaxDiffSize 0 before processing, got %d", opts.MaxDiffSize)
		}
	})

	t.Run("Custom values", func(t *testing.T) {
		opts := BulkDiffOptions{
			Parallel:         4,
			MaxDepth:         3,
			Staged:           true,
			IncludeUntracked: true,
			ContextLines:     5,
			MaxDiffSize:      50000,
		}

		if opts.Parallel != 4 {
			t.Errorf("Expected Parallel 4, got %d", opts.Parallel)
		}

		if opts.MaxDepth != 3 {
			t.Errorf("Expected MaxDepth 3, got %d", opts.MaxDepth)
		}

		if !opts.Staged {
			t.Error("Expected Staged to be true")
		}

		if !opts.IncludeUntracked {
			t.Error("Expected IncludeUntracked to be true")
		}

		if opts.ContextLines != 5 {
			t.Errorf("Expected ContextLines 5, got %d", opts.ContextLines)
		}

		if opts.MaxDiffSize != 50000 {
			t.Errorf("Expected MaxDiffSize 50000, got %d", opts.MaxDiffSize)
		}
	})
}

// Benchmark tests

func BenchmarkBulkDiffSingleRepo(b *testing.B) {
	tmpDir := b.TempDir()
	repoPath := filepath.Join(tmpDir, "repo")

	if err := createRepoWithChanges(repoPath); err != nil {
		b.Skipf("Skipping benchmark: git not available: %v", err)
	}

	ctx := context.Background()
	client := NewClient()

	opts := BulkDiffOptions{
		Directory: tmpDir,
		MaxDepth:  2,
		Logger:    NewNoopLogger(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.BulkDiff(ctx, opts)
		if err != nil {
			b.Fatalf("BulkDiff failed: %v", err)
		}
	}
}
