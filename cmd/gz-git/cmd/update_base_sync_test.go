// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

func TestBaseSyncNote(t *testing.T) {
	tests := []struct {
		name string
		sync *repository.BaseSyncResult
		want string
	}{
		{
			// nil means --sync-base was never asked for. Printing "up to date"
			// here would answer a question nobody posed.
			name: "flag off",
			sync: nil,
			want: "",
		},
		{
			name: "fast-forward",
			sync: &repository.BaseSyncResult{
				Base: "master", Action: repository.BaseSyncFastForward, Advanced: 1275,
			},
			want: "base master +1275",
		},
		{
			name: "adopted is marked, not silently equated to a fast-forward",
			sync: &repository.BaseSyncResult{
				Base: "master", Action: repository.BaseSyncAdopted, Advanced: 3,
				Reason: "2 local commit(s) already pushed elsewhere",
				Backup: "refs/gz-git/base-backup/master",
			},
			want: "base master +3 (adopted: 2 local commit(s) already pushed elsewhere; " +
				"old tip at refs/gz-git/base-backup/master)",
		},
		{
			// The case the review caught: a real adopt off a base that was
			// strictly ahead leaves Advanced at zero because RemoteOnly is zero.
			// Keying the "nothing moved" wording on Advanced rendered the one
			// action that rewinds a ref as a passive remark with no verb.
			name: "a real adopt that only rewinds still says a ref moved",
			sync: &repository.BaseSyncResult{
				Base: "master", Remote: "origin", Action: repository.BaseSyncAdopted,
				Reason: "2 local commit(s) already pushed elsewhere",
				Backup: "refs/gz-git/base-backup/master",
			},
			want: "base master rewound to origin (adopted: 2 local commit(s) already pushed elsewhere; " +
				"old tip at refs/gz-git/base-backup/master)",
		},
		{
			// Advanced is zero on a dry run because nothing moved. Falling
			// through to the "+%d" form would print "base master +0" for the
			// case the dry run exists to preview.
			name: "dry-run fast-forward reports what would happen, not +0",
			sync: &repository.BaseSyncResult{
				Base: "master", Action: repository.BaseSyncFastForward, DryRun: true,
				Reason: "would advance 1275 commits",
			},
			want: "base master would advance 1275 commits",
		},
		{
			name: "dry-run adoption likewise",
			sync: &repository.BaseSyncResult{
				Base: "master", Action: repository.BaseSyncAdopted, DryRun: true,
				Reason: "would adopt remote tip (3 local commit(s) already pushed elsewhere)",
				Backup: "refs/gz-git/base-backup/master",
			},
			want: "base master would adopt remote tip (3 local commit(s) already pushed elsewhere)",
		},
		{
			// A created branch has an Advanced of zero for a different reason:
			// there was no earlier position to advance from.
			name: "created names the commit it was created at",
			sync: &repository.BaseSyncResult{
				Base: "master", Action: repository.BaseSyncCreated, Reason: "created at 5b4ef22a",
			},
			want: "base master created at 5b4ef22a",
		},
		{
			name: "blocked carries the reason",
			sync: &repository.BaseSyncResult{
				Base: "master", Action: repository.BaseSyncBlocked, Reason: "2 local commit(s)",
			},
			want: "base master blocked: 2 local commit(s)",
		},
		{
			// The empty-repository row exactly as it printed before the split:
			// the sync died before resolving anything, so the "base %s" form had
			// nothing to put in the hole and the user read "base  sync failed".
			name: "failed without a resolved base leaves no hole",
			sync: &repository.BaseSyncResult{
				Action: repository.BaseSyncFailed,
				Reason: "failed to read current branch: exit status 128",
			},
			want: "base sync failed: failed to read current branch: exit status 128",
		},
		{
			// Once a base is known, naming it is the useful part of the row —
			// a failure is still about a specific ref.
			name: "failed names the base when one was resolved",
			sync: &repository.BaseSyncResult{
				Base: "master", Action: repository.BaseSyncFailed, Reason: "exit status 128",
			},
			want: "base master sync failed: exit status 128",
		},
		{
			name: "up-to-date is silent",
			sync: &repository.BaseSyncResult{Base: "master", Action: repository.BaseSyncUpToDate},
			want: "",
		},
		{
			name: "skipped is silent",
			sync: &repository.BaseSyncResult{Base: "master", Action: repository.BaseSyncSkipped},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := baseSyncNote(tt.sync); got != tt.want {
				t.Errorf("baseSyncNote() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestUpdateRendersMovedBaseRef pins the reason base-synced is its own status.
//
// A base sync writes to the user's repository. Before this status existed the
// pull verdict stayed "up-to-date", the row fell outside the issue filter, and
// a run that advanced a local ref by a thousand commits printed nothing at all
// about the one repository it changed.
func TestUpdateRendersMovedBaseRef(t *testing.T) {
	in := BulkRenderInput{
		TotalScanned:   2,
		TotalProcessed: 2,
		Duration:       1400 * time.Millisecond,
		Summary:        map[string]int{"up-to-date": 1, "base-synced": 1},
		Rows: []BulkRenderRow{
			{Path: "quiet-repo", Branch: "develop", Status: "up-to-date"},
			{
				Path: "repaired-repo", Branch: "develop",
				Status: repository.StatusBaseSynced, Note: "base master +1275",
			},
		},
	}

	var buf bytes.Buffer
	RenderBulkResults(&buf, BulkRenderConfig{
		Verb:          "Updated",
		Format:        "compact",
		IssueStatuses: issueStatusSet("error", "dirty", "conflict", "base-blocked", "base-synced"),
		FormatStatus:  formatUpdateStatus,
		ChangesCount:  func(row BulkRenderRow) int { return row.CommitsBehind },
	}, in)

	out := buf.String()
	if !strings.Contains(out, "repaired-repo") {
		t.Errorf("the repository whose ref moved is missing from the output:\n%s", out)
	}
	if !strings.Contains(out, "base master +1275") {
		t.Errorf("output does not say which ref moved or how far:\n%s", out)
	}
	// The census stays suppressed: only the exception earns a line.
	if strings.Contains(out, "quiet-repo") {
		t.Errorf("an untouched repository should not be listed:\n%s", out)
	}
}

func TestFormatUpdateStatusBaseRows(t *testing.T) {
	blocked := BulkRenderRow{
		Status: repository.StatusBaseBlocked,
		Note:   "base master blocked: 2 local commit(s)",
	}
	if got := formatUpdateStatus(blocked); got != blocked.Note {
		t.Errorf("formatUpdateStatus(blocked) = %q, want the note %q", got, blocked.Note)
	}

	// A row shown without a note would be a row with no stated reason for being
	// shown, so the fallback still names the finding.
	noteless := BulkRenderRow{Status: repository.StatusBaseBlocked}
	if got := formatUpdateStatus(noteless); got == "" {
		t.Error("a base-blocked row rendered as an empty status")
	}

	// base-failed is a separate status so it stays out of the blocked count, but
	// it still has to render its note. A status the formatter does not know
	// falls through to the generic branch and loses the one line saying why the
	// row is there.
	failed := BulkRenderRow{
		Status: repository.StatusBaseFailed,
		Note:   "base sync failed: failed to read current branch: exit status 128",
	}
	if got := formatUpdateStatus(failed); got != failed.Note {
		t.Errorf("formatUpdateStatus(failed) = %q, want the note %q", got, failed.Note)
	}
}
