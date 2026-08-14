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

const (
	checkPass = "PASS"
	checkFail = "FAIL"
	checkWarn = "WARN"
	checkSkip = "SKIP"
)

// CheckOptions configures a read-only readiness check.
type CheckOptions struct {
	RepoPath           string
	Branch             string
	Target             string
	DirectToDefault    bool
	Release            bool
	AllowSkippedChecks bool
	IntegrationConfig  []string
}

// CheckItem is one readiness row.
type CheckItem struct {
	Name   string
	Status string
	Detail string
}

// CheckReport is the readiness answer. It never pushes or reclaims.
type CheckReport struct {
	Ready     bool
	Plan      TargetPlan
	Items     []CheckItem
	Failures  int
	Warnings  int
	RootFacts []string
}

// Check answers whether the branch can land on the target.
func Check(ctx context.Context, exec *gitcmd.Executor, opts CheckOptions) (*CheckReport, error) {
	if exec == nil {
		return nil, fmt.Errorf("git executor is nil")
	}
	dir := strings.TrimSpace(opts.RepoPath)
	if dir == "" {
		dir = "."
	}
	g := newGitRepo(exec, dir)
	if !g.isRepo(ctx) {
		return nil, fmt.Errorf("not a git repository: %s", dir)
	}
	root, err := g.toplevel(ctx)
	if err != nil {
		return nil, err
	}
	g.dir = root

	if len(opts.IntegrationConfig) == 0 {
		decl, err := config.LoadRepoRootTaskPattern(root)
		if err != nil {
			return nil, err
		}
		opts.IntegrationConfig = decl.IntegrationBranch
	}

	plan, err := resolveTarget(ctx, g, exec, opts)
	if err != nil {
		return nil, err
	}

	report := &CheckReport{Plan: plan}
	add := func(item CheckItem) {
		report.Items = append(report.Items, item)
		switch item.Status {
		case checkFail:
			report.Failures++
		case checkWarn:
			report.Warnings++
		}
	}

	add(checkFreshness(ctx, g, plan))
	_, mergeItem := checkMergeTree(ctx, g, plan)
	add(mergeItem)
	for _, item := range checkOtherBranches(ctx, g, plan) {
		add(item)
	}
	add(checkWorkingTree(ctx, g, plan))
	add(checkPushed(ctx, g, plan))

	if plan.HeadSHA != plan.BranchSHA {
		add(CheckItem{Name: "make", Status: checkFail, Detail: "HEAD is not the branch; cannot run tests"})
	} else {
		declared := 0
		for _, target := range []string{"check", "lint"} {
			probe := runMakeTarget(ctx, g.dir, target)
			item := judgeMake(ctx, g, plan, probe, opts.AllowSkippedChecks)
			if item.Status != checkSkip {
				declared++
				add(item)
			}
		}
		if declared == 0 {
			add(CheckItem{Name: "make check/lint", Status: checkWarn, Detail: "undefined — this repo declares no integration gate"})
		}
	}
	report.Ready = report.Failures == 0
	return report, nil
}

func checkFreshness(ctx context.Context, g gitRepo, plan TargetPlan) CheckItem {
	mb, err := g.mergeBase(ctx, plan.BranchSHA, plan.TargetSHA)
	if err != nil {
		return CheckItem{Name: "freshness", Status: checkFail, Detail: err.Error()}
	}
	if mb == plan.TargetSHA {
		return CheckItem{Name: "freshness", Status: checkPass, Detail: "based on " + plan.Target + " tip"}
	}
	n, err := g.revCount(ctx, mb+".."+plan.TargetSHA)
	if err != nil {
		return CheckItem{Name: "freshness", Status: checkFail, Detail: err.Error()}
	}
	return CheckItem{Name: "freshness", Status: checkFail, Detail: fmt.Sprintf("%d commits behind %s — git rebase %s", n, plan.Target, plan.Target)}
}

func checkMergeTree(ctx context.Context, g gitRepo, plan TargetPlan) (clean bool, item CheckItem) {
	ok, err := g.mergeTreeClean(ctx, plan.TargetSHA, plan.BranchSHA)
	if err != nil {
		return false, CheckItem{Name: "merge-tree", Status: checkFail, Detail: err.Error()}
	}
	if ok {
		return true, CheckItem{Name: "merge-tree", Status: checkPass, Detail: "no conflict with " + plan.Target}
	}
	return false, CheckItem{Name: "merge-tree", Status: checkFail, Detail: "conflicts with " + plan.Target}
}

