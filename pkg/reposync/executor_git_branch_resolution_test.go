// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package reposync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	repo "github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

func TestGitExecutorResolvesConfiguredBranchBeforeReset(t *testing.T) {
	remote, target, developCommit, masterCommit := branchResolutionFixture(t)

	result, err := (GitExecutor{}).runCloneOrUpdate(t.Context(), repo.NewClient(), nopGitLogger{}, Action{
		Type:     ActionUpdate,
		Strategy: StrategyReset,
		Repo: RepoSpec{
			Name:       "fixture",
			CloneURL:   remote,
			TargetPath: target,
			Branch:     "develop,master",
		},
	}, RunOptions{}, NoopProgressSink{})
	if err != nil {
		t.Fatalf("runCloneOrUpdate() error = %v", err)
	}
	if result.Error != nil {
		t.Fatalf("result.Error = %v", result.Error)
	}
	if got := gitFixture(t, target, "rev-parse", "HEAD"); strings.TrimSpace(got) != developCommit {
		t.Errorf("HEAD after reset = %s, want develop %s (origin/HEAD is master %s)", strings.TrimSpace(got), developCommit, masterCommit)
	}
	if got := strings.TrimSpace(gitFixture(t, target, "branch", "--show-current")); got != "develop" {
		t.Errorf("current branch after reset = %q, want develop", got)
	}
}

func TestGitExecutorResetDiscardsWrongBranchChangesBeforeExactActivation(t *testing.T) {
	remote, target, developCommit, _ := branchResolutionFixture(t)
	gitOK(t, target, "checkout", "-b", "develop", "origin/develop")
	gitOK(t, target, "checkout", "master")
	masterBefore := strings.TrimSpace(gitFixture(t, target, "rev-parse", "master"))
	if err := os.WriteFile(filepath.Join(target, "branch.txt"), []byte("dirty master change\n"), 0o600); err != nil {
		t.Fatalf("write dirty master change: %v", err)
	}

	_, err := (GitExecutor{}).runCloneOrUpdate(t.Context(), repo.NewClient(), nopGitLogger{}, Action{
		Type: ActionUpdate, Strategy: StrategyReset,
		Repo: RepoSpec{Name: "fixture", CloneURL: remote, TargetPath: target, Branch: "develop"},
	}, RunOptions{}, NoopProgressSink{})
	if err != nil {
		t.Fatalf("runCloneOrUpdate() error = %v", err)
	}
	if got := strings.TrimPrefix(strings.TrimSpace(gitFixture(t, target, "symbolic-ref", "HEAD")), "refs/heads/"); got != "develop" {
		t.Errorf("symbolic branch = %q, want develop", got)
	}
	if got := strings.TrimSpace(gitFixture(t, target, "rev-parse", "HEAD")); got != developCommit {
		t.Errorf("HEAD = %s, want origin/develop %s", got, developCommit)
	}
	if got := strings.TrimSpace(gitFixture(t, target, "rev-parse", "master")); got != masterBefore {
		t.Errorf("master ref moved = %s, want unchanged %s", got, masterBefore)
	}
	if got := gitFixture(t, target, "status", "--porcelain"); got != "" {
		t.Errorf("working tree after reset = %q, want clean", got)
	}
}

func TestGitExecutorFailsBeforeResetWhenConfiguredBranchesAreMissing(t *testing.T) {
	remote, target, _, _ := branchResolutionFixture(t)
	before := strings.TrimSpace(gitFixture(t, target, "rev-parse", "HEAD"))

	_, err := (GitExecutor{}).runCloneOrUpdate(t.Context(), repo.NewClient(), nopGitLogger{}, Action{
		Type:     ActionUpdate,
		Strategy: StrategyReset,
		Repo: RepoSpec{
			Name:       "fixture",
			CloneURL:   remote,
			TargetPath: target,
			Branch:     "missing-one,missing-two",
		},
	}, RunOptions{}, NoopProgressSink{})
	if err == nil || !strings.Contains(err.Error(), "none of the configured branches exist") {
		t.Fatalf("runCloneOrUpdate() error = %v, want missing configured branch error", err)
	}
	if got := strings.TrimSpace(gitFixture(t, target, "rev-parse", "HEAD")); got != before {
		t.Errorf("HEAD moved after failed branch resolution: got %s, want %s", got, before)
	}
}

