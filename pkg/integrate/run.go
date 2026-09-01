// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"context"
	"fmt"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
)

// RunOptions configures a fast-forward integrate and reclaim.
type RunOptions struct {
	CheckOptions
}

// RunReport is the result of integrate run.
type RunReport struct {
	Check      *CheckReport
	Source     string
	Target     string
	SHA        string
	Reclaim    ReclaimResult
	Printed    []string
	Integrated bool
}

// Run fast-forwards the target and reclaims the task branch.
// Reclaim only matches declared taskPattern. No pattern → reclaim nothing.
func Run(ctx context.Context, exec *gitcmd.Executor, opts RunOptions) (*RunReport, error) {
	if exec == nil {
		return nil, fmt.Errorf("git executor is nil")
	}
	check, err := Check(ctx, exec, opts.CheckOptions)
	if err != nil {
		return nil, err
	}
	return runChecked(ctx, exec, opts, check)
}

// runChecked continues an integration from an immutable readiness report.
// It deliberately revalidates source, target, and contract provenance before
// any push or reclaim action.
func runChecked(ctx context.Context, exec *gitcmd.Executor, opts RunOptions, check *CheckReport) (*RunReport, error) {
	if exec == nil {
		return nil, fmt.Errorf("git executor is nil")
	}
	if check == nil {
		return nil, fmt.Errorf("readiness report is nil")
	}
	report := &RunReport{Check: check, Source: check.Plan.Branch, Target: check.Plan.Target}
	if !check.Ready {
		return report, fmt.Errorf("not ready")
	}

	dir := strings.TrimSpace(opts.RepoPath)
	if dir == "" {
		dir = "."
	}
	g := newGitRepo(exec, dir)
	root, err := g.toplevel(ctx)
	if err != nil {
		return report, err
	}
	g.dir = root
	if err := revalidateController(ctx, g, check); err != nil {
		return report, err
	}

	sourceSHA, targetSHA, targetName, err := revalidateCheckedRefs(ctx, g, check)
	if err != nil {
		return report, err
	}

	anc, err := g.isAncestor(ctx, targetSHA, sourceSHA)
	if err != nil {
		return report, err
	}
	if !anc {
		return report, fmt.Errorf("%s is not an ancestor of %s; rebase and re-check", check.Plan.Target, check.Plan.Branch)
	}
	if err := revalidateController(ctx, g, check); err != nil {
		return report, err
	}

	pushRemote := check.Plan.Remote
	if check.Controller != nil {
		pushRemote = check.Controller.RemoteURL
	}
	if err := pushFastForward(ctx, g, pushRemote, sourceSHA, targetName, check.Plan.TargetSHA); err != nil {
		return report, err
	}
	if err := ffTargetWorktrees(ctx, exec, g, targetName, sourceSHA); err != nil {
		return report, err
	}

	report.Integrated = true
	report.SHA = sourceSHA
	report.Printed = append(report.Printed, fmt.Sprintf("INTEGRATED %s (%s) -> %s/%s", check.Plan.Branch, sourceSHA, check.Plan.Remote, targetName))
	return finishRunReclaim(ctx, exec, g, root, targetName, report)
}

func revalidateCheckedRefs(ctx context.Context, g gitRepo, check *CheckReport) (sourceSHA, targetSHA, targetName string, err error) {
	sourceSHA, ok, err := g.revParse(ctx, check.Plan.Branch)
	if err != nil {
		return "", "", "", err
	}
	if !ok || sourceSHA != check.Plan.BranchSHA {
		return "", "", "", fmt.Errorf("source branch changed during readiness; re-run check")
	}
	if check.Plan.Remote != "" {
		if err := g.fetchPrune(ctx, check.Plan.Remote); err != nil {
			return "", "", "", err
		}
	}
	targetName = targetBranchName(check.Plan.Target, check.Plan.Remote)
	targetRef := check.Plan.Target
	if check.Plan.Remote != "" {
		targetRef = check.Plan.Remote + "/" + targetName
	}
	targetSHA, ok, err = g.revParse(ctx, targetRef)
	if err != nil {
		return "", "", "", err
	}
	if !ok {
		targetSHA, ok, err = g.revParse(ctx, check.Plan.Target)
		if err != nil {
			return "", "", "", err
		}
		if !ok {
			return "", "", "", fmt.Errorf("target ref not found after fetch: %s", check.Plan.Target)
		}
	}
	if targetSHA != check.Plan.TargetSHA {
		return "", "", "", fmt.Errorf("target branch changed during readiness; re-run check")
	}
	if check.GateMode != "contract-v1" {
		return sourceSHA, targetSHA, targetName, nil
	}
	targetContract, present, err := loadReadinessContract(ctx, g, targetSHA)
	if err != nil {
		return "", "", "", err
	}
	if !present || targetContract.Digest != check.ContractDigest || targetContract.ManifestOID != check.ManifestOID || targetContract.TreeOID != check.ReadinessTreeOID || targetContract.RunnerOID != check.RunnerOID {
		return "", "", "", fmt.Errorf("target readiness contract changed during readiness; re-run check")
	}
	return sourceSHA, targetSHA, targetName, nil
}

