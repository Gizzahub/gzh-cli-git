package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupRemote gives handoff tests a real push target. A remote is important
// here: an untracked-only repository without one is correctly blocked for a
// different reason (there is nowhere for the checkpoint to go).
func setupRemote(t *testing.T, repo *E2ERepo) {
	t.Helper()

	remoteRoot := t.TempDir()
	remotePath := filepath.Join(remoteRoot, "origin.git")
	repo.runCommand(remoteRoot, "git", "init", "--bare", remotePath)
	repo.Git("remote", "add", "origin", remotePath)
	repo.Git("push", "--set-upstream", "origin", repo.GetCurrentBranch())
}

func seedHandoffRepo(t *testing.T) *E2ERepo {
	t.Helper()

	repo := NewE2ERepo(t)
	repo.WriteFile("README.md", "# Handoff E2E\n")
	repo.Git("add", "README.md")
	repo.Git("commit", "-m", "Initial commit")
	setupRemote(t, repo)
	return repo
}

func remoteHead(t *testing.T, repo *E2ERepo) string {
	t.Helper()
	return remoteHeadForBranch(t, repo, repo.GetCurrentBranch())
}

func remoteHeadForBranch(t *testing.T, repo *E2ERepo, branch string) string {
	t.Helper()

	remote := strings.TrimSpace(repo.Git("config", "--get", "remote.origin.url"))
	if remote == "" {
		t.Fatal("repository has no origin remote")
	}
	ref := "refs/heads/" + branch
	return strings.TrimSpace(repo.runCommand(repo.repoDir, "git", "--git-dir", remote, "rev-parse", ref))
}

// TestHandoffUntrackedOnlyRealCLI verifies the complete departure path for a
// repository whose only local work is an untracked source file: check reports
// work outstanding, end commits and pushes it, and a second check is ready.
func TestHandoffUntrackedOnlyRealCLI(t *testing.T) {
	repo := seedHandoffRepo(t)
	repo.WriteFile("notes.txt", "work that must survive the handoff\n")

	checkOutput := repo.RunGzhGitExpectError("handoff", "check")
	AssertContains(t, checkOutput, "NOT YET")
	AssertContains(t, checkOutput, "1 untracked file(s) exist only here")

	endOutput := repo.RunGzhGit("handoff", "end", "--no-trailers")
	AssertContains(t, endOutput, "committed")
	AssertContains(t, endOutput, "pushed")
	if !repo.CommitExists("chore(wip): handoff checkpoint") {
		t.Fatal("handoff end did not create the checkpoint commit")
	}
	if got := strings.TrimSpace(repo.Git("status", "--porcelain")); got != "" {
		t.Fatalf("repository remains dirty after handoff end: %q", got)
	}

	readyOutput := repo.RunGzhGit("handoff", "check")
	AssertContains(t, readyOutput, "SAFE TO LEAVE")

	// Verify the checkpoint reached the actual bare remote, rather than only
	// making the local assessment look clean.
	remote := repo.Git("config", "--get", "remote.origin.url")
	remote = strings.TrimSpace(remote)
	remoteRef := "refs/heads/" + repo.GetCurrentBranch()
	remoteHead := repo.runCommand(repo.repoDir, "git", "--git-dir", remote, "rev-parse", remoteRef)
	localHead := repo.Git("rev-parse", "HEAD")
	if strings.TrimSpace(remoteHead) != strings.TrimSpace(localHead) {
		t.Fatalf("remote head %q does not match local head %q", strings.TrimSpace(remoteHead), strings.TrimSpace(localHead))
	}
}

// TestHandoffEndHoldsUntrackedArtifact verifies the real CLI's safety screen.
// The artifact stays uncommitted and the command remains non-ready until a
// person reviews it (or explicitly uses --force).
func TestHandoffEndHoldsUntrackedArtifact(t *testing.T) {
	repo := seedHandoffRepo(t)
	repo.WriteFile("node_modules/example/index.js", "generated dependency output\n")

	output := repo.RunGzhGitExpectError("handoff", "end", "--no-trailers")
	AssertContains(t, output, "held back — nothing was committed")
	AssertContains(t, output, "node_modules/example/index.js")
	AssertContains(t, output, "artifact")

	if repo.CommitExists("chore(wip): handoff checkpoint") {
		t.Fatal("handoff end committed an artifact that the guard should hold")
	}
	if _, err := os.Stat(filepath.Join(repo.repoDir, "node_modules", "example", "index.js")); err != nil {
		t.Fatalf("artifact disappeared after handoff end: %v", err)
	}
}

