// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"context"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)

// TestValidateBootstrapContractEntryDistinguishesTreeErrorFromAbsence pins the
// three verdicts this validator has to keep apart. Bootstrap lands a commit on
// a protected branch outside the normal gate, so "I could not read the target
// tree" must never resolve as "the target has no contract" -- that reading
// turns a transient git failure into permission to overwrite a contract that
// is really there.
//
// Real git rather than a fake: gitRepo is concrete, and the distinction under
// test lives in exactly how ls-tree's failure and its empty output are told
// apart.
func TestValidateBootstrapContractEntryDistinguishesTreeErrorFromAbsence(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	writeRepoFile(t, fx.Clone, "other.txt", "x\n")
	runGit(t, fx.Clone, "add", "other.txt")
	runGit(t, fx.Clone, "commit", "-m", "no contract")
	noContract := runGitInTest(t, fx.Clone, "rev-parse", "HEAD")

	writeRepoFile(t, fx.Clone, ".gz-git.yaml", "branch:\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/check\n")
	runGit(t, fx.Clone, "add", ".gz-git.yaml")
	runGit(t, fx.Clone, "commit", "-m", "add contract")
	withContract := runGitInTest(t, fx.Clone, "rev-parse", "HEAD")

	const unreadable = "0000000000000000000000000000000000000000"
	g := newGitRepo(gitcmd.NewExecutor(), fx.Clone)
	ctx := context.Background()

	for _, tc := range []struct {
		name          string
		target        string
		contractIsNew bool
		wantErr       string
	}{
		{"first contract onto a target that has none", noContract, true, ""},
		{"addition refused when the target already has one", withContract, true, "target already has one"},
		{"existing contract required on the target", noContract, false, "mode must be unchanged"},
		{"unreadable target is an error, not an absence", unreadable, false, "inspect target .gz-git.yaml"},
		// The load-bearing case. With the tree error swallowed, oldOK reads
		// false, contractIsNew is true, and this returns nil -- bootstrap
		// proceeds to overwrite a contract it never managed to look at.
		{"unreadable target is not room for a first contract", unreadable, true, "inspect target .gz-git.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBootstrapContractEntry(ctx, g, tc.target, withContract, tc.contractIsNew)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("want accepted, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestCheck_BaselineUnmeasurableCheckReportGatesUnlessAllowed carries the
// unmeasurable verdict all the way to a CheckReport. baselineCheckItem's unit
// test pins the CheckItem in isolation, but nothing pinned that Check reaches
// it with the operator's flag: opts.AllowSkippedChecks is threaded into
// judgeMakeAgainstProbe by hand, and hard-coding false there broke no test at
// all -- the escape hatch would simply have stopped working.
func TestCheck_BaselineUnmeasurableCheckReportGatesUnlessAllowed(t *testing.T) {
	// The target tip's own check fails without emitting a single file:line
	// token, so there is no baseline to compare the branch against.
	const targetMakefile = "check:\n\t@echo 'boom, no diagnostics here'; exit 1\n"
	const branchMakefile = "check:\n\t@echo 'task.go:1: broken'; exit 1\n"

	newFixture := func(t *testing.T) *testutil.WorktreeOrigin {
		t.Helper()
		fx := testutil.TempWorktreeWithBareOrigin(t)
		writeRepoFile(t, fx.Clone, ".gz-git.yaml", "branch:\n  integrationBranch: develop\n")
		writeRepoFile(t, fx.Clone, "Makefile", targetMakefile)
		runGit(t, fx.Clone, "add", ".")
		runGit(t, fx.Clone, "commit", "-m", "target check fails with no diagnostics")
		runGit(t, fx.Clone, "branch", "develop")
		runGit(t, fx.Clone, "push", "-u", fx.Remote, "develop")

		runGit(t, fx.Worktree, "checkout", "-B", "dev/actor/feat/task", "develop")
		writeRepoFile(t, fx.Worktree, "Makefile", branchMakefile)
		runGit(t, fx.Worktree, "add", "Makefile")
		runGit(t, fx.Worktree, "commit", "-m", "task check failure")
		runGit(t, fx.Worktree, "push", "-u", fx.Remote, "HEAD")
		return fx
	}

	t.Run("gates by default", func(t *testing.T) {
		fx := newFixture(t)
		report, err := Check(context.Background(), gitcmd.NewExecutor(), CheckOptions{
			RepoPath: fx.Worktree, Branch: "dev/actor/feat/task",
		})
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		if report.Ready {
			t.Fatalf("an unmeasured gate must not be READY:\n%s", FormatCheck(report))
		}
		if !hasCheckDetail(report, "make check", checkFail, "baseline unmeasurable") {
			t.Fatalf("want a failing unmeasurable item:\n%s", FormatCheck(report))
		}
	})

	t.Run("downgraded by allow-skipped-checks", func(t *testing.T) {
		fx := newFixture(t)
		report, err := Check(context.Background(), gitcmd.NewExecutor(), CheckOptions{
			RepoPath: fx.Worktree, Branch: "dev/actor/feat/task", AllowSkippedChecks: true,
		})
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		if !hasCheckDetail(report, "make check", checkWarn, "baseline unmeasurable") {
			t.Fatalf("the flag must reach the baseline verdict:\n%s", FormatCheck(report))
		}
	})
}

// TestBaselineCheckItemUnknownVerdictFailsClosed pins the default arm. It is
// unreachable from the three declared statuses -- and BaselinePass is iota 0,
// so even a zero value lands on a real one -- which is why nothing caught it
// being turned into a warn. A fourth status added later would otherwise arrive
// as a silent pass, since Ready counts failures only.
func TestBaselineCheckItemUnknownVerdictFailsClosed(t *testing.T) {
	item := baselineCheckItem("make check", BaselineResult{
		Status: BaselineStatus(99),
		Reason: "invented",
	}, true)
	if item.Status != checkFail {
		t.Fatalf("an unrecognized verdict must fail closed even with allow-skipped-checks, got %+v", item)
	}
}
