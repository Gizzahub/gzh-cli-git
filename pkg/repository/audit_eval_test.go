// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// findingByCode returns the finding with the given code, or nil.
func findingByCode(repo AuditRepo, code string) *Finding {
	for i := range repo.Findings {
		if repo.Findings[i].Code == code {
			return &repo.Findings[i]
		}
	}
	return nil
}

func codes(repo AuditRepo) []string {
	out := make([]string, 0, len(repo.Findings))
	for _, f := range repo.Findings {
		out = append(out, f.Code)
	}
	return out
}

// allowAll is the most permissive policy possible; used to prove that even it
// cannot grant autofix on an irreversible remediation.
func allowAll(string) bool { return true }

func TestEvaluateRepo_BlockerShortCircuits(t *testing.T) {
	tests := []struct {
		name   string
		status *RepositoryStatusResult
		enrich error
		want   string
	}{
		{
			name: "rebase",
			status: &RepositoryStatusResult{
				Branch: "feat/x", Upstream: "origin/feat/x",
				RebaseInProgress: true,
				CommitsAhead:     3, CommitsBehind: 9, UnstagedFiles: 4,
			},
			want: CodeRebaseInProgress,
		},
		{
			name: "merge",
			status: &RepositoryStatusResult{
				Branch: "feat/x", MergeInProgress: true, CommitsAhead: 2,
			},
			want: CodeMergeInProgress,
		},
		{
			name: "conflicts",
			status: &RepositoryStatusResult{
				Branch: "feat/x", ConflictFiles: []string{"a.go", "b.go"},
			},
			want: CodeConflicts,
		},
		{
			name:   "scan error",
			status: &RepositoryStatusResult{Branch: "feat/x", Error: errors.New("permission denied")},
			want:   CodeScanError,
		},
		{
			name:   "enrichment error",
			status: &RepositoryStatusResult{Branch: "feat/x"},
			enrich: errors.New("worktree list failed"),
			want:   CodeScanError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateRepo(AuditInput{
				Name:   "r",
				Status: tt.status,
				Base:   BaseBranchInfo{Name: "master", Source: "heuristic", Behind: 7},
				// A permissive policy must not turn a blocker into something an
				// agent quietly repairs.
				AutofixPolicy: allowAll,
				EnrichErr:     tt.enrich,
			})

			if got.Complete {
				t.Fatalf("audit_complete = true, want false for %s", tt.name)
			}
			if got.IncompleteReason != tt.want {
				t.Errorf("incomplete_reason = %q, want %q", got.IncompleteReason, tt.want)
			}
			// The whole point of a blocker is that nothing else is reported: the
			// remaining numbers were computed from state that is mid-flight.
			if len(got.Findings) != 1 {
				t.Fatalf("findings = %v, want exactly one blocker", codes(got))
			}
			if got.Findings[0].Severity != SeverityBlocker {
				t.Errorf("severity = %q, want %q", got.Findings[0].Severity, SeverityBlocker)
			}
		})
	}
}

func TestEvaluateRepo_NilStatusIsIncomplete(t *testing.T) {
	got := EvaluateRepo(AuditInput{Name: "r"})

	if got.Complete {
		t.Error("audit_complete = true, want false when no status was collected")
	}
	if got.IncompleteReason != CodeScanError {
		t.Errorf("incomplete_reason = %q, want %q", got.IncompleteReason, CodeScanError)
	}
	if got.Base.Source != baseSourceNone {
		t.Errorf("base.source = %q, want %q — source must never be empty", got.Base.Source, baseSourceNone)
	}
}

