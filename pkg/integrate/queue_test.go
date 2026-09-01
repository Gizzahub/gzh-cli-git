// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"context"
	"fmt"
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
	if report.Integration.Participates {
		t.Fatalf("integration = %+v, want none without remote HEAD", report.Integration)
	}
	got := refsOf(report)
	if contains(got, "main") {
		t.Errorf("queue listed excluded base %q: %v", "main", got)
	}
	if !contains(got, "develop") {
		t.Fatalf("undeclared develop must stay in the queue: %v", got)
	}
	if !contains(got, "feat/task") {
		t.Fatalf("queue missing feat/task: %v", got)
	}
}

func TestCollectQueue_LoadsDeclaredIntegrationBranch(t *testing.T) {
	dir := testutil.TempGitRepoWithCommit(t)
	writeFile(t, dir, ".gz-git.yaml", "branch:\n  integrationBranch: develop\n")
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
		t.Fatalf("integration = %+v, want declared develop", report.Integration)
	}
	got := refsOf(report)
	if contains(got, "develop") {
		t.Fatalf("declared integration branch listed in queue: %v", got)
	}
	if !contains(got, "feat/task") {
		t.Fatalf("queue missing feat/task: %v", got)
	}
}

func TestCollectQueue_ControllerUsesDeclaredRemoteBaseAndTaskPattern(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	runGit(t, fx.Clone, "checkout", "-b", "develop")
	runGit(t, fx.Clone, "push", "-u", fx.Remote, "develop")
	runGit(t, fx.Worktree, "fetch", fx.Remote)
	runGit(t, fx.Worktree, "checkout", "-b", "dev/task")
	writeFile(t, fx.Worktree, "task.txt", "task\n")
	runGit(t, fx.Worktree, "add", "task.txt")
	runGit(t, fx.Worktree, "commit", "-m", "task")
	runGit(t, fx.Worktree, "branch", "feat/not-a-task")
	writeFile(t, fx.Worktree, ".gz-git.yaml", "branch:\n  integrationBranch: main\n")

	remoteURL := strings.TrimSpace(runGitInTest(t, fx.Worktree, "remote", "get-url", fx.Remote))
	controller := filepath.Join(t.TempDir(), "devbox.yaml")
	writeFile(t, filepath.Dir(controller), filepath.Base(controller), fmt.Sprintf("workspaces:\n  component:\n    url: %q\n    branch:\n      integrationBranch: develop\n      taskPattern: dev/*\n    integration: {}\n", remoteURL))

	report, err := CollectQueue(context.Background(), gitcmd.NewExecutor(), QueueOptions{
		RepoPath:         fx.Worktree,
		NoFetch:          true,
		ControllerConfig: controller,
	})
	if err != nil {
		t.Fatalf("CollectQueue: %v", err)
	}
	if report.Remote != fx.Remote || report.Base != fx.Remote+"/develop" || report.BaseSource != "controller" {
		t.Fatalf("controller base = remote=%q base=%q source=%q", report.Remote, report.Base, report.BaseSource)
	}
	if !report.Integration.Participates || report.Integration.Name != "develop" {
		t.Fatalf("integration = %+v, want controller develop", report.Integration)
	}
	got := refsOf(report)
	if !contains(got, "dev/task") {
		t.Fatalf("controller task ref missing: %v", got)
	}
	if contains(got, "feat/not-a-task") {
		t.Fatalf("controller task pattern did not filter ref: %v", got)
	}
	if contains(got, "develop") || contains(got, fx.Remote+"/develop") {
		t.Fatalf("controller integration leaked into queue: %v", got)
	}
}

func TestCollectQueue_ControllerRejectsDifferentBase(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	remoteURL := strings.TrimSpace(runGitInTest(t, fx.Worktree, "remote", "get-url", fx.Remote))
	controller := filepath.Join(t.TempDir(), "devbox.yaml")
	writeFile(t, filepath.Dir(controller), filepath.Base(controller), fmt.Sprintf("workspaces:\n  component: {url: %q, branch: {integrationBranch: develop}, integration: {}}\n", remoteURL))

	_, err := CollectQueue(context.Background(), gitcmd.NewExecutor(), QueueOptions{
		RepoPath:         fx.Worktree,
		Base:             "main",
		NoFetch:          true,
		ControllerConfig: controller,
	})
	if err == nil || !strings.Contains(err.Error(), "--base must equal controller integration branch develop") {
		t.Fatalf("controller base mismatch = %v", err)
	}
}

