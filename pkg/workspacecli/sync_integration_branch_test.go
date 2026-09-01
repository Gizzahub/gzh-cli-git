// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package workspacecli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/reposync"
)

func TestRecordWorkspaceIntegrationBranchesFreshExistingAndIdempotent(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	result := successfulWorkspaceSync(fx.Clone, "engine")
	cfg := integrationWorkspaceConfig("engine", fx.Clone, fx.Origin, "release/1.27")

	if err := recordWorkspaceIntegrationBranches(context.Background(), result, cfg); err != nil {
		t.Fatalf("record fresh workspace integration branch: %v", err)
	}
	if got := localGitConfig(t, fx.Clone, "workflow.integrationBranch"); got != "release/1.27" {
		t.Fatalf("fresh integration branch = %q, want release/1.27", got)
	}

	runGit(t, fx.Clone, "config", "--local", "workflow.integrationBranch", "obsolete")
	if err := recordWorkspaceIntegrationBranches(context.Background(), result, cfg); err != nil {
		t.Fatalf("record existing workspace integration branch: %v", err)
	}
	if got := localGitConfig(t, fx.Clone, "workflow.integrationBranch"); got != "release/1.27" {
		t.Fatalf("existing integration branch = %q, want release/1.27", got)
	}

	if err := recordWorkspaceIntegrationBranches(context.Background(), result, cfg); err != nil {
		t.Fatalf("record idempotent workspace integration branch: %v", err)
	}
	if got := localGitConfig(t, fx.Clone, "workflow.integrationBranch"); got != "release/1.27" {
		t.Fatalf("idempotent integration branch = %q, want release/1.27", got)
	}
}

func TestRecordWorkspaceIntegrationBranchesSkipsNonOptInAndFailedActions(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	cfg := integrationWorkspaceConfig("engine", fx.Clone, fx.Origin, "develop")
	cfg.Workspaces["engine"].Integration = nil
	if err := recordWorkspaceIntegrationBranches(context.Background(), successfulWorkspaceSync(fx.Clone, "engine"), cfg); err != nil {
		t.Fatalf("non-opt-in workspace: %v", err)
	}
	if _, err := gitOutputE(fx.Clone, "config", "--local", "--get", "workflow.integrationBranch"); err == nil {
		t.Fatal("non-opt-in workspace wrote workflow.integrationBranch")
	}

	cfg.Workspaces["engine"].Integration = &config.IntegrationControl{}
	partial := successfulWorkspaceSync(fx.Clone, "engine")
	partial.Failed = []reposync.ActionResult{{Action: reposync.Action{Workspace: "other", Repo: reposync.RepoSpec{TargetPath: t.TempDir()}}}}
	if err := recordWorkspaceIntegrationBranches(context.Background(), partial, cfg); err != nil {
		t.Fatalf("partially failed action: %v", err)
	}
	if _, err := gitOutputE(fx.Clone, "config", "--local", "--get", "workflow.integrationBranch"); err == nil {
		t.Fatal("partially failed sync wrote workflow.integrationBranch")
	}
}

func TestRecordWorkspaceIntegrationBranchesRejectsSameNameWrongTarget(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	cfg := integrationWorkspaceConfig("engine", fx.Clone, fx.Origin, "develop")

	if err := recordWorkspaceIntegrationBranches(context.Background(), successfulWorkspaceSync(fx.Worktree, "engine"), cfg); err == nil {
		t.Fatal("same-name wrong checkout unexpectedly wrote integration branch")
	}
	if _, err := gitOutputE(fx.Clone, "config", "--local", "--get", "workflow.integrationBranch"); err == nil {
		t.Fatal("configured checkout was mutated for wrong action target")
	}
	if _, err := gitOutputE(fx.Worktree, "config", "--local", "--get", "workflow.integrationBranch"); err == nil {
		t.Fatal("wrong action target was mutated")
	}
}

