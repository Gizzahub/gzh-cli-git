// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)

func TestCheck_UndeclaredTargetsOriginHead(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	report, err := Check(context.Background(), gitcmd.NewExecutor(), CheckOptions{
		RepoPath: fx.Worktree,
		Branch:   "feature/worktree",
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if report.Plan.Target != "origin/main" {
		t.Fatalf("target = %q, want origin/main", report.Plan.Target)
	}
	if report.Plan.Integration.Name != "main" || report.Plan.Integration.Source != SourceHeuristic {
		t.Fatalf("integration = %+v, want heuristic main", report.Plan.Integration)
	}
}

func TestCheck_ReadyWhenFreshCleanPushed(t *testing.T) {
	fx := readyTaskFixture(t)
	report, err := Check(context.Background(), gitcmd.NewExecutor(), CheckOptions{
		RepoPath: fx.Worktree,
		Branch:   "dev/actor/feat/task",
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !report.Ready {
		t.Fatalf("want READY, got\n%s", FormatCheck(report))
	}
}

func TestCheck_NoGateFails(t *testing.T) {
	fx := readyTaskFixtureNoGate(t)
	report, err := Check(context.Background(), gitcmd.NewExecutor(), CheckOptions{
		RepoPath: fx.Worktree,
		Branch:   "dev/actor/feat/task",
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if report.Ready {
		t.Fatalf("no gate must not be READY:\n%s", FormatCheck(report))
	}
	found := false
	for _, item := range report.Items {
		if item.Name == "make check/lint" && item.Status == checkFail {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected make check/lint FAIL:\n%s", FormatCheck(report))
	}
}

func TestCheck_NoGateAllowedWhenSkippedFlag(t *testing.T) {
	fx := readyTaskFixtureNoGate(t)
	report, err := Check(context.Background(), gitcmd.NewExecutor(), CheckOptions{
		RepoPath:           fx.Worktree,
		Branch:             "dev/actor/feat/task",
		AllowSkippedChecks: true,
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !report.Ready {
		t.Fatalf("want READY with --allow-skipped-checks, got\n%s", FormatCheck(report))
	}
	found := false
	for _, item := range report.Items {
		if item.Name == "make check/lint" && item.Status == checkWarn {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected make check/lint WARN:\n%s", FormatCheck(report))
	}
}

func TestCheck_BaselineMissingCDFailsWithPreciseReason(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	runGit(t, fx.Clone, "checkout", "-B", "develop")
	writeRepoFile(t, fx.Clone, "foo.py", "import pathlib\n")
	writeRepoFile(t, fx.Clone, "Makefile", "check:\n\t@true\nlint:\n\t@cd missing-component && true\n")
	writeRepoFile(t, fx.Clone, ".gz-git.yaml", "branch:\n  integrationBranch: develop\n")
	runGit(t, fx.Clone, "add", ".")
	runGit(t, fx.Clone, "commit", "-m", "add target gate")
	runGit(t, fx.Clone, "push", "-u", fx.Remote, "develop")

	runGit(t, fx.Worktree, "checkout", "-B", "dev/actor/feat/task", "develop")
	writeRepoFile(t, fx.Worktree, "Makefile", "check:\n\t@true\nlint:\n\t@printf 'F401 unused import\\n --> foo.py:1:1\\n'\n\t@false\n")
	runGit(t, fx.Worktree, "add", "Makefile")
	runGit(t, fx.Worktree, "commit", "-m", "change lint runner")
	runGit(t, fx.Worktree, "push", "-u", fx.Remote, "HEAD")

	report, err := Check(context.Background(), gitcmd.NewExecutor(), CheckOptions{
		RepoPath:           fx.Worktree,
		Branch:             "dev/actor/feat/task",
		AllowSkippedChecks: true,
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if report.Ready {
		t.Fatalf("missing baseline component must not be READY:\n%s", FormatCheck(report))
	}
	for _, item := range report.Items {
		if item.Name == "make lint" {
			if item.Status != checkFail || !strings.Contains(item.Detail, "baseline make lint did not run") || !strings.Contains(item.Detail, "missing-component") {
				t.Fatalf("make lint = %+v", item)
			}
			return
		}
	}
	t.Fatalf("make lint result missing:\n%s", FormatCheck(report))
}

func TestCheck_StaleTargetFailsFreshness(t *testing.T) {
	fx := readyTaskFixture(t)
	// Advance develop after the task branch was created.
	runGit(t, fx.Clone, "checkout", "develop")
	writeFile(t, fx.Clone, "later.txt", "later\n")
	runGit(t, fx.Clone, "add", "later.txt")
	runGit(t, fx.Clone, "commit", "-m", "develop moves")
	runGit(t, fx.Clone, "push", fx.Remote, "develop")

	report, err := Check(context.Background(), gitcmd.NewExecutor(), CheckOptions{
		RepoPath: fx.Worktree,
		Branch:   "dev/actor/feat/task",
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if report.Ready {
		t.Fatal("stale target must not be READY")
	}
	found := false
	for _, item := range report.Items {
		if item.Name == "freshness" && item.Status == checkFail {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected freshness FAIL:\n%s", FormatCheck(report))
	}
}

func TestCheck_DirectToDefaultWithoutIntegration(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	runGit(t, fx.Worktree, "push", "-u", fx.Remote, "HEAD")
	report, err := Check(context.Background(), gitcmd.NewExecutor(), CheckOptions{
		RepoPath:        fx.Worktree,
		Branch:          "feature/worktree",
		Target:          fx.Remote + "/main",
		DirectToDefault: true,
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if report.Plan.Target != fx.Remote+"/main" {
		t.Fatalf("target = %q", report.Plan.Target)
	}
}

func readyTaskFixture(t *testing.T) *testutil.WorktreeOrigin {
	t.Helper()
	fx := readyTaskFixtureNoGate(t)
	writeGateMakefile(t, fx.Worktree)
	runGit(t, fx.Worktree, "add", "Makefile")
	runGit(t, fx.Worktree, "commit", "-m", "declare check gate")
	runGit(t, fx.Worktree, "push", fx.Remote, "HEAD")
	return fx
}

func readyTaskFixtureNoGate(t *testing.T) *testutil.WorktreeOrigin {
	t.Helper()
	fx := testutil.TempWorktreeWithBareOrigin(t)
	runGit(t, fx.Clone, "branch", "develop")
	runGit(t, fx.Clone, "push", "-u", fx.Remote, "develop")
	runGit(t, fx.Worktree, "checkout", "-B", "dev/actor/feat/task", "develop")
	writeFile(t, fx.Worktree, "task.txt", "task\n")
	runGit(t, fx.Worktree, "add", "task.txt")
	runGit(t, fx.Worktree, "commit", "-m", "task work")
	writeRepoFile(t, fx.Worktree, ".gz-git.yaml", "branch:\n  integrationBranch: develop\n")
	runGit(t, fx.Worktree, "add", ".gz-git.yaml")
	runGit(t, fx.Worktree, "commit", "-m", "declare integration branch")
	runGit(t, fx.Worktree, "push", "-u", fx.Remote, "HEAD")
	return fx
}

func writeGateMakefile(t *testing.T, dir string) {
	t.Helper()
	writeRepoFile(t, dir, "Makefile", "check:\n\t@true\n")
}

func writeRepoFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
