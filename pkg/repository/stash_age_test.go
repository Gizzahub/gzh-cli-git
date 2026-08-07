// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"testing"
	"time"
)

func TestParseStashDatesFindsTheOldestNotTheLast(t *testing.T) {
	// git lists newest first, but a stash pushed onto an older base can sit
	// anywhere in the list. Taking the last line would report the wrong age.
	output := "1700000300\n1700000100\n1700000200\n"

	count, oldest := ParseStashDates(output)

	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
	if want := time.Unix(1700000100, 0); !oldest.Equal(want) {
		t.Errorf("oldest = %v, want %v", oldest, want)
	}
}

func TestParseStashDatesOnAnEmptyList(t *testing.T) {
	for _, output := range []string{"", "\n", "   \n  "} {
		count, oldest := ParseStashDates(output)
		if count != 0 || !oldest.IsZero() {
			t.Errorf("ParseStashDates(%q) = (%d, %v), want (0, zero)", output, count, oldest)
		}
	}
}

func TestParseStashDatesCountsAnUnreadableDate(t *testing.T) {
	// Under-reporting how much is stranded is worse than losing one age.
	count, oldest := ParseStashDates("not-a-date\n1700000100\n")

	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if want := time.Unix(1700000100, 0); !oldest.Equal(want) {
		t.Errorf("oldest = %v, want %v", oldest, want)
	}
}

func TestParseStashDatesWithNoReadableDateAtAll(t *testing.T) {
	count, oldest := ParseStashDates("nonsense\n")
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
	if !oldest.IsZero() {
		t.Errorf("oldest = %v, want zero when no date could be read", oldest)
	}
}
