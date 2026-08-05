// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package reposync

import "context"

// Executor runs a Plan with concurrency, retries, and strategies.
type Executor interface {
	Execute(ctx context.Context, plan Plan, opts RunOptions, sink ProgressSink, store StateStore) (ExecutionResult, error)
}

// ExecutionResult captures aggregated outcomes from a run.
type ExecutionResult struct {
	Succeeded []ActionResult
	Failed    []ActionResult
	Skipped   []ActionResult
}

// ActionResult is a per-repo outcome.
type ActionResult struct {
	Action     Action
	Message    string
	Error      error
	PostStatus *PostSyncStatus
}

// PostSyncStatus captures lightweight git status collected after a successful sync.
type PostSyncStatus struct {
	Branch       string
	AheadBy      int
	BehindBy     int
	IsDirty      bool
	HasConflicts bool

	// StatusErr records a failure to read the working tree. IsDirty and
	// HasConflicts are only meaningful when it is nil — their zero values are
	// indistinguishable from a clean, conflict-free repository, so a renderer
	// that ignores this field reports an unreadable repo as a healthy one.
	// Same reasoning as WorkTreeUnknown in diagnostic.go.
	StatusErr error
}

// Strategy defines how updates are performed.
type Strategy string

// Strategy values control how an existing repository is brought up to date.
const (
	StrategyReset  Strategy = "reset"
	StrategyPull   Strategy = "pull"
	StrategyFetch  Strategy = "fetch"
	StrategyRebase Strategy = "rebase"
	StrategyClone  Strategy = "clone"
)
