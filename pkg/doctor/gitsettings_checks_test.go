// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package doctor

import (
	"context"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/gitsettings"
)

func TestVerboseSettingChecksMapsEveryState(t *testing.T) {
	report := &gitsettings.Report{
		Scope:      gitsettings.ScopeGlobal,
		GitVersion: "2.30.0",
		Statuses: []gitsettings.Status{
			{Setting: gitsettings.Setting{Key: "fetch.prune", Want: "true"}, Current: "true", State: gitsettings.StateOK},
			{Setting: gitsettings.Setting{Key: "pull.rebase", Want: "true", Why: "why"}, Current: "false", State: gitsettings.StateMismatch},
			{Setting: gitsettings.Setting{Key: "rerere.enabled", Want: "true", Why: "why"}, State: gitsettings.StateUnset},
			{Setting: gitsettings.Setting{Key: "merge.conflictStyle", Want: "zdiff3", MinGit: "2.35"}, State: gitsettings.StateUnsupported},
		},
	}

	checks := verboseSettingChecks(report)
	if len(checks) != len(report.Statuses) {
		t.Fatalf("got %d checks, want %d", len(checks), len(report.Statuses))
	}

	want := []Status{StatusOK, StatusWarning, StatusWarning, StatusSkipped}
	for i, c := range checks {
		if c.Status != want[i] {
			t.Errorf("%s: status = %s, want %s", c.Name, c.Status, want[i])
		}
		if c.Category != CategorySystem {
			t.Errorf("%s: category = %s, want %s", c.Name, c.Category, CategorySystem)
		}
		if c.Message == "" {
			t.Errorf("%s: empty message", c.Name)
		}
	}

	// The unsupported entry must name the version requirement, not read as a pass.
	if !strings.Contains(checks[3].Message, "2.35") {
		t.Errorf("unsupported check message = %q, want it to name the minimum version", checks[3].Message)
	}
}

func TestCheckGitWorkflowSettingsAggregates(t *testing.T) {
	checks := checkGitWorkflowSettings(context.Background(), false)

	if len(checks) != 1 {
		t.Fatalf("non-verbose mode produced %d checks, want 1 aggregate", len(checks))
	}

	c := checks[0]
	if c.Category != CategorySystem {
		t.Errorf("category = %s, want %s", c.Category, CategorySystem)
	}
	if c.Status != StatusOK && c.Status != StatusWarning {
		t.Errorf("status = %s, want ok or warning", c.Status)
	}
	// A drifted machine must be told how to fix it.
	if c.Status == StatusWarning && c.Detail != "" && !strings.Contains(c.Detail, "gz-git config recommended") {
		t.Errorf("warning detail = %q, want a remediation hint", c.Detail)
	}
}
