// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)

// syncBaseFixture reproduces the drift this feature exists for: a clone whose
// work happens on develop while master — never checked out, therefore never
// pulled — stays at the commit it held on clone day, even though origin/master
// has moved on.
//
// It returns the working clone; the upstream clone that publishes to origin is
// only scaffolding and stays inside.
func syncBaseFixture(t *testing.T, masterCommits int) string {
	t.Helper()

	origin := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, t.TempDir(), "init", "--bare", "--initial-branch=master", origin)

	upstream := testutil.TempGitRepoWithCommit(t)
	runGit(t, upstream, "branch", "-M", "master")
	runGit(t, upstream, "remote", "add", "origin", origin)
	runGit(t, upstream, "push", "-u", "origin", "master")
	runGit(t, upstream, "checkout", "-b", "develop")
	commitFile(t, upstream, "develop-seed.txt")
	runGit(t, upstream, "push", "-u", "origin", "develop")

	work := filepath.Join(t.TempDir(), "work")
	runGit(t, t.TempDir(), "clone", origin, work)
	runGit(t, work, "config", "user.email", "test@example.com")
	runGit(t, work, "config", "user.name", "Test")
	runGit(t, work, "checkout", "develop")

	// origin/master advances while the clone is standing on develop.
	runGit(t, upstream, "checkout", "master")
	for i := range masterCommits {
		commitFile(t, upstream, "master-"+string(rune('a'+i))+".txt")
	}
	if masterCommits > 0 {
		runGit(t, upstream, "push", "origin", "master")
	}

	return work
}

// TestSyncBase_FastForwardsUncheckedOutBase is the whole point of the feature:
// a base ref nothing checks out is advanced to its remote, without touching the
// branch the user is actually on.
func TestSyncBase_FastForwardsUncheckedOutBase(t *testing.T) {
	work := syncBaseFixture(t, 3)

	before := gitOut(t, work, "rev-parse", "master")
	developBefore := gitOut(t, work, "rev-parse", "develop")

	client := NewClient()
	got, err := client.SyncBase(context.Background(), work, "origin", []string{"master"}, true, false)
	if err != nil {
		t.Fatalf("SyncBase: %v", err)
	}

	if got.Action != BaseSyncFastForward {
		t.Errorf("Action = %q, want %q (reason: %s)", got.Action, BaseSyncFastForward, got.Reason)
	}
	if got.Advanced != 3 {
		t.Errorf("Advanced = %d, want 3", got.Advanced)
	}
	if got.Base != "master" {
		t.Errorf("Base = %q, want master", got.Base)
	}

	after := gitOut(t, work, "rev-parse", "master")
	if after == before {
		t.Error("local master did not move")
	}
	if want := gitOut(t, work, "rev-parse", "refs/remotes/origin/master"); after != want {
		t.Errorf("local master = %s, want origin/master %s", after, want)
	}

	// The checked-out branch and the working tree are none of this operation's
	// business; a base sync that moved develop would be a bug that silently
	// rewrites what the user is working on.
	if now := gitOut(t, work, "rev-parse", "develop"); now != developBefore {
		t.Errorf("develop moved from %s to %s", developBefore, now)
	}
	if branch := gitOut(t, work, "rev-parse", "--abbrev-ref", "HEAD"); branch != "develop" {
		t.Errorf("HEAD is on %q, want develop", branch)
	}
}

// TestSyncBase_SkipsCheckedOutBase pins the hand-off to the normal pull path.
// Re-pointing a checked-out branch from underneath its working tree is how you
// manufacture a repository that reports every file as deleted.
func TestSyncBase_SkipsCheckedOutBase(t *testing.T) {
	work := syncBaseFixture(t, 2)
	runGit(t, work, "checkout", "master")

	client := NewClient()
	got, err := client.SyncBase(context.Background(), work, "origin", []string{"master"}, true, false)
	if err != nil {
		t.Fatalf("SyncBase: %v", err)
	}
	if got.Action != BaseSyncSkipped {
		t.Errorf("Action = %q, want %q", got.Action, BaseSyncSkipped)
	}
	if got.Reason == "" {
		t.Error("a skip with no reason is unexplainable to the user")
	}
}

// TestSyncBase_UpToDateIsNotAWrite guards the common case: most repositories on
// most runs have nothing to do, and they must say so without touching a ref.
func TestSyncBase_UpToDateIsNotAWrite(t *testing.T) {
	work := syncBaseFixture(t, 0)

	before := gitOut(t, work, "rev-parse", "master")

	client := NewClient()
	got, err := client.SyncBase(context.Background(), work, "origin", []string{"master"}, true, false)
	if err != nil {
		t.Fatalf("SyncBase: %v", err)
	}
	if got.Action != BaseSyncUpToDate {
		t.Errorf("Action = %q, want %q", got.Action, BaseSyncUpToDate)
	}
	if after := gitOut(t, work, "rev-parse", "master"); after != before {
		t.Errorf("master moved from %s to %s on an up-to-date sync", before, after)
	}
}

