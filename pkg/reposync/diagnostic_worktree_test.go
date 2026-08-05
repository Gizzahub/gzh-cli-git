// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package reposync

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
	repo "github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// checkOneRepo runs the health check over a single local repository with the
// network paths disabled.
func checkOneRepo(t *testing.T, repoPath string) RepoHealth {
	t.Helper()

	executor := DiagnosticExecutor{Client: repo.NewClient()}

	report, err := executor.CheckHealth(context.Background(),
		[]RepoSpec{{Name: "fixture", TargetPath: repoPath}},
		DiagnosticOptions{
			SkipFetch:              true,
			Parallel:               1,
			CheckWorkTree:          true,
			IncludeRecommendations: true,
		})
	if err != nil {
		t.Fatalf("CheckHealth: %v", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("Results = %d, want 1", len(report.Results))
	}

	return report.Results[0]
}

func gitFixture(t *testing.T, repoPath string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...) //nolint:noctx // test helper, no context available
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}

	return string(out)
}

// TestCheckHealthSeesWorktreeSideRename is the regression test for a defect that
// neither package could have caught alone.
//
// A rename whose destination is intent-to-added (`mv a b && git add -N b`) is
// reported by git as " R", with the rename letter on the worktree side. The
// porcelain parser paired a rename's source record only when the letter sat on
// the index side, so the source path stayed in the stream and was re-read as a
// status line: its first two bytes became an XY code and GetStatus failed with
// `unknown index status code: h`. On its own that is a loud failure. It became a
// silent wrong answer here, because this caller treated any status error as an
// empty working tree — so `gz-git status` reported a repository holding
// uncommitted work as "healthy", recommending "No action needed", exit code 0.
//
// The parser fix and the fail-open removal each close half of that. This test
// covers the join, which is the only place the whole failure was visible.
func TestCheckHealthSeesWorktreeSideRename(t *testing.T) {
	repoPath := testutil.TempGitRepoWithCommit(t)

	if err := os.WriteFile(filepath.Join(repoPath, "handler.go"),
		[]byte("package main\n\nfunc Handle() {}\n"), 0o600); err != nil {
		t.Fatalf("write handler.go: %v", err)
	}
	gitFixture(t, repoPath, "add", "handler.go")
	gitFixture(t, repoPath, "commit", "-m", "add handler")

	if err := os.Rename(
		filepath.Join(repoPath, "handler.go"),
		filepath.Join(repoPath, "handler_v2.go"),
	); err != nil {
		t.Fatalf("rename: %v", err)
	}
	gitFixture(t, repoPath, "add", "-N", "handler_v2.go")

	// Assert the fixture's own premise, so a git that stopped producing this
	// shape fails here rather than passing while covering nothing.
	if raw := gitFixture(t, repoPath, "status", "--porcelain", "-z", "-uall"); !strings.Contains(
		raw, " R handler_v2.go\x00handler.go\x00",
	) {
		t.Fatalf("fixture yields no worktree-side rename; porcelain = %q", raw)
	}

	health := checkOneRepo(t, repoPath)

	if health.Error != nil {
		t.Fatalf("health.Error = %v, want nil", health.Error)
	}
	if health.WorkTreeStatus != WorkTreeDirty {
		t.Errorf("WorkTreeStatus = %q, want %q", health.WorkTreeStatus, WorkTreeDirty)
	}
	if health.ModifiedFiles != 1 {
		t.Errorf("ModifiedFiles = %d, want 1", health.ModifiedFiles)
	}
	if health.HealthStatus == HealthHealthy {
		t.Errorf("HealthStatus = %q for a repository with uncommitted work; recommendation was %q",
			health.HealthStatus, health.Recommendation)
	}
}

// TestCheckHealthDoesNotCallUnreadableTreeHealthy pins the fail-open removal.
//
// The old code set WorkTreeClean when GetStatus failed, which promoted "the
// status read failed" to "there is nothing to commit" — the strongest available
// claim, drawn from no evidence. A corrupt index makes `git status` exit 128
// while rev-parse still answers, so the health check reaches this branch with
// everything else intact.
func TestCheckHealthDoesNotCallUnreadableTreeHealthy(t *testing.T) {
	repoPath := testutil.TempGitRepoWithCommit(t)

	if err := os.WriteFile(filepath.Join(repoPath, ".git", "index"),
		[]byte("not an index"), 0o600); err != nil {
		t.Fatalf("corrupt index: %v", err)
	}

	health := checkOneRepo(t, repoPath)

	if health.WorkTreeStatus != WorkTreeUnknown {
		t.Errorf("WorkTreeStatus = %q, want %q", health.WorkTreeStatus, WorkTreeUnknown)
	}
	if health.HealthStatus != HealthError {
		t.Errorf("HealthStatus = %q, want %q", health.HealthStatus, HealthError)
	}
	if health.Error == nil {
		t.Error("health.Error = nil; the failure must survive into the result")
	}
	if strings.Contains(health.Recommendation, "No action needed") {
		t.Errorf("Recommendation = %q, want the unreadable-tree guidance", health.Recommendation)
	}
}
