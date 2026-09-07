// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package branch

import (
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// Branch represents a Git branch with metadata.
type Branch struct {
	Name       string     // Branch name
	Ref        string     // Full ref (refs/heads/...)
	SHA        string     // Commit SHA
	IsHead     bool       // Currently checked out
	IsMerged   bool       // Fully merged into base branch
	IsRemote   bool       // Remote branch
	Upstream   string     // Upstream branch (if set)
	AheadBy    int        // Commits ahead of upstream
	BehindBy   int        // Commits behind upstream
	LastCommit *Commit    // Last commit on this branch
	CreatedAt  *time.Time // Creation time (if available)
	UpdatedAt  *time.Time // Last update time
}

// Commit represents a Git commit with metadata.
type Commit struct {
	SHA      string
	Author   string
	Email    string
	Date     time.Time
	Message  string
	ShortMsg string // First line of message
}

// CreateOptions configures branch creation.
type CreateOptions struct {
	Name     string // Branch name (required)
	StartRef string // Starting ref (default: HEAD)
	Checkout bool   // Checkout after creation
	Track    bool   // Set upstream tracking
	Force    bool   // Overwrite existing branch
	Validate bool   // Validate naming conventions (default: true)
}

// DeleteOptions configures branch deletion.
type DeleteOptions struct {
	Name    string // Branch name (required)
	Remote  bool   // Delete remote branch
	Force   bool   // Force delete (even if unmerged)
	DryRun  bool   // Preview deletion
	Confirm bool   // Skip confirmation prompt
}

// ListOptions configures branch listing.
type ListOptions struct {
	All      bool   // Include remote branches
	Merged   bool   // Only merged branches
	Unmerged bool   // Only unmerged branches
	Pattern  string // Name pattern filter
	Sort     SortBy // Sort order
	Limit    int    // Max results (0 = unlimited)
	Remote   string // Specific remote (empty = all)
}

// SortBy defines branch sorting order.
type SortBy string

// SortBy values for ordering branch lists.
const (
	SortByName     SortBy = "name"     // Alphabetical by name
	SortByDate     SortBy = "date"     // Most recent first
	SortByAuthor   SortBy = "author"   // Alphabetical by author
	SortByUpstream SortBy = "upstream" // Group by upstream
)

// BranchType represents branch purpose/category.
type BranchType string

// BranchType values classify a branch by its purpose.
const (
	BranchTypeFeature    BranchType = "feature"    // feature/*
	BranchTypeFix        BranchType = "fix"        // fix/*
	BranchTypeHotfix     BranchType = "hotfix"     // hotfix/*
	BranchTypeRelease    BranchType = "release"    // release/*
	BranchTypeExperiment BranchType = "experiment" // experiment/*
	BranchTypeOther      BranchType = "other"      // Unclassified
)

// ProtectedBranches are branches that require --force to delete. It aliases the
// canonical list in pkg/repository so both cleanup paths share one source.
var ProtectedBranches = repository.ProtectedBranches

// IsProtected checks if a branch name matches protected patterns. It delegates
// to pkg/repository, the single source of truth for protected-branch judgment
// (pkg/repository cannot import pkg/branch — the dependency runs branch →
// repository — so ownership lives in the lower package).
func IsProtected(name string) bool {
	return repository.IsProtected(name)
}

// matchPattern checks if name matches pattern (supports * wildcard).
func matchPattern(name, pattern string) bool {
	// Simple wildcard matching
	if pattern == name {
		return true
	}

	// Handle trailing wildcard (e.g., "release/*")
	if pattern != "" && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(name) >= len(prefix) && name[:len(prefix)] == prefix
	}

	return false
}

// InferType infers branch type from name.
func InferType(name string) BranchType {
	switch {
	case matchPattern(name, "feature/*"):
		return BranchTypeFeature
	case matchPattern(name, "fix/*"):
		return BranchTypeFix
	case matchPattern(name, "hotfix/*"):
		return BranchTypeHotfix
	case matchPattern(name, "release/*"):
		return BranchTypeRelease
	case matchPattern(name, "experiment/*"):
		return BranchTypeExperiment
	default:
		return BranchTypeOther
	}
}

// Worktree represents a Git worktree.
type Worktree struct {
	Path       string // Worktree path
	Branch     string // Branch name
	Ref        string // Full ref (HEAD or commit SHA)
	IsMain     bool   // Is the main worktree
	IsLocked   bool   // Is locked
	IsPrunable bool   // Can be pruned
	IsBare     bool   // Is bare repository
	IsDetached bool   // Is detached HEAD
}

// AddOptions configures worktree addition.
type AddOptions struct {
	Path         string // Worktree path (required)
	Branch       string // Branch name (required)
	CreateBranch bool   // Create new branch
	Force        bool   // Overwrite existing
	Detach       bool   // Detached HEAD
	Checkout     string // Specific commit to checkout
}

// RemoveOptions configures worktree removal.
type RemoveOptions struct {
	Path  string // Worktree path (required)
	Force bool   // Force removal (even with uncommitted changes)
}

