// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package reposync

import (
	"strings"
	"testing"
)

func TestClassifyDivergence(t *testing.T) {
	tests := []struct {
		name     string
		health   RepoHealth
		expected DivergenceType
	}{
		{
			name: "no upstream",
			health: RepoHealth{
				UpstreamBranch: "",
			},
			expected: DivergenceNoUpstream,
		},
		{
			name: "has conflicts",
			health: RepoHealth{
				UpstreamBranch: "origin/main",
				ConflictFiles:  1,
			},
			expected: DivergenceConflict,
		},
		{
			name: "up to date",
			health: RepoHealth{
				UpstreamBranch: "origin/main",
				AheadBy:        0,
				BehindBy:       0,
			},
			expected: DivergenceNone,
		},
		{
			name: "behind remote (fast-forward)",
			health: RepoHealth{
				UpstreamBranch: "origin/main",
				AheadBy:        0,
				BehindBy:       5,
			},
			expected: DivergenceFastForward,
		},
		{
			name: "ahead of remote",
			health: RepoHealth{
				UpstreamBranch: "origin/main",
				AheadBy:        3,
				BehindBy:       0,
			},
			expected: DivergenceAhead,
		},
		{
			name: "diverged (ahead and behind)",
			health: RepoHealth{
				UpstreamBranch: "origin/main",
				AheadBy:        2,
				BehindBy:       3,
			},
			expected: DivergenceDiverged,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyDivergence(tt.health)
			if got != tt.expected {
				t.Errorf("classifyDivergence() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestClassifyHealth(t *testing.T) {
	tests := []struct {
		name     string
		health   RepoHealth
		expected HealthStatus
	}{
		{
			name: "network timeout",
			health: RepoHealth{
				NetworkStatus: NetworkTimeout,
			},
			expected: HealthUnreachable,
		},
		{
			name: "network unreachable",
			health: RepoHealth{
				NetworkStatus: NetworkUnreachable,
			},
			expected: HealthUnreachable,
		},
		{
			name: "auth failed (warning, not unreachable)",
			health: RepoHealth{
				NetworkStatus:  NetworkAuthFailed,
				WorkTreeStatus: WorkTreeClean,
			},
			expected: HealthWarning,
		},
		{
			name: "conflict in working tree",
			health: RepoHealth{
				NetworkStatus:  NetworkOK,
				WorkTreeStatus: WorkTreeConflict,
			},
			expected: HealthError,
		},
		{
			name: "dirty and behind (error)",
			health: RepoHealth{
				NetworkStatus:  NetworkOK,
				WorkTreeStatus: WorkTreeDirty,
				BehindBy:       5,
			},
			expected: HealthError,
		},
		{
			name: "diverged (warning)",
			health: RepoHealth{
				NetworkStatus:  NetworkOK,
				WorkTreeStatus: WorkTreeClean,
				DivergenceType: DivergenceDiverged,
			},
			expected: HealthWarning,
		},
		{
			name: "behind (warning)",
			health: RepoHealth{
				NetworkStatus:  NetworkOK,
				WorkTreeStatus: WorkTreeClean,
				DivergenceType: DivergenceFastForward,
			},
			expected: HealthWarning,
		},
		{
			name: "ahead (warning)",
			health: RepoHealth{
				NetworkStatus:  NetworkOK,
				WorkTreeStatus: WorkTreeClean,
				DivergenceType: DivergenceAhead,
			},
			expected: HealthWarning,
		},
		{
			name: "dirty but not behind (warning)",
			health: RepoHealth{
				NetworkStatus:  NetworkOK,
				WorkTreeStatus: WorkTreeDirty,
				DivergenceType: DivergenceNone,
				ModifiedFiles:  3,
			},
			expected: HealthWarning,
		},
		{
			name: "healthy (up to date and clean)",
			health: RepoHealth{
				NetworkStatus:  NetworkOK,
				WorkTreeStatus: WorkTreeClean,
				DivergenceType: DivergenceNone,
			},
			expected: HealthHealthy,
		},
		{
			// "Could not read the tree" is not "the tree is fine". Every verdict
			// below the WorkTreeUnknown check rests on status output we do not
			// have, so no repository in this state may be called healthy.
			name: "working tree unreadable (error, never healthy)",
			health: RepoHealth{
				NetworkStatus:  NetworkOK,
				WorkTreeStatus: WorkTreeUnknown,
				DivergenceType: DivergenceNone,
			},
			expected: HealthError,
		},
		{
			// Outranks the divergence warnings that follow it, for the same reason.
			name: "working tree unreadable outranks divergence",
			health: RepoHealth{
				NetworkStatus:  NetworkOK,
				WorkTreeStatus: WorkTreeUnknown,
				DivergenceType: DivergenceAhead,
				AheadBy:        2,
			},
			expected: HealthError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyHealth(tt.health)
			if got != tt.expected {
				t.Errorf("classifyHealth() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGenerateRecommendation(t *testing.T) {
	tests := []struct {
		name     string
		health   RepoHealth
		contains string // substring that should appear in recommendation
	}{
		{
			name: "unreachable",
			health: RepoHealth{
				HealthStatus: HealthUnreachable,
			},
			contains: "network",
		},
		{
			name: "conflict",
			health: RepoHealth{
				HealthStatus:   HealthError,
				WorkTreeStatus: WorkTreeConflict,
			},
			contains: "conflict",
		},
		{
			name: "dirty and behind",
			health: RepoHealth{
				HealthStatus:   HealthError,
				WorkTreeStatus: WorkTreeDirty,
				BehindBy:       5,
				ModifiedFiles:  3,
			},
			contains: "stash",
		},
		{
			name: "auth failed",
			health: RepoHealth{
				HealthStatus:  HealthWarning,
				NetworkStatus: NetworkAuthFailed,
			},
			contains: "credentials",
		},
		{
			name: "fast-forward",
			health: RepoHealth{
				HealthStatus:   HealthWarning,
				DivergenceType: DivergenceFastForward,
				BehindBy:       3,
			},
			contains: "Pull",
		},
		{
			name: "diverged",
			health: RepoHealth{
				HealthStatus:   HealthWarning,
				DivergenceType: DivergenceDiverged,
				AheadBy:        2,
				BehindBy:       3,
			},
			contains: "rebase",
		},
		{
			name: "ahead",
			health: RepoHealth{
				HealthStatus:   HealthWarning,
				DivergenceType: DivergenceAhead,
				AheadBy:        2,
			},
			contains: "Push",
		},
		{
			name: "dirty only",
			health: RepoHealth{
				HealthStatus:   HealthWarning,
				WorkTreeStatus: WorkTreeDirty,
				DivergenceType: DivergenceNone,
				ModifiedFiles:  3,
			},
			contains: "Uncommitted",
		},
		{
			name: "healthy",
			health: RepoHealth{
				HealthStatus: HealthHealthy,
			},
			contains: "No action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateRecommendation(tt.health)
			if got == "" {
				t.Errorf("generateRecommendation() returned empty string")
			}
			if tt.contains != "" && !strings.Contains(got, tt.contains) {
				t.Errorf("generateRecommendation() = %q, want to contain %q", got, tt.contains)
			}
		})
	}
}
