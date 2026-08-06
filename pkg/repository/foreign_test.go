// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/identity"
)

func TestValidateForeignWorkMode(t *testing.T) {
	tests := []struct {
		value   string
		want    ForeignWorkMode
		wantErr bool
	}{
		{"", ForeignWorkBlock, false},
		{"block", ForeignWorkBlock, false},
		{"allow", ForeignWorkAllow, false},
		{"Block", "", true},
		{"warn", "", true},
	}

	for _, tt := range tests {
		got, err := ValidateForeignWorkMode(tt.value)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ValidateForeignWorkMode(%q) = %q, want error", tt.value, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ValidateForeignWorkMode(%q) returned %v", tt.value, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ValidateForeignWorkMode(%q) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestForeignWorkModeTreatsUnsetAsBlock(t *testing.T) {
	var nilPolicy *PushPolicy
	if got := nilPolicy.foreignWorkMode(); got != ForeignWorkBlock {
		t.Errorf("nil policy = %q, want block", got)
	}

	if got := (&PushPolicy{}).foreignWorkMode(); got != ForeignWorkBlock {
		t.Errorf("empty policy = %q, want block", got)
	}
}

func TestFindForeignCommitsNamesOnlyOtherWriters(t *testing.T) {
	dir := testutil.TempGitRepoWithCommit(t)
	gitRun(t, dir, "checkout", "-b", "remote")
	commit(t, dir, "a.txt", "chore(wip): mine\n\nDevice: dave-office\n")
	commit(t, dir, "b.txt", "chore(wip): theirs\n\nDevice: dave-laptop\n")
	gitRun(t, dir, "branch", "local", "HEAD~2")

	mine := identity.Identity{Device: "dave-office"}
	got, err := findForeignCommits(context.Background(), newTestExecutor(), dir, "local", "remote", mine)
	if err != nil {
		t.Fatalf("findForeignCommits returned %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("found %d commits, want only the one from dave-laptop: %v", len(got), got)
	}
	if got[0].Identity.Device != "dave-laptop" {
		t.Errorf("device = %q, want dave-laptop", got[0].Identity.Device)
	}
	if got[0].Subject != "chore(wip): theirs" {
		t.Errorf("subject = %q, want the theirs commit", got[0].Subject)
	}
}

func TestFindForeignCommitsIgnoresUnsignedCommits(t *testing.T) {
	// Commits made by hand carry no trailer. Counting them as foreign would fire
	// on every branch that was ever touched outside this tool.
	dir := testutil.TempGitRepoWithCommit(t)
	gitRun(t, dir, "checkout", "-b", "remote")
	commit(t, dir, "a.txt", "fix: a thing someone typed by hand")
	gitRun(t, dir, "branch", "local", "HEAD~1")

	got, err := findForeignCommits(context.Background(), newTestExecutor(), dir,
		"local", "remote", identity.Identity{Device: "dave-office"})
	if err != nil {
		t.Fatalf("findForeignCommits returned %v", err)
	}
	if len(got) != 0 {
		t.Errorf("found %v, want nothing", got)
	}
}

func TestFindForeignCommitsKeepsAMultiParagraphMessageIntact(t *testing.T) {
	dir := testutil.TempGitRepoWithCommit(t)
	gitRun(t, dir, "checkout", "-b", "remote")
	commit(t, dir, "a.txt", "chore(wip): theirs\n\nA body paragraph.\n\nAnd another.\n\nDevice: dave-laptop\n")
	gitRun(t, dir, "branch", "local", "HEAD~1")

	got, err := findForeignCommits(context.Background(), newTestExecutor(), dir,
		"local", "remote", identity.Identity{Device: "dave-office"})
	if err != nil {
		t.Fatalf("findForeignCommits returned %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("found %d commits, want 1: %v", len(got), got)
	}
	if got[0].Identity.Device != "dave-laptop" {
		t.Errorf("device = %q, want dave-laptop", got[0].Identity.Device)
	}
}

func TestFindForeignCommitsWithoutAnIdentity(t *testing.T) {
	dir := testutil.TempGitRepoWithCommit(t)
	gitRun(t, dir, "checkout", "-b", "remote")
	commit(t, dir, "a.txt", "chore(wip): theirs\n\nDevice: dave-laptop\n")
	gitRun(t, dir, "branch", "local", "HEAD~1")

	got, err := findForeignCommits(context.Background(), newTestExecutor(), dir,
		"local", "remote", identity.Identity{})
	if err != nil {
		t.Fatalf("findForeignCommits returned %v", err)
	}
	if len(got) != 0 {
		t.Errorf("found %v; a machine that cannot name itself cannot accuse anyone", got)
	}
}

func TestFindForeignCommitsOnARefThatDoesNotExist(t *testing.T) {
	// A branch not yet on the remote is the normal first push, not a conflict.
	dir := testutil.TempGitRepoWithCommit(t)

	got, err := findForeignCommits(context.Background(), newTestExecutor(), dir,
		"HEAD", "origin/nothing-here", identity.Identity{Device: "dave-office"})
	if err != nil {
		t.Fatalf("findForeignCommits returned %v", err)
	}
	if len(got) != 0 {
		t.Errorf("found %v, want nothing", got)
	}
}

func TestForeignWorkRefs(t *testing.T) {
	info := &Info{Branch: "feat/task-001", Upstream: "origin/feat/task-001"}

	tests := []struct {
		name       string
		opts       BulkPushOptions
		wantLocal  string
		wantRemote string
		wantForces bool
	}{
		{
			name:       "a plain push forces nothing",
			opts:       BulkPushOptions{},
			wantLocal:  "feat/task-001",
			wantRemote: "origin/feat/task-001",
			wantForces: false,
		},
		{
			name:       "--force on the current branch",
			opts:       BulkPushOptions{Force: true},
			wantLocal:  "feat/task-001",
			wantRemote: "origin/feat/task-001",
			wantForces: true,
		},
		{
			name:       "a + refspec forces even without --force",
			opts:       BulkPushOptions{Refspec: "+develop:master"},
			wantLocal:  "develop",
			wantRemote: "origin/master",
			wantForces: true,
		},
		{
			name:       "a refspec resolves against the named remote",
			opts:       BulkPushOptions{Refspec: "develop:master", Force: true, Remotes: []string{"backup"}},
			wantLocal:  "develop",
			wantRemote: "backup/master",
			wantForces: true,
		},
		{
			name:       "an unparseable refspec checks nothing",
			opts:       BulkPushOptions{Refspec: "not::a::refspec", Force: true},
			wantLocal:  "",
			wantRemote: "",
			wantForces: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local, remote, forces := foreignWorkRefs(info, tt.opts)
			if local != tt.wantLocal || remote != tt.wantRemote || forces != tt.wantForces {
				t.Errorf("got (%q, %q, %v), want (%q, %q, %v)",
					local, remote, forces, tt.wantLocal, tt.wantRemote, tt.wantForces)
			}
		})
	}
}

func TestForeignWorkRefsOnABranchWithNoUpstream(t *testing.T) {
	// Nothing on the remote means nothing to discard, so the check has no range
	// to read and findForeignCommits treats the empty ref as "skip".
	_, remote, _ := foreignWorkRefs(&Info{Branch: "feat/new"}, BulkPushOptions{Force: true})
	if remote != "" {
		t.Errorf("remote ref = %q, want empty", remote)
	}
}

func TestDescribeForeignWorkNamesTheFirstFew(t *testing.T) {
	foreign := []ForeignCommit{
		{Hash: "aaaaaaaaaaaa", Subject: "first", Identity: identity.Identity{Device: "dave-laptop"}},
		{Hash: "bbbbbbbbbbbb", Subject: "second", Identity: identity.Identity{Device: "dave-laptop", Agent: "hermes-01"}},
		{Hash: "cccccccccccc", Subject: "third", Identity: identity.Identity{Agent: "hermes-02"}},
	}

	got := describeForeignWork(foreign)

	for _, want := range []string{"3 commit(s)", "aaaaaaaa first (dave-laptop)", "bbbbbbbb second (dave-laptop/hermes-01)", "and 1 more"} {
		if !strings.Contains(got, want) {
			t.Errorf("description %q is missing %q", got, want)
		}
	}
	if strings.Contains(got, "third") {
		t.Errorf("description %q should have summarized the third commit, not named it", got)
	}
}

// newTestExecutor builds the executor the client uses, without a client.
func newTestExecutor() *gitcmd.Executor {
	return gitcmd.NewExecutor()
}

// gitRun runs a git command in dir and fails the test if it does not succeed.
func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...) //nolint:noctx // test helper; no context.Context available
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// commit writes a file and commits it with the given message, trailers and all.
func commit(t *testing.T, dir, name, message string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
	gitRun(t, dir, "add", name)
	gitRun(t, dir, "commit", "-m", message)
}