func TestEvaluateUpstream_StatesAreMutuallyExclusive(t *testing.T) {
	tests := []struct {
		name          string
		upstream      string
		ahead, behind int
		want          string // empty means no upstream finding at all
	}{
		{name: "no upstream", upstream: "", want: CodeNoUpstream},
		{name: "diverged", upstream: "origin/f", ahead: 2, behind: 3, want: CodeUpstreamDiverged},
		{name: "ahead only", upstream: "origin/f", ahead: 2, want: CodeUnpushedCommits},
		{name: "behind only", upstream: "origin/f", behind: 3, want: CodeUpstreamBehind},
		{name: "in sync", upstream: "origin/f", want: ""},
	}

	upstreamCodes := []string{
		CodeNoUpstream, CodeUpstreamDiverged, CodeUnpushedCommits, CodeUpstreamBehind,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateRepo(AuditInput{
				Name: "r",
				Status: &RepositoryStatusResult{
					Branch: "f", Upstream: tt.upstream,
					CommitsAhead: tt.ahead, CommitsBehind: tt.behind,
				},
				Base: BaseBranchInfo{Name: "master", Source: "heuristic"},
			})

			for _, code := range upstreamCodes {
				found := findingByCode(got, code) != nil
				if want := code == tt.want; found != want {
					t.Errorf("finding %s present = %v, want %v (all findings: %v)",
						code, found, want, codes(got))
				}
			}
		})
	}
}

func TestEvaluateUpstream_IntegrationTargetSuppressesPushDiagnosis(t *testing.T) {
	for _, tt := range []struct {
		name             string
		taskRemoteExists bool
		wantCommand      []string
		wantReversible   bool
	}{
		{
			name:           "task ref has not been published",
			wantCommand:    []string{"git", "push", "--set-upstream", "upstream", "HEAD:refs/heads/dev/a/b/c"},
			wantReversible: false,
		},
		{
			name:             "task ref already exists",
			taskRemoteExists: true,
			wantCommand:      []string{"git", "branch", "--set-upstream-to=upstream/dev/a/b/c", "dev/a/b/c"},
			wantReversible:   true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateRepo(AuditInput{
				Name: "r",
				Status: &RepositoryStatusResult{
					Branch: "dev/a/b/c", Upstream: "upstream/develop", CommitsAhead: 3,
				},
				Base:                       BaseBranchInfo{Name: "main", Source: "config.defaultBranch[0]"},
				IntegrationName:            "develop",
				IntegrationSource:          "config[0]",
				UpstreamTargetsIntegration: true,
				UpstreamRemote:             "upstream",
				TaskRemoteExists:           tt.taskRemoteExists,
				AutofixPolicy:              AutofixPolicyFrom(nil),
			})

			finding := findingByCode(got, CodeUpstreamTargetsIntegration)
			if finding == nil {
				t.Fatalf("no %s finding; got %v", CodeUpstreamTargetsIntegration, codes(got))
			}
			for _, code := range []string{CodeUpstreamDiverged, CodeUnpushedCommits, CodeUpstreamBehind} {
				if findingByCode(got, code) != nil {
					t.Errorf("integration upstream also classified as %s", code)
				}
			}
			if finding.Fix.Autofix {
				t.Error("upstream intent change must never be automatic")
			}
			if finding.Fix.Reversible != tt.wantReversible {
				t.Errorf("reversible = %v, want %v", finding.Fix.Reversible, tt.wantReversible)
			}
			if strings.Join(finding.Fix.Command, "\x00") != strings.Join(tt.wantCommand, "\x00") {
				t.Errorf("command = %v, want %v", finding.Fix.Command, tt.wantCommand)
			}
			if got := finding.Evidence["integration"]; got != "develop" {
				t.Errorf("integration evidence = %v", got)
			}
		})
	}
}

func TestEvaluateBase_BehindOnlyReportedOffBase(t *testing.T) {
	// Being "behind master" while standing on master is definitionally zero;
	// reporting it would tell an agent to rebase a branch onto itself.
	onBase := EvaluateRepo(AuditInput{
		Name:   "r",
		Status: &RepositoryStatusResult{Branch: "master", Upstream: "origin/master"},
		Base:   BaseBranchInfo{Name: "master", Source: "heuristic", Behind: 5},
	})
	if f := findingByCode(onBase, CodeBranchBehindBase); f != nil {
		t.Errorf("got %s while standing on the base branch; want none", CodeBranchBehindBase)
	}

	offBase := EvaluateRepo(AuditInput{
		Name:   "r",
		Status: &RepositoryStatusResult{Branch: "feat/x", Upstream: "origin/feat/x"},
		Base:   BaseBranchInfo{Name: "master", Source: "heuristic", Behind: 5, Ahead: 1},
	})
	f := findingByCode(offBase, CodeBranchBehindBase)
	if f == nil {
		t.Fatalf("no %s finding off the base branch; got %v", CodeBranchBehindBase, codes(offBase))
	}
	if f.Fix.Action != ActionRebaseOntoBase {
		t.Errorf("action = %q, want %q", f.Fix.Action, ActionRebaseOntoBase)
	}
	// Command is argv, so the base name lands as one element and can never be
	// re-parsed as a second command.
	want := []string{"git", "rebase", "master"}
	if len(f.Fix.Command) != len(want) {
		t.Fatalf("command = %v, want %v", f.Fix.Command, want)
	}
	for i := range want {
		if f.Fix.Command[i] != want[i] {
			t.Fatalf("command = %v, want %v", f.Fix.Command, want)
		}
	}
}

