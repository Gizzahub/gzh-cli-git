// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package workspacecli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/reposync"
)

func TestCollectPushTargetsPreservesReadOnlyContract(t *testing.T) {
	result := reposync.ExecutionResult{Succeeded: []reposync.ActionResult{{
		Action: reposync.Action{
			Repo:     reposync.RepoSpec{TargetPath: "/external/repo"},
			ReadOnly: true,
		},
	}}}

	targets := collectPushTargets(result)
	if len(targets) != 1 || !targets[0].ReadOnly {
		t.Fatalf("targets = %#v, want one read-only target", targets)
	}
}

func TestRecordReadOnlyWorkspaceAccessPersistsExternalContract(t *testing.T) {
	repoPath := t.TempDir()
	cmd := exec.CommandContext(context.Background(), "git", "-C", repoPath, "init")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", output, err)
	}
	result := reposync.ExecutionResult{Succeeded: []reposync.ActionResult{{
		Action: reposync.Action{Repo: reposync.RepoSpec{TargetPath: repoPath}, ReadOnly: true},
	}}}

	if err := recordReadOnlyWorkspaceAccess(context.Background(), result, ""); err != nil {
		t.Fatal(err)
	}
	marker := exec.CommandContext(context.Background(), "git", "-C", repoPath, "config", "--local", "--get", "gz-git.workspaceAccess")
	output, err := marker.Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := string(output); got != "read-only\n" {
		t.Fatalf("marker = %q, want read-only", got)
	}

	result.Succeeded[0].Action.ReadOnly = false
	if err := recordReadOnlyWorkspaceAccess(context.Background(), result, ""); err != nil {
		t.Fatal(err)
	}
	marker = exec.CommandContext(context.Background(), "git", "-C", repoPath, "config", "--local", "--get", "gz-git.workspaceAccess")
	if err := marker.Run(); err == nil {
		t.Fatal("read-only marker still exists after successful read-write sync")
	}
}

func TestPushOneRepoSkipsReadOnlyBeforeGitAccess(t *testing.T) {
	result := pushOneRepo(context.Background(), pushTarget{Path: "/path/that/does/not/exist", ReadOnly: true})
	if result.Error != nil || result.Pushed || result.Message != "read-only workspace; push skipped" {
		t.Fatalf("result = %#v, want read-only skip", result)
	}
}

func TestReconcileIntegrationParticipationOnlyTouchesSuccessfulRepos(t *testing.T) {
	succeeded := initSyncParticipationRepo(t)
	failed := initSyncParticipationRepo(t)
	result := reposync.ExecutionResult{
		Succeeded: []reposync.ActionResult{{Action: reposync.Action{Type: reposync.ActionUpdate, Repo: reposync.RepoSpec{TargetPath: succeeded}}}},
		Failed:    []reposync.ActionResult{{Action: reposync.Action{Repo: reposync.RepoSpec{TargetPath: failed}}}},
	}
	if err := reconcileIntegrationParticipation(t.Context(), result, ""); err != nil {
		t.Fatal(err)
	}
	assertSyncGitConfig(t, succeeded, "workflow.integrationBranch", "master")
	assertSyncGitConfig(t, failed, "workflow.integrationBranch", "")
}

func TestReconcileIntegrationParticipationSkipsSuccessfulDelete(t *testing.T) {
	repo := initSyncParticipationRepo(t)
	result := reposync.ExecutionResult{Succeeded: []reposync.ActionResult{{Action: reposync.Action{Type: reposync.ActionDelete, Repo: reposync.RepoSpec{TargetPath: repo}}}}}
	if err := reconcileIntegrationParticipation(t.Context(), result, ""); err != nil {
		t.Fatal(err)
	}
	assertSyncGitConfig(t, repo, "workflow.integrationBranch", "")
}

