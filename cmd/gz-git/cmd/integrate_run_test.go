// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/integrate"
)

func TestIntegrateRunHelp(t *testing.T) {
	cmd := findCommand(t, rootCmd, "integrate", "run")
	for _, name := range []string{"target", "direct-to-default", "release", "allow-skipped-checks", "controller-config"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("integrate run missing --%s", name)
		}
	}
}

func TestIntegrateRunBareTargetCheckoutExitsOneWithoutMovingRemote(t *testing.T) {
	restore := setIntegrateRunGlobals(t)
	defer restore()

	fx := testutil.TempWorktreeWithBareOrigin(t)
	target := gitOutputForIntegrateRun(t, fx.Clone, "branch", "--show-current")
	writeFile(t, fx.Clone, ".gz-git.yaml", "branch:\n  integrationBranch: "+target+"\n")
	before := gitOutputForIntegrateRun(t, fx.Origin, "rev-parse", "refs/heads/"+target)
	t.Chdir(fx.Clone)
	quiet = true
	err := runIntegrateRun(integrateRunCmd, nil)
	if got := cliutil.ExitCodeForError(err); got != 1 {
		t.Fatalf("bare target checkout exit = %d, want 1; err=%v", got, err)
	}
	if !errors.Is(err, integrate.ErrImplicitSourceIsTarget) {
		t.Fatalf("error = %v, want ErrImplicitSourceIsTarget", err)
	}
	after := gitOutputForIntegrateRun(t, fx.Origin, "rev-parse", "refs/heads/"+target)
	if after != before {
		t.Fatalf("remote master moved: before=%s after=%s", before, after)
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
	writeFile(t, fx.Worktree, "Makefile", "check:\n\t@true\n")
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
	origController := integrateRunControllerConfig
	origQuiet := quiet
	integrateRunTarget = ""
	integrateRunDirectToDefault = false
	integrateRunRelease = false
	integrateRunAllowSkipped = false
	integrateRunControllerConfig = ""
	quiet = false
	return func() {
		integrateRunTarget = origTarget
		integrateRunDirectToDefault = origDirect
		integrateRunRelease = origRelease
		integrateRunAllowSkipped = origSkip
		integrateRunControllerConfig = origController
		quiet = origQuiet
	}
}

func gitOutputForIntegrateRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out))
}
