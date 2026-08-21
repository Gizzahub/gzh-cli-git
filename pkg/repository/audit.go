// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import "time"

// AuditSchema versions the machine-readable contract. Consumers should refuse
// input whose schema they do not recognize rather than guess at field meanings.
const AuditSchema = "gz-git.info.audit/v1"

// Finding codes. These are the contract: an agent dispatches on Code, not on
// Message. Message wording may change between releases; a code never changes
// meaning, and a retired code is not reused.
//
// Blocker codes describe states in which the rest of the audit cannot be
// trusted — a repository mid-rebase has a HEAD that is a transient artifact of
// the rebase, so its ahead/behind counts describe nothing a user would act on.
// They set AuditRepo.Complete to false.
const (
	// Blockers.
	CodeRebaseInProgress = "REBASE_IN_PROGRESS"
	CodeMergeInProgress  = "MERGE_IN_PROGRESS"
	CodeConflicts        = "CONFLICTS_PRESENT"
	CodeScanError        = "SCAN_ERROR"

	// Branch position.
	CodeDetachedHead         = "DETACHED_HEAD"
	CodeNoUpstream           = "NO_UPSTREAM"
	CodeUpstreamDiverged     = "UPSTREAM_DIVERGED"
	CodeUnpushedCommits      = "UNPUSHED_COMMITS"
	CodeUpstreamBehind       = "UPSTREAM_BEHIND"
	CodeBranchBehindBase     = "BRANCH_BEHIND_BASE"
	CodeBaseUnresolved       = "BASE_UNRESOLVED"
	CodeWorkOnBaseBranch     = "WORK_ON_BASE_BRANCH"
	CodeMergedNotReclaimed   = "MERGED_BRANCH_NOT_RECLAIMED"
	CodeRemoteBotReclaimable = "REMOTE_BOT_BRANCH_RECLAIMABLE"
	CodeRemoteBotSuperseded  = "REMOTE_BOT_BRANCH_SUPERSEDED"
	CodeRemoteBotPending     = "REMOTE_BOT_BRANCH_PENDING"

	// Local state.
	CodeDirtyWorktree       = "DIRTY_WORKTREE"
	CodeWorktreePrunable    = "WORKTREE_PRUNABLE"
	CodeWorktreeReclaimable = "WORKTREE_RECLAIMABLE"
	CodeStaleStash          = "STALE_STASH"
)

// Severity levels.
const (
	SeverityBlocker = "blocker"
	SeverityWarn    = "warn"
	SeverityInfo    = "info"
)

// Remediation action verbs. An agent switches on Action to decide which of its
// own capabilities to invoke; Command is the concrete argv for the simple case
// where it just wants to run git.
const (
	ActionResolveManually    = "resolve-manually"
	ActionRebaseOntoBase     = "rebase-onto-base"
	ActionPush               = "push"
	ActionPull               = "pull"
	ActionSetUpstream        = "set-upstream"
	ActionCheckoutBranch     = "checkout-branch"
	ActionMoveWorkToBranch   = "move-work-to-branch"
	ActionCommitOrDiscard    = "commit-or-discard"
	ActionDeleteBranch       = "delete-branch"
	ActionDeleteRemoteBranch = "delete-remote-branch"
	ActionPruneWorktrees     = "prune-worktrees"
	ActionRemoveWorktree     = "remove-worktree"
	ActionReviewStash        = "review-stash"
	ActionConfigureBase      = "configure-base"
)

// AuditResult is the top-level document.
type AuditResult struct {
	Schema       string       `json:"schema"`
	Directory    string       `json:"directory"`
	Repositories []AuditRepo  `json:"repositories"`
	Summary      AuditSummary `json:"summary"`
}

// AuditSummary lets a caller triage without walking every finding.
type AuditSummary struct {
	Total          int            `json:"total"`
	Complete       int            `json:"complete"`
	Incomplete     int            `json:"incomplete"`
	WithFindings   int            `json:"with_findings"`
	FindingsByCode map[string]int `json:"findings_by_code,omitempty"`
	Blockers       int            `json:"blockers"`
}