func TestRecordWorkspaceIntegrationBranchesRejectsOriginMismatchWithoutMutation(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	runGit(t, fx.Clone, "remote", "set-url", "--push", "origin", fx.Origin+"-other")
	cfg := integrationWorkspaceConfig("engine", fx.Clone, fx.Origin, "develop")

	if err := recordWorkspaceIntegrationBranches(context.Background(), successfulWorkspaceSync(fx.Clone, "engine"), cfg); err == nil {
		t.Fatal("origin push mismatch unexpectedly wrote integration branch")
	}
	if _, err := gitOutputE(fx.Clone, "config", "--local", "--get", "workflow.integrationBranch"); err == nil {
		t.Fatal("mismatched origin wrote workflow.integrationBranch")
	}
}

func TestRecordWorkspaceIntegrationBranchesRejectsRepoRootManagedMarker(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	runGit(t, fx.Clone, "config", "--local", "workflow.integrationBranch", "master")
	runGit(t, fx.Clone, "config", "--local", "gz-git.managedWorkflowIntegrationBranch", "master")
	cfg := integrationWorkspaceConfig("engine", fx.Clone, fx.Origin, "release/1.27")

	if err := recordWorkspaceIntegrationBranches(context.Background(), successfulWorkspaceSync(fx.Clone, "engine"), cfg); err == nil {
		t.Fatal("repo-root managed marker was silently taken over by controller")
	}
	if got := localGitConfig(t, fx.Clone, "workflow.integrationBranch"); got != "master" {
		t.Fatalf("workflow integration branch = %q, want preserved master", got)
	}
	if got := localGitConfig(t, fx.Clone, "gz-git.managedWorkflowIntegrationBranch"); got != "master" {
		t.Fatalf("managed marker = %q, want preserved master", got)
	}
}

func TestRecordWorkspaceIntegrationBranchesRejectsAmbiguousRawOriginURLs(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	runGit(t, fx.Clone, "config", "--local", "--add", "remote.origin.url", fx.Origin+"-second")
	cfg := integrationWorkspaceConfig("engine", fx.Clone, fx.Origin, "develop")

	if err := recordWorkspaceIntegrationBranches(context.Background(), successfulWorkspaceSync(fx.Clone, "engine"), cfg); err == nil {
		t.Fatal("ambiguous raw origin URLs unexpectedly wrote integration branch")
	}
	if _, err := gitOutputE(fx.Clone, "config", "--local", "--get", "workflow.integrationBranch"); err == nil {
		t.Fatal("ambiguous raw origin URLs wrote workflow.integrationBranch")
	}
}

