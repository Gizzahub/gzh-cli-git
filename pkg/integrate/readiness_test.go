// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)

func TestCheck_ContractUsesTargetRunnerNotHeadMakefile(t *testing.T) {
	fx := readinessFixture(t)
	writeRepoFile(t, fx.Worktree, "Makefile", "check:\n\t@false\n")
	runGit(t, fx.Worktree, "add", "Makefile")
	runGit(t, fx.Worktree, "commit", "-m", "head makefile is irrelevant")
	runGit(t, fx.Worktree, "push", fx.Remote, "HEAD")
	report, err := Check(context.Background(), gitcmd.NewExecutor(), CheckOptions{RepoPath: fx.Worktree, Branch: "dev/actor/feat/task", IntegrationConfig: []string{"develop"}})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !report.Ready || report.GateMode != "contract-v1" || report.ReadinessStatus != "ready" {
		t.Fatalf("want target-owned READY contract, got\n%s", FormatCheck(report))
	}
	if report.RunnerPath != ".gz-git/readiness/check" || report.ReadinessTreeOID == "" {
		t.Fatalf("missing provenance: %+v", report)
	}
}

func TestCheck_ContractChangeFailsClosed(t *testing.T) {
	fx := readinessFixture(t)
	if err := os.MkdirAll(filepath.Join(fx.Worktree, ".gz-git", "readiness"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, fx.Worktree, ".gz-git/readiness/check", "#!/bin/sh\nprintf '{\"version\":1,\"status\":\"ready\",\"summary\":\"weakened\"}'\n")
	if err := os.Chmod(filepath.Join(fx.Worktree, ".gz-git", "readiness", "check"), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, fx.Worktree, "add", ".gz-git/readiness/check")
	runGit(t, fx.Worktree, "commit", "-m", "weaken readiness")
	runGit(t, fx.Worktree, "push", fx.Remote, "HEAD")
	report, err := Check(context.Background(), gitcmd.NewExecutor(), CheckOptions{RepoPath: fx.Worktree, Branch: "dev/actor/feat/task", IntegrationConfig: []string{"develop"}})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if report.Ready || !hasCheckDetail(report, "readiness contract", checkFail, "changed") {
		t.Fatalf("changed contract must fail:\n%s", FormatCheck(report))
	}
}

func TestCheck_BootstrapFailureDoesNotRunHeadMake(t *testing.T) {
	fx := readyTaskFixtureNoGate(t)
	if err := os.MkdirAll(filepath.Join(fx.Worktree, ".gz-git", "readiness"), 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(t.TempDir(), "make-ran")
	writeRepoFile(t, fx.Worktree, ".gz-git.yaml", "branch:\n  integrationBranch: develop\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/check\n")
	writeRepoFile(t, fx.Worktree, ".gz-git/readiness/check", "#!/bin/sh\nprintf '{\"version\":1,\"status\":\"ready\",\"summary\":\"ok\"}'\n")
	if err := os.Chmod(filepath.Join(fx.Worktree, ".gz-git", "readiness", "check"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, fx.Worktree, "Makefile", "check:\n\t@touch "+sentinel+"\n")
	runGit(t, fx.Worktree, "add", ".")
	runGit(t, fx.Worktree, "commit", "-m", "source-only readiness")
	runGit(t, fx.Worktree, "push", fx.Remote, "HEAD")
	report, err := Check(context.Background(), gitcmd.NewExecutor(), CheckOptions{RepoPath: fx.Worktree, Branch: "dev/actor/feat/task"})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if report.Ready || !hasCheckDetail(report, "readiness contract", checkFail, "bootstrap required") {
		t.Fatalf("bootstrap must fail: %s", FormatCheck(report))
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("head Make ran during contract failure: %v", err)
	}
}

func TestCheck_AbsentContractsUseLegacyMake(t *testing.T) {
	fx := readyTaskFixture(t)
	report, err := Check(context.Background(), gitcmd.NewExecutor(), CheckOptions{RepoPath: fx.Worktree, Branch: "dev/actor/feat/task"})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Ready || report.GateMode != "legacy-make" || !hasCheckDetail(report, "readiness contract", checkWarn, "legacy head-owned gate") || !hasCheckDetail(report, "make check", checkPass, "ok") {
		t.Fatalf("expected legacy gate behavior:\n%s", FormatCheck(report))
	}
}

func TestCheck_TargetContractSourceAbsentDoesNotRunHeadMake(t *testing.T) {
	fx := readinessFixture(t)
	sentinel := filepath.Join(t.TempDir(), "make-ran")
	if err := os.Remove(filepath.Join(fx.Worktree, ".gz-git.yaml")); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, fx.Worktree, "Makefile", "check:\n\t@touch "+sentinel+"\n")
	runGit(t, fx.Worktree, "add", "-A")
	runGit(t, fx.Worktree, "commit", "-m", "remove source contract")
	runGit(t, fx.Worktree, "push", fx.Remote, "HEAD")
	report, err := Check(context.Background(), gitcmd.NewExecutor(), CheckOptions{RepoPath: fx.Worktree, Branch: "dev/actor/feat/task", IntegrationConfig: []string{"develop"}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Ready || !hasCheckDetail(report, "readiness contract", checkFail, "source is missing") {
		t.Fatalf("missing source must fail:\n%s", FormatCheck(report))
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("head Make ran: %v", err)
	}
}

func TestCheck_MalformedSourceDoesNotRunHeadMake(t *testing.T) {
	fx := readinessFixture(t)
	sentinel := filepath.Join(t.TempDir(), "make-ran")
	writeRepoFile(t, fx.Worktree, ".gz-git.yaml", "branch:\n  readiness:\n    version: 2\n    runner: .gz-git/readiness/check\n")
	writeRepoFile(t, fx.Worktree, "Makefile", "check:\n\t@touch "+sentinel+"\n")
	runGit(t, fx.Worktree, "add", ".")
	runGit(t, fx.Worktree, "commit", "-m", "malform source contract")
	runGit(t, fx.Worktree, "push", fx.Remote, "HEAD")
	report, err := Check(context.Background(), gitcmd.NewExecutor(), CheckOptions{RepoPath: fx.Worktree, Branch: "dev/actor/feat/task", IntegrationConfig: []string{"develop"}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Ready || !hasCheckDetail(report, "readiness contract", checkFail, "version") {
		t.Fatalf("malformed source must fail:\n%s", FormatCheck(report))
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("head Make ran: %v", err)
	}
}

func TestCheck_MalformedTargetDoesNotRunHeadMake(t *testing.T) {
	fx := readinessFixture(t)
	advanceReadinessTarget(t, fx,
		"branch:\n  integrationBranch: develop\n  readiness:\n    version: 2\n    runner: .gz-git/readiness/check\n",
		"", "malform target contract")
	sentinel := filepath.Join(t.TempDir(), "make-ran")
	writeRepoFile(t, fx.Worktree, "Makefile", "check:\n\t@touch "+sentinel+"\n")
	runGit(t, fx.Worktree, "add", "Makefile")
	runGit(t, fx.Worktree, "commit", "-m", "head make sentinel")
	runGit(t, fx.Worktree, "push", fx.Remote, "HEAD")
	report, err := Check(context.Background(), gitcmd.NewExecutor(), CheckOptions{RepoPath: fx.Worktree, Branch: "dev/actor/feat/task", IntegrationConfig: []string{"develop"}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Ready || !hasCheckDetail(report, "readiness contract", checkFail, "version") {
		t.Fatalf("malformed target must fail:\n%s", FormatCheck(report))
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("head Make ran: %v", err)
	}
}

func TestCheck_AllowSkippedCannotDowngradeUnavailableContract(t *testing.T) {
	fx := readinessFixture(t)
	runner := "#!/bin/sh\nprintf '{\"version\":1,\"status\":\"unavailable\",\"summary\":\"dependency offline\"}'\n"
	advanceReadinessTarget(t, fx, "", runner, "make readiness unavailable")
	report, err := Check(context.Background(), gitcmd.NewExecutor(), CheckOptions{
		RepoPath: fx.Worktree, Branch: "dev/actor/feat/task", IntegrationConfig: []string{"develop"}, AllowSkippedChecks: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Ready || !hasCheckDetail(report, "readiness contract", checkFail, "measurement unavailable") {
		t.Fatalf("allow-skipped downgraded contract failure:\n%s", FormatCheck(report))
	}
}

func TestParseReadinessResult_RejectsTrailingAndUnknown(t *testing.T) {
	for _, raw := range []string{
		`{"version":1,"status":"ready","summary":"ok"} trailing`,
		`{"version":1,"status":"ready","summary":"ok","extra":true}`,
		`{"version":1,"status":"ready","summary":""}`,
		"{\"version\":1,\"status\":\"ready\",\"summary\":\"\xff\"}",
	} {
		if _, err := parseReadinessResult([]byte(raw)); err == nil {
			t.Fatalf("accepted invalid result %q", raw)
		}
	}
}

func TestParseReadinessResult_Statuses(t *testing.T) {
	for _, status := range []string{"not_ready", "unavailable"} {
		got, err := parseReadinessResult([]byte(`{"version":1,"status":"` + status + `","summary":"no"}`))
		if err != nil || got.Status != status {
			t.Fatalf("status %s: %+v %v", status, got, err)
		}
	}
}

func TestLoadReadinessContract_RejectsManifestAndRunnerForms(t *testing.T) {
	for _, mode := range []string{"manifest-oversize", "runner-nonexec", "runner-missing"} {
		t.Run(mode, func(t *testing.T) {
			fx := readinessFixture(t)
			switch mode {
			case "manifest-oversize":
				body := "branch:\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/check\n#" + strings.Repeat("x", readinessMaxManifest) + "\n"
				writeRepoFile(t, fx.Clone, ".gz-git.yaml", body)
				runGit(t, fx.Clone, "add", ".gz-git.yaml")
			case "runner-nonexec":
				if err := os.Chmod(filepath.Join(fx.Clone, ".gz-git", "readiness", "check"), 0o644); err != nil {
					t.Fatal(err)
				}
				runGit(t, fx.Clone, "add", ".gz-git/readiness/check")
			case "runner-missing":
				if err := os.Remove(filepath.Join(fx.Clone, ".gz-git", "readiness", "check")); err != nil {
					t.Fatal(err)
				}
				runGit(t, fx.Clone, "add", "-A")
			}
			runGit(t, fx.Clone, "commit", "-m", mode)
			sha := gitOutput(t, fx.Clone, "rev-parse", "HEAD")
			if _, _, err := loadReadinessContract(context.Background(), newGitRepo(gitcmd.NewExecutor(), fx.Clone), sha); err == nil {
				t.Fatal("invalid contract accepted")
			}
		})
	}
}

func TestLoadReadinessContract_LegacyManifestFormsRemainAbsent(t *testing.T) {
	for _, mode := range []string{"shorthand-oversize", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			fx := readinessFixture(t)
			switch mode {
			case "shorthand-oversize":
				writeRepoFile(t, fx.Clone, ".gz-git.yaml", "branch: develop\n#"+strings.Repeat("x", readinessMaxManifest)+"\n")
				runGit(t, fx.Clone, "add", ".gz-git.yaml")
			case "symlink":
				writeRepoFile(t, fx.Clone, "manifest-real", "branch: develop\n")
				if err := os.Remove(filepath.Join(fx.Clone, ".gz-git.yaml")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("manifest-real", filepath.Join(fx.Clone, ".gz-git.yaml")); err != nil {
					t.Fatal(err)
				}
				runGit(t, fx.Clone, "add", "-A")
			}
			runGit(t, fx.Clone, "commit", "-m", mode)
			sha := gitOutput(t, fx.Clone, "rev-parse", "HEAD")
			_, present, err := loadReadinessContract(context.Background(), newGitRepo(gitcmd.NewExecutor(), fx.Clone), sha)
			if err != nil || present {
				t.Fatalf("legacy manifest changed behavior: present=%v err=%v", present, err)
			}
		})
	}
}

func TestExecuteReadinessWithTimeoutAndEnv(t *testing.T) {
	dir := t.TempDir()
	runner := filepath.Join(dir, "runner")
	if err := os.WriteFile(runner, []byte("#!/bin/sh\nsleep 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := executeReadinessWithTimeout(context.Background(), runner, dir, dir, "a", "b", 10*time.Millisecond); err == nil {
		t.Fatal("timeout accepted")
	}
	env := readinessEnv([]string{"PATH=/x", "HOME=/h", "BASH_ENV=x", "GIT_DIR=x", "LANG=en_US"})
	if strings.Contains(strings.Join(env, "\n"), "BASH_ENV=") || strings.Contains(strings.Join(env, "\n"), "GIT_DIR=") || !strings.Contains(strings.Join(env, "\n"), "HOME=/h") {
		t.Fatalf("unsafe filtered environment: %v", env)
	}
}

func TestExecuteReadinessTimeoutKillsDescendants(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("readiness contracts fail closed on Windows")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	t.Setenv("READINESS_TEST_PID_FILE", pidFile)
	runner := filepath.Join(dir, "runner")
	body := "#!/bin/sh\nsleep 30 &\nprintf '%s' \"$!\" > \"$READINESS_TEST_PID_FILE\"\nwait\n"
	if err := os.WriteFile(runner, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := executeReadinessWithTimeout(ctx, runner, dir, dir, "a", "b", 5*time.Second)
		done <- err
	}()
	var pid []byte
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		pid, _ = os.ReadFile(pidFile)
		if len(pid) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(pid) == 0 {
		cancel()
		t.Fatalf("runner did not publish child pid")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel result = %v, want context.Canceled", err)
	}
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if err := exec.Command("kill", "-0", string(pid)).Run(); err != nil { //nolint:noctx // bounded test probe
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("descendant process %s survived timeout", pid)
}

func TestRunContractTimeoutCleansDetachedWorktrees(t *testing.T) {
	fx := readinessFixture(t)
	writeRepoFile(t, fx.Clone, ".gz-git/readiness/check", "#!/bin/sh\nsleep 30\n")
	if err := os.Chmod(filepath.Join(fx.Clone, ".gz-git", "readiness", "check"), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, fx.Clone, "add", ".gz-git/readiness/check")
	runGit(t, fx.Clone, "commit", "-m", "slow readiness runner")
	runGit(t, fx.Clone, "push", fx.Remote, "develop")
	runGit(t, fx.Worktree, "rebase", "develop")
	runGit(t, fx.Worktree, "push", "--force-with-lease", fx.Remote, "HEAD")

	g := newGitRepo(gitcmd.NewExecutor(), fx.Worktree)
	plan := TargetPlan{
		BranchSHA: gitOutput(t, fx.Worktree, "rev-parse", "HEAD"),
		TargetSHA: gitOutput(t, fx.Worktree, "rev-parse", "develop"),
	}
	contract, present, err := loadReadinessContract(context.Background(), g, plan.TargetSHA)
	if err != nil || !present {
		t.Fatalf("load readiness contract: %+v, %v", contract, err)
	}
	isolatedTemp := t.TempDir()
	t.Setenv("TMPDIR", isolatedTemp)
	before := gitOutput(t, fx.Worktree, "worktree", "list", "--porcelain")
	if _, _, err := runContractWithTimeout(context.Background(), g, plan, contract, 20*time.Millisecond); err == nil {
		t.Fatal("slow contract accepted")
	}
	after := gitOutput(t, fx.Worktree, "worktree", "list", "--porcelain")
	if after != before {
		t.Fatalf("readiness worktree leaked\nbefore:\n%s\nafter:\n%s", before, after)
	}
	entries, err := os.ReadDir(isolatedTemp)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("readiness temp root leaked: %v", entries)
	}
}

func TestMergeCleanupErrorDoesNotPruneUnrelatedWorktrees(t *testing.T) {
	fx := readinessFixture(t)
	unrelated := filepath.Join(t.TempDir(), "unrelated")
	runGit(t, fx.Clone, "worktree", "add", "--detach", unrelated, "develop")
	if err := os.RemoveAll(unrelated); err != nil {
		t.Fatal(err)
	}
	before := gitOutput(t, fx.Clone, "worktree", "list", "--porcelain")
	if !strings.Contains(before, "/unrelated\n") {
		t.Fatalf("unrelated stale worktree missing before cleanup:\n%s", before)
	}

	notRegistered := filepath.Join(t.TempDir(), "not-registered")
	if err := os.MkdirAll(notRegistered, 0o755); err != nil {
		t.Fatal(err)
	}
	g := newGitRepo(gitcmd.NewExecutor(), fx.Worktree)
	if err := mergeCleanupError(context.Background(), nil, g, notRegistered); err == nil {
		t.Fatal("unregistered cleanup path unexpectedly succeeded")
	}
	after := gitOutput(t, fx.Clone, "worktree", "list", "--porcelain")
	if !strings.Contains(after, "/unrelated\n") {
		t.Fatalf("readiness cleanup pruned unrelated metadata:\n%s", after)
	}
	if _, err := os.Stat(notRegistered); !os.IsNotExist(err) {
		t.Fatalf("exact failed cleanup path remains: %v", err)
	}
	runGit(t, fx.Clone, "worktree", "prune", "--expire", "now")
}

func TestExecuteReadiness_NonzeroAndOversizeFailUnavailable(t *testing.T) {
	dir := t.TempDir()
	for _, tt := range []struct{ name, body string }{
		{"nonzero", "#!/bin/sh\necho bad >&2\nexit 7\n"},
		{"stdout", "#!/bin/sh\nyes x | head -c 1048577\n"},
		{"stderr", "#!/bin/sh\nyes x | head -c 1048577 >&2\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := filepath.Join(dir, tt.name)
			if err := os.WriteFile(runner, []byte(tt.body), 0o755); err != nil {
				t.Fatal(err)
			}
			if _, err := executeReadinessWithTimeout(context.Background(), runner, dir, dir, "a", "b", time.Second); err == nil {
				t.Fatal("invalid runner accepted")
			}
		})
	}
}

func TestPushFastForward_StaleLeaseRejects(t *testing.T) {
	fx := readinessFixture(t)
	checked := gitOutput(t, fx.Worktree, "rev-parse", fx.Remote+"/develop")
	source := gitOutput(t, fx.Worktree, "rev-parse", "HEAD")
	runGit(t, fx.Clone, "checkout", "develop")
	writeFile(t, fx.Clone, "target-moved.txt", "moved\n")
	runGit(t, fx.Clone, "add", "target-moved.txt")
	runGit(t, fx.Clone, "commit", "-m", "target moves")
	moved := gitOutput(t, fx.Clone, "rev-parse", "HEAD")
	runGit(t, fx.Clone, "push", fx.Remote, "develop")
	err := pushFastForward(context.Background(), newGitRepo(gitcmd.NewExecutor(), fx.Worktree), fx.Remote, source, "develop", checked)
	if err == nil {
		t.Fatal("stale lease unexpectedly pushed")
	}
	if got := gitOutput(t, fx.Origin, "rev-parse", "refs/heads/develop"); got != moved {
		t.Fatalf("stale lease changed remote target: got %s, want moved %s", got, moved)
	}
}

func TestRunChecked_TargetMovedStopsBeforeIntegrationAndReclaim(t *testing.T) {
	fx := readinessFixture(t)
	opts := RunOptions{CheckOptions: CheckOptions{
		RepoPath:          fx.Worktree,
		Branch:            "dev/actor/feat/task",
		IntegrationConfig: []string{"develop"},
	}}
	check, err := Check(context.Background(), gitcmd.NewExecutor(), opts.CheckOptions)
	if err != nil || !check.Ready {
		t.Fatalf("initial readiness: %v\n%s", err, FormatCheck(check))
	}

	writeFile(t, fx.Clone, "target-moved-after-check.txt", "moved\n")
	runGit(t, fx.Clone, "add", "target-moved-after-check.txt")
	runGit(t, fx.Clone, "commit", "-m", "move target after readiness")
	moved := gitOutput(t, fx.Clone, "rev-parse", "HEAD")
	runGit(t, fx.Clone, "push", fx.Remote, "develop")

	report, err := runChecked(context.Background(), gitcmd.NewExecutor(), opts, check)
	if err == nil || !strings.Contains(err.Error(), "target branch changed") {
		t.Fatalf("want target-change failure, got report=%+v err=%v", report, err)
	}
	if report.Integrated || report.SHA != "" || len(report.Reclaim.Done) != 0 || len(report.Reclaim.Failed) != 0 {
		t.Fatalf("target change must not integrate or reclaim: %+v", report)
	}
	if _, err := os.Stat(fx.Worktree); err != nil {
		t.Fatalf("task worktree was reclaimed: %v", err)
	}
	if !refExists(t, fx.Clone, "refs/heads/dev/actor/feat/task") || !refExists(t, fx.Origin, "refs/heads/dev/actor/feat/task") {
		t.Fatal("task branch was reclaimed")
	}
	if got := gitOutput(t, fx.Origin, "rev-parse", "refs/heads/develop"); got != moved {
		t.Fatalf("target changed unexpectedly: got %s, want %s", got, moved)
	}
}

func readinessFixture(t *testing.T) *testutil.WorktreeOrigin {
	t.Helper()
	fx := testutil.TempWorktreeWithBareOrigin(t)
	runGit(t, fx.Clone, "branch", "develop")
	runGit(t, fx.Clone, "checkout", "develop")
	if err := os.MkdirAll(filepath.Join(fx.Clone, ".gz-git", "readiness"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, fx.Clone, ".gz-git.yaml", "branch:\n  integrationBranch: develop\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/check\n")
	writeRepoFile(t, fx.Clone, ".gz-git/readiness/check", "#!/bin/sh\nprintf '{\"version\":1,\"status\":\"ready\",\"summary\":\"target gate passed\"}'\n")
	if err := os.Chmod(filepath.Join(fx.Clone, ".gz-git", "readiness", "check"), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, fx.Clone, "add", ".")
	runGit(t, fx.Clone, "commit", "-m", "declare target readiness")
	runGit(t, fx.Clone, "push", "-u", fx.Remote, "develop")
	runGit(t, fx.Worktree, "checkout", "-B", "dev/actor/feat/task", "develop")
	writeFile(t, fx.Worktree, "task.txt", "task\n")
	runGit(t, fx.Worktree, "add", "task.txt")
	runGit(t, fx.Worktree, "commit", "-m", "task")
	runGit(t, fx.Worktree, "push", "-u", fx.Remote, "HEAD")
	return fx
}

func advanceReadinessTarget(t *testing.T, fx *testutil.WorktreeOrigin, manifest, runner, message string) {
	t.Helper()
	if manifest != "" {
		writeRepoFile(t, fx.Clone, ".gz-git.yaml", manifest)
	}
	if runner != "" {
		writeRepoFile(t, fx.Clone, ".gz-git/readiness/check", runner)
		if err := os.Chmod(filepath.Join(fx.Clone, ".gz-git", "readiness", "check"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, fx.Clone, "add", ".")
	runGit(t, fx.Clone, "commit", "-m", message)
	runGit(t, fx.Clone, "push", fx.Remote, "develop")
	runGit(t, fx.Worktree, "rebase", "develop")
	runGit(t, fx.Worktree, "push", "--force-with-lease", fx.Remote, "HEAD")
}
