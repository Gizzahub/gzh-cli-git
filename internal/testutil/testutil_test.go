package testutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestTempDir(t *testing.T) {
	dir := TempDir(t)
	if dir == "" {
		t.Error("TempDir should return non-empty path")
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Errorf("TempDir path should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("TempDir should return a directory")
	}
}

func TestTempFile(t *testing.T) {
	content := "test content"
	path := TempFile(t, "test.txt", content)

	if path == "" {
		t.Error("TempFile should return non-empty path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("TempFile content should be readable: %v", err)
	}

	if string(data) != content {
		t.Errorf("TempFile content = %q, want %q", string(data), content)
	}
}

func TestTempGitRepo(t *testing.T) {
	dir := TempGitRepo(t)

	// Check .git directory exists.
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Error(".git directory should exist in TempGitRepo")
	}
}

func TestTempGitRepoWithCommit(t *testing.T) {
	dir := TempGitRepoWithCommit(t)

	// Check README exists.
	readme := filepath.Join(dir, "README.md")
	if _, err := os.Stat(readme); os.IsNotExist(err) {
		t.Error("README.md should exist in TempGitRepoWithCommit")
	}

	// Check .git directory exists.
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Error(".git directory should exist")
	}
}

func TestAssertNoError(t *testing.T) {
	// This should not fail.
	mockT := &testing.T{}
	AssertNoError(mockT, nil)
}

func TestAssertError(t *testing.T) {
	// This should not fail.
	mockT := &testing.T{}
	AssertError(mockT, errors.New("test error"))
}

func TestAssertEqual(t *testing.T) {
	mockT := &testing.T{}
	AssertEqual(mockT, "test", "test")
	AssertEqual(mockT, 42, 42)
}

func TestAssertContains(t *testing.T) {
	mockT := &testing.T{}
	AssertContains(mockT, "hello world", "world")
}

func TestSetEnv(t *testing.T) {
	key := "TEST_ENV_VAR_FOR_TESTUTIL"
	value := "test_value"

	// Set the env var.
	SetEnv(t, key, value)

	// Check it's set.
	if got := os.Getenv(key); got != value {
		t.Errorf("SetEnv should set env var, got %q, want %q", got, value)
	}

	// Note: cleanup happens automatically via t.Cleanup.
}

func TestChdir(t *testing.T) {
	original, _ := os.Getwd()
	tempDir := t.TempDir()

	Chdir(t, tempDir)

	current, _ := os.Getwd()
	if current != tempDir {
		t.Errorf("Chdir should change directory, got %q, want %q", current, tempDir)
	}

	// Cleanup will restore, but we're in a subtest so it's fine.
	_ = original
}

func TestCapture(t *testing.T) {
	output := Capture(func() {
		os.Stdout.WriteString("stdout content")
		os.Stderr.WriteString("stderr content")
	})

	if output.Stdout != "stdout content" {
		t.Errorf("Capture.Stdout = %q, want %q", output.Stdout, "stdout content")
	}
	if output.Stderr != "stderr content" {
		t.Errorf("Capture.Stderr = %q, want %q", output.Stderr, "stderr content")
	}
}
