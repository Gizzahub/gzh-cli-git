// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"os"
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
