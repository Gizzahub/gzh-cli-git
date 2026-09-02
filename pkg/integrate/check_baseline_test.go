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
