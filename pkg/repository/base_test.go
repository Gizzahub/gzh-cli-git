// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)

// runGit runs a git command in dir and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:noctx // test helper
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s in %s failed: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
}

// gitOut runs a git command in dir and returns trimmed stdout.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:noctx // test helper
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s in %s failed: %v", strings.Join(args, " "), dir, err)
	}
	return strings.TrimSpace(string(out))
}

// commitFile writes and commits a file on the currently checked-out branch.
func commitFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("content"), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	runGit(t, dir, "add", name)
	runGit(t, dir, "commit", "-m", "add "+name)
}

// mergedFixture builds a repo on master holding three branches: one merged
// into master, one with unique commits, and one that is merged but happens to
// be checked out.
func mergedFixture(t *testing.T) string {
	t.Helper()
	dir := testutil.TempGitRepoWithCommit(t)
	runGit(t, dir, "branch", "-M", "master")

	runGit(t, dir, "checkout", "-b", "feat/landed")
	commitFile(t, dir, "landed.txt")

	runGit(t, dir, "checkout", "-b", "feat/also-landed", "master")
	commitFile(t, dir, "also.txt")

	runGit(t, dir, "checkout", "-b", "feat/open", "master")
	commitFile(t, dir, "open.txt")

	runGit(t, dir, "checkout", "master")
	runGit(t, dir, "merge", "--no-ff", "--no-edit", "feat/landed")
	runGit(t, dir, "merge", "--no-ff", "--no-edit", "feat/also-landed")

	return dir
}

// TestMergedBranches_AncestryDecides verifies that only branches whose tips are
// reachable from the base are reported — the merged ones — and that the base
// itself is never included.
func TestMergedBranches_AncestryDecides(t *testing.T) {
	dir := mergedFixture(t)

	client := NewClient()
	repo, err := client.Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	got, err := client.MergedBranches(context.Background(), repo, "master")
	if err != nil {
		t.Fatalf("MergedBranches: %v", err)
	}

	want := map[string]bool{"feat/landed": true, "feat/also-landed": true}
	if len(got) != len(want) {
		t.Fatalf("MergedBranches = %v, want %v", got, want)
	}
	for _, name := range got {
		if !want[name] {
			t.Errorf("MergedBranches returned %q, which still has unique commits", name)
		}
	}
}

// TestMergedBranches_ExcludesCurrentBranch pins the exclusion that keeps the
// remediation runnable: git refuses to delete the branch you are standing on,
// so reporting it would emit a command that cannot succeed.
func TestMergedBranches_ExcludesCurrentBranch(t *testing.T) {
	dir := mergedFixture(t)
	runGit(t, dir, "checkout", "feat/landed")

	client := NewClient()
	repo, err := client.Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	got, err := client.MergedBranches(context.Background(), repo, "master")
	if err != nil {
		t.Fatalf("MergedBranches: %v", err)
	}
	for _, name := range got {
		if name == "feat/landed" {
			t.Errorf("MergedBranches included the checked-out branch %q", name)
		}
	}
	if len(got) != 1 || got[0] != "feat/also-landed" {
		t.Errorf("MergedBranches = %v, want [feat/also-landed]", got)
	}
}

