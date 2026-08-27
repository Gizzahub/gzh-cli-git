package repository

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// initGitRepoWithCommit initializes a git repository with an initial commit.
func initGitRepoWithCommit(path string) error {
	if err := initGitRepo(path); err != nil {
		return err
	}

	// Create a file and commit
	testFile := filepath.Join(path, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test Repository\n"), 0o644); err != nil {
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

	return nil
}

func TestBulkPush(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()

	// Create test repositories
	repo1Path := filepath.Join(tmpDir, "repo1")
	repo2Path := filepath.Join(tmpDir, "repo2")

	// Initialize git repositories
	if err := os.MkdirAll(repo1Path, 0o755); err != nil {
		t.Fatalf("Failed to create repo1: %v", err)
	}
	if err := os.MkdirAll(repo2Path, 0o755); err != nil {
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

	opts := BulkPushOptions{
		Directory: tmpDir,
		Parallel:  2,
		MaxDepth:  2,    // Scan tmpDir (depth 0) + immediate children (depth 1)
		DryRun:    true, // Use dry-run for test
		Verbose:   false,
		Logger:    NewNoopLogger(),
	}

	result, err := client.BulkPush(ctx, opts)
	if err != nil {
		t.Fatalf("BulkPush failed: %v", err)
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

func TestBulkPushSkipsRepositoryDeniedByPushAccess(t *testing.T) {
	rootDir := t.TempDir()
	repoPath := filepath.Join(rootDir, "foreign")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := initGitRepo(repoPath); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	called := false
	result, err := NewClient().BulkPush(context.Background(), BulkPushOptions{
		Directory: rootDir,
		MaxDepth:  2,
		Logger:    NewNoopLogger(),
		PushAccess: func(path string) (bool, string, error) {
			called = true
			if path != repoPath {
				t.Fatalf("PushAccess path = %q, want %q", path, repoPath)
			}
			return false, "read-only workspace; push disabled", nil
		},
	})
	if err != nil {
		t.Fatalf("BulkPush() error = %v", err)
	}
	if !called {
		t.Fatal("PushAccess was not called")
	}
	if len(result.Repositories) != 1 {
		t.Fatalf("len(Repositories) = %d, want 1", len(result.Repositories))
	}
	if result.Repositories[0].Status != StatusSkipped {
		t.Fatalf("status = %q, want %q", result.Repositories[0].Status, StatusSkipped)
	}
	if !strings.Contains(result.Repositories[0].Message, "push disabled") {
		t.Fatalf("message = %q", result.Repositories[0].Message)
	}
}

func TestBulkPushSetUpstreamWhenMissingAndAheadIsZero(t *testing.T) {
	rootDir := t.TempDir()
	repoPath := filepath.Join(rootDir, "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("Failed to create repo directory: %v", err)
	}

	remoteDir := t.TempDir()
	remotePath := filepath.Join(remoteDir, "origin.git")
	cmd := exec.Command("git", "init", "--bare", remotePath) //nolint:noctx // test setup, no context available
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("Skipping test: git not available or failed to init bare remote: %v\n%s", err, output)
	}

	if err := initGitRepoWithCommit(repoPath); err != nil {
		t.Skipf("Skipping test: git not available or failed to init repo: %v", err)
	}

	cmd = exec.Command("git", "checkout", "-b", "develop") //nolint:noctx // test setup, no context available
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create branch develop: %v\n%s", err, output)
	}

	cmd = exec.Command("git", "remote", "add", "origin", remotePath) //nolint:noctx // test setup, no context available
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to add origin remote: %v\n%s", err, output)
	}

	// Push without -u to ensure the remote branch exists but no upstream is configured.
	cmd = exec.Command("git", "push", "origin", "develop:develop") //nolint:noctx // test setup, no context available
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to push branch to origin: %v\n%s", err, output)
	}

	cmd = exec.Command("git", "branch", "--unset-upstream", "develop") //nolint:noctx // test setup, no context available
	cmd.Dir = repoPath
	_ = cmd.Run() // Ignore: may already have no upstream

	cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "@{upstream}") //nolint:noctx // test setup, no context available
	cmd.Dir = repoPath
	if err := cmd.Run(); err == nil {
		t.Fatalf("Expected upstream to be missing before BulkPush")
	}

	cmd = exec.Command("git", "--git-dir", remotePath, "show-ref", "--verify", "refs/heads/develop") //nolint:noctx // test setup, no context available
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Expected remote branch to exist: %v\n%s", err, output)
	}

	ctx := context.Background()
	client := NewClient()
	result, err := client.BulkPush(ctx, BulkPushOptions{
		Directory:   rootDir,
		Parallel:    1,
		MaxDepth:    2,
		DryRun:      false,
		SetUpstream: true,
		Logger:      NewNoopLogger(),
	})
	if err != nil {
		t.Fatalf("BulkPush failed: %v", err)
	}
	if result.TotalProcessed != 1 {
		t.Fatalf("BulkPush TotalProcessed = %d, want 1", result.TotalProcessed)
	}

	cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "@{upstream}") //nolint:noctx // test setup, no context available
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Expected upstream to be configured after BulkPush: %v\n%s", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != "origin/develop" {
		t.Fatalf("Upstream = %q, want %q", got, "origin/develop")
	}
}

