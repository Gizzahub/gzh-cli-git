// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunExec_RequiresDash(t *testing.T) {
	err := runExec(execCmd, []string{"true"})
	if err == nil {
		t.Fatal("expected missing -- error")
	}
	if !strings.Contains(err.Error(), "--") {
		t.Fatalf("err=%v", err)
	}
}

func TestRunExec_DryRunAndSuccess(t *testing.T) {
	prev := execFlags
	prevFF, prevTO := execFailFast, execTimeout
	prevQ, prevV := quiet, verbose
	t.Cleanup(func() {
		execFlags = prev
		execFailFast, execTimeout = prevFF, prevTO
		quiet, verbose = prevQ, prevV
		rootCmd.SetArgs(nil)
		_ = execCmd.Flags().Set("dry-run", "false")
		_ = execCmd.Flags().Set("format", "default")
		_ = execCmd.Flags().Set("scan-depth", "1")
	})

	parent := t.TempDir()
	repo := filepath.Join(parent, "r1")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, parent, "init", "r1")
	runGit(t, repo, "config", "user.email", "t@t.com")
	runGit(t, repo, "config", "user.name", "T")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "i")

	setCommandGroups(rootCmd)

	rootCmd.SetArgs([]string{"exec", "-d", "1", "-n", parent, "--", "true"})
	out := captureStdout(t, func() {
		if err := executeCommand(rootCmd); err != nil {
			t.Fatalf("exec dry-run: %v", err)
		}
	})
	if !strings.Contains(out, "would") && !strings.Contains(out, "r1") && !strings.Contains(out, "Summary") {
		t.Logf("dry-run out: %q", out)
	}

	// Explicitly clear dry-run for real execution
	_ = execCmd.Flags().Set("dry-run", "false")
	execFlags.DryRun = false
	rootCmd.SetArgs([]string{"exec", "-d", "1", "--format", "json", parent, "--", "true"})
	out = captureStdout(t, func() {
		if err := executeCommand(rootCmd); err != nil {
			t.Fatalf("exec true: %v", err)
		}
	})
	if !strings.Contains(out, "success") && !strings.Contains(out, "r1") {
		t.Logf("json out: %q", out)
	}

	_ = execCmd.Flags().Set("dry-run", "false")
	execFlags.DryRun = false
	rootCmd.SetArgs([]string{"exec", "-d", "1", parent, "--", "false"})
	if err := executeCommand(rootCmd); err == nil {
		t.Fatal("expected failure for false")
	}
}

func TestRunExec_MissingCommandAfterDash(t *testing.T) {
	setCommandGroups(rootCmd)
	rootCmd.SetArgs([]string{"exec", "--"})
	err := executeCommand(rootCmd)
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}
