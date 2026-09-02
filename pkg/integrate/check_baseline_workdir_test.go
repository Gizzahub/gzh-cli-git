// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestExtractLocationsUsesToolWorkingDirectory pins the replacement of the
// suffix guess with an observation. Both halves describe the same rule from
// opposite sides: when make says where it is, that directory decides, and when
// make is at the repository root, a diagnostic is root-relative as printed and
// must not be relocated to somewhere a suffix happens to match.
func TestExtractLocationsUsesToolWorkingDirectory(t *testing.T) {
	root := t.TempDir()

	t.Run("sub-make directory attributes an otherwise undecidable diagnostic", func(t *testing.T) {
		// Two tracked files end in lib/foo.go, so the suffix alone is
		// ambiguous and liftToTracked refuses it. The -w marker resolves it,
		// which a suffix rule cannot do at any level of cleverness.
		probe := makeProbe{
			WorkDir: root,
			Output: "make[1]: Entering directory '" + filepath.Join(root, "sub") + "'\n" +
				"lib/foo.go:3: undefined: x\n" +
				"make[1]: Leaving directory '" + filepath.Join(root, "sub") + "'\n",
		}
		got := extractLocationsForProbe(probe, []string{"sub/lib/foo.go", "other/lib/foo.go"})
		want := []string{"sub/lib/foo.go:3"}
		if len(got) != len(want) || got[0] != want[0] {
			t.Fatalf("marker directory must decide attribution: got %v, want %v", got, want)
		}
	})

	t.Run("root-run make does not lift onto a coincidental suffix", func(t *testing.T) {
		// The card's case: a build at the repository root reports its own
		// dist/bundle.js. src/dist/bundle.js is the unique suffix match, so
		// the old rule relocated the diagnostic there -- and rule (a) then
		// hard-blocked a branch that had touched src/dist/bundle.js and had
		// nothing to do with the failure.
		probe := makeProbe{WorkDir: root, Output: "dist/bundle.js:4: parse error\n"}
		got := extractLocationsForProbe(probe, []string{"src/dist/bundle.js"})
		for _, loc := range got {
			if strings.HasPrefix(loc, "src/") {
				t.Fatalf("root-run diagnostic must not be lifted into src/: got %v", got)
			}
		}
		if len(got) != 1 || got[0] != "dist/bundle.js:4" {
			t.Fatalf("root-relative diagnostic must survive as printed: got %v", got)
		}
	})
}

// TestRootLevelTestFileSkipDoesNotBlockBranch covers the residue TASK-138 left
// open. normalizeTrackedPath consulted tracked.exact before anything else, and
// a repository with a root-level version_test.go has a tracked entry that a
// go test skip line matches character for character -- so a test that skipped
// cleanly on both sides of the comparison hard-failed the branch that touched
// the file. This repository has such a file, so the fixture is not synthetic.
func TestRootLevelTestFileSkipDoesNotBlockBranch(t *testing.T) {
	// The interleaved "=== CONT" clears the pending buffer before the
	// "--- SKIP" trailer arrives, so suppressGoTestVerboseNoise cannot blank
	// this line. It reaches attribution exactly as a real diagnostic would.
	out := "=== RUN   TestVersion\n" +
		"=== PAUSE TestVersion\n" +
		"=== RUN   TestOther\n" +
		"=== PAUSE TestOther\n" +
		"=== CONT  TestVersion\n" +
		"    version_test.go:10: skipping: no network\n" +
		"=== CONT  TestOther\n" +
		"--- SKIP: TestVersion (0.00s)\n" +
		"--- PASS: TestOther (0.00s)\n"

	got := ExtractLocations(out, []string{"version_test.go", "pkg/foo/other_test.go"})
	for _, loc := range got {
		if loc == "version_test.go:10" {
			t.Fatalf("a bare base name must not land on the tracked root-level file: got %v", got)
		}
	}

	verdict := EvaluateBaseline(BaselineInput{
		BranchLocations: got,
		BaseLocations:   got,
		ChangedPaths:    []string{"version_test.go"},
	})
	if verdict.Status == BaselineFail {
		t.Fatalf("a clean skip in a changed root-level test must not fail the branch: %s", verdict.Reason)
	}
}

// TestLiftRejectsMutuallyExclusiveWorkingDirectories covers the third defect:
// liftToTracked judged one diagnostic at a time, so a probe could be resolved
// into two directories that exclude one another. Each lift looked perfectly
// unique on its own; only the combination was impossible, and nothing was
// looking at the combination.
func TestLiftRejectsMutuallyExclusiveWorkingDirectories(t *testing.T) {
	tracked := []string{"apps/api/lib/foo.ex", "apps/web/lib/bar.ex"}
	// No WorkDir: this is the no-evidence path, where lifting is still allowed
	// in principle. It must be refused here for the different reason that the
	// two lifts contradict each other -- a single probe ran in one directory.
	got := ExtractLocations("lib/foo.ex:1: warning\nlib/bar.ex:2: warning\n", tracked)

	for _, loc := range got {
		if strings.HasPrefix(loc, "apps/") {
			t.Fatalf("contradictory lifts must all be refused, not resolved: got %v", got)
		}
	}
	want := []string{"lib/bar.ex:2", "lib/foo.ex:1"}
	if len(got) != len(want) {
		t.Fatalf("both diagnostics must survive unlifted: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
