// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// updateGolden rewrites the files under testdata instead of comparing against
// them. Column padding is not something worth transcribing by hand, and a
// golden that was hand-counted tests the transcription more than the code.
var updateGolden = flag.Bool("update-golden", false, "rewrite testdata golden files")

// assertGolden compares out against testdata/<name>.golden.
func assertGolden(t *testing.T, name, out string) {
	t.Helper()

	path := filepath.Join("testdata", name+".golden")

	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(path, []byte(out), 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}

		return
	}

	want, err := os.ReadFile(path) //nolint:gosec // fixed test-local path
	if err != nil {
		t.Fatalf("read golden (run with -update-golden to create it): %v", err)
	}

	if out == string(want) {
		return
	}

	// Line-by-line so the first divergence is readable; whole-file diffs of
	// padded columns are not.
	gotLines := strings.Split(out, "\n")
	wantLines := strings.Split(string(want), "\n")
	for i := 0; i < max(len(gotLines), len(wantLines)); i++ {
		got, want := lineAt(gotLines, i), lineAt(wantLines, i)
		if got != want {
			t.Fatalf("%s: line %d differs\n  got:  %q\n  want: %q", path, i+1, got, want)
		}
	}
	t.Fatalf("%s: output differs but no line does; check trailing newline", path)
}

func lineAt(lines []string, i int) string {
	if i >= len(lines) {
		return "<missing>"
	}

	return lines[i]
}

// withDiffDisplay pins the globals the display functions read, so each test
// describes exactly one output mode.
func withDiffDisplay(t *testing.T, format string, verboseMode, noContent bool) {
	t.Helper()

	origFormat, origVerbose, origNoContent := diffFlags.Format, verbose, diffNoDiffContent
	diffFlags.Format, verbose, diffNoDiffContent = format, verboseMode, noContent

	t.Cleanup(func() {
		diffFlags.Format, verbose, diffNoDiffContent = origFormat, origVerbose, origNoContent
	})
}

// diffResultWithUntracked mirrors the reported case: four tracked files that
// `git diff` can compare, plus three untracked ones that only `git add -A`
// records — two of them nested inside directories that porcelain used to
// collapse to `docs/`. Durations are fixed so the output is deterministic.
func diffResultWithUntracked() *repository.BulkDiffResult {
	return &repository.BulkDiffResult{
		TotalScanned:     2,
		TotalWithChanges: 1,
		TotalClean:       1,
		Duration:         1500 * time.Millisecond,
		Summary:          map[string]int{"has-changes": 1, "clean": 1},
		Repositories: []repository.RepositoryDiffResult{
			{
				Path:                  "/w/repoA",
				RelativePath:          "repoA",
				Branch:                "master",
				Status:                "has-changes",
				Scope:                 "head",
				FilesChanged:          4,
				TrackedFilesChanged:   4,
				UntrackedFilesChanged: 3,
				StagedFilesChanged:    1,
				Additions:             12,
				Deletions:             4,
				DiffSummary:           "7 files changed, 12 insertions(+), 4 deletions(-)",
				DiffContent:           "diff --git a/tracked1.txt b/tracked1.txt\n@@ -1 +1 @@\n-old\n+new\n",
				ChangedFiles: []repository.ChangedFile{
					{Path: "tracked1.txt", Status: "M"},
					{Path: "renamed.txt", Status: "R", OldPath: "original.txt"},
				},
				UntrackedFiles: []string{"docs/adr/0005.md", "docs/adr/0006.md", "tasks/todo.md"},
				OmittedFiles: []repository.OmittedFile{
					{Path: "tasks/todo.md", Reason: "too-large"},
				},
				Duration: 200 * time.Millisecond,
			},
			{
				Path:         "/w/repoB",
				RelativePath: "repoB",
				Branch:       "develop",
				Status:       "clean",
				Duration:     100 * time.Millisecond,
			},
		},
	}
}

// diffResultTrackedOnly is the same shape with nothing untracked, which is the
// case that must stay free of new noise.
func diffResultTrackedOnly() *repository.BulkDiffResult {
	result := diffResultWithUntracked()

	repo := &result.Repositories[0]
	repo.UntrackedFilesChanged = 0
	repo.UntrackedFiles = nil
	repo.OmittedFiles = nil
	repo.DiffSummary = "4 files changed, 12 insertions(+), 4 deletions(-)"

	return result
}

// TestDiffDefaultFormatShowsUntracked pins the fix for the reported symptom:
// the default format printed a tracked-only file count with no hint that more
// files would be committed.
func TestDiffDefaultFormatShowsUntracked(t *testing.T) {
	withDiffDisplay(t, "default", false, false)

	out := captureStdout(t, func() { displayDiffResults(diffResultWithUntracked()) })

	if !strings.Contains(out, "4 files (+3 untracked)") {
		t.Errorf("default output does not name the untracked count:\n%s", out)
	}
	if !strings.Contains(out, "⚠ 1 untracked file omitted from the diff body") {
		t.Errorf("default output does not warn about omitted files:\n%s", out)
	}

	assertGolden(t, "diff_default_untracked", out)
}

// TestDiffDefaultFormatQuietWhenNothingUntracked covers the no-noise criterion:
// a repository with only tracked changes must read exactly as it did before.
func TestDiffDefaultFormatQuietWhenNothingUntracked(t *testing.T) {
	withDiffDisplay(t, "default", false, false)

	out := captureStdout(t, func() { displayDiffResults(diffResultTrackedOnly()) })

	if strings.Contains(out, "untracked") {
		t.Errorf("default output mentions untracked files when there are none:\n%s", out)
	}
	if strings.Contains(out, "omitted") {
		t.Errorf("default output warns about omissions when there were none:\n%s", out)
	}

	assertGolden(t, "diff_default_tracked_only", out)
}

