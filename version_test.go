// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package gzhcligitforge

import (
	"runtime/debug"
	"testing"
)

// The fallback exists so a binary never claims a version nobody gave it, which
// makes precedence the property worth testing: an injected value must survive,
// and an absent one must be filled from whatever the toolchain stamped.
func TestVersionFromBuildInfo(t *testing.T) {
	t.Parallel()

	settings := func(pairs ...string) []debug.BuildSetting {
		out := make([]debug.BuildSetting, 0, len(pairs)/2)
		for i := 0; i < len(pairs); i += 2 {
			out = append(out, debug.BuildSetting{Key: pairs[i], Value: pairs[i+1]})
		}

		return out
	}

	tests := []struct {
		name                              string
		info                              *debug.BuildInfo
		inVersion, inCommit, inDate       string
		wantVersion, wantCommit, wantDate string
	}{
		{
			name: "ldflags win over the embedded stamp",
			info: &debug.BuildInfo{
				Main:     debug.Module{Version: "v9.9.9"},
				Settings: settings("vcs.revision", "ffffffffffffffff", "vcs.time", "2000-01-01T00:00:00Z"),
			},
			inVersion: "1.2.3", inCommit: "abc1234", inDate: "2026-01-01T00:00:00Z",
			wantVersion: "1.2.3", wantCommit: "abc1234", wantDate: "2026-01-01T00:00:00Z",
		},
		{
			name:      "go install of a tagged version supplies the version",
			info:      &debug.BuildInfo{Main: debug.Module{Version: "v0.8.1"}},
			inVersion: defaultVersion, inCommit: defaultUnknown, inDate: defaultUnknown,
			wantVersion: "0.8.1", wantCommit: defaultUnknown, wantDate: defaultUnknown,
		},
		{
			name:      "(devel) carries no more information than the default",
			info:      &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}},
			inVersion: defaultVersion, inCommit: defaultUnknown, inDate: defaultUnknown,
			wantVersion: defaultVersion, wantCommit: defaultUnknown, wantDate: defaultUnknown,
		},
		{
			name: "a clean vcs stamp supplies a short commit and the commit time",
			info: &debug.BuildInfo{
				Main:     debug.Module{Version: "(devel)"},
				Settings: settings("vcs.revision", "99458df1234567890abcdef", "vcs.time", "2026-08-25T02:01:30Z", "vcs.modified", "false"),
			},
			inVersion: defaultVersion, inCommit: defaultUnknown, inDate: defaultUnknown,
			wantVersion: defaultVersion, wantCommit: "99458df", wantDate: "2026-08-25T02:01:30Z",
		},
		{
			name: "a dirty tree says so rather than naming a commit it did not build",
			info: &debug.BuildInfo{
				Main:     debug.Module{Version: "(devel)"},
				Settings: settings("vcs.revision", "99458df1234567890abcdef", "vcs.modified", "true"),
			},
			inVersion: defaultVersion, inCommit: defaultUnknown, inDate: defaultUnknown,
			wantVersion: defaultVersion, wantCommit: "99458df-dirty", wantDate: defaultUnknown,
		},
		{
			name:      "nothing stamped leaves every default alone",
			info:      &debug.BuildInfo{},
			inVersion: defaultVersion, inCommit: defaultUnknown, inDate: defaultUnknown,
			wantVersion: defaultVersion, wantCommit: defaultUnknown, wantDate: defaultUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			version, commit, date := versionFromBuildInfo(tt.info, tt.inVersion, tt.inCommit, tt.inDate)
			if version != tt.wantVersion {
				t.Errorf("version = %q, want %q", version, tt.wantVersion)
			}
			if commit != tt.wantCommit {
				t.Errorf("commit = %q, want %q", commit, tt.wantCommit)
			}
			if date != tt.wantDate {
				t.Errorf("date = %q, want %q", date, tt.wantDate)
			}
		})
	}
}

// A revision shorter than the trim length must come back whole rather than be
// indexed out of range.
func TestShortRevision(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"":                        "",
		"abc":                     "abc",
		"abc1234":                 "abc1234",
		"99458df1234567890abcdef": "99458df",
	}

	for in, want := range tests {
		if got := shortRevision(in); got != want {
			t.Errorf("shortRevision(%q) = %q, want %q", in, got, want)
		}
	}
}
