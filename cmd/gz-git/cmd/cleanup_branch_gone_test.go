// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)

// TestCleanupBranchGoneDeletesAndCounts covers the single-repository --gone path
// end to end.
//
// Two separate defects made this flag do nothing. The analysis never recognised a
// gone branch, and — even after that was fixed — this command built its
// AnalyzeOptions without IncludeGone, so --gone was accepted, printed nothing,
// and exited 0. The bulk path (a directory argument) always passed it, which is
// why the flag looked implemented.
func TestCleanupBranchGoneDeletesAndCounts(t *testing.T) {
	origin := testutil.TempGitRepoWithCommit(t)
	runGit(t, origin, "branch", "gone-upstream")

	clone := filepath.Join(t.TempDir(), "clone")
	runGit(t, t.TempDir(), "clone", origin, clone)
	runGit(t, clone, "branch", "--track", "gone-upstream", "origin/gone-upstream")

	// The upstream disappears; the clone still has its local branch and a stale
	// remote-tracking ref until the analysis prunes.
	runGit(t, origin, "branch", "-D", "gone-upstream")

	setCleanupBranchTestGlobals(t, currentBranchName(t, clone))

	cleanupBranchMerged = false
	cleanupBranchGone = true

	t.Chdir(clone)

	var runErr error

	stdout, stderr := captureOutErr(t, func() {
		runErr = runCleanupBranch(cleanupBranchCmd, nil)
	})

	if runErr != nil {
		t.Fatalf("runCleanupBranch() error = %v\nstderr:\n%s", runErr, stderr)
	}

	if !strings.Contains(stdout, "✓ Deleted 1 branch(es)") {
		t.Errorf("stdout does not report the deletion:\n%s", stdout)
	}

	// The flag's original symptom: nothing found, nothing said.
	if strings.Contains(stdout, "✓ No branches to clean up") {
		t.Errorf("--gone still finds nothing:\n%s", stdout)
	}

	cmd := exec.Command("git", "for-each-ref", "--format=%(refname:short)", "refs/heads/") //nolint:noctx // test helper
	cmd.Dir = clone

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("for-each-ref: %v\n%s", err, out)
	}

	if strings.Contains(string(out), "gone-upstream") {
		t.Errorf("gone-upstream survived the deletion:\n%s", out)
	}
}
