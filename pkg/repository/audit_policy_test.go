// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import "testing"

// TestDefaultAutofixPolicy_OnlySelfVerifyingCommands pins the rule that decided
// the default table: an entry is allowed only when its command re-checks its own
// precondition at run time, so acting on a stale audit cannot destroy work.
func TestDefaultAutofixPolicy_OnlySelfVerifyingCommands(t *testing.T) {
	want := map[string]bool{
		CodeUpstreamBehind:      true,
		CodeBranchBehindBase:    true,
		CodeMergedNotReclaimed:  true,
		CodeWorktreePrunable:    true,
		CodeWorktreeReclaimable: true,
	}

	for code, allowed := range want {
		if DefaultAutofixPolicy[code] != allowed {
			t.Errorf("DefaultAutofixPolicy[%s] = %v, want %v", code, DefaultAutofixPolicy[code], allowed)
		}
	}
	if len(DefaultAutofixPolicy) != len(want) {
		t.Errorf("DefaultAutofixPolicy has %d entries, want exactly %d: %v",
			len(DefaultAutofixPolicy), len(want), DefaultAutofixPolicy)
	}

	// Codes that need a decision no table can supply must stay off.
	for _, code := range []string{
		CodeDetachedHead, CodeWorkOnBaseBranch, CodeBaseUnresolved,
		CodeUpstreamDiverged, CodeStaleStash, CodeConflicts,
		CodeRebaseInProgress, CodeMergeInProgress,
	} {
		if DefaultAutofixPolicy[code] {
			t.Errorf("DefaultAutofixPolicy[%s] = true; this repair requires judgment", code)
		}
	}
}

func TestAutofixPolicyFrom_OverridesAreScopedToTheirCode(t *testing.T) {
	policy := AutofixPolicyFrom(map[string]bool{
		CodeBranchBehindBase: false, // project rebases by hand
		CodeDirtyWorktree:    true,  // permitted, but still irreversible
	})

	if policy(CodeBranchBehindBase) {
		t.Error("override did not disable CodeBranchBehindBase")
	}
	// Turning one code off must not disturb the rest of the defaults.
	if !policy(CodeUpstreamBehind) || !policy(CodeWorktreePrunable) {
		t.Error("an override reset unrelated defaults")
	}
	if !policy(CodeDirtyWorktree) {
		t.Error("override did not enable CodeDirtyWorktree at the policy layer")
	}

	// ...and permission at the policy layer still yields no autofix, because
	// reversibility is checked afterwards. Configuration cannot opt into
	// discarding the only copy of someone's work.
	f := Finding{Code: CodeDirtyWorktree, Fix: &Remediation{
		Action: ActionCommitOrDiscard, Reversible: false,
	}}
	applyAutofixPolicy(&f, policy)
	if f.Fix.Autofix {
		t.Error("autofix granted on an irreversible remediation via config override")
	}
}

func TestAutofixPolicyFrom_NilOverridesKeepsDefaults(t *testing.T) {
	policy := AutofixPolicyFrom(nil)
	if !policy(CodeUpstreamBehind) {
		t.Error("nil overrides lost the default table")
	}
	if policy(CodeDirtyWorktree) {
		t.Error("nil overrides invented a permission")
	}
}

// The two tests below reach applyAutofixPolicy through EvaluateRepo rather
// than calling it directly. The table above says what the policy answers; these
// say what the answer is worth once a real finding carries it, which is the
// part a mistake would actually be felt in.
func TestApplyAutofixPolicy_NeverGrantsOnIrreversible(t *testing.T) {
	// Every irreversible remediation in the catalog, reached through EvaluateRepo
	// with the most permissive policy possible.
	inputs := []AuditInput{
		{ // NO_UPSTREAM -> push -u publishes the branch
			Name:   "no-upstream",
			Status: &RepositoryStatusResult{Branch: "feat/x"},
			Base:   BaseBranchInfo{Name: "master", Source: "heuristic"},
		},
		{ // UNPUSHED_COMMITS -> push makes commits visible to others
			Name:   "unpushed",
			Status: &RepositoryStatusResult{Branch: "feat/x", Upstream: "origin/feat/x", CommitsAhead: 2},
			Base:   BaseBranchInfo{Name: "master", Source: "heuristic"},
		},
		{ // UPSTREAM_DIVERGED -> rebase-vs-merge is a judgment call
			Name:   "diverged",
			Status: &RepositoryStatusResult{Branch: "feat/x", Upstream: "origin/feat/x", CommitsAhead: 1, CommitsBehind: 1},
			Base:   BaseBranchInfo{Name: "master", Source: "heuristic"},
		},
		{ // DIRTY_WORKTREE -> uncommitted work has no second copy
			Name:   "dirty",
			Status: &RepositoryStatusResult{Branch: "feat/x", Upstream: "origin/feat/x", UnstagedFiles: 3},
			Base:   BaseBranchInfo{Name: "master", Source: "heuristic"},
		},
	}

	for _, in := range inputs {
		t.Run(in.Name, func(t *testing.T) {
			in.AutofixPolicy = allowAll
			got := EvaluateRepo(in)

			sawIrreversible := false
			for _, f := range got.Findings {
				if f.Fix == nil || f.Fix.Reversible {
					continue
				}
				sawIrreversible = true
				if f.Fix.Autofix {
					t.Errorf("%s: autofix granted on an irreversible remediation (%s)", f.Code, f.Fix.Action)
				}
			}
			if !sawIrreversible {
				t.Fatalf("no irreversible remediation reached; findings: %v", codes(got))
			}
		})
	}
}

func TestApplyAutofixPolicy_ConsultedPerCode(t *testing.T) {
	// UPSTREAM_BEHIND (pull --ff-only) and WORKTREE_PRUNABLE (worktree prune) are
	// both reversible, so policy — not reversibility — decides between them.
	in := AuditInput{
		Name: "r",
		Status: &RepositoryStatusResult{
			Branch: "feat/x", Upstream: "origin/feat/x", CommitsBehind: 2,
		},
		Base:              BaseBranchInfo{Name: "master", Source: "heuristic"},
		PrunableWorktrees: []string{"/gone/wt"},
		AutofixPolicy:     func(code string) bool { return code == CodeWorktreePrunable },
	}

	got := EvaluateRepo(in)

	prune := findingByCode(got, CodeWorktreePrunable)
	if prune == nil || !prune.Fix.Autofix {
		t.Errorf("%s autofix not granted despite policy allowing it", CodeWorktreePrunable)
	}
	behind := findingByCode(got, CodeUpstreamBehind)
	if behind == nil {
		t.Fatalf("no %s finding; got %v", CodeUpstreamBehind, codes(got))
	}
	if behind.Fix.Autofix {
		t.Errorf("%s autofix granted despite policy withholding it", CodeUpstreamBehind)
	}

	// A nil policy is the safe default: reversible is not the same as permitted.
	in.AutofixPolicy = nil
	for _, f := range EvaluateRepo(in).Findings {
		if f.Fix != nil && f.Fix.Autofix {
			t.Errorf("%s autofix granted with a nil policy", f.Code)
		}
	}
}
