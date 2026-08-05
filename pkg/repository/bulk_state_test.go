package repository

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestCheckRepositoryState tests the detailed repository state detection.
func TestCheckRepositoryState(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")

	// Initialize git repository
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("Failed to create repo: %v", err)
	}

	// Initialize git
	cmd := exec.Command("git", "init") //nolint:noctx // test setup, no context available
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git: %v", err)
	}

	// Configure git for testing
	configCmds := [][]string{
		{"config", "user.name", "Test User"},
		{"config", "user.email", "test@example.com"},
	}
	for _, args := range configCmds {
		cmd := exec.Command("git", args...) //nolint:noctx // test setup, no context available
		cmd.Dir = repoPath
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to config git: %v", err)
		}
	}

	// Create initial commit
	testFile := filepath.Join(repoPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial content"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", ".") //nolint:noctx // test setup, no context available
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit") //nolint:noctx // test setup, no context available
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git commit: %v", err)
	}

	ctx := context.Background()
	repo := NewClient()
	c, ok := repo.(*client)
	if !ok {
		t.Fatal("NewClient did not return *client")
	}

	t.Run("clean repository", func(t *testing.T) {
		state, err := c.checkRepositoryState(ctx, repoPath)
		if err != nil {
			t.Fatalf("Failed to check repository state: %v", err)
		}

		if state.HasConflicts {
			t.Error("Expected no conflicts")
		}
		if state.RebaseInProgress {
			t.Error("Expected no rebase in progress")
		}
		if state.MergeInProgress {
			t.Error("Expected no merge in progress")
		}
		if len(state.ConflictedFiles) != 0 {
			t.Errorf("Expected no conflicted files, got %v", state.ConflictedFiles)
		}
	})

	t.Run("dirty repository is not conflicted", func(t *testing.T) {
		// Modify file
		if err := os.WriteFile(testFile, []byte("modified content"), 0o644); err != nil {
			t.Fatalf("Failed to modify test file: %v", err)
		}

		state, err := c.checkRepositoryState(ctx, repoPath)
		if err != nil {
			t.Fatalf("Failed to check repository state: %v", err)
		}

		// checkRepositoryState no longer reports counts; a merely modified file
		// must not be mistaken for an unmerged one.
		if state.HasConflicts {
			t.Errorf("Expected no conflicts for a modified file, got %v", state.ConflictedFiles)
		}

		status, err := c.GetStatus(ctx, &Repository{Path: repoPath})
		if err != nil {
			t.Fatalf("Failed to get status: %v", err)
		}
		if status.TrackedChangedCount != 1 {
			t.Errorf("TrackedChangedCount = %d, want 1", status.TrackedChangedCount)
		}
		if status.UnstagedCount != 1 {
			t.Errorf("UnstagedCount = %d, want 1", status.UnstagedCount)
		}
		if status.StagedCount != 0 {
			t.Errorf("StagedCount = %d, want 0", status.StagedCount)
		}

		// Clean up - reset the file
		cmd := exec.Command("git", "checkout", ".") //nolint:noctx // test cleanup, no context available
		cmd.Dir = repoPath
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to reset file: %v", err)
		}
	})

	t.Run("rebase marker detection", func(t *testing.T) {
		// Simulate rebase in progress by creating .git/rebase-merge directory
		rebaseMergeDir := filepath.Join(repoPath, ".git", "rebase-merge")
		if err := os.MkdirAll(rebaseMergeDir, 0o755); err != nil {
			t.Fatalf("Failed to create rebase-merge dir: %v", err)
		}

		state, err := c.checkRepositoryState(ctx, repoPath)
		if err != nil {
			t.Fatalf("Failed to check repository state: %v", err)
		}

		if !state.RebaseInProgress {
			t.Error("Expected rebase in progress")
		}

		// Clean up
		os.RemoveAll(rebaseMergeDir)
	})

	t.Run("merge marker detection", func(t *testing.T) {
		// Simulate merge in progress by creating MERGE_HEAD file
		mergeHeadFile := filepath.Join(repoPath, ".git", "MERGE_HEAD")
		if err := os.WriteFile(mergeHeadFile, []byte("fake-commit-hash"), 0o644); err != nil {
			t.Fatalf("Failed to create MERGE_HEAD: %v", err)
		}

		state, err := c.checkRepositoryState(ctx, repoPath)
		if err != nil {
			t.Fatalf("Failed to check repository state: %v", err)
		}

		if !state.MergeInProgress {
			t.Error("Expected merge in progress")
		}

		// Clean up
		os.Remove(mergeHeadFile)
	})
}

// TestStatusConstants tests the new status constants.
func TestStatusConstants(t *testing.T) {
	tests := []struct {
		status   string
		expected string
	}{
		{StatusDirty, "dirty"},
		{StatusConflict, "conflict"},
		{StatusRebaseInProgress, "rebase-in-progress"},
		{StatusMergeInProgress, "merge-in-progress"},
	}

	for _, tt := range tests {
		if tt.status != tt.expected {
			t.Errorf("Status constant mismatch: got %s, want %s", tt.status, tt.expected)
		}
	}
}
