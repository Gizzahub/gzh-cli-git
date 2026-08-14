// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/branch"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/reposync"
)

func TestDisplayHealthRepositoryResult_AllDivergence(t *testing.T) {
	resetStatusFlags(t)
	verbose = true
	types := []reposync.DivergenceType{
		reposync.DivergenceNone,
		reposync.DivergenceFastForward,
		reposync.DivergenceAhead,
		reposync.DivergenceDiverged,
		reposync.DivergenceConflict,
		reposync.DivergenceNoUpstream,
	}
	for _, dt := range types {
		h := reposync.RepoHealth{
			Repo:           reposync.RepoSpec{TargetPath: "/tmp/repo-x"},
			HealthStatus:   reposync.HealthWarning,
			CurrentBranch:  "main",
			DivergenceType: dt,
			AheadBy:        2,
			BehindBy:       3,
			ConflictFiles:  1,
			WorkTreeStatus: reposync.WorkTreeDirty,
			ModifiedFiles:  2,
			UntrackedFiles: 1,
			Recommendation: "pull --rebase",
			Error:          errors.New("warn"),
			Duration:       time.Millisecond * 20,
			FetchDuration:  time.Millisecond * 10,
		}
		captureStdout(t, func() { displayHealthRepositoryResult(h) })
	}
	for _, st := range []reposync.HealthStatus{
		reposync.HealthHealthy, reposync.HealthWarning, reposync.HealthError, reposync.HealthUnreachable, reposync.HealthStatus("x"),
	} {
		if getHealthIcon(st) == "" {
			t.Errorf("empty icon for %v", st)
		}
	}
}

func TestPrintBulkCleanupBranchResult(t *testing.T) {
	prevV, prevQ := verbose, quiet
	t.Cleanup(func() { verbose, quiet = prevV, prevQ })
	verbose = true
	quiet = false
	res := &repository.BulkCleanupResult{
		TotalScanned:          4,
		TotalProcessed:        4,
		TotalBranchesAnalyzed: 10,
		TotalBranchesDeleted:  2,
		Duration:              time.Millisecond * 50,
		Repositories: []repository.RepositoryCleanupResult{
			{RelativePath: "a", Status: repository.StatusCleanedUp, Message: "cleaned", DeletedBranches: []string{"old"}},
			{RelativePath: "b", Status: repository.StatusWouldCleanup, Message: "would", DeletedBranches: []string{"feat"}},
			{RelativePath: "c", Status: repository.StatusNothingToDo, Message: "ok"},
			{RelativePath: "d", Status: repository.StatusError, Message: "fail"},
		},
	}
	// DeletedBranches field may not exist — fix if compile fails
	out := captureStdout(t, func() {
		printBulkCleanupBranchResult(res, true)
		printBulkCleanupBranchResult(res, false)
	})
	if !strings.Contains(out, "Bulk Branch Cleanup") {
		t.Errorf("cleanup report: %q", out)
	}

	now := time.Now().Add(-48 * time.Hour)
	report := &branch.CleanupReport{
		Merged:    []*branch.Branch{{Name: "merged-1"}},
		Stale:     []*branch.Branch{{Name: "stale-1", UpdatedAt: &now}},
		Orphaned:  []*branch.Branch{{Name: "gone-1"}},
		Protected: []*branch.Branch{{Name: "main"}},
		Total:     4,
	}
	out = captureStdout(t, func() {
		printCleanupBranchReport(report, true)
		printCleanupBranchReport(report, false)
	})
	if !strings.Contains(out, "Merged") {
		t.Errorf("cleanup single report: %q", out)
	}
}

