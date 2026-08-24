// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
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
	got, err := client.SyncBase(context.Background(), work, BaseSyncOptions{
		Remote: "origin", Candidates: []string{"master"}, Fetch: true, DryRun: false,
	})
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
	got, err := client.SyncBase(context.Background(), work, BaseSyncOptions{
		Remote: "origin", Candidates: []string{"master"}, Fetch: true, DryRun: false,
	})
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
	got, err := client.SyncBase(context.Background(), work, BaseSyncOptions{
		Remote: "origin", Candidates: []string{"master"}, Fetch: true, DryRun: false,
	})
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
	got, err := client.SyncBase(context.Background(), work, BaseSyncOptions{
		Remote: "origin", Candidates: []string{"master"}, Fetch: true, DryRun: true,
	})
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

// TestSyncBase_BlocksUnpushedDivergence pins the half of decideBaseDivergence
// that refuses: the local base holds a commit that exists on no remote ref, so
// moving the pointer would leave it reachable only from the reflog.
func TestSyncBase_BlocksUnpushedDivergence(t *testing.T) {
	work := syncBaseFixture(t, 2)
	runGit(t, work, "fetch", "origin")

	// Local master picks up a commit of its own, so it is no longer an ancestor
	// of origin/master.
	runGit(t, work, "checkout", "master")
	commitFile(t, work, "local-only.txt")
	runGit(t, work, "checkout", "develop")

	before := gitOut(t, work, "rev-parse", "master")

	client := NewClient()
	got, err := client.SyncBase(context.Background(), work, BaseSyncOptions{
		Remote: "origin", Candidates: []string{"master"}, Fetch: true, DryRun: false,
	})
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
		got, err := client.SyncBase(context.Background(), work, BaseSyncOptions{
			Remote: "origin", Candidates: []string{"master"}, Fetch: true, DryRun: true,
		})
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
		got, err := client.SyncBase(context.Background(), work, BaseSyncOptions{
			Remote: "origin", Candidates: []string{"master"}, Fetch: true, DryRun: true,
		})
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
	got, err := client.SyncBase(context.Background(), work, BaseSyncOptions{
		Remote: "origin", Candidates: []string{"nonexistent"}, Fetch: true, DryRun: false,
	})
	if err != nil {
		t.Fatalf("SyncBase: %v", err)
	}
	if got.Action != BaseSyncSkipped {
		t.Errorf("Action = %q, want %q", got.Action, BaseSyncSkipped)
	}
}

// TestSyncBase_AdoptsWhenNothingWouldBeStranded pins the other half of the
// policy, and it is the half that makes the feature usable. A base ref parked
// on a task-branch tip diverges from origin/master forever; blocking it means a
// warning that can never be cleared, and a warning that never clears stops
// being read. Adopting is safe here for one specific reason: origin holds every
// commit the local ref would move off.
func TestSyncBase_AdoptsWhenNothingWouldBeStranded(t *testing.T) {
	work := syncBaseFixture(t, 2)
	runGit(t, work, "fetch", "origin")

	runGit(t, work, "checkout", "master")
	commitFile(t, work, "pushed-elsewhere.txt")
	runGit(t, work, "push", "origin", "HEAD:refs/heads/dev/x/feat/parked")
	runGit(t, work, "fetch", "origin")
	runGit(t, work, "checkout", "develop")

	parked := gitOut(t, work, "rev-parse", "master")

	client := NewClient()
	got, err := client.SyncBase(context.Background(), work, BaseSyncOptions{
		Remote: "origin", Candidates: []string{"master"}, Fetch: true, DryRun: false,
	})
	if err != nil {
		t.Fatalf("SyncBase: %v", err)
	}
	if got.Action != BaseSyncAdopted {
		t.Fatalf("Action = %q (%s), want %q", got.Action, got.Reason, BaseSyncAdopted)
	}
	if got.Divergence.Stranded != 0 {
		t.Errorf("Stranded = %d, want 0", got.Divergence.Stranded)
	}

	want := gitOut(t, work, "rev-parse", "refs/remotes/origin/master")
	if after := gitOut(t, work, "rev-parse", "master"); after != want {
		t.Errorf("master = %s, want origin/master %s", after, want)
	}

	// The commit the ref moved off must still be reachable from origin, not
	// merely from the reflog. That distinction is the entire safety argument.
	if out := gitOut(t, work, "branch", "-r", "--contains", parked); out == "" {
		t.Errorf("adopted away commit %s is on no remote branch", parked)
	}
}

