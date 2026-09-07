// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package branch

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// staleFixture builds a checkout whose remote-tracking refs are a snapshot that
// the remote has since contradicted.
//
// origin/master holds commit A. origin/develop held C, which contains A — so
// the cached snapshot says master is a lossless duplicate of the canonical
// branch. Then develop is rewound on the remote to a history that never had A.
// The checkout is deliberately not fetched afterwards: that gap is the defect.
//
// It returns the work tree, the bare remote, and the tip develop actually has
// on the remote now.
func staleFixture(t *testing.T) (workDir, originDir, liveDevelopSHA string) {
	t.Helper()

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "work")

	gitCommit(t, root, "init", "-q", "--bare", origin)
	gitCommit(t, root, "init", "-q", "-b", "master", work)
	gitCommit(t, work, "config", "user.email", "t@t")
	gitCommit(t, work, "config", "user.name", "t")
	gitCommit(t, work, "remote", "add", "origin", origin)

	// A: the commit that will exist only on master.
	gitCommit(t, work, "commit", "-q", "--allow-empty", "-m", "A")
	gitCommit(t, work, "push", "-q", "origin", "master")

	gitCommit(t, work, "checkout", "-q", "-b", "develop")
	gitCommit(t, work, "commit", "-q", "--allow-empty", "-m", "C")
	gitCommit(t, work, "push", "-q", "origin", "develop")
	gitCommit(t, work, "fetch", "-q", "origin")

	// Somebody rewrote develop on the remote, onto a root that never had A.
	//
	// The rewind is applied to the bare repository directly rather than pushed
	// from here, because a push updates the pusher's own remote-tracking ref —
	// which would repair the very staleness this fixture exists to create. Only
	// the object has to arrive by push; the ref move does not.
	gitCommit(t, work, "checkout", "-q", "--orphan", "rewrite")
	gitCommit(t, work, "commit", "-q", "--allow-empty", "-m", "rewritten root")
	gitCommit(t, work, "push", "-q", "origin", "rewrite:refs/heads/rewrite")
	live := gitOutput(t, work, "rev-parse", "rewrite")
	gitCommit(t, work, "checkout", "-q", "develop")
	gitCommit(t, work, "branch", "-D", "-q", "rewrite")

	// Repoint origin's HEAD first, or the remote refuses to delete master on its
	// own and these tests pass whether or not the freshness gate exists.
	gitCommit(t, origin, "symbolic-ref", "HEAD", "refs/heads/develop")
	gitCommit(t, origin, "update-ref", "refs/heads/develop", live)
	gitCommit(t, origin, "update-ref", "-d", "refs/heads/rewrite")

	// No fetch. The cache still says A is contained in the canonical branch.
	return work, origin, live
}

func TestCleanupService_RequireCurrentCanonicalTipRefusesRewoundRemote(t *testing.T) {
	work, _, live := staleFixture(t)
	repo := &repository.Repository{Path: work}
	svc := newTestCleanupService(t)
	ctx := context.Background()

	// The stale cache genuinely says the ancestry holds — that is why the
	// deletion would otherwise be authorized.
	opts := ExecuteOptions{CanonicalBranch: "develop", CanonicalRemote: "origin"}
	candidate := &Branch{Name: "master", IsRemote: true, Ref: "refs/remotes/origin/master"}
	if !svc.authorizeRetire(ctx, repo, candidate, opts) {
		t.Fatal("fixture is wrong: the cached ancestry must still pass, or the test proves nothing")
	}

	err := svc.requireCurrentCanonicalTip(ctx, repo, candidate, opts)
	if err == nil {
		t.Fatal("develop was rewound on the remote; the evidence for this deletion is void and it must be refused")
	}
	if !strings.Contains(err.Error(), live[:8]) {
		t.Errorf("the refusal must name where the remote actually is, so the operator can act on it; got: %v", err)
	}
	if !strings.Contains(err.Error(), "git fetch") {
		t.Errorf("the refusal must say how to recover; got: %v", err)
	}
}

func TestCleanupService_RequireCurrentCanonicalTipAllowsUnchangedRemote(t *testing.T) {
	work, _, _ := staleFixture(t)
	repo := &repository.Repository{Path: work}
	svc := newTestCleanupService(t)
	ctx := context.Background()

	// Bring the cache back in line with the remote. The ancestry no longer
	// holds, but this gate is not the ancestry check — it only asks whether the
	// tip is current, and it must say yes.
	gitCommit(t, work, "fetch", "-q", "origin")

	opts := ExecuteOptions{CanonicalBranch: "develop", CanonicalRemote: "origin"}
	candidate := &Branch{Name: "master", IsRemote: true, Ref: "refs/remotes/origin/master"}

	if err := svc.requireCurrentCanonicalTip(ctx, repo, candidate, opts); err != nil {
		t.Errorf("the cached tip matches the remote; the gate must pass: %v", err)
	}
}

// A remote candidate whose canonical branch has vanished entirely is refused
// with its own wording: there is no branch left to have been an ancestor of.
func TestCleanupService_RequireCurrentCanonicalTipRefusesMissingCanonical(t *testing.T) {
	work, origin, _ := staleFixture(t)
	repo := &repository.Repository{Path: work}
	svc := newTestCleanupService(t)
	ctx := context.Background()

	gitCommit(t, origin, "update-ref", "-d", "refs/heads/develop")

	opts := ExecuteOptions{CanonicalBranch: "develop", CanonicalRemote: "origin"}
	candidate := &Branch{Name: "master", IsRemote: true, Ref: "refs/remotes/origin/master"}

	err := svc.requireCurrentCanonicalTip(ctx, repo, candidate, opts)
	if err == nil {
		t.Fatal("the canonical branch is gone from the remote; the deletion must be refused")
	}
	if !strings.Contains(err.Error(), "no longer exists") {
		t.Errorf("expected the missing-branch wording, got: %v", err)
	}
}

// Execute must apply the gate itself. Analyze may have run minutes ago, and
// CleanupReport is public — a caller can hand Execute a candidate directly.
func TestCleanupService_ExecuteRefusesRemoteDeleteOnRewoundCanonical(t *testing.T) {
	work, _, _ := staleFixture(t)
	repo := &repository.Repository{Path: work}
	svc := newTestCleanupService(t)
	ctx := context.Background()

	// A fully-formed candidate: SHA included, so the leased push has everything
	// it needs. The freshness gate must be the only thing that stops it.
	report := &CleanupReport{
		NonCanonical: []*Branch{{
			Name:     "master",
			IsRemote: true,
			Ref:      "refs/remotes/origin/master",
			SHA:      gitOutput(t, work, "rev-parse", "refs/remotes/origin/master"),
		}},
	}
	opts := ExecuteOptions{
		Force:           true,
		CanonicalBranch: "develop",
		CanonicalRemote: "origin",
	}

	result, err := svc.Execute(ctx, repo, report, opts)
	if err != nil {
		t.Fatalf("Execute returned an error: %v", err)
	}
	if len(result.Deleted) != 0 {
		t.Errorf("nothing may be deleted on a void ancestry; deleted %v", result.Deleted)
	}
	if len(result.Failed) != 1 {
		t.Fatalf("expected one reported refusal, got %d: %+v", len(result.Failed), result.Failed)
	}

	// And the branch is still on the remote.
	if out := gitOutput(t, work, "ls-remote", "--heads", "origin", "master"); !strings.Contains(out, "master") {
		t.Error("origin/master was deleted despite the refusal")
	}
}
