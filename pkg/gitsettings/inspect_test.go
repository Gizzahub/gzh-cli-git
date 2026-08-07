// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package gitsettings

import (
	"context"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)

// statusFor finds a status by key, failing the test when it is missing.
func statusFor(t *testing.T, report *Report, key string) Status {
	t.Helper()
	for _, s := range report.Statuses {
		if s.Key == key {
			return s
		}
	}
	t.Fatalf("no status for key %q", key)
	return Status{}
}

func TestInspectFreshRepoReportsUnset(t *testing.T) {
	repo := testutil.TempGitRepo(t)
	executor := gitcmd.NewExecutor()
	ctx := context.Background()

	report, err := Inspect(ctx, executor, ScopeLocal, repo)
	if err != nil {
		t.Fatalf("Inspect() error: %v", err)
	}

	if len(report.Statuses) != len(Recommended()) {
		t.Fatalf("got %d statuses, want %d", len(report.Statuses), len(Recommended()))
	}
	if report.GitVersion == "" {
		t.Error("GitVersion is empty")
	}

	for _, s := range report.Statuses {
		if s.State != StateUnset && s.State != StateUnsupported {
			t.Errorf("%s: state = %s (current %q), want unset in a fresh repo", s.Key, s.State, s.Current)
		}
	}
}

func TestInspectDetectsMismatchAndMatch(t *testing.T) {
	repo := testutil.TempGitRepo(t)
	executor := gitcmd.NewExecutor()
	ctx := context.Background()

	// One key deliberately wrong, one deliberately right.
	mustRunGit(t, executor, repo, "config", "--local", "pull.rebase", "false")
	mustRunGit(t, executor, repo, "config", "--local", "fetch.prune", "true")

	report, err := Inspect(ctx, executor, ScopeLocal, repo)
	if err != nil {
		t.Fatalf("Inspect() error: %v", err)
	}

	if got := statusFor(t, report, "pull.rebase"); got.State != StateMismatch || got.Current != "false" {
		t.Errorf("pull.rebase: state = %s, current = %q; want mismatch/false", got.State, got.Current)
	}
	if got := statusFor(t, report, "fetch.prune"); got.State != StateOK {
		t.Errorf("fetch.prune: state = %s, want ok", got.State)
	}
}

func TestInspectAcceptsBooleanAliases(t *testing.T) {
	repo := testutil.TempGitRepo(t)
	executor := gitcmd.NewExecutor()

	mustRunGit(t, executor, repo, "config", "--local", "rerere.enabled", "yes")

	report, err := Inspect(context.Background(), executor, ScopeLocal, repo)
	if err != nil {
		t.Fatalf("Inspect() error: %v", err)
	}

	if got := statusFor(t, report, "rerere.enabled"); got.State != StateOK {
		t.Errorf("rerere.enabled=yes: state = %s, want ok", got.State)
	}
}

func TestApplyMakesRepoConformAndIsIdempotent(t *testing.T) {
	repo := testutil.TempGitRepo(t)
	executor := gitcmd.NewExecutor()
	ctx := context.Background()

	mustRunGit(t, executor, repo, "config", "--local", "pull.rebase", "false")

	report, err := Inspect(ctx, executor, ScopeLocal, repo)
	if err != nil {
		t.Fatalf("Inspect() error: %v", err)
	}

	pending := report.Pending()
	if len(pending) == 0 {
		t.Fatal("expected pending changes in a fresh repo")
	}

	applied, err := Apply(ctx, executor, ScopeLocal, repo, report.Statuses)
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	if len(applied) != len(pending) {
		t.Errorf("applied %d settings, want %d", len(applied), len(pending))
	}

	after, err := Inspect(ctx, executor, ScopeLocal, repo)
	if err != nil {
		t.Fatalf("Inspect() after Apply error: %v", err)
	}
	for _, s := range after.Statuses {
		if s.State != StateOK && s.State != StateUnsupported {
			t.Errorf("%s: state = %s after Apply, want ok", s.Key, s.State)
		}
	}

	// A second Apply must be a no-op.
	reapplied, err := Apply(ctx, executor, ScopeLocal, repo, after.Statuses)
	if err != nil {
		t.Fatalf("second Apply() error: %v", err)
	}
	if len(reapplied) != 0 {
		t.Errorf("second Apply() wrote %d settings, want 0", len(reapplied))
	}
}

func TestApplySkipsUnsupportedSettings(t *testing.T) {
	repo := testutil.TempGitRepo(t)
	executor := gitcmd.NewExecutor()

	statuses := []Status{
		{Setting: Setting{Key: "merge.conflictStyle", Want: "zdiff3"}, State: StateUnsupported},
	}

	applied, err := Apply(context.Background(), executor, ScopeLocal, repo, statuses)
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	if len(applied) != 0 {
		t.Errorf("applied %d unsupported settings, want 0", len(applied))
	}
}

func mustRunGit(t *testing.T, executor *gitcmd.Executor, dir string, args ...string) {
	t.Helper()
	result, err := executor.Run(context.Background(), dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("git %v: exit %d: %s", args, result.ExitCode, result.Stderr)
	}
}