// TestSyncBase_DryRunWritesNothing verifies the dry run reports the same verdict
// it would act on, while leaving the ref alone.
func TestSyncBase_DryRunWritesNothing(t *testing.T) {
	work := syncBaseFixture(t, 4)
	runGit(t, work, "fetch", "origin")

	before := gitOut(t, work, "rev-parse", "master")

	client := NewClient()
	got, err := client.SyncBase(context.Background(), work, "origin", []string{"master"}, true, true)
	if err != nil {
		t.Fatalf("SyncBase: %v", err)
	}
	if got.Action != BaseSyncFastForward {
		t.Errorf("Action = %q, want %q", got.Action, BaseSyncFastForward)
	}
	if after := gitOut(t, work, "rev-parse", "master"); after != before {
		t.Errorf("dry run moved master from %s to %s", before, after)
	}
}

// TestSyncBase_BlocksDivergenceByDefault pins the conservative default in
// decideBaseDivergence: when the local base holds commits the remote base does
// not, nothing is written until a policy says it may be.
func TestSyncBase_BlocksDivergenceByDefault(t *testing.T) {
	work := syncBaseFixture(t, 2)
	runGit(t, work, "fetch", "origin")

	// Local master picks up a commit of its own, so it is no longer an ancestor
	// of origin/master.
	runGit(t, work, "checkout", "master")
	commitFile(t, work, "local-only.txt")
	runGit(t, work, "checkout", "develop")

	before := gitOut(t, work, "rev-parse", "master")

	client := NewClient()
	got, err := client.SyncBase(context.Background(), work, "origin", []string{"master"}, true, false)
	if err != nil {
		t.Fatalf("SyncBase: %v", err)
	}
	if got.Action != BaseSyncBlocked {
		t.Errorf("Action = %q, want %q", got.Action, BaseSyncBlocked)
	}
	if after := gitOut(t, work, "rev-parse", "master"); after != before {
		t.Errorf("a blocked sync moved master from %s to %s", before, after)
	}
}

// TestSyncBase_StrandedDistinguishesPushedFromUnpushed is the input the
// divergence policy turns on, so it is pinned independently of whatever
// decideBaseDivergence currently decides.
//
// Two local bases can both be "not a fast-forward" and mean opposite things:
// one parked on a task-branch tip that origin already has, one carrying work
// that was never pushed anywhere. Stranded is what tells them apart.
func TestSyncBase_StrandedDistinguishesPushedFromUnpushed(t *testing.T) {
	t.Run("local-only commits already pushed elsewhere", func(t *testing.T) {
		work := syncBaseFixture(t, 2)
		runGit(t, work, "fetch", "origin")

		// A commit made on master and published under a task-branch name. The
		// local base is not an ancestor of origin/master, yet origin holds every
		// one of its commits.
		runGit(t, work, "checkout", "master")
		commitFile(t, work, "pushed-elsewhere.txt")
		runGit(t, work, "push", "origin", "HEAD:refs/heads/dev/x/feat/parked")
		runGit(t, work, "fetch", "origin")
		runGit(t, work, "checkout", "develop")

		client := NewClient()
		got, err := client.SyncBase(context.Background(), work, "origin", []string{"master"}, true, true)
		if err != nil {
			t.Fatalf("SyncBase: %v", err)
		}
		if got.Divergence.LocalOnly == 0 {
			t.Fatal("fixture did not produce a divergence")
		}
		if got.Divergence.Stranded != 0 {
			t.Errorf("Stranded = %d, want 0 — every local commit is on origin",
				got.Divergence.Stranded)
		}
	})

	t.Run("local-only commits pushed nowhere", func(t *testing.T) {
		work := syncBaseFixture(t, 2)
		runGit(t, work, "fetch", "origin")

		runGit(t, work, "checkout", "master")
		commitFile(t, work, "never-pushed.txt")
		runGit(t, work, "checkout", "develop")

		client := NewClient()
		got, err := client.SyncBase(context.Background(), work, "origin", []string{"master"}, true, true)
		if err != nil {
			t.Fatalf("SyncBase: %v", err)
		}
		if got.Divergence.Stranded != 1 {
			t.Errorf("Stranded = %d, want 1 — the commit exists on no remote ref",
				got.Divergence.Stranded)
		}
	})
}

// TestSyncBase_NoBaseIsReportedNotGuessed keeps a repository with no
// integration branch from being repaired against an invented one.
func TestSyncBase_NoBaseIsReportedNotGuessed(t *testing.T) {
	work := syncBaseFixture(t, 1)
	runGit(t, work, "branch", "-D", "master")

	client := NewClient()
	got, err := client.SyncBase(context.Background(), work, "origin", []string{"nonexistent"}, true, false)
	if err != nil {
		t.Fatalf("SyncBase: %v", err)
	}
	if got.Action != BaseSyncSkipped {
		t.Errorf("Action = %q, want %q", got.Action, BaseSyncSkipped)
	}
}