func TestRecordWorkspaceIntegrationBranchesRejectsRawURLRewritesFromAllScopes(t *testing.T) {
	for _, scope := range []string{"local", "global", "include"} {
		t.Run(scope, func(t *testing.T) {
			global := filepath.Join(t.TempDir(), "global.gitconfig")
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			t.Setenv("GIT_CONFIG_GLOBAL", global)
			fx := testutil.TempWorktreeWithBareOrigin(t)
			prefix := filepath.Dir(fx.Origin) + string(filepath.Separator)
			switch scope {
			case "local":
				runGit(t, fx.Clone, "config", "--local", "url.ssh://rewrite.invalid/.insteadOf", prefix)
			case "global":
				runGit(t, fx.Clone, "config", "--global", "url.ssh://rewrite.invalid/.insteadOf", prefix)
			case "include":
				include := filepath.Join(t.TempDir(), "included.gitconfig")
				body := fmt.Sprintf("[url \"ssh://rewrite.invalid/\"]\n\tinsteadOf = %s\n", prefix)
				if err := os.WriteFile(include, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
				runGit(t, fx.Clone, "config", "--local", "include.path", include)
			}

			cfg := integrationWorkspaceConfig("engine", fx.Clone, fx.Origin, "develop")
			if err := recordWorkspaceIntegrationBranches(context.Background(), successfulWorkspaceSync(fx.Clone, "engine"), cfg); err == nil {
				t.Fatalf("%s URL rewrite unexpectedly wrote integration branch", scope)
			}
			if _, err := gitOutputE(fx.Clone, "config", "--local", "--get", "workflow.integrationBranch"); err == nil {
				t.Fatalf("%s URL rewrite wrote workflow.integrationBranch", scope)
			}
		})
	}
}

func TestRecordWorkspaceIntegrationBranchesRejectsPushInsteadOfRewrite(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	prefix := filepath.Dir(fx.Origin) + string(filepath.Separator)
	runGit(t, fx.Clone, "config", "--local", "url.ssh://push-rewrite.invalid/.pushInsteadOf", prefix)
	cfg := integrationWorkspaceConfig("engine", fx.Clone, fx.Origin, "develop")

	if err := recordWorkspaceIntegrationBranches(context.Background(), successfulWorkspaceSync(fx.Clone, "engine"), cfg); err == nil {
		t.Fatal("pushInsteadOf rewrite unexpectedly wrote integration branch")
	}
	if _, err := gitOutputE(fx.Clone, "config", "--local", "--get", "workflow.integrationBranch"); err == nil {
		t.Fatal("pushInsteadOf rewrite wrote workflow.integrationBranch")
	}
}

func TestRecordWorkspaceIntegrationBranchesPreservesRawEndpointBytes(t *testing.T) {
	t.Run("percent-encoded rewrite prefix", func(t *testing.T) {
		fx := testutil.TempWorktreeWithBareOrigin(t)
		const raw = "https://example.invalid/repo%20name"
		runGit(t, fx.Clone, "config", "--local", "--replace-all", "remote.origin.url", raw)
		runGit(t, fx.Clone, "config", "--local", "url.ssh://rewrite.invalid/.insteadOf", "https://example.invalid/repo%20")
		cfg := integrationWorkspaceConfig("engine", fx.Clone, raw, "develop")
		if err := recordWorkspaceIntegrationBranches(context.Background(), successfulWorkspaceSync(fx.Clone, "engine"), cfg); err == nil {
			t.Fatal("percent-encoded rewrite unexpectedly wrote integration branch")
		}
	})

	for _, raw := range []string{"", "https://example.invalid/trailing-space "} {
		t.Run(fmt.Sprintf("reject raw %q", raw), func(t *testing.T) {
			fx := testutil.TempWorktreeWithBareOrigin(t)
			runGit(t, fx.Clone, "config", "--local", "--replace-all", "remote.origin.url", raw)
			cfg := integrationWorkspaceConfig("engine", fx.Clone, fx.Origin, "develop")
			if err := recordWorkspaceIntegrationBranches(context.Background(), successfulWorkspaceSync(fx.Clone, "engine"), cfg); err == nil {
				t.Fatal("invalid raw endpoint unexpectedly wrote integration branch")
			}
			if _, err := gitOutputE(fx.Clone, "config", "--local", "--get", "workflow.integrationBranch"); err == nil {
				t.Fatal("invalid raw endpoint wrote workflow.integrationBranch")
			}
		})
	}
}

func TestRecordWorkspaceIntegrationBranchesRollsBackPreviousLocalValues(t *testing.T) {
	first := testutil.TempWorktreeWithBareOrigin(t)
	second := testutil.TempWorktreeWithBareOrigin(t)
	runGit(t, first.Clone, "config", "--local", "--add", "workflow.integrationBranch", "before/one")
	runGit(t, first.Clone, "config", "--local", "--add", "workflow.integrationBranch", "before/two")
	cfg := &config.Config{Workspaces: map[string]*config.Workspace{
		"first":  integrationWorkspaceConfig("first", first.Clone, first.Origin, "release/1.27").Workspaces["first"],
		"second": integrationWorkspaceConfig("second", second.Clone, second.Origin, "release/1.27").Workspaces["second"],
	}}
	result := reposync.ExecutionResult{Succeeded: []reposync.ActionResult{
		successfulWorkspaceSync(first.Clone, "first").Succeeded[0],
		successfulWorkspaceSync(second.Clone, "second").Succeeded[0],
	}}
	secondPath, err := canonicalExistingWorkspacePath(second.Clone)
	if err != nil {
		t.Fatal(err)
	}
	writer := func(ctx context.Context, path, branch string) error {
		if path == secondPath {
			return errors.New("injected second-write failure")
		}
		return writeLocalIntegrationBranch(ctx, path, branch)
	}
	if err := recordWorkspaceIntegrationBranchesWithWriter(context.Background(), result, cfg, writer); err == nil {
		t.Fatal("injected write failure unexpectedly succeeded")
	}
	if got := localGitConfigValues(t, first.Clone, "workflow.integrationBranch"); strings.Join(got, ",") != "before/one,before/two" {
		t.Fatalf("first values after rollback = %q, want original multiple values", got)
	}
	if _, err := gitOutputE(second.Clone, "config", "--local", "--get", "workflow.integrationBranch"); err == nil {
		t.Fatal("originally unset second value was created despite rollback")
	}
}

func TestRecordWorkspaceIntegrationBranchesRollsBackCanceledMutatingWriter(t *testing.T) {
	first := testutil.TempWorktreeWithBareOrigin(t)
	second := testutil.TempWorktreeWithBareOrigin(t)
	runGit(t, first.Clone, "config", "--local", "workflow.integrationBranch", "before/first")
	runGit(t, second.Clone, "config", "--local", "workflow.integrationBranch", "before/second")
	cfg := &config.Config{Workspaces: map[string]*config.Workspace{
		"first":  integrationWorkspaceConfig("first", first.Clone, first.Origin, "release/1.27").Workspaces["first"],
		"second": integrationWorkspaceConfig("second", second.Clone, second.Origin, "release/1.27").Workspaces["second"],
	}}
	result := reposync.ExecutionResult{Succeeded: []reposync.ActionResult{
		successfulWorkspaceSync(first.Clone, "first").Succeeded[0],
		successfulWorkspaceSync(second.Clone, "second").Succeeded[0],
	}}
	secondPath, err := canonicalExistingWorkspacePath(second.Clone)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	writer := func(writeCtx context.Context, path, branch string) error {
		if err := writeLocalIntegrationBranch(writeCtx, path, branch); err != nil {
			return err
		}
		if path == secondPath {
			cancel()
			return errors.New("injected cancellation after mutation")
		}
		return nil
	}
	if err := recordWorkspaceIntegrationBranchesWithWriter(ctx, result, cfg, writer); err == nil {
		t.Fatal("cancelled mutating writer unexpectedly succeeded")
	}
	if got := localGitConfig(t, first.Clone, "workflow.integrationBranch"); got != "before/first" {
		t.Fatalf("first branch after cancelled rollback = %q", got)
	}
	if got := localGitConfig(t, second.Clone, "workflow.integrationBranch"); got != "before/second" {
		t.Fatalf("current branch after cancelled rollback = %q", got)
	}
}

func TestPrepareWorkspaceIntegrationBranchesDefersWriteUntilAfterAccessMarker(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	cfg := integrationWorkspaceConfig("engine", fx.Clone, fx.Origin, "develop")
	result := successfulWorkspaceSync(fx.Clone, "engine")
	txn, err := prepareWorkspaceIntegrationBranches(context.Background(), result, cfg, writeLocalIntegrationBranch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gitOutputE(fx.Clone, "config", "--local", "--get", "workflow.integrationBranch"); err == nil {
		t.Fatal("prepare wrote workflow.integrationBranch")
	}

	markerFailure := reposync.ExecutionResult{Succeeded: []reposync.ActionResult{{Action: reposync.Action{
		Repo:     reposync.RepoSpec{TargetPath: t.TempDir()},
		ReadOnly: true,
	}}}}
	if err := recordReadOnlyWorkspaceAccess(context.Background(), markerFailure, ""); err == nil {
		t.Fatal("expected access-marker failure for non-Git target")
	}
	if _, err := gitOutputE(fx.Clone, "config", "--local", "--get", "workflow.integrationBranch"); err == nil {
		t.Fatal("integration branch remained after later access-marker failure")
	}
	if len(txn.settings) != 1 {
		t.Fatalf("prepared settings = %d, want 1", len(txn.settings))
	}
}

func TestWorkspaceSyncDryRunDoesNotRecordIntegrationBranch(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, DefaultConfigFile)
	configBody := fmt.Sprintf("version: 1\nkind: workspace\nworkspaces:\n  engine:\n    type: git\n    path: %q\n    url: %q\n    branch:\n      integrationBranch: release/1.27\n    integration: {}\n", fx.Clone, fx.Origin)
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workspace)
	cmd := CommandFactory{}.newSyncCmd()
	output := new(bytes.Buffer)
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.SetArgs([]string{"--config", configPath, "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("workspace sync dry-run: %v\n%s", err, output.String())
	}
	if _, err := gitOutputE(fx.Clone, "config", "--local", "--get", "workflow.integrationBranch"); err == nil {
		t.Fatal("dry-run wrote workflow.integrationBranch")
	}
}

func TestWorkspaceSyncRecordsIntegrationBranchAfterSuccessfulClone(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	workspace := t.TempDir()
	target := filepath.Join(workspace, "engine")
	configPath := writeIntegrationWorkspaceConfig(t, workspace, target, fx.Origin, "release/1.27")
	t.Chdir(workspace)

	for run := 0; run < 2; run++ {
		cmd := CommandFactory{}.newSyncCmd()
		output := new(bytes.Buffer)
		cmd.SetOut(output)
		cmd.SetErr(output)
		cmd.SetArgs([]string{"--config", configPath})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("workspace sync run %d: %v\n%s", run+1, err, output.String())
		}
		if got := localGitConfig(t, target, "workflow.integrationBranch"); got != "release/1.27" {
			t.Fatalf("sync run %d integration branch = %q, want release/1.27", run+1, got)
		}
	}
}

