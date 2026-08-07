// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package gitsettings

// Scope selects which git configuration file is inspected or written.
type Scope string

const (
	// ScopeGlobal targets the per-user config (~/.gitconfig).
	ScopeGlobal Scope = "global"
	// ScopeLocal targets the per-repository config (.git/config).
	ScopeLocal Scope = "local"
)

// Flag returns the git command-line flag for the scope.
func (s Scope) Flag() string {
	if s == ScopeLocal {
		return "--local"
	}
	return "--global"
}

// State describes how a repository's current value relates to the
// recommended one.
type State string

const (
	// StateOK means the current value already matches the recommendation.
	StateOK State = "ok"
	// StateUnset means the setting has no value in the inspected scope.
	StateUnset State = "unset"
	// StateMismatch means the setting is present but holds a different value.
	StateMismatch State = "mismatch"
	// StateUnsupported means the installed git is older than the setting requires.
	StateUnsupported State = "unsupported"
)

// Setting is a single recommended git configuration key.
type Setting struct {
	// Key is the git config key, e.g. "pull.rebase".
	Key string `json:"key"`
	// Want is the recommended value.
	Want string `json:"want"`
	// Why explains, in one line, the failure mode the setting prevents.
	Why string `json:"why"`
	// MinGit is the minimum git version required, empty when any version works.
	MinGit string `json:"minGit,omitempty"`
}

// Status pairs a recommended setting with the value found on this machine.
type Status struct {
	// Setting is embedded so its fields are flattened into the JSON object.
	Setting
	// Current is the value read from the inspected scope, empty when unset.
	Current string `json:"current,omitempty"`
	// State is the comparison outcome.
	State State `json:"state"`
}

// NeedsChange reports whether applying the recommendation would modify config.
func (s Status) NeedsChange() bool {
	return s.State == StateUnset || s.State == StateMismatch
}

// Report is the outcome of inspecting a scope.
type Report struct {
	Scope      Scope    `json:"scope"`
	GitVersion string   `json:"gitVersion"`
	Statuses   []Status `json:"statuses"`
}

// Pending returns the statuses that would be changed by Apply.
func (r *Report) Pending() []Status {
	var pending []Status
	for _, s := range r.Statuses {
		if s.NeedsChange() {
			pending = append(pending, s)
		}
	}
	return pending
}

// Unsupported returns the statuses skipped because git is too old.
func (r *Report) Unsupported() []Status {
	var out []Status
	for _, s := range r.Statuses {
		if s.State == StateUnsupported {
			out = append(out, s)
		}
	}
	return out
}
