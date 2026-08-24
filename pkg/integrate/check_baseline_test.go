// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
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