// AnalyzeOptions configures branch cleanup analysis.
type AnalyzeOptions struct {
	IncludeMerged     bool          // Include fully merged branches
	IncludeStale      bool          // Include stale branches (no activity)
	StaleThreshold    time.Duration // Threshold for stale (default: 30 days)
	IncludeRemote     bool          // Include remote branches
	IncludeGone       bool          // Include local branches whose upstream is gone
	IncludeSuperseded bool          // Include unmerged bot remotes whose version already landed
	Exclude           []string      // Patterns to exclude
	BaseBranch        string        // Base branch for merge detection (default: main/master)
	BotsOnly          bool          // Restrict candidates to Dependabot/Renovate/github-actions prefixes

	// IncludeNonCanonical enables retirement of branches that duplicate the
	// declared canonical branch. It requires CanonicalBranch; without a
	// declaration there is nothing to measure "non-canonical" against and the
	// classification yields no candidates.
	IncludeNonCanonical bool

	// CanonicalBranch is the repository's declared integration branch, read from
	// .gz-git.yaml. It is never guessed: detectBaseBranch's name heuristics
	// answer "which branch looks like a trunk", not "which trunk did this
	// repository declare", and only the latter can justify retiring the other.
	CanonicalBranch string

	// TaskPatterns is the declared task-branch allow-list (.gz-git.yaml
	// taskPattern). Branches matching it belong to the reclaim path and are
	// never retired here, even when they are ancestors of the canonical branch.
	TaskPatterns []string

	// CanonicalRemote names the one remote the declaration speaks for. A
	// .gz-git.yaml describes its own repository, never a third party's: in a
	// fork checkout that also tracks `upstream`, `upstream/master` is an
	// ancestor of `upstream/develop` and would otherwise classify as
	// non-canonical, aiming a delete at the upstream project on the strength of
	// a declaration that never mentioned it. Remote candidates on any other
	// remote are skipped. Empty means origin.
	CanonicalRemote string
}

// ExecuteOptions configures branch cleanup execution.
type ExecuteOptions struct {
	DryRun  bool     // Preview only, don't delete
	Force   bool     // Force delete unmerged branches
	Remote  bool     // Also delete remote branches
	Confirm bool     // Skip confirmation prompts
	Exclude []string // Additional patterns to exclude

	// CanonicalBranch re-arms the non-canonical gate inside Execute. Analyze
	// already screened report.NonCanonical, but CleanupReport is a public type
	// and callers may hand-assemble one, so Execute re-verifies ancestry against
	// this branch before it will bypass built-in protection for any candidate.
	// Empty means no candidate may bypass it.
	CanonicalBranch string

	// CanonicalRemote is the AnalyzeOptions field of the same name, re-armed
	// here for the same reason CanonicalBranch is: Execute re-verifies rather
	// than trusting a report a caller may have hand-assembled. Empty means
	// origin.
	CanonicalRemote string
}

// ExecuteResult reports what a cleanup run actually did.
//
// Execute deletes each branch independently and does not stop at the first
// failure, so neither "it returned an error" nor "it returned nil" describes the
// outcome. Deleted, Failed, and Skipped together do.
type ExecuteResult struct {
	Deleted []string        // Branches removed
	Failed  []DeleteFailure // Branches that could not be removed, and why
	Skipped []string        // Branches not attempted (protected / excluded)
}

// DeleteFailure records a branch that Execute could not delete.
type DeleteFailure struct {
	Branch string
	Err    error
}

// CleanupReport summarizes branches eligible for cleanup.
type CleanupReport struct {
	Merged []*Branch // Fully merged branches
	Stale  []*Branch // Stale branches

	// Orphaned holds *local* branches whose upstream no longer exists — what git
	// marks `[gone]` and what the CLI calls `--gone`. These are refs under
	// refs/heads, so deleting one removes only the local ref; the branch it used
	// to track is already gone from the remote.
	Orphaned []*Branch

	// Superseded holds unmerged remote bot branches whose version target is
	// already satisfied on the base. Comparison is versions, not ancestry.
	Superseded []*Branch

	// NonCanonical holds branches that duplicate the declared canonical branch:
	// they carry no commit the canonical branch lacks, and they are neither the
	// canonical branch itself nor a declared task branch. This is the one bucket
	// permitted to contain built-in protected names (master, develop), because
	// the question it answers is which trunk this repository declared — not
	// whether the name looks like a trunk.
	NonCanonical []*Branch

	Protected []*Branch // Protected (won't delete)
	Total     int       // Total branches analyzed
}

// CleanupStrategy defines cleanup approach.
type CleanupStrategy string

// CleanupStrategy values select which categories of branches to remove.
const (
	StrategyMerged   CleanupStrategy = "merged"   // Only merged branches
	StrategyStale    CleanupStrategy = "stale"    // Only stale branches
	StrategyOrphaned CleanupStrategy = "orphaned" // Only orphaned branches
	StrategyAll      CleanupStrategy = "all"      // All eligible branches
)
