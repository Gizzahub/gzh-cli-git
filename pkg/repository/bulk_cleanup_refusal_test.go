// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"strings"
	"testing"
)

// The bulk engine's silent drop, measured rather than argued.
//
// staleCanonicalFixture is the shape where the refresh *succeeds* and the
// ancestry then honestly fails: develop was rewound on the remote, so the local
// master it once contained is no longer contained in it. The classification is
// right to pass master over. What was wrong is that passing it over left no
// trace anywhere — local candidates come from `git branch --merged <target>`,
// so the one branch worth reporting is absent from the input by construction,
// and the run reported "No branches to clean up" in a repository that still has
// a duplicate trunk sitting in it.
func TestBulkCleanup_DeclinedTrunkIsRecordedNotDropped(t *testing.T) {
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
			t.Fatal("fixture is wrong: master must not be a candidate here, or there is no silent drop to report")
		}
	}

	var got *RetireRefusalEntry
	for i := range result.RetireRefusals {
		if result.RetireRefusals[i].Name == "master" && result.RetireRefusals[i].Location == branchLocationLocal {
			got = &result.RetireRefusals[i]
		}
	}
	if got == nil {
		t.Fatalf("master was examined and declined but nothing was recorded (%d refusals)", len(result.RetireRefusals))
	}
	// The reason is the whole remedy. A bare "declined" would pass a presence
	// check and leave the operator exactly where the silence did.
	if !strings.Contains(got.Reason, "develop") {
		t.Errorf("refusal does not name what it measured against: %q", got.Reason)
	}
	if !strings.Contains(got.Reason, "holds commits") {
		t.Errorf("refusal does not say what is wrong with the branch: %q", got.Reason)
	}
}

// A refusal is not a failure. FailedBranches feeds TotalBranchesFailed and the
// non-zero exit, so folding these in would turn a repository that is behaving
// correctly into a failing one — the reason the two lists are separate.
func TestBulkCleanup_DeclinedTrunkIsNotCountedAsFailure(t *testing.T) {
	_, clone, _ := staleCanonicalFixture(t)
	runGit(t, clone, "checkout", "-b", "wip")

	c, ok := NewClient().(*client)
	if !ok {
		t.Fatal("NewClient did not return *client")
	}

	result := &RepositoryCleanupResult{}
	c.collectCleanupCandidates(
		context.Background(), clone, "develop", DefaultRemoteName, "wip",
		BulkCleanupOptions{
			IncludeNonCanonical: true,
			CanonicalResolver:   developResolver(),
		},
		result,
	)

	if len(result.RetireRefusals) == 0 {
		t.Fatal("fixture produced no refusal, so this asserts nothing")
	}
	if len(result.FailedBranches) != 0 {
		t.Errorf("a classification refusal must not appear as an attempted delete: %+v", result.FailedBranches)
	}
	if result.NonCanonicalCount != 0 {
		t.Errorf("a declined trunk must not be counted as retired, got %d", result.NonCanonicalCount)
	}
}
