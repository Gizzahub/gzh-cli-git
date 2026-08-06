// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package handoff

// Reason identifies why a repository is not ready to be left behind.
type Reason string

const (
	// ReasonUncommitted marks changes that exist only in the working tree.
	ReasonUncommitted Reason = "uncommitted"
	// ReasonUnpushed marks commits that exist only in the local repository.
	ReasonUnpushed Reason = "unpushed"
	// ReasonStashed marks stash entries, which are never transferred by git.
	ReasonStashed Reason = "stashed"
	// ReasonStranded marks a stash old enough to have outlived the task that
	// created it. It is the same blocker as ReasonStashed with a worse prognosis:
	// nobody is coming back for it on their own.
	ReasonStranded Reason = "stranded"
	// ReasonConflict marks unresolved merge conflicts.
	ReasonConflict Reason = "conflict"
	// ReasonInProgress marks an interrupted rebase or merge.
	ReasonInProgress Reason = "in-progress"
	// ReasonDetached marks a detached HEAD, where new commits belong to no branch.
	ReasonDetached Reason = "detached-head"
	// ReasonNoRemote marks a repository with nowhere to push.
	ReasonNoRemote Reason = "no-remote"
	// ReasonNoUpstream marks a branch that has no upstream to push to yet.
	ReasonNoUpstream Reason = "no-upstream"
	// ReasonError marks a repository whose state could not be read.
	ReasonError Reason = "error"
)

// Blocker is one reason work in a repository would not survive the move.
type Blocker struct {
	Reason Reason `json:"reason"`
	Detail string `json:"detail"`
	// AutoFixable reports whether "handoff end" clears this blocker on its own.
	// It is false whenever clearing it needs a decision only the person at this
	// machine can make.
	AutoFixable bool `json:"auto_fixable"`
}

// RepoAssessment is the verdict for a single repository.
type RepoAssessment struct {
	Path         string    `json:"path"`
	RelativePath string    `json:"relative_path"`
	Branch       string    `json:"branch,omitempty"`
	Blockers     []Blocker `json:"blockers,omitempty"`
}

// Ready reports whether nothing in this repository exists only locally.
func (r RepoAssessment) Ready() bool { return len(r.Blockers) == 0 }

// AutoFixable reports whether "handoff end" would clear every blocker here.
func (r RepoAssessment) AutoFixable() bool {
	if r.Ready() {
		return false
	}
	for _, b := range r.Blockers {
		if !b.AutoFixable {
			return false
		}
	}
	return true
}

// FindingKind classifies why a file should not be swept into a checkpoint
// commit without someone looking at it first.
type FindingKind string

const (
	// FindingSecret marks a file that looks like it holds a credential.
	FindingSecret FindingKind = "secret"
	// FindingLargeFile marks a file too big to be source.
	FindingLargeFile FindingKind = "large-file"
	// FindingArtifact marks generated output that .gitignore does not cover.
	FindingArtifact FindingKind = "artifact"
)

// Finding is one file the guard refuses to commit unattended.
type Finding struct {
	Kind   FindingKind `json:"kind"`
	File   string      `json:"file"`
	Detail string      `json:"detail"`
}

// Verdict summarizes an assessment across all repositories.
type Verdict string

const (
	// VerdictReady means nothing exists only on this machine.
	VerdictReady Verdict = "ready"
	// VerdictFixable means "handoff end" would make everything ready.
	VerdictFixable Verdict = "fixable"
	// VerdictBlocked means at least one repository needs a decision made here.
	VerdictBlocked Verdict = "blocked"
)

// Assessment is the verdict across a scanned directory.
type Assessment struct {
	Verdict Verdict `json:"verdict"`
	// Repositories holds every scanned repository, ready ones included, so
	// callers can render a complete picture without a second scan.
	Repositories []RepoAssessment `json:"repositories"`
	TotalScanned int              `json:"total_scanned"`
}

// NotReady returns the repositories with at least one blocker, preserving the
// order they were scanned in.
func (a *Assessment) NotReady() []RepoAssessment {
	var out []RepoAssessment
	for _, r := range a.Repositories {
		if !r.Ready() {
			out = append(out, r)
		}
	}
	return out
}

// Blocked returns the repositories that "handoff end" cannot resolve.
func (a *Assessment) Blocked() []RepoAssessment {
	var out []RepoAssessment
	for _, r := range a.Repositories {
		if !r.Ready() && !r.AutoFixable() {
			out = append(out, r)
		}
	}
	return out
}

// ReasonCounts tallies blockers by reason across all repositories.
func (a *Assessment) ReasonCounts() map[Reason]int {
	counts := make(map[Reason]int)
	for _, r := range a.Repositories {
		for _, b := range r.Blockers {
			counts[b.Reason]++
		}
	}
	return counts
}
