// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"strconv"
	"strings"
	"time"
)

// StrandedStashAge is how long a stash may sit before it stops counting as work
// in progress. A handoff cycle runs in a day or so, so an entry that survived a
// week survived several of them without anyone reaching for it.
//
// It lives here rather than in either caller because "handoff check" and
// "doctor" have to agree: one calling a stash stranded while the other calls it
// normal is worse than either threshold on its own.
const StrandedStashAge = 7 * 24 * time.Hour

// StashIsStranded reports whether the oldest entry of a repository's stash has
// outlived the task that created it. A zero time means there is no stash.
func StashIsStranded(oldest time.Time) bool {
	return !oldest.IsZero() && time.Since(oldest) >= StrandedStashAge
}

// ParseStashDates reads the output of `git stash list --format=%ct` and returns
// how many entries there are and when the oldest was created.
//
// git lists stashes newest first, but that ordering is a property of the reflog
// rather than of the dates: `git stash push` on an older base, or a reordering
// drop, can leave an entry whose date is not where its position suggests. So the
// oldest is found by comparing, not by taking the last line.
func ParseStashDates(output string) (count int, oldest time.Time) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return 0, time.Time{}
	}

	for line := range strings.SplitSeq(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		count++

		// An unparseable date still counts as an entry: losing the age of one
		// stash is not a reason to under-report how much is stranded here.
		seconds, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			continue
		}

		when := time.Unix(seconds, 0)
		if oldest.IsZero() || when.Before(oldest) {
			oldest = when
		}
	}

	return count, oldest
}
