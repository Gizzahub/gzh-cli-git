// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package workspacecli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/reposync"
)

func TestWorkspaceSyncStrategyReadOnlyAlwaysPulls(t *testing.T) {
	ws := &config.Workspace{
		Access: config.WorkspaceAccessReadOnly,
		Sync:   &config.SyncConfig{Strategy: "reset"},
	}
	got, err := workspaceSyncStrategy(ws, nil, reposync.StrategyReset)
	if err != nil {
		t.Fatal(err)
	}
	if got != reposync.StrategyPull {
		t.Fatalf("workspaceSyncStrategy() = %q, want %q", got, reposync.StrategyPull)
	}
}

func TestWorkspaceSyncStrategyUsesOverrideForWritableWorkspace(t *testing.T) {
	ws := &config.Workspace{Sync: &config.SyncConfig{Strategy: "pull"}}
	got, err := workspaceSyncStrategy(ws, nil, reposync.StrategyReset)
	if err != nil {
		t.Fatal(err)
	}
	if got != reposync.StrategyReset {
		t.Fatalf("workspaceSyncStrategy() = %q, want %q", got, reposync.StrategyReset)
	}
}

func TestWorkspaceSyncStrategyPriority(t *testing.T) {
	cfg := &config.Config{Defaults: &config.DefaultsConfig{Sync: &config.SyncDefaults{Strategy: "pull"}}}
	ws := &config.Workspace{Sync: &config.SyncConfig{Strategy: "rebase"}}

	for _, tc := range []struct {
		name      string
		workspace *config.Workspace
		override  reposync.Strategy
		want      reposync.Strategy
	}{
		{name: "CLI override", workspace: ws, override: reposync.StrategyFetch, want: reposync.StrategyFetch},
		{name: "workspace strategy", workspace: ws, want: reposync.StrategyRebase},
		{name: "root defaults strategy", workspace: &config.Workspace{}, want: reposync.StrategyPull},
		{name: "reset fallback", workspace: &config.Workspace{}, want: reposync.StrategyReset},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := cfg
			if tc.name == "reset fallback" {
				root = &config.Config{}
			}
			got, err := workspaceSyncStrategy(tc.workspace, root, tc.override)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("workspaceSyncStrategy() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWorkspaceSyncStrategyRejectsInvalidRootStrategy(t *testing.T) {
	cfg := &config.Config{Defaults: &config.DefaultsConfig{Sync: &config.SyncDefaults{Strategy: "not-a-strategy"}}}
	if _, err := workspaceSyncStrategy(&config.Workspace{}, cfg, ""); err == nil {
		t.Fatal("invalid root strategy unexpectedly fell back to reset")
	}
}

func TestWorkspaceSyncRootDefaultPullPreservesDevelopInsteadOfResettingOriginHEAD(t *testing.T) {
	root := t.TempDir()
	seed := filepath.Join(root, "seed")
	origin := filepath.Join(root, "origin.git")
	publisher := filepath.Join(root, "publisher")
	target := filepath.Join(root, "target")
	workspace := filepath.Join(root, "workspace")

	runRootStrategyGit(t, "", "init", "--initial-branch=master", seed)
	configureRootStrategyGit(t, seed)
	if err := os.WriteFile(filepath.Join(seed, "README"), []byte("master\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runRootStrategyGit(t, seed, "add", "README")
	runRootStrategyGit(t, seed, "commit", "-m", "master")
	runRootStrategyGit(t, "", "clone", "--bare", seed, origin)
	runRootStrategyGit(t, "", "--git-dir="+origin, "symbolic-ref", "HEAD", "refs/heads/master")
	runRootStrategyGit(t, "", "clone", origin, publisher)
	configureRootStrategyGit(t, publisher)
	runRootStrategyGit(t, publisher, "switch", "-c", "develop")
	if err := os.WriteFile(filepath.Join(publisher, "develop.txt"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runRootStrategyGit(t, publisher, "add", "develop.txt")
	runRootStrategyGit(t, publisher, "commit", "-m", "develop one")
	runRootStrategyGit(t, publisher, "push", "-u", "origin", "develop")

	runRootStrategyGit(t, "", "clone", origin, target)
	configureRootStrategyGit(t, target)
	runRootStrategyGit(t, target, "switch", "--track", "origin/develop")
	if err := os.WriteFile(filepath.Join(publisher, "develop.txt"), []byte("two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runRootStrategyGit(t, publisher, "add", "develop.txt")
	runRootStrategyGit(t, publisher, "commit", "-m", "develop two")
	runRootStrategyGit(t, publisher, "push", "origin", "develop")
	expected := rootStrategyGitOutput(t, publisher, "rev-parse", "HEAD")

	if err := os.MkdirAll(workspace, 0o750); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(workspace, DefaultConfigFile)
	configBody := fmt.Sprintf("version: 1\nkind: workspace\ndefaults:\n  sync:\n    strategy: pull\nworkspaces:\n  engine:\n    type: git\n    path: %q\n    url: %q\n", target, origin)
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
		t.Fatalf("workspace sync: %v\n%s", err, output.String())
	}
	if branch := rootStrategyGitOutput(t, target, "branch", "--show-current"); branch != "develop" {
		t.Fatalf("branch after root default pull = %q, want develop", branch)
	}
	if got := rootStrategyGitOutput(t, target, "rev-parse", "HEAD"); got != expected {
		t.Fatalf("develop HEAD after root default pull = %s, want %s", got, expected)
	}
}

func configureRootStrategyGit(t *testing.T, dir string) {
	t.Helper()
	runRootStrategyGit(t, dir, "config", "user.email", "test@example.com")
	runRootStrategyGit(t, dir, "config", "user.name", "Test")
	runRootStrategyGit(t, dir, "config", "commit.gpgsign", "false")
}

func runRootStrategyGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...) // #nosec G204 -- test fixtures use fixed Git argv.
	if dir != "" {
		cmd.Dir = dir
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, output)
	}
}

func rootStrategyGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...) // #nosec G204 -- test fixtures use fixed Git argv.
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(bytes.TrimSpace(output))
}
