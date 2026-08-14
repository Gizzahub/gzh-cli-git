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
