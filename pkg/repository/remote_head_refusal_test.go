// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import "testing"

// TestIsRemoteHeadRefusal is the table test for the pure string classification
// IsRemoteHeadRefusal performs: turning git's (or a forge's) denyDeleteCurrent
// wording into one actionable signal, without over-matching unrelated remote
// delete failures.
func TestIsRemoteHeadRefusal(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		want   bool
	}{
		{
			"git's own denyDeleteCurrent wording",
			"! [remote rejected] master (refusing to delete the current branch: refs/heads/master)",
			true,
		},
		{
			"case-insensitive match",
			"REFUSING TO DELETE THE CURRENT BRANCH",
			true,
		},
		{
			"alternate deletion-prohibited wording",
			"remote: error: deletion of the current branch prohibited",
			true,
		},
		{
			"forge wording naming the default branch",
			"protected branch hook declined: cannot delete the default branch",
			true,
		},
		{
			"unrelated permission failure",
			"remote: Permission to org/repo.git denied to user",
			false,
		},
		{
			"unrelated network failure",
			"fatal: unable to access 'https://example.com/repo.git/': Could not resolve host",
			false,
		},
		{
			"empty stderr",
			"",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRemoteHeadRefusal(tt.stderr); got != tt.want {
				t.Errorf("IsRemoteHeadRefusal(%q) = %v, want %v", tt.stderr, got, tt.want)
			}
		})
	}
}
