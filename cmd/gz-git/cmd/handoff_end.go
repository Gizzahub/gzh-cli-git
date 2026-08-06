// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/handoff"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// defaultCheckpointMessage names the commit for what it is. Checkpoint commits
// live on work branches and are squashed when the branch merges, so the message
// only has to be honest, not descriptive.
const defaultCheckpointMessage = "chore(wip): handoff checkpoint"

var (
	handoffEndFlags      BulkCommandFlags
	handoffEndMessage    string
	handoffEndForce      bool
	handoffEndNoPush     bool
	handoffEndNoTrailers bool
)

var handoffEndCmd = &cobra.Command{
	Use:   "end [directory]",
	Short: "Commit and push everything that exists only on this machine",
	Long: cliutil.QuickStartHelp(`  # Before leaving this machine
  gz-git handoff end

  # See what would be committed first
  gz-git handoff end --dry-run

  # Use your own checkpoint message
  gz-git handoff end -m "wip: half of the parser"

  # Commit now, push when there is a network again
  gz-git handoff end --no-push

Every repository with uncommitted or unpushed work is committed and pushed, so
the next machine or agent sees all of it. Repositories that need a human
decision — merge conflicts, an interrupted rebase, a detached HEAD, no remote —
are reported and left untouched.

Before committing, each repository is screened for credentials, oversized
files, and build output that .gitignore does not cover. Anything flagged is
held back rather than swept into history; --force commits it anyway.

The checkpoint commit carries Device: and Agent: trailers naming where it came
from, since the author line is the same on every machine you own. Set them in
the global config under identity:, or with GZ_GIT_DEVICE and GZ_GIT_AGENT; the
device defaults to the hostname.

Exit Codes:
  0  every repository is now safe to leave
  1  work still exists only on this machine
  2  the operation itself could not run`),
	Args: cobra.MaximumNArgs(1),
	RunE: runHandoffEnd,
}

func init() {
	handoffCmd.AddCommand(handoffEndCmd)

	addBulkFlagsWithOpts(handoffEndCmd, &handoffEndFlags, BulkFlagOptions{
		SkipFetch: true,
		SkipWatch: true,
	})
	handoffEndCmd.Flags().StringVarP(&handoffEndMessage, "message", "m", defaultCheckpointMessage,
		"commit message for the checkpoint")
	handoffEndCmd.Flags().BoolVar(&handoffEndForce, "force", false,
		"commit files the guard flagged as secrets, oversized, or build output")
	handoffEndCmd.Flags().BoolVar(&handoffEndNoPush, "no-push", false,
		"commit without pushing (work still exists only on this machine)")
	handoffEndCmd.Flags().BoolVar(&handoffEndNoTrailers, "no-trailers", false,
		"omit the Device: and Agent: trailers from the checkpoint commit")
}

// heldRepo is a repository the guard stopped short of committing.
type heldRepo struct {
	Repository handoff.RepoAssessment `json:"repository"`
	Findings   []handoff.Finding      `json:"findings"`
}

// blockedRepo is a repository the push policy will not accept work for.
type blockedRepo struct {
	Repository handoff.RepoAssessment `json:"repository"`
	Reason     string                 `json:"reason"`
}

// handoffEndReport is the machine-readable account of one run.
type handoffEndReport struct {
	Message   string              `json:"message"`
	Plan      handoff.Plan        `json:"plan"`
	Blocked   []blockedRepo       `json:"blocked,omitempty"`
	Held      []heldRepo          `json:"held,omitempty"`
	Committed []string            `json:"committed,omitempty"`
	Pushed    []string            `json:"pushed,omitempty"`
	Failed    []string            `json:"failed,omitempty"`
	Final     *handoff.Assessment `json:"final,omitempty"`
	DryRun    bool                `json:"dry_run"`
}

