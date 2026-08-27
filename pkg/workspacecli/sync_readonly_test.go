// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package workspacecli

import (
	"context"
	"os/exec"
	"testing"

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
