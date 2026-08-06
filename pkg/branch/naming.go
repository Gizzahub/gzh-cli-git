// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package branch

import (
	"fmt"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/identity"
)

// Kind names the role a branch plays when more than one writer works on the
// same task: the shared work branch, one machine's private slice of it, or one
// agent's. The three differ only by name, which is exactly why the name has to
// be predictable — nothing else distinguishes them.
type Kind string

const (
	// KindWork is the branch a task lives on when it has a single writer.
	KindWork Kind = "work"
	// KindDevice is one machine's slice of a task worked from several machines.
	KindDevice Kind = "device"
	// KindAgent is one agent's slice, kept apart from any human's branch.
	KindAgent Kind = "agent"
)

// Default templates follow the convention the handoff commands assume: a task
// branch under feat/, a per-writer branch below it, and agent work in its own
// namespace so it is never mistaken for a person's.
const (
	DefaultWorkTemplate   = "feat/{task}"
	DefaultDeviceTemplate = "feat/{task}/{device}"
	DefaultAgentTemplate  = "agent/{task}/{agent}"
)

// Naming holds one branch-name template per kind. An empty template falls back
// to the default for that kind, so a config may override one and leave the rest.
type Naming struct {
	Work   string `yaml:"work,omitempty"`
	Device string `yaml:"device,omitempty"`
	Agent  string `yaml:"agent,omitempty"`
}

// ParseKind reads a kind from a flag value. An empty value means the work
// branch: the single-writer case is the one that needs no explanation.
func ParseKind(value string) (Kind, error) {
	switch Kind(value) {
	case "", KindWork:
		return KindWork, nil
	case KindDevice:
		return KindDevice, nil
	case KindAgent:
		return KindAgent, nil
	default:
		return "", fmt.Errorf("unknown branch kind %q (want work, device or agent)", value)
	}
}

// Template returns the template for a kind, falling back to the default.
func (n *Naming) Template(kind Kind) (string, error) {
	var configured string
	if n != nil {
		switch kind {
		case KindWork:
			configured = n.Work
		case KindDevice:
			configured = n.Device
		case KindAgent:
			configured = n.Agent
		}
	}
	if configured != "" {
		return configured, nil
	}

	switch kind {
	case KindWork:
		return DefaultWorkTemplate, nil
	case KindDevice:
		return DefaultDeviceTemplate, nil
	case KindAgent:
		return DefaultAgentTemplate, nil
	default:
		return "", fmt.Errorf("unknown branch kind %q", kind)
	}
}

// Resolve builds the branch name for a task from the template for its kind.
//
// Every substituted value is slugified first. A device name defaults to the
// hostname, and a hostname is free to contain dots and capitals that a branch
// name is not, so the alternative is a template that works on one machine and
// fails on the next.
func (n *Naming) Resolve(kind Kind, task string, id identity.Identity) (string, error) {
	template, err := n.Template(kind)
	if err != nil {
		return "", err
	}

	taskSlug := slug(task)
	if taskSlug == "" {
		return "", fmt.Errorf("task %q has nothing usable in a branch name", task)
	}

	// A device or agent branch that cannot name its writer is the shared branch
	// again under a longer name, which is the collision the split exists to avoid.
	device, agent := slug(id.Device), slug(id.Agent)
	if kind == KindDevice && device == "" {
		return "", fmt.Errorf("a device branch needs a device name: set identity.device or GZ_GIT_DEVICE")
	}
	if kind == KindAgent && agent == "" {
		return "", fmt.Errorf("an agent branch needs an agent name: set identity.agent or GZ_GIT_AGENT")
	}

	name := strings.NewReplacer(
		"{task}", taskSlug,
		"{device}", device,
		"{agent}", agent,
	).Replace(template)

	// A misspelled placeholder survives substitution intact; validation rejects
	// the braces rather than creating a branch with a literal {devcie} in it.
	if err := validateBranchName(name); err != nil {
		return "", fmt.Errorf("template %q produced an invalid branch name %q: %w", template, name, err)
	}

	return name, nil
}

// slug reduces a value to what a branch name accepts: lowercase letters,
// digits, and dashes. Runs of anything else collapse to a single dash.
func slug(value string) string {
	var b strings.Builder
	dashPending := false

	for _, r := range strings.ToLower(value) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			if dashPending && b.Len() > 0 {
				b.WriteByte('-')
			}
			dashPending = false
			b.WriteRune(r)
		default:
			// Held rather than written, so trailing separators never survive.
			dashPending = true
		}
	}

	return b.String()
}
