// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"testing"
)

// orphanHeadFixture builds the case that distinguishes "no history" from "HEAD
// does not resolve": a repository full of commits sitting on an orphan branch.
//
// `git checkout --orphan` is an ordinary thing to do — starting a gh-pages or
// docs branch — and it leaves HEAD pointing at a branch that does not exist
// yet. Every HEAD-resolving probe answers exactly as it would in a clone of an
// empty remote, while refs/heads/master is still there and still stale.
func orphanHeadFixture(t *testing.T) string {
	t.Helper()
	work := syncBaseFixture(t, 2)
	runGit(t, work, "checkout", "--orphan", "gh-pages")
	if refPresent(t, work, "HEAD") {
		t.Fatal("fixture HEAD is not unborn")
	}
	if !refPresent(t, work, "refs/heads/master") {
		t.Fatal("fixture lost the stale base ref it exists to protect")
	}
	return work
}

// TestSyncBase_OrphanHeadIsNotMistakenForAnEmptyRepository guards the failure
// mode the no-commits guard can introduce if it asks the wrong question.
//
// Reporting this repository as "no commits" would return Skipped, which renders
// as nothing at all — so a base ref that is genuinely two commits stale would
// be passed over in silence, by the very change that exists to stop base-ref
// findings from being mislabelled.
func TestSyncBase_OrphanHeadIsNotMistakenForAnEmptyRepository(t *testing.T) {
	work := orphanHeadFixture(t)

	client := NewClient()
	got, err := client.SyncBase(context.Background(), work, BaseSyncOptions{
		Remote: "origin", Candidates: []string{"master"}, Fetch: false,
	})

	if err == nil {
		t.Fatalf("SyncBase returned no error; Action = %q (%s)", got.Action, got.Reason)
	}
	if got.Reason == "repository has no commits" {
		t.Error("a repository holding commits was reported as empty")
	}
}

// TestApplyBaseSync_ErrorBecomesFailedNotBlocked pins the mapping the whole
// change exists for. Blocked carries an instruction — push these commits, then
// run again — and a repository where git simply broke has no such instruction
// to give.
func TestApplyBaseSync_ErrorBecomesFailedNotBlocked(t *testing.T) {
	work := orphanHeadFixture(t)

	c, ok := NewClient().(*client)
	if !ok {
		t.Fatal("NewClient no longer returns *client; this test needs the concrete type")
	}
	opts := BulkUpdateOptions{SyncBase: true, NoFetch: true, BaseCandidates: []string{"master"}}

	t.Run("a benign pull status is promoted to base-failed", func(t *testing.T) {
		result := RepositoryUpdateResult{Status: StatusUpToDate, Remote: "origin"}
		c.applyBaseSync(context.Background(), work, opts, &result, &noopLogger{})

		if result.BaseSync == nil {
			t.Fatal("no BaseSync result was attached to the row")
		}
		if result.BaseSync.Action != BaseSyncFailed {
			t.Errorf("Action = %q, want %q", result.BaseSync.Action, BaseSyncFailed)
		}
		if result.BaseSync.Reason == "" {
			t.Error("a failed sync carries no reason, so the row cannot say why it is there")
		}
		if result.Status != StatusBaseFailed {
			t.Errorf("Status = %q, want %q", result.Status, StatusBaseFailed)
		}
	})

	t.Run("a real finding is not overwritten by the base result", func(t *testing.T) {
		// The more urgent of the two wins. A conflict relabelled as a base
		// problem is a conflict the user stops seeing.
		result := RepositoryUpdateResult{Status: StatusConflict, Remote: "origin"}
		c.applyBaseSync(context.Background(), work, opts, &result, &noopLogger{})

		if result.Status != StatusConflict {
			t.Errorf("Status = %q, want the pull's own %q preserved", result.Status, StatusConflict)
		}
	})
}
