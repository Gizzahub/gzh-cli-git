// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package workspacecli

import (
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/reposync"
)

func TestWorkspaceSyncStrategyReadOnlyAlwaysPulls(t *testing.T) {
	ws := &config.Workspace{
		Access: config.WorkspaceAccessReadOnly,
		Sync:   &config.SyncConfig{Strategy: "reset"},
	}
	if got := workspaceSyncStrategy(ws, reposync.StrategyReset); got != reposync.StrategyPull {
		t.Fatalf("workspaceSyncStrategy() = %q, want %q", got, reposync.StrategyPull)
	}
}

func TestWorkspaceSyncStrategyUsesOverrideForWritableWorkspace(t *testing.T) {
	ws := &config.Workspace{Sync: &config.SyncConfig{Strategy: "pull"}}
	if got := workspaceSyncStrategy(ws, reposync.StrategyReset); got != reposync.StrategyReset {
		t.Fatalf("workspaceSyncStrategy() = %q, want %q", got, reposync.StrategyReset)
	}
}
