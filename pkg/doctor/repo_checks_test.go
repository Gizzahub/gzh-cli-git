// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package doctor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
)

// --- parseAheadBehind ---

func TestParseAheadBehind(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantAhead  int
		wantBehind int
	}{
		{"normal", "5\t3", 5, 3},
		{"zeros", "0\t0", 0, 0},
		{"large", "100\t200", 100, 200},
		{"spaces", "  5\t3  ", 5, 3},
		{"empty", "", 0, 0},
		{"single", "5", 0, 0},
		{"three parts", "5\t3\t1", 0, 0},
		{"non-numeric", "abc\tdef", 0, 0},
		{"mixed", "5\tabc", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ahead, behind := parseAheadBehind(tt.input)
			if ahead != tt.wantAhead || behind != tt.wantBehind {
				t.Errorf("parseAheadBehind(%q) = (%d, %d), want (%d, %d)",
					tt.input, ahead, behind, tt.wantAhead, tt.wantBehind)
			}
		})
	}
}

// --- scanGitRepos ---

func TestScanGitRepos(t *testing.T) {
	root := t.TempDir()

	// Create repo at depth 0
	initGitRepo(t, root)

	repos := scanGitRepos(root, 1)
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
}

func TestScanGitRepos_Depth1(t *testing.T) {
	root := t.TempDir()

	// Create repos at depth 1
	for _, name := range []string{"repo-a", "repo-b"} {
		dir := filepath.Join(root, name)
		os.MkdirAll(dir, 0o755)
		initGitRepo(t, dir)
	}

	repos := scanGitRepos(root, 1)
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
}

func TestScanGitRepos_SkipsDotDirs(t *testing.T) {
	root := t.TempDir()

	// Create a hidden directory with a git repo inside
	hiddenDir := filepath.Join(root, ".hidden-repo")
	os.MkdirAll(hiddenDir, 0o755)
	initGitRepo(t, hiddenDir)

	// Create a normal repo
	normalDir := filepath.Join(root, "normal-repo")
	os.MkdirAll(normalDir, 0o755)
	initGitRepo(t, normalDir)

	repos := scanGitRepos(root, 1)
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo (hidden skipped), got %d", len(repos))
	}
}

func TestScanGitRepos_DepthZero(t *testing.T) {
	root := t.TempDir()

	// Create a sub-dir repo (should NOT be found at depth 0)
	subDir := filepath.Join(root, "child")
	os.MkdirAll(subDir, 0o755)
	initGitRepo(t, subDir)

	repos := scanGitRepos(root, 0)
	if len(repos) != 0 {
		t.Fatalf("expected 0 repos at depth 0 (root is not a repo), got %d", len(repos))
	}
}

func TestScanGitRepos_NegativeDepthDefaultsTo1(t *testing.T) {
	root := t.TempDir()
	subDir := filepath.Join(root, "repo")
	os.MkdirAll(subDir, 0o755)
	initGitRepo(t, subDir)

	repos := scanGitRepos(root, -5)
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo (negative depth → 1), got %d", len(repos))
	}
}

func TestScanGitRepos_NoNestedRecursion(t *testing.T) {
	root := t.TempDir()

	// Create a repo with a nested repo inside
	outerDir := filepath.Join(root, "outer")
	os.MkdirAll(outerDir, 0o755)
	initGitRepo(t, outerDir)

	innerDir := filepath.Join(outerDir, "inner")
	os.MkdirAll(innerDir, 0o755)
	initGitRepo(t, innerDir)

	repos := scanGitRepos(root, 3)
	// Should only find outer, not recurse into nested git repos
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo (no nested recursion), got %d", len(repos))
	}
}

// --- checkIncompleteOps ---

func TestCheckIncompleteOps_Clean(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	results := checkIncompleteOps(dir, "test-repo")
	if len(results) != 0 {
		t.Fatalf("expected 0 results for clean repo, got %d", len(results))
	}
}

func TestCheckIncompleteOps_MergeInProgress(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Simulate MERGE_HEAD
	mergeHead := filepath.Join(dir, ".git", "MERGE_HEAD")
	os.WriteFile(mergeHead, []byte("abc123"), 0o644)

	results := checkIncompleteOps(dir, "test-repo")
	if len(results) != 1 {
		t.Fatalf("expected 1 result for merge-in-progress, got %d", len(results))
	}
	if results[0].Status != StatusError {
		t.Errorf("expected error status, got %s", results[0].Status)
	}
}

func TestCheckIncompleteOps_RebaseInProgress(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Simulate rebase-merge directory
	rebaseDir := filepath.Join(dir, ".git", "rebase-merge")
	os.MkdirAll(rebaseDir, 0o755)

	results := checkIncompleteOps(dir, "test-repo")
	if len(results) != 1 {
		t.Fatalf("expected 1 result for rebase-in-progress, got %d", len(results))
	}
	if results[0].Status != StatusError {
		t.Errorf("expected error status, got %s", results[0].Status)
	}
}