func TestGitExecutorResolvesConfiguredBranchBeforeClone(t *testing.T) {
	remote, _, developCommit, _ := branchResolutionFixture(t)
	target := filepath.Join(t.TempDir(), "clone-target")

	result, err := (GitExecutor{}).runCloneOrUpdate(t.Context(), repo.NewClient(), nopGitLogger{}, Action{
		Type: ActionClone,
		Repo: RepoSpec{
			Name:       "fixture",
			CloneURL:   remote,
			TargetPath: target,
			Branch:     "develop,master",
		},
	}, RunOptions{}, NoopProgressSink{})
	if err != nil {
		t.Fatalf("runCloneOrUpdate() error = %v", err)
	}
	if result.Error != nil {
		t.Fatalf("result.Error = %v", result.Error)
	}
	if got := strings.TrimSpace(gitFixture(t, target, "rev-parse", "HEAD")); got != developCommit {
		t.Errorf("HEAD after clone = %s, want develop %s", got, developCommit)
	}
	if got := strings.TrimSpace(gitFixture(t, target, "branch", "--show-current")); got != "develop" {
		t.Errorf("current branch after clone = %q, want develop", got)
	}
}

func TestResolveConfiguredBranch_HEADStopsOrderedFallback(t *testing.T) {
	remote, _, _, _ := branchResolutionFixture(t)

	branch, err := resolveConfiguredBranch(t.Context(), "", remote, nil, "HEAD,develop", false, nopGitLogger{})
	if err != nil {
		t.Fatalf("resolveConfiguredBranch() error = %v", err)
	}
	if branch != "" {
		t.Errorf("resolved branch = %q, want origin/HEAD behavior", branch)
	}

	branch, err = resolveConfiguredBranch(t.Context(), "", remote, nil, "missing,HEAD,develop", false, nopGitLogger{})
	if err != nil {
		t.Fatalf("resolveConfiguredBranch() error = %v", err)
	}
	if branch != "" {
		t.Errorf("resolved branch after missing HEAD fallback = %q, want origin/HEAD behavior", branch)
	}
}

func TestGitExecutorCreatesSymbolicBranchWhenTagSharesName(t *testing.T) {
	remote, target, developCommit, _ := branchResolutionFixture(t)
	gitOK(t, target, "tag", "develop")

	_, err := (GitExecutor{}).runCloneOrUpdate(t.Context(), repo.NewClient(), nopGitLogger{}, Action{
		Type: ActionUpdate, Strategy: StrategyReset,
		Repo: RepoSpec{Name: "fixture", CloneURL: remote, TargetPath: target, Branch: "develop"},
	}, RunOptions{}, NoopProgressSink{})
	if err != nil {
		t.Fatalf("runCloneOrUpdate() error = %v", err)
	}
	if got := strings.TrimPrefix(strings.TrimSpace(gitFixture(t, target, "symbolic-ref", "HEAD")), "refs/heads/"); got != "develop" {
		t.Errorf("symbolic branch = %q, want develop", got)
	}
	if got := strings.TrimSpace(gitFixture(t, target, "rev-parse", "HEAD")); got != developCommit {
		t.Errorf("HEAD = %s, want develop %s", got, developCommit)
	}
}

func TestGitExecutorUpdatesResolvedStaleLocalBranch(t *testing.T) {
	remote, target, _, _ := branchResolutionFixture(t)
	gitOK(t, target, "checkout", "-b", "develop", "origin/develop")
	gitOK(t, target, "checkout", "master")

	source := filepath.Join(filepath.Dir(remote), "source")
	if err := os.WriteFile(filepath.Join(source, "branch.txt"), []byte("develop updated\n"), 0o600); err != nil {
		t.Fatalf("write remote update: %v", err)
	}
	gitOK(t, source, "commit", "-am", "develop updated")
	gitOK(t, source, "push", "origin", "develop")
	want := strings.TrimSpace(gitFixture(t, source, "rev-parse", "HEAD"))

	_, err := (GitExecutor{}).runCloneOrUpdate(t.Context(), repo.NewClient(), nopGitLogger{}, Action{
		Type: ActionUpdate, Strategy: StrategyReset,
		Repo: RepoSpec{Name: "fixture", CloneURL: remote, TargetPath: target, Branch: "develop,master"},
	}, RunOptions{}, NoopProgressSink{})
	if err != nil {
		t.Fatalf("runCloneOrUpdate() error = %v", err)
	}
	if got := strings.TrimSpace(gitFixture(t, target, "branch", "--show-current")); got != "develop" {
		t.Errorf("current branch = %q, want develop", got)
	}
	if got := strings.TrimSpace(gitFixture(t, target, "rev-parse", "develop")); got != want {
		t.Errorf("develop ref = %s, want remote tip %s", got, want)
	}
}

