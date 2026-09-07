// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/branch"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// nonCanonicalCloneFixture builds the repository this command exists to clean:
// a remote whose default is develop and which still carries a duplicate master
// at the same commit, cloned fresh. A fresh clone has no local master, so the
// candidate is the remote one and its ancestry rests entirely on the
// remote-tracking cache.
//
// It returns the work clone's path. The caller decides whether origin is
// reachable, which is the whole variable under test.
func nonCanonicalCloneFixture(t *testing.T) (workPath, remotePath string) {
	t.Helper()

	root := t.TempDir()
	seed := filepath.Join(root, "seed")
	runGit(t, root, "init", "-q", seed)
	runGit(t, seed, "config", "user.email", "t@example.com")
	runGit(t, seed, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(seed, "f"), []byte("one\n"), 0o600); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	runGit(t, seed, "add", "-A")
	runGit(t, seed, "commit", "-qm", "one")
	runGit(t, seed, "branch", "-M", "develop")
	runGit(t, seed, "branch", "master")

	// Cloned rather than pushed: a bare clone reaches the same state and keeps
	// the fixture from depending on push permissions or hooks.
	remotePath = filepath.Join(root, "remote.git")
	runGit(t, root, "clone", "-q", "--bare", seed, remotePath)
	runGit(t, root, "--git-dir", remotePath, "symbolic-ref", "HEAD", "refs/heads/develop")

	workPath = filepath.Join(root, "work")
	runGit(t, root, "clone", "-q", remotePath, workPath)
	runGit(t, workPath, "config", "user.email", "t@example.com")
	runGit(t, workPath, "config", "user.name", "t")

	decl := "branch:\n  integrationBranch:\n    - develop\n"
	if err := os.WriteFile(filepath.Join(workPath, ".gz-git.yaml"), []byte(decl), 0o600); err != nil {
		t.Fatalf("write .gz-git.yaml: %v", err)
	}

	return workPath, remotePath
}

// setNonCanonicalDryRunGlobals puts the command in the shape of
// `cleanup branch --non-canonical -r --dry-run` and restores every global it
// touches.
func setNonCanonicalDryRunGlobals(t *testing.T) {
	t.Helper()

	setCleanupBranchTestGlobals(t, "")
	cleanupBranchMerged = false
	cleanupBranchForce = false
	cleanupBranchDryRun = true
	cleanupBranchRemote = true

	origNonCanon, origFormat := cleanupBranchNonCanon, cleanupBranchBulkFlags.Format
	cleanupBranchNonCanon = true
	cleanupBranchBulkFlags.Format = "default"
	t.Cleanup(func() {
		cleanupBranchNonCanon, cleanupBranchBulkFlags.Format = origNonCanon, origFormat
	})
}

// The defect: the single-repository dry run returned straight from Analyze and
// never reached the canonical-tip gate, which lives inside Execute. With origin
// unreachable the preview offered `master` and exited 0, and the real run then
// refused the same branch. The bulk engine had already been fixed, so the two
// engines disagreed about the same repository — and the single one is the one
// operators run first to sanity-check.
func TestCleanupBranchSingleDryRun_RefusesOnUnconfirmableCanonicalTip(t *testing.T) {
	work, _ := nonCanonicalCloneFixture(t)
	runGit(t, work, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "unreachable.git"))

	setNonCanonicalDryRunGlobals(t)
	t.Chdir(work)

	var runErr error
	stdout, stderr := captureOutErr(t, func() { runErr = runCleanupBranch(cleanupBranchCmd, nil) })

	if got := cliutil.ExitCodeForError(runErr); got != cliutil.ExitPartialFailed {
		t.Errorf("exit code = %d, want %d; err=%v", got, cliutil.ExitPartialFailed, runErr)
	}
	if !strings.Contains(stdout, "0 branch(es) to clean up") {
		t.Errorf("preview still offers a deletion the run would refuse:\n%s", stdout)
	}
	if !strings.Contains(stderr, "master") {
		t.Errorf("refusal does not name the branch:\n%s", stderr)
	}
	if !strings.Contains(stderr, "could not reach origin") {
		t.Errorf("refusal does not carry the remedy:\n%s", stderr)
	}
}

