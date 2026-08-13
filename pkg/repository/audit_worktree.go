// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import "fmt"

// partitionMerged splits branches already contained in the base into the ones a
// plain delete can reclaim and the ones a worktree is holding.
//
// The split exists because the two states need different commands, not because
// they need different wording. `git branch -d` refuses a branch that any
// worktree has checked out, so emitting it for one would hand an agent an
// instruction guaranteed to fail — and an agent that retries failures is one
// `--force` away from deleting something it should not.
func partitionMerged(merged []string, worktrees []AuditWorktree) (deletable []string, held []AuditWorktree) {
	byBranch := make(map[string]AuditWorktree, len(worktrees))
	for _, wt := range worktrees {
		if wt.Branch != "" {
			byBranch[wt.Branch] = wt
		}
	}

	for _, name := range merged {
		if wt, ok := byBranch[name]; ok {
			held = append(held, wt)
			continue
		}
		deletable = append(deletable, name)
	}
	return deletable, held
}

// evaluateWorktreeReclaim reports linked worktrees whose branch has fully landed
// in the base — finished task work still holding a directory and a branch name.
//
// Reclaiming is the last step of integrating, not a separate chore: a worktree
// left behind after its branch merged is indistinguishable, from the outside,
// from one holding work in progress. That ambiguity is what makes it worth
// reporting.
func evaluateWorktreeReclaim(held []AuditWorktree, base string) []Finding {
	if len(held) == 0 {
		return nil
	}

	paths := make([]string, 0, len(held))
	branches := make([]string, 0, len(held))
	for _, wt := range held {
		paths = append(paths, wt.Path)
		branches = append(branches, wt.Branch)
	}

	// One command per worktree would be ambiguous about ordering; git accepts
	// only one path per invocation, so the first is the command and the rest are
	// evidence. An agent that handles the finding re-runs the audit and gets the
	// next one.
	return []Finding{{
		Code:     CodeWorktreeReclaimable,
		Severity: SeverityInfo,
		Message: fmt.Sprintf("%d worktree(s) hold branches already merged into %s",
			len(held), base),
		Evidence: map[string]any{
			"paths":       paths,
			"branches":    branches,
			"base":        base,
			"verified_by": "git merge-base --is-ancestor",
		},
		Fix: &Remediation{
			Action:     ActionRemoveWorktree,
			Command:    []string{"git", "worktree", "remove", paths[0]},
			Reversible: true,
			Note: "refuses on modified or untracked files, so it cannot discard work; " +
				"delete the branch afterwards, and repeat for the remaining paths",
		},
	}}
}
