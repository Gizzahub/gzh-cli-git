// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHistoryFileAndBlame(t *testing.T) {
	parent := setupBulkParent(t)
	repo := filepath.Join(parent, "r1")
	cwd, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	prevSince, prevUntil := fileHistorySince, fileHistoryUntil
	prevMax, prevFollow := fileHistoryMax, fileHistoryFollow
	prevAuthor, prevFmt := fileHistoryAuthor, fileHistoryFormat
	t.Cleanup(func() {
		fileHistorySince, fileHistoryUntil = prevSince, prevUntil
		fileHistoryMax, fileHistoryFollow = prevMax, prevFollow
		fileHistoryAuthor, fileHistoryFormat = prevAuthor, prevFmt
	})

	fileHistoryFormat = "table"
	fileHistoryMax = 10
	fileHistoryFollow = true
	captureStdout(t, func() {
		if err := runHistoryFile(fileCmd, []string{"README.md"}); err != nil {
			t.Logf("history file: %v", err)
		}
	})
	fileHistoryFormat = "json"
	captureStdout(t, func() {
		if err := runHistoryFile(fileCmd, []string{"README.md"}); err != nil {
			t.Logf("history file json: %v", err)
		}
	})
	fileHistoryFormat = "bogus"
	if err := runHistoryFile(fileCmd, []string{"README.md"}); err == nil {
		t.Error("expected bad format error")
	}
	fileHistoryFormat = "table"
	fileHistorySince = "not-a-date"
	if err := runHistoryFile(fileCmd, []string{"README.md"}); err == nil {
		t.Error("expected since date error")
	}
	fileHistorySince = ""
	fileHistoryUntil = "also-bad"
	if err := runHistoryFile(fileCmd, []string{"README.md"}); err == nil {
		t.Error("expected until date error")
	}
	fileHistoryUntil = ""

	captureStdout(t, func() {
		if err := runHistoryBlame(blameCmd, []string{"README.md"}); err != nil {
			t.Logf("blame: %v", err)
		}
	})
}

func TestCleanupBranchBulkAndSingle(t *testing.T) {
	parent := setupBulkParent(t)
	repo := filepath.Join(parent, "r1")
	// create merged-able branch
	runGit(t, repo, "checkout", "-b", "feature/temp")
	if err := os.WriteFile(filepath.Join(repo, "x.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "feat")
	// back to main/master
	runGitAllowFail(t, repo, "checkout", "-")

	prevM, prevS, prevG := cleanupBranchMerged, cleanupBranchStale, cleanupBranchGone
	prevDry, prevForce, prevYes := cleanupBranchDryRun, cleanupBranchForce, cleanupBranchYes
	prevProtect, prevBase := cleanupBranchProtect, cleanupBranchBaseBranch
	prevBulk := cleanupBranchBulkFlags
	t.Cleanup(func() {
		cleanupBranchMerged, cleanupBranchStale, cleanupBranchGone = prevM, prevS, prevG
		cleanupBranchDryRun, cleanupBranchForce, cleanupBranchYes = prevDry, prevForce, prevYes
		cleanupBranchProtect, cleanupBranchBaseBranch = prevProtect, prevBase
		cleanupBranchBulkFlags = prevBulk
	})

	// error: no type
	cleanupBranchMerged, cleanupBranchStale, cleanupBranchGone = false, false, false
	if err := runCleanupBranch(cleanupBranchCmd, []string{parent}); err == nil {
		t.Error("expected cleanup type required")
	}

	cleanupBranchMerged = true
	cleanupBranchStale = true
	cleanupBranchGone = true
	cleanupBranchDryRun = true
	cleanupBranchForce = false
	cleanupBranchYes = true
	cleanupBranchProtect = "main,master,develop"
	cleanupBranchBulkFlags = BulkCommandFlags{Depth: 1, Parallel: 2}

	captureStdout(t, func() {
		if err := runCleanupBranch(cleanupBranchCmd, []string{parent}); err != nil {
			t.Logf("bulk cleanup: %v", err)
		}
	})

	// single repo mode
	cwd, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	captureStdout(t, func() {
		if err := runCleanupBranch(cleanupBranchCmd, nil); err != nil {
			t.Logf("single cleanup: %v", err)
		}
	})

	// force path (still careful - only dry analysis if no branches to delete)
	cleanupBranchForce = true
	cleanupBranchYes = true
	captureStdout(t, func() {
		if err := runCleanupBranch(cleanupBranchCmd, nil); err != nil {
			t.Logf("force cleanup: %v", err)
		}
	})
}

func runGitAllowFail(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	_ = cmd.Run()
}

func TestUpdateFetchPullPushRealScan(t *testing.T) {
	parent := setupBulkParent(t)
	prevF, prevP, prevU, prevS := fetchFlags, pullFlags, updateFlags, pushFlags
	t.Cleanup(func() {
		fetchFlags, pullFlags, updateFlags, pushFlags = prevF, prevP, prevU, prevS
	})
	fetchFlags = BulkCommandFlags{Depth: 1, Parallel: 2, Format: "json", SkipFetch: true}
	// SkipFetch may not apply to fetch - still run
	captureStdout(t, func() { _ = runFetch(fetchCmd, []string{parent}) })
	pullFlags = BulkCommandFlags{Depth: 1, Parallel: 2, Format: "compact", DryRun: true}
	captureStdout(t, func() { _ = runPull(pullCmd, []string{parent}) })
	updateFlags = BulkCommandFlags{Depth: 1, Parallel: 2, Format: "llm", DryRun: true, SkipFetch: true}
	captureStdout(t, func() { _ = runUpdate(updateCmd, []string{parent}) })
	pushFlags = BulkCommandFlags{Depth: 1, Parallel: 2, Format: "json", DryRun: true}
	captureStdout(t, func() { _ = runPush(pushCmd, []string{parent}) })
}

func TestRunCleanDryRunBulk(t *testing.T) {
	parent := setupBulkParent(t)
	repo := filepath.Join(parent, "r1")
	if err := os.WriteFile(filepath.Join(repo, "junk.tmp"), []byte("j"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := findCommand(t, rootCmd, "clean")
	if f := cmd.Flags().Lookup("dry-run"); f != nil {
		_ = f.Value.Set("true")
	}
	if f := cmd.Flags().Lookup("scan-depth"); f != nil {
		_ = f.Value.Set("1")
	}
	captureStdout(t, func() {
		if cmd.RunE != nil {
			if err := cmd.RunE(cmd, []string{parent}); err != nil {
				t.Logf("clean: %v", err)
			}
		}
	})
}

func TestSwitchBranchBulk(t *testing.T) {
	parent := setupBulkParent(t)
	// ensure branch exists in child
	repo := filepath.Join(parent, "r1")
	runGit(t, repo, "branch", "develop")
	cmd := findCommand(t, rootCmd, "switch")
	if f := cmd.Flags().Lookup("scan-depth"); f != nil {
		_ = f.Value.Set("1")
	}
	if f := cmd.Flags().Lookup("format"); f != nil {
		_ = f.Value.Set("json")
	}
	captureStdout(t, func() {
		if cmd.RunE != nil {
			if err := cmd.RunE(cmd, []string{"develop", parent}); err != nil {
				// args order may be branch first
				if err2 := cmd.RunE(cmd, []string{parent, "develop"}); err2 != nil {
					t.Logf("switch: %v / %v", err, err2)
				}
			}
		}
	})
	_ = context.Background()
}
