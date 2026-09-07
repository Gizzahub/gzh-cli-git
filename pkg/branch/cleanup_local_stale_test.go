// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package branch

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// staleLocalFixture builds the shape a --refspec master:develop workflow leaves
// behind on a fresh clone, with the remote-tracking cache since contradicted.
//
// The clone has no local develop — git clone creates one branch, and origin's
// HEAD is master — so a local candidate can only be measured against
// refs/remotes/origin/develop, which is a cache. Local master holds D, which
// was pushed to develop and never to master, so origin/master does not have it.
// develop is then rewound on the remote to a history that never had D, and the
// clone is deliberately not fetched: deleting local master now drops D from
// every ref, and `branch -D` takes the branch reflog with it.
//
// wip is branched from origin/master rather than from master on purpose. From
// master it would keep D reachable and hide exactly the loss being measured;
// and master must not be HEAD, or git refuses the delete on its own and the
// test passes whether or not the gate exists.
func staleLocalFixture(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	seed := filepath.Join(root, "seed")
	work := filepath.Join(root, "work")

	gitCommit(t, root, "init", "-q", "--bare", origin)
	gitCommit(t, root, "init", "-q", "-b", "master", seed)
	gitCommit(t, seed, "config", "user.email", "t@t")
	gitCommit(t, seed, "config", "user.name", "t")
	gitCommit(t, seed, "remote", "add", "origin", origin)

	gitCommit(t, seed, "commit", "-q", "--allow-empty", "-m", "A")
	gitCommit(t, seed, "push", "-q", "origin", "master")

	// D goes to develop only — the mirroring refspec moves one ref and stops.
	gitCommit(t, seed, "commit", "-q", "--allow-empty", "-m", "D")
	gitCommit(t, seed, "push", "-q", "origin", "master:refs/heads/develop")

	gitCommit(t, root, "clone", "-q", origin, work)
	gitCommit(t, work, "config", "user.email", "t@t")
	gitCommit(t, work, "config", "user.name", "t")
	gitCommit(t, work, "reset", "-q", "--hard", "refs/remotes/origin/develop")
	gitCommit(t, work, "checkout", "-q", "-b", "wip", "refs/remotes/origin/master")

	// The rewind reaches the bare repository as an object by push and as a ref
	// move by update-ref. Pushing the ref itself would update this clone's own
	// remote-tracking copy and repair the staleness under test.
	gitCommit(t, seed, "checkout", "-q", "--orphan", "rewrite")
	gitCommit(t, seed, "commit", "-q", "--allow-empty", "-m", "rewritten root")
	gitCommit(t, seed, "push", "-q", "origin", "rewrite:refs/heads/rewrite")
	live := gitOutput(t, seed, "rev-parse", "rewrite")
	gitCommit(t, origin, "update-ref", "refs/heads/develop", live)
	gitCommit(t, origin, "update-ref", "-d", "refs/heads/rewrite")

	return work
}

func TestCleanupService_RetirementRestsOnCacheWhenNoLocalCanonical(t *testing.T) {
	work := staleLocalFixture(t)
	repo := &repository.Repository{Path: work}
	svc := newTestCleanupService(t)
	ctx := context.Background()

	opts := ExecuteOptions{CanonicalBranch: "develop", CanonicalRemote: "origin"}
	local := &Branch{Name: "master", Ref: "refs/heads/master"}

	if !svc.retirementRestsOnCache(ctx, repo, local, opts) {
		t.Fatal("this clone has no refs/heads/develop, so the ancestry rests on a cached ref and must be gated")
	}

	// Give the clone a local canonical branch and the same candidate is no
	// longer cache-authorized: nothing a remote does can move a local ref.
	gitCommit(t, work, "branch", "develop", "refs/remotes/origin/develop")

	if svc.retirementRestsOnCache(ctx, repo, local, opts) {
		t.Error("with a local canonical branch the evidence is local; requiring the network here breaks offline cleanup for no safety gain")
	}
}

func TestCleanupService_ExecuteRefusesLocalDeleteOnRewoundCanonical(t *testing.T) {
	work := staleLocalFixture(t)
	repo := &repository.Repository{Path: work}
	svc := newTestCleanupService(t)
	ctx := context.Background()

	opts := ExecuteOptions{CanonicalBranch: "develop", CanonicalRemote: "origin", Force: true, Confirm: false}
	local := &Branch{Name: "master", Ref: "refs/heads/master"}

	// The stale cache genuinely authorizes this deletion; that is the premise.
	if !svc.authorizeRetire(ctx, repo, local, opts) {
		t.Fatal("fixture is wrong: the cached ancestry must still pass, or the test proves nothing")
	}

	result, err := svc.Execute(ctx, repo, &CleanupReport{NonCanonical: []*Branch{local}}, opts)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(result.Deleted) != 0 {
		t.Errorf("develop was rewound on the remote; deleting master drops D from every ref. deleted=%v", result.Deleted)
	}
	if len(result.Failed) != 1 {
		t.Fatalf("the refusal must be reported, not silently dropped; failed=%v", result.Failed)
	}
	if gitOutput(t, work, "rev-parse", "--verify", "--quiet", "refs/heads/master") == "" {
		t.Error("master was deleted despite the refusal")
	}
}

// A local canonical branch is not a cache, so the retirement must still work
// with no network at all. Without this the fix would trade a data-loss bug for
// a command that cannot run offline.
func TestCleanupService_ExecuteAllowsLocalDeleteAgainstLocalCanonical(t *testing.T) {
	work := staleLocalFixture(t)
	repo := &repository.Repository{Path: work}
	svc := newTestCleanupService(t)
	ctx := context.Background()

	gitCommit(t, work, "branch", "develop", "refs/heads/master")
	gitCommit(t, work, "remote", "remove", "origin")

	opts := ExecuteOptions{CanonicalBranch: "develop", CanonicalRemote: "origin", Force: true}
	local := &Branch{Name: "master", Ref: "refs/heads/master"}

	result, err := svc.Execute(ctx, repo, &CleanupReport{NonCanonical: []*Branch{local}}, opts)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(result.Deleted) != 1 {
		t.Fatalf("a local canonical branch needs no network to authorize; deleted=%v failed=%v", result.Deleted, result.Failed)
	}
}