func TestBulkPushSetUpstreamDryRunDoesNotChangeUpstream(t *testing.T) {
	rootDir := t.TempDir()
	repoPath := filepath.Join(rootDir, "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("Failed to create repo directory: %v", err)
	}

	remoteDir := t.TempDir()
	remotePath := filepath.Join(remoteDir, "origin.git")
	cmd := exec.Command("git", "init", "--bare", remotePath) //nolint:noctx // test setup, no context available
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("Skipping test: git not available or failed to init bare remote: %v\n%s", err, output)
	}

	if err := initGitRepoWithCommit(repoPath); err != nil {
		t.Skipf("Skipping test: git not available or failed to init repo: %v", err)
	}

	cmd = exec.Command("git", "checkout", "-b", "develop") //nolint:noctx // test setup, no context available
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create branch develop: %v\n%s", err, output)
	}

	cmd = exec.Command("git", "remote", "add", "origin", remotePath) //nolint:noctx // test setup, no context available
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to add origin remote: %v\n%s", err, output)
	}

	cmd = exec.Command("git", "branch", "--unset-upstream", "develop") //nolint:noctx // test setup, no context available
	cmd.Dir = repoPath
	_ = cmd.Run() // Ignore: may already have no upstream

	ctx := context.Background()
	client := NewClient()
	result, err := client.BulkPush(ctx, BulkPushOptions{
		Directory:   rootDir,
		Parallel:    1,
		MaxDepth:    2,
		DryRun:      true,
		SetUpstream: true,
		Logger:      NewNoopLogger(),
	})
	if err != nil {
		t.Fatalf("BulkPush failed: %v", err)
	}
	if result.TotalProcessed != 1 {
		t.Fatalf("BulkPush TotalProcessed = %d, want 1", result.TotalProcessed)
	}
	if len(result.Repositories) != 1 {
		t.Fatalf("BulkPush len(Repositories) = %d, want 1", len(result.Repositories))
	}
	if result.Repositories[0].Status != StatusWouldPush {
		t.Fatalf("BulkPush Status = %q, want %q", result.Repositories[0].Status, StatusWouldPush)
	}

	cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "@{upstream}") //nolint:noctx // test setup, no context available
	cmd.Dir = repoPath
	if err := cmd.Run(); err == nil {
		t.Fatalf("Expected upstream to remain missing after DryRun")
	}
}