// TestHandoffEndNoPushLeavesRemoteBehind verifies that --no-push is an
// explicit offline checkpoint: the local commit is made, but the remote stays
// at its previous ref and the handoff remains not ready.
func TestHandoffEndNoPushLeavesRemoteBehind(t *testing.T) {
	repo := seedHandoffRepo(t)
	repo.WriteFile("notes.txt", "offline checkpoint\n")
	before := remoteHead(t, repo)

	output, err := repo.RunGzhGitResult("handoff", "end", "--no-push", "--no-trailers")
	AssertExitCode(t, err, 1)
	if err == nil {
		t.Fatal("handoff end --no-push unexpectedly reported success")
	}
	AssertContains(t, output, "committed")
	AssertNotContains(t, output, " pushed")
	if !repo.CommitExists("chore(wip): handoff checkpoint") {
		t.Fatal("handoff end --no-push did not create the checkpoint commit")
	}
	if got := remoteHead(t, repo); got != before {
		t.Fatalf("--no-push changed remote head from %q to %q", before, got)
	}

	checkOutput, checkErr := repo.RunGzhGitResult("handoff", "check")
	AssertExitCode(t, checkErr, 1)
	if checkErr == nil {
		t.Fatal("handoff check unexpectedly reported success after a no-push checkpoint")
	}
	AssertContains(t, checkOutput, "NOT YET")
	AssertContains(t, checkOutput, "1 commit(s) are not on the remote")
}

// TestHandoffEndHoldsCredentialContent verifies that a generic filename is
// still held when its contents look like a credential.
func TestHandoffEndHoldsCredentialContent(t *testing.T) {
	repo := seedHandoffRepo(t)
	content := "token=ghp_" + strings.Repeat("A", 36) + "\n"
	repo.WriteFile("local-settings.txt", content)
	before := remoteHead(t, repo)

	output, err := repo.RunGzhGitResult("handoff", "end", "--no-trailers")
	AssertExitCode(t, err, 1)
	if err == nil {
		t.Fatal("handoff end unexpectedly committed credential-looking content")
	}
	AssertContains(t, output, "held back — nothing was committed")
	AssertContains(t, output, "local-settings.txt")
	AssertContains(t, output, "contains a GitHub token")
	if got := repo.ReadFile("local-settings.txt"); got != content {
		t.Fatalf("credential-looking file changed after guard: %q", got)
	}
	if repo.CommitExists("chore(wip): handoff checkpoint") {
		t.Fatal("handoff end committed credential-looking content")
	}
	if got := remoteHead(t, repo); got != before {
		t.Fatalf("credential guard changed remote head from %q to %q", before, got)
	}
}

// TestHandoffEndNoRemotePreservesWork verifies that a repository with nowhere
// to push is a hard blocker, not an opportunity to create a stranded commit.
func TestHandoffEndNoRemotePreservesWork(t *testing.T) {
	repo := NewE2ERepo(t)
	repo.WriteFile("README.md", "# Handoff E2E\n")
	repo.Git("add", "README.md")
	repo.Git("commit", "-m", "Initial commit")
	repo.WriteFile("notes.txt", "must stay in the working tree\n")
	before := strings.TrimSpace(repo.Git("rev-parse", "HEAD"))

	output, err := repo.RunGzhGitResult("handoff", "end", "--no-trailers")
	AssertExitCode(t, err, 1)
	if err == nil {
		t.Fatal("handoff end unexpectedly succeeded without a remote")
	}
	AssertContains(t, output, "no remote configured")
	if got := strings.TrimSpace(repo.Git("rev-parse", "HEAD")); got != before {
		t.Fatalf("no-remote blocker changed HEAD from %q to %q", before, got)
	}
	if repo.CommitExists("chore(wip): handoff checkpoint") {
		t.Fatal("no-remote blocker still created a checkpoint commit")
	}
	if got := repo.ReadFile("notes.txt"); got != "must stay in the working tree\n" {
		t.Fatalf("no-remote blocker changed work file: %q", got)
	}
}

