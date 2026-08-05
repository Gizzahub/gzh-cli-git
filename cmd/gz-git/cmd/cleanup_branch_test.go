// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
)

// captureOutErr redirects both os.Stdout and os.Stderr for the duration of fn.
// This command reports deletions on stdout and failures on stderr, so a test of
// the two together needs both.
func captureOutErr(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()

	origOut, origErr := os.Stdout, os.Stderr

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	os.Stdout, os.Stderr = outW, errW

	defer func() { os.Stdout, os.Stderr = origOut, origErr }()

	fn()

	_ = outW.Close()
	_ = errW.Close()

	var outBuf, errBuf bytes.Buffer

	if _, err := io.Copy(&outBuf, outR); err != nil {
		t.Fatalf("copy stdout: %v", err)
	}

	if _, err := io.Copy(&errBuf, errR); err != nil {
		t.Fatalf("copy stderr: %v", err)
	}

	return outBuf.String(), errBuf.String()
}

// setCleanupBranchTestGlobals pins the command's flag globals and restores them.
func setCleanupBranchTestGlobals(t *testing.T, baseBranch string) {
	t.Helper()

	orig := struct {
		merged, stale, gone, dryRun, force, remote bool
		protect, base                              string
		quiet                                      bool
	}{
		cleanupBranchMerged, cleanupBranchStale, cleanupBranchGone,
		cleanupBranchDryRun, cleanupBranchForce, cleanupBranchRemote,
		cleanupBranchProtect, cleanupBranchBaseBranch, quiet,
	}

	cleanupBranchMerged = true
	cleanupBranchStale = false
	cleanupBranchGone = false
	cleanupBranchDryRun = false
	cleanupBranchForce = true
	cleanupBranchRemote = false
	cleanupBranchProtect = ""
	cleanupBranchBaseBranch = baseBranch
	quiet = false

	t.Cleanup(func() {
		cleanupBranchMerged, cleanupBranchStale, cleanupBranchGone = orig.merged, orig.stale, orig.gone
		cleanupBranchDryRun, cleanupBranchForce, cleanupBranchRemote = orig.dryRun, orig.force, orig.remote
		cleanupBranchProtect, cleanupBranchBaseBranch, quiet = orig.protect, orig.base, orig.quiet
	})
}

// currentBranchName returns the branch HEAD points at.
func currentBranchName(t *testing.T, dir string) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD") //nolint:noctx // test helper
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}

	return strings.TrimSpace(string(out))
}

// TestCleanupBranchCountsDeletionsNotCandidates makes every deletion fail and
// asserts the three things that used to be invisible: the printed count is the
// number deleted (0) rather than the number of candidates (2), the failures are
// named on stderr, and the command exits 2.
//
// Deletions are broken by making .git/refs/heads read-only, so git cannot create
// the lock file it needs to drop a ref. Everything cleanup reads still works.
func TestCleanupBranchCountsDeletionsNotCandidates(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: directory permissions do not stop unlink")
	}

	repoPath := testutil.TempGitRepoWithCommit(t)
	base := currentBranchName(t, repoPath)

	// Two branches at the same commit as base, so Analyze reports both as merged.
	// The names carry no slash on purpose: a "feature/one" ref lives in its own
	// refs/heads/feature directory, which the chmod below would not cover.
	runGit(t, repoPath, "branch", "cleanup-one")
	runGit(t, repoPath, "branch", "cleanup-two")

	heads := filepath.Join(repoPath, ".git", "refs", "heads")
	if err := os.Chmod(heads, 0o500); err != nil { //nolint:gosec // deliberately read-only
		t.Fatalf("chmod %s: %v", heads, err)
	}

	// Restore before t.TempDir cleanup, which cannot remove a read-only directory.
	t.Cleanup(func() { _ = os.Chmod(heads, 0o700) }) //nolint:gosec // restore

	setCleanupBranchTestGlobals(t, base)
	t.Chdir(repoPath)

	var runErr error

	stdout, stderr := captureOutErr(t, func() {
		runErr = runCleanupBranch(cleanupBranchCmd, nil)
	})

	if got := cliutil.ExitCodeForError(runErr); got != cliutil.ExitPartialFailed {
		t.Errorf("exit code = %d, want %d (partial failure); err=%v", got, cliutil.ExitPartialFailed, runErr)
	}

	if !strings.Contains(stdout, "✓ Deleted 0 branch(es)") {
		t.Errorf("stdout does not report zero deletions:\n%s", stdout)
	}

	if strings.Contains(stdout, "✓ Deleted 2 branch(es)") {
		t.Error("stdout reports the candidate count as the deletion count")
	}

	for _, name := range []string{"cleanup-one", "cleanup-two"} {
		if !strings.Contains(stderr, name) {
			t.Errorf("stderr does not name the failed branch %q:\n%s", name, stderr)
		}
	}
}