func TestControllerWorkspaceExcludesRepoRootParticipation(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	if err := os.WriteFile(filepath.Join(fx.Clone, ".gz-git.yaml"), []byte("branch:\n  integrationBranch: [master]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := successfulWorkspaceSync(fx.Clone, "engine")
	result.Succeeded[0].Action.Type = reposync.ActionUpdate
	cfg := integrationWorkspaceConfig("engine", fx.Clone, fx.Origin, "release/1.27")

	txn, err := prepareWorkspaceIntegrationBranches(t.Context(), result, cfg, writeLocalIntegrationBranch)
	if err != nil {
		t.Fatal(err)
	}
	if err := reconcileIntegrationParticipationExcept(t.Context(), result, "", controllerIntegrationWorkspaces(cfg)); err != nil {
		t.Fatal(err)
	}
	if err := txn.apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertSyncGitConfig(t, fx.Clone, "workflow.integrationBranch", "release/1.27")
	assertSyncGitConfig(t, fx.Clone, "gz-git.managedWorkflowIntegrationBranch", "")
}

func TestMixedControllerAndRepoRootParticipation(t *testing.T) {
	controller := testutil.TempWorktreeWithBareOrigin(t)
	legacy := initSyncParticipationRepo(t)
	result := reposync.ExecutionResult{Succeeded: []reposync.ActionResult{
		{Action: reposync.Action{Type: reposync.ActionUpdate, Workspace: "engine", Repo: reposync.RepoSpec{TargetPath: controller.Clone}}},
		{Action: reposync.Action{Type: reposync.ActionUpdate, Workspace: "legacy", Repo: reposync.RepoSpec{TargetPath: legacy}}},
	}}
	cfg := integrationWorkspaceConfig("engine", controller.Clone, controller.Origin, "release/1.27")

	txn, err := prepareWorkspaceIntegrationBranches(t.Context(), result, cfg, writeLocalIntegrationBranch)
	if err != nil {
		t.Fatal(err)
	}
	if err := reconcileIntegrationParticipationExcept(t.Context(), result, "", controllerIntegrationWorkspaces(cfg)); err != nil {
		t.Fatal(err)
	}
	if err := txn.apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertSyncGitConfig(t, controller.Clone, "workflow.integrationBranch", "release/1.27")
	assertSyncGitConfig(t, controller.Clone, "gz-git.managedWorkflowIntegrationBranch", "")
	assertSyncGitConfig(t, legacy, "workflow.integrationBranch", "master")
	assertSyncGitConfig(t, legacy, "gz-git.managedWorkflowIntegrationBranch", "master")
}

func TestControllerParticipationDoesNotFallbackOnPartialFailure(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	if err := os.WriteFile(filepath.Join(fx.Clone, ".gz-git.yaml"), []byte("branch:\n  integrationBranch: [master]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := successfulWorkspaceSync(fx.Clone, "engine")
	result.Succeeded[0].Action.Type = reposync.ActionUpdate
	result.Failed = []reposync.ActionResult{{Action: reposync.Action{Workspace: "other"}}}
	cfg := integrationWorkspaceConfig("engine", fx.Clone, fx.Origin, "release/1.27")

	txn, err := prepareWorkspaceIntegrationBranches(t.Context(), result, cfg, writeLocalIntegrationBranch)
	if err != nil {
		t.Fatal(err)
	}
	if err := reconcileIntegrationParticipationExcept(t.Context(), result, "", controllerIntegrationWorkspaces(cfg)); err != nil {
		t.Fatal(err)
	}
	if err := txn.apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertSyncGitConfig(t, fx.Clone, "workflow.integrationBranch", "")
	assertSyncGitConfig(t, fx.Clone, "gz-git.managedWorkflowIntegrationBranch", "")
}

func TestRepoRootParticipationFailurePreventsControllerApply(t *testing.T) {
	controller := testutil.TempWorktreeWithBareOrigin(t)
	legacy := initSyncParticipationRepo(t)
	if err := os.WriteFile(filepath.Join(legacy, ".gz-git.yaml"), []byte("branch:\n  integrationBranch: [missing]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := reposync.ExecutionResult{Succeeded: []reposync.ActionResult{
		{Action: reposync.Action{Type: reposync.ActionUpdate, Workspace: "engine", Repo: reposync.RepoSpec{TargetPath: controller.Clone}}},
		{Action: reposync.Action{Type: reposync.ActionUpdate, Workspace: "legacy", Repo: reposync.RepoSpec{TargetPath: legacy}}},
	}}
	cfg := integrationWorkspaceConfig("engine", controller.Clone, controller.Origin, "release/1.27")

	txn, err := prepareWorkspaceIntegrationBranches(t.Context(), result, cfg, writeLocalIntegrationBranch)
	if err != nil {
		t.Fatal(err)
	}
	if err := reconcileIntegrationParticipationExcept(t.Context(), result, "", controllerIntegrationWorkspaces(cfg)); err == nil {
		t.Fatal("invalid repo-root participation unexpectedly succeeded")
	}
	assertSyncGitConfig(t, controller.Clone, "workflow.integrationBranch", "")
	if len(txn.settings) != 1 {
		t.Fatalf("prepared controller settings = %d, want 1", len(txn.settings))
	}
}

func initSyncParticipationRepo(t *testing.T) string {
	t.Helper()
	const branch = "master"
	repo := t.TempDir()
	cmd := exec.CommandContext(t.Context(), "git", "-C", repo, "init", "--initial-branch="+branch) //nolint:gosec // test-controlled branch.
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", output, err)
	}
	for _, args := range [][]string{{"config", "user.email", "test@example.com"}, {"config", "user.name", "Test"}} {
		cmd = exec.CommandContext(t.Context(), "git", append([]string{"-C", repo}, args...)...) //nolint:gosec // fixed test Git commands.
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, output, err)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "README"}, {"commit", "-m", "initial"}} {
		cmd = exec.CommandContext(t.Context(), "git", append([]string{"-C", repo}, args...)...) //nolint:gosec // fixed test Git commands.
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, output, err)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, ".gz-git.yaml"), []byte("branch:\n  integrationBranch: ["+branch+"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return repo
}

func assertSyncGitConfig(t *testing.T, repo, key, want string) {
	t.Helper()
	output, err := exec.CommandContext(t.Context(), "git", "-C", repo, "config", "--local", "--get", key).Output() //nolint:gosec // fixed test Git command.
	if want == "" {
		if err == nil {
			t.Fatalf("git config %s = %q, want absent", key, output)
		}
		return
	}
	if err != nil || string(output) != want+"\n" {
		t.Fatalf("git config %s = %q, %v; want %q", key, output, err, want+"\n")
	}
}
