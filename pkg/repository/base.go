// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"
	"strings"
)

// BaseBranchInfo describes the integration branch a repository's work is meant
// to land on, and how the current HEAD relates to it.
//
// "Base" is a policy decision, not a git fact: a project declares an ordered
// set of candidate integration branches (config defaultBranch, e.g.
// "develop,master") and the first one present locally wins. Source records
// where the answer came from so a caller can explain it — "master, because
// config.defaultBranch[1]" reads differently from "master, by heuristic", and
// a workflow that blocks on the wrong base is worse than one that admits it
// could not find one.
type BaseBranchInfo struct {
	// Name is the resolved base branch, or empty when no candidate exists.
	Name string

	// Source explains the resolution: "config.defaultBranch[i]" for the i-th
	// configured candidate that won, "heuristic" for the fallback list, or
	// "none" when nothing matched. Never empty.
	Source string

	// Ahead is the number of commits reachable from HEAD but not from the base
	// (work the base does not have yet). Zero when there is no base.
	Ahead int

	// Behind is the number of commits reachable from the base but not from HEAD
	// (work the base has that HEAD lacks — the rebase/pull signal). Zero when
	// there is no base.
	Behind int

	// SHA is the short hash of the base tip, or empty when there is no base.
	SHA string
}

// heuristicBaseCandidates is the fallback integration-branch search order used
// when config declares nothing or none of its candidates exist locally. It
// mirrors the order the rest of the codebase already probes (see detectBaseBranch).
var heuristicBaseCandidates = []string{"main", "master", "develop", "development"}

// ResolveBase picks the integration branch for a repository.
//
// candidates — typically EffectiveConfig.Branch.DefaultBranch — are tried in
// order; the first that exists as a local ref wins, and Source names its index.
// When candidates is empty or none exist, the heuristic list is tried and
// Source is "heuristic". When nothing matches, Name is empty and Source is
// "none"; divergence is zero and the caller should treat the repo as having no
// known base rather than guessing.
//
// The function never returns an error for a missing base — that is a normal,
// reportable state. It returns an error only when git itself fails.
func (c *client) ResolveBase(ctx context.Context, repo *Repository, candidates []string) (BaseBranchInfo, error) {
	info := BaseBranchInfo{Source: baseSourceNone}

	if repo == nil {
		return info, fmt.Errorf("repository cannot be nil")
	}

	name, idx := c.firstExistingBranch(ctx, repo.Path, candidates)
	if name == "" {
		// Config either declared nothing or none of its candidates are present
		// here. Fall back to the conventional list rather than declaring no base,
		// since a repo cloned without its config still has a trunk.
		name, _ = c.firstExistingBranch(ctx, repo.Path, heuristicBaseCandidates)
		info.Name = name
		info.Source = baseSourceHeuristic
	} else {
		info.Name = name
		info.Source = fmt.Sprintf("%s%d]", baseSourceConfigPrefix, idx)
	}

	if name == "" {
		// No base branch exists at all (fresh repo, or only feature branches).
		// Report it plainly rather than guessing a name that is not there.
		info.Source = baseSourceNone
		return info, nil
	}

	// Divergence: rev-list --left-right --count HEAD...base yields
	// "<ahead>\t<behind>" where ahead = commits in HEAD not in base, behind =
	// commits in base not in HEAD. Same shape as the upstream computation in
	// GetInfo, just against the base ref instead of @{upstream}.
	output, err := c.executor.RunOutput(ctx, repo.Path, "rev-list", "--left-right", "--count", "HEAD..."+name)
	if err != nil {
		c.logger.Debug("ResolveBase: divergence probe failed for %s against %s: %v", repo.Path, name, err)
		// Name is still valid even if divergence is unavailable.
		return info, nil
	}

	ahead, behind, err := parseAheadBehind(output)
	if err != nil {
		c.logger.Debug("ResolveBase: unparseable divergence %q for %s: %v", output, repo.Path, err)
		return info, nil
	}
	info.Ahead = ahead
	info.Behind = behind

	if sha, err := c.executor.RunOutput(ctx, repo.Path, "rev-parse", "--short", name); err == nil {
		info.SHA = strings.TrimSpace(sha)
	}

	return info, nil
}

// MergedBranches returns the local branches whose tips are ancestors of base,
// excluding base itself and the currently checked-out branch.
//
// Ancestry is the only evidence that settles this. A branch can look finished
// by every other signal — no unique commits in the diff, a name matching a
// closed ticket, zero ahead in some other comparison — and still hold a commit
// the base never took. `merge-base --is-ancestor` answers the actual question:
// is every commit on this branch already reachable from base. Nothing else is
// safe to delete on.
//
// The current branch is excluded because deleting it is not a thing git will
// do, so reporting it would produce a remediation that cannot run.
func (c *client) MergedBranches(ctx context.Context, repo *Repository, base string) ([]string, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository cannot be nil")
	}
	base = strings.TrimSpace(base)
	if base == "" {
		return nil, nil
	}

	current, err := c.executor.RunOutput(ctx, repo.Path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to read current branch: %w", err)
	}
	currentBranch := strings.TrimSpace(current)

	output, err := c.executor.RunOutput(ctx, repo.Path, "for-each-ref", "--format=%(refname:short)", "refs/heads/")
	if err != nil {
		return nil, fmt.Errorf("failed to list local branches: %w", err)
	}

	var merged []string
	for _, line := range strings.Split(output, "\n") {
		name := strings.TrimSpace(line)
		if name == "" || name == base || name == currentBranch {
			continue
		}
		result, _ := c.executor.Run(ctx, repo.Path, "merge-base", "--is-ancestor", name, base) //nolint:errcheck // exit 1 means "not an ancestor", which is an answer
		if result.ExitCode == 0 {
			merged = append(merged, name)
		}
	}
	return merged, nil
}

// firstExistingBranch returns the first candidate that resolves as a local ref
// and its index within candidates. An empty name means nothing matched; the
// caller decides whether to fall back and how to label the source.
func (c *client) firstExistingBranch(ctx context.Context, repoPath string, candidates []string) (name string, index int) {
	for i, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		// rev-parse --verify exits non-zero when the ref does not exist; we
		// treat that as "candidate absent" rather than an error. Restricting the
		// lookup to refs/heads keeps a tag or remote-tracking ref of the same
		// name from being mistaken for a local branch.
		result, _ := c.executor.Run(ctx, repoPath, "rev-parse", "--verify", "--quiet", "refs/heads/"+candidate) //nolint:errcheck // exit≠0 means ref missing
		if result.ExitCode == 0 {
			return candidate, i
		}
	}
	return "", -1
}

const (
	baseSourceConfigPrefix = "config.defaultBranch["
	baseSourceHeuristic    = "heuristic"
	baseSourceNone         = "none"
)