func TestCheckIncompleteOps_BothMergeAndRebase(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	os.WriteFile(filepath.Join(dir, ".git", "MERGE_HEAD"), []byte("abc"), 0o644)
	os.MkdirAll(filepath.Join(dir, ".git", "rebase-apply"), 0o755)

	results := checkIncompleteOps(dir, "test-repo")
	if len(results) != 2 {
		t.Fatalf("expected 2 results (merge + rebase), got %d", len(results))
	}
}

// --- checkNoRemote ---

func TestCheckNoRemote_NoRemote(t *testing.T) {
	dir := t.TempDir()
	initGitRepoWithCommit(t, dir)

	ctx := context.Background()
	executor := gitcmd.NewExecutor(gitcmd.WithTimeout(5 * time.Second))

	results := checkNoRemote(ctx, executor, dir, "test-repo")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != StatusError {
		t.Errorf("expected error status, got %s", results[0].Status)
	}
}

func TestCheckNoRemote_WithRemote(t *testing.T) {
	dir := t.TempDir()
	initGitRepoWithCommit(t, dir)
	runGit(t, dir, "remote", "add", "origin", "https://example.com/repo.git")

	ctx := context.Background()
	executor := gitcmd.NewExecutor(gitcmd.WithTimeout(5 * time.Second))

	results := checkNoRemote(ctx, executor, dir, "test-repo")
	if len(results) != 0 {
		t.Fatalf("expected 0 results for repo with remote, got %d", len(results))
	}
}

// --- checkDetachedHead ---

func TestCheckDetachedHead_Normal(t *testing.T) {
	dir := t.TempDir()
	initGitRepoWithCommit(t, dir)

	ctx := context.Background()
	executor := gitcmd.NewExecutor(gitcmd.WithTimeout(5 * time.Second))

	results := checkDetachedHead(ctx, executor, dir, "test-repo")
	if len(results) != 0 {
		t.Fatalf("expected 0 results for normal branch, got %d", len(results))
	}
}

func TestCheckDetachedHead_Detached(t *testing.T) {
	dir := t.TempDir()
	initGitRepoWithCommit(t, dir)

	// Detach HEAD
	runGit(t, dir, "checkout", "--detach", "HEAD")

	ctx := context.Background()
	executor := gitcmd.NewExecutor(gitcmd.WithTimeout(5 * time.Second))

	results := checkDetachedHead(ctx, executor, dir, "test-repo")
	if len(results) != 1 {
		t.Fatalf("expected 1 result for detached HEAD, got %d", len(results))
	}
	if results[0].Status != StatusWarning {
		t.Errorf("expected warning status, got %s", results[0].Status)
	}
}

// --- checkConflicts ---

func TestCheckConflicts_Clean(t *testing.T) {
	dir := t.TempDir()
	initGitRepoWithCommit(t, dir)

	ctx := context.Background()
	executor := gitcmd.NewExecutor(gitcmd.WithTimeout(5 * time.Second))

	results := checkConflicts(ctx, executor, dir, "test-repo")
	if len(results) != 0 {
		t.Fatalf("expected 0 results for clean repo, got %d", len(results))
	}
}

func TestCheckConflicts_Unmerged(t *testing.T) {
	dir := t.TempDir()
	initGitRepoWithCommit(t, dir)

	// Both sides add the same path with different content, producing an AA
	// record — an unmerged code with no 'U' in it.
	runGit(t, dir, "checkout", "-b", "feature")
	writeFile(t, dir, "both.txt", "from feature\n")
	runGit(t, dir, "add", "both.txt")
	runGit(t, dir, "commit", "-m", "add both.txt on feature")

	runGit(t, dir, "checkout", "-")
	writeFile(t, dir, "both.txt", "from base\n")
	runGit(t, dir, "add", "both.txt")
	runGit(t, dir, "commit", "-m", "add both.txt on base")

	// A conflicting merge exits non-zero; that is the point of the fixture.
	mergeCmd := exec.CommandContext(context.Background(), "git", "merge", "feature")
	mergeCmd.Dir = dir
	_, _ = mergeCmd.CombinedOutput()

	ctx := context.Background()
	executor := gitcmd.NewExecutor(gitcmd.WithTimeout(5 * time.Second))

	results := checkConflicts(ctx, executor, dir, "test-repo")
	if len(results) != 1 {
		t.Fatalf("expected 1 result for conflicted repo, got %d: %+v", len(results), results)
	}
	if results[0].Status != StatusError {
		t.Errorf("Status = %v, want %v", results[0].Status, StatusError)
	}
}

