// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package reposync

import "context"

// Planner produces a Plan from desired repositories and options.
// Concrete implementation will live in future steps; this placeholder defines
// the interface surface for consumers and CLI wiring.
type Planner interface {
	Plan(ctx context.Context, req PlanRequest) (Plan, error)
}

// PlanRequest combines the desired repositories with planning-time options.
type PlanRequest struct {
	Input   PlanInput
	Options PlanOptions
}

// PlanOptions influence how a plan is produced (defaults, cleanup policies).
type PlanOptions struct {
	DefaultStrategy Strategy
	CleanupOrphans  bool
	Roots           []string // optional roots to detect orphan directories
}

// PlanInput captures desired repositories and optional context (e.g., host
// aliases, path rules). It is intentionally minimal for now; richer fields
// will be added in follow-up steps.
type PlanInput struct {
	Repos []RepoSpec
}

// Plan is the result of planning (e.g., clone/pull/fetch/delete actions).
// Details will be expanded as the orchestration logic lands.
type Plan struct {
	Actions []Action
}

// RepoSpec describes a repository to manage.
type RepoSpec struct {
	Name                 string
	Description          string // optional: human-readable description of the repository
	Provider             string
	CloneURL             string
	AdditionalRemotes    map[string]string // Additional git remotes (name: url), configured after clone
	TargetPath           string
	Branch               string // optional: branch to checkout after clone/update (empty = no checkout)
	StrictBranchCheckout bool   // if true, branch checkout failure causes action failure (default: false)
	Strategy             Strategy
	Enabled              *bool // if false, repo is excluded from sync (default: true, nil = true)
	AssumePresent        bool  // if true, planner treats repo as already present (skip clone check)

	// Auth contains authentication config for this repo's clone operation.
	// If empty, system defaults are used (git credential helper, ssh-agent).
	Auth AuthConfig
}

// IsEnabled returns true if the repo should be included in sync operations.
// Default is true (nil or true).
func (r RepoSpec) IsEnabled() bool {
	return r.Enabled == nil || *r.Enabled
}

// Action describes a single operation in a plan.
type Action struct {
	Repo      RepoSpec
	Type      ActionType
	Strategy  Strategy
	ReadOnly  bool // remote writes are forbidden by the declaring workspace
	Reason    string
	PlannedBy string
	Workspace string // workspace name for grouping display (empty = flat repositories)
}

// ActionType enumerates planned operations.
type ActionType string

// ActionType values represent the planned operation for each repository.
const (
	ActionClone  ActionType = "clone"
	ActionUpdate ActionType = "update"
	ActionSkip   ActionType = "skip"
	ActionDelete ActionType = "delete"
)
