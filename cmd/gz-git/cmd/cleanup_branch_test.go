// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
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
		merged, stale, gone, superseded, dryRun, force, remote bool
		protect, base                                          string
		quiet                                                  bool
	}{
		cleanupBranchMerged, cleanupBranchStale, cleanupBranchGone, cleanupBranchSuperseded,
		cleanupBranchDryRun, cleanupBranchForce, cleanupBranchRemote,
		cleanupBranchProtect, cleanupBranchBaseBranch, quiet,
	}

	cleanupBranchMerged = true
	cleanupBranchStale = false
	cleanupBranchGone = false
	cleanupBranchSuperseded = false
	cleanupBranchDryRun = false
	cleanupBranchForce = true
	cleanupBranchRemote = false
	cleanupBranchProtect = ""
	cleanupBranchBaseBranch = baseBranch
	quiet = false

	t.Cleanup(func() {
		cleanupBranchMerged, cleanupBranchStale, cleanupBranchGone, cleanupBranchSuperseded = orig.merged, orig.stale, orig.gone, orig.superseded
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

func TestCleanupBranchHelpMentionsSuperseded(t *testing.T) {
	flag := cleanupBranchCmd.Flags().Lookup("superseded")
	if flag == nil {
		t.Fatal("missing --superseded flag")
	}
	if !strings.Contains(flag.Usage, "bot") {
		t.Errorf("--superseded usage = %q, want it to mention bot remotes", flag.Usage)
	}
}

func TestCleanupBranchRequiresTypeIncludingSuperseded(t *testing.T) {
	setCleanupBranchTestGlobals(t, "master")
	cleanupBranchMerged = false
	cleanupBranchStale = false
	cleanupBranchGone = false
	cleanupBranchSuperseded = false

	err := runCleanupBranch(cleanupBranchCmd, nil)
	if err == nil {
		t.Fatal("expected an error when no cleanup type is set")
	}
	if !strings.Contains(err.Error(), "--superseded") {
		t.Errorf("error = %q, want it to list --superseded", err)
	}
}

func TestCleanupBranchHelpMentionsNonCanonical(t *testing.T) {
	flag := cleanupBranchCmd.Flags().Lookup("non-canonical")
	if flag == nil {
		t.Fatal("missing --non-canonical flag")
	}
	if !strings.Contains(flag.Usage, "canonical") {
		t.Errorf("--non-canonical usage = %q, want it to mention the canonical branch", flag.Usage)
	}
}

// setCleanupBranchNonCanonTestGlobal pins cleanupBranchNonCanon and restores
// it. It is separate from setCleanupBranchTestGlobals because that helper
// predates the --non-canonical flag and does not manage this global; folding
// it in there would touch every other test's baseline.
func setCleanupBranchNonCanonTestGlobal(t *testing.T, value bool) {
	t.Helper()
	orig := cleanupBranchNonCanon
	cleanupBranchNonCanon = value
	t.Cleanup(func() { cleanupBranchNonCanon = orig })
}

// TestCleanupBranchRequiresTypeIncludingNonCanonical mirrors
// TestCleanupBranchRequiresTypeIncludingSuperseded for --non-canonical: it
// alone must satisfy the "specify at least one cleanup type" gate, and the
// gate's error message must list it when no cleanup type is set at all.
func TestCleanupBranchRequiresTypeIncludingNonCanonical(t *testing.T) {
	setCleanupBranchTestGlobals(t, "master")
	cleanupBranchMerged = false
	cleanupBranchStale = false
	cleanupBranchGone = false
	cleanupBranchSuperseded = false
	setCleanupBranchNonCanonTestGlobal(t, false)

	err := runCleanupBranch(cleanupBranchCmd, nil)
	if err == nil {
		t.Fatal("expected an error when no cleanup type is set")
	}
	if !strings.Contains(err.Error(), "--non-canonical") {
		t.Errorf("error = %q, want it to list --non-canonical", err)
	}

	// --non-canonical alone satisfies the gate. This repository has no
	// .gz-git.yaml integrationBranch declaration, so runCleanupBranch still
	// fails past the gate — but on resolveCanonicalDeclaration's error, not on
	// "specify at least one cleanup type".
	setCleanupBranchNonCanonTestGlobal(t, true)

	repoPath := testutil.TempGitRepoWithCommit(t)
	t.Chdir(repoPath)

	err = runCleanupBranch(cleanupBranchCmd, nil)
	if err == nil {
		t.Fatal("expected an error past the gate (no integrationBranch declaration)")
	}
	if strings.Contains(err.Error(), "specify at least one cleanup type") {
		t.Errorf("error = %q, --non-canonical alone should have satisfied the cleanup-type gate", err)
	}
}

// TestConfirmationLines_OneEntryPerRef pins confirmationLines to render a
// separate line per ref rather than deduplicating by name: a local branch and
// its remote namesake are two deletions, and --non-canonical makes that pair
// the ordinary case, not the exception a name-deduplicated summary could get
// away with hiding.
func TestConfirmationLines_OneEntryPerRef(t *testing.T) {
	repo := &repository.RepositoryCleanupResult{
		Branches: []repository.CleanupBranchEntry{
			{Name: "master", Location: "local"},
			{Name: "master", Location: "remote"},
			{Name: "old-topic", Location: "remote"},
		},
	}

	got := confirmationLines(repo)
	want := []string{"master", "master (remote)", "old-topic (remote)"}

	if len(got) != len(want) {
		t.Fatalf("confirmationLines = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("confirmationLines[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestConfirmationLines_FallsBackToDeletedBranches covers the human-output
// path that predates the per-ref Branches field: when Branches is empty,
// confirmationLines must still return the flat, name-deduplicated
// DeletedBranches list rather than an empty slice.
func TestConfirmationLines_FallsBackToDeletedBranches(t *testing.T) {
	repo := &repository.RepositoryCleanupResult{
		DeletedBranches: []string{"feature-one", "feature-two"},
	}

	got := confirmationLines(repo)
	want := []string{"feature-one", "feature-two"}

	if len(got) != len(want) {
		t.Fatalf("confirmationLines = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("confirmationLines[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestResolveCanonicalDeclarationRequiresDeclaration pins
// resolveCanonicalDeclaration's own error, not the CLI gate above: a
// repository whose .gz-git.yaml declares no branch.integrationBranch must get
// an explicit error explaining that a declaration is required, never a
// silently empty canonical branch.
func TestResolveCanonicalDeclarationRequiresDeclaration(t *testing.T) {
	repoPath := t.TempDir()

	canonical, patterns, err := resolveCanonicalDeclaration(context.Background(), repoPath)
	if err == nil {
		t.Fatalf("expected an error for an undeclared repository, got canonical=%q patterns=%v", canonical, patterns)
	}
	if !strings.Contains(err.Error(), "requires branch.integrationBranch") {
		t.Errorf("error = %q, want it to explain that branch.integrationBranch must be declared", err)
	}
}
