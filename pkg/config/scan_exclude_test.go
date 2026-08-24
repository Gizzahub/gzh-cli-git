// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"reflect"
	"testing"
)

func TestGetScanExcludePatterns(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		want []string
	}{
		{"nil defaults", &Config{}, nil},
		{"nil scan", &Config{Defaults: &DefaultsConfig{}}, nil},
		{
			"declared",
			&Config{Defaults: &DefaultsConfig{Scan: &ScanDefaults{Exclude: []string{"mirror"}}}},
			[]string{"mirror"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.GetScanExcludePatterns(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetScanExcludePatterns() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGetExcludePatternsIsSeparateFromScan guards the distinction the two keys
// were given. defaults.filter.exclude scopes the forge API listing in
// `workspace sync`; reading it as a local-scan filter is the misunderstanding
// defaults.scan.exclude was added to end, so neither getter may fall back to
// the other.
func TestGetExcludePatternsIsSeparateFromScan(t *testing.T) {
	cfg := &Config{Defaults: &DefaultsConfig{
		Filter: &FilterDefaults{Exclude: []string{"forge-only"}},
		Scan:   &ScanDefaults{Exclude: []string{"scan-only"}},
	}}
	if got := cfg.GetExcludePatterns(); !reflect.DeepEqual(got, []string{"forge-only"}) {
		t.Errorf("GetExcludePatterns() = %v, want [forge-only]", got)
	}
	if got := cfg.GetScanExcludePatterns(); !reflect.DeepEqual(got, []string{"scan-only"}) {
		t.Errorf("GetScanExcludePatterns() = %v, want [scan-only]", got)
	}

	onlyFilter := &Config{Defaults: &DefaultsConfig{Filter: &FilterDefaults{Exclude: []string{"forge-only"}}}}
	if got := onlyFilter.GetScanExcludePatterns(); got != nil {
		t.Errorf("defaults.filter.exclude leaked into the local scan: %v", got)
	}
}

func TestMergeExcludePatterns(t *testing.T) {
	tests := []struct {
		name          string
		parent, child []string
		want          []string
	}{
		{"no parent", nil, []string{"a"}, []string{"a"}},
		{"no child", []string{"a"}, nil, []string{"a"}},
		// A child declaring its own exclusion is adding one, not revoking the
		// parent's: a parent excludes a vendored mirror precisely so that no
		// descendant can write to it by omitting the rule.
		{"accumulate", []string{"a"}, []string{"b"}, []string{"a", "b"}},
		{"dedupe keeps first-seen order", []string{"a", "b"}, []string{"b", "c"}, []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeExcludePatterns(tt.parent, tt.child); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeExcludePatterns(%v, %v) = %v, want %v", tt.parent, tt.child, got, tt.want)
			}
		})
	}
}
