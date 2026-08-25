// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"fmt"
	"strings"
)

// EvaluateRepo runs the finding catalog against one repository.
//
// Blockers short-circuit: when the working state is untrustworthy the function
// returns that single finding with Complete false, rather than appending it to
// a list of numbers it just declared unreliable. An agent that sees
// audit_complete=false knows to fix the blocker and re-run, which is a safer
// instruction than a long list of findings derived from a half-finished rebase.
func EvaluateRepo(in AuditInput) AuditRepo {
	out := AuditRepo{
		Name:     in.Name,
		Path:     in.Path,
		Complete: true,
		Base: AuditBase{
			Name:   in.Base.Name,
			Source: in.Base.Source,
			Ahead:  in.Base.Ahead,
			Behind: in.Base.Behind,
		},
		Findings: []Finding{},
	}
	if out.Base.Source == "" {
		out.Base.Source = baseSourceNone
	}

	st := in.Status
	if st == nil {
		out.Complete = false
		out.IncompleteReason = CodeScanError
		out.Findings = append(out.Findings, Finding{
			Code:     CodeScanError,
			Severity: SeverityBlocker,
			Message:  "no status was collected for this repository",
			Fix:      &Remediation{Action: ActionResolveManually, Reversible: true},
		})
		return out
	}

	out.Branch = st.Branch
	out.Upstream = st.Upstream
	out.Worktrees = in.Worktrees

	if blocker, ok := evaluateBlockers(in, st); ok {
		out.Complete = false
		out.IncompleteReason = blocker.Code
		out.Findings = append(out.Findings, blocker)
		applyAutofixPolicy(&out.Findings[0], in.AutofixPolicy)
		return out
	}

	out.Findings = append(out.Findings, evaluateBranchPosition(in, st)...)
	out.Findings = append(out.Findings, evaluateLocalState(in, st)...)

	for i := range out.Findings {
		applyAutofixPolicy(&out.Findings[i], in.AutofixPolicy)
	}
	return out
}

// evaluateBlockers reports the first state that invalidates the rest of the
// audit, if any.
func evaluateBlockers(in AuditInput, st *RepositoryStatusResult) (Finding, bool) {
	switch {
	case st.Error != nil:
		return Finding{
			Code:     CodeScanError,
			Severity: SeverityBlocker,
			Message:  "repository could not be read: " + st.Error.Error(),
			Evidence: map[string]any{"error": st.Error.Error()},
			Fix:      &Remediation{Action: ActionResolveManually, Reversible: true},
		}, true

	case len(st.ConflictFiles) > 0:
		return Finding{
			Code:     CodeConflicts,
			Severity: SeverityBlocker,
			Message:  fmt.Sprintf("%d file(s) have unresolved conflicts", len(st.ConflictFiles)),
			Evidence: map[string]any{"files": st.ConflictFiles},
			Fix: &Remediation{
				Action:     ActionResolveManually,
				Reversible: true,
				Note:       "conflict resolution requires judgment about intent; no automatic choice is correct",
			},
		}, true

	case st.RebaseInProgress:
		return Finding{
			Code:     CodeRebaseInProgress,
			Severity: SeverityBlocker,
			Message:  "a rebase is in progress; HEAD is a transient rebase artifact",
			Fix: &Remediation{
				Action:     ActionResolveManually,
				Command:    []string{"git", "rebase", "--continue"},
				Reversible: true,
				Note:       "or --abort; divergence counts are meaningless until this finishes",
			},
		}, true

	case st.MergeInProgress:
		return Finding{
			Code:     CodeMergeInProgress,
			Severity: SeverityBlocker,
			Message:  "a merge is in progress",
			Fix: &Remediation{
				Action:     ActionResolveManually,
				Command:    []string{"git", "merge", "--continue"},
				Reversible: true,
				Note:       "or --abort",
			},
		}, true

	case in.EnrichErr != nil:
		return Finding{
			Code:     CodeScanError,
			Severity: SeverityBlocker,
			Message:  "base branch and worktree facts could not be collected: " + in.EnrichErr.Error(),
			Evidence: map[string]any{"error": in.EnrichErr.Error()},
			Fix:      &Remediation{Action: ActionResolveManually, Reversible: true},
		}, true
	}

	return Finding{}, false
}

