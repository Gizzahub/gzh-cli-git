// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

// ProtectedBranches lists the branch names and patterns that require --force to
// delete. It is the single source of truth for protected-branch judgment: both
// the single-repo path (pkg/branch) and the bulk path (this package) resolve
// protection through IsProtected, so adding a pattern here closes it on every
// deletion path at once.
var ProtectedBranches = []string{
	"main",
	"master",
	"develop",
	"development",
	"release/*",
	"hotfix/*",
}

// IsProtected reports whether name matches a built-in protected branch pattern.
func IsProtected(name string) bool {
	for _, pattern := range ProtectedBranches {
		if matchBranchPattern(name, pattern) {
			return true
		}
	}
	return false
}

// matchBranchPattern checks if name matches pattern (supports a trailing *
// wildcard, e.g. "release/*").
func matchBranchPattern(name, pattern string) bool {
	if pattern == name {
		return true
	}
	if pattern != "" && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(name) >= len(prefix) && name[:len(prefix)] == prefix
	}
	return false
}

// RetirableTrunkNames lists the built-in protected names that a canonical-branch
// declaration is allowed to overrule.
//
// It is deliberately narrower than ProtectedBranches, and deliberately a list of
// its own rather than "ProtectedBranches minus a couple of patterns": the two
// lists answer different questions, and deriving one from the other would widen
// this one silently the next time a pattern is added above.
//
// release/* and hotfix/* are absent on purpose. They are protected for a
// different reason than the trunk names are — a release line is *expected* to be
// an ancestor of the trunk, which is exactly the shape the non-canonical gate
// looks for, so every merged release branch would qualify. Retiring those is a
// release-policy decision, not a duplicate-trunk cleanup.
var RetirableTrunkNames = []string{
	"main",
	"master",
	"develop",
	"development",
}

// IsRetirableTrunkName reports whether a declared canonical branch may overrule
// built-in protection for this name.
func IsRetirableTrunkName(name string) bool {
	for _, candidate := range RetirableTrunkNames {
		if name == candidate {
			return true
		}
	}
	return false
}
