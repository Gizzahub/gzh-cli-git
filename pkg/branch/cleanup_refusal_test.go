// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package branch

import (
	"context"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// divergedTrunkFixture builds the case the operator cannot currently see: the
// declaration names develop, a local master is still here, and master carries a
// commit develop does not. The ancestry gate correctly refuses it — the whole
// question is whether the operator is ever told so.
//
// Nothing about this fixture is unusual. It is what a repository looks like
// after the trunk was renamed and one commit landed on the old name afterwards,
// which is precisely when someone runs cleanup to tidy up.
func divergedTrunkFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	gitCommit(t, dir, "init", "-q", "-b", "master")
	gitCommit(t, dir, "config", "user.email", "t@t")
	gitCommit(t, dir, "config", "user.name", "t")
	gitCommit(t, dir, "commit", "-q", "--allow-empty", "-m", "init")
	gitCommit(t, dir, "checkout", "-q", "-b", "develop")
	gitCommit(t, dir, "commit", "-q", "--allow-empty", "-m", "move to develop")

	// The commit that makes master unretirable: it exists on master and nowhere
	// else, so retiring master would drop it.
	gitCommit(t, dir, "checkout", "-q", "master")
	gitCommit(t, dir, "commit", "-q", "--allow-empty", "-m", "stray work on the old trunk")
	gitCommit(t, dir, "checkout", "-q", "develop")

	gitCommit(t, dir, "update-ref", "refs/remotes/origin/develop", gitOutput(t, dir, "rev-parse", "develop"))

	return dir
}

func divergedTrunkOptions() AnalyzeOptions {
	return AnalyzeOptions{
		IncludeNonCanonical: true,
		CanonicalBranch:     "develop",
		CanonicalRemote:     "origin",
	}
}

// The gate's verdict is unchanged and must stay unchanged: master still holds a
// commit develop lacks, so it is not retirable. What is asserted here is the
// second return value — the reason — because a verdict with no reason is what
// reached the operator as silence.
func TestClassifyNonCanonical_DivergedTrunkYieldsReason(t *testing.T) {
	dir := divergedTrunkFixture(t)
	svc := newTestCleanupService(t)

	retirable, _, refusal := svc.classifyNonCanonical(
		context.Background(),
		&repository.Repository{Path: dir},
		&Branch{Name: "master", Ref: "refs/heads/master"},
		divergedTrunkOptions(),
	)

	if retirable {
		t.Fatal("master carries a commit develop does not; retiring it would drop that commit")
	}
	if refusal == "" {
		t.Fatal("the gate refused master and said nothing; that silence is the defect")
	}
	// The reason has to name what was compared, not merely that something was.
	// "not retirable" would satisfy a presence check and help nobody.
	if !strings.Contains(refusal, "develop") {
		t.Errorf("refusal does not name the canonical branch it measured against: %q", refusal)
	}
	if !strings.Contains(refusal, "holds commits") {
		t.Errorf("refusal does not say what is wrong with the branch: %q", refusal)
	}
}

// An eligible trunk that passes the gate produces no refusal. Without this the
// obvious way to make the test above pass is to refuse everything, which would
// bury the real case under a line for every clean repository.
func TestClassifyNonCanonical_RetirableTrunkYieldsNoReason(t *testing.T) {
	dir := t.TempDir()
	gitCommit(t, dir, "init", "-q", "-b", "master")
	gitCommit(t, dir, "config", "user.email", "t@t")
	gitCommit(t, dir, "config", "user.name", "t")
	gitCommit(t, dir, "commit", "-q", "--allow-empty", "-m", "init")
	gitCommit(t, dir, "checkout", "-q", "-b", "develop")
	gitCommit(t, dir, "commit", "-q", "--allow-empty", "-m", "move to develop")
	gitCommit(t, dir, "update-ref", "refs/remotes/origin/develop", gitOutput(t, dir, "rev-parse", "develop"))

	retirable, target, refusal := newTestCleanupService(t).classifyNonCanonical(
		context.Background(),
		&repository.Repository{Path: dir},
		&Branch{Name: "master", Ref: "refs/heads/master"},
		divergedTrunkOptions(),
	)
	if !retirable {
		t.Fatal("master is an ancestor of develop and must classify as retirable")
	}
	if refusal != "" {
		t.Errorf("a retirable trunk must produce no refusal line, got %q", refusal)
	}
	// The same call that authorizes the deletion is what names the ref that
	// authorized it. Returning the verdict without the target is how the
	// preview came to print a name with no basis behind it.
	if target != "refs/heads/develop" {
		t.Errorf("classification must report what it measured against, got %q", target)
	}
}

// A branch that is not a trunk name never reaches the gate and must stay
// silent. This is the noise guard: report every non-candidate and the one line
// that matters is lost among the feature branches.
func TestClassifyNonCanonical_FeatureBranchStaysSilent(t *testing.T) {
	dir := divergedTrunkFixture(t)
	svc := newTestCleanupService(t)

	_, _, refusal := svc.classifyNonCanonical(
		context.Background(),
		&repository.Repository{Path: dir},
		&Branch{Name: "feature/x", Ref: "refs/heads/feature/x"},
		divergedTrunkOptions(),
	)
	if refusal != "" {
		t.Errorf("a non-trunk branch is not a thwarted retirement; got %q", refusal)
	}
}

// Analyze must file the refused trunk under Refused and not under Protected.
//
// Protected is where it landed before, and that is the misreport this card
// exists to fix: RetirableTrunkNames is a subset of ProtectedBranches, so the
// operator who passed --non-canonical to look past name protection was answered
// with name protection. Asserting both halves — present in one, absent from the
// other — is what makes this fail if the routing is put back.
func TestAnalyze_RefusedTrunkIsNotReportedAsProtected(t *testing.T) {
	dir := divergedTrunkFixture(t)
	svc := newTestCleanupService(t)

	report, err := svc.Analyze(context.Background(), &repository.Repository{Path: dir}, divergedTrunkOptions())
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}

	found := false
	for _, r := range report.Refused {
		if r.Branch == "master" {
			found = true
			if r.Reason == "" {
				t.Error("a refusal with no reason is the silence this replaces")
			}
		}
	}
	if !found {
		t.Fatalf("master was examined and declined but is absent from Refused (%d entries)", len(report.Refused))
	}

	for _, b := range report.Protected {
		if b.Name == "master" {
			t.Error("master is reported as protected; that is the label --non-canonical was passed to look past")
		}
	}

	// A refusal is not a candidate and must not inflate the deletion count.
	if report.CountBranches() != 0 {
		t.Errorf("refusals must not count as branches to clean up, got %d", report.CountBranches())
	}
}
