// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"strings"
	"testing"
)

// A clone made while origin's HEAD was master has a local master and no local
// develop, so its local candidates are measured against
// refs/remotes/origin/develop — a cache. Refreshing that ref is not something
// --remote earns: without it, the invocation that deletes nothing on the remote
// is the one that deletes a local branch on evidence the remote has retracted.
func TestBulkCleanup_NonCanonicalLocalOnlyRefreshesCanonical(t *testing.T) {
	_, clone, _ := staleCanonicalFixture(t)
	runGit(t, clone, "checkout", "-b", "wip")

	c, ok := NewClient().(*client)
	if !ok {
		t.Fatal("NewClient did not return *client")
	}

	result := &RepositoryCleanupResult{}
	opts := BulkCleanupOptions{
		IncludeNonCanonical: true,
		DeleteRemote:        false,
		CanonicalResolver:   developResolver(),
	}

	toDelete := c.collectCleanupCandidates(
		context.Background(), clone, "develop", DefaultRemoteName, "wip", opts, result,
	)

	for _, b := range toDelete {
		if b.name == "master" && b.location == branchLocationLocal {
			t.Error("develop was rewound on the remote; local master is not contained in it and must not be classified")
		}
	}
}

// The refresh is best effort, so an unreachable remote leaves the stale cache in
// place and the classification still passes. The delete is where that must be
// caught: refusing there is what keeps "cannot verify" from being spelled the
// same way as "verified safe".
func TestBulkCleanup_NonCanonicalLocalDeleteRefusesUnverifiableCanonical(t *testing.T) {
	_, clone, _ := staleCanonicalFixture(t)
	runGit(t, clone, "checkout", "-b", "wip")
	runGit(t, clone, "remote", "set-url", DefaultRemoteName, "/nonexistent/unreachable.git")

	c, ok := NewClient().(*client)
	if !ok {
		t.Fatal("NewClient did not return *client")
	}

	ctx := context.Background()
	result := &RepositoryCleanupResult{}
	opts := BulkCleanupOptions{
		IncludeNonCanonical: true,
		DeleteRemote:        false,
		CanonicalResolver:   developResolver(),
	}

	toDelete := c.collectCleanupCandidates(ctx, clone, "develop", DefaultRemoteName, "wip", opts, result)

	var candidate *branchInfo
	for i := range toDelete {
		if toDelete[i].name == "master" && toDelete[i].location == branchLocationLocal {
			candidate = &toDelete[i]
		}
	}
	if candidate == nil {
		t.Fatal("fixture is wrong: with the remote unreachable the stale cache must still classify master, or the delete gate is never reached")
	}
	if candidate.canonical != "develop" {
		t.Errorf("a candidate measured against the cached ref must carry the canonical basis; got %q", candidate.canonical)
	}

	err := c.deleteCleanupBranch(ctx, clone, DefaultRemoteName, *candidate)
	if err == nil {
		t.Fatal("the canonical branch could not be confirmed; the deletion must be refused rather than assumed safe")
	}
	if !strings.Contains(err.Error(), "develop") {
		t.Errorf("the refusal must name the branch whose authority could not be confirmed; got: %v", err)
	}
	if !refExists(t, clone, "refs/heads/master") {
		t.Error("master was deleted despite the refusal")
	}
}

// A local canonical branch is owned by this clone, so no remote can invalidate
// it and the retirement must still work with the network gone. Without this the
// gate would make offline cleanup impossible rather than merely honest.
func TestBulkCleanup_NonCanonicalLocalDeleteAllowedAgainstLocalCanonical(t *testing.T) {
	_, clone, _ := staleCanonicalFixture(t)
	runGit(t, clone, "branch", "develop", "refs/heads/master")
	runGit(t, clone, "checkout", "-b", "wip")
	runGit(t, clone, "remote", "set-url", DefaultRemoteName, "/nonexistent/unreachable.git")

	c, ok := NewClient().(*client)
	if !ok {
		t.Fatal("NewClient did not return *client")
	}

	ctx := context.Background()
	result := &RepositoryCleanupResult{}
	opts := BulkCleanupOptions{
		IncludeNonCanonical: true,
		DeleteRemote:        false,
		CanonicalResolver:   developResolver(),
	}

	toDelete := c.collectCleanupCandidates(ctx, clone, "develop", DefaultRemoteName, "wip", opts, result)

	var candidate *branchInfo
	for i := range toDelete {
		if toDelete[i].name == "master" && toDelete[i].location == branchLocationLocal {
			candidate = &toDelete[i]
		}
	}
	if candidate == nil {
		t.Fatal("master is an ancestor of the local develop and must be classified")
	}
	if candidate.canonical != "" {
		t.Errorf("evidence from a local ref must not be marked as cached; got %q", candidate.canonical)
	}

	if err := c.deleteCleanupBranch(ctx, clone, DefaultRemoteName, *candidate); err != nil {
		t.Fatalf("a local canonical branch needs no network to authorize: %v", err)
	}
	if refExists(t, clone, "refs/heads/master") {
		t.Error("master should have been retired")
	}
}
