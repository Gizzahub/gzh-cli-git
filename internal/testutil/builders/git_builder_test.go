package builders

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitRepoBuilder(t *testing.T) {
	dir := NewGitRepoBuilder(t).
		WithFile("README.md", "# Test").
		WithFile("src/main.go", "package main").
		WithCommit("Second commit").
		WithBranch("develop").
		Build()

	// Check directory exists.
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("repo directory should exist")
	}

	// Check .git exists.
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Error(".git directory should exist")
	}

	// Check files exist.
	readme := filepath.Join(dir, "README.md")
	if _, err := os.Stat(readme); os.IsNotExist(err) {
		t.Error("README.md should exist")
	}

	mainGo := filepath.Join(dir, "src", "main.go")
	if _, err := os.Stat(mainGo); os.IsNotExist(err) {
		t.Error("src/main.go should exist")
	}
}

func TestGitRepoBuilderWithRemote(t *testing.T) {
	dir := NewGitRepoBuilder(t).
		WithFile("README.md", "# Test").
		WithRemote("origin", "https://github.com/test/repo.git").
		Build()

	// Check remote is configured (by checking .git/config).
	configPath := filepath.Join(dir, ".git", "config")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read git config: %v", err)
	}

	if !contains(string(content), "origin") {
		t.Error("remote 'origin' should be configured")
	}
}

func TestCloneOptionsBuilder(t *testing.T) {
	opts := NewCloneOptionsBuilder().
		WithURL("https://github.com/test/repo.git").
		WithPath("/tmp/repo").
		WithBranch("develop").
		WithDepth(1).
		Build()

	if opts["url"] != "https://github.com/test/repo.git" {
		t.Error("url should be set")
	}
	if opts["path"] != "/tmp/repo" {
		t.Error("path should be set")
	}
	if opts["branch"] != "develop" {
		t.Error("branch should be set")
	}
	if opts["depth"] != 1 {
		t.Error("depth should be 1")
	}
}

func TestCloneOptionsBuilderBare(t *testing.T) {
	opts := NewCloneOptionsBuilder().
		WithURL("https://github.com/test/repo.git").
		AsBare().
		Build()

	if opts["bare"] != true {
		t.Error("bare should be true")
	}
}

func TestCloneOptionsBuilderMirror(t *testing.T) {
	opts := NewCloneOptionsBuilder().
		WithURL("https://github.com/test/repo.git").
		AsMirror().
		Build()

	if opts["mirror"] != true {
		t.Error("mirror should be true")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
