// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/identity"
)

// runBranchNameIn runs the command from dir, so a .gz-git.yaml written there is
// the one that gets loaded.
func runBranchNameIn(t *testing.T, dir, kind string, args ...string) (string, error) {
	t.Helper()
	t.Chdir(dir)

	orig := branchNameKind
	branchNameKind = kind
	defer func() { branchNameKind = orig }()

	var err error
	out := captureStdout(t, func() {
		err = runBranchName(branchNameCmd, args)
	})

	return strings.TrimSpace(out), err
}

func TestBranchNamePrintsOnlyTheName(t *testing.T) {
	t.Setenv(identity.EnvDevice, "dave-office")
	t.Setenv(identity.EnvAgent, "")

	got, err := runBranchNameIn(t, t.TempDir(), "device", "task-001-product-unit")
	if err != nil {
		t.Fatalf("runBranchName returned %v", err)
	}

	// Anything else on stdout would break $(gz-git branch name ...).
	if want := "feat/task-001-product-unit/dave-office"; got != want {
		t.Errorf("stdout = %q, want exactly %q", got, want)
	}
}

func TestBranchNameReadsTheProjectTemplate(t *testing.T) {
	t.Setenv(identity.EnvDevice, "dave-office")
	t.Setenv(identity.EnvAgent, "")

	dir := t.TempDir()
	writeFile(t, dir, ".gz-git.yaml", "branch:\n  naming:\n    device: wip/{device}/{task}\n")

	got, err := runBranchNameIn(t, dir, "device", "task-002")
	if err != nil {
		t.Fatalf("runBranchName returned %v", err)
	}
	if want := "wip/dave-office/task-002"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBranchNameExitsTwoOnABadKind(t *testing.T) {
	_, err := runBranchNameIn(t, t.TempDir(), "machine", "task-003")
	if err == nil {
		t.Fatal("expected an error for an unknown kind")
	}
	if code := cliutil.ExitCodeForError(err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestBranchNameExitsTwoWhenTheIdentityIsMissing(t *testing.T) {
	t.Setenv(identity.EnvDevice, "")
	t.Setenv(identity.EnvAgent, "")

	// No agent is named, so an agent branch has no segment to build from.
	_, err := runBranchNameIn(t, t.TempDir(), "agent", "task-004")
	if err == nil {
		t.Fatal("expected an error when no agent is named")
	}
	if code := cliutil.ExitCodeForError(err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}
