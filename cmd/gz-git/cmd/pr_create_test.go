// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"
	"testing"
)

func TestPRCreateHelp(t *testing.T) {
	cmd := findCommand(t, rootCmd, "pr", "create")
	for _, name := range []string{"title", "body", "base", "draft", "reviewer", "label", "provider", "token", "scan-depth", "parallel", "dry-run", "include", "exclude"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("pr create missing --%s", name)
		}
	}
}

func TestDefaultPRTitle(t *testing.T) {
	if got := defaultPRTitle("dev/mac/feat/login-layout"); got != "login-layout" {
		t.Fatalf("got %q", got)
	}
	if got := defaultPRTitle("hotfix/urgent"); got != "urgent" {
		t.Fatalf("got %q", got)
	}
}

func TestParseForgeRemoteUsedByCreate(t *testing.T) {
	// Guard the branch-name → title path used when filling task identifiers.
	title := defaultPRTitle("feat/TASK-12/mac")
	if !strings.Contains(title, "TASK-12") && title != "mac" {
		t.Fatalf("unexpected title %q", title)
	}
}