// TestSyncBase_AdoptSurvivesAStaleTrackingRef is the regression guard for the
// hole in the adopt policy, and it is the reason the backup ref exists.
//
// Stranded is counted with `rev-list --not --remotes=origin`, which reads
// refs/remotes/origin/*: a *local* cache. A tracking ref for a branch someone
// deleted upstream lives on until the next prune and, until then, testifies
// that a commit is on the remote when it is on no remote at all. The count
// therefore reads 0 for genuinely unpushed work and the ref is adopted away.
//
// The fixture below is that exact state, built without ever touching the
// remote: a commit reachable only from a tracking ref whose branch does not
// exist on origin. What is asserted is not that the policy gets it right — it
// cannot, from local data alone — but that getting it wrong still loses
// nothing.
func TestSyncBase_AdoptSurvivesAStaleTrackingRef(t *testing.T) {
	work := syncBaseFixture(t, 2)
	runGit(t, work, "fetch", "origin")

	runGit(t, work, "checkout", "master")
	commitFile(t, work, "never-pushed.txt")
	orphan := gitOut(t, work, "rev-parse", "master")
	runGit(t, work, "checkout", "develop")

	// A tracking ref for a branch origin has never heard of. This is what a
	// deleted-upstream branch looks like locally before `fetch --prune`.
	runGit(t, work, "update-ref", "refs/remotes/origin/gone", orphan)

	client := NewClient()
	got, err := client.SyncBase(context.Background(), work, BaseSyncOptions{
		Remote: "origin", Candidates: []string{"master"}, Fetch: true, DryRun: false,
	})
	if err != nil {
		t.Fatalf("SyncBase: %v", err)
	}

	// The stale ref makes this look safe, and the policy adopts. That is the
	// bug being defended against, not a behavior worth pinning on its own.
	if got.Action != BaseSyncAdopted {
		t.Fatalf("Action = %q (%s), want %q", got.Action, got.Reason, BaseSyncAdopted)
	}

	if got.Backup == "" {
		t.Fatal("an adopt that rewound the base reported no backup ref")
	}

	if at := gitOut(t, work, "rev-parse", got.Backup); at != orphan {
		t.Errorf("%s = %s, want the old base tip %s", got.Backup, at, orphan)
	}

	// The point of the whole exercise: prune away the lie the decision rested
	// on, and the commit is still reachable from a ref.
	runGit(t, work, "update-ref", "-d", "refs/remotes/origin/gone")

	if out := gitOut(t, work, "for-each-ref", "--contains", orphan, "--format=%(refname)"); out == "" {
		t.Errorf("commit %s is reachable from no ref at all after adoption", orphan)
	}
}

// TestSyncBase_SkipsBaseCheckedOutInAnotherWorktree covers the guard that
// `git update-ref` does not supply.
//
// update-ref is plumbing and enforces no checkout rule: it will rewind a branch
// a linked worktree is standing on, where `git branch -f` refuses outright. The
// worktree is then left with an index disagreeing with HEAD, so every file the
// moved-off commits added reads as a staged deletion and the next commit made
// there quietly reverts them.
func TestSyncBase_SkipsBaseCheckedOutInAnotherWorktree(t *testing.T) {
	work := syncBaseFixture(t, 3)

	linked := filepath.Join(t.TempDir(), "linked")
	runGit(t, work, "worktree", "add", linked, "master")

	before := gitOut(t, work, "rev-parse", "master")

	client := NewClient()
	got, err := client.SyncBase(context.Background(), work, BaseSyncOptions{
		Remote: "origin", Candidates: []string{"master"}, Fetch: true, DryRun: false,
	})
	if err != nil {
		t.Fatalf("SyncBase: %v", err)
	}
	if got.Action != BaseSyncSkipped {
		t.Errorf("Action = %q (%s), want %q", got.Action, got.Reason, BaseSyncSkipped)
	}
	if !strings.Contains(got.Reason, "worktree") {
		t.Errorf("Reason = %q, want it to name the worktree holding the branch", got.Reason)
	}
	if after := gitOut(t, work, "rev-parse", "master"); after != before {
		t.Errorf("master moved to %s under a worktree that has it checked out", after)
	}
}

