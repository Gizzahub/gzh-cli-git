// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRepoRootTaskPattern_HotfixLoads(t *testing.T) {
	root := t.TempDir()
	writeRepoConfig(t, root, ".gz-git.yaml", "branch:\n  taskPattern: hotfix/*\n")

	got, err := LoadRepoRootTaskPattern(root)
	if err != nil {
		t.Fatalf("LoadRepoRootTaskPattern: %v", err)
	}
	if len(got.Patterns) != 1 || got.Patterns[0] != "hotfix/*" {
		t.Fatalf("patterns = %v, want [hotfix/*]", got.Patterns)
	}
	if got.Source != filepath.Join(root, ".gz-git.yaml") {
		t.Fatalf("source = %q", got.Source)
	}
	if !MatchTaskPattern("hotfix/urgent", "hotfix/*") {
		t.Fatal("hotfix/urgent should match hotfix/*")
	}
}

func TestLoadRepoRootTaskPattern_MasterRejected(t *testing.T) {
	root := t.TempDir()
	writeRepoConfig(t, root, ".gz-git.yaml", "branch:\n  taskPattern: master\n")

	_, err := LoadRepoRootTaskPattern(root)
	if err == nil {
		t.Fatal("expected reject of literal protected name master")
	}
	if !strings.Contains(err.Error(), "master") {
		t.Fatalf("error %q should mention master", err)
	}
}

func TestLoadRepoRootTaskPattern_NonRootIgnoredAndReported(t *testing.T) {
	root := t.TempDir()
	writeRepoConfig(t, root, ".gz-git.yaml", "branch:\n  defaultBranch: develop\n")
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	nestedPath := filepath.Join(nested, ".gz-git.yaml")
	writeRepoConfig(t, nested, ".gz-git.yaml", "branch:\n  taskPattern: feat/*\n")

	got, err := LoadRepoRootTaskPattern(root)
	if err != nil {
		t.Fatalf("LoadRepoRootTaskPattern: %v", err)
	}
	if len(got.Patterns) != 0 {
		t.Fatalf("non-root patterns must not apply, got %v", got.Patterns)
	}
	reported := false
	for _, fact := range got.Facts {
		if strings.Contains(fact, nestedPath) {
			reported = true
		}
	}
	if !reported {
		t.Fatalf("expected ignored path %s in facts %v", nestedPath, got.Facts)
	}
}

func TestLoadRepoRootTaskPattern_MissingFileEmpty(t *testing.T) {
	root := t.TempDir()
	got, err := LoadRepoRootTaskPattern(root)
	if err != nil {
		t.Fatalf("LoadRepoRootTaskPattern: %v", err)
	}
	if len(got.Patterns) != 0 {
		t.Fatalf("missing file must be empty patterns, got %v", got.Patterns)
	}
	found := false
	for _, fact := range got.Facts {
		if fact == factNoDeclaration {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing file must report %q, got %v", factNoDeclaration, got.Facts)
	}
}

func TestLoadRepoRootTaskPattern_IgnoresParentOfRepo(t *testing.T) {
	parent := t.TempDir()
	writeRepoConfig(t, parent, ".gz-git.yaml", "branch:\n  taskPattern: feat/*\n")
	root := filepath.Join(parent, "repo")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}

	got, err := LoadRepoRootTaskPattern(root)
	if err != nil {
		t.Fatalf("LoadRepoRootTaskPattern: %v", err)
	}
	if len(got.Patterns) != 0 {
		t.Fatalf("parent-of-root file must not apply (findConfigUpward trap), got %v", got.Patterns)
	}
}

func TestLoadRepoRootTaskPattern_DevelopRejectedReleaseAllowed(t *testing.T) {
	root := t.TempDir()
	writeRepoConfig(t, root, ".gz-git.yaml", "branch:\n  taskPattern:\n    - release/*\n    - develop\n")
	if _, err := LoadRepoRootTaskPattern(root); err == nil {
		t.Fatal("expected reject when develop is declared")
	}

	writeRepoConfig(t, root, ".gz-git.yaml", "branch:\n  taskPattern: [dev/*/*/* , release/*]\n")
	got, err := LoadRepoRootTaskPattern(root)
	if err != nil {
		t.Fatalf("release/* must load: %v", err)
	}
	if !containsStr(got.Patterns, "release/*") || !containsStr(got.Patterns, "dev/*/*/*") {
		t.Fatalf("patterns = %v", got.Patterns)
	}
}

func writeRepoConfig(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func containsStr(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
