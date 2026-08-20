// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)

// botRemoteFixture plants origin remote-tracking refs without a real remote:
// a merged bot, an unmerged bot, and a merged protected name (develop).
func botRemoteFixture(t *testing.T) string {
	t.Helper()
	dir := testutil.TempGitRepoWithCommit(t)
	runGit(t, dir, "branch", "-M", "master")

	runGit(t, dir, "checkout", "-b", "tmp-merged")
	commitFile(t, dir, "landed.txt")
	mergedSHA := gitOut(t, dir, "rev-parse", "HEAD")

	runGit(t, dir, "checkout", "-b", "tmp-unmerged", "master")
	commitFile(t, dir, "open.txt")
	unmergedSHA := gitOut(t, dir, "rev-parse", "HEAD")

	runGit(t, dir, "checkout", "master")
	runGit(t, dir, "merge", "--no-ff", "--no-edit", "tmp-merged")
	runGit(t, dir, "branch", "-D", "tmp-merged")
	runGit(t, dir, "branch", "-D", "tmp-unmerged")

	masterSHA := gitOut(t, dir, "rev-parse", "HEAD")
	runGit(t, dir, "update-ref", "refs/remotes/origin/dependabot/go_modules/x", mergedSHA)
	runGit(t, dir, "update-ref", "refs/remotes/origin/dependabot/go_modules/unmerged", unmergedSHA)
	runGit(t, dir, "update-ref", "refs/remotes/origin/renovate/docker-alpine", mergedSHA)
	runGit(t, dir, "update-ref", "refs/remotes/origin/develop", masterSHA)
	runGit(t, dir, "update-ref", "refs/remotes/origin/HEAD", masterSHA)
	runGit(t, dir, "update-ref", "refs/remotes/origin/feat/done", mergedSHA)

	return dir
}

func TestBotRemoteBranches_PartitionsMergedAndPending(t *testing.T) {
	dir := botRemoteFixture(t)

	client := NewClient()
	repo, err := client.Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	merged, pending, err := client.BotRemoteBranches(context.Background(), repo, "master")
	if err != nil {
		t.Fatalf("BotRemoteBranches: %v", err)
	}

	wantMerged := []string{"dependabot/go_modules/x", "renovate/docker-alpine"}
	slices.Sort(merged)
	slices.Sort(wantMerged)
	if !slices.Equal(merged, wantMerged) {
		t.Errorf("merged = %v, want %v", merged, wantMerged)
	}

	if len(pending) != 1 || pending[0] != "dependabot/go_modules/unmerged" {
		t.Errorf("pending = %v, want [dependabot/go_modules/unmerged]", pending)
	}

	for _, name := range append(append([]string{}, merged...), pending...) {
		if name == "develop" || name == "HEAD" || name == "feat/done" {
			t.Errorf("BotRemoteBranches leaked non-bot or protected name %q", name)
		}
	}
}

func TestBotRemoteBranches_EmptyBase(t *testing.T) {
	dir := botRemoteFixture(t)
	client := NewClient()
	repo, err := client.Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	merged, pending, err := client.BotRemoteBranches(context.Background(), repo, "")
	if err != nil {
		t.Errorf("empty base returned error: %v", err)
	}
	if merged != nil || pending != nil {
		t.Errorf("empty base = (%v, %v), want nil, nil", merged, pending)
	}

	if _, _, err := client.BotRemoteBranches(context.Background(), nil, "master"); err == nil {
		t.Error("BotRemoteBranches(nil) returned no error")
	}
}

