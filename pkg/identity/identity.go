// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

// Package identity names the machine and the agent behind an automated commit.
//
// A checkpoint commit made by [gz-git handoff end] is written without anyone
// watching, so the commit itself has to say where it came from. Reading a work
// branch later, "which machine left this half-finished" is the question that
// decides whether it is safe to rebase, and git records only the author — which
// is the same person on every machine they own.
package identity

import (
	"os"
	"regexp"
	"strings"
)

// Environment variables that name the current machine and agent. They win over
// configuration because an agent process knows its own name at launch, while a
// config file is written once and shared by every run on the machine.
const (
	EnvDevice = "GZ_GIT_DEVICE"
	EnvAgent  = "GZ_GIT_AGENT"
)

// Identity is who made a commit, beyond the git author.
//
// It belongs in machine-local configuration — the global config file or a
// profile — and not in a project's .gz-git.yaml, which is committed and would
// then give every machine that clones it the same device name.
type Identity struct {
	// Device names the machine. Defaults to the hostname.
	Device string `yaml:"device,omitempty" json:"device,omitempty"`

	// Agent names the automation acting on this machine. Empty means a person
	// is driving, so there is nothing to record.
	Agent string `yaml:"agent,omitempty" json:"agent,omitempty"`
}

// Resolve fills in an identity from the environment and the hostname.
//
// A nil or partial configured identity is normal: most machines set nothing and
// still get a usable device name.
func Resolve(configured *Identity) Identity {
	resolved := Identity{}
	if configured != nil {
		resolved = *configured
	}

	if env := os.Getenv(EnvDevice); env != "" {
		resolved.Device = env
	}
	if env := os.Getenv(EnvAgent); env != "" {
		resolved.Agent = env
	}

	resolved.Device = sanitize(resolved.Device)
	resolved.Agent = sanitize(resolved.Agent)

	if resolved.Device == "" {
		// An unreadable hostname leaves the device unnamed rather than failing:
		// a commit missing one trailer is better than a checkpoint that does
		// not happen.
		host, err := os.Hostname()
		if err == nil {
			resolved.Device = sanitize(host)
		}
	}

	return resolved
}

// Trailers renders the identity as git trailer lines, in the order they should
// appear. An unnamed field contributes nothing.
func (i Identity) Trailers() []string {
	trailers := make([]string, 0, 2)
	if i.Device != "" {
		trailers = append(trailers, "Device: "+i.Device)
	}
	if i.Agent != "" {
		trailers = append(trailers, "Agent: "+i.Agent)
	}
	return trailers
}

// FromMessage reads back the identity a commit message was signed with.
//
// A message with no trailers yields a zero Identity, which is not the same as
// "made by nobody": most commits are written by hand and carry no trailer at
// all, so an empty result means unknown rather than absent.
func FromMessage(message string) Identity {
	var found Identity
	for line := range strings.SplitSeq(message, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Device:"):
			found.Device = strings.TrimSpace(strings.TrimPrefix(line, "Device:"))
		case strings.HasPrefix(line, "Agent:"):
			found.Agent = strings.TrimSpace(strings.TrimPrefix(line, "Agent:"))
		}
	}
	return found
}

// Known reports whether the identity names anything at all.
func (i Identity) Known() bool {
	return i.Device != "" || i.Agent != ""
}

// Name renders the identity for a person to read: "device/agent" when both are
// named, otherwise whichever one is. An unknown identity renders as "unknown"
// rather than as an empty string, so it cannot be mistaken for a missing field.
func (i Identity) Name() string {
	switch {
	case i.Device != "" && i.Agent != "":
		return i.Device + "/" + i.Agent
	case i.Device != "":
		return i.Device
	case i.Agent != "":
		return i.Agent
	default:
		return "unknown"
	}
}

// DiffersFrom reports whether other is positive evidence of a different writer.
//
// It compares only the fields both sides name. An unsigned commit differs from
// nothing: it is unattributed, not attributed elsewhere, and treating the
// absence of a trailer as a foreign writer would fire on every commit made by
// hand.
func (i Identity) DiffersFrom(other Identity) bool {
	if other.Device != "" && i.Device != "" && other.Device != i.Device {
		return true
	}
	return other.Agent != "" && i.Agent != "" && other.Agent != i.Agent
}

// AppendTrailers returns message with the identity's trailers added, skipping
// any the message already carries so the result is stable under a rerun.
func (i Identity) AppendTrailers(message string) string {
	trailers := i.Trailers()
	if len(trailers) == 0 {
		return message
	}

	body := strings.TrimRight(message, "\n \t")
	existing := lineSet(body)

	missing := make([]string, 0, len(trailers))
	for _, trailer := range trailers {
		if !existing[trailer] {
			missing = append(missing, trailer)
		}
	}
	if len(missing) == 0 {
		return message
	}

	separator := "\n\n"
	if endsWithTrailerBlock(body) {
		separator = "\n"
	}

	return body + separator + strings.Join(missing, "\n") + "\n"
}

// trailerLine matches a git trailer: a token, a colon, then a value.
var trailerLine = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*:[ \t]`)

// endsWithTrailerBlock reports whether the message's last paragraph is already
// trailers, in which case the new ones join it instead of starting a paragraph.
//
// A message with no blank line is never a trailer block, however it is spelled:
// a one-line "fix: thing" is a subject, and treating it as a trailer would put
// the identity on the subject line.
func endsWithTrailerBlock(body string) bool {
	paragraphs := strings.Split(body, "\n\n")
	if len(paragraphs) < 2 {
		return false
	}

	last := strings.TrimSpace(paragraphs[len(paragraphs)-1])
	if last == "" {
		return false
	}

	for line := range strings.SplitSeq(last, "\n") {
		if !trailerLine.MatchString(line) {
			return false
		}
	}
	return true
}

// lineSet indexes the trimmed lines of a message for an exact-match lookup.
func lineSet(body string) map[string]bool {
	lines := make(map[string]bool)
	for line := range strings.SplitSeq(body, "\n") {
		lines[strings.TrimSpace(line)] = true
	}
	return lines
}

// sanitize reduces a name to something that can sit on one trailer line.
func sanitize(name string) string {
	name = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		return r
	}, name)
	return strings.TrimSpace(name)
}
