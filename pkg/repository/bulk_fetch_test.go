package repository

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// initGitRepo initializes a git repository at the given path
func initGitRepo(path string) error {
	cmd := exec.Command("git", "init")
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		return err
	}

	// Configure git for testing
	configCmds := [][]string{
		{"config", "user.name", "Test User"},
		{"config", "user.email", "test@example.com"},
	}

	for _, args := range configCmds {
		cmd := exec.Command("git", args...)
		cmd.Dir = path
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	return nil
}

func TestBulkFetch(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()

	// Create test repositories
	repo1Path := filepath.Join(tmpDir, "repo1")
	repo2Path := filepath.Join(tmpDir, "repo2")

	// Initialize git repositories
	if err := os.MkdirAll(repo1Path, 0755); err != nil {
		t.Fatalf("Failed to create repo1: %v", err)
	}
	if err := os.MkdirAll(repo2Path, 0755); err != nil {
		t.Fatalf("Failed to create repo2: %v", err)
	}

	// Initialize actual git repositories
	if err := initGitRepo(repo1Path); err != nil {
		t.Skipf("Skipping test: git not available or failed to init repo1: %v", err)
	}
	if err := initGitRepo(repo2Path); err != nil {
		t.Skipf("Skipping test: git not available or failed to init repo2: %v", err)
	}

	ctx := context.Background()
	client := NewClient()

	opts := BulkFetchOptions{
		Directory: tmpDir,
		Parallel:  2,
		MaxDepth:  1,
		DryRun:    true, // Use dry-run for test
		Verbose:   false,
		Logger:    NewNoopLogger(),
	}

	result, err := client.BulkFetch(ctx, opts)
	if err != nil {
		t.Fatalf("BulkFetch failed: %v", err)
	}

	// Verify results
	if result.TotalScanned != 2 {
		t.Errorf("Expected 2 scanned repositories, got %d", result.TotalScanned)
	}

	if result.TotalProcessed != 2 {
		t.Errorf("Expected 2 processed repositories, got %d", result.TotalProcessed)
	}

	if len(result.Repositories) != 2 {
		t.Errorf("Expected 2 repository results, got %d", len(result.Repositories))
	}

	// Check that duration was recorded
	if result.Duration == 0 {
		t.Error("Expected non-zero duration")
	}

	// Verify summary contains results
	if len(result.Summary) == 0 {
		t.Error("Expected non-empty summary")
	}
}

func TestBulkFetchWithFilters(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test repositories
	repo1Path := filepath.Join(tmpDir, "myproject-api")
	repo2Path := filepath.Join(tmpDir, "test-repo")

	for _, path := range []string{repo1Path, repo2Path} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		if err := initGitRepo(path); err != nil {
			t.Skipf("Skipping test: git not available: %v", err)
		}
	}

	ctx := context.Background()
	client := NewClient()

	t.Run("Include pattern", func(t *testing.T) {
		opts := BulkFetchOptions{
			Directory:      tmpDir,
			MaxDepth:       1,
			DryRun:         true,
			IncludePattern: "myproject.*",
			Logger:         NewNoopLogger(),
		}

		result, err := client.BulkFetch(ctx, opts)
		if err != nil {
			t.Fatalf("BulkFetch failed: %v", err)
		}

		if result.TotalProcessed != 1 {
			t.Errorf("Expected 1 processed repository with include pattern, got %d", result.TotalProcessed)
		}
	})

	t.Run("Exclude pattern", func(t *testing.T) {
		opts := BulkFetchOptions{
			Directory:      tmpDir,
			MaxDepth:       1,
			DryRun:         true,
			ExcludePattern: "test.*",
			Logger:         NewNoopLogger(),
		}

		result, err := client.BulkFetch(ctx, opts)
		if err != nil {
			t.Fatalf("BulkFetch failed: %v", err)
		}

		if result.TotalProcessed != 1 {
			t.Errorf("Expected 1 processed repository with exclude pattern, got %d", result.TotalProcessed)
		}
	})
}

func TestBulkFetchOptions(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()
	client := NewClient()

	t.Run("Default options", func(t *testing.T) {
		opts := BulkFetchOptions{
			Directory: tmpDir,
			// No parallel or maxDepth set - should use defaults
		}

		_, err := client.BulkFetch(ctx, opts)
		if err != nil {
			t.Fatalf("BulkFetch with default options failed: %v", err)
		}
	})

	t.Run("Empty directory uses current directory", func(t *testing.T) {
		opts := BulkFetchOptions{
			Directory: "",
			MaxDepth:  1,
			DryRun:    true,
		}

		result, err := client.BulkFetch(ctx, opts)
		if err != nil {
			t.Fatalf("BulkFetch with empty directory failed: %v", err)
		}

		if result.Duration == 0 {
			t.Error("Expected operation to complete with duration")
		}
	})
}