// TestHandoffEndDetachedPreservesWork verifies that a detached HEAD is a hard
// blocker even when a remote exists.
func TestHandoffEndDetachedPreservesWork(t *testing.T) {
	repo := seedHandoffRepo(t)
	branch := repo.GetCurrentBranch()
	remoteBefore := remoteHeadForBranch(t, repo, branch)
	repo.Git("checkout", "--detach", "HEAD")
	repo.WriteFile("notes.txt", "detached work\n")
	before := strings.TrimSpace(repo.Git("rev-parse", "HEAD"))

	output, err := repo.RunGzhGitResult("handoff", "end", "--no-trailers")
	AssertExitCode(t, err, 1)
	if err == nil {
		t.Fatal("handoff end unexpectedly succeeded on detached HEAD")
	}
	AssertContains(t, output, "HEAD is detached")
	if got := strings.TrimSpace(repo.Git("rev-parse", "HEAD")); got != before {
		t.Fatalf("detached blocker changed HEAD from %q to %q", before, got)
	}
	if repo.CommitExists("chore(wip): handoff checkpoint") {
		t.Fatal("detached blocker still created a checkpoint commit")
	}
	if got := repo.ReadFile("notes.txt"); got != "detached work\n" {
		t.Fatalf("detached blocker changed work file: %q", got)
	}
	if got := remoteHeadForBranch(t, repo, branch); got != remoteBefore {
		t.Fatalf("detached blocker changed remote head from %q to %q", remoteBefore, got)
	}
}

// TestHandoffStartUpdatesCleanAndSkipsDirty verifies arrival behavior across
// two independent repositories: a clean repository rebases onto its incoming
// remote commit while a dirty repository is fetched but left untouched.
func TestHandoffStartUpdatesCleanAndSkipsDirty(t *testing.T) {
	workspace := t.TempDir()
	clean := newE2ERepoAt(t, filepath.Join(workspace, "clean"))
	dirty := newE2ERepoAt(t, filepath.Join(workspace, "dirty"))
	for _, repo := range []*E2ERepo{clean, dirty} {
		repo.WriteFile("README.md", "# Handoff E2E\n")
		repo.Git("add", "README.md")
		repo.Git("commit", "-m", "Initial commit")
		setupRemote(t, repo)
	}

	remote := strings.TrimSpace(clean.Git("config", "--get", "remote.origin.url"))
	incoming := filepath.Join(workspace, "incoming")
	clean.runCommand(workspace, "git", "clone", remote, incoming)
	clean.runCommand(incoming, "git", "config", "user.name", "Incoming User")
	clean.runCommand(incoming, "git", "config", "user.email", "incoming@example.com")
	clean.WriteFileAt(incoming, "remote.txt", "arrived while away\n")
	clean.runCommand(incoming, "git", "add", "remote.txt")
	clean.runCommand(incoming, "git", "commit", "-m", "Remote work")
	clean.runCommand(incoming, "git", "push")

	dirty.WriteFile("local.txt", "must not be rebased over\n")
	dirtyBefore := strings.TrimSpace(dirty.Git("rev-parse", "HEAD"))
	dirtyRemoteBefore := remoteHead(t, dirty)

	output, err := clean.RunGzhGitAtResult(workspace, "handoff", "start", "-d", "1")
	AssertExitCode(t, err, 1)
	if err == nil {
		t.Fatal("handoff start unexpectedly reported success with a dirty repository")
	}
	AssertContains(t, output, "clean")
	AssertContains(t, output, "updated")
	AssertContains(t, output, "dirty")
	AssertContains(t, output, "not rebased")
	AssertContains(t, output, "ATTENTION")
	if got := clean.ReadFile("remote.txt"); got != "arrived while away\n" {
		t.Fatalf("clean repository did not receive remote work: %q", got)
	}
	if got := strings.TrimSpace(dirty.Git("rev-parse", "HEAD")); got != dirtyBefore {
		t.Fatalf("dirty repository HEAD changed from %q to %q", dirtyBefore, got)
	}
	if got := remoteHead(t, clean); got != strings.TrimSpace(clean.Git("rev-parse", "refs/remotes/origin/"+clean.GetCurrentBranch())) {
		t.Fatalf("clean repository remote-tracking ref did not settle at remote head: %q", got)
	}
	if got := remoteHead(t, dirty); got != dirtyRemoteBefore {
		t.Fatalf("dirty repository remote head changed from %q to %q", dirtyRemoteBefore, got)
	}
	if got := dirty.ReadFile("local.txt"); got != "must not be rebased over\n" {
		t.Fatalf("dirty repository work changed: %q", got)
	}
}