// evaluateBranchPosition covers where HEAD sits relative to its upstream and
// its base.
func evaluateBranchPosition(in AuditInput, st *RepositoryStatusResult) []Finding {
	var findings []Finding

	if st.Branch == "" {
		findings = append(findings, Finding{
			Code:     CodeDetachedHead,
			Severity: SeverityWarn,
			Message:  "HEAD is detached; commits made here are not on any branch",
			Evidence: map[string]any{"head": st.HeadSHA},
			Fix: &Remediation{
				Action:     ActionCheckoutBranch,
				Reversible: true,
				Note:       "checking out a branch discards nothing, but commits made while detached become unreachable",
			},
		})
	}

	findings = append(findings, evaluateUpstream(in, st)...)
	findings = append(findings, evaluateBase(in, st)...)

	// Branches fully contained in the base are finished work still occupying a
	// name. Reported only with ancestry evidence, since "looks merged" inferred
	// from divergence counts is a different and much weaker claim.
	deletable, held := partitionMerged(in.MergedBranches, in.Worktrees)

	if len(deletable) > 0 {
		findings = append(findings, Finding{
			Code:     CodeMergedNotReclaimed,
			Severity: SeverityInfo,
			Message: fmt.Sprintf("%d branch(es) are fully merged into %s and can be reclaimed",
				len(deletable), in.Base.Name),
			Evidence: map[string]any{
				"branches":    deletable,
				"base":        in.Base.Name,
				"base_source": in.Base.Source,
				"verified_by": "git merge-base --is-ancestor",
			},
			Fix: &Remediation{
				Action:     ActionDeleteBranch,
				Command:    append([]string{"git", "branch", "-d"}, deletable...),
				Reversible: true,
				Note:       "-d (not -D) refuses anything not actually merged, so the command re-verifies before deleting",
			},
		})
	}

	findings = append(findings, evaluateWorktreeReclaim(held, in.Base.Name)...)
	findings = append(findings, evaluateRemoteBots(in)...)

	return findings
}

// evaluateRemoteBots reports leftover Dependabot/Renovate/github-actions
// remote-tracking refs. One finding per code per repo, not per branch.
func evaluateRemoteBots(in AuditInput) []Finding {
	var findings []Finding

	if len(in.RemoteBotMerged) > 0 {
		findings = append(findings, Finding{
			Code:     CodeRemoteBotReclaimable,
			Severity: SeverityInfo,
			Message: fmt.Sprintf("%d remote bot branch(es) are fully merged into %s and can be deleted",
				len(in.RemoteBotMerged), in.Base.Name),
			Evidence: map[string]any{
				"branches":    in.RemoteBotMerged,
				"base":        in.Base.Name,
				"verified_by": "git merge-base --is-ancestor",
			},
			Fix: &Remediation{
				Action:     ActionDeleteRemoteBranch,
				Command:    []string{"gz-git", "cleanup", "branch", "--bots", "--merged", "--remote", "--force", "--yes"},
				Reversible: false,
				Note:       "irreversible; the cleanup command leases the classified SHA so a newer remote tip is not deleted. Do not raw git push --delete",
			},
		})
	}

	if len(in.RemoteBotSuperseded) > 0 {
		findings = append(findings, Finding{
			Code:     CodeRemoteBotSuperseded,
			Severity: SeverityInfo,
			Message: fmt.Sprintf("%d remote bot branch(es) are superseded by versions already on %s",
				len(in.RemoteBotSuperseded), in.Base.Name),
			Evidence: map[string]any{
				"branches":    in.RemoteBotSuperseded,
				"base":        in.Base.Name,
				"verified_by": "version comparison",
			},
			Fix: &Remediation{
				Action:     ActionDeleteRemoteBranch,
				Command:    []string{"gz-git", "cleanup", "branch", "--bots", "--superseded", "--remote", "--force", "--yes"},
				Reversible: false,
				Note:       "irreversible; the cleanup command leases the classified SHA so a newer remote tip is not deleted. Do not raw git push --delete",
			},
		})
	}

	if len(in.RemoteBotPending) > 0 {
		findings = append(findings, Finding{
			Code:     CodeRemoteBotPending,
			Severity: SeverityInfo,
			Message: fmt.Sprintf("%d unmerged remote bot branch(es) may be open pull requests",
				len(in.RemoteBotPending)),
			Evidence: map[string]any{
				"branches": in.RemoteBotPending,
				"base":     in.Base.Name,
			},
			Fix: &Remediation{
				Action:     ActionResolveManually,
				Reversible: true,
				Note:       "unmerged bot ref; may be an open PR — do not delete",
			},
		})
	}

	return findings
}

