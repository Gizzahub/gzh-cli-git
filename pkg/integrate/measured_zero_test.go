// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"context"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)

// TestMeasuredZeroIsNotSkippable pins the distinction TASK-141 is about.
// len(base) == 0 collapses two states that need opposite handling:
//
//	A. The target-tip run died before it could analyze anything — mix without
//	   deps/, a venv-less pytest. Nothing was measured, the gate was skipped,
//	   and --allow-skipped-checks may downgrade it. This is the state
//	   BaselineUnmeasurable was added for and it must keep working.
//	B. The target-tip run went all the way through and failed in a shape that
//	   carries no file:line — gofmt -l naming the files it would rewrite. That
//	   is a measurement of zero, so a branch emitting N has genuinely worsened
//	   it, and the flag must NOT excuse it: its contract is "this check was
//	   skipped", and this check was not.
//
// Before this, B was reported as BaselineUnmeasurable and one flag waved
// through a verdict that used to be an undowngradable BaselineFail
// ("diagnostic count increased (0 → N)").
//
// Both sides are asserted here on purpose, and the A side is the one that
// keeps this test honest. B is reached only through a positive signature, so
// a change that widened that signature — or that flipped the default to
// "measured" — would take the escape hatch away from every repository whose
// baseline genuinely cannot run. Pinning only B would let exactly that
// through, and re-collapsing the two states is the bug.
func TestMeasuredZeroIsNotSkippable(t *testing.T) {
	// The branch's check emits one file:line either way, so the branch side
	// is held constant and the target tip's failure shape is the only
	// variable between the two fixtures.
	const branchMakefile = "check:\n\t@echo 'task.go:1: broken'; exit 1\n"

	fixture := func(t *testing.T, targetMakefile string) *testutil.WorktreeOrigin {
		t.Helper()
		fx := testutil.TempWorktreeWithBareOrigin(t)
		writeRepoFile(t, fx.Clone, ".gz-git.yaml", "branch:\n  integrationBranch: develop\n")
		// A tracked file for the target tip's check to name. It is committed
		// on the target and never touched by the branch, so it cannot reach
		// rule (a) — only the count rules are in play.
		writeRepoFile(t, fx.Clone, "checked.go", "package main\n")
		writeRepoFile(t, fx.Clone, "Makefile", targetMakefile)
		runGit(t, fx.Clone, "add", ".")
		runGit(t, fx.Clone, "commit", "-m", "target check fails")
		runGit(t, fx.Clone, "branch", "develop")
		runGit(t, fx.Clone, "push", "-u", fx.Remote, "develop")

		runGit(t, fx.Worktree, "checkout", "-B", "dev/actor/feat/task", "develop")
		writeRepoFile(t, fx.Worktree, "Makefile", branchMakefile)
		runGit(t, fx.Worktree, "add", "Makefile")
		runGit(t, fx.Worktree, "commit", "-m", "task check failure")
		runGit(t, fx.Worktree, "push", "-u", fx.Remote, "HEAD")
		return fx
	}

	checkWithFlag := func(t *testing.T, fx *testutil.WorktreeOrigin) *CheckReport {
		t.Helper()
		report, err := Check(context.Background(), gitcmd.NewExecutor(), CheckOptions{
			RepoPath: fx.Worktree, Branch: "dev/actor/feat/task", AllowSkippedChecks: true,
		})
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		return report
	}

	t.Run("state B: a measured zero is not downgradable", func(t *testing.T) {
		// gofmt -l's shape: the check names a repository file it read, and
		// exits non-zero without a single line number. It reached analysis,
		// so its zero is a real zero.
		const targetMakefile = "check:\n\t@echo 'checked.go'; exit 1\n"
		report := checkWithFlag(t, fixture(t, targetMakefile))
		if report.Ready {
			t.Fatalf("a real 0 → N worsening must gate even with --allow-skipped-checks:\n%s", FormatCheck(report))
		}
		if !hasCheckDetail(report, "make check", checkFail, "diagnostic count increased (0 → 1)") {
			t.Fatalf("want the undowngradable count verdict:\n%s", FormatCheck(report))
		}
		if hasCheckDetail(report, "make check", checkWarn, "baseline unmeasurable") {
			t.Fatalf("the flag must not excuse a baseline that was measured:\n%s", FormatCheck(report))
		}
	})

	t.Run("state A: an unmeasured baseline still downgrades", func(t *testing.T) {
		// Same empty location list, different cause: the run never reached
		// analysis, so it names no repository file and nothing evidences a
		// measurement.
		const targetMakefile = "check:\n\t@echo 'Unchecked dependencies for environment dev:'; exit 1\n"
		report := checkWithFlag(t, fixture(t, targetMakefile))
		if !hasCheckDetail(report, "make check", checkWarn, "baseline unmeasurable") {
			t.Fatalf("the escape hatch must survive for a genuinely unmeasured baseline:\n%s", FormatCheck(report))
		}
	})

	t.Run("the verdict turns on the probe evidence, not the location count", func(t *testing.T) {
		// Identical location lists — the count cannot tell these apart, which
		// is exactly why the discriminator had to move onto the probe.
		in := BaselineInput{BranchLocations: []string{"lib/a.ex:1"}, BaseLocations: nil}

		in.BaseMeasurement = BaseMeasured
		measured := EvaluateBaseline(in)
		if measured.Status != BaselineFail {
			t.Fatalf("a measured zero against a branch at 1 is a worsening, got %+v", measured)
		}
		if item := baselineCheckItem("make check", measured, true); item.Status != checkFail {
			t.Fatalf("--allow-skipped-checks must not reach a BaselineFail, got %+v", item)
		}

		in.BaseMeasurement = BaseMeasurementUnknown
		unmeasured := EvaluateBaseline(in)
		if unmeasured.Status != BaselineUnmeasurable {
			t.Fatalf("a baseline with no evidence of measurement must stay unmeasurable, got %+v", unmeasured)
		}
		if item := baselineCheckItem("make check", unmeasured, true); item.Status != checkWarn {
			t.Fatalf("--allow-skipped-checks must still downgrade an unmeasured baseline, got %+v", item)
		}
	})

	t.Run("baseMeasurement demands positive evidence", func(t *testing.T) {
		// BaseMeasurementUnknown is the zero value and reads as unmeasured,
		// so every row that is not a recognized "the tool read the repository"
		// shape leaves the operator the escape hatch. Only the last three
		// rows earn BaseMeasured.
		tracked := []string{"checked.go", "Makefile"}
		for _, tc := range []struct {
			name  string
			probe makeProbe
			want  BaseMeasurement
		}{
			{"nothing filled in at all", makeProbe{}, BaseMeasurementUnknown},
			{"bare exit 1 with prose", makeProbe{Output: "boom, no diagnostics here"}, BaseMeasurementUnknown},
			{"mix without deps", makeProbe{Output: "Unchecked dependencies for environment dev:"}, BaseMeasurementUnknown},
			{"node without node_modules", makeProbe{Output: "Error: Cannot find module 'eslint'"}, BaseMeasurementUnknown},
			{"python without a venv", makeProbe{Output: "ModuleNotFoundError: No module named 'pytest'"}, BaseMeasurementUnknown},
			{"go runtime crash", makeProbe{Output: "panic: boom", ToolCrash: "panic: boom"}, BaseMeasurementUnknown},
			{"an untracked path proves nothing", makeProbe{Output: "vendor/other/x.go"}, BaseMeasurementUnknown},
			// The echo cases: the recipe named a tracked file on its way to
			// dying because the checker was never installed. Enumeration
			// happened, measurement did not. Reading these as BaseMeasured
			// produces an undowngradable "count increased (0 -> N)" and is
			// the TASK-134 failure by the back door.
			{"an echoed path with the checker absent", makeProbe{Output: "checked.go\n/bin/sh: shellcheck: command not found\nmake: *** [check] Error 127\n"}, BaseMeasurementUnknown},
			{"an echoed path with only make's 127", makeProbe{Output: "checked.go\nmake: *** [check] Error 127\n"}, BaseMeasurementUnknown},
			{"gofmt -l names a tracked file", makeProbe{Output: "checked.go\n"}, BaseMeasured},
			{"gci -d diff header names one", makeProbe{Output: "--- a/checked.go\n+++ b/checked.go\n"}, BaseMeasured},
			{"named among make's own noise", makeProbe{Output: "make[1]: Entering directory '/x'\nchecked.go\n"}, BaseMeasured},
		} {
			if got := baseMeasurement(tc.probe, tracked); got != tc.want {
				t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
			}
		}
		// With nothing tracked there is no vocabulary to recognize, so no
		// claim can be made.
		if got := baseMeasurement(makeProbe{Output: "checked.go"}, nil); got != BaseMeasurementUnknown {
			t.Fatalf("an empty tracked list cannot evidence a measurement, got %v", got)
		}

		// The discriminating control for the screen above. The same tracked
		// path, the same failing run, but the tool launched and reported for
		// itself — so the screen must stay silent and the enumeration must
		// still count. Without this, a screen that rejected everything would
		// pass the two echo rows for the wrong reason.
		ran := makeProbe{Output: "checked.go\nmake: *** [check] Error 1\n"}
		if got := baseMeasurement(ran, tracked); got != BaseMeasured {
			t.Fatalf("a tool that launched and failed on its own still measured, got %v", got)
		}
	})

	t.Run("a path printed inside a sub-make is rebased before it is matched", func(t *testing.T) {
		// extractLocationsForProbe already rebases sub-make paths onto the
		// repository root. When this function did not, `x.go` from a recursive
		// `make check` failed to match the tracked `sub/x.go` and a genuine
		// measured zero read as unmeasured — the safe direction, but the two
		// functions disagreeing about what a printed path means will not stay
		// safe.
		probe := makeProbe{
			Output:  "make[1]: Entering directory '/repo/sub'\nx.go\n",
			WorkDir: "/repo",
		}
		if got := baseMeasurement(probe, []string{"sub/x.go"}); got != BaseMeasured {
			t.Fatalf("a sub-make path must be rebased onto the repo root, got %v", got)
		}
		// The control: rebasing must be driven by the marker, not applied
		// blindly. With no marker the same line is root-relative and must not
		// acquire a prefix.
		bare := makeProbe{Output: "x.go\n", WorkDir: "/repo"}
		if got := baseMeasurement(bare, []string{"sub/x.go"}); got != BaseMeasurementUnknown {
			t.Fatalf("a root-relative path must not be rebased onto a subdirectory, got %v", got)
		}
	})
}
