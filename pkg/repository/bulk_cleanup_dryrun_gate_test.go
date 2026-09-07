// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// The property under test is agreement, not refusal: whatever --dry-run names
// as a deletion, the real run must actually delete. The gate used to live only
// inside the delete, so the preview counted candidates the run then refused --
// the operator approved one thing and received another.
func TestBulkCleanup_DryRunAgreesWithRunOnStaleCanonical(t *testing.T) {
	_, clone, _ := staleCanonicalFixture(t)
	runGit(t, clone, "checkout", "-b", "wip")
	runGit(t, clone, "remote", "set-url", DefaultRemoteName, "/nonexistent/unreachable.git")

	c, ok := NewClient().(*client)
	if !ok {
		t.Fatal("NewClient did not return *client")
	}

	opts := BulkCleanupOptions{
		IncludeNonCanonical: true,
		CanonicalResolver:   developResolver(),
	}

	dryOpts := opts
	dryOpts.DryRun = true
	preview := c.processCleanupRepository(context.Background(), filepath.Dir(clone), clone, dryOpts, NewNoopLogger())

	if len(preview.DeletedBranches) != 0 {
		t.Errorf(
			"the remote is unreachable, so the run will refuse every non-canonical candidate;"+
				" the preview must not offer any. would-delete=%v",
			preview.DeletedBranches,
		)
	}
	if len(preview.FailedBranches) == 0 {
		t.Fatal("a refusal the run will hit must appear in the preview, not be discovered afterwards")
	}
	if !strings.Contains(preview.FailedBranches[0].Error, "develop") {
		t.Errorf("the preview refusal must name the branch that could not be confirmed; got %q",
			preview.FailedBranches[0].Error)
	}
	if preview.Status != StatusError {
		t.Errorf(
			"nothing would be deleted and something is blocked; %q reads as an approvable plan. got %q",
			StatusWouldCleanup, preview.Status,
		)
	}

	executed := c.processCleanupRepository(context.Background(), filepath.Dir(clone), clone, opts, NewNoopLogger())

	if len(executed.DeletedBranches) != len(preview.DeletedBranches) {
		t.Errorf("preview promised %d deletion(s), run performed %d",
			len(preview.DeletedBranches), len(executed.DeletedBranches))
	}
	if len(executed.FailedBranches) != len(preview.FailedBranches) {
		t.Errorf("preview reported %d refusal(s), run reported %d",
			len(preview.FailedBranches), len(executed.FailedBranches))
	}
	if !refExists(t, clone, "refs/heads/master") {
		t.Error("master was deleted despite both paths refusing")
	}
}

// The screen must not swallow the run's own result. A repository whose
// candidates all pass the gate has to preview and delete exactly as before --
// otherwise the fix trades a reporting bug for a cleanup that never fires.
func TestBulkCleanup_DryRunAgreesWithRunOnLocalCanonical(t *testing.T) {
	_, clone, _ := staleCanonicalFixture(t)
	runGit(t, clone, "branch", "develop", "refs/heads/master")
	runGit(t, clone, "checkout", "-b", "wip")
	runGit(t, clone, "remote", "set-url", DefaultRemoteName, "/nonexistent/unreachable.git")

	c, ok := NewClient().(*client)
	if !ok {
		t.Fatal("NewClient did not return *client")
	}

	opts := BulkCleanupOptions{
		IncludeNonCanonical: true,
		CanonicalResolver:   developResolver(),
	}

	dryOpts := opts
	dryOpts.DryRun = true
	preview := c.processCleanupRepository(context.Background(), filepath.Dir(clone), clone, dryOpts, NewNoopLogger())

	if len(preview.DeletedBranches) == 0 {
		t.Fatal("master is an ancestor of the local develop; the preview must offer it")
	}
	if len(preview.FailedBranches) != 0 {
		t.Errorf("a local canonical branch needs no network; nothing should be blocked. got %v",
			preview.FailedBranches)
	}
	if preview.Status != StatusWouldCleanup {
		t.Errorf("expected %q, got %q", StatusWouldCleanup, preview.Status)
	}
	if !refExists(t, clone, "refs/heads/master") {
		t.Fatal("a dry run must not delete anything")
	}

	executed := c.processCleanupRepository(context.Background(), filepath.Dir(clone), clone, opts, NewNoopLogger())

	if len(executed.DeletedBranches) != len(preview.DeletedBranches) {
		t.Errorf("preview promised %d deletion(s), run performed %d",
			len(preview.DeletedBranches), len(executed.DeletedBranches))
	}
	if refExists(t, clone, "refs/heads/master") {
		t.Error("master should have been retired")
	}
}

// needsCanonicalTipCheck is the single predicate all three call sites share, so
// its boundaries are worth pinning directly: a screen narrower than the deletes
// reopens the disagreement, and a wider one refuses in preview what the run
// would do.
func TestNeedsCanonicalTipCheck(t *testing.T) {
	cases := []struct {
		name string
		b    branchInfo
		want bool
	}{
		{
			name: "remote non-canonical is always measured against the cache",
			b:    branchInfo{reason: nonCanonicalReason, location: branchLocationRemote},
			want: true,
		},
		{
			name: "remote non-canonical with no recorded basis is refused, not waved through",
			b:    branchInfo{reason: nonCanonicalReason, location: branchLocationRemote, canonical: ""},
			want: true,
		},
		{
			name: "local non-canonical that fell back to the cache",
			b: branchInfo{
				reason: nonCanonicalReason, location: branchLocationLocal, canonical: "develop",
			},
			want: true,
		},
		{
			name: "local non-canonical measured against a local ref needs no network",
			b:    branchInfo{reason: nonCanonicalReason, location: branchLocationLocal, canonical: ""},
			want: false,
		},
		{
			name: "merged does not draw its authority from a declaration",
			b:    branchInfo{reason: "merged", location: branchLocationLocal, canonical: "develop"},
			want: false,
		},
		{
			name: "gone remote candidate is not a non-canonical retirement",
			b:    branchInfo{reason: "gone", location: branchLocationRemote},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := needsCanonicalTipCheck(tc.b); got != tc.want {
				t.Errorf("needsCanonicalTipCheck(%+v) = %v, want %v", tc.b, got, tc.want)
			}
		})
	}
}