func runHandoffEnd(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	directory, err := validateBulkDirectory(args)
	if err != nil {
		return err
	}
	if err := validateBulkDepth(cmd, handoffEndFlags.Depth); err != nil {
		return err
	}
	if err := validateBulkFormat(handoffEndFlags.Format); err != nil {
		return err
	}

	assessment, err := assessHandoff(ctx, directory, handoffEndFlags)
	if err != nil {
		return cliutil.NewExitError(2, err)
	}

	effective, _ := LoadEffectiveConfig(cmd, nil)
	policy, err := resolvePushPolicy(effective, "")
	if err != nil {
		return err
	}

	plan := handoff.PlanCheckpoint(assessment)
	report := &handoffEndReport{
		Message: checkpointMessage(effective),
		Plan:    plan,
		DryRun:  handoffEndFlags.DryRun,
	}

	// Apply the push policy before the commit, not just at the push. A branch
	// this workspace may not push to is one an unattended checkpoint has no
	// business writing to either.
	plan.Checkpoint, report.Blocked = applyPushPolicy(plan.Checkpoint, policy)
	report.Plan = plan

	// Screen before committing. This is the whole reason an explicit checkpoint
	// command is safe where a background auto-commit loop is not.
	if !handoffEndForce {
		plan.Checkpoint, report.Held = screenCheckpoint(ctx, plan.Checkpoint, handoffEndFlags.Parallel)
		report.Plan = plan
	}

	if handoffEndFlags.DryRun {
		return finishHandoffEnd(report, assessment)
	}

	if len(plan.Checkpoint) > 0 {
		if err := checkpointRepositories(ctx, directory, plan.Checkpoint, policy, report); err != nil {
			return cliutil.NewExitError(2, err)
		}
	}

	// Re-read the workspace rather than inferring the outcome: the verdict has
	// to describe what is actually on disk now, not what was intended.
	final, err := assessHandoff(ctx, directory, handoffEndFlags)
	if err != nil {
		return cliutil.NewExitError(2, err)
	}
	report.Final = final

	return finishHandoffEnd(report, final)
}

// finishHandoffEnd renders the report and picks the exit code from the verdict
// that describes the workspace now.
func finishHandoffEnd(report *handoffEndReport, verdictSource *handoff.Assessment) error {
	if handoffEndFlags.Format == "json" {
		if err := encodeJSON(report); err != nil {
			return cliutil.NewExitError(2, err)
		}
	} else if !quiet {
		printHandoffEnd(report, verdictSource)
	}

	if verdictSource.Verdict == handoff.VerdictReady {
		return nil
	}
	return cliutil.NewExitError(1, fmt.Errorf("handoff verdict: %s", verdictSource.Verdict))
}

// checkpointMessage builds the commit message, signing it with the machine and
// agent that produced it.
//
// A checkpoint is written with nobody watching, and the author line is the same
// person on every machine they own, so without a trailer the commit cannot say
// where the work is. Checkpoints are squashed when the branch merges, so this
// costs nothing in the history that survives.
func checkpointMessage(effective *config.EffectiveConfig) string {
	if handoffEndNoTrailers || effective == nil {
		return handoffEndMessage
	}
	return effective.Identity.AppendTrailers(handoffEndMessage)
}

// applyPushPolicy splits off the repositories whose branch the policy will not
// accept a push to.
func applyPushPolicy(repos []handoff.RepoAssessment, policy *repository.PushPolicy) (allowed []handoff.RepoAssessment, blocked []blockedRepo) {
	for _, repo := range repos {
		denial := policy.Check(repository.PushIntent{Branch: repo.Branch})
		if denial == nil {
			allowed = append(allowed, repo)
			continue
		}
		blocked = append(blocked, blockedRepo{Repository: repo, Reason: denial.Detail})
	}
	return allowed, blocked
}

