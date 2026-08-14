// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"context"
	"fmt"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
)

// TargetPlan is the resolved check/run destination.
type TargetPlan struct {
	Branch      string
	BranchSHA   string
	Remote      string
	Target      string
	TargetSHA   string
	DefaultRef  string
	Integration Resolution
	HeadSHA     string
}

func resolveTarget(ctx context.Context, g gitRepo, exec *gitcmd.Executor, opts CheckOptions) (TargetPlan, error) {
	var plan TargetPlan

	branch := strings.TrimSpace(opts.Branch)
	if branch == "" {
		cur, err := g.currentBranch(ctx)
		if err != nil {
			return plan, err
		}
		branch = cur
	}
	if branch == "" || branch == "HEAD" {
		return plan, fmt.Errorf("detached HEAD — pass a branch")
	}
	if err := gitcmd.SanitizeBranchName(branch); err != nil {
		return plan, fmt.Errorf("invalid branch: %w", err)
	}
	plan.Branch = branch

	sha, ok, err := g.revParse(ctx, branch)
	if err != nil {
		return plan, err
	}
	if !ok {
		return plan, fmt.Errorf("branch not found: %s", branch)
	}
	plan.BranchSHA = sha

	head, err := g.headSHA(ctx)
	if err != nil {
		return plan, err
	}
	plan.HeadSHA = head

	remote, err := detectRemote(ctx, g, branch)
	if err != nil {
		return plan, err
	}
	plan.Remote = remote

	if err := planFetchDefault(ctx, g, remote, &plan); err != nil {
		return plan, err
	}

	integ, err := ResolveIntegrationBranch(ctx, exec, g.dir, opts.IntegrationConfig)
	if err != nil {
		return plan, err
	}
	plan.Integration = integ

	target := strings.TrimSpace(opts.Target)
	if target == "" {
		if !integ.Participates {
			return plan, fmt.Errorf("no integration branch; pass --target")
		}
		if remote != "" {
			target = remote + "/" + integ.Name
			if _, ok, err := g.revParse(ctx, target); err != nil {
				return plan, err
			} else if !ok {
				target = integ.Name
			}
		} else {
			target = integ.Name
		}
	}
	if err := gitcmd.SanitizeBranchName(target); err != nil {
		return plan, fmt.Errorf("invalid --target: %w", err)
	}
	if err := validateTarget(plan, target, opts); err != nil {
		return plan, err
	}
	tsha, ok, err := g.revParse(ctx, target)
	if err != nil {
		return plan, err
	}
	if !ok {
		return plan, fmt.Errorf("target ref not found: %s", target)
	}
	plan.Target = target
	plan.TargetSHA = tsha
	return plan, nil
}

func planFetchDefault(ctx context.Context, g gitRepo, remote string, plan *TargetPlan) error {
	if remote == "" {
		return nil
	}
	if err := g.fetchPrune(ctx, remote); err != nil {
		return err
	}
	def, ok, err := g.symbolicRef(ctx, "refs/remotes/"+remote+"/HEAD")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s/HEAD is missing; cannot detect the default branch", remote)
	}
	plan.DefaultRef = def
	return nil
}

func detectRemote(ctx context.Context, g gitRepo, branch string) (string, error) {
	configured, err := g.branchRemote(ctx, branch)
	if err != nil {
		return "", err
	}
	if configured != "" && configured != "." {
		return configured, nil
	}
	remotes, err := g.remotes(ctx)
	if err != nil {
		return "", err
	}
	if isRemoteName("origin", remotes) {
		return "origin", nil
	}
	if len(remotes) == 1 {
		return remotes[0], nil
	}
	if len(remotes) == 0 {
		return "", fmt.Errorf("no remote configured")
	}
	return "", fmt.Errorf("cannot detect which remote to use")
}

func validateTarget(plan TargetPlan, target string, opts CheckOptions) error {
	if opts.DirectToDefault && opts.Release {
		return fmt.Errorf("--direct-to-default and --release cannot be combined")
	}
	if !plan.Integration.Participates {
		if target != plan.DefaultRef || !opts.DirectToDefault {
			if plan.DefaultRef == "" {
				return fmt.Errorf("no integration branch; pass --target <default> --direct-to-default")
			}
			return fmt.Errorf("no integration branch; pass --target %s --direct-to-default", plan.DefaultRef)
		}
		return nil
	}
	integRef := plan.Integration.Name
	if plan.Remote != "" {
		integRef = plan.Remote + "/" + plan.Integration.Name
	}
	switch target {
	case integRef, plan.Integration.Name:
		if opts.DirectToDefault || opts.Release {
			return fmt.Errorf("do not use --direct-to-default/--release against the integration branch")
		}
	case plan.DefaultRef:
		if !opts.Release {
			return fmt.Errorf("promoting onto the default branch requires --release")
		}
		if plan.Branch != plan.Integration.Name && plan.Branch != integRef {
			return fmt.Errorf("release promotion must start from %s", integRef)
		}
	default:
		return fmt.Errorf("target must be the integration branch (%s) or the default branch (%s)", integRef, plan.DefaultRef)
	}
	return nil
}