// TestDiffCompactFormatShowsUntrackedColumn pins the extra column and the
// "Tracked" rename that keeps the two file counts from being confused.
func TestDiffCompactFormatShowsUntrackedColumn(t *testing.T) {
	withDiffDisplay(t, "compact", false, false)

	out := captureStdout(t, func() { displayDiffResults(diffResultWithUntracked()) })

	if !strings.Contains(out, "Untracked") {
		t.Errorf("compact table has no untracked column:\n%s", out)
	}
	if strings.Contains(out, "Files") {
		t.Errorf("compact table still calls the tracked column \"Files\" next to an untracked one:\n%s", out)
	}
	if !strings.Contains(out, "[1 omitted]") {
		t.Errorf("compact table does not flag omitted files:\n%s", out)
	}

	assertGolden(t, "diff_compact_untracked", out)
}

// TestDiffCompactFormatKeepsOldShapeWhenNothingUntracked covers the no-noise
// criterion for the table: a column of zeroes is worse than no column.
func TestDiffCompactFormatKeepsOldShapeWhenNothingUntracked(t *testing.T) {
	withDiffDisplay(t, "compact", false, false)

	out := captureStdout(t, func() { displayDiffResults(diffResultTrackedOnly()) })

	if strings.Contains(out, "Untracked") {
		t.Errorf("compact table grew an untracked column with nothing to put in it:\n%s", out)
	}
	if !strings.Contains(out, "Files") {
		t.Errorf("compact table lost its Files column:\n%s", out)
	}

	assertGolden(t, "diff_compact_tracked_only", out)
}

// TestDiffJSONFormatExposesIndividualPaths covers the criterion that untracked
// paths reach structured consumers as real files rather than the collapsed
// `docs/` entries porcelain used to emit.
func TestDiffJSONFormatExposesIndividualPaths(t *testing.T) {
	withDiffDisplay(t, "json", false, false)

	out := captureStdout(t, func() { displayDiffResults(diffResultWithUntracked()) })

	var decoded DiffJSONOutput
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}

	repo := decoded.Repositories[0]
	want := []string{"docs/adr/0005.md", "docs/adr/0006.md", "tasks/todo.md"}
	if !slices.Equal(repo.UntrackedFiles, want) {
		t.Errorf("untracked_files = %q, want %q", repo.UntrackedFiles, want)
	}
	for _, path := range repo.UntrackedFiles {
		if strings.HasSuffix(path, "/") {
			t.Errorf("untracked_files contains a collapsed directory: %q", path)
		}
	}
	if repo.TrackedFilesChanged+repo.UntrackedFilesChanged != 7 {
		t.Errorf("tracked(%d) + untracked(%d) != 7, the number a commit would record",
			repo.TrackedFilesChanged, repo.UntrackedFilesChanged)
	}
	if repo.Scope != "head" {
		t.Errorf("scope = %q, want \"head\"", repo.Scope)
	}

	assertGolden(t, "diff_json_untracked", out)
}

// TestDiffJSONFormatOmitsEmptyKeys pins the back-compatibility promise from
// task 01: the new keys are omitempty, so a repository with nothing untracked
// serializes exactly as it did before they existed.
func TestDiffJSONFormatOmitsEmptyKeys(t *testing.T) {
	withDiffDisplay(t, "json", false, false)

	out := captureStdout(t, func() { displayDiffResults(diffResultTrackedOnly()) })

	if strings.Contains(out, "untracked_files_changed") {
		t.Errorf("untracked_files_changed present with a zero value:\n%s", out)
	}
	if strings.Contains(out, "omitted_files") {
		t.Errorf("omitted_files present with nothing omitted:\n%s", out)
	}

	assertGolden(t, "diff_json_tracked_only", out)
}

// TestDiffLLMFormatShowsUntracked keeps the agent-facing format honest: the
// workflow that started this whole issue feeds it straight into a commit
// message. SUMMARY map keys are sorted by gzh-cli-core WriteLLM (formatMap),
// so golden comparison uses raw output — requires local go.work (or a
// published core that includes sorted maps).
func TestDiffLLMFormatShowsUntracked(t *testing.T) {
	withDiffDisplay(t, "llm", false, false)

	out := captureStdout(t, func() { displayDiffResults(diffResultWithUntracked()) })

	for _, path := range []string{"docs/adr/0005.md", "docs/adr/0006.md", "tasks/todo.md"} {
		if !strings.Contains(out, path) {
			t.Errorf("llm output is missing untracked path %q:\n%s", path, out)
		}
	}
	if !strings.Contains(out, "UNTRACKED_FILES_CHANGED: 3") {
		t.Errorf("llm output does not carry the untracked count:\n%s", out)
	}

	assertGolden(t, "diff_llm_untracked", out)
}

// TestDiffVerboseNoContentKeepsSummary covers the flag-interaction criterion:
// --no-content drops the diff body and nothing else. The file lists and the
// omission warning are what a reader falls back on once the body is gone, so
// dropping them too would leave no evidence at all.
func TestDiffVerboseNoContentKeepsSummary(t *testing.T) {
	withDiffDisplay(t, "default", true, true)

	out := captureStdout(t, func() { displayDiffResults(diffResultWithUntracked()) })

	if strings.Contains(out, "--- Diff ---") {
		t.Errorf("--no-content still printed the diff body:\n%s", out)
	}
	for _, want := range []string{
		"7 files changed, 12 insertions(+), 4 deletions(-)",
		"docs/adr/0005.md",
		"original.txt → renamed.txt",
		"⚠ 1 untracked file omitted from the diff body:",
		"! tasks/todo.md (too-large)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("--no-content dropped %q:\n%s", want, out)
		}
	}

	assertGolden(t, "diff_verbose_no_content", out)
}