func TestBulkPushWithFilters(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test repositories
	repo1Path := filepath.Join(tmpDir, "myproject-api")
	repo2Path := filepath.Join(tmpDir, "test-repo")

	for _, path := range []string{repo1Path, repo2Path} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		if err := initGitRepo(path); err != nil {
			t.Skipf("Skipping test: git not available: %v", err)
		}
	}

	ctx := context.Background()
	client := NewClient()

	t.Run("Include pattern", func(t *testing.T) {
		opts := BulkPushOptions{
			Directory:      tmpDir,
			MaxDepth:       2,
			DryRun:         true,
			IncludePattern: "myproject.*",
			Logger:         NewNoopLogger(),
		}

		result, err := client.BulkPush(ctx, opts)
		if err != nil {
			t.Fatalf("BulkPush failed: %v", err)
		}

		if result.TotalProcessed != 1 {
			t.Errorf("Expected 1 processed repository with include pattern, got %d", result.TotalProcessed)
		}
	})

	t.Run("Exclude pattern", func(t *testing.T) {
		opts := BulkPushOptions{
			Directory:      tmpDir,
			MaxDepth:       2,
			DryRun:         true,
			ExcludePattern: "test.*",
			Logger:         NewNoopLogger(),
		}

		result, err := client.BulkPush(ctx, opts)
		if err != nil {
			t.Fatalf("BulkPush failed: %v", err)
		}

		if result.TotalProcessed != 1 {
			t.Errorf("Expected 1 processed repository with exclude pattern, got %d", result.TotalProcessed)
		}
	})
}

func TestBulkPushOptions(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()
	client := NewClient()

	t.Run("Default options", func(t *testing.T) {
		opts := BulkPushOptions{
			Directory: tmpDir,
			// No parallel or maxDepth set - should use defaults
		}

		_, err := client.BulkPush(ctx, opts)
		if err != nil {
			t.Fatalf("BulkPush with default options failed: %v", err)
		}
	})

	t.Run("Empty directory uses current directory", func(t *testing.T) {
		opts := BulkPushOptions{
			Directory: "",
			MaxDepth:  2,
			DryRun:    true,
		}

		result, err := client.BulkPush(ctx, opts)
		if err != nil {
			t.Fatalf("BulkPush with empty directory failed: %v", err)
		}

		if result.Duration == 0 {
			t.Error("Expected operation to complete with duration")
		}
	})
}

func TestBulkPushProgressCallback(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test repository
	repoPath := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	if err := initGitRepo(repoPath); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	ctx := context.Background()
	client := NewClient()

	callbackCalled := false
	opts := BulkPushOptions{
		Directory: tmpDir,
		MaxDepth:  2,
		DryRun:    true,
		Logger:    NewNoopLogger(),
		ProgressCallback: func(current, total int, repo string) {
			callbackCalled = true
			if current < 1 || current > total {
				t.Errorf("Invalid progress: current=%d, total=%d", current, total)
			}
		},
	}

	_, err := client.BulkPush(ctx, opts)
	if err != nil {
		t.Fatalf("BulkPush failed: %v", err)
	}

	if !callbackCalled {
		t.Error("Progress callback was not called")
	}
}

func TestBulkPushContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test repository
	repoPath := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	if err := initGitRepo(repoPath); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	// Create canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	client := NewClient()

	opts := BulkPushOptions{
		Directory: tmpDir,
		MaxDepth:  2,
		DryRun:    true,
		Logger:    NewNoopLogger(),
	}

	_, err := client.BulkPush(ctx, opts)
	// Should handle canceled context gracefully
	// The error may or may not be returned depending on timing
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Logf("BulkPush with canceled context returned: %v", err)
	}
}