// AuditRepo is one repository's verdict.
type AuditRepo struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"`

	// Complete reports whether every check ran against trustworthy state.
	// False means findings on this repository are partial, and an agent must
	// resolve the blocker before acting on anything else here — emitting
	// confident numbers derived from a half-finished rebase would be worse
	// than admitting the audit could not run.
	//
	// The wire name keeps the "audit_" prefix the Go field drops: inside
	// AuditRepo the type already supplies that context, but in the emitted
	// document a bare "complete" reads as a property of the repository rather
	// than of the audit that examined it.
	Complete bool `json:"audit_complete"` //nolint:tagliatelle // wire name is part of the published v1 schema

	// IncompleteReason is the code of the blocker that set Complete false.
	IncompleteReason string `json:"incomplete_reason,omitempty"`

	Base     AuditBase `json:"base"`
	Upstream string    `json:"upstream,omitempty"`

	// Worktrees lists the linked checkouts. They are reported even when they
	// produce no finding: a branch checked out elsewhere is not visible in this
	// repository's own status, so without this an agent reading the document
	// would conclude the work does not exist.
	Worktrees []AuditWorktree `json:"worktrees,omitempty"`

	Findings []Finding `json:"findings"`
}

// AuditWorktree is one checkout of the repository other than the main one.
type AuditWorktree struct {
	Path string `json:"path"`

	// Branch is empty for a detached worktree.
	Branch string `json:"branch,omitempty"`
}

// AuditBase records the resolved integration branch and how it was chosen, so
// a consumer can weigh a divergence claim against the confidence of the base
// it was measured from.
type AuditBase struct {
	Name   string `json:"name,omitempty"`
	Source string `json:"source"`
	Ahead  int    `json:"ahead"`
	Behind int    `json:"behind"`
}

// Finding is one actionable observation.
type Finding struct {
	Code     string         `json:"code"`
	Severity string         `json:"severity"`
	Message  string         `json:"message"`
	Evidence map[string]any `json:"evidence,omitempty"`
	Fix      *Remediation   `json:"fix,omitempty"`
}

// Remediation is the typed repair contract.
//
// Command is argv, never a shell string: the caller execs it directly, so a
// branch name containing shell metacharacters cannot become a second command.
// Autofix is a policy answer ("is an agent allowed to run this unattended"),
// deliberately separate from Reversible, which is a fact about the operation.
// Keeping them apart lets policy tighten without rewriting the catalog.
type Remediation struct {
	Action     string   `json:"action"`
	Command    []string `json:"command,omitempty"`
	Autofix    bool     `json:"autofix"`
	Reversible bool     `json:"reversible"`
	Note       string   `json:"note,omitempty"`
}

// AuditInput is everything the evaluator needs, already collected. Keeping the
// evaluation a pure function of this struct is what makes the catalog testable
// without a git fixture per case.
type AuditInput struct {
	Name   string
	Path   string
	Status *RepositoryStatusResult
	Base   BaseBranchInfo

	// Worktrees are the checkouts other than the main one, with the branch each
	// holds. Branches checked out here are excluded from plain branch deletion:
	// `git branch -d` refuses a branch a worktree is using, so a remediation
	// that ignored this would emit a command that cannot run.
	Worktrees []AuditWorktree

	// PrunableWorktrees are paths git reports as orphaned metadata.
	PrunableWorktrees []string

	// MergedBranches are local branches fully contained in the base branch,
	// excluding the base itself. Verified by ancestry, never inferred from
	// naming or from divergence counts.
	MergedBranches []string

	// RemoteBotMerged are origin remote-tracking bot branches whose tips are
	// ancestors of the base. Names have no origin/ prefix.
	RemoteBotMerged []string

	// RemoteBotSuperseded are origin remote-tracking bot branches that are not
	// ancestors of the base, but whose version target is already satisfied
	// there (equal or newer Go module / Actions pin).
	RemoteBotSuperseded []string

	// RemoteBotPending are origin remote-tracking bot branches that are not
	// ancestors of the base and still newer or not comparable — they may
	// still be an open PR.
	RemoteBotPending []string

	// EnrichErr is a failure from collecting the extra facts above.
	EnrichErr error

	// StaleStashAfter is how old the oldest stash must be to be reported.
	// Zero disables the check.
	StaleStashAfter time.Duration

	// Now is the reference time for age comparisons, injected so results are
	// reproducible in tests.
	Now time.Time

	// AutofixPolicy answers whether an agent may apply a given code
	// unattended. A nil policy means nothing is auto-fixable.
	AutofixPolicy func(code string) bool
}

// Summarize aggregates repository verdicts for triage.
func Summarize(repos []AuditRepo) AuditSummary {
	s := AuditSummary{Total: len(repos), FindingsByCode: map[string]int{}}
	for _, r := range repos {
		if r.Complete {
			s.Complete++
		} else {
			s.Incomplete++
		}
		if len(r.Findings) > 0 {
			s.WithFindings++
		}
		for _, f := range r.Findings {
			s.FindingsByCode[f.Code]++
			if f.Severity == SeverityBlocker {
				s.Blockers++
			}
		}
	}
	return s
}
