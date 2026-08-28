// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParseReadinessDocument_StrictV1(t *testing.T) {
	for _, tt := range []struct {
		name string
		doc  string
		ok   bool
	}{
		{"valid", "branch:\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/check\n", true},
		{"unknown", "branch:\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/check\n    args: [oops]\n", false},
		{"wrong version", "branch:\n  readiness:\n    version: 2\n    runner: .gz-git/readiness/check\n", false},
		{"traversal", "branch:\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/../check\n", false},
		{"outside", "branch:\n  readiness:\n    version: 1\n    runner: script/check\n", false},
		{"duplicate yaml", "branch:\n  readiness:\n    version: 1\n    version: 1\n    runner: .gz-git/readiness/check\n", false},
		{"backslash", "branch:\n  readiness:\n    version: 1\n    runner: .gz-git/readiness\\check\n", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, present, err := ParseReadinessDocument([]byte(tt.doc), false)
			if tt.ok && (err != nil || !present) {
				t.Fatalf("ParseReadinessDocument() = %v, %v", got, err)
			}
			if !tt.ok && err == nil {
				t.Fatal("ParseReadinessDocument() unexpectedly succeeded")
			}
		})
	}
}

func TestParseReadinessDocument_RejectsDuplicateJSON(t *testing.T) {
	for _, doc := range []string{
		`{"branch":"develop","branch":{"readiness":{"version":1,"runner":".gz-git/readiness/check"}}}`,
		`{"branch":{"readiness":{"version":1,"runner":".gz-git/readiness/check"},"readiness":{"version":1,"runner":".gz-git/readiness/check"}}}`,
		`{"branch":{"readiness":{"version":1,"version":1,"runner":".gz-git/readiness/check"}}}`,
	} {
		if _, _, err := ParseReadinessDocument([]byte(doc), true); err == nil {
			t.Fatalf("duplicate JSON readiness path accepted: %s", doc)
		}
	}
}

func TestParseReadinessDocument_RejectsDuplicateYAMLPath(t *testing.T) {
	for _, doc := range []string{
		"branch: develop\nbranch:\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/check\n",
		"branch:\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/check\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/check\n",
	} {
		if _, _, err := ParseReadinessDocument([]byte(doc), false); err == nil {
			t.Fatalf("duplicate YAML readiness path accepted: %s", doc)
		}
	}
}

func TestParseReadinessDocument_LegacyBranchShorthandIsAbsent(t *testing.T) {
	for _, tt := range []struct {
		name   string
		doc    string
		isJSON bool
	}{
		{name: "yaml", doc: "branch: develop\n"},
		{name: "json", doc: `{"branch":"develop"}`, isJSON: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, present, err := ParseReadinessDocument([]byte(tt.doc), tt.isJSON)
			if err != nil || present {
				t.Fatalf("legacy shorthand changed behavior: present=%v err=%v", present, err)
			}
		})
	}
}

func TestProjectConfigReadinessRoundTripAndSchema(t *testing.T) {
	input := "branch:\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/check\n"
	var project ProjectConfig
	if err := yaml.Unmarshal([]byte(input), &project); err != nil {
		t.Fatal(err)
	}
	if project.Branch == nil || project.Branch.Readiness == nil {
		t.Fatalf("readiness was not preserved: %+v", project.Branch)
	}
	if err := NewValidator().ValidateProjectConfig(&project); err != nil {
		t.Fatalf("readiness did not validate: %v", err)
	}
	out, err := yaml.Marshal(&project)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "readiness:") || !strings.Contains(ExampleConfig, "runner: .gz-git/readiness/check") {
		t.Fatalf("readiness missing from round trip or schema:\n%s", out)
	}
}

func TestReadinessRejectedOutsideProjectRoot(t *testing.T) {
	readiness := &Readiness{Version: 1, Runner: ".gz-git/readiness/check"}
	v := NewValidator()
	if err := v.ValidateConfig(&Config{Branch: &BranchConfig{Readiness: readiness}}); err == nil {
		t.Fatal("recursive config readiness accepted")
	}
	if err := v.ValidateWorkspace(&Workspace{Branch: &BranchConfig{Readiness: readiness}}, "repo"); err == nil {
		t.Fatal("workspace readiness accepted")
	}
	if err := v.ValidateProfile(&Profile{Name: "work", Branch: &BranchConfig{Readiness: readiness}}); err == nil {
		t.Fatal("profile readiness accepted")
	}
}