// TestMergedBranches_NoBase verifies that an unresolved base yields nothing
// rather than an error: "no base" is a reportable state, not a failure.
func TestMergedBranches_NoBase(t *testing.T) {
	dir := mergedFixture(t)

	client := NewClient()
	repo, err := client.Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	got, err := client.MergedBranches(context.Background(), repo, "")
	if err != nil {
		t.Errorf("MergedBranches with empty base returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("MergedBranches = %v, want empty", got)
	}

	if _, err := client.MergedBranches(context.Background(), nil, "master"); err == nil {
		t.Error("MergedBranches(nil) returned no error")
	}
}

// TestResolveBase_ConfigCandidateWins verifies that the first existing
// candidate in the configured order wins, and its index is reflected in Source.
func TestResolveBase_ConfigCandidateWins(t *testing.T) {
	dir := testutil.TempGitRepoWithCommit(t)
	runGit(t, dir, "checkout", "-b", "master")
	runGit(t, dir, "checkout", "-b", "develop") // both master and develop exist

	client := NewClient()
	repo, err := client.Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// "develop" is listed first and exists → wins at index 0.
	got, err := client.ResolveBase(context.Background(), repo, []string{"develop", "master"})
	if err != nil {
		t.Fatalf("ResolveBase: %v", err)
	}
	if got.Name != "develop" {
		t.Errorf("Name = %q, want develop", got.Name)
	}
	if got.Source != "config.defaultBranch[0]" {
		t.Errorf("Source = %q, want config.defaultBranch[0]", got.Source)
	}

	// Reversed order: "master" now wins at index 0.
	got, _ = client.ResolveBase(context.Background(), repo, []string{"master", "develop"})
	if got.Name != "master" || got.Source != "config.defaultBranch[0]" {
		t.Errorf("reversed: Name=%q Source=%q, want master / config.defaultBranch[0]", got.Name, got.Source)
	}
}

// TestResolveBase_HeuristicFallback verifies that with no (or non-matching)
// candidates, the conventional list is used and Source is "heuristic".
//
// The repo is normalized to a single "master" branch so the assertion does not
// depend on the host's init.defaultBranch: "main" precedes "master" in the
// heuristic order, and a machine defaulting to main would otherwise make this
// test pass or fail for reasons unrelated to the fallback being exercised.
func TestResolveBase_HeuristicFallback(t *testing.T) {
	dir := testutil.TempGitRepoWithCommit(t)
	runGit(t, dir, "branch", "-M", "master")

	client := NewClient()
	repo, err := client.Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Candidates that do not exist here must not win; the fallback list must.
	got, err := client.ResolveBase(context.Background(), repo, []string{"nonexistent-a", "nonexistent-b"})
	if err != nil {
		t.Fatalf("ResolveBase: %v", err)
	}
	if got.Name != "master" {
		t.Errorf("Name = %q, want master", got.Name)
	}
	if got.Source != "heuristic" {
		t.Errorf("Source = %q, want heuristic", got.Source)
	}
}

// TestResolveBase_Divergence verifies ahead/behind counts against the base.
func TestResolveBase_Divergence(t *testing.T) {
	dir := testutil.TempGitRepoWithCommit(t)
	runGit(t, dir, "checkout", "-b", "master")
	runGit(t, dir, "checkout", "-b", "feat")

	// One commit on master that feat lacks → feat behind by 1.
	runGit(t, dir, "checkout", "master")
	commitFile(t, dir, "base.txt")
	// One commit on feat that master lacks → feat ahead by 1.
	runGit(t, dir, "checkout", "feat")
	commitFile(t, dir, "feat.txt")

	client := NewClient()
	repo, err := client.Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	got, err := client.ResolveBase(context.Background(), repo, []string{"master"})
	if err != nil {
		t.Fatalf("ResolveBase: %v", err)
	}
	if got.Name != "master" {
		t.Fatalf("Name = %q, want master", got.Name)
	}
	if got.Ahead != 1 {
		t.Errorf("Ahead = %d, want 1", got.Ahead)
	}
	if got.Behind != 1 {
		t.Errorf("Behind = %d, want 1", got.Behind)
	}
	if got.SHA == "" {
		t.Error("SHA is empty, want the base tip short hash")
	}
}

// TestResolveBase_None verifies a repo with no recognized base reports
// Source "none" and an empty Name rather than guessing.
func TestResolveBase_None(t *testing.T) {
	dir := testutil.TempGitRepoWithCommit(t)
	defaultBranch := gitOut(t, dir, "branch", "--show-current")
	runGit(t, dir, "checkout", "-b", "personal-task")
	// Remove the original default so no main/master/develop remains.
	runGit(t, dir, "branch", "-D", defaultBranch)

	client := NewClient()
	repo, err := client.Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	got, err := client.ResolveBase(context.Background(), repo, nil)
	if err != nil {
		t.Fatalf("ResolveBase: %v", err)
	}
	if got.Name != "" {
		t.Errorf("Name = %q, want empty", got.Name)
	}
	if got.Source != "none" {
		t.Errorf("Source = %q, want none", got.Source)
	}
	if got.Ahead != 0 || got.Behind != 0 {
		t.Errorf("divergence = ahead %d behind %d, want 0/0", got.Ahead, got.Behind)
	}
}

// TestResolveBase_NilRepo verifies the guard.
func TestResolveBase_NilRepo(t *testing.T) {
	client := NewClient()
	_, err := client.ResolveBase(context.Background(), nil, []string{"master"})
	if err == nil {
		t.Error("ResolveBase should error for nil repo")
	}
}