// evaluateUpstream classifies the relationship to the tracking branch. The
// states are mutually exclusive and each has a different repair, which is why
// they are separate codes rather than one "out of sync" finding with counts.
func evaluateUpstream(in AuditInput, st *RepositoryStatusResult) []Finding {
	if st.Branch == "" {
		return nil // detached HEAD is already reported; upstream is not meaningful
	}

	if st.Upstream == "" {
		return []Finding{{
			Code:     CodeNoUpstream,
			Severity: SeverityWarn,
			Message:  "branch tracks no upstream; its commits exist only on this machine",
			Evidence: map[string]any{"branch": st.Branch},
			Fix: &Remediation{
				Action:     ActionSetUpstream,
				Command:    []string{"git", "push", "-u", "origin", st.Branch},
				Reversible: false,
				Note:       "publishes the branch; others may fetch it once it exists",
			},
		}}
	}

	if in.UpstreamTargetsIntegration {
		remote := strings.TrimSpace(in.UpstreamRemote)
		if remote == "" {
			remote = "origin"
		}
		command := []string{"git", "push", "--set-upstream", remote, "HEAD:refs/heads/" + st.Branch}
		reversible := false
		note := "publishes only the same-named task ref; the explicit destination cannot update the integration branch"
		if in.TaskRemoteExists {
			command = []string{"git", "branch", "--set-upstream-to=" + remote + "/" + st.Branch, st.Branch}
			reversible = true
			note = "changes only local tracking configuration; review push and pull intent before applying"
		}
		return []Finding{{
			Code:     CodeUpstreamTargetsIntegration,
			Severity: SeverityWarn,
			Message:  fmt.Sprintf("task branch tracks integration branch %s", st.Upstream),
			Evidence: map[string]any{
				"branch":             st.Branch,
				"upstream":           st.Upstream,
				"integration":        in.IntegrationName,
				"integration_source": in.IntegrationSource,
			},
			Fix: &Remediation{
				Action:     ActionSetUpstream,
				Command:    command,
				Reversible: reversible,
				Note:       note,
			},
		}}
	}

	switch {
	case st.CommitsAhead > 0 && st.CommitsBehind > 0:
		return []Finding{{
			Code:     CodeUpstreamDiverged,
			Severity: SeverityWarn,
			Message: fmt.Sprintf("diverged from %s: %d ahead, %d behind",
				st.Upstream, st.CommitsAhead, st.CommitsBehind),
			Evidence: map[string]any{
				"upstream": st.Upstream,
				"ahead":    st.CommitsAhead,
				"behind":   st.CommitsBehind,
			},
			Fix: &Remediation{
				Action:     ActionResolveManually,
				Reversible: false,
				Note:       "rebase and merge produce different history here; a wrong pick rewrites published commits",
			},
		}}

	case st.CommitsAhead > 0:
		return []Finding{{
			Code:     CodeUnpushedCommits,
			Severity: SeverityWarn,
			Message:  fmt.Sprintf("%d commit(s) not pushed to %s", st.CommitsAhead, st.Upstream),
			Evidence: map[string]any{"upstream": st.Upstream, "ahead": st.CommitsAhead},
			Fix: &Remediation{
				Action:     ActionPush,
				Command:    []string{"git", "push"},
				Reversible: false,
				Note:       "fast-forward push; nothing is rewritten, but the commits become visible to others",
			},
		}}

	case st.CommitsBehind > 0:
		return []Finding{{
			Code:     CodeUpstreamBehind,
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("%d commit(s) behind %s", st.CommitsBehind, st.Upstream),
			Evidence: map[string]any{"upstream": st.Upstream, "behind": st.CommitsBehind},
			Fix: &Remediation{
				Action:     ActionPull,
				Command:    []string{"git", "pull", "--ff-only"},
				Reversible: true,
				Note:       "--ff-only cannot create a merge commit; it fails instead of inventing history",
			},
		}}
	}

	return nil
}

// evaluateBase covers the relationship to the integration branch, including
// the workflow rule that work does not belong directly on it.
func evaluateBase(in AuditInput, st *RepositoryStatusResult) []Finding {
	if in.Base.Name == "" {
		return []Finding{{
			Code:     CodeBaseUnresolved,
			Severity: SeverityInfo,
			Message:  "no integration branch could be resolved; base-relative checks were skipped",
			Evidence: map[string]any{"source": in.Base.Source},
			Fix: &Remediation{
				Action:     ActionConfigureBase,
				Reversible: true,
				Note:       "declare branch.defaultBranch in .gz-git.yaml so the base stops being a guess",
			},
		}}
	}

	var findings []Finding
	onBase := st.Branch == in.Base.Name

	// Uncommitted work sitting directly on the integration branch. This is the
	// state a branch-per-task workflow exists to prevent: the edits are not
	// isolated, and the branch they are on is the one everything else merges
	// into.
	if onBase && hasUncommittedWork(st) {
		findings = append(findings, Finding{
			Code:     CodeWorkOnBaseBranch,
			Severity: SeverityWarn,
			Message:  fmt.Sprintf("uncommitted work sits directly on the integration branch %q", in.Base.Name),
			Evidence: map[string]any{
				"base":      in.Base.Name,
				"staged":    st.StagedFiles,
				"unstaged":  st.UnstagedFiles,
				"untracked": st.UntrackedFiles,
			},
			Fix: &Remediation{
				Action:     ActionMoveWorkToBranch,
				Reversible: true,
				Note:       "move the edits onto a task branch; do not stash — an interrupted stash is work with no owner",
			},
		})
	}

	// "Behind the base" is only meaningful off the base branch: on it, behind
	// is measured against itself and is always zero.
	if !onBase && in.Base.Behind > 0 {
		findings = append(findings, Finding{
			Code:     CodeBranchBehindBase,
			Severity: SeverityWarn,
			Message: fmt.Sprintf("%d commit(s) behind %s; this branch is built on stale history",
				in.Base.Behind, in.Base.Name),
			Evidence: map[string]any{
				"base":        in.Base.Name,
				"base_source": in.Base.Source,
				"behind":      in.Base.Behind,
				"ahead":       in.Base.Ahead,
			},
			Fix: &Remediation{
				Action:     ActionRebaseOntoBase,
				Command:    []string{"git", "rebase", in.Base.Name},
				Reversible: true,
				Note:       "rewrites local commits; stop on conflict rather than choosing a side",
			},
		})
	}

	return findings
}

