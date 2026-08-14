// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package handoff

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// cleanRepo returns a status result with nothing local outstanding, which
// individual tests then perturb.
func cleanRepo() repository.RepositoryStatusResult {
	return repository.RepositoryStatusResult{
		Path:         "/w/repo",
		RelativePath: "repo",
		Status:       repository.StatusClean,
		Branch:       "feat/task-001",
		RemoteURL:    "git@example.com:org/repo.git",
	}
}

// reasons extracts the blocker reasons of a single-repo assessment.
func reasons(t *testing.T, a *Assessment) []Reason {
	t.Helper()
	if len(a.Repositories) != 1 {
		t.Fatalf("expected 1 repository, got %d", len(a.Repositories))
	}
	out := make([]Reason, 0, len(a.Repositories[0].Blockers))
	for _, b := range a.Repositories[0].Blockers {
		out = append(out, b.Reason)
	}
	return out
}

func hasReason(rs []Reason, want Reason) bool {
	return slices.Contains(rs, want)
}

func TestAssessCleanRepoIsReady(t *testing.T) {
	a := Assess([]repository.RepositoryStatusResult{cleanRepo()})

	if a.Verdict != VerdictReady {
		t.Errorf("verdict = %s, want %s", a.Verdict, VerdictReady)
	}
	if !a.Repositories[0].Ready() {
		t.Errorf("clean repo reported blockers: %+v", a.Repositories[0].Blockers)
	}
	if len(a.NotReady()) != 0 {
		t.Errorf("NotReady() = %+v, want empty", a.NotReady())
	}
}

func TestAssessUncommittedAndUnpushedAreAutoFixable(t *testing.T) {
	r := cleanRepo()
	r.Status = repository.StatusDirty
	r.TrackedChangedFiles = 3
	r.UntrackedFiles = 2
	r.CommitsAhead = 4

	a := Assess([]repository.RepositoryStatusResult{r})

	got := reasons(t, a)
	if !hasReason(got, ReasonUncommitted) || !hasReason(got, ReasonUnpushed) {
		t.Fatalf("reasons = %v, want uncommitted and unpushed", got)
	}
	if a.Verdict != VerdictFixable {
		t.Errorf("verdict = %s, want %s", a.Verdict, VerdictFixable)
	}
	if !a.Repositories[0].AutoFixable() {
		t.Error("dirty repo with a remote and a branch should be auto-fixable")
	}
}

func TestAssessUntrackedOnlyWorkIsAutoFixable(t *testing.T) {
	r := cleanRepo()
	r.Status = repository.StatusDirty
	r.UntrackedFiles = 1

	a := Assess([]repository.RepositoryStatusResult{r})

	got := reasons(t, a)
	if len(got) != 1 || got[0] != ReasonUncommitted {
		t.Fatalf("reasons = %v, want exactly [uncommitted]", got)
	}
	if a.Verdict != VerdictFixable {
		t.Errorf("verdict = %s, want %s", a.Verdict, VerdictFixable)
	}
	if detail := a.Repositories[0].Blockers[0].Detail; detail != "1 untracked file(s) exist only here" {
		t.Errorf("detail = %q", detail)
	}
}

func TestAssessStashIsNeverAutoFixable(t *testing.T) {
	r := cleanRepo()
	r.StashCount = 2

	a := Assess([]repository.RepositoryStatusResult{r})

	if !hasReason(reasons(t, a), ReasonStashed) {
		t.Fatalf("reasons = %v, want stashed", reasons(t, a))
	}
	if a.Verdict != VerdictBlocked {
		t.Errorf("verdict = %s, want %s — a stash needs a decision, not a cleanup", a.Verdict, VerdictBlocked)
	}
	if len(a.Blocked()) != 1 {
		t.Errorf("Blocked() = %+v, want the stashed repo", a.Blocked())
	}
}

func TestAssessOldStashIsReportedAsStranded(t *testing.T) {
	r := cleanRepo()
	r.StashCount = 1
	r.OldestStash = time.Now().Add(-30 * 24 * time.Hour)

	a := Assess([]repository.RepositoryStatusResult{r})

	got := reasons(t, a)
	if len(got) != 1 || got[0] != ReasonStranded {
		t.Fatalf("reasons = %v, want exactly [stranded]", got)
	}
	if a.Verdict != VerdictBlocked {
		t.Errorf("verdict = %s, want %s", a.Verdict, VerdictBlocked)
	}

	detail := a.Repositories[0].Blockers[0].Detail
	if !strings.Contains(detail, "30 days old") {
		t.Errorf("detail = %q, want the age of the oldest entry", detail)
	}
}

