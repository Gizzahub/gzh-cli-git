// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

func TestPushPolicyUnmarshalsFromProjectConfig(t *testing.T) {
	const doc = `
push:
  setUpstream: true
  policy:
    protected:
      - main
      - "release/*"
    forceMode: deny
`

	var project ProjectConfig
	if err := yaml.Unmarshal([]byte(doc), &project); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if project.Push == nil || project.Push.Policy == nil {
		t.Fatal("push policy was not parsed")
	}

	policy := project.Push.Policy
	if got, want := len(policy.Protected), 2; got != want {
		t.Fatalf("protected = %v, want %d entries", policy.Protected, want)
	}
	if policy.ForceMode != repository.ForceModeDeny {
		t.Errorf("forceMode = %q, want %q", policy.ForceMode, repository.ForceModeDeny)
	}

	// The parsed policy has to actually decide something, not just hold strings.
	if denial := policy.Check(repository.PushIntent{Branch: "release/9"}); denial == nil {
		t.Error("expected release/9 to be refused")
	}
}

func TestPushConfigWithoutPolicyLeavesItNil(t *testing.T) {
	var project ProjectConfig
	if err := yaml.Unmarshal([]byte("push:\n  setUpstream: true\n"), &project); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if project.Push == nil {
		t.Fatal("push config was not parsed")
	}
	if project.Push.Policy != nil {
		t.Errorf("policy = %+v, want nil when unconfigured", project.Push.Policy)
	}
}

func TestApplyPushConfigReplacesPolicy(t *testing.T) {
	loader := &ConfigLoader{}

	// A profile sets a wide policy; the project narrows it. The narrower one
	// must win outright rather than being merged into the wider list.
	effective := PushConfig{}
	loader.applyPushConfig(&effective, &PushConfig{
		Policy: &repository.PushPolicy{Protected: []string{"main", "develop"}},
	})
	loader.applyPushConfig(&effective, &PushConfig{
		Policy: &repository.PushPolicy{Protected: []string{"main"}},
	})

	if got := effective.Policy.Protected; len(got) != 1 || got[0] != "main" {
		t.Errorf("protected = %v, want [main]", got)
	}
}
