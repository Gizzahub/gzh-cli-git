// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/branch"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// The one fact under test: this retirement was authorized by a ref on another
// machine, not by the local trunk of the same name. Before the label, both
// cases printed "• master" and the operator could not tell them apart.
const (
	basisTargetRef = "refs/remotes/origin/master"
	basisTargetSHA = "3f1c2b7d9e0a4c5b6d7e8f90a1b2c3d4e5f60718"
	basisWant      = " → contained in refs/remotes/origin/master @ 3f1c2b7d"
)

func basisReport() *branch.CleanupReport {
	return &branch.CleanupReport{
		Total: 1,
		NonCanonical: []*branch.Branch{
			{Name: "master", Ref: "refs/heads/master"},
		},
		Bases: []branch.RetireBasis{
			{Ref: "refs/heads/master", TargetRef: basisTargetRef, TargetSHA: basisTargetSHA},
		},
	}
}

func basisBulkResult() *repository.BulkCleanupResult {
	return &repository.BulkCleanupResult{
		TotalScanned:   1,
		TotalProcessed: 1,
		Repositories: []repository.RepositoryCleanupResult{
			{
				RelativePath:    "repos/foo",
				Status:          repository.StatusWouldCleanup,
				Message:         "Would clean up 1 branch(es)",
				DeletedBranches: []string{"master"},
				Branches: []repository.CleanupBranchEntry{
					{
						Name:      "master",
						Location:  "local",
						TargetRef: basisTargetRef,
						TargetSHA: basisTargetSHA,
					},
				},
			},
		},
	}
}

// Criterion 2, single-repo half.
func TestPrintCleanupBranchReport_CandidateCarriesBasis(t *testing.T) {
	resetStatusFlags(t)
	quiet = false
	verbose = false

	out, _ := captureOutErr(t, func() { printCleanupBranchReport(basisReport(), true) })

	if !strings.Contains(out, basisWant) {
		t.Errorf("single-repo candidate line has no basis\nwant substring %q\n---\n%s", basisWant, out)
	}
}

// Criterion 2, bulk half — and the reason the two are asserted against the
// same constant rather than against each other's shapes: what the card asks
// for is that an operator reading either preview reads the same sentence.
func TestPrintBulkCleanup_CandidateCarriesSameBasis(t *testing.T) {
	resetStatusFlags(t)
	quiet = false
	verbose = false

	out, _ := captureOutErr(t, func() { printBulkCleanupBranchResult(basisBulkResult(), true) })

	if !strings.Contains(out, "master"+basisWant) {
		t.Errorf("bulk candidate line has no basis\nwant substring %q\n---\n%s", "master"+basisWant, out)
	}
}

// A local and a remote copy share a name but are two different refs whose
// bases can differ. Keying the dedup on the name alone would print one of them
// and describe the other; this pins both lines.
func TestCleanupBranchListLabel_KeepsLocalAndRemoteApart(t *testing.T) {
	repo := repository.RepositoryCleanupResult{
		DeletedBranches: []string{"master"},
		Branches: []repository.CleanupBranchEntry{
			{Name: "master", Location: "local", TargetRef: "refs/heads/develop", TargetSHA: basisTargetSHA},
			{Name: "master", Location: "remote", TargetRef: basisTargetRef, TargetSHA: basisTargetSHA},
			{Name: "master", Location: "local", TargetRef: "refs/heads/develop", TargetSHA: basisTargetSHA},
		},
	}

	got := cleanupBranchListLabel(&repo)

	if n := strings.Count(got, ", "); n != 1 {
		t.Errorf("expected exactly two entries — the local and remote copies, the repeat deduped\n---\n%s", got)
	}
	if !strings.Contains(got, "refs/heads/develop") || !strings.Contains(got, basisTargetRef) {
		t.Errorf("both bases must survive the dedup\n---\n%s", got)
	}
}

// A caller that assembled a result without per-branch detail still gets the
// names it used to get, rather than an empty bracket.
func TestCleanupBranchListLabel_FallsBackToNames(t *testing.T) {
	repo := repository.RepositoryCleanupResult{DeletedBranches: []string{"a", "b"}}

	if got := cleanupBranchListLabel(&repo); got != " [a, b]" {
		t.Errorf("fallback list = %q", got)
	}
}

// Question 4 of the card, answered by measurement rather than assumption: the
// machine path was the one that had the fields and did not fill them. A
// consumer diffing the two formats would have found the human line richer than
// the JSON, which is backwards.
func TestCleanupEntriesFromReport_CarriesBasis(t *testing.T) {
	entries := cleanupEntriesFromReport(basisReport())

	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(entries))
	}
	if entries[0].TargetRef != basisTargetRef || entries[0].TargetSHA != basisTargetSHA {
		t.Errorf("JSON entry lost the basis: %+v", entries[0])
	}
}

// Only non-canonical retirements are measured against a target ref; a merged
// branch has none, and inventing one would make the field meaningless.
func TestCleanupEntriesFromReport_NoBasisForOtherReasons(t *testing.T) {
	report := &branch.CleanupReport{
		Merged: []*branch.Branch{{Name: "feature/x", Ref: "refs/heads/feature/x"}},
	}

	entries := cleanupEntriesFromReport(report)

	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(entries))
	}
	if entries[0].TargetRef != "" || entries[0].TargetSHA != "" {
		t.Errorf("a merged branch has no retirement basis: %+v", entries[0])
	}
}
