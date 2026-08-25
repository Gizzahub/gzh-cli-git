// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package branch

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// Every test in this file drives a git command that fails, and asserts the
// failure is reported.
//
// gitcmd.Executor.Run signals a failed git through Result.ExitCode and returns a
// nil error unless the process could not be started, so `if _, err := Run(...);
// err != nil` accepts every git failure as a success. In this file that turned a
// failed `git branch` into a created branch, a failed `git checkout` into a moved
// HEAD, a refused `git branch -d` into a deletion, and a failed `git branch -vv`
// into a repository with no branches.

// gitCommit runs git in dir and fails the test if git does.
//
// The identity is supplied through the environment rather than repo config
// because tests in this package build repositories by hand — `git clone` into
// a fresh directory produces a repository that never passed through
// testutil's configureTempGit, so its local config carries no user. That
// commits at all on a developer machine only because a global identity is
// there to fall back on; on a runner there is none, and the commit fails with
// "empty ident name". Setting it here covers every git this package runs,
// including the ones inside hand-rolled clones, without reintroducing the
// dependency on whatever the developer has configured globally.
func gitCommit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...) //nolint:noctx // test helper
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// ensureBranch creates refs/heads/<name> at HEAD when missing. No-op when the
// branch already exists (including when it was the former default branch).
func ensureBranch(t *testing.T, dir, name string) {
	t.Helper()

	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+name) //nolint:noctx // test helper
	cmd.Dir = dir
	if cmd.Run() == nil {
		return
	}
	gitCommit(t, dir, "branch", name)
}

// headOf returns the branch name HEAD points at.
func headOf(t *testing.T, dir string) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD") //nolint:noctx // test helper
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}

	return string(out[:len(out)-1]) // drop trailing newline
}

func TestManager_CreateFailsOnNonRepository(t *testing.T) {
	mgr := NewManager()
	repo := &repository.Repository{Path: t.TempDir()}

	err := mgr.Create(context.Background(), repo, CreateOptions{Name: "feature/x"})
	if err == nil {
		t.Error("Create() on a non-repository returned nil, want failure")
	}
}

func TestManager_CreateRejectsUnknownStartRef(t *testing.T) {
	mgr := NewManager()
	repo := &repository.Repository{Path: testutil.TempGitRepoWithCommit(t)}

	err := mgr.Create(context.Background(), repo, CreateOptions{
		Name:     "feature/x",
		StartRef: "no-such-ref",
	})
	if !errors.Is(err, ErrInvalidRef) {
		t.Errorf("Create() with an unknown start ref = %v, want ErrInvalidRef", err)
	}
}

// TestManager_CreateWithCheckoutMovesHEAD is the positive half of the same
// property: when Create reports success, the checkout it claims to have done has
// actually happened.
func TestManager_CreateWithCheckoutMovesHEAD(t *testing.T) {
	repoPath := testutil.TempGitRepoWithCommit(t)
	mgr := NewManager()
	repo := &repository.Repository{Path: repoPath}

	before := headOf(t, repoPath)

	if err := mgr.Create(context.Background(), repo, CreateOptions{
		Name:     "feature/checked-out",
		Checkout: true,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if got := headOf(t, repoPath); got != "feature/checked-out" {
		t.Errorf("HEAD = %q (was %q), want feature/checked-out", got, before)
	}
}

// TestManager_DeleteReportsGitRefusal covers git's own safety net. Confirm skips
// this package's unmerged guard, which leaves `git branch -d` to refuse the
// deletion — a refusal that used to be reported as a completed one.
func TestManager_DeleteReportsGitRefusal(t *testing.T) {
	repoPath := testutil.TempGitRepoWithCommit(t)
	mgr := NewManager()
	repo := &repository.Repository{Path: repoPath}
	ctx := context.Background()

	// An unmerged branch: a commit that the default branch does not contain.
	gitCommit(t, repoPath, "checkout", "-b", "feature/unmerged")

	if err := os.WriteFile(filepath.Join(repoPath, "extra.txt"), []byte("x\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	gitCommit(t, repoPath, "add", "extra.txt")
	gitCommit(t, repoPath, "commit", "-m", "unmerged work")
	gitCommit(t, repoPath, "checkout", "-")

	err := mgr.Delete(ctx, repo, DeleteOptions{Name: "feature/unmerged", Confirm: true})
	if err == nil {
		t.Fatal("Delete() of an unmerged branch with -d returned nil, want git's refusal")
	}

	exists, existsErr := mgr.Exists(ctx, repo, "feature/unmerged")
	if existsErr != nil {
		t.Fatalf("Exists() error = %v", existsErr)
	}

	if !exists {
		t.Error("branch is gone after a refused delete")
	}
}

func TestManager_ListFailsOnNonRepository(t *testing.T) {
	mgr := NewManager()
	repo := &repository.Repository{Path: t.TempDir()}

	branches, err := mgr.List(context.Background(), repo, ListOptions{})
	if err == nil {
		t.Errorf("List() on a non-repository returned %d branches and a nil error, want failure", len(branches))
	}
}

// TestManager_CurrentFailsOnNonRepository is a guard, not a reproduction: before
// the fix a failed rev-parse produced an empty branch name, and Get rejected the
// empty name, so the caller got an error by accident. What changed is which error
// arrives, not whether one does.
func TestManager_CurrentFailsOnNonRepository(t *testing.T) {
	mgr := NewManager()
	repo := &repository.Repository{Path: t.TempDir()}

	b, err := mgr.Current(context.Background(), repo)
	if err == nil {
		t.Errorf("Current() on a non-repository returned %+v and a nil error, want failure", b)
	}
}

// TestManager_ExistsStillAnswersFalseForMissingBranch guards the one place that
// must keep reading the exit code as a boolean. Routing it through the shared
// helper would turn "no such branch" into an error and break every caller that
// asks the question.
func TestManager_ExistsStillAnswersFalseForMissingBranch(t *testing.T) {
	mgr := NewManager()
	repo := &repository.Repository{Path: testutil.TempGitRepoWithCommit(t)}

	exists, err := mgr.Exists(context.Background(), repo, "no-such-branch")
	if err != nil {
		t.Fatalf("Exists() error = %v, want nil", err)
	}

	if exists {
		t.Error("Exists() = true for a branch that was never created")
	}
}
