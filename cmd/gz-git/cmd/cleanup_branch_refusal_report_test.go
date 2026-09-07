// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/branch"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

const declinedReason = "holds commits develop does not, measured against refs/heads/develop;" +
	" retiring it would drop them — merge it or delete it deliberately"

// declinedOnlyResult is the shape that reached the operator as silence: nothing
// to delete, and a trunk that was examined and turned down. Before this change
// the run printed the word "clean" and no more.
func declinedOnlyResult() *repository.BulkCleanupResult {
	return &repository.BulkCleanupResult{
		TotalScanned:          1,
		TotalProcessed:        1,
		TotalBranchesAnalyzed: 2,
		Repositories: []repository.RepositoryCleanupResult{
			{
				RelativePath: "repos/foo",
				Status:       repository.StatusNothingToDo,
				Message:      "No branches to clean up (1 trunk(s) examined and declined)",
				RetireRefusals: []repository.RetireRefusalEntry{
					{Name: "master", Location: "local", Reason: declinedReason},
				},
			},
		},
	}
}

// StatusNothingToDo prints nothing at all, which is how "examined and declined"
// arrived looking identical to "already clean". The refusal has to escape that
// branch of the switch.
func TestPrintBulkCleanup_DeclinedTrunkIsReportedOnNothingToDo(t *testing.T) {
	resetStatusFlags(t)
	quiet = false
	verbose = false

	_, errOut := captureOutErr(t, func() {
		printBulkCleanupBranchResult(declinedOnlyResult(), true)
	})

	for _, want := range []string{"master", "holds commits develop does not"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("declined trunk is missing %q from stderr\n---\n%s", want, errOut)
		}
	}
}

// --quiet silences progress on stdout, not diagnostics on stderr. This is the
// distinction TASK-181 settled for the gate refusals, and a classification
// refusal is the same kind of thing: the operator who wants it gone has
// 2>/dev/null, which is per-stream rather than all-or-nothing.
func TestPrintBulkCleanup_DeclinedTrunkSurvivesQuiet(t *testing.T) {
	resetStatusFlags(t)
	quiet = true
	verbose = false

	out, errOut := captureOutErr(t, func() {
		printBulkCleanupBranchResult(declinedOnlyResult(), true)
	})

	if !strings.Contains(errOut, "master") {
		t.Errorf("--quiet swallowed a diagnostic\n---\n%s", errOut)
	}
	if strings.Contains(out, "holds commits develop does not") {
		t.Errorf("the refusal text belongs on stderr, not stdout\n---\n%s", out)
	}
}

func declinedReport() *branch.CleanupReport {
	return &branch.CleanupReport{
		Total: 2,
		Refused: []branch.RetireRefusal{
			{Branch: "master", Reason: declinedReason},
		},
	}
}

// The single engine's half of the same guarantee.
func TestPrintReportRefusals_WritesToStderr(t *testing.T) {
	resetStatusFlags(t)
	quiet = false
	verbose = false

	out, errOut := captureOutErr(t, func() { printReportRefusals(declinedReport()) })

	if !strings.Contains(errOut, "master") || !strings.Contains(errOut, "holds commits develop does not") {
		t.Errorf("single-repo refusal missing from stderr\n---\n%s", errOut)
	}
	if strings.Contains(out, "master") {
		t.Errorf("single-repo refusal leaked to stdout\n---\n%s", out)
	}
}

// The binding that keeps --quiet honest for the single engine.
//
// printCleanupBranchReport is the stdout report and its caller gates it on
// --quiet, so a refusal printed from inside it would be silenced with
// everything else. Asserting the refusal is absent here is what fails if the
// block is moved back in — the split is the fix, and presence checks alone
// would not notice it being undone.
func TestPrintCleanupBranchReport_DoesNotCarryRefusals(t *testing.T) {
	resetStatusFlags(t)
	quiet = false
	verbose = false

	out, errOut := captureOutErr(t, func() { printCleanupBranchReport(declinedReport(), true) })

	if strings.Contains(out, "holds commits develop does not") ||
		strings.Contains(errOut, "holds commits develop does not") {
		t.Errorf("the quiet-gated report must not be where refusals are printed\nstdout:\n%s\nstderr:\n%s", out, errOut)
	}
}