func TestBulkPushNestedRepositories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create parent repository
	parentPath := filepath.Join(tmpDir, "parent")
	if err := os.MkdirAll(parentPath, 0o755); err != nil {
		t.Fatalf("Failed to create parent: %v", err)
	}
	if err := initGitRepo(parentPath); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	// Create first nested independent repository (not a submodule)
	nested1Path := filepath.Join(parentPath, "nested-repo1")
	if err := os.MkdirAll(nested1Path, 0o755); err != nil {
		t.Fatalf("Failed to create nested repo1: %v", err)
	}
	if err := initGitRepo(nested1Path); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	// Create second nested independent repository
	nested2Path := filepath.Join(parentPath, "nested-repo2")
	if err := os.MkdirAll(nested2Path, 0o755); err != nil {
		t.Fatalf("Failed to create nested repo2: %v", err)
	}
	if err := initGitRepo(nested2Path); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	// Create deeply nested repository (inside nested-repo1)
	deepNestedPath := filepath.Join(nested1Path, "deep-nested")
	if err := os.MkdirAll(deepNestedPath, 0o755); err != nil {
		t.Fatalf("Failed to create deep nested: %v", err)
	}
	if err := initGitRepo(deepNestedPath); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	ctx := context.Background()
	client := NewClient()

	t.Run("Find all nested repositories", func(t *testing.T) {
		opts := BulkPushOptions{
			Directory:         tmpDir,
			MaxDepth:          5,
			DryRun:            true,
			IncludeSubmodules: false,
			Logger:            NewNoopLogger(),
		}

		result, err := client.BulkPush(ctx, opts)
		if err != nil {
			t.Fatalf("BulkPush failed: %v", err)
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
		// Directory structure from tmpDir:
		// tmpDir/                    (depth 0, not a repo)
		// └── parent/                (depth 1, repo)
		//     ├── nested-repo1/      (depth 2, repo)
		//     │   └── deep-nested/   (depth 3, repo)
		//     └── nested-repo2/      (depth 2, repo)
		//
		// maxDepth=3 scans up to and including depth 3
		// Should find: parent (d1), nested-repo1 (d2), deep-nested (d3), nested-repo2 (d2)
		opts := BulkPushOptions{
			Directory:         tmpDir,
			MaxDepth:          3, // Scan depths 0, 1, 2, 3
			DryRun:            true,
			IncludeSubmodules: false,
			Logger:            NewNoopLogger(),
		}

		result, err := client.BulkPush(ctx, opts)
		if err != nil {
			t.Fatalf("BulkPush failed: %v", err)
		}

		// Should find all 4 repos: parent, nested-repo1, deep-nested, nested-repo2
		if result.TotalScanned != 4 {
			t.Errorf("Expected 4 repositories with max-depth 3, got %d", result.TotalScanned)
			for _, repo := range result.Repositories {
				t.Logf("Found: %s", repo.RelativePath)
			}
		}
	})

	t.Run("Depth 0 uses default depth", func(t *testing.T) {
		// depth=0 should use default depth at package level (DefaultBulkMaxDepth=1)
		// CLI level validation prevents users from explicitly passing 0
		// maxDepth=1 scans depth 0 and 1 (tmpDir is at depth 0, parent at depth 1)
		// Finds: parent (d1)
		opts := BulkPushOptions{
			Directory:         tmpDir,
			MaxDepth:          0, // Will be set to default (DefaultBulkMaxDepth=1)
			DryRun:            true,
			IncludeSubmodules: false,
			Logger:            NewNoopLogger(),
		}

		result, err := client.BulkPush(ctx, opts)
		if err != nil {
			t.Fatalf("BulkPush with depth=0 failed: %v", err)
		}

		// depth=0 is set to DefaultBulkMaxDepth (1), which scans depth 0 and 1
		// Finds: parent (d1) = 1 repo
		if result.TotalScanned != 1 {
			t.Errorf("Expected 1 repository with default depth=1, got %d", result.TotalScanned)
			for _, repo := range result.Repositories {
				t.Logf("Found: %s", repo.RelativePath)
			}
		}
	})
}

func TestBulkPushEmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	ctx := context.Background()
	client := NewClient()

	opts := BulkPushOptions{
		Directory: tmpDir,
		MaxDepth:  2,
		DryRun:    true,
		Logger:    NewNoopLogger(),
	}

	result, err := client.BulkPush(ctx, opts)
	if err != nil {
		t.Fatalf("BulkPush on empty directory failed: %v", err)
	}

	if result.TotalScanned != 0 {
		t.Errorf("Expected 0 scanned repositories in empty directory, got %d", result.TotalScanned)
	}

	if result.TotalProcessed != 0 {
		t.Errorf("Expected 0 processed repositories in empty directory, got %d", result.TotalProcessed)
	}
}

