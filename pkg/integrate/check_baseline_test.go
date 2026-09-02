// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestBaselineSameCountDifferentSetsPass(t *testing.T) {
	// Mandatory DECISION-004 fixture:
	//   - baseline and branch location SETS differ
	//   - counts are equal
	//   - no diagnostic sits on a path this branch changed
	// → PASS.
	//
	// Mutation: if EvaluateBaseline is replaced with a set-diff
	// (setDiffWouldReject), this test must go red. Do not "fix" the
	// fixture to make the sets equal.
	base := []string{
		"internal/script/js_engine.go:193",
		"pkg/foo/bar.go:10",
		"cmd/gz-git/cmd/root.go:40",
	}
	branch := []string{
		"internal/script/js_engine.go:193",
		"pkg/foo/other.go:7",
		"cmd/gz-git/cmd/help.go:12",
	}
	changed := []string{"feat/new.go", "README.md"}

	if !setDiffWouldReject(base, branch) {
		t.Fatal("fixture sets must differ so a future set-diff gate fails this test")
	}
	if len(uniqueSorted(base)) != len(uniqueSorted(branch)) {
		t.Fatal("fixture counts must be equal")
	}

	got := EvaluateBaseline(BaselineInput{
		BranchLocations: branch,
		BaseLocations:   base,
		ChangedPaths:    changed,
	})
	if got.Status != BaselinePass {
		t.Fatalf("same count, different sets, no changed-path diagnostics must PASS, got %+v", got)
	}
}

func TestSetDiffWouldRejectMandatoryFixture(t *testing.T) {
	// Inverted helper: documents that set-diff rejects the fixture the
	// real gate must accept.
	base := []string{"a.go:1", "b.go:2"}
	branch := []string{"a.go:1", "c.go:3"}
	if !setDiffWouldReject(base, branch) {
		t.Fatal("setDiffWouldReject must be true for different equal-size sets")
	}
	if setDiffWouldReject(base, base) {
		t.Fatal("identical sets are not a set-diff reject")
	}
}

func TestEvaluateBaseline_CountIncreaseFails(t *testing.T) {
	got := EvaluateBaseline(BaselineInput{
		BranchLocations: []string{"a.go:1", "b.go:2", "c.go:3"},
		BaseLocations:   []string{"a.go:1", "b.go:2"},
	})
	if got.Status != BaselineFail {
		t.Fatalf("count increase must FAIL, got %+v", got)
	}
}

func TestEvaluateBaseline_ChangedPathFails(t *testing.T) {
	got := EvaluateBaseline(BaselineInput{
		BranchLocations: []string{"feat/new.go:4", "a.go:1"},
		BaseLocations:   []string{"a.go:1", "b.go:2"},
		ChangedPaths:    []string{"feat/new.go"},
	})
	if got.Status != BaselineFail {
		t.Fatalf("diagnostic on changed path must FAIL, got %+v", got)
	}
}

func TestEvaluateBaseline_NoLocationsFails(t *testing.T) {
	got := EvaluateBaseline(BaselineInput{
		BranchLocations: nil,
		BaseLocations:   []string{"a.go:1"},
	})
	if got.Status != BaselineFail {
		t.Fatalf("zero branch locations must FAIL, got %+v", got)
	}
}

