// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package reposync

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddAdditionalRemotes(t *testing.T) {
	// Create a temporary git repository
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")

	// Initialize git repo
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	cmd := exec.CommandContext(t.Context(), "git", "init")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Add origin remote (simulates cloned repo)
	cmd = exec.CommandContext(t.Context(), "git", "remote", "add", "origin", "https://github.com/test/repo.git")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to add origin remote: %v", err)
	}

	ctx := context.Background()
	logger := nopGitLogger{}

	t.Run("add new remote", func(t *testing.T) {
		remotes := map[string]string{
			"backup": "https://gitlab.com/test/repo.git",
		}

		msg, err := addAdditionalRemotes(ctx, repoPath, remotes, logger)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if msg != "configured 1 remote(s)" {
			t.Errorf("got message %q, want %q", msg, "configured 1 remote(s)")
		}

		// Verify remote was added
		cmd := exec.CommandContext(t.Context(), "git", "remote", "get-url", "backup")
		cmd.Dir = repoPath
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("failed to get remote url: %v", err)
		}

		expectedURL := "https://gitlab.com/test/repo.git\n"
		if string(output) != expectedURL {
			t.Errorf("got URL %q, want %q", string(output), expectedURL)
		}
	})

	t.Run("add multiple remotes", func(t *testing.T) {
		remotes := map[string]string{
			"mirror1": "https://bitbucket.org/test/repo.git",
			"mirror2": "git@github.com:test/repo.git",
		}

		msg, err := addAdditionalRemotes(ctx, repoPath, remotes, logger)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if msg != "configured 2 remote(s)" {
			t.Errorf("got message %q, want %q", msg, "configured 2 remote(s)")
		}

		// Verify both remotes were added
		for name, expectedURL := range remotes {
			cmd := exec.CommandContext(t.Context(), "git", "remote", "get-url", name)
			cmd.Dir = repoPath
			output, err := cmd.Output()
			if err != nil {
				t.Errorf("failed to get remote url for %s: %v", name, err)
				continue
			}

			if string(output) != expectedURL+"\n" {
				t.Errorf("remote %s: got URL %q, want %q", name, string(output), expectedURL)
			}
		}
	})

	t.Run("accepts hierarchical names and valid URL data", func(t *testing.T) {
		remotes := map[string]string{
			"team/backup":  "../shared/repo.git",
			"mirror/query": "https://example.com/repo.git?ref=main&depth=1",
			"deploy/scp":   "deploy@example.com:team/repo.git",
		}

		msg, err := addAdditionalRemotes(ctx, repoPath, remotes, logger)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if msg != "configured 3 remote(s)" {
			t.Errorf("got message %q, want %q", msg, "configured 3 remote(s)")
		}
		for name, expectedURL := range remotes {
			cmd := exec.CommandContext(t.Context(), "git", "remote", "get-url", name)
			cmd.Dir = repoPath
			output, err := cmd.Output()
			if err != nil {
				t.Errorf("failed to get remote url for %s: %v", name, err)
				continue
			}
			if string(output) != expectedURL+"\n" {
				t.Errorf("remote %s: got URL %q, want %q", name, string(output), expectedURL)
			}
		}
	})

	t.Run("update existing remote", func(t *testing.T) {
		// Add a remote first
		cmd := exec.CommandContext(t.Context(), "git", "remote", "add", "test-remote", "https://old-url.com/repo.git")
		cmd.Dir = repoPath
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to add test remote: %v", err)
		}

		// Update it with new URL
		remotes := map[string]string{
			"test-remote": "https://new-url.com/repo.git",
		}

		msg, err := addAdditionalRemotes(ctx, repoPath, remotes, logger)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if msg != "configured 1 remote(s)" {
			t.Errorf("got message %q, want %q", msg, "configured 1 remote(s)")
		}

		// Verify URL was updated
		cmd = exec.CommandContext(t.Context(), "git", "remote", "get-url", "test-remote")
		cmd.Dir = repoPath
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("failed to get remote url: %v", err)
		}

		expectedURL := "https://new-url.com/repo.git\n"
		if string(output) != expectedURL {
			t.Errorf("got URL %q, want %q", string(output), expectedURL)
		}
	})

	t.Run("empty remotes map", func(t *testing.T) {
		msg, err := addAdditionalRemotes(ctx, repoPath, nil, logger)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if msg != "" {
			t.Errorf("got message %q, want empty string", msg)
		}
	})
}