func TestResolveQueueBase_ControllerRejectsOtherRemoteSpellings(t *testing.T) {
	controller := &controllerBinding{Integration: []string{"develop"}}
	for _, base := range []string{"other/develop", "refs/remotes/other/develop", "refs/remotes/origin/other/develop"} {
		t.Run(base, func(t *testing.T) {
			_, _, _, err := resolveQueueBase(context.Background(), gitRepo{}, base, "origin", controller)
			if err == nil || !strings.Contains(err.Error(), "--base must equal controller integration branch develop") {
				t.Fatalf("base %q error = %v", base, err)
			}
		})
	}
}

func TestResolveQueueBase_ControllerAcceptsOnlyDeclaredSpellings(t *testing.T) {
	controller := &controllerBinding{Integration: []string{"develop"}}
	for _, base := range []string{"develop", "refs/heads/develop", "origin/develop", "refs/remotes/origin/develop"} {
		t.Run(base, func(t *testing.T) {
			got, source, ok, err := resolveQueueBase(context.Background(), gitRepo{}, base, "origin", controller)
			if err != nil || !ok || got != "origin/develop" || source != "controller-flag" {
				t.Fatalf("base %q = (%q, %q, %t, %v)", base, got, source, ok, err)
			}
		})
	}
}

func TestCollectQueue_ControllerAcceptsQualifiedDeclaredBase(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	runGit(t, fx.Clone, "checkout", "-b", "develop")
	runGit(t, fx.Clone, "push", "-u", fx.Remote, "develop")
	runGit(t, fx.Worktree, "fetch", fx.Remote)
	remoteURL := strings.TrimSpace(runGitInTest(t, fx.Worktree, "remote", "get-url", fx.Remote))
	controller := filepath.Join(t.TempDir(), "devbox.yaml")
	writeFile(t, filepath.Dir(controller), filepath.Base(controller), fmt.Sprintf("workspaces:\n  component: {url: %q, branch: {integrationBranch: develop}, integration: {}}\n", remoteURL))

	report, err := CollectQueue(context.Background(), gitcmd.NewExecutor(), QueueOptions{
		RepoPath:         fx.Worktree,
		Base:             "refs/remotes/" + fx.Remote + "/develop",
		NoFetch:          true,
		ControllerConfig: controller,
	})
	if err != nil {
		t.Fatalf("CollectQueue: %v", err)
	}
	if report.Base != fx.Remote+"/develop" || report.BaseSource != "controller-flag" {
		t.Fatalf("controller qualified base = %q (%s)", report.Base, report.BaseSource)
	}
}

func TestCollectQueue_ControllerEmptyTaskPatternLeavesRefsUnfiltered(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	runGit(t, fx.Clone, "checkout", "-b", "develop")
	runGit(t, fx.Clone, "push", "-u", fx.Remote, "develop")
	runGit(t, fx.Worktree, "fetch", fx.Remote)
	remoteURL := strings.TrimSpace(runGitInTest(t, fx.Worktree, "remote", "get-url", fx.Remote))
	controller := filepath.Join(t.TempDir(), "devbox.yaml")
	writeFile(t, filepath.Dir(controller), filepath.Base(controller), fmt.Sprintf("workspaces:\n  component: {url: %q, branch: {integrationBranch: develop}, integration: {}}\n", remoteURL))

	report, err := CollectQueue(context.Background(), gitcmd.NewExecutor(), QueueOptions{
		RepoPath:         fx.Worktree,
		NoFetch:          true,
		ControllerConfig: controller,
	})
	if err != nil {
		t.Fatalf("CollectQueue: %v", err)
	}
	if !contains(refsOf(report), "feature/worktree") {
		t.Fatalf("empty controller taskPattern unexpectedly filtered queue: %v", refsOf(report))
	}
}

func TestCollectQueueRefs_PreservesRemoteProvenance(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	external := cloneForRemoteQueueTest(t, fx.Origin)
	runGit(t, external, "checkout", "-b", "dev/remote-only")
	writeFile(t, external, "remote-only.txt", "remote\n")
	runGit(t, external, "add", "remote-only.txt")
	runGit(t, external, "commit", "-m", "remote task")
	runGit(t, external, "push", "-u", fx.Remote, "dev/remote-only")
	runGit(t, fx.Worktree, "fetch", fx.Remote)

	refs, err := collectQueueRefs(context.Background(), newGitRepo(gitcmd.NewExecutor(), fx.Worktree), fx.Remote, fx.Remote+"/main", Resolution{}, []string{"dev/*"})
	if err != nil {
		t.Fatal(err)
	}
	if !hasQueueRef(refs, "refs/remotes/"+fx.Remote+"/dev/remote-only") {
		t.Fatalf("remote-only ref missing: %#v", refs)
	}
}