func TestRepositoryPushResult(t *testing.T) {
	result := RepositoryPushResult{
		Path:          "/tmp/test",
		Status:        "success",
		Duration:      100 * time.Millisecond,
		CommitsAhead:  5,
		PushedCommits: 5,
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

	if result.CommitsAhead != 5 {
		t.Errorf("Expected CommitsAhead 5, got %d", result.CommitsAhead)
	}

	if result.PushedCommits != 5 {
		t.Errorf("Expected PushedCommits 5, got %d", result.PushedCommits)
	}
}

func TestCalculatePushSummary(t *testing.T) {
	results := []RepositoryPushResult{
		{Status: "success"},
		{Status: "success"},
		{Status: "error"},
		{Status: "no-remote"},
		{Status: "would-push"},
		{Status: "nothing-to-push"},
	}

	summary := calculatePushSummary(results)

	if summary["success"] != 2 {
		t.Errorf("Expected 2 success, got %d", summary["success"])
	}

	if summary["error"] != 1 {
		t.Errorf("Expected 1 error, got %d", summary["error"])
	}

	if summary["no-remote"] != 1 {
		t.Errorf("Expected 1 no-remote, got %d", summary["no-remote"])
	}

	if summary["would-push"] != 1 {
		t.Errorf("Expected 1 would-push, got %d", summary["would-push"])
	}

	if summary["nothing-to-push"] != 1 {
		t.Errorf("Expected 1 nothing-to-push, got %d", summary["nothing-to-push"])
	}
}

func TestBulkPushNoRemote(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test repository without remote
	repoPath := filepath.Join(tmpDir, "repo-no-remote")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	if err := initGitRepoWithCommit(repoPath); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	ctx := context.Background()
	client := NewClient()

	opts := BulkPushOptions{
		Directory: tmpDir,
		MaxDepth:  2,
		DryRun:    false, // Try actual push
		Logger:    NewNoopLogger(),
	}

	result, err := client.BulkPush(ctx, opts)
	if err != nil {
		t.Fatalf("BulkPush failed: %v", err)
	}

	if len(result.Repositories) != 1 {
		t.Fatalf("Expected 1 repository, got %d", len(result.Repositories))
	}

	// Should report no-remote status
	if result.Repositories[0].Status != StatusNoRemote {
		t.Errorf("Expected status %s, got %s", StatusNoRemote, result.Repositories[0].Status)
	}
}

// Benchmark tests for bulk push performance

func BenchmarkBulkPushSingleRepo(b *testing.B) {
	tmpDir := b.TempDir()
	repoPath := filepath.Join(tmpDir, "repo")

	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		b.Fatalf("Failed to create repo: %v", err)
	}
	if err := initGitRepo(repoPath); err != nil {
		b.Skipf("Skipping benchmark: git not available: %v", err)
	}

	ctx := context.Background()
	client := NewClient()

	opts := BulkPushOptions{
		Directory: tmpDir,
		MaxDepth:  2,
		DryRun:    true,
		Logger:    NewNoopLogger(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.BulkPush(ctx, opts)
		if err != nil {
			b.Fatalf("BulkPush failed: %v", err)
		}
	}
}

func BenchmarkBulkPushMultipleRepos(b *testing.B) {
	tmpDir := b.TempDir()

	// Create 10 test repositories
	for i := range 10 {
		repoPath := filepath.Join(tmpDir, filepath.Base(tmpDir)+"-repo-"+string(rune('0'+i)))
		if err := os.MkdirAll(repoPath, 0o755); err != nil {
			b.Fatalf("Failed to create repo%d: %v", i, err)
		}
		if err := initGitRepo(repoPath); err != nil {
			b.Skipf("Skipping benchmark: git not available: %v", err)
		}
	}

	ctx := context.Background()
	client := NewClient()

	opts := BulkPushOptions{
		Directory: tmpDir,
		MaxDepth:  2,
		DryRun:    true,
		Parallel:  10,
		Logger:    NewNoopLogger(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.BulkPush(ctx, opts)
		if err != nil {
			b.Fatalf("BulkPush failed: %v", err)
		}
	}
}

func BenchmarkBulkPushNestedRepos(b *testing.B) {
	tmpDir := b.TempDir()

	// Create parent repository
	parentPath := filepath.Join(tmpDir, "parent")
	if err := os.MkdirAll(parentPath, 0o755); err != nil {
		b.Fatalf("Failed to create parent: %v", err)
	}
	if err := initGitRepo(parentPath); err != nil {
		b.Skipf("Skipping benchmark: git not available: %v", err)
	}

	// Create 5 nested repositories
	for i := range 5 {
		nestedPath := filepath.Join(parentPath, filepath.Base(tmpDir)+"-nested-"+string(rune('0'+i)))
		if err := os.MkdirAll(nestedPath, 0o755); err != nil {
			b.Fatalf("Failed to create nested%d: %v", i, err)
		}
		if err := initGitRepo(nestedPath); err != nil {
			b.Skipf("Skipping benchmark: git not available: %v", err)
		}
	}

	ctx := context.Background()
	client := NewClient()

	opts := BulkPushOptions{
		Directory:         tmpDir,
		MaxDepth:          3,
		DryRun:            true,
		Parallel:          10,
		IncludeSubmodules: false,
		Logger:            NewNoopLogger(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.BulkPush(ctx, opts)
		if err != nil {
			b.Fatalf("BulkPush failed: %v", err)
		}
	}
}

func BenchmarkPushParallelProcessing(b *testing.B) {
	tmpDir := b.TempDir()

	// Create 20 repositories
	for i := range 20 {
		repoPath := filepath.Join(tmpDir, filepath.Base(tmpDir)+"-repo-"+string(rune('0'+i/10))+string(rune('0'+i%10)))
		if err := os.MkdirAll(repoPath, 0o755); err != nil {
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
			opts := BulkPushOptions{
				Directory: tmpDir,
				MaxDepth:  2,
				DryRun:    true,
				Parallel:  parallel,
				Logger:    NewNoopLogger(),
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := client.BulkPush(ctx, opts)
				if err != nil {
					b.Fatalf("BulkPush failed: %v", err)
				}
			}
		})
	}
}

func TestBulkPushDepthBehavior(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested repository structure:
	// tmpDir/
	//   repo1/
	//   subdir/
	//     repo2/
	//     nested/
	//       repo3/

	repo1Path := filepath.Join(tmpDir, "repo1")
	repo2Path := filepath.Join(tmpDir, "subdir", "repo2")
	repo3Path := filepath.Join(tmpDir, "subdir", "nested", "repo3")

	for _, path := range []string{repo1Path, repo2Path, repo3Path} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", path, err)
		}
		if err := initGitRepo(path); err != nil {
			t.Skipf("Skipping test: git not available: %v", err)
		}
	}

	ctx := context.Background()
	client := NewClient()

	t.Run("depth=1 scans only root directory", func(t *testing.T) {
		opts := BulkPushOptions{
			Directory: tmpDir,
			MaxDepth:  1,
			DryRun:    true,
			Logger:    NewNoopLogger(),
		}

		result, err := client.BulkPush(ctx, opts)
		if err != nil {
			t.Fatalf("BulkPush failed: %v", err)
		}

		// Should only find repo1 (direct child of tmpDir)
		// Should NOT find repo2 or repo3 (they are deeper)
		if result.TotalScanned != 1 {
			t.Errorf("depth=1 should scan only 1 repository (repo1), got %d", result.TotalScanned)
		}

		// Verify it's repo1
		if len(result.Repositories) == 1 {
			if result.Repositories[0].RelativePath != "repo1" {
				t.Errorf("Expected to find repo1, got %s", result.Repositories[0].RelativePath)
			}
		}
	})

	t.Run("depth=2 scans root + 1 level", func(t *testing.T) {
		opts := BulkPushOptions{
			Directory: tmpDir,
			MaxDepth:  2,
			DryRun:    true,
			Logger:    NewNoopLogger(),
		}

		result, err := client.BulkPush(ctx, opts)
		if err != nil {
			t.Fatalf("BulkPush failed: %v", err)
		}

		// Should find repo1 and repo2
		// Should NOT find repo3 (it's at depth 3)
		if result.TotalScanned != 2 {
			t.Errorf("depth=2 should scan 2 repositories (repo1, repo2), got %d", result.TotalScanned)
		}
	})

	t.Run("depth=3 scans root + 2 levels", func(t *testing.T) {
		opts := BulkPushOptions{
			Directory: tmpDir,
			MaxDepth:  3,
			DryRun:    true,
			Logger:    NewNoopLogger(),
		}

		result, err := client.BulkPush(ctx, opts)
		if err != nil {
			t.Fatalf("BulkPush failed: %v", err)
		}

		// Should find all 3 repositories
		if result.TotalScanned != 3 {
			t.Errorf("depth=3 should scan 3 repositories (repo1, repo2, repo3), got %d", result.TotalScanned)
		}
	})
}