func TestGitExecutorFailsBeforeResetWhenResolvedBranchIsInLinkedWorktree(t *testing.T) {
	remote, target, _, _ := branchResolutionFixture(t)
	before := strings.TrimSpace(gitFixture(t, target, "rev-parse", "HEAD"))
	linked := filepath.Join(t.TempDir(), "linked")
	gitOK(t, target, "worktree", "add", "-b", "develop", linked, "origin/develop")

	_, err := (GitExecutor{}).runCloneOrUpdate(t.Context(), repo.NewClient(), nopGitLogger{}, Action{
		Type: ActionUpdate, Strategy: StrategyReset,
		Repo: RepoSpec{Name: "fixture", CloneURL: remote, TargetPath: target, Branch: "develop,master"},
	}, RunOptions{}, NoopProgressSink{})
	if err == nil || !strings.Contains(err.Error(), "checkout configured branch") {
		t.Fatalf("runCloneOrUpdate() error = %v, want pre-update checkout error", err)
	}
	if got := strings.TrimSpace(gitFixture(t, target, "rev-parse", "HEAD")); got != before {
		t.Errorf("HEAD moved after linked-worktree checkout failure: got %s, want %s", got, before)
	}
}

func TestGitExecutorCloneStrategyProbesConfiguredCloneURL(t *testing.T) {
	remote, _, developCommit, _ := branchResolutionFixture(t)
	root := t.TempDir()
	wrongRemote := filepath.Join(root, "wrong.git")
	target := filepath.Join(root, "target")
	gitOK(t, root, "init", "--bare", wrongRemote)
	gitOK(t, root, "clone", wrongRemote, target)

	_, err := (GitExecutor{}).runCloneOrUpdate(t.Context(), repo.NewClient(), nopGitLogger{}, Action{
		Type: ActionUpdate, Strategy: StrategyClone,
		Repo: RepoSpec{Name: "fixture", CloneURL: remote, TargetPath: target, Branch: "develop,master"},
	}, RunOptions{}, NoopProgressSink{})
	if err != nil {
		t.Fatalf("runCloneOrUpdate() error = %v", err)
	}
	if got := strings.TrimSpace(gitFixture(t, target, "rev-parse", "HEAD")); got != developCommit {
		t.Errorf("HEAD after reclone = %s, want configured remote develop %s", got, developCommit)
	}
}

func TestGitExecutorAcceptsForcedRemoteBranchUpdate(t *testing.T) {
	remote, target, _, _ := branchResolutionFixture(t)
	gitOK(t, target, "checkout", "-b", "develop", "origin/develop")
	gitOK(t, target, "checkout", "master")

	source := filepath.Join(filepath.Dir(remote), "source")
	gitOK(t, source, "reset", "--hard", "master")
	if err := os.WriteFile(filepath.Join(source, "forced.txt"), []byte("forced\n"), 0o600); err != nil {
		t.Fatalf("write forced update: %v", err)
	}
	gitOK(t, source, "add", "forced.txt")
	gitOK(t, source, "commit", "-m", "forced develop")
	gitOK(t, source, "push", "--force", "origin", "HEAD:develop")
	want := strings.TrimSpace(gitFixture(t, source, "rev-parse", "HEAD"))

	_, err := (GitExecutor{}).runCloneOrUpdate(t.Context(), repo.NewClient(), nopGitLogger{}, Action{
		Type: ActionUpdate, Strategy: StrategyReset,
		Repo: RepoSpec{Name: "fixture", CloneURL: remote, TargetPath: target, Branch: "develop"},
	}, RunOptions{}, NoopProgressSink{})
	if err != nil {
		t.Fatalf("runCloneOrUpdate() error = %v", err)
	}
	if got := strings.TrimSpace(gitFixture(t, target, "rev-parse", "develop")); got != want {
		t.Errorf("develop ref after forced update = %s, want %s", got, want)
	}
}

func TestGitExecutorFailsLocallyForNonGitUpdate(t *testing.T) {
	target := filepath.Join(t.TempDir(), "not-a-repo")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	_, err := (GitExecutor{}).runCloneOrUpdate(t.Context(), repo.NewClient(), nopGitLogger{}, Action{
		Type: ActionUpdate, Strategy: StrategyReset,
		Repo: RepoSpec{Name: "fixture", CloneURL: filepath.Join(t.TempDir(), "unreachable.git"), TargetPath: target, Branch: "develop"},
	}, RunOptions{MaxRetries: 1}, NoopProgressSink{})
	if err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("runCloneOrUpdate() error = %v, want local non-git error", err)
	}
}

