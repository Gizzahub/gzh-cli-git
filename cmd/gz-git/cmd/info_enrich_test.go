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
	fx := testutil.TempWorktreeWithBareOriginRemote(t, "upstream")
	runGit(t, fx.Clone, "branch", "develop")
	runGit(t, fx.Clone, "push", "upstream", "develop")
	runGit(t, fx.Worktree, "checkout", "-B", "dev/actor/feat/task", "upstream/develop")

	configBody := "branch:\n  defaultBranch: main\n  integrationBranch: develop\n  taskPattern: dev/*/*/*\n"
	if err := os.WriteFile(filepath.Join(fx.Worktree, ".gz-git.yaml"), []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	runGit(t, fx.Worktree, "add", ".gz-git.yaml")
	runGit(t, fx.Worktree, "commit", "-m", "declare branch policy")

	status := repository.RepositoryStatusResult{
		Path:         fx.Worktree,
		Branch:       "dev/actor/feat/task",
		Upstream:     "upstream/develop",
		Remote:       "upstream",
		Remotes:      map[string]string{"upstream": fx.Origin},
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
	if got.Integration.Name != "develop" || got.Integration.Source != "config[0]" {
		t.Fatalf("integration = %+v, want configured develop", got.Integration)
	}
	if !got.UpstreamTargetsIntegration {
		t.Fatal("configured integration upstream was not detected")
	}
	if got.UpstreamRemote != "upstream" {
		t.Fatalf("upstream remote = %q", got.UpstreamRemote)
	}
}
