// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package branch

import (
	"context"
	"slices"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// nonCanonicalFixture builds a repository whose checked-out branch "trunk" is
// the declared canonical branch. Every other branch below is created at
// trunk's initial commit, before trunk gains one more commit of its own — so
// each duplicate is a strict ancestor of trunk, not merely equal to it, which
// is what production's isAncestorOf actually asks git.
//
//   - "master": a duplicate carrying a built-in protected name. This is the
//     headline case the whole feature exists for.
//   - "staging": a duplicate reserved for the --protect (Exclude) test.
//   - "dev/a/b/c" and "dev/p/q/r/s": duplicates reserved for the TaskPatterns
//     test — the second, deeper one proves the namespace-prefix match, not
//     just an exact pattern match. They deliberately do not share a leading
//     path segment past "dev/", since a name that is a prefix directory of
//     another (e.g. "dev/a/b/c" and "dev/a/b/c/d") cannot coexist as two
//     refs/heads refs.
//   - "feature-ahead": NOT a duplicate. It carries a commit trunk lacks, so it
//     must never be classified regardless of what else is asked for.
//   - refs/remotes/origin/trunk: the canonical branch's own remote-tracking
//     spelling, so "the canonical branch is never classified" can be checked
//     under both spellings when IncludeRemote is set.
func nonCanonicalFixture(t *testing.T) string {
	t.Helper()

	dir := testutil.TempGitRepoWithCommit(t)
	gitCommit(t, dir, "branch", "-M", "trunk")

	gitCommit(t, dir, "branch", "master")
	gitCommit(t, dir, "branch", "staging")
	gitCommit(t, dir, "branch", "dev/a/b/c")
	gitCommit(t, dir, "branch", "dev/p/q/r/s")

	gitCommit(t, dir, "checkout", "-b", "feature-ahead")
	writeAndCommit(t, dir, "ahead.txt", "ahead")
	gitCommit(t, dir, "checkout", "trunk")

	// trunk now carries a commit none of the duplicates have, which is what
	// makes them strict ancestors rather than an untested edge case of
	// "identical branch".
	writeAndCommit(t, dir, "trunk-only.txt", "trunk moves on")

	trunkSHA := gitOutput(t, dir, "rev-parse", "trunk")
	gitCommit(t, dir, "update-ref", "refs/remotes/origin/trunk", trunkSHA)

	return dir
}

func analyzeNonCanonical(t *testing.T, dir string, opts AnalyzeOptions) *CleanupReport {
	t.Helper()
	repo := &repository.Repository{Path: dir}
	report, err := NewCleanupService().Analyze(context.Background(), repo, opts)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	return report
}

// TestCleanupService_AnalyzeNonCanonicalClassifiesProtectedDuplicate is the
// headline behavior: a local branch named "master" — a built-in protected
// name — is classified NonCanonical when it is an ancestor of the declared
// canonical branch. NonCanonical is the one bucket the classification is
// allowed to reach past IsProtected.
func TestCleanupService_AnalyzeNonCanonicalClassifiesProtectedDuplicate(t *testing.T) {
	dir := nonCanonicalFixture(t)

	report := analyzeNonCanonical(t, dir, AnalyzeOptions{
		IncludeNonCanonical: true,
		CanonicalBranch:     "trunk",
	})

	got := namesOf(report.NonCanonical)
	if !slices.Contains(got, "master") {
		t.Fatalf("NonCanonical = %v, want it to contain master", got)
	}
	if slices.Contains(got, "feature-ahead") {
		t.Errorf("NonCanonical = %v, want feature-ahead excluded (not an ancestor)", got)
	}
	if slices.Contains(got, "trunk") {
		t.Errorf("NonCanonical = %v, want the canonical branch itself excluded", got)
	}
}

// TestCleanupService_AnalyzeNonCanonicalRequiresDeclaration pins condition 1:
// without a declared CanonicalBranch there is no baseline, so nothing is ever
// classified — even though master duplicates trunk in this fixture.
func TestCleanupService_AnalyzeNonCanonicalRequiresDeclaration(t *testing.T) {
	dir := nonCanonicalFixture(t)

	report := analyzeNonCanonical(t, dir, AnalyzeOptions{
		IncludeNonCanonical: true,
		CanonicalBranch:     "",
	})

	if len(report.NonCanonical) != 0 {
		t.Errorf("NonCanonical = %v, want empty when CanonicalBranch is unset", namesOf(report.NonCanonical))
	}
}

// TestCleanupService_AnalyzeNonCanonicalNeverClassifiesCanonicalItself pins
// condition 2 under both spellings git can report it: the bare local name and
// the remotes/origin/<name> remote-tracking ref.
func TestCleanupService_AnalyzeNonCanonicalNeverClassifiesCanonicalItself(t *testing.T) {
	dir := nonCanonicalFixture(t)

	report := analyzeNonCanonical(t, dir, AnalyzeOptions{
		IncludeNonCanonical: true,
		CanonicalBranch:     "trunk",
		IncludeRemote:       true,
	})

	for _, b := range report.NonCanonical {
		if b.Name == "trunk" {
			t.Errorf("NonCanonical contains trunk (IsRemote=%v); the canonical branch must never classify itself", b.IsRemote)
		}
	}
}

// TestCleanupService_AnalyzeNonCanonicalExcludesAheadBranch pins condition 4:
// a branch carrying a commit the canonical branch lacks is not a duplicate and
// must never be classified, no matter how old or oddly named.
func TestCleanupService_AnalyzeNonCanonicalExcludesAheadBranch(t *testing.T) {
	dir := nonCanonicalFixture(t)

	report := analyzeNonCanonical(t, dir, AnalyzeOptions{
		IncludeNonCanonical: true,
		CanonicalBranch:     "trunk",
	})

	if slices.Contains(namesOf(report.NonCanonical), "feature-ahead") {
		t.Error("feature-ahead was classified NonCanonical, but it carries a commit trunk lacks")
	}
}

// TestCleanupService_AnalyzeNonCanonicalExcludesTaskPatternMatch pins
// condition 3's task-branch half: a declared TaskPatterns entry routes a
// branch to the reclaim path, even when it also happens to duplicate the
// canonical branch. The deeper "dev/a/b/c/d" proves the namespace-prefix
// match (DECISION-004), not just an exact-string match on the pattern.
func TestCleanupService_AnalyzeNonCanonicalExcludesTaskPatternMatch(t *testing.T) {
	dir := nonCanonicalFixture(t)

	report := analyzeNonCanonical(t, dir, AnalyzeOptions{
		IncludeNonCanonical: true,
		CanonicalBranch:     "trunk",
		TaskPatterns:        []string{"dev/*/*/*"},
	})

	got := namesOf(report.NonCanonical)
	if slices.Contains(got, "dev/a/b/c") {
		t.Errorf("NonCanonical = %v, want dev/a/b/c excluded as a declared task branch", got)
	}
	if slices.Contains(got, "dev/p/q/r/s") {
		t.Errorf("NonCanonical = %v, want dev/p/q/r/s excluded (namespace-prefix match)", got)
	}
}

// TestCleanupService_AnalyzeNonCanonicalHonoursExcludePattern pins the
// --protect half of condition 3: an operator-supplied Exclude pattern is
// honored even for a candidate that would otherwise qualify.
func TestCleanupService_AnalyzeNonCanonicalHonoursExcludePattern(t *testing.T) {
	dir := nonCanonicalFixture(t)

	report := analyzeNonCanonical(t, dir, AnalyzeOptions{
		IncludeNonCanonical: true,
		CanonicalBranch:     "trunk",
		Exclude:             []string{"staging"},
	})

	if slices.Contains(namesOf(report.NonCanonical), "staging") {
		t.Error("staging was classified NonCanonical despite matching an Exclude pattern")
	}
}

// TestCleanupService_AnalyzeNonCanonicalDisabledByDefault pins the flag to the
// bucket: IncludeNonCanonical: false must leave it empty even though a
// canonical branch is declared and a duplicate exists.
func TestCleanupService_AnalyzeNonCanonicalDisabledByDefault(t *testing.T) {
	dir := nonCanonicalFixture(t)

	report := analyzeNonCanonical(t, dir, AnalyzeOptions{
		IncludeNonCanonical: false,
		CanonicalBranch:     "trunk",
	})

	if len(report.NonCanonical) != 0 {
		t.Errorf("NonCanonical = %v, want empty when IncludeNonCanonical is false", namesOf(report.NonCanonical))
	}
}

// executeNonCanonicalAncestorFixture builds a repo where local branch
// "master" is a genuine ancestor of "trunk" — the case Execute is allowed to
// retire.
func executeNonCanonicalAncestorFixture(t *testing.T) string {
	t.Helper()

	dir := testutil.TempGitRepoWithCommit(t)
	gitCommit(t, dir, "branch", "-M", "trunk")
	gitCommit(t, dir, "branch", "master")
	writeAndCommit(t, dir, "trunk-only.txt", "trunk moves on")

	return dir
}

// executeNonCanonicalDivergentFixture builds a repo where local branch
// "master" carries a commit "trunk" lacks — not an ancestor, and Execute must
// refuse to retire it even when handed a report that claims otherwise.
func executeNonCanonicalDivergentFixture(t *testing.T) string {
	t.Helper()

	dir := testutil.TempGitRepoWithCommit(t)
	gitCommit(t, dir, "branch", "-M", "trunk")
	gitCommit(t, dir, "checkout", "-b", "master")
	writeAndCommit(t, dir, "master-only.txt", "diverges")
	gitCommit(t, dir, "checkout", "trunk")

	return dir
}

// TestCleanupService_ExecuteNonCanonicalRefusesWithoutCanonicalBranch is the
// defense-in-depth headline: a hand-assembled CleanupReport claiming "master"
// is NonCanonical must not delete it when ExecuteOptions.CanonicalBranch is
// empty. Analyze is not consulted here on purpose — CleanupReport is a public
// type, and a caller who builds one directly must not be able to turn this
// feature into "delete master".
func TestCleanupService_ExecuteNonCanonicalRefusesWithoutCanonicalBranch(t *testing.T) {
	dir := executeNonCanonicalAncestorFixture(t)
	repo := &repository.Repository{Path: dir}
	ctx := context.Background()

	report := &CleanupReport{NonCanonical: []*Branch{{Name: "master"}}}

	result, err := NewCleanupService().Execute(ctx, repo, report, ExecuteOptions{
		Force:           true,
		CanonicalBranch: "",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !slices.Contains(result.Skipped, "master") {
		t.Errorf("Skipped = %v, want it to contain master", result.Skipped)
	}
	if len(result.Deleted) != 0 {
		t.Errorf("Deleted = %v, want none — CanonicalBranch was empty", result.Deleted)
	}

	exists, err := NewManager().Exists(ctx, repo, "master")
	if err != nil {
		t.Fatalf("Exists(master): %v", err)
	}
	if !exists {
		t.Error("master was deleted despite an empty ExecuteOptions.CanonicalBranch")
	}
}

// TestCleanupService_ExecuteNonCanonicalRefusesNonAncestor re-verifies
// ancestry inside Execute itself: even with a valid CanonicalBranch, a
// hand-assembled report naming a branch that is not actually an ancestor must
// be skipped, not deleted.
func TestCleanupService_ExecuteNonCanonicalRefusesNonAncestor(t *testing.T) {
	dir := executeNonCanonicalDivergentFixture(t)
	repo := &repository.Repository{Path: dir}
	ctx := context.Background()

	report := &CleanupReport{NonCanonical: []*Branch{{Name: "master"}}}

	result, err := NewCleanupService().Execute(ctx, repo, report, ExecuteOptions{
		Force:           true,
		CanonicalBranch: "trunk",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !slices.Contains(result.Skipped, "master") {
		t.Errorf("Skipped = %v, want it to contain master", result.Skipped)
	}
	if len(result.Deleted) != 0 {
		t.Errorf("Deleted = %v, want none — master is not an ancestor of trunk", result.Deleted)
	}
}

// TestCleanupService_ExecuteNonCanonicalDeletesRealAncestor is the positive
// case: a valid CanonicalBranch plus a branch that really is an ancestor is
// deleted.
func TestCleanupService_ExecuteNonCanonicalDeletesRealAncestor(t *testing.T) {
	dir := executeNonCanonicalAncestorFixture(t)
	repo := &repository.Repository{Path: dir}
	ctx := context.Background()

	report := &CleanupReport{NonCanonical: []*Branch{{Name: "master"}}}

	result, err := NewCleanupService().Execute(ctx, repo, report, ExecuteOptions{
		Force:           true,
		CanonicalBranch: "trunk",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !slices.Contains(result.Deleted, "master") {
		t.Fatalf("Deleted = %v, want it to contain master", result.Deleted)
	}
	if len(result.Skipped) != 0 {
		t.Errorf("Skipped = %v, want none", result.Skipped)
	}

	exists, err := NewManager().Exists(ctx, repo, "master")
	if err != nil {
		t.Fatalf("Exists(master): %v", err)
	}
	if exists {
		t.Error("master should have been deleted")
	}
}

// TestCleanupService_ExecuteNonCanonicalHonoursExcludePattern proves an
// operator --protect pattern still blocks a NonCanonical entry inside
// Execute, even with a valid CanonicalBranch and a genuine ancestor.
func TestCleanupService_ExecuteNonCanonicalHonoursExcludePattern(t *testing.T) {
	dir := executeNonCanonicalAncestorFixture(t)
	repo := &repository.Repository{Path: dir}
	ctx := context.Background()

	report := &CleanupReport{NonCanonical: []*Branch{{Name: "master"}}}

	result, err := NewCleanupService().Execute(ctx, repo, report, ExecuteOptions{
		Force:           true,
		CanonicalBranch: "trunk",
		Exclude:         []string{"master"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !slices.Contains(result.Skipped, "master") {
		t.Errorf("Skipped = %v, want it to contain master", result.Skipped)
	}
	if len(result.Deleted) != 0 {
		t.Errorf("Deleted = %v, want none — master matches an Exclude pattern", result.Deleted)
	}

	exists, err := NewManager().Exists(ctx, repo, "master")
	if err != nil {
		t.Fatalf("Exists(master): %v", err)
	}
	if !exists {
		t.Error("master was deleted despite matching an Exclude pattern")
	}
}

// remoteOnlyNonCanonicalFixture builds a repo where "master" duplicates the
// canonical branch "trunk" but exists ONLY as a remote-tracking ref
// (refs/remotes/origin/master) — the local branch has already been deleted,
// which is exactly what a previous --non-canonical run leaves behind. This is
// the regression fixture for cleanupBranchRef: parseBranchLine/
// normalizeCleanupBranch report this candidate with Name == "master" (bare)
// and Ref == "refs/remotes/origin/master", and "master" alone does not
// resolve to that ref through git's default unqualified-name lookup rules —
// only a remote literally named "master" would. Passing Name to
// merge-base --is-ancestor therefore fails closed, and (before the fix) a
// built-in protected name like "master" fell through to the Protected bucket
// instead of NonCanonical, where it would never be offered for retirement.
func remoteOnlyNonCanonicalFixture(t *testing.T) string {
	t.Helper()

	dir := testutil.TempGitRepoWithCommit(t)
	gitCommit(t, dir, "branch", "-M", "trunk")
	gitCommit(t, dir, "branch", "master")

	masterSHA := gitOutput(t, dir, "rev-parse", "master")
	gitCommit(t, dir, "update-ref", "refs/remotes/origin/master", masterSHA)
	gitCommit(t, dir, "branch", "-D", "master")

	// trunk must move ahead so the old master commit is a strict ancestor, not
	// merely identical.
	writeAndCommit(t, dir, "trunk-only.txt", "trunk moves on")

	// The remote needs its own copy of the canonical branch. Deleting
	// refs/remotes/origin/master is a claim about the remote, and only
	// refs/remotes/origin/trunk can justify it: a local trunk that is ahead of
	// its own remote would otherwise authorize discarding commits the remote
	// still holds nowhere else.
	trunkSHA := gitOutput(t, dir, "rev-parse", "trunk")
	gitCommit(t, dir, "update-ref", "refs/remotes/origin/trunk", trunkSHA)

	return dir
}

// TestCleanupService_AnalyzeNonCanonicalClassifiesRemoteOnlyDuplicate is the
// regression test for the cleanupBranchRef fix: a duplicate that survives only
// as a remote-tracking ref (no local namesake) must still classify as
// NonCanonical, not fall through to Protected because its bare name happens to
// be a built-in protected name.
func TestCleanupService_AnalyzeNonCanonicalClassifiesRemoteOnlyDuplicate(t *testing.T) {
	dir := remoteOnlyNonCanonicalFixture(t)

	report := analyzeNonCanonical(t, dir, AnalyzeOptions{
		IncludeNonCanonical: true,
		CanonicalBranch:     "trunk",
		IncludeRemote:       true,
	})

	got := namesOf(report.NonCanonical)
	if !slices.Contains(got, "master") {
		t.Fatalf("NonCanonical = %v, want it to contain the remote-only master", got)
	}
	if slices.Contains(namesOf(report.Protected), "master") {
		t.Error("master (remote-only) was classified Protected — the ancestry probe against Name failed to resolve, the bug cleanupBranchRef fixes")
	}
}

// TestCleanupBranchRef pins cleanupBranchRef's preference for Ref over Name: a
// remote branch's Name is shortened to the bare name by normalizeCleanupBranch,
// which no longer resolves once the local namesake is gone, so ancestry checks
// must be asked of Ref.
// newTestCleanupService returns the concrete service so unexported helpers can
// be exercised directly. The interface hands back exactly this type.
func newTestCleanupService(t *testing.T) *cleanupService {
	t.Helper()

	svc, ok := NewCleanupService().(*cleanupService)
	if !ok {
		t.Fatalf("NewCleanupService() returned %T, want *cleanupService", NewCleanupService())
	}

	return svc
}

func TestCleanupBranchRef(t *testing.T) {
	tests := []struct {
		name   string
		branch *Branch
		want   string
	}{
		{"nil branch", nil, ""},
		{"prefers Ref when set", &Branch{Name: "master", Ref: "refs/remotes/origin/master"}, "refs/remotes/origin/master"},
		{"falls back to Name when Ref is empty", &Branch{Name: "master", Ref: ""}, "master"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanupBranchRef(tt.branch); got != tt.want {
				t.Errorf("cleanupBranchRef(%+v) = %q, want %q", tt.branch, got, tt.want)
			}
		})
	}
}

// TestCleanupService_AuthorizeRetireAcceptsRemoteOnlyAncestor is the Execute-side
// half of the same regression: authorizeRetire re-verifies ancestry directly
// (not through Analyze), and a hand-assembled Branch describing a remote-only
// duplicate (Name stripped to "master", Ref carrying the full remote-tracking
// path) must still authorize retirement once Ref is consulted instead of Name.
func TestCleanupService_AuthorizeRetireAcceptsRemoteOnlyAncestor(t *testing.T) {
	dir := remoteOnlyNonCanonicalFixture(t)
	repo := &repository.Repository{Path: dir}
	svc := newTestCleanupService(t)

	branch := &Branch{Name: "master", IsRemote: true, Ref: "refs/remotes/origin/master"}

	if !svc.authorizeRetire(context.Background(), repo, branch, ExecuteOptions{CanonicalBranch: "trunk"}) {
		t.Error("authorizeRetire refused a remote-only ancestor; it should consult Ref, not Name")
	}
}

// TestCleanupService_AuthorizeRetireRefusesRemoteCandidateWithoutRemoteTrunk
// pins the other half of that rule. A remote candidate whose canonical branch
// exists only locally must be refused: the local trunk proves where the commits
// are on this machine, not on the remote the delete would be pushed to.
func TestCleanupService_AuthorizeRetireRefusesRemoteCandidateWithoutRemoteTrunk(t *testing.T) {
	dir := remoteOnlyNonCanonicalFixture(t)
	gitCommit(t, dir, "update-ref", "-d", "refs/remotes/origin/trunk")

	repo := &repository.Repository{Path: dir}
	svc := newTestCleanupService(t)
	branch := &Branch{Name: "master", IsRemote: true, Ref: "refs/remotes/origin/master"}

	if svc.authorizeRetire(context.Background(), repo, branch, ExecuteOptions{CanonicalBranch: "trunk"}) {
		t.Error("authorizeRetire allowed a remote delete justified only by a local trunk")
	}
}

// TestCleanupReport_NonCanonicalCountedInAggregates pins CountBranches,
// IsEmpty and GetAllBranches to account for NonCanonical alongside the other
// four buckets.
func TestCleanupReport_NonCanonicalCountedInAggregates(t *testing.T) {
	report := &CleanupReport{
		Merged:       []*Branch{{Name: "m1"}},
		NonCanonical: []*Branch{{Name: "master"}, {Name: "staging"}},
	}

	if got := report.CountBranches(); got != 3 {
		t.Errorf("CountBranches() = %d, want 3", got)
	}
	if report.IsEmpty() {
		t.Error("IsEmpty() = true, want false — NonCanonical has entries")
	}

	all := namesOf(report.GetAllBranches())
	for _, want := range []string{"m1", "master", "staging"} {
		if !slices.Contains(all, want) {
			t.Errorf("GetAllBranches() = %v, want it to contain %q", all, want)
		}
	}

	empty := &CleanupReport{}
	if !empty.IsEmpty() {
		t.Error("IsEmpty() = false for an empty report, want true")
	}
	if empty.CountBranches() != 0 {
		t.Errorf("CountBranches() = %d for an empty report, want 0", empty.CountBranches())
	}
}
