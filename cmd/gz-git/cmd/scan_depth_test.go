// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

func scanDepthCommand(t *testing.T, value int) (*cobra.Command, *int) {
	t.Helper()
	cmd := &cobra.Command{Use: "scan"}
	depth := value
	cmd.Flags().IntVarP(&depth, "scan-depth", "d", value, "scan depth")
	return cmd, &depth
}

func TestResolveBulkDepthUsesConfiguredDepth(t *testing.T) {
	dir := writeScanConfig(t, "defaults:\n  scan:\n    depth: 3\n")
	cmd, depth := scanDepthCommand(t, repository.DefaultLocalScanDepth)

	if err := resolveBulkDepth(cmd, dir, depth); err != nil {
		t.Fatalf("resolveBulkDepth: %v", err)
	}
	if *depth != 3 {
		t.Fatalf("depth = %d, want configured depth 3", *depth)
	}
}

func TestResolveBulkDepthExplicitFlagWins(t *testing.T) {
	dir := writeScanConfig(t, "defaults:\n  scan:\n    depth: 5\n")
	cmd, depth := scanDepthCommand(t, repository.DefaultLocalScanDepth)
	if err := cmd.Flags().Set("scan-depth", "2"); err != nil {
		t.Fatalf("set scan-depth: %v", err)
	}

	if err := resolveBulkDepth(cmd, dir, depth); err != nil {
		t.Fatalf("resolveBulkDepth: %v", err)
	}
	if *depth != 2 {
		t.Fatalf("depth = %d, want explicit flag depth 2", *depth)
	}
}

func TestResolveBulkDepthInheritsParentConfig(t *testing.T) {
	root := t.TempDir()
	parentDir := filepath.Join(root, "parent")
	childDir := filepath.Join(root, "child")
	for _, dir := range []string{parentDir, childDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(parentDir, ".gz-git.yaml"),
		[]byte("defaults:\n  scan:\n    depth: 4\n"), 0o600); err != nil {
		t.Fatalf("write parent config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(childDir, ".gz-git.yaml"),
		[]byte("parent: ../parent/.gz-git.yaml\n"), 0o600); err != nil {
		t.Fatalf("write child config: %v", err)
	}

	cmd, depth := scanDepthCommand(t, repository.DefaultLocalScanDepth)
	if err := resolveBulkDepth(cmd, childDir, depth); err != nil {
		t.Fatalf("resolveBulkDepth: %v", err)
	}
	if *depth != 4 {
		t.Fatalf("depth = %d, want inherited depth 4", *depth)
	}
}

func TestResolveBulkDepthChangesActualScanReach(t *testing.T) {
	root := writeScanConfig(t, "defaults:\n  scan:\n    depth: 2\n")
	repoDir := filepath.Join(root, "group", "repo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o750); err != nil {
		t.Fatalf("create nested repository: %v", err)
	}

	cmd, depth := scanDepthCommand(t, repository.DefaultLocalScanDepth)
	if err := resolveBulkDepth(cmd, root, depth); err != nil {
		t.Fatalf("resolveBulkDepth: %v", err)
	}
	result, err := repository.NewClient().ScanRepositories(t.Context(), repository.ScanOptions{
		Directory: root,
		MaxDepth:  *depth,
	})
	if err != nil {
		t.Fatalf("scan repositories: %v", err)
	}
	if len(result.Paths) != 1 || result.Paths[0] != repoDir {
		t.Fatalf("scan paths = %v, want [%s]", result.Paths, repoDir)
	}
}

func TestResolveBulkDepthRejectsInvalidConfiguredDepth(t *testing.T) {
	dir := writeScanConfig(t, "defaults:\n  scan:\n    depth: -1\n")
	cmd, depth := scanDepthCommand(t, repository.DefaultLocalScanDepth)

	if err := resolveBulkDepth(cmd, dir, depth); err == nil {
		t.Fatal("resolveBulkDepth accepted a negative configured depth")
	}
}
