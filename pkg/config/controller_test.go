// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadControllerConfigAllowsNonControllerWorkspace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controller.yaml")
	raw := []byte(`workspaces:
  client:
    url: https://example.invalid/client.git
    branch: develop
  generated:
    branch: develop
  engine:
    url: https://example.invalid/engine.git
    branch:
      integrationBranch: develop
      taskPattern: dev/*/*/*
    integration:
      prepareProfile: familybook-ent-v1
`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadControllerConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Workspaces["client"].Integration != nil {
		t.Fatal("non-controller workspace unexpectedly gained integration policy")
	}
	if got.Workspaces["generated"].Integration != nil {
		t.Fatal("URL-less non-controller workspace unexpectedly gained integration policy")
	}
	if got.Workspaces["engine"].Branch.IntegrationBranch[0] != "develop" {
		t.Fatalf("engine integration branch = %#v", got.Workspaces["engine"].Branch.IntegrationBranch)
	}
}
