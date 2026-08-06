// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import "fmt"

// ForceMode decides which kind of force push a policy tolerates.
type ForceMode string

const (
	// ForceModeLeaseOnly permits --force, which pushes with --force-with-lease,
	// and refuses a "+" refspec, which does not. It is the default.
	ForceModeLeaseOnly ForceMode = "lease-only"

	// ForceModeAllow permits every force push, including an unleased one.
	ForceModeAllow ForceMode = "allow"

	// ForceModeDeny refuses every force push.
	ForceModeDeny ForceMode = "deny"
)

// ValidateForceMode resolves a configured or flag-supplied force mode. An empty
// value is the default rather than an error, so callers can pass through an
// unset flag.
func ValidateForceMode(value string) (ForceMode, error) {
	switch ForceMode(value) {
	case "":
		return ForceModeLeaseOnly, nil
	case ForceModeLeaseOnly, ForceModeAllow, ForceModeDeny:
		return ForceMode(value), nil
	default:
		return "", fmt.Errorf("invalid force mode %q (expected lease-only, allow, or deny)", value)
	}
}

// PushPolicy restricts which branches a push may write and how.
//
// It is separate from ProtectedBranches, which guards deletion with a built-in
// list. This one starts empty: refusing to push to main is a workflow decision
// a project makes, not something the tool assumes.
type PushPolicy struct {
	// Protected lists branch names and trailing-* patterns that may not be
	// pushed to at all, matching the pattern syntax used for deletion.
	Protected []string `yaml:"protected,omitempty"`

	// ForceMode decides which force pushes are allowed to every other branch.
	ForceMode ForceMode `yaml:"forceMode,omitempty"`

	// ForeignWork decides what happens to a force push that would discard
	// commits signed by another machine or agent. Unset means block.
	//
	// Unlike the rules above it cannot be decided from intent alone: it needs
	// to read the commits the remote has and this machine does not.
	ForeignWork ForeignWorkMode `yaml:"foreignWork,omitempty"`
}

// foreignWorkMode returns the configured mode, treating an unset value — and a
// nil policy — as the default rather than as "allow".
func (p *PushPolicy) foreignWorkMode() ForeignWorkMode {
	if p == nil || p.ForeignWork == "" {
		return ForeignWorkBlock
	}
	return p.ForeignWork
}

// PushRule names the rule a push ran into.
type PushRule string

const (
	// PushRuleProtected marks a push to a branch listed as protected.
	PushRuleProtected PushRule = "protected-branch"
	// PushRuleRawForce marks a "+" refspec under lease-only mode.
	PushRuleRawForce PushRule = "raw-force"
	// PushRuleForceDenied marks any force push under deny mode.
	PushRuleForceDenied PushRule = "force-denied"
	// PushRuleForeignWork marks a force push that would discard commits
	// signed by another machine or agent.
	PushRuleForeignWork PushRule = "foreign-work"
)

// PushDenial is a policy's refusal of one repository's push.
type PushDenial struct {
	Rule   PushRule
	Branch string
	Detail string
}

// PushIntent describes what a single repository is about to push.
type PushIntent struct {
	// Branch is the branch HEAD is on, used when no refspec names a target.
	Branch string
	// Refspec is the --refspec value, if one was given.
	Refspec string
	// Force reports whether --force was given, which pushes with a lease.
	Force bool
}

// Check reports why the policy refuses this push, or nil if it permits it.
// A nil policy permits everything.
func (p *PushPolicy) Check(intent PushIntent) *PushDenial {
	if p == nil {
		return nil
	}

	branch, rawForce := intent.target()

	if p.isProtected(branch) {
		return &PushDenial{
			Rule:   PushRuleProtected,
			Branch: branch,
			Detail: fmt.Sprintf("%q is protected; land work there through a pull request", branch),
		}
	}

	switch p.mode() {
	case ForceModeAllow:
		return nil

	case ForceModeDeny:
		if rawForce || intent.Force {
			return &PushDenial{
				Rule:   PushRuleForceDenied,
				Branch: branch,
				Detail: "force pushing is disabled by policy",
			}
		}

	case ForceModeLeaseOnly:
		if rawForce {
			return &PushDenial{
				Rule:   PushRuleRawForce,
				Branch: branch,
				Detail: `a "+" refspec force pushes without a lease, discarding commits this machine never fetched; use --force, which pushes with --force-with-lease`,
			}
		}
	}

	return nil
}

// target resolves the branch that would be written and whether the push carries
// an unleased force. An unparseable refspec is left to the caller, which reports
// the parse error itself; here it simply matches nothing.
func (i PushIntent) target() (branch string, rawForce bool) {
	if i.Refspec == "" {
		return i.Branch, false
	}

	parsed, err := ValidateRefspec(i.Refspec)
	if err != nil {
		return "", false
	}
	return parsed.GetDestinationBranch(), parsed.Force
}

func (p *PushPolicy) isProtected(branch string) bool {
	if branch == "" {
		return false
	}
	for _, pattern := range p.Protected {
		if matchBranchPattern(branch, pattern) {
			return true
		}
	}
	return false
}

// mode returns the configured force mode, treating an unset value as the
// default rather than as "allow": a policy that exists should not be weaker
// than one assembled from scratch.
func (p *PushPolicy) mode() ForceMode {
	if p.ForceMode == "" {
		return ForceModeLeaseOnly
	}
	return p.ForceMode
}
