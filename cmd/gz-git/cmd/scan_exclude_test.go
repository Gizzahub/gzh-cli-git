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

// TestCombineExcludePatternsSkipsEmpty pins the failure an empty entry would
// otherwise cause. `(?:)` is a valid regex matching the empty string, so a
// single empty branch in the alternation matches every repository — turning one
// declared exclusion into a scan that silently finds nothing.
func TestCombineExcludePatternsSkipsEmpty(t *testing.T) {
	got := combineExcludePatterns([]string{"vendor", ""})
	if got != "vendor" {
		t.Fatalf("combineExcludePatterns = %q, want %q", got, "vendor")
	}
	re := regexp.MustCompile(got)
	if re.MatchString("totally-unrelated-repo") {
		t.Errorf("pattern %q excludes every repository", got)
	}
	if !re.MatchString("vendor") {
		t.Errorf("pattern %q lost the exclusion that was actually written", got)
	}
}

func TestCombineExcludePatternsAllEmptyIsNoFilter(t *testing.T) {
	// "" is what filterRepositories reads as "no exclude filter", which is the
	// right outcome when no pattern was actually written. The dangerous shape is
	// the one above — an empty branch joined to a real one.
	if got := combineExcludePatterns([]string{"", ""}); got != "" {
		t.Errorf("combineExcludePatterns all-empty = %q, want empty", got)
	}
}

// TestResolveScanExcludeIgnoresEmptyConfigEntry runs the slip end to end: a
// trailing blank list item must not convert a one-repository exclusion into a
// scan that returns nothing.
func TestResolveScanExcludeIgnoresEmptyConfigEntry(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"app", "mirror-repo"} {
		if err := os.MkdirAll(filepath.Join(root, name, ".git"), 0o750); err != nil {
			t.Fatalf("create fake repo: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".gz-git.yaml"),
		[]byte("defaults:\n  scan:\n    exclude:\n      - mirror-repo\n      - \"\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result, err := repository.NewClient().ScanRepositories(t.Context(), repository.ScanOptions{
		Directory:      root,
		MaxDepth:       1,
		ExcludePattern: resolveScanExclude(root, ""),
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(result.Paths) != 1 || filepath.Base(result.Paths[0]) != "app" {
		t.Errorf("empty exclude entry changed the scan: got %v, want only app", result.Paths)
	}
}

func TestResolveScanExcludeOnlyEmptyEntryFallsBackToFlag(t *testing.T) {
	// The opposite half of the same slip: alone, an empty entry used to be
	// returned verbatim and then read downstream as "no filter". Dropping it
	// reaches the same outcome deliberately, and says so on stderr.
	dir := writeScanConfig(t, "defaults:\n  scan:\n    exclude:\n      - \"\"\n")
	if got := resolveScanExclude(dir, "scratch"); got != "scratch" {
		t.Errorf("resolveScanExclude = %q, want the flag value %q", got, "scratch")
	}
	if got := resolveScanExclude(dir, ""); got != "" {
		t.Errorf("resolveScanExclude = %q, want empty so the scanner skips filtering", got)
	}
}
