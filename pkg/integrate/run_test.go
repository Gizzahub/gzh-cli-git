// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
)

func TestRun_NoPatternSkipsReclaim(t *testing.T) {
	fx := runFixture(t, "")
	report, err := Run(context.Background(), gitcmd.NewExecutor(), RunOptions{
		CheckOptions: CheckOptions{
			RepoPath: fx.Worktree,
			Branch:   "dev/actor/feat/task",
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !report.Integrated {
		t.Fatalf("want integrated:\n%s", FormatRun(report))
	}
	if report.Reclaim.Incomplete() {
		t.Fatalf("no-pattern reclaim must not be incomplete: %+v", report.Reclaim)
	}
	if !strings.Contains(report.Reclaim.Skipped, "no taskPattern") &&
		!strings.Contains(report.Reclaim.Skipped, "no declaration") {
		t.Fatalf("want no-pattern skip, got %+v", report.Reclaim)
	}
	// Task branch must still exist — reclaim did nothing.
	if _, err := os.Stat(fx.Worktree); err != nil {
		t.Fatalf("worktree should remain: %v", err)
	}
	assertRef(t, fx.Clone, "refs/heads/dev/actor/feat/task")
}

func TestRun_ReclaimIncompleteWhenMultipleWorktrees(t *testing.T) {
	fx := runFixture(t, "dev/*")
	extra := filepath.Join(t.TempDir(), "extra")
	runGit(t, fx.Clone, "worktree", "add", "--force", extra, "dev/actor/feat/task")

	report, err := Run(context.Background(), gitcmd.NewExecutor(), RunOptions{
		CheckOptions: CheckOptions{
			RepoPath: fx.Worktree,
			Branch:   "dev/actor/feat/task",
		},
	})
	if err != nil {
		t.Fatalf("Run: %v (integrate itself must succeed)", err)
	}
	if !report.Integrated {
		t.Fatalf("want integrated before reclaim:\n%s", FormatRun(report))
	}
	if !report.Reclaim.Incomplete() {
		t.Fatalf("want incomplete reclaim, got %+v", report.Reclaim)
	}
	if cliutil.ExitReclaimIncomplete != 3 {
		t.Fatalf("ExitReclaimIncomplete = %d, want 3", cliutil.ExitReclaimIncomplete)
	}
}

func TestRun_ReclaimsMatchingPattern(t *testing.T) {
	fx := runFixture(t, "dev/*")
	report, err := Run(context.Background(), gitcmd.NewExecutor(), RunOptions{
		CheckOptions: CheckOptions{
			RepoPath: fx.Worktree,
			Branch:   "dev/actor/feat/task",
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !report.Integrated {
		t.Fatalf("want integrated:\n%s", FormatRun(report))
	}
	if report.Reclaim.Incomplete() {
		t.Fatalf("reclaim incomplete: %+v", report.Reclaim)
	}
	if report.Reclaim.Skipped != "" {
		t.Fatalf("should reclaim, skipped: %s", report.Reclaim.Skipped)
	}
	if _, err := os.Stat(fx.Worktree); err == nil {
		t.Fatal("worktree should be removed")
	}
	if refExists(t, fx.Clone, "refs/heads/dev/actor/feat/task") {
		t.Fatal("local task branch should be deleted")
	}
	if refExists(t, fx.Origin, "refs/heads/dev/actor/feat/task") {
		t.Fatal("remote task branch should be deleted")
	}
}

func TestRun_ProtectedTaskPatternIntegratesWithoutReclaim(t *testing.T) {
	for _, branch := range []string{"hotfix/urgent", "release/1.0"} {
		t.Run(branch, func(t *testing.T) {
			fx := runFixtureForBranch(t, branch, strings.Split(branch, "/")[0]+"/*")
			report, err := Run(context.Background(), gitcmd.NewExecutor(), RunOptions{
				CheckOptions: CheckOptions{RepoPath: fx.Worktree, Branch: branch},
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if !report.Integrated || report.Reclaim.Incomplete() {
				t.Fatalf("want integrated with intentional reclaim skip: %s", FormatRun(report))
			}
			if !strings.Contains(report.Reclaim.Skipped, "protected from reclaim") {
				t.Fatalf("reclaim skip = %q, want protected diagnosis", report.Reclaim.Skipped)
			}
			if _, err := os.Stat(fx.Worktree); err != nil {
				t.Fatalf("protected worktree should remain: %v", err)
			}
			assertRef(t, fx.Clone, "refs/heads/"+branch)
			assertRef(t, fx.Origin, "refs/heads/"+branch)
		})
	}
}

func TestRun_ReclaimPreservesSiblingTaskWorktreeAndParent(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	runGit(t, fx.Clone, "branch", "develop")
	runGit(t, fx.Clone, "push", "-u", fx.Remote, "develop")

	parent := filepath.Join(t.TempDir(), "task-worktrees")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatalf("create sibling worktree parent: %v", err)
	}
	branchA := "dev/actor/feat/task-a"
	branchB := "dev/actor/feat/task-b"
	worktreeA := filepath.Join(parent, "task-a")
	worktreeB := filepath.Join(parent, "task-b")
	runGit(t, fx.Clone, "worktree", "add", "-b", branchA, worktreeA, "develop")
	writeFile(t, worktreeA, "task-a.txt", "task a\n")
	writeRepoFile(t, worktreeA, ".gz-git.yaml", "branch:\n  integrationBranch: develop\n  taskPattern: dev/*\n")
	writeGateMakefile(t, worktreeA)
	runGit(t, worktreeA, "add", ".")
	runGit(t, worktreeA, "commit", "-m", "task a")
	runGit(t, worktreeA, "push", "-u", fx.Remote, "HEAD")

	runGit(t, fx.Clone, "worktree", "add", "-b", branchB, worktreeB, "develop")
	writeFile(t, worktreeB, "sibling-sentinel.txt", "must survive reclaim\n")

	report, err := Run(context.Background(), gitcmd.NewExecutor(), RunOptions{
		CheckOptions: CheckOptions{
			RepoPath: worktreeA,
			Branch:   branchA,
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !report.Integrated || report.Reclaim.Incomplete() {
		t.Fatalf("want completed integration and reclaim: %s", FormatRun(report))
	}
	if _, err := os.Stat(worktreeA); !os.IsNotExist(err) {
		t.Fatalf("reclaimed task worktree = %v, want removed", err)
	}
	if _, err := os.Stat(parent); err != nil {
		t.Fatalf("sibling parent removed: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(worktreeB, "sibling-sentinel.txt")); err != nil || string(got) != "must survive reclaim\n" {
		t.Fatalf("sibling sentinel = %q, %v; want preserved", got, err)
	}
	if !refExists(t, fx.Clone, "refs/heads/"+branchB) {
		t.Fatalf("sibling branch %s should remain", branchB)
	}
	siblingInfo, err := os.Stat(worktreeB)
	if err != nil {
		t.Fatalf("stat sibling worktree: %v", err)
	}
	worktrees, err := newGitRepo(gitcmd.NewExecutor(), fx.Clone).listWorktrees(context.Background())
	if err != nil {
		t.Fatalf("list worktrees: %v", err)
	}
	registered := false
	for _, worktree := range worktrees {
		info, statErr := os.Stat(worktree.Path)
		if statErr == nil && os.SameFile(siblingInfo, info) && worktree.Branch == branchB {
			registered = true
			break
		}
	}
	if !registered {
		t.Fatalf("sibling worktree %s on %s is not registered: %+v", worktreeB, branchB, worktrees)
	}
}

func TestReclaimRemoteBranch_LeaseRefusesMovedTip(t *testing.T) {
	fx := runFixture(t, "dev/*")
	task := "dev/actor/feat/task"
	oldSHA := gitOutput(t, fx.Worktree, "rev-parse", "HEAD")

	other := filepath.Join(t.TempDir(), "other")
	runGit(t, "", "clone", fx.Origin, other)
	runGit(t, other, "config", "user.email", "test@test.com")
	runGit(t, other, "config", "user.name", "Test User")
	runGit(t, other, "checkout", "-B", task, "origin/"+task)
	writeFile(t, other, "sneak.txt", "sneak\n")
	runGit(t, other, "add", "sneak.txt")
	runGit(t, other, "commit", "-m", "sneak")
	runGit(t, other, "push", "origin", task)

	var out ReclaimResult
	ok := reclaimRemoteBranch(context.Background(), newGitRepo(gitcmd.NewExecutor(), fx.Clone), reclaimOpts{
		Branch:  task,
		Remote:  fx.Remote,
		TaskSHA: oldSHA,
	}, &out)
	if ok {
		t.Fatalf("lease should refuse a moved remote: %+v", out)
	}
	if !refExists(t, fx.Origin, "refs/heads/"+task) {
		t.Fatal("moved remote branch must still exist")
	}
}

func TestReclaimRemoteBranch_AlreadyDeleted(t *testing.T) {
	fx := runFixture(t, "dev/*")
	task := "dev/actor/feat/task"
	sha := gitOutput(t, fx.Worktree, "rev-parse", "HEAD")
	runGit(t, fx.Clone, "push", fx.Remote, ":"+task)
	runGit(t, fx.Clone, "update-ref", "refs/remotes/"+fx.Remote+"/"+task, sha)
	if !refExists(t, fx.Clone, "refs/remotes/"+fx.Remote+"/"+task) {
		t.Fatal("tracking ref must remain so reclaim still attempts the delete")
	}

	var out ReclaimResult
	ok := reclaimRemoteBranch(context.Background(), newGitRepo(gitcmd.NewExecutor(), fx.Clone), reclaimOpts{
		Branch:  task,
		Remote:  fx.Remote,
		TaskSHA: sha,
	}, &out)
	if !ok {
		t.Fatalf("already-deleted remote must not fail reclaim: %+v", out)
	}
	found := false
	for _, done := range out.Done {
		if strings.Contains(done, "already-deleted") || done == "remote-branch" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("want already-deleted (or a no-op delete), got %+v", out)
	}
}

func TestReclaimRemoteBranch_EmptyTaskSHARefusesDelete(t *testing.T) {
	fx := runFixture(t, "dev/*")
	task := "dev/actor/feat/task"

	var out ReclaimResult
	ok := reclaimRemoteBranch(context.Background(), newGitRepo(gitcmd.NewExecutor(), fx.Clone), reclaimOpts{
		Branch: task,
		Remote: fx.Remote,
	}, &out)
	if ok {
		t.Fatalf("empty TaskSHA must not delete: %+v", out)
	}
	if !refExists(t, fx.Origin, "refs/heads/"+task) {
		t.Fatal("remote task branch must still exist")
	}
}

func runFixture(t *testing.T, pattern string) *testutil.WorktreeOrigin {
	t.Helper()
	return runFixtureForBranch(t, "dev/actor/feat/task", pattern)
}

func runFixtureForBranch(t *testing.T, branch, pattern string) *testutil.WorktreeOrigin {
	t.Helper()
	fx := testutil.TempWorktreeWithBareOrigin(t)
	runGit(t, fx.Clone, "branch", "develop")
	runGit(t, fx.Clone, "push", "-u", fx.Remote, "develop")
	runGit(t, fx.Worktree, "checkout", "-B", branch, "develop")
	writeFile(t, fx.Worktree, "task.txt", "task\n")
	body := "branch:\n  integrationBranch: develop\n"
	if pattern != "" {
		body += "  taskPattern: " + pattern + "\n"
	}
	writeRepoFile(t, fx.Worktree, ".gz-git.yaml", body)
	writeGateMakefile(t, fx.Worktree)
	runGit(t, fx.Worktree, "add", ".")
	runGit(t, fx.Worktree, "commit", "-m", "task work")
	runGit(t, fx.Worktree, "push", "-u", fx.Remote, "HEAD")
	return fx
}

func assertRef(t *testing.T, dir, ref string) {
	t.Helper()
	if !refExists(t, dir, ref) {
		t.Fatalf("missing ref %s", ref)
	}
}

func refExists(t *testing.T, dir, ref string) bool {
	t.Helper()
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", ref) //nolint:noctx // test helper
	cmd.Dir = dir
	return cmd.Run() == nil
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:noctx // test helper
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out))
}
