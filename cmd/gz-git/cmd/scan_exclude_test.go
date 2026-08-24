// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

func writeScanConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gz-git.yaml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return dir
}

func TestCombineExcludePatternsGroupsAlternations(t *testing.T) {
	// A pattern containing its own top-level alternation must not absorb the
	// pattern that follows it. Without the non-capturing groups, "a|b" and "c"
	// would join as "a|bc", which matches neither "b" nor "c" and silently
	// stops excluding both.
	got := combineExcludePatterns([]string{"a|b", "c"})
	re, err := regexp.Compile(got)
	if err != nil {
		t.Fatalf("combined pattern %q does not compile: %v", got, err)
	}
	for _, name := range []string{"a", "b", "c"} {
		if !re.MatchString(name) {
			t.Errorf("combined pattern %q fails to match %q", got, name)
		}
	}
}

func TestCombineExcludePatternsSingleIsUnwrapped(t *testing.T) {
	// One pattern needs no alternation, and leaving it verbatim keeps the
	// string the user sees in an error message identical to what they wrote.
	if got := combineExcludePatterns([]string{"vendor"}); got != "vendor" {
		t.Errorf("combineExcludePatterns single = %q, want %q", got, "vendor")
	}
}

func TestResolveScanExcludeNoConfigReturnsFlag(t *testing.T) {
	dir := t.TempDir()
	if got := resolveScanExclude(dir, "only-flag"); got != "only-flag" {
		t.Errorf("resolveScanExclude = %q, want %q", got, "only-flag")
	}
	if got := resolveScanExclude(dir, ""); got != "" {
		t.Errorf("resolveScanExclude with no sources = %q, want empty so the scanner skips filtering", got)
	}
}

func TestResolveScanExcludeMergesConfigAndFlag(t *testing.T) {
	dir := writeScanConfig(t, "defaults:\n  scan:\n    exclude:\n      - mirror-repo\n")

	got := resolveScanExclude(dir, "scratch")
	re, err := regexp.Compile(got)
	if err != nil {
		t.Fatalf("resolved pattern %q does not compile: %v", got, err)
	}
	// The flag adds to the configured exclusion instead of replacing it.
	for _, name := range []string{"/repos/mirror-repo", "/repos/scratch"} {
		if !re.MatchString(name) {
			t.Errorf("resolved pattern %q should exclude %q", got, name)
		}
	}
	if re.MatchString("/repos/app") {
		t.Errorf("resolved pattern %q must not exclude unrelated repositories", got)
	}
}

// TestConfigExcludeBeatsIncludeFlag pins the precedence the config key exists
// for: a repository declared off-limits stays out of the scan even when the
// user asks for it by name with --include. The safety of a declared exclusion
// is that it cannot be revoked by forgetting — or by a broad --include.
func TestConfigExcludeBeatsIncludeFlag(t *testing.T) {
	root := t.TempDir()
	names := []string{"app", "mirror-repo"}
	for _, name := range names {
		repoDir := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o750); err != nil {
			t.Fatalf("create fake repo: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".gz-git.yaml"),
		[]byte("defaults:\n  scan:\n    exclude:\n      - mirror-repo\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result, err := repository.NewClient().ScanRepositories(t.Context(), repository.ScanOptions{
		Directory:      root,
		MaxDepth:       1,
		IncludePattern: "mirror-repo", // asks for exactly the excluded one
		ExcludePattern: resolveScanExclude(root, ""),
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(result.Paths) != 0 {
		t.Errorf("--include resurrected a config-excluded repository: %v", result.Paths)
	}
}
