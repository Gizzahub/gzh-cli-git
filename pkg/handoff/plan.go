// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package handoff

// hardBlockers are the reasons "handoff end" must not touch a repository at
// all. Each one means an automatic commit would either lose work or write
// something wrong into history, so the person at this machine has to act first.
var hardBlockers = map[Reason]bool{
	ReasonConflict:   true, // "git add -A" would mark conflicts resolved and commit the markers
	ReasonInProgress: true, // a commit mid-rebase lands on the wrong base
	ReasonDetached:   true, // the commit would belong to no branch and no push could carry it
	ReasonNoRemote:   true, // there is nowhere for the work to go
	ReasonError:      true, // the repository state could not be read, so nothing about it is known
}

// movable are the reasons "handoff end" exists to clear.
var movable = map[Reason]bool{
	ReasonUncommitted: true,
	ReasonUnpushed:    true,
	ReasonNoUpstream:  true,
}

// Plan is the split of a scanned workspace into what "handoff end" will move
// and what it must leave for a person.
type Plan struct {
	// Checkpoint holds the repositories that will be committed and pushed.
	Checkpoint []RepoAssessment `json:"checkpoint"`
	// Skipped holds the repositories with a blocker no automatic step can clear.
	Skipped []RepoAssessment `json:"skipped,omitempty"`
}

// Empty reports whether there is nothing for "handoff end" to do.
func (p Plan) Empty() bool { return len(p.Checkpoint) == 0 }

// PlanCheckpoint decides, per repository, whether "handoff end" may act.
//
// A repository qualifies when it has work that a commit and a push would move
// and nothing that makes those steps unsafe. A stash does not disqualify it:
// the stash is untouched either way, and the committed work still deserves to
// reach the remote.
func PlanCheckpoint(a *Assessment) Plan {
	var plan Plan

	for _, repo := range a.Repositories {
		if repo.Ready() {
			continue
		}

		if _, blocked := FirstHardBlocker(repo); blocked {
			plan.Skipped = append(plan.Skipped, repo)
			continue
		}

		if hasMovableWork(repo) {
			plan.Checkpoint = append(plan.Checkpoint, repo)
		}
	}

	return plan
}

// startBlockers are the reasons "handoff start" must not rebase a repository.
// The set overlaps with hardBlockers but is not the same: uncommitted work
// stops an arrival (a rebase would replay commits over it) while it is exactly
// what a departure exists to clear.
var startBlockers = map[Reason]bool{
	ReasonConflict:    true, // the previous session left a conflict to resolve
	ReasonInProgress:  true, // an interrupted rebase has to be finished or aborted first
	ReasonDetached:    true, // there is no branch to replay onto
	ReasonNoRemote:    true, // there is nothing to pull from
	ReasonNoUpstream:  true, // the branch has no remote counterpart yet
	ReasonError:       true, // the repository state could not be read
	ReasonUncommitted: true, // rebasing over uncommitted work risks losing it
}

// StartPlan is the split of a scanned workspace into what "handoff start" will
// bring up to date and what it must leave for a person.
type StartPlan struct {
	// Update holds the repositories that will be pulled with a rebase.
	Update []RepoAssessment `json:"update"`
	// Skipped holds the repositories a rebase would endanger.
	Skipped []RepoAssessment `json:"skipped,omitempty"`
}

// PlanStart decides, per repository, whether "handoff start" may rebase it.
//
// Unpushed commits do not disqualify a repository: replaying them onto the
// updated remote branch is the point of arriving with a rebase. A stash does
// not either, since a rebase never touches one.
func PlanStart(a *Assessment) StartPlan {
	var plan StartPlan

	for _, repo := range a.Repositories {
		if _, blocked := FirstStartBlocker(repo); blocked {
			plan.Skipped = append(plan.Skipped, repo)
			continue
		}
		plan.Update = append(plan.Update, repo)
	}

	return plan
}

// FirstHardBlocker returns the blocker that disqualifies a repository from an
// automatic checkpoint, in the order the blockers were recorded.
func FirstHardBlocker(r RepoAssessment) (Blocker, bool) {
	return firstBlocker(r, hardBlockers)
}

// FirstStartBlocker returns the blocker that disqualifies a repository from an
// automatic rebase on arrival.
func FirstStartBlocker(r RepoAssessment) (Blocker, bool) {
	return firstBlocker(r, startBlockers)
}

func firstBlocker(r RepoAssessment, set map[Reason]bool) (Blocker, bool) {
	for _, b := range r.Blockers {
		if set[b.Reason] {
			return b, true
		}
	}
	return Blocker{}, false
}

func hasMovableWork(r RepoAssessment) bool {
	for _, b := range r.Blockers {
		if movable[b.Reason] {
			return true
		}
	}
	return false
}

// Paths returns the absolute paths of the repositories in a set, in scan order.
func Paths(repos []RepoAssessment) []string {
	paths := make([]string, 0, len(repos))
	for _, r := range repos {
		paths = append(paths, r.Path)
	}
	return paths
}