func TestCollectQueueRefs_DeduplicatesOnlyIdenticalLocalAndRemote(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	runGit(t, fx.Worktree, "checkout", "-b", "dev/same")
	writeFile(t, fx.Worktree, "same.txt", "same\n")
	runGit(t, fx.Worktree, "add", "same.txt")
	runGit(t, fx.Worktree, "commit", "-m", "same task")
	runGit(t, fx.Worktree, "push", "-u", fx.Remote, "dev/same")

	refs, err := collectQueueRefs(context.Background(), newGitRepo(gitcmd.NewExecutor(), fx.Worktree), fx.Remote, fx.Remote+"/main", Resolution{}, []string{"dev/*"})
	if err != nil {
		t.Fatal(err)
	}
	if !hasQueueRef(refs, "refs/heads/dev/same") || hasQueueRef(refs, "refs/remotes/"+fx.Remote+"/dev/same") {
		t.Fatalf("identical pair was not deduplicated correctly: %#v", refs)
	}
}

func TestCollectQueueRefs_KeepsDivergentLocalAndRemote(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	runGit(t, fx.Worktree, "checkout", "-b", "dev/divergent")
	writeFile(t, fx.Worktree, "divergent.txt", "remote\n")
	runGit(t, fx.Worktree, "add", "divergent.txt")
	runGit(t, fx.Worktree, "commit", "-m", "remote task")
	runGit(t, fx.Worktree, "push", "-u", fx.Remote, "dev/divergent")
	writeFile(t, fx.Worktree, "divergent.txt", "local\n")
	runGit(t, fx.Worktree, "add", "divergent.txt")
	runGit(t, fx.Worktree, "commit", "-m", "local task")

	refs, err := collectQueueRefs(context.Background(), newGitRepo(gitcmd.NewExecutor(), fx.Worktree), fx.Remote, fx.Remote+"/main", Resolution{}, []string{"dev/*"})
	if err != nil {
		t.Fatal(err)
	}
	for _, full := range []string{"refs/heads/dev/divergent", "refs/remotes/" + fx.Remote + "/dev/divergent"} {
		if !hasQueueRef(refs, full) {
			t.Fatalf("divergent ref %q missing: %#v", full, refs)
		}
	}
}

func TestCollectQueueRefs_SlashContainingRemoteAndLocalPrefixCollision(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOriginRemote(t, "team/upstream")
	external := cloneForRemoteQueueTest(t, fx.Origin)
	runGit(t, external, "checkout", "-b", "dev/remote")
	writeFile(t, external, "remote.txt", "remote\n")
	runGit(t, external, "add", "remote.txt")
	runGit(t, external, "commit", "-m", "remote task")
	runGit(t, external, "push", "-u", "origin", "dev/remote")
	runGit(t, fx.Worktree, "fetch", fx.Remote)
	runGit(t, fx.Worktree, "branch", "team/upstream/dev/remote")

	refs, err := collectQueueRefs(context.Background(), newGitRepo(gitcmd.NewExecutor(), fx.Worktree), fx.Remote, fx.Remote+"/main", Resolution{}, []string{"dev/*", "team/upstream/*"})
	if err != nil {
		t.Fatal(err)
	}
	for _, full := range []string{"refs/remotes/team/upstream/dev/remote", "refs/heads/team/upstream/dev/remote"} {
		if !hasQueueRef(refs, full) {
			t.Fatalf("slash remote/prefix-collision ref %q missing: %#v", full, refs)
		}
	}
}