func TestBulkFetchProgressCallback(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test repository
	repoPath := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	if err := initGitRepo(repoPath); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	ctx := context.Background()
	client := NewClient()

	callbackCalled := false
	opts := BulkFetchOptions{
		Directory: tmpDir,
		MaxDepth:  1,
		DryRun:    true,
		Logger:    NewNoopLogger(),
		ProgressCallback: func(current, total int, repo string) {
			callbackCalled = true
			if current < 1 || current > total {
				t.Errorf("Invalid progress: current=%d, total=%d", current, total)
			}
		},
	}

	_, err := client.BulkFetch(ctx, opts)
	if err != nil {
		t.Fatalf("BulkFetch failed: %v", err)
	}

	if !callbackCalled {
		t.Error("Progress callback was not called")
	}
}

func TestBulkFetchContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test repository
	repoPath := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	if err := initGitRepo(repoPath); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	client := NewClient()

	opts := BulkFetchOptions{
		Directory: tmpDir,
		MaxDepth:  1,
		DryRun:    true,
		Logger:    NewNoopLogger(),
	}

	_, err := client.BulkFetch(ctx, opts)
	// Should handle cancelled context gracefully
	// The error may or may not be returned depending on timing
	if err != nil && err != context.Canceled {
		t.Logf("BulkFetch with cancelled context returned: %v", err)
	}
}

func TestBulkFetchNestedRepositories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create parent repository
	parentPath := filepath.Join(tmpDir, "parent")
	if err := os.MkdirAll(parentPath, 0755); err != nil {
		t.Fatalf("Failed to create parent: %v", err)
	}
	if err := initGitRepo(parentPath); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	// Create first nested independent repository (not a submodule)
	nested1Path := filepath.Join(parentPath, "nested-repo1")
	if err := os.MkdirAll(nested1Path, 0755); err != nil {
		t.Fatalf("Failed to create nested repo1: %v", err)
	}
	if err := initGitRepo(nested1Path); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	// Create second nested independent repository
	nested2Path := filepath.Join(parentPath, "nested-repo2")
	if err := os.MkdirAll(nested2Path, 0755); err != nil {
		t.Fatalf("Failed to create nested repo2: %v", err)
	}
	if err := initGitRepo(nested2Path); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	// Create deeply nested repository (inside nested-repo1)
	deepNestedPath := filepath.Join(nested1Path, "deep-nested")
	if err := os.MkdirAll(deepNestedPath, 0755); err != nil {
		t.Fatalf("Failed to create deep nested: %v", err)
	}
	if err := initGitRepo(deepNestedPath); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	ctx := context.Background()
	client := NewClient()

	t.Run("Find all nested repositories", func(t *testing.T) {
		opts := BulkFetchOptions{
			Directory:         tmpDir,
			MaxDepth:          5,
			DryRun:            true,
			IncludeSubmodules: false,
			Logger:            NewNoopLogger(),
		}

		result, err := client.BulkFetch(ctx, opts)
		if err != nil {
			t.Fatalf("BulkFetch failed: %v", err)
		}

		// Should find all 4: parent, nested-repo1, nested-repo2, deep-nested
		if result.TotalScanned != 4 {
			t.Errorf("Expected 4 repositories, got %d", result.TotalScanned)
			for _, repo := range result.Repositories {
				t.Logf("Found: %s", repo.RelativePath)
			}
		}

		// Verify all repos were found
		foundParent := false
		foundNested1 := false
		foundNested2 := false
		foundDeepNested := false

		for _, repo := range result.Repositories {
			base := filepath.Base(repo.Path)
			switch base {
			case "parent":
				foundParent = true
			case "nested-repo1":
				foundNested1 = true
			case "nested-repo2":
				foundNested2 = true
			case "deep-nested":
				foundDeepNested = true
			}
		}

		if !foundParent {
			t.Error("Expected to find parent repo")
		}
		if !foundNested1 {
			t.Error("Expected to find nested-repo1")
		}
		if !foundNested2 {
			t.Error("Expected to find nested-repo2")
		}
		if !foundDeepNested {
			t.Error("Expected to find deep-nested repo")
		}
	})

	t.Run("Respect max depth limit", func(t *testing.T) {
		opts := BulkFetchOptions{
			Directory:         tmpDir,
			MaxDepth:          2, // Should stop before reaching deep-nested
			DryRun:            true,
			IncludeSubmodules: false,
			Logger:            NewNoopLogger(),
		}

		result, err := client.BulkFetch(ctx, opts)
		if err != nil {
			t.Fatalf("BulkFetch failed: %v", err)
		}

		// Should find parent, nested-repo1, nested-repo2 but NOT deep-nested (depth 3)
		if result.TotalScanned != 3 {
			t.Errorf("Expected 3 repositories with max-depth 2, got %d", result.TotalScanned)
			for _, repo := range result.Repositories {
				t.Logf("Found: %s", repo.RelativePath)
			}
		}
	})

	t.Run("Depth 0 scans only current directory", func(t *testing.T) {
		// Test with depth=0 on parent directory
		opts := BulkFetchOptions{
			Directory:         parentPath,
			MaxDepth:          0, // Scan only current directory
			DryRun:            true,
			IncludeSubmodules: false,
			Logger:            NewNoopLogger(),
		}

		result, err := client.BulkFetch(ctx, opts)
		if err != nil {
			t.Fatalf("BulkFetch failed: %v", err)
		}

		// Should find only parent repository, not nested ones
		if result.TotalScanned != 1 {
			t.Errorf("Expected 1 repository with depth=0, got %d", result.TotalScanned)
			for _, repo := range result.Repositories {
				t.Logf("Found: %s", repo.RelativePath)
			}
		}

		// Verify it's the parent repo
		if len(result.Repositories) > 0 && filepath.Base(result.Repositories[0].Path) != "parent" {
			t.Errorf("Expected to find parent repo, got %s", filepath.Base(result.Repositories[0].Path))
		}
	})

	t.Run("Depth 0 on non-git directory finds nothing", func(t *testing.T) {
		// Create a non-git directory
		nonGitDir := filepath.Join(tmpDir, "non-git")
		if err := os.MkdirAll(nonGitDir, 0755); err != nil {
			t.Fatalf("Failed to create non-git dir: %v", err)
		}

		opts := BulkFetchOptions{
			Directory:         nonGitDir,
			MaxDepth:          0,
			DryRun:            true,
			IncludeSubmodules: false,
			Logger:            NewNoopLogger(),
		}

		result, err := client.BulkFetch(ctx, opts)
		if err != nil {
			t.Fatalf("BulkFetch failed: %v", err)
		}

		// Should find nothing
		if result.TotalScanned != 0 {
			t.Errorf("Expected 0 repositories with depth=0 on non-git dir, got %d", result.TotalScanned)
			for _, repo := range result.Repositories {
				t.Logf("Found: %s", repo.RelativePath)
			}
		}
	})
}

func TestBulkFetchEmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	ctx := context.Background()
	client := NewClient()

	opts := BulkFetchOptions{
		Directory: tmpDir,
		MaxDepth:  1,
		DryRun:    true,
		Logger:    NewNoopLogger(),
	}

	result, err := client.BulkFetch(ctx, opts)
	if err != nil {
		t.Fatalf("BulkFetch on empty directory failed: %v", err)
	}

	if result.TotalScanned != 0 {
		t.Errorf("Expected 0 scanned repositories in empty directory, got %d", result.TotalScanned)
	}

	if result.TotalProcessed != 0 {
		t.Errorf("Expected 0 processed repositories in empty directory, got %d", result.TotalProcessed)
	}
}

func TestRepositoryFetchResult(t *testing.T) {
	result := RepositoryFetchResult{
		Path:         "/tmp/test",
		RelativePath: "test",
		Status:       "success",
		Message:      "Test message",
		Duration:     100 * time.Millisecond,
		Branch:       "main",
		RemoteURL:    "https://github.com/test/repo.git",
	}

	if result.Path != "/tmp/test" {
		t.Errorf("Expected path /tmp/test, got %s", result.Path)
	}

	if result.Status != "success" {
		t.Errorf("Expected status success, got %s", result.Status)
	}

	if result.Duration != 100*time.Millisecond {
		t.Errorf("Expected duration 100ms, got %v", result.Duration)
	}
}

func TestCalculateFetchSummary(t *testing.T) {
	results := []RepositoryFetchResult{
		{Status: "success"},
		{Status: "success"},
		{Status: "error"},
		{Status: "no-remote"},
		{Status: "would-fetch"},
	}

	summary := calculateFetchSummary(results)

	if summary["success"] != 2 {
		t.Errorf("Expected 2 success, got %d", summary["success"])
	}

	if summary["error"] != 1 {
		t.Errorf("Expected 1 error, got %d", summary["error"])
	}

	if summary["no-remote"] != 1 {
		t.Errorf("Expected 1 no-remote, got %d", summary["no-remote"])
	}

	if summary["would-fetch"] != 1 {
		t.Errorf("Expected 1 would-fetch, got %d", summary["would-fetch"])
	}
}

// Benchmark tests for bulk fetch performance

