// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package reposync

import (
	"time"
)

// HealthStatus represents the overall health of a repository.
type HealthStatus string

const (
	// HealthHealthy indicates the repository is in good state (up-to-date, clean).
	HealthHealthy HealthStatus = "healthy"

	// HealthWarning indicates the repository needs attention (diverged, can be resolved).
	HealthWarning HealthStatus = "warning"

	// HealthError indicates the repository has serious issues (conflicts, dirty + behind).
	HealthError HealthStatus = "error"

	// HealthUnreachable indicates the repository couldn't be checked (network timeout, invalid repo).
	HealthUnreachable HealthStatus = "unreachable"
)

// DivergenceType classifies how local and remote branches differ.
type DivergenceType string

const (
	// DivergenceNone means local and remote are identical.
	DivergenceNone DivergenceType = "none"

	// DivergenceFastForward means local is behind remote, can fast-forward.
	DivergenceFastForward DivergenceType = "fast-forward"

	// DivergenceDiverged means local and remote have diverged, requires merge/rebase.
	DivergenceDiverged DivergenceType = "diverged"

	// DivergenceAhead means local is ahead of remote, can push.
	DivergenceAhead DivergenceType = "ahead"

	// DivergenceConflict means there are merge conflicts or incompatible states.
	DivergenceConflict DivergenceType = "conflict"

	// DivergenceNoUpstream means no upstream branch is configured.
	DivergenceNoUpstream DivergenceType = "no-upstream"
)

// NetworkStatus represents the network connectivity status.
type NetworkStatus string

const (
	// NetworkOK means remote fetch succeeded.
	NetworkOK NetworkStatus = "ok"

	// NetworkTimeout means remote fetch timed out.
	NetworkTimeout NetworkStatus = "timeout"

	// NetworkUnreachable means remote is unreachable (DNS, connection refused, etc).
	NetworkUnreachable NetworkStatus = "unreachable"

	// NetworkAuthFailed means authentication failed.
	NetworkAuthFailed NetworkStatus = "auth-failed"
)

// WorkTreeStatus represents the working tree state.
type WorkTreeStatus string

const (
	// WorkTreeClean means no uncommitted changes.
	WorkTreeClean WorkTreeStatus = "clean"

	// WorkTreeDirty means there are uncommitted changes.
	WorkTreeDirty WorkTreeStatus = "dirty"

	// WorkTreeConflict means there are merge/rebase conflicts.
	WorkTreeConflict WorkTreeStatus = "conflict"

	// WorkTreeRebaseInProgress means a rebase is in progress.
	WorkTreeRebaseInProgress WorkTreeStatus = "rebase-in-progress"

	// WorkTreeMergeInProgress means a merge is in progress.
	WorkTreeMergeInProgress WorkTreeStatus = "merge-in-progress"

	// WorkTreeUnknown means the working tree could not be read (for example a
	// corrupt .git/index). It is deliberately distinct from WorkTreeClean: the
	// question was asked and went unanswered, which is not the same as an answer
	// of "no changes". The empty zero value still means "not checked", which is
	// what CheckWorkTree=false leaves behind.
	WorkTreeUnknown WorkTreeStatus = "unknown"
)

// RepoHealth represents the diagnostic result for a single repository.
type RepoHealth struct {
	// Repo is the repository descriptor.
	Repo RepoSpec

	// HealthStatus is the overall health classification.
	HealthStatus HealthStatus

	// NetworkStatus indicates remote connectivity.
	NetworkStatus NetworkStatus

	// DivergenceType classifies local vs remote state.
	DivergenceType DivergenceType

	// WorkTreeStatus indicates working tree state.
	WorkTreeStatus WorkTreeStatus

	// CurrentBranch is the active branch name.
	CurrentBranch string

	// UpstreamBranch is the tracked upstream branch (e.g., "origin/main").
	UpstreamBranch string

	// AheadBy is commits ahead of upstream.
	AheadBy int

	// BehindBy is commits behind upstream.
	BehindBy int

	// ModifiedFiles is count of modified files.
	ModifiedFiles int

	// UntrackedFiles is count of untracked files.
	UntrackedFiles int

	// ConflictFiles is count of files with conflicts.
	ConflictFiles int

	// Recommendation provides actionable guidance.
	Recommendation string

	// Error contains error details if health check failed.
	Error error

	// Duration is how long the health check took.
	Duration time.Duration

	// FetchDuration is how long remote fetch took (if performed).
	FetchDuration time.Duration
}

// HealthReport aggregates health check results for multiple repositories.
type HealthReport struct {
	// Results contains per-repository health status.
	Results []RepoHealth

	// Summary provides counts by health status.
	Summary HealthSummary

	// TotalDuration is the total time for all checks.
	TotalDuration time.Duration

	// CheckedAt is when the health check was performed.
	CheckedAt time.Time
}

// HealthSummary provides aggregate statistics.
type HealthSummary struct {
	// Healthy is count of healthy repositories.
	Healthy int

	// Warning is count of repositories with warnings.
	Warning int

	// Error is count of repositories with errors.
	Error int

	// Unreachable is count of unreachable repositories.
	Unreachable int

	// Total is total number of repositories checked.
	Total int
}

// DiagnosticOptions configures health check behavior.
type DiagnosticOptions struct {
	// SkipFetch skips remote fetch before checking divergence.
	// This is faster but may give stale results.
	SkipFetch bool

	// FetchTimeout is max time to wait for remote fetch (per repo).
	// Default: 30s.
	FetchTimeout time.Duration

	// Parallel is number of concurrent health checks.
	// Default: 4.
	Parallel int

	// CheckWorkTree enables working tree status checks.
	// Default: true.
	CheckWorkTree bool

	// IncludeRecommendations generates actionable guidance.
	// Default: true.
	IncludeRecommendations bool

	// Progress is an optional progress callback.
	Progress DiagnosticProgress
}

// DiagnosticProgress receives progress notifications during health checks.
type DiagnosticProgress interface {
	OnRepoStart(repo RepoSpec)
	OnRepoComplete(health RepoHealth)
}

// DefaultDiagnosticOptions returns sensible defaults.
func DefaultDiagnosticOptions() DiagnosticOptions {
	return DiagnosticOptions{
		SkipFetch:              false,
		FetchTimeout:           30 * time.Second,
		Parallel:               4,
		CheckWorkTree:          true,
		IncludeRecommendations: true,
	}
}