// TestSyncBase_CreateMissingBase covers the repositories that motivated the
// flag: cloned, switched to develop, and never once checking out master, so
// refs/heads/master does not exist at all.
//
// The assertion that matters is not just that master appears — it is that
// nothing happens without the opt-in, and that the branch created points where
// origin points.
func TestSyncBase_CreateMissingBase(t *testing.T) {
	newFixture := func(t *testing.T) string {
		t.Helper()
		work := syncBaseFixture(t, 2)
		runGit(t, work, "fetch", "origin")
		// The state a develop-only clone is actually in: one local branch.
		runGit(t, work, "branch", "-D", "master")
		return work
	}

	t.Run("off by default", func(t *testing.T) {
		work := newFixture(t)

		client := NewClient()
		got, err := client.SyncBase(context.Background(), work, BaseSyncOptions{
			Remote: "origin", Candidates: []string{"master"}, Fetch: true, DryRun: false,
		})
		if err != nil {
			t.Fatalf("SyncBase: %v", err)
		}
		if got.Action == BaseSyncCreated {
			t.Fatal("created a base branch without CreateMissing")
		}
		if refPresent(t, work, "refs/heads/master") {
			t.Error("refs/heads/master exists after a run that should not have created it")
		}
	})

	t.Run("creates at the remote tip", func(t *testing.T) {
		work := newFixture(t)

		client := NewClient()
		got, err := client.SyncBase(context.Background(), work, BaseSyncOptions{
			Remote: "origin", Candidates: []string{"master"}, Fetch: true, DryRun: false,
			CreateMissing: true,
		})
		if err != nil {
			t.Fatalf("SyncBase: %v", err)
		}
		if got.Action != BaseSyncCreated {
			t.Fatalf("Action = %q (%s), want %q", got.Action, got.Reason, BaseSyncCreated)
		}
		if got.Base != "master" {
			t.Errorf("Base = %q, want master", got.Base)
		}

		want := gitOut(t, work, "rev-parse", "refs/remotes/origin/master")
		if after := gitOut(t, work, "rev-parse", "refs/heads/master"); after != want {
			t.Errorf("created master at %s, want origin/master %s", after, want)
		}
	})

	t.Run("dry run creates nothing", func(t *testing.T) {
		work := newFixture(t)

		client := NewClient()
		got, err := client.SyncBase(context.Background(), work, BaseSyncOptions{
			Remote: "origin", Candidates: []string{"master"}, Fetch: true, DryRun: true,
			CreateMissing: true,
		})
		if err != nil {
			t.Fatalf("SyncBase: %v", err)
		}
		if got.Action != BaseSyncCreated {
			t.Fatalf("Action = %q, want %q — a dry run still reports the verdict", got.Action, BaseSyncCreated)
		}
		if refPresent(t, work, "refs/heads/master") {
			t.Error("dry run created refs/heads/master")
		}
	})

	t.Run("leaves an explicitly configured local base alone", func(t *testing.T) {
		// master exists locally and is config-declared, while origin also has a
		// main. Nothing is missing, so nothing is created — the flag must not
		// second-guess a repository that states its own trunk.
		work := syncBaseFixture(t, 2)
		runGit(t, work, "fetch", "origin")
		runGit(t, work, "push", "origin", "refs/remotes/origin/master:refs/heads/main")
		runGit(t, work, "fetch", "origin")

		client := NewClient()
		got, err := client.SyncBase(context.Background(), work, BaseSyncOptions{
			Remote: "origin", Candidates: []string{"master"}, Fetch: true, DryRun: true,
			CreateMissing: true,
		})
		if err != nil {
			t.Fatalf("SyncBase: %v", err)
		}
		if got.Base != "master" {
			t.Errorf("Base = %q, want master — config declared it and it exists locally", got.Base)
		}
		if got.Action == BaseSyncCreated {
			t.Error("created a branch when the configured base was already present")
		}
	})

	t.Run("does not retarget away from a base it could repair", func(t *testing.T) {
		// The regression the review caught. master exists locally and is stale,
		// so there is real repair work to do; origin also has an unrelated main
		// that is absent locally. Consulting the create path here retargets the
		// whole sync to main, creates it, and leaves the stale master exactly as
		// it was — so a flag whose name promises to create something absent
		// silently stops repairing what is present.
		//
		// Worse, it is self-perpetuating: heuristicBaseCandidates prefers main,
		// so every later run in that repository resolves to the branch this one
		// invented and never looks at master again.
		work := syncBaseFixture(t, 2)
		runGit(t, work, "fetch", "origin")
		runGit(t, work, "push", "origin", "refs/remotes/origin/master:refs/heads/main")
		runGit(t, work, "fetch", "origin")

		client := NewClient()
		got, err := client.SyncBase(context.Background(), work, BaseSyncOptions{
			Remote: "origin", Fetch: true, DryRun: false, CreateMissing: true,
		})
		if err != nil {
			t.Fatalf("SyncBase: %v", err)
		}
		if got.Base != "master" {
			t.Fatalf("Base = %q, want master — it exists locally and is behind its remote", got.Base)
		}
		if got.Action != BaseSyncFastForward {
			t.Errorf("Action = %q (%s), want the stale base repaired", got.Action, got.Reason)
		}
		if refPresent(t, work, "refs/heads/main") {
			t.Error("created main while master still needed repairing")
		}
	})

	t.Run("finds a base a single-branch clone has no tracking ref for", func(t *testing.T) {
		// The case the flag was written for, and the one a tracking-ref probe
		// cannot see: `clone --single-branch -b develop` sets a refspec that
		// fetches only develop, so refs/remotes/origin/master never exists even
		// though origin has master. Asking the remote is what finds it.
		seed := syncBaseFixture(t, 2)
		origin := gitOut(t, seed, "config", "--get", "remote.origin.url")

		work := filepath.Join(t.TempDir(), "single")
		runGit(t, t.TempDir(), "clone", "--single-branch", "-b", "develop", origin, work)

		if refPresent(t, work, "refs/remotes/origin/master") {
			t.Fatal("fixture is not single-branch: origin/master is already cached")
		}

		client := NewClient()
		got, err := client.SyncBase(context.Background(), work, BaseSyncOptions{
			Remote: "origin", Fetch: true, DryRun: false, CreateMissing: true,
		})
		if err != nil {
			t.Fatalf("SyncBase: %v", err)
		}
		if got.Action != BaseSyncCreated {
			t.Fatalf("Action = %q (%s), want %q", got.Action, got.Reason, BaseSyncCreated)
		}
		if got.Base != "master" {
			t.Errorf("Base = %q, want master", got.Base)
		}

		want := gitOut(t, work, "rev-parse", "refs/remotes/origin/master")
		if at := gitOut(t, work, "rev-parse", "refs/heads/master"); at != want {
			t.Errorf("master = %s, want origin/master %s", at, want)
		}
	})
}

