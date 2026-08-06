// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package identity

import (
	"os"
	"strings"
	"testing"
)

func TestResolveFallsBackToHostname(t *testing.T) {
	t.Setenv(EnvDevice, "")
	t.Setenv(EnvAgent, "")

	got := Resolve(nil)

	host, err := os.Hostname()
	if err != nil {
		t.Skip("hostname unavailable")
	}
	if got.Device != host {
		t.Errorf("device = %q, want hostname %q", got.Device, host)
	}
	if got.Agent != "" {
		t.Errorf("agent = %q, want empty when nothing names one", got.Agent)
	}
}

func TestEnvironmentOverridesConfig(t *testing.T) {
	t.Setenv(EnvDevice, "dave-laptop")
	t.Setenv(EnvAgent, "hermes-01")

	got := Resolve(&Identity{Device: "dave-office", Agent: "none"})

	if got.Device != "dave-laptop" {
		t.Errorf("device = %q, want dave-laptop", got.Device)
	}
	if got.Agent != "hermes-01" {
		t.Errorf("agent = %q, want hermes-01", got.Agent)
	}
}

func TestResolveStripsNewlinesFromNames(t *testing.T) {
	t.Setenv(EnvDevice, "dave\noffice")
	t.Setenv(EnvAgent, "")

	// A name spanning two lines would end the trailer block and turn the rest
	// into commit body text.
	if got := Resolve(nil).Device; strings.Contains(got, "\n") {
		t.Errorf("device = %q, want no newline", got)
	}
}

func TestTrailersOmitUnnamedFields(t *testing.T) {
	got := Identity{Device: "dave-office"}.Trailers()
	if len(got) != 1 || got[0] != "Device: dave-office" {
		t.Errorf("trailers = %v, want only the device", got)
	}

	if got := (Identity{}).Trailers(); len(got) != 0 {
		t.Errorf("trailers = %v, want none", got)
	}
}

func TestAppendTrailersToSubjectOnlyMessage(t *testing.T) {
	id := Identity{Device: "dave-office", Agent: "hermes-01"}

	got := id.AppendTrailers("chore(wip): handoff checkpoint")
	want := "chore(wip): handoff checkpoint\n\nDevice: dave-office\nAgent: hermes-01\n"

	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestSubjectThatLooksLikeATrailerStillGetsABlankLine(t *testing.T) {
	// "fix: thing" matches the trailer shape. Joining the identity to it would
	// produce a two-line subject.
	got := Identity{Device: "dave-office"}.AppendTrailers("fix: thing")

	if !strings.HasPrefix(got, "fix: thing\n\n") {
		t.Errorf("got %q, want a blank line after the subject", got)
	}
}

func TestAppendJoinsAnExistingTrailerBlock(t *testing.T) {
	message := "chore(wip): checkpoint\n\nsome body\n\nCo-Authored-By: Someone <s@example.com>"

	got := Identity{Device: "dave-office"}.AppendTrailers(message)
	want := message + "\nDevice: dave-office\n"

	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestAppendIsIdempotent(t *testing.T) {
	id := Identity{Device: "dave-office", Agent: "hermes-01"}

	once := id.AppendTrailers("chore(wip): checkpoint")
	twice := id.AppendTrailers(once)

	if once != twice {
		t.Errorf("second append changed the message:\n%q\n%q", once, twice)
	}
}

func TestAppendAddsOnlyTheMissingTrailer(t *testing.T) {
	id := Identity{Device: "dave-office", Agent: "hermes-01"}
	message := "chore(wip): checkpoint\n\nDevice: dave-office"

	got := id.AppendTrailers(message)
	want := message + "\nAgent: hermes-01\n"

	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestAppendWithoutIdentityLeavesMessageAlone(t *testing.T) {
	const message = "chore(wip): checkpoint"
	if got := (Identity{}).AppendTrailers(message); got != message {
		t.Errorf("got %q, want it unchanged", got)
	}
}