// evaluateLocalState covers the working tree and worktree hygiene.
func evaluateLocalState(in AuditInput, st *RepositoryStatusResult) []Finding {
	var findings []Finding

	// Dirty is reported on its own only when it is not already covered by the
	// more specific WORK_ON_BASE_BRANCH finding, so an agent never sees the
	// same edits described twice with two different repairs.
	onBase := in.Base.Name != "" && st.Branch == in.Base.Name
	if hasUncommittedWork(st) && !onBase {
		findings = append(findings, Finding{
			Code:     CodeDirtyWorktree,
			Severity: SeverityInfo,
			Message: fmt.Sprintf("uncommitted changes: %d staged, %d unstaged, %d untracked",
				st.StagedFiles, st.UnstagedFiles, st.UntrackedFiles),
			Evidence: map[string]any{
				"staged":    st.StagedFiles,
				"unstaged":  st.UnstagedFiles,
				"untracked": st.UntrackedFiles,
			},
			Fix: &Remediation{
				Action:     ActionCommitOrDiscard,
				Reversible: false,
				Note:       "uncommitted work has no second copy; never discard it on an agent's initiative",
			},
		})
	}

	if len(in.PrunableWorktrees) > 0 {
		findings = append(findings, Finding{
			Code:     CodeWorktreePrunable,
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("%d worktree record(s) point at directories that no longer exist", len(in.PrunableWorktrees)),
			Evidence: map[string]any{"paths": in.PrunableWorktrees},
			Fix: &Remediation{
				Action:     ActionPruneWorktrees,
				Command:    []string{"git", "worktree", "prune"},
				Reversible: true,
				Note:       "removes bookkeeping only; the directories are already gone",
			},
		})
	}

	if in.StaleStashAfter > 0 && st.StashCount > 0 && !st.OldestStash.IsZero() {
		if age := in.Now.Sub(st.OldestStash); age >= in.StaleStashAfter {
			findings = append(findings, Finding{
				Code:     CodeStaleStash,
				Severity: SeverityInfo,
				Message: fmt.Sprintf("%d stash entr(ies), oldest %d days old; a stash never leaves this machine",
					st.StashCount, int(age.Hours()/24)),
				Evidence: map[string]any{
					"count":      st.StashCount,
					"oldest_age": age.String(),
				},
				Fix: &Remediation{
					Action:     ActionReviewStash,
					Command:    []string{"git", "stash", "list"},
					Reversible: true,
					Note:       "review only; dropping a stash destroys the only copy of that work",
				},
			})
		}
	}

	return findings
}

// hasUncommittedWork is the audit's single definition of a non-clean tree.
func hasUncommittedWork(st *RepositoryStatusResult) bool {
	return st.StagedFiles > 0 || st.UnstagedFiles > 0 ||
		st.UntrackedFiles > 0 || st.TrackedChangedFiles > 0
}

// applyAutofixPolicy stamps each remediation with whether an agent may run it
// unattended.
//
// Policy is consulted per code, but it can never grant permission on an
// irreversible operation: that an operation destroys the only copy of some work
// is a property of the operation, not a preference, so no configuration should
// be able to opt into it. Policy can therefore only ever subtract.
func applyAutofixPolicy(f *Finding, policy func(code string) bool) {
	if f.Fix == nil {
		return
	}
	if !f.Fix.Reversible {
		f.Fix.Autofix = false
		return
	}
	f.Fix.Autofix = policy != nil && policy(f.Code)
}
