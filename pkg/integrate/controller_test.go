// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)

func TestResolveControllerRequiresUniqueCanonicalWorkspaceMatch(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	remoteOut := runGitInTest(t, fx.Worktree, "remote", "get-url", "origin")
	remoteURL := strings.TrimSpace(remoteOut)
	path := filepath.Join(t.TempDir(), "devbox.yaml")
	writeFile(t, filepath.Dir(path), filepath.Base(path), fmt.Sprintf("workspaces:\n  engine:\n    url: %q\n    branch:\n      integrationBranch: develop\n      taskPattern: dev/*\n    integration:\n      prepareProfile: familybook-ent-v1\n", remoteURL))
	g := newGitRepo(gitcmd.NewExecutor(), fx.Worktree)
	b, err := resolveController(context.Background(), g, path, "feature/worktree")
	if err != nil {
		t.Fatalf("resolveController: %v", err)
	}
	if b.Workspace != "engine" || b.Integration[0] != "develop" || b.PrepareProfile != familybookEntPrepareV1 {
		t.Fatalf("unexpected binding: %#v", b)
	}
	if got := targetBranchName("origin/develop", b.Remote); got != "develop" {
		t.Fatalf("qualified controller target = %q", got)
	}

	writeFile(t, filepath.Dir(path), filepath.Base(path), fmt.Sprintf("workspaces:\n  one: {url: %q, branch: {integrationBranch: develop}}\n  two: {url: %q, branch: {integrationBranch: develop}}\n", remoteURL, remoteURL))
	if _, err := resolveController(context.Background(), g, path, "feature/worktree"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous controller = %v", err)
	}
	writeFile(t, filepath.Dir(path), filepath.Base(path), "workspaces:\n  other: {url: https://example.invalid/other.git, branch: {integrationBranch: develop}}\n")
	if _, err := resolveController(context.Background(), g, path, "feature/worktree"); err == nil || !strings.Contains(err.Error(), "no workspace matching") {
		t.Fatalf("mismatch controller = %v", err)
	}
	// Controller identity follows the effective push endpoint, not fetch URL;
	// multiple push URLs fail closed before any integration can start.
	runGitInTest(t, fx.Worktree, "remote", "set-url", "--add", "--push", "origin", remoteURL)
	runGitInTest(t, fx.Worktree, "remote", "set-url", "--add", "--push", "origin", remoteURL+"-other")
	if _, err := resolveController(context.Background(), g, path, "feature/worktree"); err == nil || !strings.Contains(err.Error(), "ambiguous push endpoint") {
		t.Fatalf("ambiguous push endpoint = %v", err)
	}
}

func TestResolveControllerRejectsMatchedWorkspaceWithoutIntegrationPolicy(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	remoteURL := strings.TrimSpace(runGitInTest(t, fx.Worktree, "remote", "get-url", "origin"))
	path := filepath.Join(t.TempDir(), "devbox.yaml")
	writeFile(t, filepath.Dir(path), filepath.Base(path), fmt.Sprintf("workspaces:\n  engine: {url: %q, branch: develop}\n", remoteURL))
	_, err := resolveController(context.Background(), newGitRepo(gitcmd.NewExecutor(), fx.Worktree), path, "feature/worktree")
	if err == nil || !strings.Contains(err.Error(), "no integration policy") {
		t.Fatalf("missing integration policy = %v", err)
	}
}

func TestResolveControllerRejectsDifferentFetchAndPushEndpoints(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	remoteURL := strings.TrimSpace(runGitInTest(t, fx.Worktree, "remote", "get-url", "origin"))
	path := filepath.Join(t.TempDir(), "devbox.yaml")
	writeFile(t, filepath.Dir(path), filepath.Base(path), fmt.Sprintf("workspaces:\n  engine: {url: %q, branch: {integrationBranch: develop}, integration: {prepareProfile: familybook-ent-v1}}\n", remoteURL+"-push"))
	runGitInTest(t, fx.Worktree, "remote", "set-url", "--push", "origin", remoteURL+"-push")
	_, err := resolveController(context.Background(), newGitRepo(gitcmd.NewExecutor(), fx.Worktree), path, "feature/worktree")
	if err == nil || !strings.Contains(err.Error(), "different fetch and push endpoints") {
		t.Fatalf("different fetch/push endpoints = %v", err)
	}
}

func TestRevalidateControllerRejectsDigestRace(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	remote := strings.TrimSpace(runGitInTest(t, fx.Worktree, "remote", "get-url", "origin"))
	path := filepath.Join(t.TempDir(), "controller.yaml")
	writeFile(t, filepath.Dir(path), filepath.Base(path), fmt.Sprintf("workspaces:\n  engine: {url: %q, branch: {integrationBranch: develop}, integration: {prepareProfile: familybook-ent-v1}}\n", remote))
	g := newGitRepo(gitcmd.NewExecutor(), fx.Worktree)
	b, err := resolveController(context.Background(), g, path, "feature/worktree")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Dir(path), filepath.Base(path), fmt.Sprintf("workspaces:\n  engine: {url: %q, branch: {integrationBranch: develop, taskPattern: dev/*}, integration: {prepareProfile: familybook-ent-v1}}\n", remote))
	if err := revalidateController(context.Background(), g, &CheckReport{Controller: b, Plan: TargetPlan{Branch: "feature/worktree"}}); err == nil || !strings.Contains(err.Error(), "changed during readiness") {
		t.Fatalf("race = %v", err)
	}
}