// The gate must not fire when the evidence can be confirmed, or offline-hostile
// strictness would be the new defect. A reachable origin previews the deletion
// and exits 0, exactly as before the fix.
func TestCleanupBranchSingleDryRun_OffersDeletionWhenTipConfirms(t *testing.T) {
	work, _ := nonCanonicalCloneFixture(t)

	setNonCanonicalDryRunGlobals(t)
	t.Chdir(work)

	var runErr error
	stdout, stderr := captureOutErr(t, func() { runErr = runCleanupBranch(cleanupBranchCmd, nil) })

	if runErr != nil {
		t.Errorf("healthy preview returned an error: %v", runErr)
	}
	if !strings.Contains(stdout, "1 branch(es) to clean up") {
		t.Errorf("preview does not offer the retirable branch:\n%s", stdout)
	}
	if strings.Contains(stderr, "Failed to delete") {
		t.Errorf("healthy preview reported a refusal:\n%s", stderr)
	}
}

// A dry run must not write. The gate reaches the network to confirm the tip,
// which is a fetch and not a deletion — the remote's master has to survive it.
func TestCleanupBranchSingleDryRun_LeavesRemoteIntact(t *testing.T) {
	work, remote := nonCanonicalCloneFixture(t)

	setNonCanonicalDryRunGlobals(t)
	t.Chdir(work)

	captureOutErr(t, func() { _ = runCleanupBranch(cleanupBranchCmd, nil) })

	if out := gitOutputForIntegrateRun(t, remote, "--git-dir", remote, "branch", "--list", "master"); !strings.Contains(out, "master") {
		t.Errorf("dry run deleted the remote branch; remaining: %q", out)
	}
}

func TestReportWithoutBranches(t *testing.T) {
	report := &branch.CleanupReport{
		Merged:       []*branch.Branch{{Name: "merged-keep"}},
		NonCanonical: []*branch.Branch{{Name: "master"}, {Name: "trunk"}},
	}
	got := reportWithoutBranches(report, []branch.DeleteFailure{{Branch: "master", Err: errors.New("blocked")}})

	if len(got.NonCanonical) != 1 || got.NonCanonical[0].Name != "trunk" {
		t.Errorf("refused branch not removed: %+v", got.NonCanonical)
	}
	if len(got.Merged) != 1 {
		t.Errorf("unrelated bucket was disturbed: %+v", got.Merged)
	}
	// The input must not be mutated: the caller still needs the unfiltered
	// report to describe the refusal it is about to print.
	if len(report.NonCanonical) != 2 {
		t.Errorf("input report was mutated: %+v", report.NonCanonical)
	}
}

// A refusal is described in the same words as a deletable candidate, taken from
// the same builder rather than spelled a second time.
func TestFailureEntriesFrom_BorrowsDescription(t *testing.T) {
	described := []repository.CleanupBranchEntry{
		{Name: "master", Reason: "non-canonical", Location: "remote"},
	}
	got := failureEntriesFrom(
		[]branch.DeleteFailure{
			{Branch: "master", Err: errors.New("tip moved")},
			{Branch: "unknown", Err: errors.New("tip moved")},
		},
		described,
	)

	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d", len(got))
	}
	if got[0].Reason != "non-canonical" || got[0].Location != "remote" {
		t.Errorf("described refusal lost its vocabulary: %+v", got[0])
	}
	if got[0].Error != "tip moved" {
		t.Errorf("remedy text not carried: %+v", got[0])
	}
	// A branch with no matching description still reports, with the fields it
	// has. Dropping it would hide a refusal to keep the output tidy.
	if got[1].Name != "unknown" || got[1].Reason != "" {
		t.Errorf("undescribed refusal mishandled: %+v", got[1])
	}
}

// Matches the bulk engine's rule exactly: a partial plan is still a plan and
// exits zero; a preview with nothing to approve and a refusal behind it is not.
func TestDryRunRefusalError(t *testing.T) {
	one := []repository.CleanupBranchEntry{{Name: "feature/x"}}
	blocked := []branch.DeleteFailure{{Branch: "master", Err: errors.New("blocked")}}

	if err := dryRunRefusalError(one, nil); err != nil {
		t.Errorf("clean preview returned an error: %v", err)
	}
	if err := dryRunRefusalError(one, blocked); err != nil {
		t.Errorf("mixed preview must exit zero — a partial plan is still a plan: %v", err)
	}
	if err := dryRunRefusalError(nil, blocked); err == nil {
		t.Error("fully blocked preview reported success")
	}
	if err := dryRunRefusalError(nil, nil); err != nil {
		t.Errorf("empty preview returned an error: %v", err)
	}
}
