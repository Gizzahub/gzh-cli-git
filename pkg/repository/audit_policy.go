// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

// DefaultAutofixPolicy answers, per finding code, whether an agent may apply
// the remediation without asking.
//
// There is one table, not a set of named tiers. A tier name ("conservative",
// "aggressive") is a label a user has to decode into behavior before they can
// trust it, and it forces every future code into someone else's idea of a
// risk bucket. A per-code boolean says exactly what it does, and a project that
// disagrees about one code overrides that one code instead of jumping tiers.
//
// The default set is deliberately narrow: an entry is true only when the
// remediation is a single command that re-verifies its own precondition at run
// time, so a stale audit cannot cause it to destroy anything.
//
//   - UPSTREAM_BEHIND       `pull --ff-only` refuses to invent a merge commit.
//   - BRANCH_BEHIND_BASE    `rebase <base>` stops on conflict; ORIG_HEAD holds
//     the pre-rebase tip.
//   - MERGED_BRANCH_NOT_RECLAIMED  `branch -d` re-checks the merge itself and
//     refuses anything unmerged.
//   - WORKTREE_PRUNABLE     `worktree prune` removes bookkeeping for
//     directories that are already gone.
//   - WORKTREE_RECLAIMABLE  `worktree remove` refuses on modified or untracked
//     files, so it cannot discard anything not already in the base.
//
// Everything absent is false. Absence covers three different reasons, all of
// which reduce to "not unattended": the operation is irreversible and
// applyAutofixPolicy would refuse anyway (push, commit-or-discard), the repair
// needs a decision no table can supply (which branch to check out, which base
// to configure, rebase-or-merge on a divergence), or the remediation only
// gathers information and repairs nothing (reviewing a stash).
var DefaultAutofixPolicy = map[string]bool{
	CodeUpstreamBehind:      true,
	CodeBranchBehindBase:    true,
	CodeMergedNotReclaimed:  true,
	CodeWorktreePrunable:    true,
	CodeWorktreeReclaimable: true,
}

// AutofixPolicyFrom builds the policy function EvaluateRepo consults, starting
// from DefaultAutofixPolicy and applying overrides on top.
//
// An override may enable or disable any code; enabling one still cannot make an
// irreversible remediation auto-fixable, because applyAutofixPolicy checks
// reversibility after consulting policy. Configuration therefore adjusts what
// is permitted, never what is safe.
func AutofixPolicyFrom(overrides map[string]bool) func(code string) bool {
	merged := make(map[string]bool, len(DefaultAutofixPolicy)+len(overrides))
	for code, allowed := range DefaultAutofixPolicy {
		merged[code] = allowed
	}
	for code, allowed := range overrides {
		merged[code] = allowed
	}
	return func(code string) bool { return merged[code] }
}