func TestAssessRecentStashIsNotStranded(t *testing.T) {
	r := cleanRepo()
	r.StashCount = 1
	r.OldestStash = time.Now().Add(-2 * time.Hour)

	got := reasons(t, Assess([]repository.RepositoryStatusResult{r}))
	if len(got) != 1 || got[0] != ReasonStashed {
		t.Fatalf("reasons = %v, want exactly [stashed] — a stash made today is work in progress", got)
	}
}

// A repository with several stashes is one repository. Reporting the fresh ones
// separately from the old ones would make one machine look like two problems.
func TestAssessStashesOfMixedAgeAreOneBlocker(t *testing.T) {
	r := cleanRepo()
	r.StashCount = 3
	r.OldestStash = time.Now().Add(-20 * 24 * time.Hour)

	a := Assess([]repository.RepositoryStatusResult{r})

	if n := len(a.Repositories[0].Blockers); n != 1 {
		t.Fatalf("blockers = %d, want 1: %+v", n, a.Repositories[0].Blockers)
	}
	if got := a.Repositories[0].Blockers[0]; got.Reason != ReasonStranded ||
		!strings.HasPrefix(got.Detail, "3 stash entries") {
		t.Errorf("blocker = %+v, want all three counted under the oldest one's reason", got)
	}
}

func TestHumanAge(t *testing.T) {
	tests := []struct {
		age  time.Duration
		want string
	}{
		{3 * time.Hour, "from today"},
		{25 * time.Hour, "1 day old"},
		{9 * 24 * time.Hour, "9 days old"},
		{200 * 24 * time.Hour, "6 months old"},
	}

	for _, tt := range tests {
		if got := humanAge(tt.age); got != tt.want {
			t.Errorf("humanAge(%s) = %q, want %q", tt.age, got, tt.want)
		}
	}
}

func TestAssessLocalWorkWithoutRemoteIsNotAutoFixable(t *testing.T) {
	r := cleanRepo()
	r.RemoteURL = ""
	r.Status = repository.StatusDirty
	r.TrackedChangedFiles = 1

	a := Assess([]repository.RepositoryStatusResult{r})

	if !hasReason(reasons(t, a), ReasonNoRemote) {
		t.Fatalf("reasons = %v, want no-remote", reasons(t, a))
	}
	for _, b := range a.Repositories[0].Blockers {
		if b.Reason == ReasonUncommitted && b.AutoFixable {
			t.Error("uncommitted work is not auto-fixable when there is nowhere to push")
		}
	}
	if a.Verdict != VerdictBlocked {
		t.Errorf("verdict = %s, want %s", a.Verdict, VerdictBlocked)
	}
}

func TestAssessDetachedHeadBlocksAutoFix(t *testing.T) {
	r := cleanRepo()
	r.Branch = ""
	r.CommitsAhead = 1

	a := Assess([]repository.RepositoryStatusResult{r})

	if !hasReason(reasons(t, a), ReasonDetached) {
		t.Fatalf("reasons = %v, want detached-head", reasons(t, a))
	}
	if a.Repositories[0].AutoFixable() {
		t.Error("a detached HEAD cannot be pushed, so it must not be auto-fixable")
	}
}

func TestAssessConflictAndInProgressStates(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*repository.RepositoryStatusResult)
		want   Reason
	}{
		{
			name: "conflict",
			mutate: func(r *repository.RepositoryStatusResult) {
				r.Status = repository.StatusConflict
				r.ConflictFiles = []string{"a.go", "b.go"}
			},
			want: ReasonConflict,
		},
		{
			name:   "rebase",
			mutate: func(r *repository.RepositoryStatusResult) { r.RebaseInProgress = true },
			want:   ReasonInProgress,
		},
		{
			name:   "merge",
			mutate: func(r *repository.RepositoryStatusResult) { r.MergeInProgress = true },
			want:   ReasonInProgress,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := cleanRepo()
			tt.mutate(&r)

			a := Assess([]repository.RepositoryStatusResult{r})

			if !hasReason(reasons(t, a), tt.want) {
				t.Errorf("reasons = %v, want %s", reasons(t, a), tt.want)
			}
			if a.Verdict != VerdictBlocked {
				t.Errorf("verdict = %s, want %s", a.Verdict, VerdictBlocked)
			}
		})
	}
}

