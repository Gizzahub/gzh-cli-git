// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import "testing"

// TestMatchTaskPattern is the table test for the namespace-prefix match
// itself: the prefix before a pattern's first '*' is the whole test, per
// DECISION-004 (see MatchTaskPattern's doc comment).
func TestMatchTaskPattern(t *testing.T) {
	tests := []struct {
		name    string
		branch  string
		pattern string
		want    bool
	}{
		{"triple-star matches shallow", "dev/a/b/c", "dev/*/*/*", true},
		{"triple-star matches deeper path", "dev/a/b/c/d", "dev/*/*/*", true},
		{"triple-star rejects different namespace", "hotfix/foo", "dev/*/*/*", false},
		{"bare star matches nothing", "dev/a/b/c", "*", false},
		{"bare star matches nothing even for empty name", "", "*", false},
		{"exact equality matches", "hotfix/urgent", "hotfix/urgent", true},
		{"exact equality is not a prefix match on a longer name", "hotfix/urgent2", "hotfix/urgent", false},
		{"trailing-star prefix matches", "release/1.2.3", "release/*", true},
		{"trailing-star prefix rejects non-matching prefix", "hotfix/1.2.3", "release/*", false},
		{"empty pattern matches nothing but exact empty name", "", "", true},
		{"empty pattern rejects non-empty name", "anything", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchTaskPattern(tt.branch, tt.pattern); got != tt.want {
				t.Errorf("MatchTaskPattern(%q, %q) = %v, want %v", tt.branch, tt.pattern, got, tt.want)
			}
		})
	}
}

// TestMatchesAnyTaskPattern covers the multi-pattern fan-out: a match on any
// declared pattern is enough, and an empty pattern list never matches.
func TestMatchesAnyTaskPattern(t *testing.T) {
	patterns := []string{"hotfix/*", "dev/*/*/*"}

	tests := []struct {
		name   string
		branch string
		want   bool
	}{
		{"matches first pattern", "hotfix/urgent", true},
		{"matches second pattern", "dev/a/b/c", true},
		{"matches second pattern at depth", "dev/a/b/c/d", true},
		{"matches neither pattern", "release/1.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchesAnyTaskPattern(tt.branch, patterns); got != tt.want {
				t.Errorf("MatchesAnyTaskPattern(%q, %v) = %v, want %v", tt.branch, patterns, got, tt.want)
			}
		})
	}

	if MatchesAnyTaskPattern("anything", nil) {
		t.Error("MatchesAnyTaskPattern with no declared patterns must never match")
	}
}