// screenCheckpoint runs the guard over every repository due to be committed and
// splits off the ones with findings.
func screenCheckpoint(ctx context.Context, repos []handoff.RepoAssessment, parallel int) (cleared []handoff.RepoAssessment, held []heldRepo) {
	if len(repos) == 0 {
		return repos, nil
	}
	if parallel <= 0 {
		parallel = repository.DefaultLocalParallel
	}

	executor := gitcmd.NewExecutor()
	findings := make([][]handoff.Finding, len(repos))

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, parallel)

	for i, repo := range repos {
		wg.Add(1)
		go func(idx int, path string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// A guard that cannot run must not silently wave the repository
			// through, so treat the failure itself as a finding.
			found, err := handoff.Guard(ctx, executor, path)
			if err != nil {
				findings[idx] = []handoff.Finding{{
					Kind:   handoff.FindingSecret,
					File:   ".",
					Detail: fmt.Sprintf("could not be screened: %v", err),
				}}
				return
			}
			findings[idx] = found
		}(i, repo.Path)
	}
	wg.Wait()

	for i, repo := range repos {
		if len(findings[i]) == 0 {
			cleared = append(cleared, repo)
			continue
		}
		held = append(held, heldRepo{Repository: repo, Findings: findings[i]})
	}

	return cleared, held
}

// checkpointRepositories commits and pushes exactly the planned repositories.
func checkpointRepositories(ctx context.Context, directory string, repos []handoff.RepoAssessment, policy *repository.PushPolicy, report *handoffEndReport) error {
	client := repository.NewClient()
	// The plan was built from a scan of this same directory, so restricting the
	// bulk operations to those exact paths reproduces the selection.
	pattern := repoPathPattern(handoff.Paths(repos))

	commitResult, err := client.BulkCommit(ctx, repository.BulkCommitOptions{
		Directory:         directory,
		Parallel:          handoffEndFlags.Parallel,
		MaxDepth:          handoffEndFlags.Depth,
		IncludeSubmodules: handoffEndFlags.IncludeSubmodules,
		IncludePattern:    pattern,
		Message:           report.Message,
		Yes:               true,
		Verbose:           verbose,
		Logger:            createBulkLogger(verbose),
	})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	for _, r := range commitResult.Repositories {
		switch r.Status {
		case repository.StatusSuccess:
			report.Committed = append(report.Committed, r.RelativePath)
		case repository.StatusClean:
			// Nothing was staged; the push below still has commits to send.
		default:
			report.Failed = append(report.Failed, r.RelativePath)
		}
	}

	if handoffEndNoPush {
		return nil
	}

	pushResult, err := client.BulkPush(ctx, repository.BulkPushOptions{
		Directory:         directory,
		Parallel:          handoffEndFlags.Parallel,
		MaxDepth:          handoffEndFlags.Depth,
		IncludeSubmodules: handoffEndFlags.IncludeSubmodules,
		IncludePattern:    pattern,
		// A checkpoint on a brand new branch is the common case, and without
		// this the push has no target to fail against.
		SetUpstream: true,
		// Redundant with the pre-commit gate, but a policy belongs on every
		// path that writes to a remote, not only the one that remembered.
		Policy:  policy,
		Verbose: verbose,
		Logger:  createBulkLogger(verbose),
	})
	if err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}

	for _, r := range pushResult.Repositories {
		switch r.Status {
		case repository.StatusPushed:
			report.Pushed = append(report.Pushed, r.RelativePath)
		case repository.StatusUpToDate, repository.StatusSkipped:
			// The remote already had everything.
		default:
			// Anything else — auth refused, no upstream, a rejected push —
			// means the work is still only on this machine.
			report.Failed = append(report.Failed, r.RelativePath)
		}
	}

	return nil
}

// repoPathPattern builds an anchored regex matching exactly the given paths, so
// a bulk operation touches those repositories and no others.
func repoPathPattern(paths []string) string {
	quoted := make([]string, 0, len(paths))
	for _, path := range paths {
		quoted = append(quoted, regexp.QuoteMeta(path))
	}
	return "^(?:" + strings.Join(quoted, "|") + ")$"
}

