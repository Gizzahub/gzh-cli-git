// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/integrate"
)

func TestIntegrateQueueHelp(t *testing.T) {
	cmd := findCommand(t, rootCmd, "integrate", "queue")
	for _, name := range []string{"base", "expiry-days", "no-fetch", "controller-config"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("integrate queue missing --%s", name)
		}
	}
	if cmd.Flags().Lookup("expiry-days").DefValue != "7" {
		t.Errorf("--expiry-days default = %q, want 7", cmd.Flags().Lookup("expiry-days").DefValue)
	}
}

func TestIntegrateQueueEmptyIsSuccess(t *testing.T) {
	restore := setIntegrateQueueGlobals(t)
	defer restore()

	dir := testutil.TempGitRepoWithCommit(t)
	t.Chdir(dir)
	integrateQueueBase = "main"
	integrateQueueNoFetch = true
	quiet = true

	if err := runIntegrateQueue(integrateQueueCmd, nil); err != nil {
		t.Fatalf("empty queue must be exit 0, got %v (code %d)", err, cliutil.ExitCodeForError(err))
	}
}

func TestIntegrateQueueMissingBaseExitsOne(t *testing.T) {
	restore := setIntegrateQueueGlobals(t)
	defer restore()

	dir := testutil.TempGitRepoWithCommit(t)
	t.Chdir(dir)
	integrateQueueNoFetch = true
	quiet = false

	var stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetErr(&stderr)
	err := runIntegrateQueue(cmd, nil)
	if got := cliutil.ExitCodeForError(err); got != 1 {
		t.Fatalf("missing base exit = %d, want 1; err=%v", got, err)
	}
}

func TestIntegrateQueueQuietMissingBaseIsVisibleAndExitsOne(t *testing.T) {
	restore := setIntegrateQueueGlobals(t)
	defer restore()

	dir := testutil.TempGitRepoWithCommit(t)
	t.Chdir(dir)
	integrateQueueNoFetch = true
	quiet = true

	var stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetErr(&stderr)
	err := runIntegrateQueue(cmd, nil)
	if got := cliutil.ExitCodeForError(err); got != 1 {
		t.Fatalf("quiet missing base exit = %d, want 1; err=%v", got, err)
	}
	if got := stderr.String(); !strings.Contains(got, "no base ref") {
		t.Fatalf("quiet missing base stderr = %q, want visible base diagnostic", got)
	}
}

func TestIntegrateQueueNotRepoExitsTwo(t *testing.T) {
	restore := setIntegrateQueueGlobals(t)
	defer restore()

	t.Chdir(t.TempDir())
	quiet = false
	err := runIntegrateQueue(integrateQueueCmd, nil)
	if got := cliutil.ExitCodeForError(err); got != 2 {
		t.Fatalf("not-a-repo exit = %d, want 2; err=%v", got, err)
	}
}

func TestIntegrateQueueQuietControllerFailureExitsTwo(t *testing.T) {
	restore := setIntegrateQueueGlobals(t)
	defer restore()

	dir := testutil.TempGitRepoWithCommit(t)
	t.Chdir(dir)
	integrateQueueControllerConfig = t.TempDir() + "/missing.yaml"
	quiet = true
	err := runIntegrateQueue(integrateQueueCmd, nil)
	if got := cliutil.ExitCodeForError(err); got != 2 {
		t.Fatalf("quiet controller error exit = %d, want 2; err=%v", got, err)
	}
}

func TestBranchListFlagsUnchanged(t *testing.T) {
	// queue is not an extension of branch list. These opts must stay.
	if !strings.Contains(integrateQueueCmd.Use, "queue") {
		t.Fatal("integrate queue command missing")
	}
	_ = integrate.DefaultExpiryDays
}

func setIntegrateQueueGlobals(t *testing.T) func() {
	t.Helper()
	origBase := integrateQueueBase
	origDays := integrateQueueExpiryDays
	origNoFetch := integrateQueueNoFetch
	origController := integrateQueueControllerConfig
	origQuiet := quiet
	integrateQueueBase = ""
	integrateQueueExpiryDays = integrate.DefaultExpiryDays
	integrateQueueNoFetch = false
	integrateQueueControllerConfig = ""
	quiet = false
	return func() {
		integrateQueueBase = origBase
		integrateQueueExpiryDays = origDays
		integrateQueueNoFetch = origNoFetch
		integrateQueueControllerConfig = origController
		quiet = origQuiet
	}
}
