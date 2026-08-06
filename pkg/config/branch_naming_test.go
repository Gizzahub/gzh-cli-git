// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/branch"
)

func TestBranchNamingUnmarshalsFromProjectConfig(t *testing.T) {
	const doc = `
branch:
  defaultBranch: main
  naming:
    device: wip/{device}/{task}
`

	var project ProjectConfig
	if err := yaml.Unmarshal([]byte(doc), &project); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if project.Branch == nil || project.Branch.Naming == nil {
		t.Fatal("branch naming was not parsed")
	}
	if got := project.Branch.Naming.Device; got != "wip/{device}/{task}" {
		t.Errorf("device template = %q", got)
	}
	// The string shorthand for `branch:` must not have swallowed the map form.
	if got := project.Branch.DefaultBranch; len(got) != 1 || got[0] != "main" {
		t.Errorf("defaultBranch = %v, want [main]", got)
	}
}

func TestBranchNamingMergesOneKindAtATime(t *testing.T) {
	loader := &ConfigLoader{}

	// A profile spells two of the three; the project overrides only one. The
	// untouched template has to survive rather than reset to its default.
	effective := BranchConfig{}
	loader.applyBranchConfig(&effective, &BranchConfig{
		Naming: &branch.Naming{Work: "work/{task}", Device: "dev/{task}/{device}"},
	})
	loader.applyBranchConfig(&effective, &BranchConfig{
		Naming: &branch.Naming{Device: "wip/{device}/{task}"},
	})

	if got := effective.Naming.Device; got != "wip/{device}/{task}" {
		t.Errorf("device = %q, want the project's", got)
	}
	if got := effective.Naming.Work; got != "work/{task}" {
		t.Errorf("work = %q, want the profile's to survive", got)
	}
	if got := effective.Naming.Agent; got != "" {
		t.Errorf("agent = %q, want it left to the default", got)
	}
}

func TestBranchConfigWithoutNamingLeavesItNil(t *testing.T) {
	var project ProjectConfig
	if err := yaml.Unmarshal([]byte("branch:\n  defaultBranch: main\n"), &project); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if project.Branch == nil {
		t.Fatal("branch config was not parsed")
	}
	if project.Branch.Naming != nil {
		t.Errorf("naming = %+v, want nil when unconfigured", project.Branch.Naming)
	}
}