func TestBulkCleanup_RemoteMergedDryRun(t *testing.T) {
	dir := botRemoteFixture(t)
	parent := filepath.Dir(dir)
	client := NewClient()

	result, err := client.BulkCleanup(context.Background(), BulkCleanupOptions{
		Directory:     parent,
		MaxDepth:      1,
		DryRun:        true,
		IncludeMerged: true,
		DeleteRemote:  true,
		BotsOnly:      true,
		BaseBranch:    "master",
	})
	if err != nil {
		t.Fatalf("BulkCleanup: %v", err)
	}

	var found *RepositoryCleanupResult
	for i := range result.Repositories {
		if result.Repositories[i].Path == dir {
			found = &result.Repositories[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("repo %s not in results: %+v", dir, result.Repositories)
	}
	if found.Status != StatusWouldCleanup {
		t.Errorf("status = %q, want %q", found.Status, StatusWouldCleanup)
	}

	got := map[string]CleanupBranchEntry{}
	for _, b := range found.Branches {
		got[b.Name] = b
	}
	for _, name := range []string{"dependabot/go_modules/x", "renovate/docker-alpine"} {
		entry, ok := got[name]
		if !ok {
			t.Errorf("missing %s in %v", name, found.Branches)
			continue
		}
		if entry.Location != branchLocationRemote {
			t.Errorf("%s location = %q, want remote", name, entry.Location)
		}
		if entry.Reason != "merged" {
			t.Errorf("%s reason = %q, want merged", name, entry.Reason)
		}
	}
	if _, ok := got["dependabot/go_modules/unmerged"]; ok {
		t.Error("unmerged bot listed as a delete candidate")
	}
	if _, ok := got["develop"]; ok {
		t.Error("protected develop listed as a delete candidate")
	}
	if _, ok := got["feat/done"]; ok {
		t.Error("--bots leaked a human topic branch")
	}
}

func TestBulkCleanup_StaleDoesNotDeleteRemoteOnly(t *testing.T) {
	dir := botRemoteFixture(t)
	parent := filepath.Dir(dir)
	client := NewClient()

	result, err := client.BulkCleanup(context.Background(), BulkCleanupOptions{
		Directory:      parent,
		MaxDepth:       1,
		DryRun:         true,
		IncludeStale:   true,
		DeleteRemote:   true,
		BotsOnly:       true,
		StaleThreshold: 0, // every local commit is "stale" if we used a positive duration; 0 is rewritten to 30d
		BaseBranch:     "master",
	})
	if err != nil {
		t.Fatalf("BulkCleanup: %v", err)
	}

	for _, repo := range result.Repositories {
		if repo.Path != dir {
			continue
		}
		for _, b := range repo.Branches {
			if b.Location == branchLocationRemote {
				t.Errorf("--stale listed remote-only %s", b.Name)
			}
		}
	}
}

func TestBulkCleanup_RemoteMergedExecute(t *testing.T) {
	origin, clone := botRemoteBareClone(t)
	client := NewClient()

	result, err := client.BulkCleanup(context.Background(), BulkCleanupOptions{
		Directory:     clone,
		MaxDepth:      1,
		DryRun:        false,
		IncludeMerged: true,
		DeleteRemote:  true,
		BotsOnly:      true,
		BaseBranch:    "master",
	})
	if err != nil {
		t.Fatalf("BulkCleanup: %v", err)
	}

	var found *RepositoryCleanupResult
	for i := range result.Repositories {
		if result.Repositories[i].Path == clone {
			found = &result.Repositories[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("clone not in results: %+v", result.Repositories)
	}
	if found.Status != StatusCleanedUp {
		t.Errorf("status = %q message = %q, want cleaned-up", found.Status, found.Message)
	}

	deleted := map[string]bool{}
	for _, name := range found.DeletedBranches {
		deleted[name] = true
	}
	if !deleted["dependabot/go_modules/x"] {
		t.Errorf("DeletedBranches = %v, want dependabot/go_modules/x", found.DeletedBranches)
	}

	if refExists(t, origin, "refs/heads/dependabot/go_modules/x") {
		t.Error("origin still has dependabot/go_modules/x after push --delete")
	}
	if !refExists(t, origin, "refs/heads/dependabot/go_modules/unmerged") {
		t.Error("origin lost the unmerged bot branch")
	}
}

func TestBulkCleanup_RemoteMergedLeaseRefusesMovedTip(t *testing.T) {
	origin, clone := botRemoteBareClone(t)
	c, ok := NewClient().(*client)
	if !ok {
		t.Fatal("NewClient did not return *client")
	}

	ctx := context.Background()
	result := &RepositoryCleanupResult{}
	opts := BulkCleanupOptions{
		IncludeMerged: true,
		DeleteRemote:  true,
		BotsOnly:      true,
		BaseBranch:    "master",
	}
	toDelete := c.collectCleanupCandidates(ctx, clone, "master", defaultRemoteName, "master", opts, result)

	classifiedSHA := ""
	foundRemote := false
	for _, b := range toDelete {
		if b.name == "dependabot/go_modules/x" && b.location == branchLocationRemote {
			foundRemote = true
			classifiedSHA = b.sha
		}
	}
	if !foundRemote {
		t.Fatal("expected remote merged bot candidate")
	}
	if classifiedSHA == "" {
		t.Fatal("classified SHA is empty")
	}

	other := filepath.Join(t.TempDir(), "other")
	runGit(t, "", "clone", origin, other)
	runGit(t, other, "config", "user.email", "test@test.com")
	runGit(t, other, "config", "user.name", "Test")
	runGit(t, other, "config", "commit.gpgsign", "false")
	runGit(t, other, "checkout", "-B", "dependabot/go_modules/x", "origin/dependabot/go_modules/x")
	commitFile(t, other, "sneak.txt")
	runGit(t, other, "push", "origin", "dependabot/go_modules/x")
	newSHA := gitOut(t, other, "rev-parse", "HEAD")

	deleted := c.executeCleanupDeletes(ctx, clone, defaultRemoteName, toDelete, NewNoopLogger(), "clone")
	for _, b := range deleted {
		if b.name == "dependabot/go_modules/x" && b.location == branchLocationRemote {
			t.Error("leased delete reported success after remote tip moved")
		}
	}
	if !refExists(t, origin, "refs/heads/dependabot/go_modules/x") {
		t.Fatal("moved remote branch must still exist")
	}
	got := gitOut(t, origin, "rev-parse", "refs/heads/dependabot/go_modules/x")
	if got != newSHA {
		t.Errorf("origin tip = %s, want new commit %s", got, newSHA)
	}
	if classifiedSHA == newSHA {
		t.Fatal("race did not move the tip")
	}
}

func botRemoteBareClone(t *testing.T) (origin, clone string) {
	t.Helper()
	seed := testutil.TempGitRepoWithCommit(t)
	runGit(t, seed, "branch", "-M", "master")

	runGit(t, seed, "checkout", "-b", "dependabot/go_modules/x")
	commitFile(t, seed, "bot.txt")
	runGit(t, seed, "checkout", "master")
	runGit(t, seed, "merge", "--no-ff", "--no-edit", "dependabot/go_modules/x")

	runGit(t, seed, "checkout", "-b", "dependabot/go_modules/unmerged")
	commitFile(t, seed, "unmerged.txt")
	runGit(t, seed, "checkout", "master")

	root := t.TempDir()
	origin = filepath.Join(root, "origin.git")
	clone = filepath.Join(root, "clone")
	runGit(t, "", "clone", "--bare", seed, origin)
	runGit(t, "", "clone", origin, clone)
	runGit(t, clone, "config", "user.email", "test@test.com")
	runGit(t, clone, "config", "user.name", "Test")
	runGit(t, clone, "config", "commit.gpgsign", "false")
	return origin, clone
}

func refExists(t *testing.T, dir, ref string) bool {
	t.Helper()
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", ref) //nolint:noctx // test helper
	cmd.Dir = dir
	return cmd.Run() == nil
}