func TestWorkspaceSyncRecordsIntegrationBranchForRelativeWorkspacePath(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, DefaultConfigFile)
	configBody := fmt.Sprintf("version: 1\nkind: workspace\nworkspaces:\n  engine:\n    type: git\n    path: ./engine\n    url: %q\n    branch:\n      integrationBranch: release/1.27\n    integration: {}\n", fx.Origin)
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workspace)
	cmd := CommandFactory{}.newSyncCmd()
	output := new(bytes.Buffer)
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.SetArgs([]string{"--config", configPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("workspace sync relative path: %v\n%s", err, output.String())
	}
	if got := localGitConfig(t, filepath.Join(workspace, "engine"), "workflow.integrationBranch"); got != "release/1.27" {
		t.Fatalf("relative workspace integration branch = %q", got)
	}
}

func successfulWorkspaceSync(path, workspace string) reposync.ExecutionResult {
	return reposync.ExecutionResult{Succeeded: []reposync.ActionResult{{Action: reposync.Action{
		Workspace: workspace,
		Repo:      reposync.RepoSpec{TargetPath: path},
	}}}}
}

func integrationWorkspaceConfig(name, path, remote, branch string) *config.Config {
	return &config.Config{Workspaces: map[string]*config.Workspace{name: {
		Path:        path,
		Type:        config.WorkspaceTypeGit,
		URL:         remote,
		Branch:      &config.BranchConfig{IntegrationBranch: config.BranchList{branch}},
		Integration: &config.IntegrationControl{},
	}}}
}

