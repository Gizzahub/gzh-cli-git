// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspacePushAccessFindsParentDeclaration(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "tasuku-repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gz-git.yaml"), []byte(`version: "1.0"
workspaces:
  tasuku-repo:
    access: read-only
`), 0o644); err != nil {
		t.Fatal(err)
	}
	// A child config must not hide the parent workspace's access contract.
	if err := os.WriteFile(filepath.Join(repoPath, ".gz-git.yaml"), []byte("version: \"1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	access, source, err := WorkspacePushAccess(repoPath)
	if err != nil {
		t.Fatalf("WorkspacePushAccess() error = %v", err)
	}
	if access != WorkspaceAccessReadOnly {
		t.Fatalf("WorkspacePushAccess() access = %q, want %q", access, WorkspaceAccessReadOnly)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if source != filepath.Join(canonicalRoot, ".gz-git.yaml") {
		t.Fatalf("WorkspacePushAccess() source = %q", source)
	}
}

func TestWorkspacePushAccessRejectsInvalidMode(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "foreign")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gz-git.yaml"), []byte(`version: "1.0"
workspaces:
  foreign:
    path: foreign
    access: maybe
`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := WorkspacePushAccess(repoPath)
	if err == nil || !strings.Contains(err.Error(), "invalid access") {
		t.Fatalf("WorkspacePushAccess() error = %v, want invalid access", err)
	}
}

func TestWorkspacePushAccessProtectsDescendantRepository(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "vendor", "child")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gz-git.yaml"), []byte(`version: "1.0"
workspaces:
  vendor:
    access: read-only
`), 0o644); err != nil {
		t.Fatal(err)
	}

	access, _, err := WorkspacePushAccess(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	if !access.IsReadOnly() {
		t.Fatalf("access = %q, want read-only", access)
	}
}

func TestWorkspacePushAccessDiscoversOwnerThroughSymlink(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	linked := filepath.Join(root, "linked")
	if err := os.Symlink(external, linked); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gz-git.yaml"), []byte(`version: "1.0"
workspaces:
  linked:
    access: read-only
`), 0o644); err != nil {
		t.Fatal(err)
	}

	access, _, err := WorkspacePushAccess(linked)
	if err != nil {
		t.Fatal(err)
	}
	if !access.IsReadOnly() {
		t.Fatalf("access = %q, want read-only", access)
	}
}

func TestWorkspacePushAccessFollowsExternalChildParent(t *testing.T) {
	owner := t.TempDir()
	external := t.TempDir()
	workspacePath := filepath.Join(external, "vendor")
	repoPath := filepath.Join(workspacePath, "child")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	ownerConfig := filepath.Join(owner, ".gz-git.yaml")
	if err := os.WriteFile(ownerConfig, []byte("version: \"1.0\"\nworkspaces:\n  vendor:\n    path: "+workspacePath+"\n    access: read-only\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, ".gz-git.yaml"), []byte("version: \"1.0\"\nparent: "+ownerConfig+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	access, _, err := WorkspacePushAccess(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	if !access.IsReadOnly() {
		t.Fatalf("access = %q, want read-only through parent config", access)
	}
}

func TestWorkspacePushAccessUsesConfigLinkTargetBase(t *testing.T) {
	owner := t.TempDir()
	external := t.TempDir()
	workspacePath := filepath.Join(external, "vendor")
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		t.Fatal(err)
	}
	ownerConfig := filepath.Join(owner, ".gz-git.yaml")
	if err := os.WriteFile(ownerConfig, []byte("version: \"1.0\"\nworkspaces:\n  vendor:\n    path: "+workspacePath+"\n    access: read-only\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(ownerConfig, filepath.Join(workspacePath, ".gz-git.yaml")); err != nil {
		t.Fatal(err)
	}

	access, _, err := WorkspacePushAccess(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	if !access.IsReadOnly() {
		t.Fatalf("access = %q, want read-only through configLink", access)
	}
}

func TestWorkspacePushAccessReadOnlyWinsOverOverlappingReadWrite(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "vendor", "child")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gz-git.yaml"), []byte(`version: "1.0"
workspaces:
  ancestor:
    path: vendor
    access: read-only
  child:
    path: vendor/child
    access: read-write
`), 0o644); err != nil {
		t.Fatal(err)
	}

	for range 20 {
		access, _, err := WorkspacePushAccess(repoPath)
		if err != nil {
			t.Fatal(err)
		}
		if !access.IsReadOnly() {
			t.Fatalf("access = %q, want read-only to win", access)
		}
	}
}

func TestWorkspacePushAccessUsesRepositoryMarker(t *testing.T) {
	repoPath := t.TempDir()
	cmd := exec.CommandContext(context.Background(), "git", "-C", repoPath, "init")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", output, err)
	}
	if err := RecordWorkspaceReadOnly(context.Background(), repoPath); err != nil {
		t.Fatal(err)
	}

	access, source, err := WorkspacePushAccess(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	if !access.IsReadOnly() || !strings.HasSuffix(source, filepath.Join(".git", "config")) {
		t.Fatalf("access/source = %q/%q, want read-only local marker", access, source)
	}
}

func TestReadWriteMarkerCannotOverrideAncestorReadOnly(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "foreign")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(context.Background(), "git", "-C", repoPath, "init")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", output, err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gz-git.yaml"), []byte(`version: "1.0"
workspaces:
  foreign:
    access: read-only
`), 0o644); err != nil {
		t.Fatal(err)
	}
	marker := exec.CommandContext(context.Background(), "git", "-C", repoPath, "config", "--local", "gz-git.workspaceAccess", "read-write")
	if output, err := marker.CombinedOutput(); err != nil {
		t.Fatalf("set marker: %s: %v", output, err)
	}

	access, _, err := WorkspacePushAccess(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	if !access.IsReadOnly() {
		t.Fatalf("access = %q, want ancestor read-only to win", access)
	}
}