// refPresent reports whether a ref exists. Unlike gitOut it does not fail the
// test on a non-zero exit, because in the cases below the ref being absent is
// the assertion rather than a broken fixture.
func refPresent(t *testing.T, dir, ref string) bool {
	t.Helper()
	return exec.CommandContext(t.Context(), "git", "-C", dir, "rev-parse", "--verify", "--quiet", ref).Run() == nil
}

// TestSyncBase_EmptyRepositoryIsSkippedNotBlocked pins the case the real run
// exposed: a clone of an empty remote has no commits, so there is no base ref
// to sync and nothing a person can do about it.
//
// Before this guard, `rev-parse --abbrev-ref HEAD` failed on the unborn HEAD,
// the error was absorbed into the blocked verdict, and the run reported one
// repository as needing attention with a note that had a hole where the base
// name belonged. Skipped is the right verdict because the operation is not
// applicable here, not because it went wrong.
func TestSyncBase_EmptyRepositoryIsSkippedNotBlocked(t *testing.T) {
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	runGit(t, root, "init", "--bare", "--initial-branch=master", origin)

	work := filepath.Join(root, "work")
	runGit(t, root, "clone", origin, work)

	// The fixture only means anything while HEAD really is unborn: a well-formed
	// symbolic ref pointing at a branch that does not exist yet.
	if refPresent(t, work, "HEAD") {
		t.Fatal("fixture has commits: HEAD is not unborn")
	}

	client := NewClient()
	got, err := client.SyncBase(context.Background(), work, BaseSyncOptions{
		Remote: "origin", Fetch: true, DryRun: false,
	})
	if err != nil {
		t.Fatalf("SyncBase: %v", err)
	}

	// Asserting Skipped also asserts it is not Blocked and not Failed, which is
	// the entire point: the blocked list is a list of repositories a person must
	// act on, and this is not one of them.
	if got.Action != BaseSyncSkipped {
		t.Errorf("Action = %q (%s), want %q", got.Action, got.Reason, BaseSyncSkipped)
	}
	if !strings.Contains(got.Reason, "no commits") {
		t.Errorf("Reason = %q, want it to say the repository has no commits", got.Reason)
	}
}
