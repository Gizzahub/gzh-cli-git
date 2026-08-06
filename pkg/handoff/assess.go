// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package handoff

import (
	"fmt"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// Assess classifies the results of a bulk status scan into a handoff verdict.
func Assess(results []repository.RepositoryStatusResult) *Assessment {
	assessment := &Assessment{
		Repositories: make([]RepoAssessment, 0, len(results)),
		TotalScanned: len(results),
	}

	for _, result := range results {
		assessment.Repositories = append(assessment.Repositories, assessRepository(result))
	}

	assessment.Verdict = verdictFor(assessment.Repositories)
	return assessment
}

func verdictFor(repos []RepoAssessment) Verdict {
	verdict := VerdictReady
	for _, r := range repos {
		switch {
		case r.Ready():
			continue
		case r.AutoFixable():
			verdict = VerdictFixable
		default:
			return VerdictBlocked
		}
	}
	return verdict
}

// assessRepository lists everything in one repository that exists only locally.
func assessRepository(r repository.RepositoryStatusResult) RepoAssessment {
	assessment := RepoAssessment{
		Path:         r.Path,
		RelativePath: r.RelativePath,
		Branch:       r.Branch,
	}

	if r.Status == repository.StatusError {
		assessment.Blockers = append(assessment.Blockers, Blocker{
			Reason: ReasonError,
			Detail: errorDetail(r),
		})
		// Nothing else in the result can be trusted once the scan failed.
		return assessment
	}

	// "handoff end" commits and pushes. Both halves have to be possible for
	// local-only work to count as auto-fixable.
	pushable := r.RemoteURL != "" && r.Branch != ""

	if r.RemoteURL == "" {
		assessment.Blockers = append(assessment.Blockers, Blocker{
			Reason: ReasonNoRemote,
			Detail: "no remote configured, so nothing here can leave this machine",
		})
	}

	if r.Branch == "" {
		assessment.Blockers = append(assessment.Blockers, Blocker{
			Reason: ReasonDetached,
			Detail: "HEAD is detached, so new commits belong to no branch",
		})
	}

	if len(r.ConflictFiles) > 0 {
		assessment.Blockers = append(assessment.Blockers, Blocker{
			Reason: ReasonConflict,
			Detail: fmt.Sprintf("%d file(s) with unresolved conflicts", len(r.ConflictFiles)),
		})
	}

	if detail := progressDetail(r); detail != "" {
		assessment.Blockers = append(assessment.Blockers, Blocker{
			Reason: ReasonInProgress,
			Detail: detail,
		})
	}

	if r.UncommittedFiles > 0 {
		assessment.Blockers = append(assessment.Blockers, Blocker{
			Reason:      ReasonUncommitted,
			Detail:      uncommittedDetail(r),
			AutoFixable: pushable,
		})
	}

	if r.CommitsAhead > 0 {
		assessment.Blockers = append(assessment.Blockers, Blocker{
			Reason:      ReasonUnpushed,
			Detail:      fmt.Sprintf("%d commit(s) are not on the remote", r.CommitsAhead),
			AutoFixable: pushable,
		})
	}

	// A branch with no upstream yet is only a problem once there is something
	// to push; push --set-upstream resolves it as part of "handoff end".
	if r.Status == repository.StatusNoUpstream {
		assessment.Blockers = append(assessment.Blockers, Blocker{
			Reason:      ReasonNoUpstream,
			Detail:      "branch has no upstream, so pushes have no default target",
			AutoFixable: pushable,
		})
	}

	// A stash is deliberately not auto-fixable. It is invisible to every other
	// machine, and turning one into a commit is a decision, not a cleanup step.
	if r.StashCount > 0 {
		assessment.Blockers = append(assessment.Blockers, Blocker{
			Reason: ReasonStashed,
			Detail: stashDetail(r.StashCount),
		})
	}

	return assessment
}

func progressDetail(r repository.RepositoryStatusResult) string {
	switch {
	case r.RebaseInProgress && r.MergeInProgress:
		return "a rebase and a merge are both interrupted"
	case r.RebaseInProgress:
		return "a rebase is interrupted partway through"
	case r.MergeInProgress:
		return "a merge is interrupted partway through"
	default:
		return ""
	}
}

func errorDetail(r repository.RepositoryStatusResult) string {
	switch {
	case r.Error != nil:
		return fmt.Sprintf("repository state could not be read: %v", r.Error)
	case r.Message != "":
		return r.Message
	default:
		return "repository state could not be read"
	}
}

// uncommittedDetail describes the working tree.
//
// UncommittedFiles counts every line git status prints, untracked files
// included, so the modified count is what remains after subtracting them.
func uncommittedDetail(r repository.RepositoryStatusResult) string {
	untracked := min(r.UntrackedFiles, r.UncommittedFiles)
	modified := r.UncommittedFiles - untracked

	switch {
	case modified > 0 && untracked > 0:
		return fmt.Sprintf("%d modified, %d untracked file(s) exist only here", modified, untracked)
	case untracked > 0:
		return fmt.Sprintf("%d untracked file(s) exist only here", untracked)
	default:
		return fmt.Sprintf("%d modified file(s) exist only here", modified)
	}
}

func stashDetail(count int) string {
	if count == 1 {
		return "1 stash entry never leaves this machine"
	}
	return fmt.Sprintf("%d stash entries never leave this machine", count)
}
