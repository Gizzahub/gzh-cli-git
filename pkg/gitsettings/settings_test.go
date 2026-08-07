// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package gitsettings

import "testing"

func TestRecommendedIsACopy(t *testing.T) {
	first := Recommended()
	if len(first) == 0 {
		t.Fatal("Recommended() returned no settings")
	}

	first[0].Want = "mutated"

	if Recommended()[0].Want == "mutated" {
		t.Error("Recommended() exposed the package-level slice")
	}
}

func TestRecommendedSettingsAreWellFormed(t *testing.T) {
	seen := make(map[string]bool)
	for _, s := range Recommended() {
		if s.Key == "" || s.Want == "" || s.Why == "" {
			t.Errorf("incomplete setting: %+v", s)
		}
		if seen[s.Key] {
			t.Errorf("duplicate key: %s", s.Key)
		}
		seen[s.Key] = true
	}
}

func TestValuesEqual(t *testing.T) {
	tests := []struct {
		name    string
		current string
		want    string
		equal   bool
	}{
		{"identical", "current", "current", true},
		{"case insensitive", "Current", "current", true},
		{"surrounding space", " true ", "true", true},
		{"boolean alias yes", "yes", "true", true},
		{"boolean alias on", "on", "true", true},
		{"boolean alias 1", "1", "true", true},
		{"boolean alias off", "off", "false", true},
		{"boolean opposite", "false", "true", false},
		{"non boolean mismatch", "diff3", "zdiff3", false},
		{"empty current", "", "true", false},
		{"non boolean want ignores aliases", "1", "current", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := valuesEqual(tt.current, tt.want); got != tt.equal {
				t.Errorf("valuesEqual(%q, %q) = %v, want %v", tt.current, tt.want, got, tt.equal)
			}
		})
	}
}

func TestVersionAtLeast(t *testing.T) {
	tests := []struct {
		version string
		minimum string
		ok      bool
	}{
		{"2.37.0", "2.37", true},
		{"2.36.9", "2.37", false},
		{"2.40.1", "2.37", true},
		{"3.0.0", "2.37", true},
		{"2.39.5.windows.1", "2.35", true},
		{"2.34", "2.35", false},
		{"2.35", "2.35", true},
		// Unparseable versions must not block configuration.
		{"unknown", "2.37", true},
		{"", "2.37", true},
	}

	for _, tt := range tests {
		t.Run(tt.version+"_vs_"+tt.minimum, func(t *testing.T) {
			if got := versionAtLeast(tt.version, tt.minimum); got != tt.ok {
				t.Errorf("versionAtLeast(%q, %q) = %v, want %v", tt.version, tt.minimum, got, tt.ok)
			}
		})
	}
}

func TestScopeFlag(t *testing.T) {
	if got := ScopeGlobal.Flag(); got != "--global" {
		t.Errorf("ScopeGlobal.Flag() = %q", got)
	}
	if got := ScopeLocal.Flag(); got != "--local" {
		t.Errorf("ScopeLocal.Flag() = %q", got)
	}
}

func TestStatusNeedsChange(t *testing.T) {
	tests := map[State]bool{
		StateOK:          false,
		StateUnset:       true,
		StateMismatch:    true,
		StateUnsupported: false,
	}

	for state, want := range tests {
		if got := (Status{State: state}).NeedsChange(); got != want {
			t.Errorf("State %s: NeedsChange() = %v, want %v", state, got, want)
		}
	}
}

func TestReportPendingAndUnsupported(t *testing.T) {
	report := &Report{Statuses: []Status{
		{Setting: Setting{Key: "a"}, State: StateOK},
		{Setting: Setting{Key: "b"}, State: StateUnset},
		{Setting: Setting{Key: "c"}, State: StateMismatch},
		{Setting: Setting{Key: "d"}, State: StateUnsupported},
	}}

	pending := report.Pending()
	if len(pending) != 2 || pending[0].Key != "b" || pending[1].Key != "c" {
		t.Errorf("Pending() = %+v", pending)
	}

	unsupported := report.Unsupported()
	if len(unsupported) != 1 || unsupported[0].Key != "d" {
		t.Errorf("Unsupported() = %+v", unsupported)
	}
}