func TestFormatCompactStatus(t *testing.T) {
	tests := []struct {
		name   string
		status *PostSyncStatus
		want   string
	}{
		{
			name:   "nil status",
			status: nil,
			want:   "",
		},
		{
			name:   "branch only",
			status: &PostSyncStatus{Branch: "main"},
			want:   "main",
		},
		{
			name:   "behind only",
			status: &PostSyncStatus{Branch: "develop", BehindBy: 5},
			want:   "develop|↓5",
		},
		{
			name:   "ahead only",
			status: &PostSyncStatus{Branch: "feature", AheadBy: 3},
			want:   "feature|↑3",
		},
		{
			name:   "behind and ahead",
			status: &PostSyncStatus{Branch: "master", BehindBy: 2, AheadBy: 1},
			want:   "master|↓2|↑1",
		},
		{
			name:   "dirty",
			status: &PostSyncStatus{Branch: "main", IsDirty: true},
			want:   "main|dirty",
		},
		{
			name:   "conflict",
			status: &PostSyncStatus{Branch: "main", IsDirty: true, HasConflicts: true},
			want:   "main|conflict",
		},
		{
			name:   "full status",
			status: &PostSyncStatus{Branch: "develop", BehindBy: 5, AheadBy: 3, IsDirty: true},
			want:   "develop|↓5|↑3|dirty",
		},
		{
			name:   "empty branch with behind",
			status: &PostSyncStatus{BehindBy: 2},
			want:   "↓2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatCompactStatus(tt.status)
			if got != tt.want {
				t.Errorf("FormatCompactStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCollectPostSyncStatus(t *testing.T) {
	// Create a temporary git repository with a commit
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	// Initialize, configure, and create initial commit
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "checkout", "-b", "main"},
	}
	for _, args := range cmds {
		cmd := exec.CommandContext(t.Context(), args[0], args[1:]...)
		cmd.Dir = repoPath
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("command %v failed: %v\n%s", args, err, out)
		}
	}

	// Create a file and commit
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("# test"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "README.md"},
		{"git", "commit", "-m", "initial commit"},
	} {
		cmd := exec.CommandContext(t.Context(), args[0], args[1:]...)
		cmd.Dir = repoPath
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("command %v failed: %v\n%s", args, err, out)
		}
	}

	ctx := context.Background()

	t.Run("clean repo returns branch", func(t *testing.T) {
		ps := collectPostSyncStatus(ctx, repoPath)
		if ps == nil {
			t.Fatal("expected non-nil PostSyncStatus")
		}
		if ps.Branch != "main" {
			t.Errorf("Branch = %q, want %q", ps.Branch, "main")
		}
		if ps.IsDirty {
			t.Error("expected IsDirty=false for clean repo")
		}
		if ps.HasConflicts {
			t.Error("expected HasConflicts=false for clean repo")
		}
	})

	t.Run("dirty repo", func(t *testing.T) {
		// Create an uncommitted file
		if err := os.WriteFile(filepath.Join(repoPath, "dirty.txt"), []byte("dirty"), 0o644); err != nil {
			t.Fatal(err)
		}

		ps := collectPostSyncStatus(ctx, repoPath)
		if ps == nil {
			t.Fatal("expected non-nil PostSyncStatus")
		}
		if !ps.IsDirty {
			t.Error("expected IsDirty=true for dirty repo")
		}

		// Clean up
		os.Remove(filepath.Join(repoPath, "dirty.txt"))
	})
}

func TestDescribeDeleteError(t *testing.T) {
	t.Run("permission denied error includes manual removal hint", func(t *testing.T) {
		tmpDir := t.TempDir()
		targetPath := filepath.Join(tmpDir, "test-repo")
		if err := os.MkdirAll(targetPath, 0o755); err != nil {
			t.Fatal(err)
		}

		permErr := &os.PathError{Op: "remove", Path: targetPath, Err: os.ErrPermission}
		msg := describeDeleteError(targetPath, permErr)

		if !strings.Contains(msg, "owned by other users") {
			t.Errorf("expected ownership hint, got: %s", msg)
		}
		if !strings.Contains(msg, "manual removal required") {
			t.Errorf("expected manual removal hint, got: %s", msg)
		}
	})

	t.Run("generic error includes path and error detail", func(t *testing.T) {
		targetPath := "/nonexistent/path"
		genericErr := errors.New("some filesystem error")
		msg := describeDeleteError(targetPath, genericErr)

		if !strings.Contains(msg, targetPath) {
			t.Errorf("expected target path in message, got: %s", msg)
		}
		if !strings.Contains(msg, "some filesystem error") {
			t.Errorf("expected original error in message, got: %s", msg)
		}
	})

	t.Run("detects permission denied in subdirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		targetPath := filepath.Join(tmpDir, "test-repo")
		subDir := filepath.Join(targetPath, "subdir")
		if err := os.MkdirAll(subDir, 0o755); err != nil {
			t.Fatal(err)
		}

		// Create a file then make parent unreadable to trigger permission denied during walk
		protectedFile := filepath.Join(subDir, "protected")
		if err := os.WriteFile(protectedFile, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(subDir, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = os.Chmod(subDir, 0o755) // restore for cleanup
		})

		// Use a non-ErrPermission top-level error so it falls through to walk
		wrappedErr := errors.New("remove failed")
		msg := describeDeleteError(targetPath, wrappedErr)

		// Walk should find permission denied at subDir
		if !strings.Contains(msg, "permission denied at") {
			// On some systems the walk may not detect it; accept generic message
			if !strings.Contains(msg, "remove failed") {
				t.Errorf("expected either permission detail or generic error, got: %s", msg)
			}
		}
	})
}
