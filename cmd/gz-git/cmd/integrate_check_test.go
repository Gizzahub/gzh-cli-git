// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
)

func TestIntegrateCheckHelp(t *testing.T) {
	cmd := findCommand(t, rootCmd, "integrate", "check")
	for _, name := range []string{"target", "direct-to-default", "release", "allow-skipped-checks"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("integrate check missing --%s", name)
		}
	}
}

func TestIntegrateCheckUndeclaredResolvesOriginHead(t *testing.T) {
	restore := setIntegrateCheckGlobals(t)
	defer restore()

	fx := testutil.TempWorktreeWithBareOrigin(t)
	t.Chdir(fx.Worktree)
	quiet = true
	err := runIntegrateCheck(integrateCheckCmd, []string{"feature/worktree"})
	if err != nil && strings.Contains(err.Error(), "--target") {
		t.Fatalf("undeclared repo with origin/HEAD must not require --target: %v", err)
	}
}

func TestIntegrateCheckNotRepoExitsTwo(t *testing.T) {
	restore := setIntegrateCheckGlobals(t)
	defer restore()
	t.Chdir(t.TempDir())
	quiet = true
	err := runIntegrateCheck(integrateCheckCmd, nil)
	if got := cliutil.ExitCodeForError(err); got != 2 {
		t.Fatalf("not-a-repo exit = %d, want 2; err=%v", got, err)
	}
}

func setIntegrateCheckGlobals(t *testing.T) func() {
	t.Helper()
	origTarget := integrateCheckTarget
	origDirect := integrateCheckDirectToDefault
	origRelease := integrateCheckRelease
	origSkip := integrateCheckAllowSkipped
	origQuiet := quiet
	integrateCheckTarget = ""
	integrateCheckDirectToDefault = false
	integrateCheckRelease = false
	integrateCheckAllowSkipped = false
	quiet = false
	return func() {
		integrateCheckTarget = origTarget
		integrateCheckDirectToDefault = origDirect
		integrateCheckRelease = origRelease
		integrateCheckAllowSkipped = origSkip
		quiet = origQuiet
	}
}
