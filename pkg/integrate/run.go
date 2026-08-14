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

	sourceSHA, ok, err := g.revParse(ctx, check.Plan.Branch)
	if err != nil {
		return report, err
	}
	if !ok || sourceSHA != check.Plan.BranchSHA {
		return report, fmt.Errorf("source branch changed during readiness; re-run check")
	}

	if check.Plan.Remote != "" {
		if err := g.fetchPrune(ctx, check.Plan.Remote); err != nil {
			return report, err
		}
	}

	targetName := targetBranchName(check.Plan.Target, check.Plan.Remote)
	targetRef := check.Plan.Target
	if check.Plan.Remote != "" {
		targetRef = check.Plan.Remote + "/" + targetName
	}
	targetSHA, ok, err := g.revParse(ctx, targetRef)
	if err != nil {
		return report, err
	}
	if !ok {
		targetSHA, ok, err = g.revParse(ctx, check.Plan.Target)
		if err != nil {
			return report, err
		}
		if !ok {
			return report, fmt.Errorf("target ref not found after fetch: %s", check.Plan.Target)
		}
	}

	anc, err := g.isAncestor(ctx, targetSHA, sourceSHA)
	if err != nil {
		return report, err
	}
	if !anc {
		return report, fmt.Errorf("%s is not an ancestor of %s; rebase and re-check", check.Plan.Target, check.Plan.Branch)
	}

	if check.Plan.Remote == "" {
		return report, fmt.Errorf("no remote; cannot push the fast-forward")
	}
	refspec := sourceSHA + ":refs/heads/" + targetName
	push, err := g.run(ctx, "push", check.Plan.Remote, refspec)
	if err != nil {
		return report, err
	}
	if push.ExitCode != 0 {
		return report, fmt.Errorf("git push %s %s failed: %s", check.Plan.Remote, refspec, strings.TrimSpace(push.Stderr))
	}

	trees, err := g.listWorktrees(ctx)
	if err != nil {
		return report, fmt.Errorf("integrated but listing worktrees failed: %w", err)
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
			return report, fmt.Errorf("remote integrated but local target worktree failed: %s: %s", wt.Path, detail)
		}
		break
	}

	report.Integrated = true
	report.SHA = sourceSHA
	report.Printed = append(report.Printed, fmt.Sprintf("INTEGRATED %s (%s) -> %s/%s", check.Plan.Branch, sourceSHA, check.Plan.Remote, targetName))

	decl, err := config.LoadRepoRootTaskPattern(root)
	if err != nil {
		report.Reclaim.Skipped = "reclaim nothing: " + err.Error()
		report.Printed = append(report.Printed, "RECLAIM skipped: "+report.Reclaim.Skipped)
		return report, nil
	}

	report.Reclaim = reclaimAfter(ctx, exec, g, reclaimOpts{
		Branch:       check.Plan.Branch,
		TargetBranch: targetName,
		DefaultName:  defaultBranchName(check.Plan.DefaultRef, check.Plan.Remote),
		Integration:  check.Plan.Integration.Name,
		Remote:       check.Plan.Remote,
		Patterns:     decl.Patterns,
		Facts:        decl.Facts,
	})
	if report.Reclaim.Skipped != "" {
		report.Printed = append(report.Printed, "RECLAIM skipped: "+report.Reclaim.Skipped)
	}
	if len(report.Reclaim.Done) > 0 {
		report.Printed = append(report.Printed, "RECLAIMED "+check.Plan.Branch+" — "+strings.Join(report.Reclaim.Done, " "))
	}
	if report.Reclaim.Incomplete() {
		report.Printed = append(report.Printed, "RECLAIM incomplete: "+strings.Join(report.Reclaim.Failed, "; "))
	}
	return report, nil
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