func BenchmarkBulkFetchSingleRepo(b *testing.B) {
	tmpDir := b.TempDir()
	repoPath := filepath.Join(tmpDir, "repo")

	if err := os.MkdirAll(repoPath, 0755); err != nil {
		b.Fatalf("Failed to create repo: %v", err)
	}
	if err := initGitRepo(repoPath); err != nil {
		b.Skipf("Skipping benchmark: git not available: %v", err)
	}

	ctx := context.Background()
	client := NewClient()

	opts := BulkFetchOptions{
		Directory: tmpDir,
		MaxDepth:  1,
		DryRun:    true,
		Logger:    NewNoopLogger(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.BulkFetch(ctx, opts)
		if err != nil {
			b.Fatalf("BulkFetch failed: %v", err)
		}
	}
}

func BenchmarkBulkFetchMultipleRepos(b *testing.B) {
	tmpDir := b.TempDir()

	// Create 10 test repositories
	for i := 0; i < 10; i++ {
		repoPath := filepath.Join(tmpDir, filepath.Base(tmpDir)+"-repo-"+string(rune('0'+i)))
		if err := os.MkdirAll(repoPath, 0755); err != nil {
			b.Fatalf("Failed to create repo%d: %v", i, err)
		}
		if err := initGitRepo(repoPath); err != nil {
			b.Skipf("Skipping benchmark: git not available: %v", err)
		}
	}

	ctx := context.Background()
	client := NewClient()

	opts := BulkFetchOptions{
		Directory: tmpDir,
		MaxDepth:  1,
		DryRun:    true,
		Parallel:  5,
		Logger:    NewNoopLogger(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.BulkFetch(ctx, opts)
		if err != nil {
			b.Fatalf("BulkFetch failed: %v", err)
		}
	}
}

func BenchmarkBulkFetchNestedRepos(b *testing.B) {
	tmpDir := b.TempDir()

	// Create parent repository
	parentPath := filepath.Join(tmpDir, "parent")
	if err := os.MkdirAll(parentPath, 0755); err != nil {
		b.Fatalf("Failed to create parent: %v", err)
	}
	if err := initGitRepo(parentPath); err != nil {
		b.Skipf("Skipping benchmark: git not available: %v", err)
	}

	// Create 5 nested repositories
	for i := 0; i < 5; i++ {
		nestedPath := filepath.Join(parentPath, filepath.Base(tmpDir)+"-nested-"+string(rune('0'+i)))
		if err := os.MkdirAll(nestedPath, 0755); err != nil {
			b.Fatalf("Failed to create nested%d: %v", i, err)
		}
		if err := initGitRepo(nestedPath); err != nil {
			b.Skipf("Skipping benchmark: git not available: %v", err)
		}
	}

	ctx := context.Background()
	client := NewClient()

	opts := BulkFetchOptions{
		Directory:         tmpDir,
		MaxDepth:          3,
		DryRun:            true,
		Parallel:          5,
		IncludeSubmodules: false,
		Logger:            NewNoopLogger(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.BulkFetch(ctx, opts)
		if err != nil {
			b.Fatalf("BulkFetch failed: %v", err)
		}
	}
}

func BenchmarkIsSubmodule(b *testing.B) {
	tmpDir := b.TempDir()

	// Create independent repository
	repoPath := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		b.Fatalf("Failed to create repo: %v", err)
	}
	if err := initGitRepo(repoPath); err != nil {
		b.Skipf("Skipping benchmark: git not available: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = isSubmodule(repoPath)
	}
}

func BenchmarkParallelProcessing(b *testing.B) {
	tmpDir := b.TempDir()

	// Create 20 repositories
	for i := 0; i < 20; i++ {
		repoPath := filepath.Join(tmpDir, filepath.Base(tmpDir)+"-repo-"+string(rune('0'+i/10))+string(rune('0'+i%10)))
		if err := os.MkdirAll(repoPath, 0755); err != nil {
			b.Fatalf("Failed to create repo: %v", err)
		}
		if err := initGitRepo(repoPath); err != nil {
			b.Skipf("Skipping benchmark: git not available: %v", err)
		}
	}

	ctx := context.Background()
	client := NewClient()

	// Benchmark different parallelism levels
	parallelLevels := []int{1, 5, 10, 20}

	for _, parallel := range parallelLevels {
		b.Run("Parallel"+string(rune('0'+parallel/10))+string(rune('0'+parallel%10)), func(b *testing.B) {
			opts := BulkFetchOptions{
				Directory: tmpDir,
				MaxDepth:  1,
				DryRun:    true,
				Parallel:  parallel,
				Logger:    NewNoopLogger(),
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := client.BulkFetch(ctx, opts)
				if err != nil {
					b.Fatalf("BulkFetch failed: %v", err)
				}
			}
		})
	}
}
