// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"path/filepath"

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

	// Worktrees pairs each linked worktree's path with the branch it holds.
	// The compact table only needs the branch names above; the audit needs the
	// paths too, because reclaiming a worktree is addressed by path.
	Worktrees []repository.AuditWorktree

	// PrunableWorktrees are paths git still records but whose directories are
	// gone. They are kept out of LinkedWorktrees because they hold no work:
	// counting a stale record as a live worktree would overstate how much is
	// checked out elsewhere, which is exactly the number the WT column exists
	// to answer.
	PrunableWorktrees []string

	// MergedBranches are local branches already contained in Base, collected
	// only in audit mode — it costs one git process per local branch, which the
	// default one-line view has no use for.
	MergedBranches []string

	// RemoteBotMerged / RemoteBotPending partition origin bot remotes by
	// ancestry against Base. Audit-only, same cost reason as MergedBranches.
	RemoteBotMerged  []string
	RemoteBotPending []string

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
	deep bool,
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
			enriched[i] = enrichOne(gctx, client, wtMgr, repos[i], baseCandidates, deep)
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
// repository once and reuses that handle for every probe.
//
// deep adds the audit-only probes. They are opt-in because their cost scales
// with the repository's branch count rather than being constant, and the
// one-line view does not display what they produce.
func enrichOne(
	ctx context.Context,
	client repository.Client,
	wtMgr branch.WorktreeManager,
	status repository.RepositoryStatusResult,
	baseCandidates []string,
	deep bool,
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

	if deep && out.Base.Name != "" {
		if merged, err := client.MergedBranches(ctx, repo, out.Base.Name); err != nil {
			if out.Err == nil {
				out.Err = err
			}
		} else {
			out.MergedBranches = merged
		}
		if botMerged, botPending, err := client.BotRemoteBranches(ctx, repo, out.Base.Name); err != nil {
			if out.Err == nil {
				out.Err = err
			}
		} else {
			out.RemoteBotMerged = botMerged
			out.RemoteBotPending = botPending
		}
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

	// Worktrees are reported relative to the repository git considers main, so
	// scanning a linked worktree directly makes it list itself. Excluding the
	// scanned path keeps "work parked elsewhere" meaning elsewhere; this
	// repository's own branch and status already describe what is here.
	self := resolvePath(status.Path)

	for _, wt := range worktrees {
		if wt == nil || wt.IsMain || resolvePath(wt.Path) == self {
			continue
		}
		if wt.IsPrunable {
			out.PrunableWorktrees = append(out.PrunableWorktrees, wt.Path)
			continue
		}
		out.LinkedWorktrees++
		out.Worktrees = append(out.Worktrees, repository.AuditWorktree{
			Path:   wt.Path,
			Branch: wt.Branch,
		})
		if wt.Branch != "" {
			out.WorktreeBranches = append(out.WorktreeBranches, wt.Branch)
		}
	}

	return out
}

// resolvePath canonicalizes a path for identity comparison. Symlinks are
// resolved because the scanner and git can name the same directory differently
// (/tmp vs /private/tmp on macOS being the common case); when resolution fails
// — the path is gone, as with a prunable worktree — the cleaned path is still
// the best available answer.
func resolvePath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}