// TestCheckConflicts_UnreadableRepo pins the difference between "examined and
// found healthy" and "not examined".
//
// Every check here signals a healthy repository by returning no results, so
// discarding the error from git status filed an unreadable repository under the
// same heading as a clean one — in the one command whose entire job is to tell
// those apart.
func TestCheckConflicts_UnreadableRepo(t *testing.T) {
	dir := t.TempDir() // not a git repository

	ctx := context.Background()
	executor := gitcmd.NewExecutor(gitcmd.WithTimeout(5 * time.Second))

	results := checkConflicts(ctx, executor, dir, "test-repo")
	if len(results) != 1 {
		t.Fatalf("expected 1 result for an unreadable repo, got %d: %+v", len(results), results)
	}
	if results[0].Status == StatusOK {
		t.Errorf("Status = %v, want a non-OK status", results[0].Status)
	}
}

// --- checkDirtyBehind ---

func TestCheckDirtyBehind_UnreadableRepo(t *testing.T) {
	dir := t.TempDir() // not a git repository

	ctx := context.Background()
	executor := gitcmd.NewExecutor(gitcmd.WithTimeout(5 * time.Second))

	results := checkDirtyBehind(ctx, executor, dir, "test-repo")
	if len(results) != 1 {
		t.Fatalf("expected 1 result for an unreadable repo, got %d: %+v", len(results), results)
	}
	if results[0].Status == StatusOK {
		t.Errorf("Status = %v, want a non-OK status", results[0].Status)
	}
}

func TestCheckDirtyBehind_Clean(t *testing.T) {
	dir := t.TempDir()
	initGitRepoWithCommit(t, dir)

	ctx := context.Background()
	executor := gitcmd.NewExecutor(gitcmd.WithTimeout(5 * time.Second))

	results := checkDirtyBehind(ctx, executor, dir, "test-repo")
	if len(results) != 0 {
		t.Fatalf("expected 0 results for clean repo, got %d: %+v", len(results), results)
	}
}

// --- checkStrandedStash ---

func TestCheckStrandedStash_NoStash(t *testing.T) {
	dir := t.TempDir()
	initGitRepoWithCommit(t, dir)

	executor := gitcmd.NewExecutor(gitcmd.WithTimeout(5 * time.Second))

	results := checkStrandedStash(context.Background(), executor, dir, "test-repo")
	if len(results) != 0 {
		t.Fatalf("expected 0 results with no stash, got %d: %+v", len(results), results)
	}
}

// A stash made just now is what the command is for. Doctor should stay quiet
// about it, or it would nag through every normal mid-task run.
func TestCheckStrandedStash_FreshStashIsSilent(t *testing.T) {
	dir := t.TempDir()
	initGitRepoWithCommit(t, dir)
	stashChange(t, dir, "")

	executor := gitcmd.NewExecutor(gitcmd.WithTimeout(5 * time.Second))

	results := checkStrandedStash(context.Background(), executor, dir, "test-repo")
	if len(results) != 0 {
		t.Fatalf("expected 0 results for a stash made today, got %d: %+v", len(results), results)
	}
}

func TestCheckStrandedStash_OldStashWarns(t *testing.T) {
	dir := t.TempDir()
	initGitRepoWithCommit(t, dir)
	stashChange(t, dir, time.Now().Add(-40*24*time.Hour).Format(time.RFC3339))

	executor := gitcmd.NewExecutor(gitcmd.WithTimeout(5 * time.Second))

	results := checkStrandedStash(context.Background(), executor, dir, "test-repo")
	if len(results) != 1 {
		t.Fatalf("expected 1 result for a 40-day-old stash, got %d: %+v", len(results), results)
	}
	if results[0].Status != StatusWarning {
		t.Errorf("status = %s, want %s", results[0].Status, StatusWarning)
	}
	if !strings.Contains(results[0].Message, "1 stash entry") {
		t.Errorf("message = %q, want the entry count", results[0].Message)
	}
	if !strings.Contains(results[0].Message, "days old") {
		t.Errorf("message = %q, want the age of the oldest entry", results[0].Message)
	}
}

// --- checkDevelopMainDistance ---

func TestCheckDevelopMainDistance_NoDevelop(t *testing.T) {
	dir := t.TempDir()
	initGitRepoWithCommit(t, dir)

	ctx := context.Background()
	executor := gitcmd.NewExecutor(gitcmd.WithTimeout(5 * time.Second))

	results := checkDevelopMainDistance(ctx, executor, dir, "test-repo")
	if len(results) != 0 {
		t.Fatalf("expected 0 results when no develop branch, got %d", len(results))
	}
}

