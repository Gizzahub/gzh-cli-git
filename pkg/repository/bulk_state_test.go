package repository

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestCheckRepositoryState tests the detailed repository state detection
func TestCheckRepositoryState(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")

	// Initialize git repository
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create repo: %v", err)
	}

	// Initialize git
	cmd := exec.Command("git", "init")
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
		cmd := exec.Command("git", args...)
		cmd.Dir = repoPath
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to config git: %v", err)
		}
	}

	// Create initial commit
	testFile := filepath.Join(repoPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git commit: %v", err)
	}

	ctx := context.Background()
	c := NewClient().(*client)

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
		if state.IsDirty {
			t.Error("Expected clean repository")
		}
	})

	t.Run("dirty repository", func(t *testing.T) {
		// Modify file
		if err := os.WriteFile(testFile, []byte("modified content"), 0644); err != nil {
			t.Fatalf("Failed to modify test file: %v", err)
		}

		state, err := c.checkRepositoryState(ctx, repoPath)
		if err != nil {
			t.Fatalf("Failed to check repository state: %v", err)
		}

		if !state.IsDirty {
			t.Error("Expected dirty repository")
		}
		if state.UncommittedFiles == 0 {
			t.Error("Expected uncommitted files")
		}

		// Clean up - reset the file
		cmd := exec.Command("git", "checkout", ".")
		cmd.Dir = repoPath
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to reset file: %v", err)
		}
	})

	t.Run("rebase marker detection", func(t *testing.T) {
		// Simulate rebase in progress by creating .git/rebase-merge directory
		rebaseMergeDir := filepath.Join(repoPath, ".git", "rebase-merge")
		if err := os.MkdirAll(rebaseMergeDir, 0755); err != nil {
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
		if err := os.WriteFile(mergeHeadFile, []byte("fake-commit-hash"), 0644); err != nil {
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

// TestStatusConstants tests the new status constants
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
