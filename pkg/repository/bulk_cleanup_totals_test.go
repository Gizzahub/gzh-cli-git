// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import "testing"

// TestSumCleanupTotals_CountsDeletionsNotCandidates is the regression for a
// bulk run that reported every candidate as deleted.
//
// The per-reason counters (MergedCount, NonCanonicalCount, …) are incremented
// while candidates are collected, before anything is deleted. Summing them told
// the operator "Deleted: 4" for a run in which git refused two of the four —
// the remote's default branch cannot be deleted — leaving the duplicate
// branches in place and the report claiming the tree was clean.
func TestSumCleanupTotals_CountsDeletionsNotCandidates(t *testing.T) {
	results := []RepositoryCleanupResult{
		{
			// Two candidates found, one deleted, one refused by the remote.
			NonCanonicalCount: 2,
			TotalAnalyzed:     7,
			Branches:          []CleanupBranchEntry{{Name: "master", Location: "local"}},
			FailedBranches: []CleanupFailureEntry{
				{Name: "master", Location: "remote", Error: "refused because it is still origin's default branch"},
			},
		},
		{
			MergedCount:   3,
			TotalAnalyzed: 5,
			Branches:      []CleanupBranchEntry{{Name: "feature/a", Location: "local"}},
		},
	}

	deleted, failed, analyzed := sumCleanupTotals(results)

	if deleted != 2 {
		t.Errorf("deleted = %d, want 2 (the counters would have said 5)", deleted)
	}
	if failed != 1 {
		t.Errorf("failed = %d, want 1", failed)
	}
	if analyzed != 12 {
		t.Errorf("analyzed = %d, want 12", analyzed)
	}
}

// TestSumCleanupTotals_Empty pins the zero case, since a scan that matched no
// repository must not report a deletion.
func TestSumCleanupTotals_Empty(t *testing.T) {
	deleted, failed, analyzed := sumCleanupTotals(nil)
	if deleted != 0 || failed != 0 || analyzed != 0 {
		t.Errorf("sumCleanupTotals(nil) = (%d, %d, %d), want (0, 0, 0)", deleted, failed, analyzed)
	}
}

// TestFirstLine keeps a multi-line git error to the one line a bulk summary can
// show, without swallowing the empty case into a blank message.
func TestFirstLine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"single line", "fatal: no such ref", "fatal: no such ref"},
		{"multi line keeps the first", "remote: error: denied\nremote: hint: see docs\n", "remote: error: denied"},
		{"blank becomes a named error", "   \n  ", "unknown error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstLine(tt.in); got != tt.want {
				t.Errorf("firstLine(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