func writeIntegrationWorkspaceConfig(t *testing.T, workspace, target, remote, branch string) string {
	t.Helper()
	configPath := filepath.Join(workspace, DefaultConfigFile)
	configBody := fmt.Sprintf("version: 1\nkind: workspace\nworkspaces:\n  engine:\n    type: git\n    path: %q\n    url: %q\n    branch:\n      integrationBranch: %s\n    integration: {}\n", target, remote, branch)
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatal(err)
	}
	return configPath
}

func localGitConfig(t *testing.T, path, key string) string {
	t.Helper()
	output, err := gitOutputE(path, "config", "--local", "--get", key)
	if err != nil {
		t.Fatalf("read %s: %v", key, err)
	}
	return strings.TrimSpace(output)
}

func localGitConfigValues(t *testing.T, path, key string) []string {
	t.Helper()
	output, err := gitOutputE(path, "config", "--local", "-z", "--get-all", key)
	if err != nil {
		t.Fatalf("read %s: %v", key, err)
	}
	values, err := splitNULConfigValues([]byte(output))
	if err != nil {
		t.Fatalf("parse %s: %v", key, err)
	}
	return values
}

func runGit(t *testing.T, path string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...) // #nosec G204 -- test helper uses fixed git argv.
	cmd.Dir = path
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, output)
	}
}

func gitOutputE(path string, args ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "git", args...) // #nosec G204 -- test helper uses fixed git argv.
	cmd.Dir = path
	output, err := cmd.Output()
	return string(output), err
}
