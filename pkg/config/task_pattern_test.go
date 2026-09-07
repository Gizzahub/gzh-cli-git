// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

func TestLoadRepoRootTaskPattern_HotfixLoads(t *testing.T) {
	root := t.TempDir()
	writeRepoConfig(t, root, "branch:\n  taskPattern: hotfix/*\n")

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

func TestMatchTaskPattern_DevTripleStar(t *testing.T) {
	// DECISION-004: devenv declares dev/*/*/*. Trailing-* prefix compare
	// would look for the literal "dev/*/*/" and miss every real branch.
	if !MatchTaskPattern("dev/a/b/c", "dev/*/*/*") {
		t.Fatal("DECISION-004: dev/*/*/* must match dev/a/b/c")
	}
	if !MatchTaskPattern("dev/grok/feat/integrate-surface", "dev/*/*/*") {
		t.Fatal("dev/*/*/* must match a real task branch")
	}
	if MatchTaskPattern("hotfix/foo", "dev/*/*/*") {
		t.Fatal("dev/*/*/* must not match hotfix/foo")
	}
	if MatchTaskPattern("dev/a/b/c", "*") {
		t.Fatal("pattern * must not match everything")
	}
}

// TestMatchTaskPattern_ParityWithRepository pins config.MatchTaskPattern and
// config.MatchesAnyTaskPattern to behave identically to the pkg/repository
// functions they now delegate to (see task_pattern.go's doc comment on why
// ownership moved there). A future edit that reintroduces a forked copy in
// this package instead of delegating would still pass every other test in
// this file — those call config.MatchTaskPattern directly — so this test
// exists to compare the two call sites side by side instead of trusting that
// they stayed in sync.
func TestMatchTaskPattern_ParityWithRepository(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
	}{
		{"dev/a/b/c", "dev/*/*/*"},
		{"dev/a/b/c/d", "dev/*/*/*"},
		{"hotfix/foo", "dev/*/*/*"},
		{"dev/a/b/c", "*"},
		{"hotfix/urgent", "hotfix/*"},
		{"release/1.0.0", "release/*"},
	}
	for _, tc := range cases {
		got := MatchTaskPattern(tc.name, tc.pattern)
		want := repository.MatchTaskPattern(tc.name, tc.pattern)
		if got != want {
			t.Errorf("MatchTaskPattern(%q, %q) = %v, want parity with repository.MatchTaskPattern = %v", tc.name, tc.pattern, got, want)
		}
	}

	patterns := []string{"hotfix/*", "dev/*/*/*"}
	for _, name := range []string{"hotfix/urgent", "dev/a/b/c/d", "release/1.0.0"} {
		got := MatchesAnyTaskPattern(name, patterns)
		want := repository.MatchesAnyTaskPattern(name, patterns)
		if got != want {
			t.Errorf("MatchesAnyTaskPattern(%q, %v) = %v, want parity with repository.MatchesAnyTaskPattern = %v", name, patterns, got, want)
		}
	}
}

func TestLoadRepoRootTaskPattern_StarRejected(t *testing.T) {
	root := t.TempDir()
	writeRepoConfig(t, root, "branch:\n  taskPattern: '*'\n")
	if _, err := LoadRepoRootTaskPattern(root); err == nil {
		t.Fatal("expected reject of match-everything taskPattern")
	}
}

func TestLoadRepoRootTaskPattern_MasterRejected(t *testing.T) {
	root := t.TempDir()
	writeRepoConfig(t, root, "branch:\n  taskPattern: master\n")

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
	writeRepoConfig(t, root, "branch:\n  defaultBranch: develop\n")
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	nestedPath := filepath.Join(nested, ".gz-git.yaml")
	writeRepoConfig(t, nested, "branch:\n  taskPattern: feat/*\n")

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

func TestLoadRepoRootTaskPattern_RejectsOutsideConfigSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), ".gz-git.yaml")
	writeRepoConfig(t, filepath.Dir(outside), "branch:\n  taskPattern: master\n")
	if err := os.Symlink(outside, filepath.Join(root, ".gz-git.yaml")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got, err := LoadRepoRootTaskPattern(root)
	if err == nil {
		t.Fatal("LoadRepoRootTaskPattern succeeded through outside config symlink")
	}
	if len(got.Patterns) != 0 {
		t.Fatalf("outside taskPattern was loaded: %v", got.Patterns)
	}
}

func TestLoadRepoRootTaskPattern_IgnoresParentOfRepo(t *testing.T) {
	parent := t.TempDir()
	writeRepoConfig(t, parent, "branch:\n  taskPattern: feat/*\n")
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
	writeRepoConfig(t, root, "branch:\n  taskPattern:\n    - release/*\n    - develop\n")
	if _, err := LoadRepoRootTaskPattern(root); err == nil {
		t.Fatal("expected reject when develop is declared")
	}

	writeRepoConfig(t, root, "branch:\n  taskPattern: [dev/*/*/* , release/*]\n")
	got, err := LoadRepoRootTaskPattern(root)
	if err != nil {
		t.Fatalf("release/* must load: %v", err)
	}
	if !containsStr(got.Patterns, "release/*") || !containsStr(got.Patterns, "dev/*/*/*") {
		t.Fatalf("patterns = %v", got.Patterns)
	}
}

func writeRepoConfig(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".gz-git.yaml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
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