func printHandoffEnd(report *handoffEndReport, verdictSource *handoff.Assessment) {
	fmt.Println()

	if report.DryRun {
		printHandoffEndPlan(report)
	} else {
		printHandoffEndOutcome(report)
	}

	for _, blocked := range report.Blocked {
		fmt.Printf("  %s✗%s %s%s\n", cliutil.ColorRed, cliutil.ColorReset,
			blocked.Repository.RelativePath, handoffBranchSuffix(blocked.Repository))
		fmt.Printf("      %sblocked by push policy — %s%s\n", cliutil.ColorGray, blocked.Reason, cliutil.ColorReset)
	}

	for _, held := range report.Held {
		fmt.Printf("  %s%s%s %s\n", cliutil.ColorYellow, "⚠", cliutil.ColorReset, held.Repository.RelativePath)
		fmt.Printf("      %sheld back — nothing was committed%s\n", cliutil.ColorGray, cliutil.ColorReset)
		for _, finding := range held.Findings {
			fmt.Printf("      %s%s: %s (%s)%s\n",
				cliutil.ColorGray, finding.File, finding.Detail, finding.Kind, cliutil.ColorReset)
		}
	}

	for _, skipped := range report.Plan.Skipped {
		blocker, _ := handoff.FirstHardBlocker(skipped)
		fmt.Printf("  %s✗%s %s\n", cliutil.ColorRed, cliutil.ColorReset, skipped.RelativePath)
		fmt.Printf("      %s%s%s\n", cliutil.ColorGray, blocker.Detail, cliutil.ColorReset)
	}

	if len(report.Held) > 0 {
		fmt.Printf("\n%sReview the held files, then rerun. Use --force to commit them as they are.%s\n",
			cliutil.ColorGray, cliutil.ColorReset)
	}

	// Without this the verdict below reads "fixable", which is true of the work
	// but not of this command: no rerun clears a policy refusal.
	if len(report.Blocked) > 0 {
		fmt.Printf("\n%sMove this work onto a branch you may push to; a checkpoint cannot land on a protected one.%s\n",
			cliutil.ColorGray, cliutil.ColorReset)
	}

	fmt.Println()
	printHandoffVerdict(verdictSource, false)
	fmt.Println()
}

func printHandoffEndPlan(report *handoffEndReport) {
	if report.Plan.Empty() {
		fmt.Printf("%sNothing to commit.%s\n", cliutil.ColorGray, cliutil.ColorReset)
		return
	}

	// A dry run is where someone decides whether to let the checkpoint happen,
	// so show the message it would carry, trailers included.
	for line := range strings.SplitSeq(report.Message, "\n") {
		if line != "" {
			fmt.Printf("  %s%s%s\n", cliutil.ColorGray, line, cliutil.ColorReset)
		}
	}
	fmt.Println()

	for _, repo := range report.Plan.Checkpoint {
		fmt.Printf("  %s→%s %s%s\n", cliutil.ColorCyan, cliutil.ColorReset,
			repo.RelativePath, handoffBranchSuffix(repo))
		for _, blocker := range repo.Blockers {
			fmt.Printf("      %swould %s: %s%s\n",
				cliutil.ColorGray, checkpointVerb(blocker.Reason), blocker.Detail, cliutil.ColorReset)
		}
	}
}

// checkpointVerb names the step that clears a blocker, for the dry-run preview.
func checkpointVerb(reason handoff.Reason) string {
	switch reason {
	case handoff.ReasonUncommitted:
		return "commit"
	case handoff.ReasonUnpushed, handoff.ReasonNoUpstream:
		return "push"
	default:
		return "leave"
	}
}

func printHandoffEndOutcome(report *handoffEndReport) {
	for _, path := range report.Committed {
		fmt.Printf("  %s✓%s %s %scommitted%s\n",
			cliutil.ColorGreen, cliutil.ColorReset, path, cliutil.ColorGray, cliutil.ColorReset)
	}
	for _, path := range report.Pushed {
		fmt.Printf("  %s✓%s %s %spushed%s\n",
			cliutil.ColorGreen, cliutil.ColorReset, path, cliutil.ColorGray, cliutil.ColorReset)
	}
	for _, path := range report.Failed {
		fmt.Printf("  %s✗%s %s %sfailed%s\n",
			cliutil.ColorRed, cliutil.ColorReset, path, cliutil.ColorGray, cliutil.ColorReset)
	}
}
