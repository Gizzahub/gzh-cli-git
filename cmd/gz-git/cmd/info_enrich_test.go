// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/branch"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

func TestEnrichOne_UsesRepoRootIntegrationBranchForUpstreamSafety(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOriginRemote(t, "team/upstream")
	integration := "release/2.0"
	runGit(t, fx.Clone, "branch", integration)
	runGit(t, fx.Clone, "push", fx.Remote, integration)
	runGit(t, fx.Worktree, "checkout", "-B", "dev/actor/feat/task", fx.Remote+"/"+integration)

	configBody := "branch:\n  defaultBranch: main\n  integrationBranch: " + integration + "\n  taskPattern: dev/*/*/*\n"
	if err := os.WriteFile(filepath.Join(fx.Worktree, ".gz-git.yaml"), []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	runGit(t, fx.Worktree, "add", ".gz-git.yaml")
	runGit(t, fx.Worktree, "commit", "-m", "declare branch policy")

	status := repository.RepositoryStatusResult{
		Path:         fx.Worktree,
		Branch:       "dev/actor/feat/task",
		Upstream:     fx.Remote + "/" + integration,
		Remote:       fx.Remote,
		Remotes:      map[string]string{fx.Remote: fx.Origin},
		CommitsAhead: 1,
	}
	got := enrichOne(
		context.Background(), repository.NewClient(), branch.NewWorktreeManager(),
		status, []string{"main"}, true,
	)

	if got.Err != nil {
		t.Fatalf("enrichOne: %v", got.Err)
	}
	if got.Base.Name != "main" {
		t.Fatalf("base = %q, want existing defaultBranch main", got.Base.Name)
	}
	if got.Integration.Name != integration || got.Integration.Source != "config[0]" {
		t.Fatalf("integration = %+v, want configured %s", got.Integration, integration)
	}
	if !got.UpstreamTargetsIntegration {
		t.Fatal("configured integration upstream was not detected")
	}
	if got.UpstreamRemote != fx.Remote {
		t.Fatalf("upstream remote = %q", got.UpstreamRemote)
	}
}

func TestDeclaredTaskBranch(t *testing.T) {
	for _, tt := range []struct {
		name     string
		branch   string
		patterns []string
		want     bool
	}{
		{name: "matching declaration", branch: "dev/a/b/c", patterns: []string{"dev/*/*/*"}, want: true},
		{name: "nonmatching declaration", branch: "feature/x", patterns: []string{"dev/*/*/*"}},
		{name: "undeclared namespace", branch: "dev/a/b/c"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDeclaredTaskBranch(tt.branch, tt.patterns); got != tt.want {
				t.Fatalf("isDeclaredTaskBranch() = %v, want %v", got, tt.want)
			}
		})
	}
}