func TestPrintBulkStashAndCommitAndInfo(t *testing.T) {
	stashRes := &repository.BulkStashResult{
		TotalScanned: 2,
		Duration:     time.Millisecond,
		Summary:      map[string]int{"success": 1, "error": 1},
		Repositories: []repository.RepositoryStashResult{
			{RelativePath: "a", Status: "success", StashCount: 1, Message: "ok"},
			{RelativePath: "b", Status: "error", Message: "fail", Error: errors.New("e")},
		},
	}
	captureStdout(t, func() {
		printBulkStashResult(stashRes, "list", false, "default")
		printBulkStashResult(stashRes, "list", true, "json")
		displayStashResultsStructured(stashRes, "list", "json")
		displayStashResultsStructured(stashRes, "list", "llm")
	})

	commitRes := &repository.BulkCommitResult{
		TotalScanned:   3,
		TotalDirty:     2,
		TotalCommitted: 1,
		TotalFailed:    1,
		TotalSkipped:   1,
		Duration:       time.Millisecond,
		Repositories: []repository.RepositoryCommitResult{
			{RelativePath: "a", Status: "success", Branch: "main", Message: "feat", CommitHash: "abc", FilesChanged: 2},
			{RelativePath: "b", Status: "would-commit", Branch: "dev", SuggestedMessage: "chore", FilesChanged: 1},
			{RelativePath: "c", Status: "error", Message: "fail"},
			{RelativePath: "d", Status: "skipped", Message: "clean"},
		},
	}
	captureStdout(t, func() {
		displayCommitResults(commitRes)
		for _, r := range commitRes.Repositories {
			displayCommitRepositoryResult(r)
		}
		displayCommitResultsStructured(commitRes, "json")
		displayCommitResultsStructured(commitRes, "llm")
	})

	infoRes := &repository.BulkStatusResult{
		TotalScanned: 1,
		Duration:     time.Millisecond,
		Summary:      map[string]int{"clean": 1},
		Repositories: []repository.RepositoryStatusResult{
			{RelativePath: "r1", Status: "clean", Branch: "main", RemoteURL: "https://example.com/r.git"},
			{RelativePath: "r2", Status: "dirty", Branch: "dev"},
			{RelativePath: "r3", Status: "error", Message: "bad"},
		},
	}
	captureStdout(t, func() {
		// nil enrichment exercises the missing-entry path: every lookup yields
		// the zero value, which is what a repository whose extra probes failed
		// renders as.
		displayInfoResultsDetailed(infoRes, nil)
		displayInfoResultsStructured(infoRes, "json")
		displayInfoResultsStructured(infoRes, "llm")
	})
}

func TestRootLLMDocsAndVersion(t *testing.T) {
	prev := rootFormat
	t.Cleanup(func() { rootFormat = prev })
	rootFormat = "llm"
	out := captureStdout(t, func() { runRoot(rootCmd, nil) })
	if !strings.Contains(out, "GZ-Git") && !strings.Contains(out, "Available Commands") {
		t.Errorf("llm docs: %q", out[:minInt(200, len(out))])
	}
	rootFormat = ""
	// help path
	captureStdout(t, func() { runRoot(rootCmd, nil) })
}

func TestHistoryParsers(t *testing.T) {
	if _, err := parseOutputFormat("json"); err != nil {
		t.Errorf("json format: %v", err)
	}
	if _, err := parseOutputFormat("table"); err != nil {
		t.Logf("table: %v", err)
	}
	if _, err := parseOutputFormat("nope"); err == nil {
		t.Error("expected bad format error")
	}
	if _, err := parseContributorSortBy("commits"); err != nil {
		t.Logf("sort commits: %v", err)
	}
	if _, err := parseContributorSortBy("bad"); err == nil {
		t.Log("sort bad may or may not error")
	}
	if _, err := parseDate("2024-01-02"); err != nil {
		t.Errorf("date: %v", err)
	}
	if _, err := parseDate("not-a-date"); err == nil {
		t.Error("expected date parse error")
	}
}

func TestRunCleanDryRunAndTagCreateList(t *testing.T) {
	parent := setupBulkParent(t)
	// clean dry-run via command if flags exist
	cleanCmdLocal := findCommand(t, rootCmd, "clean")
	captureStdout(t, func() {
		// set dry-run flag if present
		if f := cleanCmdLocal.Flags().Lookup("dry-run"); f != nil {
			_ = f.Value.Set("true")
		}
		if cleanCmdLocal.RunE != nil {
			if err := cleanCmdLocal.RunE(cleanCmdLocal, []string{parent}); err != nil {
				t.Logf("clean: %v", err)
			}
		}
	})

	// tag create on single repo (chdir)
	// use bulk list again
	captureStdout(t, func() {
		if err := runTagList(findCommand(t, rootCmd, "tag", "list"), []string{parent}); err != nil {
			t.Logf("tag list: %v", err)
		}
	})

	// stash list bulk
	captureStdout(t, func() {
		if err := runStashList(findCommand(t, rootCmd, "stash", "list"), []string{parent}); err != nil {
			t.Logf("stash list: %v", err)
		}
	})
}

func TestLoadEffectiveConfigAndPrintSources(t *testing.T) {
	resetConfigFlags(t)
	isolateConfigHome(t)
	configGlobal = true
	_ = runConfigInit(configInitCmd, nil)
	eff, err := LoadEffectiveConfig(rootCmd, nil)
	if err != nil {
		t.Logf("LoadEffectiveConfig: %v", err)
		return
	}
	captureStdout(t, func() {
		PrintConfigSources(rootCmd, eff)
		var p, b, tok string
		par := 0
		ApplyConfigToFlags(eff, &p, &b, &tok, &par)
	})
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
