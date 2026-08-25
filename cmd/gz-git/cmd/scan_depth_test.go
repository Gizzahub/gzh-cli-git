// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"strconv"
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

func TestResolveBulkDepthSupportsEveryProjectConfigExtension(t *testing.T) {
	for _, ext := range []string{".yaml", ".yml", ".json"} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			body := "defaults:\n  scan:\n    depth: 3\n"
			if ext == ".json" {
				body = `{"defaults":{"scan":{"depth":3}}}`
			}
			if err := os.WriteFile(filepath.Join(dir, ".gz-git"+ext), []byte(body), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cmd, depth := scanDepthCommand(t, repository.DefaultLocalScanDepth)
			if err := resolveBulkDepth(cmd, dir, depth); err != nil {
				t.Fatalf("resolveBulkDepth: %v", err)
			}
			if *depth != 3 {
				t.Fatalf("depth = %d, want configured depth 3", *depth)
			}
		})
	}
}

func TestResolveBulkDepthUsesConfigExtensionPriority(t *testing.T) {
	dir := t.TempDir()
	for name, depth := range map[string]int{
		".gz-git.yaml": 2,
		".gz-git.yml":  3,
		".gz-git.json": 4,
	} {
		body := []byte("defaults:\n  scan:\n    depth: " + strconv.Itoa(depth) + "\n")
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	cmd, depth := scanDepthCommand(t, repository.DefaultLocalScanDepth)
	if err := resolveBulkDepth(cmd, dir, depth); err != nil {
		t.Fatalf("resolveBulkDepth: %v", err)
	}
	if *depth != 2 {
		t.Fatalf("depth = %d, want .yaml priority depth 2", *depth)
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

func TestScanDepthFlagLifecycleResetsAfterEarlyError(t *testing.T) {
	validDir := t.TempDir()
	missingDir := filepath.Join(t.TempDir(), "missing")
	flags := BulkCommandFlags{}
	observed := 0
	cmd := &cobra.Command{
		Use:          "scan [directory]",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			directory, err := validateBulkDirectory(args)
			if err != nil {
				return err
			}
			if err := resolveBulkDepth(cmd, directory, &flags.Depth); err != nil {
				return err
			}
			observed = flags.Depth
			return nil
		},
	}
	addBulkFlags(cmd, &flags)

	cmd.SetArgs([]string{"--scan-depth", "7", missingDir})
	if err := cmd.Execute(); err == nil {
		t.Fatal("first Execute unexpectedly accepted a missing directory")
	}
	cmd.SetArgs([]string{validDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if observed != repository.DefaultLocalScanDepth {
		t.Fatalf("explicit depth leaked across Execute calls: got %d, want %d", observed, repository.DefaultLocalScanDepth)
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

func TestResolveBulkDepthResetsToFlagDefaultWithoutConfig(t *testing.T) {
	configuredDir := writeScanConfig(t, "defaults:\n  scan:\n    depth: 5\n")
	cmd, depth := scanDepthCommand(t, repository.DefaultLocalScanDepth)
	if err := resolveBulkDepth(cmd, configuredDir, depth); err != nil {
		t.Fatalf("resolve configured depth: %v", err)
	}
	if *depth != 5 {
		t.Fatalf("configured depth = %d, want 5", *depth)
	}

	if err := resolveBulkDepth(cmd, t.TempDir(), depth); err != nil {
		t.Fatalf("resolve default depth: %v", err)
	}
	if *depth != repository.DefaultLocalScanDepth {
		t.Fatalf("depth leaked from prior run: got %d, want %d", *depth, repository.DefaultLocalScanDepth)
	}
}

func TestResolveBulkDepthTreatsZeroAsOmitted(t *testing.T) {
	dir := writeScanConfig(t, "defaults:\n  scan:\n    depth: 0\n")
	cmd, depth := scanDepthCommand(t, repository.DefaultLocalScanDepth)

	if err := resolveBulkDepth(cmd, dir, depth); err != nil {
		t.Fatalf("resolveBulkDepth: %v", err)
	}
	if *depth != repository.DefaultLocalScanDepth {
		t.Fatalf("depth = %d, want CLI default %d", *depth, repository.DefaultLocalScanDepth)
	}
}

func TestTagAutoDoesNotAdvertiseBulkScanFlags(t *testing.T) {
	for _, name := range []string{"scan-depth", "parallel", "recursive", "include", "exclude"} {
		if flag := tagAutoCmd.Flags().Lookup(name); flag != nil {
			t.Errorf("tag auto unexpectedly exposes unused --%s", name)
		}
	}
	if flag := tagAutoCmd.Flags().Lookup("dry-run"); flag == nil {
		t.Error("tag auto lost its used --dry-run flag")
	}
}