func TestEvaluateBase_UnresolvedBaseSkipsBaseChecks(t *testing.T) {
	got := EvaluateRepo(AuditInput{
		Name:   "r",
		Status: &RepositoryStatusResult{Branch: "feat/x", Upstream: "origin/feat/x"},
		Base:   BaseBranchInfo{Source: baseSourceNone},
	})

	if findingByCode(got, CodeBaseUnresolved) == nil {
		t.Fatalf("no %s finding; got %v", CodeBaseUnresolved, codes(got))
	}
	if findingByCode(got, CodeBranchBehindBase) != nil {
		t.Errorf("got %s without a resolved base", CodeBranchBehindBase)
	}
	if !got.Complete {
		t.Error("audit_complete = false; a missing base is reportable, not a blocker")
	}
}

func TestEvaluateRepo_DirtyOnBaseReportedOnce(t *testing.T) {
	dirty := &RepositoryStatusResult{
		Branch: "master", Upstream: "origin/master",
		StagedFiles: 1, UnstagedFiles: 2, UntrackedFiles: 3,
	}

	onBase := EvaluateRepo(AuditInput{
		Name: "r", Status: dirty,
		Base: BaseBranchInfo{Name: "master", Source: "heuristic"},
	})
	if findingByCode(onBase, CodeWorkOnBaseBranch) == nil {
		t.Fatalf("no %s finding; got %v", CodeWorkOnBaseBranch, codes(onBase))
	}
	// Same edits, two codes, two different repairs — an agent would act twice.
	if findingByCode(onBase, CodeDirtyWorktree) != nil {
		t.Errorf("both %s and %s reported for the same edits: %v",
			CodeWorkOnBaseBranch, CodeDirtyWorktree, codes(onBase))
	}

	offBase := EvaluateRepo(AuditInput{
		Name: "r",
		Status: &RepositoryStatusResult{
			Branch: "feat/x", Upstream: "origin/feat/x", UnstagedFiles: 2,
		},
		Base: BaseBranchInfo{Name: "master", Source: "heuristic"},
	})
	if findingByCode(offBase, CodeDirtyWorktree) == nil {
		t.Errorf("no %s finding off the base branch; got %v", CodeDirtyWorktree, codes(offBase))
	}
	if findingByCode(offBase, CodeWorkOnBaseBranch) != nil {
		t.Errorf("got %s while off the base branch", CodeWorkOnBaseBranch)
	}
}

