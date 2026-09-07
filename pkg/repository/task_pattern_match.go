// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import "strings"

// MatchTaskPattern reports whether name falls inside the task-branch namespace a
// declared pattern opens. The namespace is the prefix before the first '*', so
// "dev/*/*/*" matches "dev/a/b/c" and equally "dev/a/b/c/d" (DECISION-004). A
// trailing-'*' compare on the whole pattern would read the middle stars as
// literal characters and miss every real task branch.
//
// Pattern "*" (empty prefix) never matches; the loader rejects it.
//
// Ownership lives here rather than in pkg/config because two callers need it and
// the dependency runs config → branch → repository: pkg/branch cannot import
// pkg/config without a cycle. This mirrors how IsProtected is owned — the lower
// package holds the judgment, the higher ones delegate, and neither forks the
// semantics.
func MatchTaskPattern(name, pattern string) bool {
	if pattern == name {
		return true
	}
	star := strings.IndexByte(pattern, '*')
	if star <= 0 {
		return false
	}
	return strings.HasPrefix(name, pattern[:star])
}

// MatchesAnyTaskPattern reports whether name matches any declared pattern.
func MatchesAnyTaskPattern(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if MatchTaskPattern(name, pattern) {
			return true
		}
	}
	return false
}