func TestBulkPush_RefspecSourceBranchMissing(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test repository with only master branch
	repoPath := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("Failed to create repo: %v", err)
	}
	if err := initGitRepoWithCommit(repoPath); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	ctx := context.Background()
	client := NewClient()

	// Try to push with refspec that references non-existent develop branch
	opts := BulkPushOptions{
		Directory: tmpDir,
		MaxDepth:  2,
		Refspec:   "develop:master",
		DryRun:    false,
		Verbose:   false,
		Logger:    NewNoopLogger(),
	}

	result, err := client.BulkPush(ctx, opts)
	if err != nil {
		t.Fatalf("BulkPush() unexpected error: %v", err)
	}

	// Should have processed 1 repository
	if result.TotalProcessed != 1 {
		t.Errorf("BulkPush() TotalProcessed = %d, want 1", result.TotalProcessed)
	}

	// Should have 1 repository result
	if len(result.Repositories) != 1 {
		t.Fatalf("BulkPush() len(Repositories) = %d, want 1", len(result.Repositories))
	}

	repoResult := result.Repositories[0]

	// Status should be "error"
	if repoResult.Status != StatusError {
		t.Errorf("BulkPush() Status = %q, want %q", repoResult.Status, StatusError)
	}

	// Error should mention the missing branch
	if repoResult.Error == nil {
		t.Fatal("BulkPush() Error = nil, want error about missing branch")
	}

	errMsg := repoResult.Error.Error()
	if !strings.Contains(errMsg, "develop") {
		t.Errorf("BulkPush() Error = %q, want error containing 'develop'", errMsg)
	}

	if !strings.Contains(errMsg, "not found") {
		t.Errorf("BulkPush() Error = %q, want error containing 'not found'", errMsg)
	}

	// Message should indicate source branch missing
	if !strings.Contains(repoResult.Message, "does not exist") {
		t.Errorf("BulkPush() Message = %q, want message containing 'does not exist'", repoResult.Message)
	}
}