func TestEvaluateRepo_RemoteBotReclaimableUsesCleanupCommand(t *testing.T) {
	got := EvaluateRepo(AuditInput{
		Name:            "r",
		Status:          &RepositoryStatusResult{Branch: "master", Upstream: "origin/master"},
		Base:            BaseBranchInfo{Name: "master", Source: "heuristic"},
		RemoteBotMerged: []string{"dependabot/go_modules/x", "renovate/docker-alpine"},
		AutofixPolicy:   allowAll,
	})

	f := findingByCode(got, CodeRemoteBotReclaimable)
	if f == nil {
		t.Fatalf("no %s finding; got %v", CodeRemoteBotReclaimable, codes(got))
	}
	if f.Severity != SeverityInfo {
		t.Errorf("severity = %q, want %q", f.Severity, SeverityInfo)
	}
	if f.Fix == nil || f.Fix.Action != ActionDeleteRemoteBranch {
		t.Fatalf("action = %v, want %s", f.Fix, ActionDeleteRemoteBranch)
	}
	want := []string{"gz-git", "cleanup", "branch", "--bots", "--merged", "--remote", "--force", "--yes"}
	if len(f.Fix.Command) != len(want) {
		t.Fatalf("command = %v, want %v", f.Fix.Command, want)
	}
	for i := range want {
		if f.Fix.Command[i] != want[i] {
			t.Fatalf("command = %v, want %v", f.Fix.Command, want)
		}
	}
	for _, arg := range f.Fix.Command {
		if arg == "--delete" {
			t.Errorf("audit must not suggest raw git push --delete: %v", f.Fix.Command)
		}
	}
	if f.Fix.Reversible {
		t.Error("remote delete must be marked irreversible")
	}
	if f.Fix.Autofix {
		t.Error("autofix granted on irreversible remote delete")
	}
	if f.Evidence["verified_by"] != "git merge-base --is-ancestor" {
		t.Errorf("evidence lacks the ancestry proof: %v", f.Evidence)
	}
	if !strings.Contains(f.Fix.Note, "lease") {
		t.Errorf("note must mention the lease: %q", f.Fix.Note)
	}
	if !strings.Contains(f.Fix.Note, "Do not raw git push --delete") {
		t.Errorf("note must refuse raw push --delete: %q", f.Fix.Note)
	}
}

func TestEvaluateRepo_RemoteBotSupersededUsesCleanupCommand(t *testing.T) {
	got := EvaluateRepo(AuditInput{
		Name:                "r",
		Status:              &RepositoryStatusResult{Branch: "master", Upstream: "origin/master"},
		Base:                BaseBranchInfo{Name: "master", Source: "heuristic"},
		RemoteBotSuperseded: []string{"dependabot/go_modules/github.com/aws/aws-sdk-go-v2-1.40.0"},
		AutofixPolicy:       allowAll,
	})

	f := findingByCode(got, CodeRemoteBotSuperseded)
	if f == nil {
		t.Fatalf("no %s finding; got %v", CodeRemoteBotSuperseded, codes(got))
	}
	if findingByCode(got, CodeRemoteBotPending) != nil {
		t.Errorf("superseded ref was also reported as pending: %v", codes(got))
	}
	if f.Severity != SeverityInfo {
		t.Errorf("severity = %q, want %q", f.Severity, SeverityInfo)
	}
	if f.Fix == nil || f.Fix.Action != ActionDeleteRemoteBranch {
		t.Fatalf("action = %v, want %s", f.Fix, ActionDeleteRemoteBranch)
	}
	want := []string{"gz-git", "cleanup", "branch", "--bots", "--superseded", "--remote", "--force", "--yes"}
	if len(f.Fix.Command) != len(want) {
		t.Fatalf("command = %v, want %v", f.Fix.Command, want)
	}
	for i := range want {
		if f.Fix.Command[i] != want[i] {
			t.Fatalf("command = %v, want %v", f.Fix.Command, want)
		}
	}
	for _, arg := range f.Fix.Command {
		if arg == "--delete" {
			t.Errorf("audit must not suggest raw git push --delete: %v", f.Fix.Command)
		}
	}
	if f.Fix.Reversible {
		t.Error("remote delete must be marked irreversible")
	}
	if f.Fix.Autofix {
		t.Error("autofix granted on superseded remote delete despite allowAll policy")
	}
	if f.Evidence["verified_by"] != "version comparison" {
		t.Errorf("evidence must cite version comparison, not ancestry: %v", f.Evidence)
	}
}

func TestEvaluateRepo_RemoteBotPendingHasNoDeleteCommand(t *testing.T) {
	got := EvaluateRepo(AuditInput{
		Name:             "r",
		Status:           &RepositoryStatusResult{Branch: "master", Upstream: "origin/master"},
		Base:             BaseBranchInfo{Name: "master", Source: "heuristic"},
		RemoteBotPending: []string{"dependabot/go_modules/unmerged"},
		AutofixPolicy:    allowAll,
	})

	f := findingByCode(got, CodeRemoteBotPending)
	if f == nil {
		t.Fatalf("no %s finding; got %v", CodeRemoteBotPending, codes(got))
	}
	if f.Fix == nil {
		t.Fatal("pending finding has no remediation")
	}
	if f.Fix.Action != ActionResolveManually {
		t.Errorf("action = %q, want %q", f.Fix.Action, ActionResolveManually)
	}
	for _, arg := range f.Fix.Command {
		if arg == "--delete" || arg == "push" {
			t.Errorf("pending remediation must not delete: %v", f.Fix.Command)
		}
	}
	if !f.Fix.Reversible {
		t.Error("pending note-only remediation should stay reversible")
	}
}

