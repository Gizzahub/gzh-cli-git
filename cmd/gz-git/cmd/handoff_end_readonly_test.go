// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/handoff"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

func TestApplyPushPolicyBlocksReadOnlyBeforeCheckpoint(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "foreign")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gz-git.yaml"), []byte(`version: "1.0"
workspaces:
  foreign:
    access: read-only
`), 0o644); err != nil {
		t.Fatal(err)
	}

	repos := []handoff.RepoAssessment{{Path: repoPath, RelativePath: "foreign", Branch: "main"}}
	allowed, blocked := applyPushPolicy(repos, &repository.PushPolicy{})
	if len(allowed) != 0 || len(blocked) != 1 {
		t.Fatalf("allowed/blocked = %d/%d, want 0/1", len(allowed), len(blocked))
	}
	if !strings.Contains(blocked[0].Reason, "read-only workspace") {
		t.Fatalf("reason = %q, want read-only workspace", blocked[0].Reason)
	}
}