func TestAssessNoUpstreamIsAutoFixable(t *testing.T) {
	r := cleanRepo()
	r.Status = repository.StatusNoUpstream
	r.CommitsAhead = 2

	a := Assess([]repository.RepositoryStatusResult{r})

	if !hasReason(reasons(t, a), ReasonNoUpstream) {
		t.Fatalf("reasons = %v, want no-upstream", reasons(t, a))
	}
	if a.Verdict != VerdictFixable {
		t.Errorf("verdict = %s, want %s — push --set-upstream resolves it", a.Verdict, VerdictFixable)
	}
}

func TestAssessErrorRepoStopsAtTheError(t *testing.T) {
	r := cleanRepo()
	r.Status = repository.StatusError
	r.Error = errors.New("not a git repository")
	// Values that would otherwise produce blockers must be ignored: a failed
	// scan means none of them can be trusted.
	r.TrackedChangedFiles = 9
	r.StashCount = 3

	a := Assess([]repository.RepositoryStatusResult{r})

	got := reasons(t, a)
	if len(got) != 1 || got[0] != ReasonError {
		t.Fatalf("reasons = %v, want exactly [error]", got)
	}
	if a.Verdict != VerdictBlocked {
		t.Errorf("verdict = %s, want %s", a.Verdict, VerdictBlocked)
	}
}

func TestAssessVerdictTakesTheWorstRepo(t *testing.T) {
	clean := cleanRepo()

	fixable := cleanRepo()
	fixable.RelativePath = "fixable"
	fixable.Status = repository.StatusDirty
	fixable.TrackedChangedFiles = 1

	blocked := cleanRepo()
	blocked.RelativePath = "blocked"
	blocked.StashCount = 1

	a := Assess([]repository.RepositoryStatusResult{clean, fixable, blocked})

	if a.Verdict != VerdictBlocked {
		t.Errorf("verdict = %s, want %s", a.Verdict, VerdictBlocked)
	}
	if a.TotalScanned != 3 {
		t.Errorf("TotalScanned = %d, want 3", a.TotalScanned)
	}
	if got := len(a.NotReady()); got != 2 {
		t.Errorf("NotReady() = %d repos, want 2", got)
	}
	if got := a.Blocked(); len(got) != 1 || got[0].RelativePath != "blocked" {
		t.Errorf("Blocked() = %+v, want only the stashed repo", got)
	}

	counts := a.ReasonCounts()
	if counts[ReasonUncommitted] != 1 || counts[ReasonStashed] != 1 {
		t.Errorf("ReasonCounts() = %v", counts)
	}
}

func TestAssessEmptyScanIsReady(t *testing.T) {
	a := Assess(nil)

	if a.Verdict != VerdictReady {
		t.Errorf("verdict = %s, want %s", a.Verdict, VerdictReady)
	}
	if a.TotalScanned != 0 {
		t.Errorf("TotalScanned = %d, want 0", a.TotalScanned)
	}
}

func TestUncommittedDetailCountsEachPathOnce(t *testing.T) {
	tests := []struct {
		name      string
		tracked   int
		untracked int
		want      string
	}{
		// The counters are disjoint — a path is either tracked with uncommitted
		// changes or untracked, never both — so neither is subtracted from the
		// other.
		{"untracked only", 0, 1, "1 untracked file(s) exist only here"},
		{"mixed", 3, 1, "3 modified, 1 untracked file(s) exist only here"},
		{"modified only", 2, 0, "2 modified file(s) exist only here"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := cleanRepo()
			r.Status = repository.StatusDirty
			r.TrackedChangedFiles = tt.tracked
			r.UntrackedFiles = tt.untracked

			if got := uncommittedDetail(r); got != tt.want {
				t.Errorf("detail = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPluralFormatting(t *testing.T) {
	r := cleanRepo()
	r.StashCount = 1
	a := Assess([]repository.RepositoryStatusResult{r})

	detail := a.Repositories[0].Blockers[0].Detail
	if want := "1 stash entry never leaves this machine"; detail != want {
		t.Errorf("detail = %q, want %q", detail, want)
	}
}