func TestCollectQueue_ControllerFormatsLocalRemotePrefixCollisionDistinctly(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOriginRemote(t, "team/upstream")
	external := cloneForRemoteQueueTest(t, fx.Origin)
	runGit(t, external, "checkout", "-b", "dev/collision")
	writeFile(t, external, "collision.txt", "remote\n")
	runGit(t, external, "add", "collision.txt")
	runGit(t, external, "commit", "-m", "remote task")
	runGit(t, external, "push", "-u", "origin", "dev/collision")
	runGit(t, fx.Worktree, "fetch", fx.Remote)
	runGit(t, fx.Worktree, "branch", "team/upstream/dev/collision")

	remoteURL := strings.TrimSpace(runGitInTest(t, fx.Worktree, "remote", "get-url", fx.Remote))
	controller := filepath.Join(t.TempDir(), "devbox.yaml")
	writeFile(t, filepath.Dir(controller), filepath.Base(controller), fmt.Sprintf("workspaces:\n  component:\n    url: %q\n    branch:\n      integrationBranch: main\n      taskPattern: [dev/*, team/upstream/*]\n    integration: {}\n", remoteURL))
	report, err := CollectQueue(context.Background(), gitcmd.NewExecutor(), QueueOptions{
		RepoPath:         fx.Worktree,
		NoFetch:          true,
		ControllerConfig: controller,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"heads/team/upstream/dev/collision", "remotes/team/upstream/dev/collision"} {
		if !contains(refsOf(report), want) {
			t.Fatalf("collision display %q missing: %v", want, refsOf(report))
		}
		if !strings.Contains(FormatQueue(report), want) {
			t.Fatalf("formatted collision display %q missing:\n%s", want, FormatQueue(report))
		}
	}
}

func hasQueueRef(refs []queueRef, full string) bool {
	for _, ref := range refs {
		if ref.full == full {
			return true
		}
	}
	return false
}

func cloneForRemoteQueueTest(t *testing.T, origin string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "external")
	runGit(t, "", "clone", origin, dir)
	runGit(t, dir, "config", "user.name", "Queue Test")
	runGit(t, dir, "config", "user.email", "queue-test@example.invalid")
	return dir
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

func TestCollectQueueRefs_LegacyHeadsFullBaseExcludesDivergentLocalAndRemote(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	runGit(t, fx.Clone, "checkout", "main")
	writeFile(t, fx.Clone, "local-main.txt", "local main\n")
	runGit(t, fx.Clone, "add", "local-main.txt")
	runGit(t, fx.Clone, "commit", "-m", "local main moves")
	runGit(t, fx.Worktree, "checkout", "-b", "feat/task")
	writeFile(t, fx.Worktree, "task.txt", "task\n")
	runGit(t, fx.Worktree, "add", "task.txt")
	runGit(t, fx.Worktree, "commit", "-m", "task")

	refs, err := collectQueueRefs(context.Background(), newGitRepo(gitcmd.NewExecutor(), fx.Worktree), fx.Remote, "refs/heads/main", Resolution{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertLegacyFullBaseExcluded(t, refs, fx.Remote)
	if !hasQueueRef(refs, "refs/heads/feat/task") {
		t.Fatalf("task disappeared with legacy full base: %#v", refs)
	}
}

func TestCollectQueueRefs_LegacyRemoteFullBaseExcludesDivergentLocalAndRemote(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	external := cloneForRemoteQueueTest(t, fx.Origin)
	writeFile(t, external, "remote-main.txt", "remote main\n")
	runGit(t, external, "add", "remote-main.txt")
	runGit(t, external, "commit", "-m", "remote main moves")
	runGit(t, external, "push", "origin", "main")
	runGit(t, fx.Worktree, "fetch", fx.Remote)
	runGit(t, fx.Worktree, "checkout", "-b", "feat/task")
	writeFile(t, fx.Worktree, "task.txt", "task\n")
	runGit(t, fx.Worktree, "add", "task.txt")
	runGit(t, fx.Worktree, "commit", "-m", "task")

	refs, err := collectQueueRefs(context.Background(), newGitRepo(gitcmd.NewExecutor(), fx.Worktree), fx.Remote, "refs/remotes/"+fx.Remote+"/main", Resolution{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertLegacyFullBaseExcluded(t, refs, fx.Remote)
	if !hasQueueRef(refs, "refs/heads/feat/task") {
		t.Fatalf("task disappeared with legacy full base: %#v", refs)
	}
}

func assertLegacyFullBaseExcluded(t *testing.T, refs []queueRef, remote string) {
	t.Helper()
	for _, full := range []string{"refs/heads/main", "refs/remotes/" + remote + "/main"} {
		if hasQueueRef(refs, full) {
			t.Fatalf("legacy full base leaked %q: %#v", full, refs)
		}
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