func pushFastForward(ctx context.Context, g gitRepo, remote, sourceSHA, targetName, checkedTargetSHA string) error {
	if remote == "" {
		return fmt.Errorf("no remote; cannot push the fast-forward")
	}
	refspec := sourceSHA + ":refs/heads/" + targetName
	lease := "--force-with-lease=refs/heads/" + targetName + ":" + checkedTargetSHA
	push, err := g.run(ctx, "push", lease, remote, refspec)
	if err != nil {
		return err
	}
	if push.ExitCode != 0 {
		return fmt.Errorf("git push %s %s failed: %s", remote, refspec, strings.TrimSpace(push.Stderr))
	}
	return nil
}

func ffTargetWorktrees(ctx context.Context, exec *gitcmd.Executor, g gitRepo, targetName, sourceSHA string) error {
	trees, err := g.listWorktrees(ctx)
	if err != nil {
		return fmt.Errorf("integrated but listing worktrees failed: %w", err)
	}
	for _, wt := range trees {
		if wt.Branch != targetName {
			continue
		}
		tg := newGitRepo(exec, wt.Path)
		res, err := tg.run(ctx, "merge", "--ff-only", sourceSHA)
		if err != nil || (res != nil && res.ExitCode != 0) {
			detail := ""
			if res != nil {
				detail = strings.TrimSpace(res.Stderr)
			}
			if err != nil && detail == "" {
				detail = err.Error()
			}
			return fmt.Errorf("remote integrated but local target worktree failed: %s: %s", wt.Path, detail)
		}
		break
	}
	return nil
}

func finishRunReclaim(ctx context.Context, exec *gitcmd.Executor, g gitRepo, root, targetName string, report *RunReport) (*RunReport, error) {
	if err := revalidateController(ctx, g, report.Check); err != nil {
		return report, err
	}
	decl, loadErr := config.LoadRepoRootTaskPattern(root)
	if report.Check.Controller != nil {
		decl.Patterns = append([]string(nil), report.Check.Controller.TaskPattern...)
		decl.Facts = nil
		loadErr = nil
	}
	if loadErr != nil {
		report.Reclaim.Skipped = "reclaim nothing: " + loadErr.Error()
		report.Printed = append(report.Printed, "RECLAIM skipped: "+report.Reclaim.Skipped)
	} else {
		report.Reclaim = reclaimAfter(ctx, exec, g, reclaimOpts{
			Branch:       report.Check.Plan.Branch,
			TargetBranch: targetName,
			DefaultName:  defaultBranchName(report.Check.Plan.DefaultRef, report.Check.Plan.Remote),
			Integration:  report.Check.Plan.Integration.Name,
			Remote:       report.Check.Plan.Remote,
			PushRemote:   controllerPushRemote(report.Check),
			TaskSHA:      report.SHA,
			Patterns:     decl.Patterns,
			Facts:        decl.Facts,
		})
		if report.Reclaim.Skipped != "" {
			report.Printed = append(report.Printed, "RECLAIM skipped: "+report.Reclaim.Skipped)
		}
		if len(report.Reclaim.Done) > 0 {
			report.Printed = append(report.Printed, "RECLAIMED "+report.Check.Plan.Branch+" — "+strings.Join(report.Reclaim.Done, " "))
		}
		if report.Reclaim.Incomplete() {
			report.Printed = append(report.Printed, "RECLAIM incomplete: "+strings.Join(report.Reclaim.Failed, "; "))
		}
	}
	return report, nil
}

func controllerPushRemote(check *CheckReport) string {
	if check != nil && check.Controller != nil {
		return check.Controller.RemoteURL
	}
	return ""
}

// FormatRun renders integrate/reclaim lines.
func FormatRun(r *RunReport) string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	if r.Check != nil {
		b.WriteString(FormatCheck(r.Check))
		b.WriteByte('\n')
	}
	for _, line := range r.Printed {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
