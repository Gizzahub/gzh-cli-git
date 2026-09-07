// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package branch

import (
	"context"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// forkFixture builds the shape a fork checkout has: `origin` is the user's own
// fork, `upstream` is somebody else's project. Both carry a `master` that is an
// ancestor of `develop`, and the declaration names develop.
//
// The point of the fixture is that the *only* thing distinguishing the two
// candidates is which remote they sit on. Nothing about the branch itself says
// whether deleting it is this repository's business.
func forkFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	gitCommit(t, dir, "init", "-q", "-b", "master")
	gitCommit(t, dir, "config", "user.email", "t@t")
	gitCommit(t, dir, "config", "user.name", "t")
	gitCommit(t, dir, "commit", "-q", "--allow-empty", "-m", "init")
	gitCommit(t, dir, "checkout", "-q", "-b", "develop")
	gitCommit(t, dir, "commit", "-q", "--allow-empty", "-m", "move to develop")

	developSHA := gitOutput(t, dir, "rev-parse", "develop")
	masterSHA := gitOutput(t, dir, "rev-parse", "master")

	// Both remotes have moved to develop; both still carry the old master.
	for _, remote := range []string{"origin", "upstream"} {
		gitCommit(t, dir, "update-ref", "refs/remotes/"+remote+"/develop", developSHA)
		gitCommit(t, dir, "update-ref", "refs/remotes/"+remote+"/master", masterSHA)
	}
	gitCommit(t, dir, "branch", "-D", "-q", "master")

	return dir
}

func TestCleanupService_NonCanonicalIgnoresForeignRemote(t *testing.T) {
	dir := forkFixture(t)
	repo := &repository.Repository{Path: dir}
	svc := newTestCleanupService(t)
	ctx := context.Background()

	opts := AnalyzeOptions{
		IncludeNonCanonical: true,
		CanonicalBranch:     "develop",
		CanonicalRemote:     "origin",
	}

	governed := &Branch{Name: "master", IsRemote: true, Ref: "refs/remotes/origin/master"}
	if !svc.isNonCanonical(ctx, repo, governed, opts) {
		t.Error("origin/master duplicates the declared canonical branch on the governed remote; it must classify")
	}

	foreign := &Branch{Name: "master", IsRemote: true, Ref: "refs/remotes/upstream/master"}
	if svc.isNonCanonical(ctx, repo, foreign, opts) {
		t.Error("upstream/master sits on a remote the declaration never described; it must not classify")
	}
}

// The Execute-side gate has to refuse independently. CleanupReport is public, so
// a caller can hand it a candidate Analyze never classified.
func TestCleanupService_AuthorizeRetireRefusesForeignRemote(t *testing.T) {
	dir := forkFixture(t)
	repo := &repository.Repository{Path: dir}
	svc := newTestCleanupService(t)
	ctx := context.Background()

	opts := ExecuteOptions{CanonicalBranch: "develop", CanonicalRemote: "origin"}

	governed := &Branch{Name: "master", IsRemote: true, Ref: "refs/remotes/origin/master"}
	if !svc.authorizeRetire(ctx, repo, governed, opts) {
		t.Error("authorizeRetire refused a candidate on the governed remote")
	}

	foreign := &Branch{Name: "master", IsRemote: true, Ref: "refs/remotes/upstream/master"}
	if svc.authorizeRetire(ctx, repo, foreign, opts) {
		t.Error("authorizeRetire authorized a delete on a remote the declaration never described")
	}
}

// An empty CanonicalRemote must mean origin, not "any remote". The zero value is
// what a caller who never heard of the field passes, so it has to be the safe
// one.
func TestCleanupService_EmptyCanonicalRemoteMeansOrigin(t *testing.T) {
	dir := forkFixture(t)
	repo := &repository.Repository{Path: dir}
	svc := newTestCleanupService(t)
	ctx := context.Background()

	opts := ExecuteOptions{CanonicalBranch: "develop"}

	if !svc.authorizeRetire(ctx, repo, &Branch{
		Name: "master", IsRemote: true, Ref: "refs/remotes/origin/master",
	}, opts) {
		t.Error("an empty CanonicalRemote must still authorize the origin candidate")
	}
	if svc.authorizeRetire(ctx, repo, &Branch{
		Name: "master", IsRemote: true, Ref: "refs/remotes/upstream/master",
	}, opts) {
		t.Error("an empty CanonicalRemote must not be read as 'any remote'")
	}
}

func TestGovernedRemote(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty defaults to origin", "", "origin"},
		{"whitespace defaults to origin", "   ", "origin"},
		{"explicit remote is kept", "upstream", "upstream"},
		{"surrounding whitespace is trimmed", "  gitlab  ", "gitlab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := governedRemote(tt.in); got != tt.want {
				t.Errorf("governedRemote(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
