// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package workspacecli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)

// runAddCmd drives the real `workspace add` cobra command from workDir. Passing
// an explicit -c config path bypasses upward config discovery so the test is
// isolated from any .gz-git.yaml above the temp dir. Command output is captured
// into a buffer only to keep it off the test console; callers assert on the
// returned error.
func runAddCmd(t *testing.T, workDir string, args ...string) error {
	t.Helper()
	t.Chdir(workDir)
	cmd := CommandFactory{}.newAddCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	return cmd.Execute()
}

// TestWorkspaceAdd_RejectsEmptyAndInvalidURL is the AC3 core: `workspace add`
// must return an error for an empty or malformed URL and must not write anything
// to disk, while a valid URL is persisted with the canonical "path" key.
func TestWorkspaceAdd_RejectsEmptyAndInvalidURL(t *testing.T) {
	t.Run("empty URL errors and writes nothing", func(t *testing.T) {
		dir := t.TempDir()
		cfg := filepath.Join(dir, DefaultConfigFile)
		if err := runAddCmd(t, dir, "-c", cfg); err == nil {
			t.Error("expected error for missing URL, got nil")
		}
		if _, err := os.Stat(cfg); !os.IsNotExist(err) {
			t.Errorf("config file must not be created on rejected add, stat err=%v", err)
		}
	})

	t.Run("malformed URL errors and writes nothing", func(t *testing.T) {
		dir := t.TempDir()
		cfg := filepath.Join(dir, DefaultConfigFile)
		if err := runAddCmd(t, dir, "-c", cfg, "garbage-not-a-url"); err == nil {
			t.Error("expected error for malformed URL, got nil")
		}
		if _, err := os.Stat(cfg); !os.IsNotExist(err) {
			t.Errorf("config file must not be created on rejected add, stat err=%v", err)
		}
	})

	t.Run("valid URL is written with canonical path key", func(t *testing.T) {
		dir := t.TempDir()
		cfg := filepath.Join(dir, DefaultConfigFile)
		if err := runAddCmd(t, dir, "-c", cfg, "https://github.com/user/repo.git"); err != nil {
			t.Fatalf("valid URL add failed: %v", err)
		}
		data, err := os.ReadFile(cfg)
		if err != nil {
			t.Fatalf("read written config: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "https://github.com/user/repo.git") {
			t.Errorf("written config missing URL:\n%s", content)
		}
		// The old code wrote "targetPath", which no loader reads; the fix uses "path".
		if strings.Contains(content, "targetPath") {
			t.Errorf("written config uses stale targetPath key:\n%s", content)
		}
		if !strings.Contains(content, "path:") {
			t.Errorf("written config missing canonical path key:\n%s", content)
		}
	})
}

// TestWorkspaceAdd_RejectsDuplicateURL covers the "URL 중복 미검사" gap: adding
// the same repository URL twice must be rejected.
func TestWorkspaceAdd_RejectsDuplicateURL(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, DefaultConfigFile)
	const url = "https://github.com/user/repo.git"

	if err := runAddCmd(t, dir, "-c", cfg, url); err != nil {
		t.Fatalf("first add failed: %v", err)
	}
	// Same URL, different name/path — must still be rejected as a URL duplicate.
	if err := runAddCmd(t, dir, "-c", cfg, "--name", "other", "--path", "./other", url); err == nil {
		t.Error("expected duplicate-URL error, got nil")
	}
}

// TestWorkspaceAddFromCurrent_ReadsRemoteOrErrors is the AC3 fix for the
// --from-current path: it must resolve the real origin remote (never persist an
// empty URL). With no remote it errors; with an origin it writes that URL.
func TestWorkspaceAddFromCurrent_ReadsRemoteOrErrors(t *testing.T) {
	t.Run("no remote errors, writes nothing", func(t *testing.T) {
		repo := testutil.TempGitRepoWithCommit(t)
		cfg := filepath.Join(t.TempDir(), DefaultConfigFile)
		if err := runAddCmd(t, repo, "-c", cfg, "--from-current"); err == nil {
			t.Error("expected error when current repo has no remote, got nil")
		}
		if _, err := os.Stat(cfg); !os.IsNotExist(err) {
			t.Errorf("config must not be written when remote is missing, stat err=%v", err)
		}
	})

	t.Run("origin remote is resolved and written", func(t *testing.T) {
		repo := testutil.TempGitRepoWithCommit(t)
		// Use a host outside common url.*.insteadOf rewrites (e.g. github.com→ssh).
		const url = "https://git.example.test/example/current.git"
		if out, err := exec.CommandContext(context.Background(), "git", "-C", repo, "remote", "add", "origin", url).CombinedOutput(); err != nil {
			t.Fatalf("git remote add: %v (%s)", err, out)
		}
		cfg := filepath.Join(t.TempDir(), DefaultConfigFile)
		if err := runAddCmd(t, repo, "-c", cfg, "--from-current"); err != nil {
			t.Fatalf("from-current add failed: %v", err)
		}
		data, err := os.ReadFile(cfg)
		if err != nil {
			t.Fatalf("read written config: %v", err)
		}
		if !strings.Contains(string(data), url) {
			t.Errorf("written config missing resolved remote URL %q:\n%s", url, data)
		}
	})
}
