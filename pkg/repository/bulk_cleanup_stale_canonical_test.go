// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)

// staleCanonicalFixture builds a clone whose remote-tracking snapshot says a
// duplicate trunk is safe to retire, and a remote that no longer agrees.
//
// master holds commit A. develop was built on top of A, so origin/master looked
// like a lossless duplicate of the canonical branch. Then develop is rewound on
// the bare remote onto a root that never contained A. The clone is not fetched
// afterwards — that gap is the whole defect.
//
// The rewind is applied to the bare repository directly. Pushing it from a
// working clone would update that clone's own remote-tracking ref, and this
// fixture exists precisely to keep one stale.
func staleCanonicalFixture(t *testing.T) (origin, clone, liveDevelop string) {
	t.Helper()

	seed := testutil.TempGitRepoWithCommit(t)
	runGit(t, seed, "branch", "-M", "master")
	commitFile(t, seed, "a.txt") // A: the commit only master will hold

	runGit(t, seed, "checkout", "-b", "develop")
	commitFile(t, seed, "c.txt")
	runGit(t, seed, "checkout", "master")

	// An unrelated history, to become develop's new tip on the remote.
	runGit(t, seed, "checkout", "--orphan", "rewrite")
	runGit(t, seed, "rm", "-r", "-f", "-q", ".")
	commitFile(t, seed, "rewritten.txt")
	liveDevelop = gitOut(t, seed, "rev-parse", "HEAD")
	runGit(t, seed, "checkout", "master")

	root := t.TempDir()
	origin = filepath.Join(root, "origin.git")
	clone = filepath.Join(root, "clone")
	runGit(t, "", "clone", "--bare", seed, origin)
	runGit(t, "", "clone", origin, clone)
	runGit(t, clone, "config", "user.email", "test@test.com")
	runGit(t, clone, "config", "user.name", "Test")
	runGit(t, clone, "config", "commit.gpgsign", "false")

	// The clone's snapshot is taken. Now the remote moves under it.
	//
	// origin's HEAD is repointed first so that the only thing standing between
	// these tests and a deleted master is the freshness gate. Left on master,
	// the remote would refuse the deletion on its own and the tests would pass
	// whether or not the gate exists.
	runGit(t, origin, "symbolic-ref", "HEAD", "refs/heads/develop")
	runGit(t, origin, "update-ref", "refs/heads/develop", liveDevelop)
	runGit(t, origin, "update-ref", "-d", "refs/heads/rewrite")

	return origin, clone, liveDevelop
}

func canonicalResolver(name string) func(context.Context, string) (string, []string, error) {
	return func(context.Context, string) (string, []string, error) {
		return name, nil, nil
	}
}

// Classification must not trust the snapshot. Refreshing the canonical branch's
// remote-tracking ref is what tells "the trunk advanced" (still a lossless
// retirement) apart from "the trunk was rewound" (the evidence is void), and
// ls-remote cannot: it returns an object id without the object.
func TestBulkCleanup_NonCanonicalRefusesRewoundCanonical(t *testing.T) {
	origin, clone, _ := staleCanonicalFixture(t)
	c, ok := NewClient().(*client)
	if !ok {
		t.Fatal("NewClient did not return *client")
	}

	ctx := context.Background()
	result := &RepositoryCleanupResult{}
	opts := BulkCleanupOptions{
		IncludeNonCanonical: true,
		DeleteRemote:        true,
		CanonicalResolver:   canonicalResolver("develop"),
	}

	toDelete := c.collectCleanupCandidates(ctx, clone, "develop", DefaultRemoteName, "develop", opts, result)

	for _, b := range toDelete {
		if b.name == "master" && b.location == branchLocationRemote {
			t.Error("origin/master holds a commit the rewound develop does not; it must not be classified for deletion")
		}
	}

	if !refExists(t, origin, "refs/heads/master") {
		t.Fatal("fixture lost the remote master")
	}
}

// The delete path re-checks independently. Classification may have happened
// minutes earlier, and --force-with-lease cannot close this gap: the lease
// names the branch being deleted, never the branch whose ancestry authorized it.
func TestBulkCleanup_NonCanonicalRemoteDeleteRefusesMovedCanonical(t *testing.T) {
	origin, clone, liveDevelop := staleCanonicalFixture(t)
	c, ok := NewClient().(*client)
	if !ok {
		t.Fatal("NewClient did not return *client")
	}

	ctx := context.Background()
	staleDevelop := gitOut(t, clone, "rev-parse", "refs/remotes/origin/develop")
	if staleDevelop == liveDevelop {
		t.Fatal("fixture did not leave the clone's snapshot stale")
	}

	// Exactly what a pre-refresh classification would have produced.
	candidate := branchInfo{
		name:         "master",
		reason:       nonCanonicalReason,
		location:     branchLocationRemote,
		sha:          gitOut(t, clone, "rev-parse", "refs/remotes/origin/master"),
		canonical:    "develop",
		canonicalSHA: staleDevelop,
	}

	deleted, failed := c.executeCleanupDeletes(
		ctx, clone, DefaultRemoteName, []branchInfo{candidate}, NewNoopLogger(), "clone",
	)

	if len(deleted) != 0 {
		t.Errorf("nothing may be deleted once the authorizing branch has moved; deleted %+v", deleted)
	}
	if len(failed) != 1 {
		t.Fatalf("the refusal must be reported, not silently withheld; failures = %+v", failed)
	}
	if !strings.Contains(failed[0].Error, "develop") {
		t.Errorf("the refusal must name the branch that moved; got %q", failed[0].Error)
	}
	if !refExists(t, origin, "refs/heads/master") {
		t.Fatal("origin/master was deleted on evidence that no longer held")
	}
}

// A candidate carrying no record of what authorized it is refused rather than
// pushed. CleanupReport's bulk equivalent is assembled in-process, but the zero
// value must still fail closed.
func TestBulkCleanup_NonCanonicalRemoteDeleteRefusesMissingEvidence(t *testing.T) {
	origin, clone, _ := staleCanonicalFixture(t)
	c, ok := NewClient().(*client)
	if !ok {
		t.Fatal("NewClient did not return *client")
	}

	candidate := branchInfo{
		name:     "master",
		reason:   nonCanonicalReason,
		location: branchLocationRemote,
		sha:      gitOut(t, clone, "rev-parse", "refs/remotes/origin/master"),
	}

	_, failed := c.executeCleanupDeletes(
		context.Background(), clone, DefaultRemoteName, []branchInfo{candidate}, NewNoopLogger(), "clone",
	)

	if len(failed) != 1 {
		t.Fatalf("expected a refusal, got %+v", failed)
	}
	if !refExists(t, origin, "refs/heads/master") {
		t.Fatal("origin/master was deleted with no record of what authorized it")
	}
}