func TestBulkPush_RefspecWithValidSourceBranch(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test repository
	repoPath := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("Failed to create repo: %v", err)
	}
	if err := initGitRepoWithCommit(repoPath); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	// Create develop branch
	cmd := exec.Command("git", "checkout", "-b", "develop") //nolint:noctx // test setup, no context available
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Skipf("Skipping test: failed to create develop branch: %v", err)
	}

	// Make a commit on develop
	testFile := filepath.Join(repoPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	cmd = exec.Command("git", "add", ".") //nolint:noctx // test setup, no context available
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Skipf("Skipping test: git add failed: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Test commit") //nolint:noctx // test setup, no context available
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Skipf("Skipping test: git commit failed: %v", err)
	}

	ctx := context.Background()
	client := NewClient()

	// Push with refspec should work (dry-run to avoid actual remote push)
	opts := BulkPushOptions{
		Directory: tmpDir,
		MaxDepth:  2,
		Refspec:   "develop:master",
		DryRun:    true,
		Verbose:   false,
		Logger:    NewNoopLogger(),
	}

	result, err := client.BulkPush(ctx, opts)
	if err != nil {
		t.Fatalf("BulkPush() unexpected error: %v", err)
	}

	// Should have processed 1 repository
	if result.TotalProcessed != 1 {
		t.Errorf("BulkPush() TotalProcessed = %d, want 1", result.TotalProcessed)
	}

	// Should have 1 repository result
	if len(result.Repositories) != 1 {
		t.Fatalf("BulkPush() len(Repositories) = %d, want 1", len(result.Repositories))
	}

	repoResult := result.Repositories[0]

	// Status should NOT be "error" with "not found" message
	// (Will be "no-remote" or "would-push" since we have no remote)
	if repoResult.Status == StatusError {
		if repoResult.Error != nil && strings.Contains(repoResult.Error.Error(), "not found") {
			t.Errorf("BulkPush() got 'not found' error when source branch exists: %v", repoResult.Error)
		}
	}
}

