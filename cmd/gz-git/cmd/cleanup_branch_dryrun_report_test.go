// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// mixedBlockedResult is the shape the reporting gap lived in: one repository
// that would delete a branch and is also refusing one. The all-blocked variant
// is a StatusError and always printed its refusals; only this mixed shape
// reached the operator as a bare count.
func mixedBlockedResult(status, message string) *repository.BulkCleanupResult {
	return &repository.BulkCleanupResult{
		TotalScanned:          1,
		TotalProcessed:        1,
		TotalBranchesAnalyzed: 2,
		Repositories: []repository.RepositoryCleanupResult{
			{
				RelativePath:    "repos/foo",
				Status:          status,
				Message:         message,
				DeletedBranches: []string{"feature/x"},
				FailedBranches: []repository.CleanupFailureEntry{
					{
						Name:     "master",
						Reason:   "non-canonical",
						Location: "local",
						Error:    "master moved on the remote since the last fetch — run git fetch origin and re-run",
					},
				},
			},
		},
	}
}

// The refusal text is the entire remedy. It names the branch and the command
// that unblocks it, and in a mixed repository the dry run used to print neither
// — only "1 blocked", which tells the operator that something is wrong and
// nothing about what.
func TestPrintBulkCleanup_DryRunPrintsRefusalDetail(t *testing.T) {
	resetStatusFlags(t)
	quiet = false
	verbose = false

	out, errOut := captureOutErr(t, func() {
		printBulkCleanupBranchResult(
			mixedBlockedResult(repository.StatusWouldCleanup, "Would delete 1 branch(es), 1 blocked"),
			true,
		)
	})

	// The per-branch remedy is a diagnostic and goes to stderr; the summary
	// count is part of the report and stays on stdout. Asserting each on its own
	// stream is what keeps the two from quietly swapping places.
	for _, want := range []string{"master", "git fetch origin"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("refusal detail is missing %q from stderr\n---\n%s", want, errOut)
		}
	}
	if !strings.Contains(out, "Blocked: 1 branch(es)") {
		t.Errorf("dry-run summary omits the blocked total\n---\n%s", out)
	}
}

// The point is not that the preview prints something, but that it prints what
// the run prints. Asserting agreement rather than presence means a change that
// makes either side quieter, or louder, fails here.
func TestPrintBulkCleanup_DryRunAndRunReportSameRefusal(t *testing.T) {
	resetStatusFlags(t)
	quiet = false
	verbose = false

	_, preview := captureOutErr(t, func() {
		printBulkCleanupBranchResult(
			mixedBlockedResult(repository.StatusWouldCleanup, "Would delete 1 branch(es), 1 blocked"),
			true,
		)
	})
	_, executed := captureOutErr(t, func() {
		printBulkCleanupBranchResult(
			mixedBlockedResult(repository.StatusCleanedUp, "Deleted 1 branch(es), 1 failed"),
			false,
		)
	})

	const refusal = "✗ repos/foo: master (local) — master moved on the remote"
	if !strings.Contains(preview, refusal) {
		t.Errorf("preview omits the refusal line\n---\n%s", preview)
	}
	if !strings.Contains(executed, refusal) {
		t.Errorf("run omits the refusal line\n---\n%s", executed)
	}
}

// --quiet and the output stream are two different axes, and TASK-179 conflated
// them. "The exit status is what a quiet caller reads" was the reasoning then,
// and it was wrong: a mixed repository is StatusWouldCleanup, so the exit status
// is zero, and quiet took the only remaining report with it.
func TestPrintBulkCleanup_QuietKeepsRefusalOnStderr(t *testing.T) {
	resetStatusFlags(t)
	quiet = true
	verbose = false

	out, errOut := captureOutErr(t, func() {
		printBulkCleanupBranchResult(
			mixedBlockedResult(repository.StatusWouldCleanup, "Would delete 1 branch(es), 1 blocked"),
			true,
		)
	})

	// --quiet silences progress, which lives on stdout.
	if strings.Contains(out, "master moved on the remote") {
		t.Errorf("refusal detail is on stdout, where --quiet is entitled to drop it\n---\n%s", out)
	}
	// It does not silence diagnostics. The operator who wants these gone has
	// 2>/dev/null; the one who passed --quiet to a script asked for less
	// progress, not for a refusal to go unreported on every channel at once.
	if !strings.Contains(errOut, "master moved on the remote") {
		t.Errorf("--quiet swallowed the refusal entirely\n---\n%s", errOut)
	}
}

// The blocked total counts would-cleanup repositories only. A fully blocked
// repository is a StatusError and is already counted there; adding it to both
// would report one refusal twice under two different names.
func TestPrintBulkCleanup_BlockedTotalExcludesErrorRepos(t *testing.T) {
	resetStatusFlags(t)
	quiet = false
	verbose = false

	res := mixedBlockedResult(repository.StatusError, "Nothing deletable, 1 blocked")
	res.Repositories[0].DeletedBranches = nil

	out, errOut := captureOutErr(t, func() { printBulkCleanupBranchResult(res, true) })

	if strings.Contains(out, "Blocked:") {
		t.Errorf("error repo was double-counted into the blocked total\n---\n%s", out)
	}
	// Excluded from the total, but never from the report — a fully blocked
	// repository is the case where the remedy matters most.
	if !strings.Contains(errOut, "master moved on the remote") {
		t.Errorf("error repo lost its refusal detail\n---\n%s", errOut)
	}
	if !strings.Contains(out, "Errors: 1") {
		t.Errorf("error repo missing from the error total\n---\n%s", out)
	}
}
