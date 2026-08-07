// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package gitsettings

import (
	"strconv"
	"strings"
)

// recommended is the curated set of settings for multi-device work.
// Order is meaningful: it is the order shown to the user.
var recommended = []Setting{
	{
		Key:  "pull.rebase",
		Want: "true",
		Why:  "pull replays local commits instead of creating merge commits",
	},
	{
		Key:  "rebase.autoStash",
		Want: "true",
		Why:  "rebase stashes and restores local changes instead of aborting",
	},
	{
		Key:  "fetch.prune",
		Want: "true",
		Why:  "fetch drops references to branches deleted on the remote",
	},
	{
		Key:    "push.autoSetupRemote",
		Want:   "true",
		Why:    "first push of a new branch sets its upstream automatically",
		MinGit: "2.37",
	},
	{
		Key:  "push.default",
		Want: "current",
		Why:  "push targets the same-named remote branch, never an unrelated one",
	},
	{
		Key:  "rerere.enabled",
		Want: "true",
		Why:  "a conflict resolved once is replayed automatically next time",
	},
	{
		Key:    "merge.conflictStyle",
		Want:   "zdiff3",
		Why:    "conflict markers include the merge base, making resolution decidable",
		MinGit: "2.35",
	},
}

// Recommended returns a copy of the recommended settings.
func Recommended() []Setting {
	out := make([]Setting, len(recommended))
	copy(out, recommended)
	return out
}

// booleanAliases maps the spellings git accepts for booleans onto the
// canonical value, so "yes" is not reported as a mismatch against "true".
var booleanAliases = map[string]string{
	"true": "true", "yes": "true", "on": "true", "1": "true",
	"false": "false", "no": "false", "off": "false", "0": "false",
}

// valuesEqual compares a configured value against a wanted value, treating the
// boolean spellings git accepts as equivalent.
func valuesEqual(current, want string) bool {
	current = strings.TrimSpace(current)
	if strings.EqualFold(current, want) {
		return true
	}

	wantCanonical, wantIsBool := booleanAliases[strings.ToLower(want)]
	if !wantIsBool {
		return false
	}

	currentCanonical, currentIsBool := booleanAliases[strings.ToLower(current)]
	return currentIsBool && currentCanonical == wantCanonical
}

// versionAtLeast reports whether the installed git version is at least min.
// Both values are dotted numeric strings; trailing non-numeric segments (as in
// "2.39.5.windows.1") are ignored. An unparseable version is treated as
// new enough — refusing to write a setting because the version string was odd
// is worse than letting git reject the value itself.
func versionAtLeast(version, minimum string) bool {
	have := parseVersion(version)
	want := parseVersion(minimum)
	if len(have) == 0 || len(want) == 0 {
		return true
	}

	for i := range want {
		if i >= len(have) {
			return false
		}
		if have[i] != want[i] {
			return have[i] > want[i]
		}
	}
	return true
}

// parseVersion converts "2.40.1" into [2, 40, 1], stopping at the first
// segment that is not a number.
func parseVersion(version string) []int {
	var parts []int
	for segment := range strings.SplitSeq(strings.TrimSpace(version), ".") {
		n, err := strconv.Atoi(segment)
		if err != nil {
			break
		}
		parts = append(parts, n)
	}
	return parts
}
