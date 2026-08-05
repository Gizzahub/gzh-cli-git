// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package porcelain

import "testing"

// records builds a `git status --porcelain -z` payload.
//
// git terminates every record with a NUL, including the last, so the fixture
// does too — the trailing empty split element it produces is part of what the
// parser has to tolerate.
func records(recs ...string) string {
	out := ""
	for _, rec := range recs {
		out += rec + "\x00"
	}

	return out
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		stdout  string
		want    []Record
		wantErr bool
	}{
		{
			name:   "empty output",
			stdout: "",
			want:   []Record{},
		},
		{
			name:   "trailing NUL does not become a phantom record",
			stdout: records("M  only.go"),
			want:   []Record{{Code: "M ", Path: "only.go"}},
		},
		{
			// The whole reason for -z: neither of these paths survives plain
			// --porcelain, which C-quotes them into strings naming no real file.
			name:   "paths with spaces and non-ASCII survive intact",
			stdout: records("M  my file.go", " M 한글.md"),
			want: []Record{
				{Code: "M ", Path: "my file.go"},
				{Code: " M", Path: "한글.md"},
			},
		},
		{
			// -z drops the " -> " separator and reverses the order: the
			// destination is in the status record, the source follows it.
			name:   "rename pairs its source from the next record",
			stdout: records("R  new.go", "old.go", "M  other.go"),
			want: []Record{
				{Code: "R ", Path: "new.go", OldPath: "old.go"},
				{Code: "M ", Path: "other.go"},
			},
		},
		{
			// `mv a b && git add -N b` puts the R on the worktree column. Keying
			// the lookahead on X alone leaves the source unconsumed, and it is
			// then re-read as a status line whose first two bytes ("ol") are
			// taken for an XY code.
			name:   "worktree-side rename pairs its source",
			stdout: records(" R new.go", "old.go"),
			want:   []Record{{Code: " R", Path: "new.go", OldPath: "old.go"}},
		},
		{
			name:    "record too short to hold a status code",
			stdout:  records("M"),
			wantErr: true,
		},
		{
			// Three bytes: an XY pair and its separator with the path cut off.
			// Indistinguishable from a truncated read, so it must not be skipped.
			name:    "record with status code but no path",
			stdout:  records("?? "),
			wantErr: true,
		},
		{
			// The split's trailing empty element makes this look like "a next
			// record exists", so a length-only lookahead reports a rename with
			// an empty origin.
			name:    "rename with no source record",
			stdout:  records("R  renamed.go"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.stdout)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				return
			}

			if len(got) != len(tt.want) {
				t.Fatalf("Parse() returned %d records, want %d: %+v", len(got), len(tt.want), got)
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("record %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIsUnmerged(t *testing.T) {
	// The seven combinations git defines as unmerged, plus near-misses that
	// must not trip the guard.
	unmerged := []string{"DD", "AU", "UD", "UA", "DU", "AA", "UU"}
	for _, code := range unmerged {
		if !IsUnmerged(code) {
			t.Errorf("IsUnmerged(%q) = false, want true", code)
		}
	}

	merged := []string{"M ", " M", "MM", "A ", " D", "??", "R ", "AM", "AD", "", "U"}
	for _, code := range merged {
		if IsUnmerged(code) {
			t.Errorf("IsUnmerged(%q) = true, want false", code)
		}
	}
}
