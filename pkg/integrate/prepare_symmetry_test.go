// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"context"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)

// TestPrepareSymmetry pins the property TASK-140 is about: with no controller
// profile the two probes are NOT measured in the same kind of tree, and that
// difference has to be reported as the reason rather than left as an unstated
// premise.
//
// The branch probe runs where the repository already is — the live working
// directory, carrying deps/, node_modules/ and .venv from earlier runs — while
// the baseline probe runs in a worktree checked out fresh at the target SHA
// and bootstrapped by nothing. Running the same `make` target in those two
// trees is not the same experiment, so a bootstrap-hungry checker dies on the
// baseline side only. Reporting that as "the target tip failed" points the
// operator at a commit that is innocent.
//
// This repository's own gate is what these probes decide, so the asymmetry is
// recorded rather than removed: symmetrizing would charge every gate run a
// full bootstrap. The stated reason is the deliverable, and it only exists if
// every link in the chain stamps its tree — prepareLegacyTrees, annotateProbe,
// baselineAgainstTarget, BaselineInput, unmeasurableReason. The end-to-end
// subtest below is the one that holds all five at once.
func TestPrepareSymmetry(t *testing.T) {
	t.Run("no controller profile stamps the live working directory", func(t *testing.T) {
		// c == nil returns before any git or filesystem work, so a bare
		// temp dir is enough and this stays a unit test.
		g := newGitRepo(gitcmd.NewExecutor(), t.TempDir())
		prepared, err := prepareLegacyTrees(context.Background(), g, TargetPlan{}, nil)
		if err != nil {
			t.Fatalf("prepareLegacyTrees: %v", err)
		}
		if prepared.sourcePrepared != PrepareStateWorkingDir {
			t.Fatalf("the branch side runs in the live working directory and must say so, got %q", prepared.sourcePrepared)
		}
		// The stamp is worthless unless it reaches the probe: check.go
		// measures the branch through annotateProbe, and that is the only
		// place BranchPrepared can come from.
		probe := prepared.annotateProbe(context.Background(), makeProbe{Target: "check"})
		if probe.Prepared != PrepareStateWorkingDir {
			t.Fatalf("annotateProbe must carry the tree onto the probe, got %q", probe.Prepared)
		}
	})

	t.Run("the asymmetry is reported as the stated reason", func(t *testing.T) {
		// The target tip's check dies the way a bootstrap-hungry ecosystem
		// dies in a pristine worktree: no deps, so no analysis, so no
		// file:line and no repository file named. The branch's check emits a
		// location, and it does so from the live working directory.
		const targetMakefile = "check:\n\t@echo 'Unchecked dependencies for environment dev:'; exit 1\n"
		const branchMakefile = "check:\n\t@echo 'task.go:1: broken'; exit 1\n"

		fx := testutil.TempWorktreeWithBareOrigin(t)
		writeRepoFile(t, fx.Clone, ".gz-git.yaml", "branch:\n  integrationBranch: develop\n")
		writeRepoFile(t, fx.Clone, "Makefile", targetMakefile)
		runGit(t, fx.Clone, "add", ".")
		runGit(t, fx.Clone, "commit", "-m", "target check dies before analysis")
		runGit(t, fx.Clone, "branch", "develop")
		runGit(t, fx.Clone, "push", "-u", fx.Remote, "develop")

		runGit(t, fx.Worktree, "checkout", "-B", "dev/actor/feat/task", "develop")
		writeRepoFile(t, fx.Worktree, "Makefile", branchMakefile)
		runGit(t, fx.Worktree, "add", "Makefile")
		runGit(t, fx.Worktree, "commit", "-m", "task check failure")
		runGit(t, fx.Worktree, "push", "-u", fx.Remote, "HEAD")

		report, err := Check(context.Background(), gitcmd.NewExecutor(), CheckOptions{
			RepoPath: fx.Worktree, Branch: "dev/actor/feat/task",
		})
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		// Naming the asymmetry is the whole point, so all three parts are
		// required: that the runs differed, and which tree each one was.
		for _, want := range []string{
			"not prepared alike",
			string(PrepareStateWorkingDir),
			string(PrepareStatePristine),
		} {
			if !hasCheckDetail(report, "make check", checkFail, want) {
				t.Fatalf("the unmeasurable verdict must name the asymmetry (%q missing):\n%s", want, FormatCheck(report))
			}
		}
	})

	t.Run("symmetric preparation states no asymmetry", func(t *testing.T) {
		// With a controller profile both sides get a fresh worktree and the
		// same profile, so there is no asymmetry to report and the clause
		// must stay silent. Without this the clause would be unconditional
		// text, which would make the subtest above pass on nothing.
		symmetric := unmeasurableReason(3, PrepareStateProfilePrepared, PrepareStateProfilePrepared)
		if strings.Contains(symmetric, "not prepared alike") {
			t.Fatalf("two profile-prepared runs are symmetric, got %q", symmetric)
		}
		// A caller that said nothing cannot have the claim made on its
		// behalf in either direction.
		silent := unmeasurableReason(3, PrepareStateUnknown, PrepareStateUnknown)
		if strings.Contains(silent, "not prepared alike") {
			t.Fatalf("an unstamped verdict must claim nothing about symmetry, got %q", silent)
		}
		asymmetric := unmeasurableReason(3, PrepareStateWorkingDir, PrepareStatePristine)
		if !strings.Contains(asymmetric, "not prepared alike") {
			t.Fatalf("a working-dir branch against a pristine baseline is the asymmetry, got %q", asymmetric)
		}
	})
}
