// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import "testing"

func TestParseGoModuleBotTarget(t *testing.T) {
	tests := []struct {
		name       string
		branch     string
		wantModule string
		wantVer    string
		wantOK     bool
	}{
		{
			name:       "top-level module",
			branch:     "dependabot/go_modules/github.com/aws/aws-sdk-go-v2-1.41.1",
			wantModule: "github.com/aws/aws-sdk-go-v2",
			wantVer:    "v1.41.1",
			wantOK:     true,
		},
		{
			name:       "nested module",
			branch:     "dependabot/go_modules/github.com/aws/aws-sdk-go-v2/config-1.32.7",
			wantModule: "github.com/aws/aws-sdk-go-v2/config",
			wantVer:    "v1.32.7",
			wantOK:     true,
		},
		{
			name:       "origin prefix",
			branch:     "origin/dependabot/go_modules/golang.org/x/sys-0.35.0",
			wantModule: "golang.org/x/sys",
			wantVer:    "v0.35.0",
			wantOK:     true,
		},
		{
			name:       "gopkg.in",
			branch:     "dependabot/go_modules/gopkg.in/yaml.v3-3.0.1",
			wantModule: "gopkg.in/yaml.v3",
			wantVer:    "v3.0.1",
			wantOK:     true,
		},
		{
			name:   "bare v2 is the module path not a version",
			branch: "dependabot/go_modules/github.com/aws/aws-sdk-go-v2",
		},
		{
			name:   "renovate is not comparable from the name",
			branch: "renovate/github.com-aws-aws-sdk-go-v2-1.x",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mod, ver, ok := parseGoModuleBotTarget(tt.branch)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (mod=%q ver=%q)", ok, tt.wantOK, mod, ver)
			}
			if mod != tt.wantModule || ver != tt.wantVer {
				t.Errorf("got (%q, %q), want (%q, %q)", mod, ver, tt.wantModule, tt.wantVer)
			}
		})
	}
}

func TestGoVersionGTE(t *testing.T) {
	tests := []struct {
		base, bot string
		want      bool
	}{
		{"v1.41.1", "v1.40.0", true},
		{"v1.40.0", "v1.41.1", false},
		{"v1.41.1", "1.41.1", true},
		{"v1.2.3", "v1.2.3-alpha", true},
		{"v1.2.3-alpha", "v1.2.3", false},
		{"v0.0.0-20240101120000-abcdefabcdef", "v0.0.0-20230101120000-abcdefabcdef", true},
		{"v0.0.0-20230101120000-abcdefabcdef", "v0.0.0-20240101120000-abcdefabcdef", false},
		{"not-a-version", "v1.0.0", false},
	}
	for _, tt := range tests {
		if got := goVersionGTE(tt.base, tt.bot); got != tt.want {
			t.Errorf("goVersionGTE(%q, %q) = %v, want %v", tt.base, tt.bot, got, tt.want)
		}
	}
}

func TestCompareActionVersion_MajorTagJumpIncomparable(t *testing.T) {
	_, ok := compareActionVersion("v4", "v7")
	if ok {
		t.Fatal("v4 vs v7 must be incomparable")
	}
	_, ok = compareActionVersion("v7", "v4")
	if ok {
		t.Fatal("v7 vs v4 must be incomparable")
	}
}

func TestCompareActionVersion_Semver(t *testing.T) {
	rel, ok := compareActionVersion("v4.2.2", "v4.1.0")
	if !ok || rel < 0 {
		t.Fatalf("v4.2.2 vs v4.1.0 = (%d, %v), want comparable and >=", rel, ok)
	}
	rel, ok = compareActionVersion("v4.1.0", "v4.2.2")
	if !ok || rel >= 0 {
		t.Fatalf("v4.1.0 vs v4.2.2 = (%d, %v), want comparable and <", rel, ok)
	}
	rel, ok = compareActionVersion("v4", "v4")
	if !ok || rel != 0 {
		t.Fatalf("v4 vs v4 = (%d, %v), want equal", rel, ok)
	}
}

func TestBotTargetSuperseded_GoModule(t *testing.T) {
	base := "module example.com/app\n\ngo 1.22\n\nrequire github.com/aws/aws-sdk-go-v2 v1.41.1\n"
	bot := "module example.com/app\n\ngo 1.22\n\nrequire github.com/aws/aws-sdk-go-v2 v1.40.0\n"
	newer := "module example.com/app\n\ngo 1.22\n\nrequire github.com/aws/aws-sdk-go-v2 v1.42.0\n"
	name := "dependabot/go_modules/github.com/aws/aws-sdk-go-v2-1.40.0"

	if !botTargetSuperseded(name, botVersionFiles{baseGoMod: base, botGoMod: bot}) {
		t.Fatal("base v1.41.1 vs bot v1.40.0 should be superseded")
	}
	if botTargetSuperseded("dependabot/go_modules/github.com/aws/aws-sdk-go-v2-1.42.0", botVersionFiles{
		baseGoMod: base, botGoMod: newer,
	}) {
		t.Fatal("still-newer bot target must stay pending")
	}
}

func TestBotTargetSuperseded_ActionsMajorTagPending(t *testing.T) {
	base := "jobs:\n  t:\n    steps:\n      - uses: actions/checkout@v4\n"
	name := "dependabot/github_actions/actions/checkout-7"
	if botTargetSuperseded(name, botVersionFiles{baseWorkflows: []string{base}}) {
		t.Fatal("v4 vs v7 major tag jump must stay pending")
	}
}

func TestBotTargetSuperseded_ActionsBaseAlreadyNewer(t *testing.T) {
	base := "jobs:\n  t:\n    steps:\n      - uses: actions/checkout@v4.2.2\n"
	name := "dependabot/github_actions/actions/checkout-4.1.0"
	if !botTargetSuperseded(name, botVersionFiles{baseWorkflows: []string{base}}) {
		t.Fatal("base v4.2.2 vs bot v4.1.0 should be superseded")
	}
}

func TestBotTargetSuperseded_UnknownEcosystemPending(t *testing.T) {
	if botTargetSuperseded("renovate/docker-alpine", botVersionFiles{}) {
		t.Fatal("renovate must stay pending without a comparator")
	}
}

func TestParseWorkflowUses(t *testing.T) {
	body := `
name: ci
jobs:
  t:
    steps:
      - uses: actions/checkout@v4
      - uses: "actions/setup-go@v5.0.1" # pin
      # uses: actions/cache@v3
      - uses: docker://alpine:3
      - uses: ./local
`
	pins := parseWorkflowUses(body)
	if len(pins) != 2 {
		t.Fatalf("pins = %+v, want 2 real uses", pins)
	}
	if pins[0].action != "actions/checkout" || pins[0].version != "v4" {
		t.Errorf("pin0 = %+v", pins[0])
	}
	if pins[1].action != "actions/setup-go" || pins[1].version != "v5.0.1" {
		t.Errorf("pin1 = %+v", pins[1])
	}
}