func TestEvaluateRepo_MergedBranchesUseSafeDelete(t *testing.T) {
	got := EvaluateRepo(AuditInput{
		Name:           "r",
		Status:         &RepositoryStatusResult{Branch: "master", Upstream: "origin/master"},
		Base:           BaseBranchInfo{Name: "master", Source: "heuristic"},
		MergedBranches: []string{"feat/done", "fix/old"},
	})

	f := findingByCode(got, CodeMergedNotReclaimed)
	if f == nil {
		t.Fatalf("no %s finding; got %v", CodeMergedNotReclaimed, codes(got))
	}
	// -D would delete unmerged work on a stale audit; -d re-verifies at run time,
	// so the command is safe even if the repository changed since the scan.
	if len(f.Fix.Command) < 3 || f.Fix.Command[2] != "-d" {
		t.Errorf("command = %v, want `git branch -d ...` (never -D)", f.Fix.Command)
	}
	if f.Evidence["verified_by"] != "git merge-base --is-ancestor" {
		t.Errorf("evidence lacks the ancestry proof: %v", f.Evidence)
	}
}

func TestEvaluateRepo_WorktreeHeldBranchGetsWorktreeRemediation(t *testing.T) {
	got := EvaluateRepo(AuditInput{
		Name:           "r",
		Status:         &RepositoryStatusResult{Branch: "master", Upstream: "origin/master"},
		Base:           BaseBranchInfo{Name: "master", Source: "heuristic"},
		MergedBranches: []string{"feat/parked", "fix/loose"},
		Worktrees: []AuditWorktree{
			{Path: "/wt/feat__parked", Branch: "feat/parked"},
		},
	})

	// `git branch -d feat/parked` would fail: a worktree is using it. Only the
	// branch no worktree holds may appear in the delete command.
	del := findingByCode(got, CodeMergedNotReclaimed)
	if del == nil {
		t.Fatalf("no %s finding; got %v", CodeMergedNotReclaimed, codes(got))
	}
	for _, arg := range del.Fix.Command {
		if arg == "feat/parked" {
			t.Errorf("delete command targets a worktree-held branch: %v", del.Fix.Command)
		}
	}
	if len(del.Fix.Command) != 4 || del.Fix.Command[3] != "fix/loose" {
		t.Errorf("command = %v, want `git branch -d fix/loose`", del.Fix.Command)
	}

	reclaim := findingByCode(got, CodeWorktreeReclaimable)
	if reclaim == nil {
		t.Fatalf("no %s finding; got %v", CodeWorktreeReclaimable, codes(got))
	}
	want := []string{"git", "worktree", "remove", "/wt/feat__parked"}
	for i := range want {
		if i >= len(reclaim.Fix.Command) || reclaim.Fix.Command[i] != want[i] {
			t.Fatalf("command = %v, want %v", reclaim.Fix.Command, want)
		}
	}
}

func TestEvaluateRepo_WorktreesReportedWithoutFindings(t *testing.T) {
	// A worktree holding unfinished work is not a problem, but an agent that
	// cannot see it will conclude the branch does not exist anywhere.
	got := EvaluateRepo(AuditInput{
		Name:   "r",
		Status: &RepositoryStatusResult{Branch: "master", Upstream: "origin/master"},
		Base:   BaseBranchInfo{Name: "master", Source: "heuristic"},
		Worktrees: []AuditWorktree{
			{Path: "/wt/feat__wip", Branch: "feat/wip"},
		},
	})

	if len(got.Findings) != 0 {
		t.Errorf("findings = %v, want none for an in-progress worktree", codes(got))
	}
	if len(got.Worktrees) != 1 || got.Worktrees[0].Branch != "feat/wip" {
		t.Errorf("worktrees = %+v, want the linked worktree carried through", got.Worktrees)
	}
}

