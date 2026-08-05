// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package branch

import (
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// A "gone" branch is a *local* branch whose upstream no longer resolves — what
// git marks `[gone]` in `%(upstream:track)`. Reproducing one needs a real
// upstream, so these tests build an origin repository, clone it, and delete a
// branch from the origin. A local path as the remote keeps the fetch offline.

// goneFixture returns a clone whose local branch "gone-upstream" tracks a branch
// that no longer exists on origin, alongside "live-upstream" which still does.
func goneFixture(t *testing.T) (clonePath, originPath string) {
	t.Helper()

	originPath = testutil.TempGitRepoWithCommit(t)

	gitCommit(t, originPath, "branch", "gone-upstream")
	gitCommit(t, originPath, "branch", "live-upstream")

	clonePath = filepath.Join(t.TempDir(), "clone")
	gitCommit(t, t.TempDir(), "clone", originPath, clonePath)

	// Create the local branches without moving HEAD: Analyze skips the checked-out
	// branch, and --track is what gives them an upstream to lose.
	gitCommit(t, clonePath, "branch", "--track", "gone-upstream", "origin/gone-upstream")
	gitCommit(t, clonePath, "branch", "--track", "live-upstream", "origin/live-upstream")

	// The upstream disappears. The clone does not know yet — findGoneBranches
	// prunes before it reads, which is the behavior under test.
	gitCommit(t, originPath, "branch", "-D", "gone-upstream")

	return clonePath, originPath
}

// branchNames lists the local branches of a repository, sorted.
func branchNames(t *testing.T, dir string) []string {
	t.Helper()

	cmd := exec.Command("git", "for-each-ref", "--format=%(refname:short)", "refs/heads/") //nolint:noctx // test helper
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("for-each-ref in %s: %v\n%s", dir, err, out)
	}

	names := strings.Fields(string(out))
	slices.Sort(names)

	return names
}

// TestCleanupService_AnalyzeFindsBranchWithGoneUpstream is the regression test
// for --gone having done nothing.
//
// The old isBranchOrphaned asked whether a *remote-tracking* branch's remote was
// still registered, and required a "remotes/" name prefix that List never
// produces — so the check returned false for every branch it ever saw, and
// report.Orphaned was always empty.
func TestCleanupService_AnalyzeFindsBranchWithGoneUpstream(t *testing.T) {
	clonePath, _ := goneFixture(t)
	repo := &repository.Repository{Path: clonePath}

	report, err := NewCleanupService().Analyze(context.Background(), repo, AnalyzeOptions{IncludeGone: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	got := make([]string, 0, len(report.Orphaned))
	for _, b := range report.Orphaned {
		got = append(got, b.Name)
	}

	if !slices.Contains(got, "gone-upstream") {
		t.Errorf("Orphaned = %v, want it to contain gone-upstream", got)
	}

	// The other half of the assertion: a branch whose upstream is intact must not
	// be swept up. An over-broad rule here deletes work that is still tracked.
	if slices.Contains(got, "live-upstream") {
		t.Errorf("Orphaned = %v, want live-upstream left alone — its upstream still exists", got)
	}
}

// TestCleanupService_AnalyzeSkipsGoneWhenNotRequested pins the flag to the
// behavior. Without it the previous test would pass on a service that reported
// every branch as orphaned.
func TestCleanupService_AnalyzeSkipsGoneWhenNotRequested(t *testing.T) {
	clonePath, _ := goneFixture(t)
	repo := &repository.Repository{Path: clonePath}

	report, err := NewCleanupService().Analyze(context.Background(), repo, AnalyzeOptions{IncludeGone: false})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if len(report.Orphaned) != 0 {
		t.Errorf("Orphaned = %+v, want empty when IncludeGone is false", report.Orphaned)
	}
}

// TestCleanupService_ExecuteGoneLeavesOriginUntouched fixes the decision that
// --gone removes the local ref and nothing on the server.
//
// The two are not comparably reversible: a local branch whose upstream is gone
// can be restored from the reflog, while `git push --delete` removes a branch
// from a repository other people share. Execute passes
// `Remote: opts.Remote && branch.IsRemote`, and a gone branch is local, so the
// push cannot fire — this asserts that even with --remote asked for.
func TestCleanupService_ExecuteGoneLeavesOriginUntouched(t *testing.T) {
	clonePath, originPath := goneFixture(t)
	repo := &repository.Repository{Path: clonePath}
	ctx := context.Background()

	svc := NewCleanupService()

	report, err := svc.Analyze(ctx, repo, AnalyzeOptions{IncludeGone: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	originBefore := branchNames(t, originPath)

	result, err := svc.Execute(ctx, repo, report, ExecuteOptions{
		Force:   true,
		Remote:  true, // asked for, and still must not reach origin
		Confirm: true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !slices.Contains(result.Deleted, "gone-upstream") {
		t.Errorf("Deleted = %v, want it to contain gone-upstream", result.Deleted)
	}

	// A push --delete would have targeted a branch origin no longer has and failed,
	// so an empty Failed is itself evidence that no push was attempted.
	if len(result.Failed) != 0 {
		t.Errorf("Failed = %+v, want none", result.Failed)
	}

	if got := branchNames(t, clonePath); slices.Contains(got, "gone-upstream") {
		t.Errorf("clone branches = %v, want gone-upstream removed", got)
	}

	if got := branchNames(t, originPath); !slices.Equal(got, originBefore) {
		t.Errorf("origin branches = %v, want unchanged %v", got, originBefore)
	}
}
