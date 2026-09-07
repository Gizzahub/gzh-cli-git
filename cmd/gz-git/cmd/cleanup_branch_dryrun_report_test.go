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

	out := captureStdout(t, func() {
		printBulkCleanupBranchResult(
			mixedBlockedResult(repository.StatusWouldCleanup, "Would delete 1 branch(es), 1 blocked"),
			true,
		)
	})

	for _, want := range []string{"master", "git fetch origin", "Blocked: 1 branch(es)"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run report is missing %q\n---\n%s", want, out)
		}
	}
}

// The point is not that the preview prints something, but that it prints what
// the run prints. Asserting agreement rather than presence means a change that
// makes either side quieter, or louder, fails here.
func TestPrintBulkCleanup_DryRunAndRunReportSameRefusal(t *testing.T) {
	resetStatusFlags(t)
	quiet = false
	verbose = false

	preview := captureStdout(t, func() {
		printBulkCleanupBranchResult(
			mixedBlockedResult(repository.StatusWouldCleanup, "Would delete 1 branch(es), 1 blocked"),
			true,
		)
	})
	executed := captureStdout(t, func() {
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

// --quiet is an explicit instruction to say less, and it outranks the rule
// above. The failure detail is suppressed with everything else; the exit status
// is what a quiet caller reads.
func TestPrintBulkCleanup_QuietSuppressesRefusalDetail(t *testing.T) {
	resetStatusFlags(t)
	quiet = true
	verbose = false

	out := captureStdout(t, func() {
		printBulkCleanupBranchResult(
			mixedBlockedResult(repository.StatusWouldCleanup, "Would delete 1 branch(es), 1 blocked"),
			true,
		)
	})

	if strings.Contains(out, "master moved on the remote") {
		t.Errorf("--quiet still printed the refusal detail\n---\n%s", out)
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

	out := captureStdout(t, func() { printBulkCleanupBranchResult(res, true) })

	if strings.Contains(out, "Blocked:") {
		t.Errorf("error repo was double-counted into the blocked total\n---\n%s", out)
	}
	if !strings.Contains(out, "master moved on the remote") {
		t.Errorf("error repo lost its refusal detail\n---\n%s", out)
	}
	if !strings.Contains(out, "Errors: 1") {
		t.Errorf("error repo missing from the error total\n---\n%s", out)
	}
}
