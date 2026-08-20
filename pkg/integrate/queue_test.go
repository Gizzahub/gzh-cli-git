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
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)

func TestCollectQueue_EmptyIsSuccess(t *testing.T) {
	dir := testutil.TempGitRepoWithCommit(t)
	report, err := CollectQueue(context.Background(), gitcmd.NewExecutor(), QueueOptions{
		RepoPath: dir,
		Base:     "main",
		NoFetch:  true,
	})
	if err != nil {
		t.Fatalf("CollectQueue: %v", err)
	}
	if report.BaseMissing {
		t.Fatalf("base missing in a repo that has main: %+v", report)
	}
	if len(report.Entries) != 0 {
		t.Fatalf("empty trunk must be an empty queue, got %v", refsOf(report))
	}
}

func TestCollectQueue_ExcludesBaseAndIntegration(t *testing.T) {
	dir := testutil.TempGitRepoWithCommit(t)
	runGit(t, dir, "branch", "develop")
	runGit(t, dir, "checkout", "-b", "feat/task")
	writeFile(t, dir, "task.txt", "task\n")
	runGit(t, dir, "add", "task.txt")
	runGit(t, dir, "commit", "-m", "task")

	report, err := CollectQueue(context.Background(), gitcmd.NewExecutor(), QueueOptions{
		RepoPath: dir,
		Base:     "main",
		NoFetch:  true,
	})
	if err != nil {
		t.Fatalf("CollectQueue: %v", err)
	}
	if !report.Integration.Participates || report.Integration.Name != "develop" {
		t.Fatalf("integration = %+v, want develop", report.Integration)
	}
	got := refsOf(report)
	for _, name := range []string{"main", "develop"} {
		if contains(got, name) {
			t.Errorf("queue listed excluded branch %q: %v", name, got)
		}
	}
	if !contains(got, "feat/task") {
		t.Fatalf("queue missing feat/task: %v", got)
	}
}

func TestCollectQueue_BaseReleaseKeepsSlash(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	dir := fx.Clone
	runGit(t, dir, "checkout", "-b", "release/2.0")
	runGit(t, dir, "push", "-u", fx.Remote, "release/2.0")
	runGit(t, dir, "checkout", "-b", "feat/task")

	report, err := CollectQueue(context.Background(), gitcmd.NewExecutor(), QueueOptions{
		RepoPath: dir,
		Base:     fx.Remote + "/release/2.0",
		NoFetch:  true,
	})
	if err != nil {
		t.Fatalf("CollectQueue: %v", err)
	}
	got := refsOf(report)
	if contains(got, "release/2.0") || contains(got, "2.0") {
		t.Fatalf("base release/2.0 leaked into queue as %v", got)
	}
}

func TestCollectQueue_ReportsFreshnessConflictAge(t *testing.T) {
	dir := testutil.TempGitRepoWithCommit(t)

	runGit(t, dir, "checkout", "-b", "feat/conflict")
	writeFile(t, dir, "README.md", "conflict side\n")
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "conflict")

	runGit(t, dir, "checkout", "main")
	writeFile(t, dir, "README.md", "main side\n")
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "main moves")

	runGit(t, dir, "checkout", "-b", "feat/old")
	writeFile(t, dir, "old.txt", "old\n")
	runGit(t, dir, "add", "old.txt")
	runGitDated(t, dir, time.Now().Add(-10*24*time.Hour), "old work")

	report, err := CollectQueue(context.Background(), gitcmd.NewExecutor(), QueueOptions{
		RepoPath:   dir,
		Base:       "main",
		NoFetch:    true,
		ExpiryDays: 7,
		Now:        time.Now(),
	})
	if err != nil {
		t.Fatalf("CollectQueue: %v", err)
	}

	byRef := map[string]QueueEntry{}
	for _, e := range report.Entries {
		byRef[e.Ref] = e
	}
	conflict, ok := byRef["feat/conflict"]
	if !ok {
		t.Fatalf("missing feat/conflict: %v", refsOf(report))
	}
	if conflict.MergeState != "CONFLICT" {
		t.Errorf("feat/conflict merge = %q, want CONFLICT", conflict.MergeState)
	}
	if !strings.HasPrefix(conflict.BaseState, "stale") {
		t.Errorf("feat/conflict base = %q, want stale", conflict.BaseState)
	}

	old, ok := byRef["feat/old"]
	if !ok {
		t.Fatalf("missing feat/old: %v", refsOf(report))
	}
	if !old.Expired {
		t.Errorf("feat/old should be expired: %+v", old)
	}
	if report.ConflictCount < 1 || report.ExpiredCount < 1 {
		t.Errorf("counts conflict=%d expired=%d, want both >= 1", report.ConflictCount, report.ExpiredCount)
	}
}

func TestCollectQueue_MissingBaseIsReportable(t *testing.T) {
	dir := testutil.TempGitRepoWithCommit(t)
	report, err := CollectQueue(context.Background(), gitcmd.NewExecutor(), QueueOptions{
		RepoPath: dir,
		NoFetch:  true,
	})
	if err != nil {
		t.Fatalf("CollectQueue: %v", err)
	}
	if !report.BaseMissing {
		t.Fatalf("want BaseMissing when no remote HEAD and no --base, got %+v", report)
	}
	if len(report.Entries) != 0 {
		t.Fatalf("missing base must not invent a queue, got %v", refsOf(report))
	}
}

func TestFormatQueue_EmptyTable(t *testing.T) {
	out := FormatQueue(&QueueReport{Base: "main", ExpiryDays: 7})
	if !strings.Contains(out, "BRANCH") || !strings.Contains(out, "base=main") {
		t.Fatalf("format missing headers:\n%s", out)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:noctx // test helper
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func runGitDated(t *testing.T, dir string, when time.Time, message string) {
	t.Helper()
	stamp := when.Format("2006-01-02T15:04:05-0700")
	cmd := exec.Command("git", "commit", "-m", message) //nolint:noctx // test helper
	cmd.Dir = dir
	cmd.Env = append(
		os.Environ(),
		"GIT_AUTHOR_DATE="+stamp,
		"GIT_COMMITTER_DATE="+stamp,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("dated commit: %v\n%s", err, out)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func refsOf(r *QueueReport) []string {
	out := make([]string, 0, len(r.Entries))
	for _, e := range r.Entries {
		out = append(out, e.Ref)
	}
	return out
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