func TestExtractLocations_NormalizesPrefix(t *testing.T) {
	out := `../other-checkout/internal/script/js_engine.go:193: oops
internal/script/js_engine.go:200: other`
	got := ExtractLocations(out, []string{"internal/script/js_engine.go"})
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestEvaluateBaseline_ForeignLocationsAreRejectedBeforeNormalization(t *testing.T) {
	output := "../deleted-worktree/internal/script/js_engine.go:193: stale\n./internal/script/js_engine.go:200: local\n"
	if got := foreignDiagnosticLocations(output); len(got) != 1 {
		t.Fatalf("foreign locations = %v", got)
	}
	if got := ExtractLocations(output, []string{"internal/script/js_engine.go"}); len(got) != 2 {
		t.Fatalf("normalizer fixture must demonstrate why the pre-check is needed, got %v", got)
	}
}

func TestExtractLocations_LabelPrefixedLine(t *testing.T) {
	// infra-ops's scripts/link-check.sh and scripts/task-index-check.sh emit
	// "TAG<spaces>path:line: message" instead of a bare "path:line" prefix.
	// Without label stripping this line never matches locationLine, so
	// EvaluateBaseline sees zero locations for both base and branch and
	// fails every run with "no file:line diagnostics" regardless of content.
	out := "BROKEN  tasks/batch.md:9: todo/560-stale.md\n" +
		"DANGLING_ROW    tasks/batch.md:9: todo/560-stale.md\n" +
		"internal/script/js_engine.go:200: unrelated tool output\n"
	got := ExtractLocations(out, []string{"tasks/batch.md", "internal/script/js_engine.go"})
	want := []string{"internal/script/js_engine.go:200", "tasks/batch.md:9"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestExtractLocations_RuffFullOutput(t *testing.T) {
	out := "F401 [*] `pathlib` imported but unused\n" +
		" --> scripts/source_to_instruction.py:2:8\n" +
		"  |\n" +
		"2 | import pathlib\n"
	got := ExtractLocations(out, []string{"scripts/source_to_instruction.py"})
	want := []string{"scripts/source_to_instruction.py:2"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExtractLocations_AriadneOxcFixtures(t *testing.T) {
	// Real, captured rolldown/oxc (vite 8 / rolldown 1.2.5) build failure:
	// three [UNLOADABLE_DEPENDENCY] errors, each with an ariadne-style
	// "╭─[ path:line:col ]" frame opener instead of a leading "path:line".
	// Before this fix ExtractLocations returned 0 for this output — three
	// genuine build failures were invisible to EvaluateBaseline.
	want := []string{
		"src/routes/BuildHistory.svelte:20",
		"src/routes/ProjectDashboard.svelte:12",
		"src/routes/ProjectDashboard.svelte:13",
	}
	tracked := []string{
		"src/routes/BuildHistory.svelte",
		"src/routes/ProjectDashboard.svelte",
	}
	cases := []string{
		"rolldown-oxc-unloadable-dependency-noansi.txt",
		"rolldown-oxc-unloadable-dependency.txt", // raw, with ANSI SGR + docker "#19 12.34 " prefixes
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile("testdata/" + name)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			got := ExtractLocations(string(raw), tracked)
			if len(got) != len(want) {
				t.Fatalf("got %v, want %v", got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("got %v, want %v", got, want)
				}
			}
		})
	}
}

func TestForeignDiagnosticLocations_RuffFullOutput(t *testing.T) {
	output := " --> ../other-worktree/check.py:3:7\n" +
		" --> /absolute/check.py:4:2\n" +
		" --> ./local/check.py:5:1\n"
	got := foreignDiagnosticLocations(output)
	want := []string{"../other-worktree/check.py:3", "/absolute/check.py:4"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// strings.TrimSpace erases the only signal (indentation depth) that
// separates a go test -v subtest's t.Skip/t.Log line from a t.Errorf
// line — both print as "    file.go:42: message". Without
// suppressGoTestVerboseNoise, a run full of skips is counted as a run full
// of failures.

func TestExtractLocations_GoTestVerboseSkipBlockNotCounted(t *testing.T) {
	out := "=== RUN   TestFoo\n" +
		"    foo_test.go:10: skipping: not applicable in this environment\n" +
		"--- SKIP: TestFoo (0.00s)\n" +
		"PASS\n" +
		"ok  \texample.com/pkg\t0.002s\n"
	got := ExtractLocations(out, []string{"foo_test.go"})
	if len(got) != 0 {
		t.Fatalf("a --- SKIP: block's indented line must not count as a diagnostic, got %v", got)
	}
}

func TestExtractLocations_GoTestVerboseFailBlockCounted(t *testing.T) {
	out := "=== RUN   TestBar\n" +
		"    bar_test.go:20: got 1, want 2\n" +
		"    bar_test.go:21: additional context\n" +
		"--- FAIL: TestBar (0.00s)\n" +
		"FAIL\n" +
		"FAIL\texample.com/pkg\t0.003s\n"
	want := []string{"bar_test.go:20", "bar_test.go:21"}
	got := ExtractLocations(out, []string{"bar_test.go"})
	if len(got) != len(want) {
		t.Fatalf("a --- FAIL: block's indented lines must both count, got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestExtractLocations_GoTestVerboseMixedRunOnlyCountsFail(t *testing.T) {
	out := "=== RUN   TestFoo\n" +
		"=== RUN   TestFoo/skip_case\n" +
		"    foo_test.go:10: skipping: not applicable\n" +
		"--- SKIP: TestFoo/skip_case (0.00s)\n" +
		"=== RUN   TestFoo/fail_case\n" +
		"    foo_test.go:20: assertion failed\n" +
		"    foo_test.go:21: more context\n" +
		"--- FAIL: TestFoo/fail_case (0.00s)\n" +
		"--- FAIL: TestFoo (0.00s)\n" +
		"=== RUN   TestBaz\n" +
		"--- PASS: TestBaz (0.00s)\n" +
		"FAIL\n" +
		"FAIL\texample.com/pkg\t0.004s\n"
	want := []string{"foo_test.go:20", "foo_test.go:21"}
	got := ExtractLocations(out, []string{"foo_test.go"})
	if len(got) != len(want) {
		t.Fatalf("mixed run must count only the --- FAIL: block's lines, got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// TestEvaluateBaseline_UnmeasurableBaselineIsNotFail covers the state that made
// notifire-backend-phoenix unlandable: its target tip's `make check` dies before
// mix can emit anything (no deps/ in the detached baseline worktree), so the
// baseline measures zero. Both ways out were closed — a branch that also failed
// to run hit the "no file:line diagnostics" rule, and a branch that did run was
// read as "count increased (0 -> N)". Neither says anything about the branch.
func TestEvaluateBaseline_UnmeasurableBaselineIsNotFail(t *testing.T) {
	t.Run("branch also produced nothing", func(t *testing.T) {
		got := EvaluateBaseline(BaselineInput{BranchLocations: nil, BaseLocations: nil})
		if got.Status != BaselineUnmeasurable {
			t.Fatalf("both sides unmeasured must be BaselineUnmeasurable, got %+v", got)
		}
	})
	t.Run("branch ran and the baseline did not", func(t *testing.T) {
		branch := make([]string, 0, 190)
		for i := 1; i <= 190; i++ {
			branch = append(branch, fmt.Sprintf("lib/app_%d.ex:%d", i, i))
		}
		got := EvaluateBaseline(BaselineInput{
			BranchLocations: branch,
			BaseLocations:   nil,
			ChangedPaths:    []string{"Makefile"},
		})
		if got.Status == BaselineFail {
			t.Fatalf("an unmeasured baseline must not be reported as a count increase, got %+v", got)
		}
		if got.Status != BaselineUnmeasurable {
			t.Fatalf("want BaselineUnmeasurable, got %+v", got)
		}
	})
	t.Run("a diagnostic on a changed path still fails", func(t *testing.T) {
		got := EvaluateBaseline(BaselineInput{
			BranchLocations: []string{"lib/new.ex:4"},
			BaseLocations:   nil,
			ChangedPaths:    []string{"lib/new.ex"},
		})
		if got.Status != BaselineFail {
			t.Fatalf("evidence of harm outranks an unmeasurable baseline, got %+v", got)
		}
	})
}

// TestEvaluateBaseline_BaselineZeroIsNotClean pins the distinction the gate was
// missing. Zero baseline diagnostics from a run that FAILED is a failed
// measurement, and must not be read as the clean baseline that a real zero-vs-N
// comparison would imply. The contrast case keeps a measured baseline behaving
// exactly as before.
func TestEvaluateBaseline_BaselineZeroIsNotClean(t *testing.T) {
	unmeasured := EvaluateBaseline(BaselineInput{
		BranchLocations: []string{"lib/a.ex:1"},
		BaseLocations:   nil,
	})
	if unmeasured.Status != BaselineUnmeasurable {
		t.Fatalf("zero base must read as unmeasured, got %+v", unmeasured)
	}
	if strings.Contains(unmeasured.Reason, "count increased") {
		t.Fatalf("unmeasured baseline must not be described as a count increase: %q", unmeasured.Reason)
	}
	measured := EvaluateBaseline(BaselineInput{
		BranchLocations: []string{"lib/a.ex:1", "lib/b.ex:2"},
		BaseLocations:   []string{"lib/a.ex:1"},
	})
	if measured.Status != BaselineFail {
		t.Fatalf("a measured baseline must still catch a real increase, got %+v", measured)
	}
}

// TestEvaluateBaseline_PrepareProfileAbsentDoesNotForceFail is the same defect
// seen from prepare.go's side: runPrepareProfile knows one profile, so any other
// stack reaches baselineAgainstTarget with an unprepared detached worktree. The
// gate must not convert that into a verdict about the branch.
func TestEvaluateBaseline_PrepareProfileAbsentDoesNotForceFail(t *testing.T) {
	for _, changed := range [][]string{nil, {"Makefile"}, {"mix.exs"}} {
		got := EvaluateBaseline(BaselineInput{
			BranchLocations: []string{"lib/a.ex:1", "lib/b.ex:2"},
			BaseLocations:   nil,
			ChangedPaths:    changed,
		})
		if got.Status != BaselineUnmeasurable {
			t.Fatalf("changed=%v: want BaselineUnmeasurable, got %+v", changed, got)
		}
	}
}

// TestBaselineUnmeasurableGatesUnlessAllowed pins the property the first cut of
// this change got wrong. An unmeasurable baseline was reported as checkWarn,
// and a warn does not gate: report.Ready is report.Failures == 0 and warns
// only touch report.Warnings. The gate therefore stopped gating in exactly the
// repositories the new status exists for. It must fail unless the operator
// explicitly downgrades it, the same contract every other skipped check has.
func TestBaselineUnmeasurableGatesUnlessAllowed(t *testing.T) {
	verdict := BaselineResult{Status: BaselineUnmeasurable, Reason: "target tip emitted nothing"}

	blocking := baselineCheckItem("make check", verdict, false)
	if blocking.Status != checkFail {
		t.Fatalf("an unmeasured gate must block by default, got status %v (%s)", blocking.Status, blocking.Detail)
	}
	if !strings.Contains(blocking.Detail, "--allow-skipped-checks") {
		t.Fatalf("the operator must be told how to proceed, got %q", blocking.Detail)
	}

	downgraded := baselineCheckItem("make check", verdict, true)
	if downgraded.Status != checkWarn {
		t.Fatalf("--allow-skipped-checks must downgrade, got status %v (%s)", downgraded.Status, downgraded.Detail)
	}

	// A real worsening is never downgradable, flag or no flag.
	worsened := BaselineResult{Status: BaselineFail, Reason: "diagnostics on changed paths: lib/new.ex:4"}
	for _, allowed := range []bool{false, true} {
		if got := baselineCheckItem("make check", worsened, allowed); got.Status != checkFail {
			t.Fatalf("allowSkipped=%v downgraded a real failure: %v (%s)", allowed, got.Status, got.Detail)
		}
	}
}

// TestNormalizeTrackedPathPrefixLift pins the direction normalizeTrackedPath
// used to be missing. A recipe of the form "cd apps/api && mix compile" makes
// its tool print lib/foo.ex for a file the repository tracks as
// apps/api/lib/foo.ex. The old code could only remove components, so it ground
// the path down to foo.ex, which matches no entry in the changed-path set —
// the changed-paths rule could not fire at all on such a repository.
func TestNormalizeTrackedPathPrefixLift(t *testing.T) {
	tracked := newTrackedIndex([]string{
		"apps/api/lib/foo.ex",
		"apps/api/lib/bar.ex",
		"Makefile",
	})

	if got := normalizeTrackedPath("lib/foo.ex", tracked); got != "apps/api/lib/foo.ex" {
		t.Fatalf("subdirectory-relative path: got %q, want apps/api/lib/foo.ex", got)
	}
	if got := normalizeTrackedPath("./lib/foo.ex", tracked); got != "apps/api/lib/foo.ex" {
		t.Fatalf("dot-prefixed path: got %q, want apps/api/lib/foo.ex", got)
	}
	// Guards, not evidence of the lift: both of these also pass on the
	// strip-only code this test was written for. They are here so a future
	// change to the lift cannot quietly break the two directions that already
	// worked.
	if got := normalizeTrackedPath("apps/api/lib/foo.ex", tracked); got != "apps/api/lib/foo.ex" {
		t.Fatalf("already root-relative path must be untouched: got %q", got)
	}
	if got := normalizeTrackedPath("/build/wrap/Makefile", tracked); got != "Makefile" {
		t.Fatalf("over-qualified path must still be stripped: got %q, want Makefile", got)
	}
}

// TestNormalizeTrackedPathBareBaseNameIsNotLifted pins the boundary the lift
// must not cross. Go's testing package decorates every t.Log and t.Errorf line
// with filepath.Base, so a bare file name is the ordinary output of a passing
// or skipping test, not a subdirectory-relative path. An earlier revision of
// the lift accepted it whenever the match happened to be unique, which turned
// a clean t.Skip message into a hard rule (a) failure for any branch that
// touched that test file. This test fails on that revision.
func TestNormalizeTrackedPathBareBaseNameIsNotLifted(t *testing.T) {
	tracked := newTrackedIndex([]string{
		"pkg/integrate/check_baseline_test.go",
		"cmd/gz-git/main.go",
	})

	for _, name := range []string{"check_baseline_test.go", "main.go"} {
		if got := normalizeTrackedPath(name, tracked); got != name {
			t.Fatalf("bare base name must not be lifted: %q became %q", name, got)
		}
	}
}

// TestExtractLocationsGoTestSkipDoesNotBlockChangedTestFile is the end-to-end
// consequence of the rule above, on real go test -v output.
// suppressGoTestVerboseNoise cannot blank the skip line here: an interleaved
// "=== CONT" clears the pending buffer before the "--- SKIP" trailer that
// would identify it as noise arrives, so the line survives into the location
// set. It must still not be attributable to a tracked path, or a branch that
// merely edited a_test.go would be blocked by a test that skipped cleanly on
// both sides of the comparison.
func TestExtractLocationsGoTestSkipDoesNotBlockChangedTestFile(t *testing.T) {
	out := `=== RUN   TestA
=== PAUSE TestA
=== RUN   TestB
=== PAUSE TestB
=== CONT  TestA
    a_test.go:10: skipping: no docker
=== CONT  TestB
    b_test.go:20: assertion failed
--- FAIL: TestB (0.00s)
--- SKIP: TestA (0.00s)
`
	tracked := []string{"pkg/foo/a_test.go", "pkg/foo/b_test.go"}

	got := ExtractLocations(out, tracked)

	// Floor first. Without it this test passes when extraction returns
	// nothing at all: the loop below never runs, and EvaluateBaseline answers
	// BaselineUnmeasurable, which is not BaselineFail — both assertions would
	// hold while measuring nothing. Pinning the exact slice keeps this a test
	// of non-attribution rather than a test of silence.
	want := []string{"a_test.go:10", "b_test.go:20"}
	if len(got) != len(want) {
		t.Fatalf("extraction changed: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("extraction changed: got %v, want %v", got, want)
		}
	}

	for _, loc := range got {
		if strings.Contains(loc, "/") {
			t.Fatalf("go test base names must stay unattributed: got %v", got)
		}
	}

	verdict := EvaluateBaseline(BaselineInput{
		BranchLocations: got,
		BaseLocations:   got,
		ChangedPaths:    []string{"pkg/foo/a_test.go"},
	})
	if verdict.Status == BaselineFail {
		t.Fatalf("a skip message present on both sides must not fail the branch: %s", verdict.Reason)
	}
}

// TestNormalizeTrackedPathPrefixAmbiguityIsNotGuessed keeps the lift honest.
// It is a guard, not evidence of the change: the strip-only code returned
// foo.ex here too, for want of any lift at all.
// Two tracked files ending in the same suffix mean the output does not say
// which one the tool meant; attributing the diagnostic to either would blame a
// file the branch may never have touched. The path is left unmatched instead,
// which is what the old code did for every such input anyway.
func TestNormalizeTrackedPathPrefixAmbiguityIsNotGuessed(t *testing.T) {
	tracked := newTrackedIndex([]string{
		"apps/api/lib/foo.ex",
		"apps/web/lib/foo.ex",
	})

	if got := normalizeTrackedPath("lib/foo.ex", tracked); got != "foo.ex" {
		t.Fatalf("ambiguous suffix must not be guessed: got %q, want foo.ex", got)
	}
}

// TestNormalizeTrackedPathLiftsBeforeStripping is why the lift runs on the
// path as emitted rather than after each strip. A compiled copy under _build
// is a different file with a different history; rewriting it onto the source
// it was generated from would invent evidence.
//
// The second case records the limit of that ordering rather than a property of
// it. Lifting first only protects an artifact tree nested deeper than its
// source; an artifact emitted one level shallower still lands on its tracked
// lookalike, because a unique suffix match is a coincidence and this function
// has no way to tell a coincidence from a working directory.
//
// Only the first assertion is a guard: strip-only code also ground
// _build/dev/lib/foo.ex down to foo.ex, so it discriminates nothing and is
// here to keep the artifact case visible. The second is not — strip-only code
// answered "bundle.js" for dist/bundle.js, so that assertion fails against it.
// It characterizes the lift's new and unwanted reach, and that reach is live
// rather than theoretical: dist/bundle.js:4 against a tracked
// src/dist/bundle.js reaches rule (a) and hard-fails the branch, which is the
// same false block this change set removed elsewhere.
func TestNormalizeTrackedPathLiftsBeforeStripping(t *testing.T) {
	tracked := newTrackedIndex([]string{"apps/api/lib/foo.ex"})

	if got := normalizeTrackedPath("_build/dev/lib/foo.ex", tracked); got != "foo.ex" {
		t.Fatalf("build artifact must not be lifted onto its source: got %q, want foo.ex", got)
	}

	shallow := newTrackedIndex([]string{"src/dist/bundle.js"})
	if got := normalizeTrackedPath("dist/bundle.js", shallow); got != "src/dist/bundle.js" {
		t.Fatalf("known limit no longer present: got %q, want src/dist/bundle.js — "+
			"fixing this limit is expected, so update this test rather than "+
			"restoring the old behavior", got)
	}
}

// TestExtractLocationsPrefixLiftFeedsChangedPathRule is the end-to-end shape
// the card reports: a cd-ing Makefile's output, the repository's tracked set,
// and the changed-path set the branch actually produced.
func TestExtractLocationsPrefixLiftFeedsChangedPathRule(t *testing.T) {
	out := "lib/foo.ex:12: warning: variable \"x\" is unused\n"
	tracked := []string{"apps/api/lib/foo.ex", "apps/api/mix.exs"}

	got := ExtractLocations(out, tracked)
	want := []string{"apps/api/lib/foo.ex:12"}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("ExtractLocations: got %v, want %v", got, want)
	}

	verdict := EvaluateBaseline(BaselineInput{
		BranchLocations: got,
		BaseLocations:   nil,
		ChangedPaths:    []string{"apps/api/lib/foo.ex"},
	})
	if verdict.Status != BaselineFail {
		t.Fatalf("a diagnostic on a changed path must fail: got status %v (%s)", verdict.Status, verdict.Reason)
	}
}

// TestNormalizeTrackedPathDuplicateTrackedEntryStillLifts pins that ambiguity
// means two different files, not the same file listed twice. git normally emits
// each path once, but nothing in this package's signature promises that, and a
// duplicate silently costing a lift would be indistinguishable from a genuine
// ambiguity refusal.
func TestNormalizeTrackedPathDuplicateTrackedEntryStillLifts(t *testing.T) {
	tracked := newTrackedIndex([]string{
		"apps/api/lib/foo.ex",
		"apps/api/lib/foo.ex",
	})
	if got := normalizeTrackedPath("lib/foo.ex", tracked); got != "apps/api/lib/foo.ex" {
		t.Fatalf("duplicate tracked entry defeated the lift: got %q", got)
	}
}

// TestNormalizeTrackedPathCleansBeforeMatching covers the spellings a tool may
// emit for a path it means literally. The bare base name is here as the limit:
// cleaning must not manufacture a directory component that would make it
// liftable.
func TestNormalizeTrackedPathCleansBeforeMatching(t *testing.T) {
	tracked := newTrackedIndex([]string{"apps/api/lib/foo.ex"})
	for _, spelling := range []string{
		"lib/foo.ex",
		"./lib/foo.ex",
		"lib//foo.ex",
		"lib/./foo.ex",
		"sub/../lib/foo.ex",
	} {
		if got := normalizeTrackedPath(spelling, tracked); got != "apps/api/lib/foo.ex" {
			t.Errorf("%q: got %q, want apps/api/lib/foo.ex", spelling, got)
		}
	}
	if got := normalizeTrackedPath("./foo.ex", tracked); got != "foo.ex" {
		t.Fatalf("cleaning must not make a bare base name liftable: got %q", got)
	}
}

// TestEvaluateBaselinePreexistingDiagnosticOnChangedPathIsNotHarm pins the case
// the card measured on 59c1f60: base and branch reporting the identical
// diagnostic, on a path the branch touched. Rule (a) used to hard-fail this,
// which made any file already carrying a warning unmodifiable — the input says
// outright that nothing worsened.
func TestEvaluateBaselinePreexistingDiagnosticOnChangedPathIsNotHarm(t *testing.T) {
	got := EvaluateBaseline(BaselineInput{
		BranchLocations: []string{"a.go:1"},
		BaseLocations:   []string{"a.go:1"},
		ChangedPaths:    []string{"a.go"},
	})
	if got.Status == BaselineFail {
		t.Fatalf("identical base and branch diagnostic is not harm, got %+v", got)
	}
	if got.Status != BaselinePass {
		t.Fatalf("want PASS, got %+v", got)
	}

	// Also when the branch removed one: strictly better must not fail either.
	better := EvaluateBaseline(BaselineInput{
		BranchLocations: []string{"a.go:1"},
		BaseLocations:   []string{"a.go:1", "a.go:7"},
		ChangedPaths:    []string{"a.go"},
	})
	if better.Status != BaselinePass {
		t.Fatalf("removing a diagnostic must not fail, got %+v", better)
	}
}

// TestEvaluateBaselineNewDiagnosticOnChangedPathStillFails guards the other
// direction: the subtraction must not blunt rule (a) into uselessness. A
// diagnostic the branch genuinely added on a path it changed is still harm,
// and the reason must name that one rather than the pre-existing neighbor.
func TestEvaluateBaselineNewDiagnosticOnChangedPathStillFails(t *testing.T) {
	got := EvaluateBaseline(BaselineInput{
		BranchLocations: []string{"a.go:1", "a.go:9"},
		BaseLocations:   []string{"a.go:1"},
		ChangedPaths:    []string{"a.go"},
	})
	if got.Status != BaselineFail {
		t.Fatalf("newly added diagnostic on a changed path must FAIL, got %+v", got)
	}
	if !strings.Contains(got.Reason, "a.go:9") {
		t.Fatalf("reason must name the new diagnostic, got %q", got.Reason)
	}
	if strings.Contains(got.Reason, "a.go:1 ") || strings.HasSuffix(got.Reason, "a.go:1") {
		t.Fatalf("reason must not blame the pre-existing diagnostic, got %q", got.Reason)
	}

	// Same file:line but a path the branch did not touch stays out of rule (a).
	untouched := EvaluateBaseline(BaselineInput{
		BranchLocations: []string{"a.go:1", "b.go:9"},
		BaseLocations:   []string{"a.go:1"},
		ChangedPaths:    []string{"a.go"},
	})
	if untouched.Status != BaselineFail {
		t.Fatalf("count increase still fails via rule (c), got %+v", untouched)
	}
	if strings.Contains(untouched.Reason, "diagnostics on changed paths") {
		t.Fatalf("rule (a) must not claim an unchanged path, got %q", untouched.Reason)
	}
}