// TestCheckDevelopMainDistance_SameCommitWarns pins the distance==0 case: a
// develop branch that duplicates main/master rather than diverging from it
// still deserves a warning, not silence, because it is exactly the "declare a
// canonical branch and retire the other" situation --non-canonical exists for.
//
// The branch is force-renamed to "master" rather than relying on whatever
// init.defaultBranch the environment configures (see TestFindMainBranch_Master's
// comment above), so this test's outcome does not depend on git config.
func TestCheckDevelopMainDistance_SameCommitWarns(t *testing.T) {
	dir := t.TempDir()
	initGitRepoWithCommit(t, dir)
	runGit(t, dir, "branch", "-M", "master")
	runGit(t, dir, "branch", "develop")

	ctx := context.Background()
	executor := gitcmd.NewExecutor(gitcmd.WithTimeout(5 * time.Second))

	results := checkDevelopMainDistance(ctx, executor, dir, "test-repo")
	if len(results) != 1 {
		t.Fatalf("expected 1 result when develop and master are identical, got %d: %+v", len(results), results)
	}
	if results[0].Status != StatusWarning {
		t.Errorf("status = %s, want %s", results[0].Status, StatusWarning)
	}
	if !strings.Contains(results[0].Message, "same commit") {
		t.Errorf("message = %q, want it to say develop and master point at the same commit", results[0].Message)
	}
}

// --- findMainBranch ---

func TestFindMainBranch_Master(t *testing.T) {
	dir := t.TempDir()
	initGitRepoWithCommit(t, dir) // default branch is typically master or main

	ctx := context.Background()
	executor := gitcmd.NewExecutor(gitcmd.WithTimeout(5 * time.Second))

	branch := findMainBranch(ctx, executor, dir)
	if branch != "main" && branch != "master" {
		t.Errorf("expected main or master, got %q", branch)
	}
}

// --- findBaseBranch ---

func TestFindBaseBranch_FallsBackToMain(t *testing.T) {
	dir := t.TempDir()
	initGitRepoWithCommit(t, dir)

	ctx := context.Background()
	executor := gitcmd.NewExecutor(gitcmd.WithTimeout(5 * time.Second))

	branch := findBaseBranch(ctx, executor, dir)
	// Should find main or master (no develop/dev exists)
	if branch != "main" && branch != "master" {
		t.Errorf("expected main or master as fallback, got %q", branch)
	}
}

func TestFindBaseBranch_PrefersDevelop(t *testing.T) {
	dir := t.TempDir()
	initGitRepoWithCommit(t, dir)
	runGit(t, dir, "branch", "develop")

	ctx := context.Background()
	executor := gitcmd.NewExecutor(gitcmd.WithTimeout(5 * time.Second))

	branch := findBaseBranch(ctx, executor, dir)
	if branch != "develop" {
		t.Errorf("expected develop, got %q", branch)
	}
}

// --- branchDistance ---

func TestBranchDistance_SameBranch(t *testing.T) {
	dir := t.TempDir()
	initGitRepoWithCommit(t, dir)

	// Get current branch name
	ctx := context.Background()
	executor := gitcmd.NewExecutor(gitcmd.WithTimeout(5 * time.Second))

	mainBranch := findMainBranch(ctx, executor, dir)
	if mainBranch == "" {
		t.Skip("no main branch found")
	}

	dist := branchDistance(ctx, executor, dir, mainBranch, mainBranch)
	if dist != 0 {
		t.Errorf("expected distance 0 for same branch, got %d", dist)
	}
}

func TestBranchDistance_DivergentBranches(t *testing.T) {
	dir := t.TempDir()
	initGitRepoWithCommit(t, dir)

	// Create develop with extra commits
	runGit(t, dir, "checkout", "-b", "develop")
	writeFile(t, dir, "dev.txt", "develop content")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "develop commit")

	ctx := context.Background()
	executor := gitcmd.NewExecutor(gitcmd.WithTimeout(5 * time.Second))

	mainBranch := findMainBranch(ctx, executor, dir)
	if mainBranch == "" {
		t.Skip("no main branch found")
	}

	dist := branchDistance(ctx, executor, dir, "develop", mainBranch)
	if dist != 1 {
		t.Errorf("expected distance 1, got %d", dist)
	}
}

// --- Test helpers ---

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")
}

func initGitRepoWithCommit(t *testing.T, dir string) {
	t.Helper()
	initGitRepo(t, dir)
	writeFile(t, dir, "README.md", "# Test")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "Initial commit")
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
}

// stashChange dirties the worktree and stashes it. An RFC3339 date back-dates
// the stash commit, which is how `git stash list --format=%ct` reports its age;
// an empty date leaves it at now.
func stashChange(t *testing.T, dir, date string) {
	t.Helper()
	writeFile(t, dir, "README.md", "# Test, edited")

	cmd := exec.CommandContext(context.Background(), "git", "stash", "push", "-m", "wip")
	cmd.Dir = dir
	if date != "" {
		cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE="+date, "GIT_AUTHOR_DATE="+date)
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git stash push failed: %v\n%s", err, output)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}
