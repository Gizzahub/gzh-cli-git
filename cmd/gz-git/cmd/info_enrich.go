// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"

	"golang.org/x/sync/errgroup"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/branch"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// infoEnrichment carries the per-repository facts that BulkStatus does not
// collect. BulkStatus answers "how does this repo relate to its remote"; the
// two questions this adds are "how does it relate to the branch its work is
// supposed to land on" and "is any of that work checked out somewhere else".
//
// Every field has a meaningful zero value, so a repository whose enrichment
// failed renders as a row with those columns blank rather than being dropped
// from the table. A repository that could not be enriched is still a
// repository the user asked about.
type infoEnrichment struct {
	// Base is the resolved integration branch and HEAD's divergence from it.
	// Base.Source is "none" when the repository has no recognizable base.
	Base repository.BaseBranchInfo

	// LinkedWorktrees counts worktrees other than the main checkout. The main
	// worktree is excluded because every repository has one; only the extra
	// ones represent work parked elsewhere.
	LinkedWorktrees int

	// WorktreeBranches lists the branches checked out in those linked
	// worktrees, in the order git reported them. Detached worktrees contribute
	// no entry, so this can be shorter than LinkedWorktrees.
	WorktreeBranches []string

	// Err records why enrichment was incomplete, or nil. It is reported in the
	// detail view rather than aborting the scan: failing to resolve a base
	// branch must not hide the repository's status.
	Err error
}

// enrichInfoResults gathers base-branch and worktree facts for every scanned
// repository, keyed by absolute repository path.
//
// The work is independent per repository and dominated by git process spawns,
// so it runs at the same parallelism as the scan itself. Errors are collected
// into each entry instead of being returned: one unreadable repository should
// degrade one row, not the whole report.
func enrichInfoResults(
	ctx context.Context,
	client repository.Client,
	wtMgr branch.WorktreeManager,
	repos []repository.RepositoryStatusResult,
	baseCandidates []string,
	parallel int,
) map[string]infoEnrichment {
	enriched := make([]infoEnrichment, len(repos))

	if parallel < 1 {
		parallel = 1
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(parallel)

	for i := range repos {
		i := i
		g.Go(func() error {
			enriched[i] = enrichOne(gctx, client, wtMgr, repos[i], baseCandidates)
			return nil // never abort siblings; the error lives in the entry
		})
	}
	// enrichOne never returns an error, so Wait can only surface ctx
	// cancellation, which the per-entry Err already reflects.
	_ = g.Wait()

	byPath := make(map[string]infoEnrichment, len(repos))
	for i, repo := range repos {
		byPath[repo.Path] = enriched[i]
	}
	return byPath
}

// enrichOne collects the extra facts for a single repository. It opens the
// repository once and reuses that handle for both probes.
func enrichOne(
	ctx context.Context,
	client repository.Client,
	wtMgr branch.WorktreeManager,
	status repository.RepositoryStatusResult,
	baseCandidates []string,
) infoEnrichment {
	var out infoEnrichment
	out.Base.Source = "none"

	repo, err := client.Open(ctx, status.Path)
	if err != nil {
		out.Err = err
		return out
	}

	if base, err := client.ResolveBase(ctx, repo, baseCandidates); err != nil {
		out.Err = err
	} else {
		out.Base = base
	}

	worktrees, err := wtMgr.List(ctx, repo)
	if err != nil {
		// Keep whatever base resolution produced; record the first failure only
		// so the detail view names a cause rather than a chain.
		if out.Err == nil {
			out.Err = err
		}
		return out
	}

	for _, wt := range worktrees {
		if wt == nil || wt.IsMain {
			continue
		}
		out.LinkedWorktrees++
		if wt.Branch != "" {
			out.WorktreeBranches = append(out.WorktreeBranches, wt.Branch)
		}
	}

	return out
}
