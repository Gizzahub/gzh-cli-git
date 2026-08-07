// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/identity"
)

func TestIdentityUnmarshalsFromGlobalConfig(t *testing.T) {
	const doc = `
identity:
  device: dave-office
  agent: hermes-01
`

	var global GlobalConfig
	if err := yaml.Unmarshal([]byte(doc), &global); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if global.Identity == nil {
		t.Fatal("identity was not parsed")
	}
	if global.Identity.Device != "dave-office" || global.Identity.Agent != "hermes-01" {
		t.Errorf("identity = %+v, want dave-office/hermes-01", *global.Identity)
	}
}

func TestApplyIdentityMergesFieldwise(t *testing.T) {
	loader := &ConfigLoader{}
	cfg := &EffectiveConfig{Sources: make(map[string]string)}

	// The global config names the machine; a profile names only the agent
	// running on it. The device has to survive.
	loader.applyIdentity(cfg, &identity.Identity{Device: "dave-office"}, string(SourceGlobal))
	loader.applyIdentity(cfg, &identity.Identity{Agent: "hermes-01"}, string(SourceProfile))

	if cfg.Identity.Device != "dave-office" {
		t.Errorf("device = %q, want dave-office", cfg.Identity.Device)
	}
	if cfg.Identity.Agent != "hermes-01" {
		t.Errorf("agent = %q, want hermes-01", cfg.Identity.Agent)
	}
	if cfg.Sources["identity.device"] != string(SourceGlobal) {
		t.Errorf("device source = %q, want global", cfg.Sources["identity.device"])
	}
}

func TestResolveIdentityLetsEnvironmentWin(t *testing.T) {
	t.Setenv(identity.EnvDevice, "dave-laptop")
	t.Setenv(identity.EnvAgent, "")

	loader := &ConfigLoader{}
	cfg := &EffectiveConfig{
		Sources:  make(map[string]string),
		Identity: identity.Identity{Device: "dave-office"},
	}

	loader.resolveIdentity(cfg)

	if cfg.Identity.Device != "dave-laptop" {
		t.Errorf("device = %q, want dave-laptop", cfg.Identity.Device)
	}
	if cfg.Sources["identity.device"] != string(SourceEnv) {
		t.Errorf("device source = %q, want env", cfg.Sources["identity.device"])
	}
}

func TestResolveIdentityRecordsHostnameAsADefault(t *testing.T) {
	t.Setenv(identity.EnvDevice, "")
	t.Setenv(identity.EnvAgent, "")

	loader := &ConfigLoader{}
	cfg := &EffectiveConfig{Sources: make(map[string]string)}

	loader.resolveIdentity(cfg)

	if cfg.Identity.Device == "" {
		t.Skip("hostname unavailable")
	}
	if cfg.Sources["identity.device"] != string(SourceDefault) {
		t.Errorf("device source = %q, want default", cfg.Sources["identity.device"])
	}
}
