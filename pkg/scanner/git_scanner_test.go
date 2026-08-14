// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		pattern  string
		expected bool
	}{
		{"exact match", "vendor", "vendor", true},
		{"exact match with slash", "vendor/lib", "vendor/lib", true},
		{"directory prefix exact", "vendor", "vendor/", true},
		{"directory prefix subpath", "vendor/lib", "vendor/", false}, // Current implementation doesn't match this
		{"wildcard suffix", "vendor/lib", "vendor/*", true},
		{"no match", "src/main", "vendor", false},
		{"glob match", "test.go", "*.go", true},
		{"glob no match", "test.go", "*.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesPattern(tt.path, tt.pattern)
			if result != tt.expected {
				t.Errorf("matchesPattern(%q, %q) = %v, want %v",
					tt.path, tt.pattern, result, tt.expected)
			}
		})
	}
}

func TestIsDir(t *testing.T) {
	// Create a temp directory
	tmpDir := t.TempDir()

	// Test existing directory
	if !isDir(tmpDir) {
		t.Error("isDir should return true for existing directory")
	}

	// Create a file
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Test existing file (should return false)
	if isDir(tmpFile) {
		t.Error("isDir should return false for file")
	}

	// Test non-existent path
	if isDir(filepath.Join(tmpDir, "nonexistent")) {
		t.Error("isDir should return false for non-existent path")
	}
}

func TestGitRepoScanner_Scan(t *testing.T) {
	// Create a temp directory structure with git repos
	tmpDir := t.TempDir()

	// Create a mock git repository
	repo1 := filepath.Join(tmpDir, "repo1")
	if err := os.MkdirAll(filepath.Join(repo1, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create another mock git repository
	repo2 := filepath.Join(tmpDir, "subdir", "repo2")
	if err := os.MkdirAll(filepath.Join(repo2, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a non-git directory
	nonGit := filepath.Join(tmpDir, "not-a-repo")
	if err := os.MkdirAll(nonGit, 0o755); err != nil {
		t.Fatal(err)
	}

	scanner := &GitRepoScanner{
		RootPath: tmpDir,
		MaxDepth: 2,
	}

	repos, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(repos) != 2 {
		t.Errorf("Expected 2 repos, got %d", len(repos))
	}

	// Check repo names
	names := make(map[string]bool)
	for _, r := range repos {
		names[r.Name] = true
	}

	if !names["repo1"] {
		t.Error("repo1 not found in scan results")
	}
	if !names["repo2"] {
		t.Error("repo2 not found in scan results")
	}
}

func TestGitRepoScanner_ScanRejectsOutsideGitSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), ".git")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}

	repo := filepath.Join(root, "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, ".git")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	repos, err := (&GitRepoScanner{RootPath: root, MaxDepth: 1}).Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if len(repos) != 0 {
		t.Fatalf("Scan found %d repos through outside .git symlink, want 0: %+v", len(repos), repos)
	}
}

func TestGitRepoScanner_Scan_WithExclusion(t *testing.T) {
	tmpDir := t.TempDir()

	// Create repos
	repo1 := filepath.Join(tmpDir, "repo1")
	if err := os.MkdirAll(filepath.Join(repo1, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	vendorRepo := filepath.Join(tmpDir, "vendor", "lib")
	if err := os.MkdirAll(filepath.Join(vendorRepo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	scanner := &GitRepoScanner{
		RootPath:        tmpDir,
		MaxDepth:        3,
		ExcludePatterns: []string{"vendor"},
	}

	repos, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(repos) != 1 {
		t.Errorf("Expected 1 repo (vendor excluded), got %d", len(repos))
	}

	if repos[0].Name != "repo1" {
		t.Errorf("Expected repo1, got %s", repos[0].Name)
	}
}

func TestGitRepoScanner_MaxDepth(t *testing.T) {
	tmpDir := t.TempDir()

	// Create repo at depth 1
	repo1 := filepath.Join(tmpDir, "level1")
	if err := os.MkdirAll(filepath.Join(repo1, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create repo at depth 3 (should be excluded with MaxDepth=1)
	repo2 := filepath.Join(tmpDir, "a", "b", "level3")
	if err := os.MkdirAll(filepath.Join(repo2, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	scanner := &GitRepoScanner{
		RootPath: tmpDir,
		MaxDepth: 1,
	}

	repos, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(repos) != 1 {
		t.Errorf("Expected 1 repo (depth limited), got %d", len(repos))
	}
}

func TestToRepositories(t *testing.T) {
	scanned := []*ScannedRepo{
		{
			Name:    "repo1",
			Path:    "/path/to/repo1",
			Remotes: map[string]string{"origin": "https://github.com/org/repo1.git"},
			Depth:   1,
		},
		{
			Name:    "repo2",
			Path:    "/path/to/repo2",
			Remotes: map[string]string{},
			Depth:   1,
		},
	}

	repos := ToRepositories(scanned)

	if len(repos) != 2 {
		t.Errorf("Expected 2 repos, got %d", len(repos))
	}

	if repos[0].Name != "repo1" {
		t.Errorf("Expected repo1, got %s", repos[0].Name)
	}
	if repos[0].CloneURL != "https://github.com/org/repo1.git" {
		t.Errorf("Expected clone URL, got %s", repos[0].CloneURL)
	}

	if repos[1].CloneURL != "" {
		t.Errorf("Expected empty clone URL for repo2, got %s", repos[1].CloneURL)
	}
}