func checkOtherBranches(ctx context.Context, g gitRepo, plan TargetPlan) []CheckItem {
	if plan.Remote == "" {
		return []CheckItem{{Name: "cross-merge", Status: checkPass, Detail: "no remote branches to compare"}}
	}
	refs, err := g.shortRefs(ctx, "refs/remotes/"+plan.Remote)
	if err != nil {
		return []CheckItem{{Name: "cross-merge", Status: checkWarn, Detail: err.Error()}}
	}
	var items []CheckItem
	others := 0
	for _, other := range refs {
		if other == plan.Target || other == plan.DefaultRef || other == plan.Remote || other == plan.Remote+"/HEAD" {
			continue
		}
		sha, ok, err := g.revParse(ctx, other)
		if err != nil || !ok || sha == plan.BranchSHA {
			continue
		}
		if anc, _ := g.isAncestor(ctx, sha, plan.BranchSHA); anc {
			continue
		}
		if anc, _ := g.isAncestor(ctx, plan.BranchSHA, sha); anc {
			continue
		}
		others++
		clean, err := g.mergeTreeClean(ctx, plan.BranchSHA, sha)
		if err != nil {
			items = append(items, CheckItem{Name: "cross-merge " + other, Status: checkWarn, Detail: err.Error()})
			continue
		}
		if clean {
			items = append(items, CheckItem{Name: "cross-merge " + other, Status: checkPass, Detail: "no conflict"})
			continue
		}
		items = append(items, CheckItem{Name: "cross-merge " + other, Status: checkWarn, Detail: "cross conflict"})
	}
	if others == 0 {
		return []CheckItem{{Name: "cross-merge", Status: checkPass, Detail: "no other open branches"}}
	}
	return items
}

func checkWorkingTree(ctx context.Context, g gitRepo, plan TargetPlan) CheckItem {
	if plan.HeadSHA != plan.BranchSHA {
		return CheckItem{Name: "working-tree", Status: checkFail, Detail: "HEAD is not the branch"}
	}
	out, err := g.porcelain(ctx)
	if err != nil {
		return CheckItem{Name: "working-tree", Status: checkFail, Detail: err.Error()}
	}
	if strings.TrimSpace(out) != "" {
		return CheckItem{Name: "working-tree", Status: checkFail, Detail: "uncommitted changes"}
	}
	return CheckItem{Name: "working-tree", Status: checkPass, Detail: "clean"}
}

func checkPushed(ctx context.Context, g gitRepo, plan TargetPlan) CheckItem {
	up, ok, err := g.upstreamSHA(ctx, plan.Branch)
	if err != nil {
		return CheckItem{Name: "push", Status: checkFail, Detail: err.Error()}
	}
	if !ok {
		remote := plan.Remote
		if remote == "" {
			remote = "origin"
		}
		return CheckItem{Name: "push", Status: checkFail, Detail: fmt.Sprintf("no upstream — git push -u %s %s", remote, plan.Branch)}
	}
	if up != plan.BranchSHA {
		return CheckItem{Name: "push", Status: checkFail, Detail: "upstream differs — git push"}
	}
	return CheckItem{Name: "push", Status: checkPass, Detail: "pushed"}
}

// FormatCheck renders the readiness report.
func FormatCheck(r *CheckReport) string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "integrate check: %s  (target=%s", r.Plan.Branch, r.Plan.Target)
	if r.Plan.DefaultRef != "" {
		fmt.Fprintf(&b, ", default=%s", r.Plan.DefaultRef)
	}
	if r.Plan.Integration.Participates {
		fmt.Fprintf(&b, ", integration=%s", r.Plan.Integration.Name)
	}
	b.WriteString(")\n\n")
	for _, item := range r.Items {
		fmt.Fprintf(&b, "  %s  %s — %s\n", item.Status, item.Name, item.Detail)
	}
	b.WriteByte('\n')
	if r.Ready {
		fmt.Fprintf(&b, "READY to integrate into %s — %d warning(s)\n", r.Plan.Target, r.Warnings)
	} else {
		fmt.Fprintf(&b, "NOT READY — %d failure(s), %d warning(s)\n", r.Failures, r.Warnings)
	}
	return b.String()
}
