// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"path/filepath"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
)

func TestIntegrateRunHelp(t *testing.T) {
	cmd := findCommand(t, rootCmd, "integrate", "run")
	for _, name := range []string{"target", "direct-to-default", "release", "allow-skipped-checks"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("integrate run missing --%s", name)
		}
	}
}

func TestIntegrateRunReclaimIncompleteExitsThree(t *testing.T) {
	restore := setIntegrateRunGlobals(t)
	defer restore()

	fx := testutil.TempWorktreeWithBareOrigin(t)
	runGit(t, fx.Clone, "branch", "develop")
	runGit(t, fx.Clone, "push", "-u", fx.Remote, "develop")
	runGit(t, fx.Worktree, "checkout", "-B", "dev/actor/feat/task", "develop")
	writeFile(t, fx.Worktree, "task.txt", "task\n")
	writeFile(t, fx.Worktree, ".gz-git.yaml", "branch:\n  integrationBranch: develop\n  taskPattern: dev/*\n")
	runGit(t, fx.Worktree, "add", ".")
	runGit(t, fx.Worktree, "commit", "-m", "task")
	runGit(t, fx.Worktree, "push", "-u", fx.Remote, "HEAD")

	extra := filepath.Join(t.TempDir(), "extra")
	runGit(t, fx.Clone, "worktree", "add", "--force", extra, "dev/actor/feat/task")

	t.Chdir(fx.Worktree)
	quiet = true
	err := runIntegrateRun(integrateRunCmd, []string{"dev/actor/feat/task"})
	if got := cliutil.ExitCodeForError(err); got != cliutil.ExitReclaimIncomplete {
		t.Fatalf("partial reclaim exit = %d, want %d (%v)", got, cliutil.ExitReclaimIncomplete, err)
	}
}

func TestIntegrateRunNotRepoExitsTwo(t *testing.T) {
	restore := setIntegrateRunGlobals(t)
	defer restore()
	t.Chdir(t.TempDir())
	quiet = true
	err := runIntegrateRun(integrateRunCmd, nil)
	if got := cliutil.ExitCodeForError(err); got != 2 {
		t.Fatalf("not-a-repo exit = %d, want 2; err=%v", got, err)
	}
}

func setIntegrateRunGlobals(t *testing.T) func() {
	t.Helper()
	origTarget := integrateRunTarget
	origDirect := integrateRunDirectToDefault
	origRelease := integrateRunRelease
	origSkip := integrateRunAllowSkipped
	origQuiet := quiet
	integrateRunTarget = ""
	integrateRunDirectToDefault = false
	integrateRunRelease = false
	integrateRunAllowSkipped = false
	quiet = false
	return func() {
		integrateRunTarget = origTarget
		integrateRunDirectToDefault = origDirect
		integrateRunRelease = origRelease
		integrateRunAllowSkipped = origSkip
		quiet = origQuiet
	}
}