func TestGitExecutorSkipsDirtyPullBeforeRemoteProbe(t *testing.T) {
	_, target, _, _ := branchResolutionFixture(t)
	before := strings.TrimSpace(gitFixture(t, target, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(target, "dirty.txt"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}

	result, err := (GitExecutor{}).runCloneOrUpdate(t.Context(), repo.NewClient(), nopGitLogger{}, Action{
		Type: ActionUpdate, Strategy: StrategyPull,
		Repo: RepoSpec{Name: "fixture", CloneURL: filepath.Join(t.TempDir(), "unreachable.git"), TargetPath: target, Branch: "develop"},
	}, RunOptions{}, NoopProgressSink{})
	if err != nil {
		t.Fatalf("runCloneOrUpdate() error = %v, want dirty skip", err)
	}
	if !strings.Contains(result.Message, "working tree is dirty") {
		t.Errorf("result message = %q, want dirty report", result.Message)
	}
	if got := strings.TrimSpace(gitFixture(t, target, "rev-parse", "HEAD")); got != before {
		t.Errorf("HEAD moved during dirty pull: got %s, want %s", got, before)
	}
}

func TestResolveConfiguredBranchRetriesRemoteProbe(t *testing.T) {
	missingRemote := filepath.Join(t.TempDir(), "missing.git")
	sink := &branchResolutionSink{}
	_, err := resolveConfiguredBranchWithRetries(t.Context(), "", missingRemote, nil, "develop", false, nopGitLogger{}, 2, sink, Action{})
	if err == nil {
		t.Fatal("resolveConfiguredBranchWithRetries() error = nil, want remote probe failure")
	}
	if sink.progressCalls != 1 {
		t.Errorf("resolution retry progress calls = %d, want 1", sink.progressCalls)
	}
}

func TestResolveConfiguredBranchDoesNotRetryMissingCandidates(t *testing.T) {
	remote, _, _, _ := branchResolutionFixture(t)
	sink := &branchResolutionSink{}
	_, err := resolveConfiguredBranchWithRetries(t.Context(), "", remote, nil, "missing", false, nopGitLogger{}, 2, sink, Action{})
	if err == nil {
		t.Fatal("resolveConfiguredBranchWithRetries() error = nil, want missing branch error")
	}
	if sink.progressCalls != 0 {
		t.Errorf("resolution retries for missing branch = %d, want 0", sink.progressCalls)
	}
}

func TestBranchUsedByAnotherWorktreeRecognizesSymlinkAlias(t *testing.T) {
	_, target, _, _ := branchResolutionFixture(t)
	alias := filepath.Join(t.TempDir(), "target-alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	used, err := branchUsedByAnotherWorktree(t.Context(), alias, "master")
	if err != nil {
		t.Fatalf("branchUsedByAnotherWorktree() error = %v", err)
	}
	if used {
		t.Error("same worktree through symlink alias reported as another worktree")
	}
}

type branchResolutionSink struct{ progressCalls int }

func (*branchResolutionSink) OnStart(Action) {}

func (s *branchResolutionSink) OnProgress(Action, string, float64) { s.progressCalls++ }

func (*branchResolutionSink) OnComplete(ActionResult) {}

// branchResolutionFixture creates a bare remote whose HEAD is master, while
// develop has a distinct commit. The target starts on master so a reset to
// origin/HEAD is observably different from a reset to origin/develop.
func branchResolutionFixture(t *testing.T) (remote, target, developCommit, masterCommit string) {
	t.Helper()

	root := t.TempDir()
	remote = filepath.Join(root, "remote.git")
	source := filepath.Join(root, "source")
	target = filepath.Join(root, "target")

	gitOK(t, root, "init", "--bare", remote)
	gitOK(t, root, "init", "--initial-branch=master", source)
	gitOK(t, source, "config", "user.email", "fixture@example.invalid")
	gitOK(t, source, "config", "user.name", "Fixture")
	if err := os.WriteFile(filepath.Join(source, "branch.txt"), []byte("master\n"), 0o600); err != nil {
		t.Fatalf("write master fixture: %v", err)
	}
	gitOK(t, source, "add", "branch.txt")
	gitOK(t, source, "commit", "-m", "master")
	masterCommit = strings.TrimSpace(gitFixture(t, source, "rev-parse", "HEAD"))
	gitOK(t, source, "remote", "add", "origin", remote)
	gitOK(t, source, "push", "origin", "master")

	gitOK(t, source, "checkout", "-b", "develop")
	if err := os.WriteFile(filepath.Join(source, "branch.txt"), []byte("develop\n"), 0o600); err != nil {
		t.Fatalf("write develop fixture: %v", err)
	}
	gitOK(t, source, "commit", "-am", "develop")
	developCommit = strings.TrimSpace(gitFixture(t, source, "rev-parse", "HEAD"))
	gitOK(t, source, "push", "origin", "develop")
	gitOK(t, root, "--git-dir", remote, "symbolic-ref", "HEAD", "refs/heads/master")

	gitOK(t, root, "clone", remote, target)
	return remote, target, developCommit, masterCommit
}