func TestPartitionMerged_DetachedWorktreeHoldsNoBranch(t *testing.T) {
	deletable, held := partitionMerged(
		[]string{"feat/a", "feat/b"},
		[]AuditWorktree{{Path: "/wt/detached"}}, // no branch
	)

	if len(held) != 0 {
		t.Errorf("held = %+v, want none: a detached worktree blocks no branch name", held)
	}
	if len(deletable) != 2 {
		t.Errorf("deletable = %v, want both branches", deletable)
	}
}

func TestEvaluateLocalState_StaleStash(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	base := BaseBranchInfo{Name: "master", Source: "heuristic"}

	tests := []struct {
		name      string
		count     int
		oldest    time.Time
		threshold time.Duration
		want      bool
	}{
		{name: "old enough", count: 2, oldest: now.AddDate(0, 0, -30), threshold: 14 * 24 * time.Hour, want: true},
		{name: "too recent", count: 2, oldest: now.AddDate(0, 0, -3), threshold: 14 * 24 * time.Hour, want: false},
		{name: "check disabled", count: 2, oldest: now.AddDate(0, 0, -30), threshold: 0, want: false},
		{name: "no stashes", count: 0, threshold: 14 * 24 * time.Hour, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateRepo(AuditInput{
				Name: "r",
				Status: &RepositoryStatusResult{
					Branch: "feat/x", Upstream: "origin/feat/x",
					StashCount: tt.count, OldestStash: tt.oldest,
				},
				Base:            base,
				StaleStashAfter: tt.threshold,
				Now:             now,
			})
			if found := findingByCode(got, CodeStaleStash) != nil; found != tt.want {
				t.Errorf("%s present = %v, want %v", CodeStaleStash, found, tt.want)
			}
		})
	}
}

func TestEvaluateRepo_DetachedHeadSuppressesUpstreamChecks(t *testing.T) {
	got := EvaluateRepo(AuditInput{
		Name:   "r",
		Status: &RepositoryStatusResult{Branch: "", HeadSHA: "abc1234"},
		Base:   BaseBranchInfo{Name: "master", Source: "heuristic"},
	})

	if findingByCode(got, CodeDetachedHead) == nil {
		t.Fatalf("no %s finding; got %v", CodeDetachedHead, codes(got))
	}
	// "tracks no upstream" is true but useless here: a detached HEAD cannot have
	// one, and the repair (push -u origin "") would not even run.
	if findingByCode(got, CodeNoUpstream) != nil {
		t.Errorf("got %s on a detached HEAD: %v", CodeNoUpstream, codes(got))
	}
}

func TestSummarize(t *testing.T) {
	repos := []AuditRepo{
		{Complete: true},
		{Complete: true, Findings: []Finding{
			{Code: CodeDirtyWorktree, Severity: SeverityInfo},
			{Code: CodeUnpushedCommits, Severity: SeverityWarn},
		}},
		{Complete: false, IncompleteReason: CodeRebaseInProgress, Findings: []Finding{
			{Code: CodeRebaseInProgress, Severity: SeverityBlocker},
		}},
		{Complete: true, Findings: []Finding{
			{Code: CodeDirtyWorktree, Severity: SeverityInfo},
		}},
	}

	got := Summarize(repos)

	if got.Total != 4 || got.Complete != 3 || got.Incomplete != 1 {
		t.Errorf("total/complete/incomplete = %d/%d/%d, want 4/3/1",
			got.Total, got.Complete, got.Incomplete)
	}
	if got.WithFindings != 3 {
		t.Errorf("with_findings = %d, want 3", got.WithFindings)
	}
	if got.Blockers != 1 {
		t.Errorf("blockers = %d, want 1", got.Blockers)
	}
	if got.FindingsByCode[CodeDirtyWorktree] != 2 {
		t.Errorf("findings_by_code[%s] = %d, want 2", CodeDirtyWorktree, got.FindingsByCode[CodeDirtyWorktree])
	}
}