func TestBulkPush_RefspecInvalidFormat(t *testing.T) {
	tmpDir := t.TempDir()

	repoPath := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("Failed to create repo: %v", err)
	}
	if err := initGitRepo(repoPath); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	ctx := context.Background()
	client := NewClient()

	invalidRefspecs := []string{
		"develop::master",  // double colon
		":master",          // empty source
		"develop:",         // empty destination
		"branch name:main", // contains space
	}

	for _, refspec := range invalidRefspecs {
		t.Run("Invalid_"+refspec, func(t *testing.T) {
			opts := BulkPushOptions{
				Directory: tmpDir,
				MaxDepth:  2,
				Refspec:   refspec,
				DryRun:    true,
				Logger:    NewNoopLogger(),
			}

			result, err := client.BulkPush(ctx, opts)
			if err != nil {
				t.Fatalf("BulkPush() unexpected error: %v", err)
			}

			// Should have error status for invalid refspec
			if len(result.Repositories) > 0 {
				if result.Repositories[0].Status != StatusError {
					t.Errorf("Expected StatusError for invalid refspec %q, got %q", refspec, result.Repositories[0].Status)
				}
			}
		})
	}
}

func TestBuildPushError(t *testing.T) {
	t.Run("returns execErr when non-nil", func(t *testing.T) {
		execErr := errors.New("process start failed")
		got := buildPushError(execErr, 128, "some stderr", nil)
		if !errors.Is(got, execErr) {
			t.Errorf("expected execErr returned directly, got %v", got)
		}
	})

	// Regression: a rejected push exits 128 with the reason only in stderr,
	// while the command error is a bare "exit status 128". Reporting the
	// command error alone produced "push exited with code 128: exit status
	// 128", which restates the code and drops git's diagnosis entirely.
	t.Run("prefers stderr over the bare exit status", func(t *testing.T) {
		stderr := "ERROR: Permission to iheanyi/tasuku.git denied to archmagece.\n" +
			"fatal: Could not read from remote repository.\n"
		got := buildPushError(nil, 128, stderr, errors.New("exit status 128"))
		if strings.Contains(got.Error(), "exit status 128") {
			t.Errorf("bare exit status should not survive when stderr explains the failure, got %q", got.Error())
		}
		if !strings.Contains(got.Error(), "Permission to iheanyi/tasuku.git denied") {
			t.Errorf("expected git's diagnosis in message, got %q", got.Error())
		}
	})

	t.Run("collapses multi-line stderr onto one line", func(t *testing.T) {
		stderr := "ERROR: Permission denied.\nfatal: Could not read from remote repository.\n"
		got := buildPushError(nil, 128, stderr, nil)
		if strings.Contains(got.Error(), "\n") {
			t.Errorf("bulk output prints one line per repo, got %q", got.Error())
		}
		want := "push exited with code 128: ERROR: Permission denied. fatal: Could not read from remote repository."
		if got.Error() != want {
			t.Errorf("expected %q, got %q", want, got.Error())
		}
	})

	t.Run("falls back to cmdErr when stderr empty", func(t *testing.T) {
		cmdErr := errors.New("exit status 1")
		got := buildPushError(nil, 1, "", cmdErr)
		if !errors.Is(got, cmdErr) {
			t.Errorf("expected cmdErr wrapped, got %v", got)
		}
		if !strings.Contains(got.Error(), "code 1") {
			t.Errorf("expected exit code in message, got %q", got.Error())
		}
	})

	t.Run("returns exit code only when nothing else available", func(t *testing.T) {
		got := buildPushError(nil, 128, "", nil)
		want := "push exited with code 128"
		if got.Error() != want {
			t.Errorf("expected %q, got %q", want, got.Error())
		}
	})
}

func TestGitStderrSummary(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		want   string
	}{
		{"empty", "", ""},
		{"whitespace only", "  \n\t\n  ", ""},
		{"single line trimmed", "  fatal: no upstream  \n", "fatal: no upstream"},
		{
			"blank lines dropped",
			"ERROR: denied.\n\nfatal: could not read.\n\nPlease check access.\n",
			"ERROR: denied. fatal: could not read. Please check access.",
		},
		{"carriage returns trimmed", "remote: rejected\r\nfatal: failed\r\n", "remote: rejected fatal: failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gitStderrSummary(tt.stderr); got != tt.want {
				t.Errorf("gitStderrSummary(%q) = %q, want %q", tt.stderr, got, tt.want)
			}
		})
	}
}
