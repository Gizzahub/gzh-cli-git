// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package handoff

import (
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// dirty returns a status result with uncommitted work under the given name.
func dirty(name string) repository.RepositoryStatusResult {
	r := cleanRepo()
	r.Path = "/w/" + name
	r.RelativePath = name
	r.Status = repository.StatusDirty
	r.UncommittedFiles = 1
	return r
}

func planFor(results ...repository.RepositoryStatusResult) Plan {
	return PlanCheckpoint(Assess(results))
}

func names(repos []RepoAssessment) []string {
	out := make([]string, 0, len(repos))
	for _, r := range repos {
		out = append(out, r.RelativePath)
	}
	return out
}

func TestPlanCheckpointSelectsMovableWork(t *testing.T) {
	unpushed := cleanRepo()
	unpushed.Path = "/w/unpushed"
	unpushed.RelativePath = "unpushed"
	unpushed.CommitsAhead = 2

	plan := planFor(cleanRepo(), dirty("dirty"), unpushed)

	if got := names(plan.Checkpoint); len(got) != 2 || got[0] != "dirty" || got[1] != "unpushed" {
		t.Errorf("checkpoint = %v, want [dirty unpushed]", got)
	}
	if len(plan.Skipped) != 0 {
		t.Errorf("skipped = %v, want none", names(plan.Skipped))
	}
}

func TestPlanCheckpointSkipsHardBlockers(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*repository.RepositoryStatusResult)
	}{
		{"conflict", func(r *repository.RepositoryStatusResult) {
			r.Status = repository.StatusConflict
			r.ConflictFiles = []string{"a.go"}
		}},
		{"rebase", func(r *repository.RepositoryStatusResult) { r.RebaseInProgress = true }},
		{"detached", func(r *repository.RepositoryStatusResult) { r.Branch = "" }},
		{"no-remote", func(r *repository.RepositoryStatusResult) { r.RemoteURL = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Every case also has ordinary uncommitted work, which is exactly
			// what makes skipping it the interesting decision.
			r := dirty(tt.name)
			tt.mutate(&r)

			plan := planFor(r)

			if len(plan.Checkpoint) != 0 {
				t.Errorf("checkpoint = %v, want none — %s must be resolved by a person", names(plan.Checkpoint), tt.name)
			}
			if got := names(plan.Skipped); len(got) != 1 || got[0] != tt.name {
				t.Errorf("skipped = %v, want [%s]", got, tt.name)
			}
		})
	}
}

func TestPlanCheckpointCommitsAlongsideAStash(t *testing.T) {
	r := dirty("stashed")
	r.StashCount = 1

	plan := planFor(r)

	if got := names(plan.Checkpoint); len(got) != 1 || got[0] != "stashed" {
		t.Errorf("checkpoint = %v, want [stashed] — a stash does not make the rest of the work unmovable", got)
	}
}

func TestPlanCheckpointIgnoresStashOnlyRepositories(t *testing.T) {
	r := cleanRepo()
	r.StashCount = 1

	plan := planFor(r)

	if !plan.Empty() {
		t.Errorf("checkpoint = %v, want none — there is nothing to commit or push", names(plan.Checkpoint))
	}
	if len(plan.Skipped) != 0 {
		t.Errorf("skipped = %v, want none — a stash is reported by the final assessment, not by the plan", names(plan.Skipped))
	}
}

func TestPlanCheckpointIsEmptyForACleanWorkspace(t *testing.T) {
	plan := planFor(cleanRepo())

	if !plan.Empty() {
		t.Errorf("checkpoint = %v, want none", names(plan.Checkpoint))
	}
}

func TestPlanStartUpdatesCleanAndUnpushedRepositories(t *testing.T) {
	unpushed := cleanRepo()
	unpushed.Path = "/w/unpushed"
	unpushed.RelativePath = "unpushed"
	unpushed.CommitsAhead = 2

	stashed := cleanRepo()
	stashed.Path = "/w/stashed"
	stashed.RelativePath = "stashed"
	stashed.StashCount = 1

	plan := PlanStart(Assess([]repository.RepositoryStatusResult{cleanRepo(), unpushed, stashed}))

	if got := len(plan.Update); got != 3 {
		t.Errorf("update = %v, want all three — a rebase replays unpushed commits and ignores stashes", names(plan.Update))
	}
	if len(plan.Skipped) != 0 {
		t.Errorf("skipped = %v, want none", names(plan.Skipped))
	}
}

func TestPlanStartSkipsUncommittedWork(t *testing.T) {
	plan := PlanStart(Assess([]repository.RepositoryStatusResult{dirty("dirty")}))

	if len(plan.Update) != 0 {
		t.Errorf("update = %v, want none — a rebase must not run over uncommitted work", names(plan.Update))
	}
	if got := names(plan.Skipped); len(got) != 1 || got[0] != "dirty" {
		t.Errorf("skipped = %v, want [dirty]", got)
	}
}

func TestPlanStartSkipsRepositoriesWithNothingToPullFrom(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*repository.RepositoryStatusResult)
	}{
		{"no-remote", func(r *repository.RepositoryStatusResult) { r.RemoteURL = "" }},
		{"no-upstream", func(r *repository.RepositoryStatusResult) { r.Status = repository.StatusNoUpstream }},
		{"detached", func(r *repository.RepositoryStatusResult) { r.Branch = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := cleanRepo()
			r.RelativePath = tt.name
			tt.mutate(&r)

			plan := PlanStart(Assess([]repository.RepositoryStatusResult{r}))

			if got := names(plan.Skipped); len(got) != 1 || got[0] != tt.name {
				t.Errorf("skipped = %v, want [%s]", got, tt.name)
			}
		})
	}
}

func TestPaths(t *testing.T) {
	plan := planFor(dirty("a"), dirty("b"))

	got := Paths(plan.Checkpoint)
	if len(got) != 2 || got[0] != "/w/a" || got[1] != "/w/b" {
		t.Errorf("Paths() = %v, want [/w/a /w/b]", got)
	}
}
