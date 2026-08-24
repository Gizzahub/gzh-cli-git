// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"
	"strings"
)

// The opt-in half of base syncing: materializing a base branch a repository
// never had, rather than repairing one that fell behind. It lives apart from
// base_sync.go because the two answer different questions and the safety
// argument for each is different — the repair path only ever has to justify
// moving a pointer, this one has to justify inventing a branch.

// resolveSyncBase picks the branch this sync will act on.
//
// ResolveBase looks only at refs/heads, which is right for reporting: a branch
// that does not exist locally cannot be what local work diverged from. The
// consequence is that a clone which only ever checked out develop does not
// resolve to "no base" — it falls through ResolveBase's heuristic list and
// lands on develop, the branch that *is* checked out, so the sync hands off to
// the pull path and never repairs anything. Those are exactly the repositories
// a base sync would help most, and they are what --create-missing-base is for.
//
// The create path therefore only runs when the resolved base cannot be
// repaired at all — see baseNeedsCreation. Running it whenever *some* candidate
// happened to be absent locally would be worse than not having the flag: a
// repository with a stale local master and an unrelated origin/main would
// retarget to main, create it, and leave the stale master untouched, so a flag
// whose name promises to create something absent would silently stop repairing
// what was present.
func (c *client) resolveSyncBase(
	ctx context.Context, repo *Repository, repoPath, remote, current string, opts BaseSyncOptions,
) (BaseBranchInfo, error) {
	base, err := c.ResolveBase(ctx, repo, opts.Candidates)
	if err != nil {
		return base, fmt.Errorf("failed to resolve base branch: %w", err)
	}

	if !opts.CreateMissing || !opts.Fetch || !baseNeedsCreation(base, current) {
		return base, nil
	}

	if name := c.missingLocalBase(ctx, repoPath, remote, opts.Candidates); name != "" {
		base.Name = name
		base.Source = baseSourceRemote
	}

	return base, nil
}

// baseNeedsCreation reports whether the resolved base leaves the repository
// with nothing this sync can fix.
//
// Two cases qualify. Nothing resolved at all, and — the common one — the base
// resolved to the branch that is checked out, where the pull path already owns
// the ref and the sync has no work to do no matter how stale the trunk is.
//
// A config-declared base is never overridden even when it is checked out.
// Declaring a trunk is the repository telling the tool what its base is, and a
// flag that reads that declaration and then picks a different branch has
// decided it knows the repository better than the repository does.
func baseNeedsCreation(base BaseBranchInfo, current string) bool {
	if base.Name == "" {
		return true
	}
	if strings.HasPrefix(base.Source, baseSourceConfigPrefix) {
		return false
	}

	return base.Name == current
}

// createLocalBase materializes a base ref the repository does not have.
//
// Empty oldSHA is update-ref's "the ref must not exist" guard, the same
// compare-and-swap the move path uses and for the same reason: between the read
// that found no ref and this write, another process may have created the
// branch, and creating it here would then overwrite a ref this run never
// looked at.
func (c *client) createLocalBase(
	ctx context.Context, repoPath, localRef, baseName, remoteSHA string, dryRun bool,
) (BaseSyncAction, string, error) {
	if dryRun {
		return BaseSyncCreated, fmt.Sprintf("would create %s at %s", baseName, shortSHA(remoteSHA)), nil
	}

	if err := c.moveBaseRef(ctx, repoPath, localRef, remoteSHA, ""); err != nil {
		return BaseSyncSkipped, "", err
	}

	return BaseSyncCreated, "created at " + shortSHA(remoteSHA), nil
}

// missingLocalBase returns the first base candidate that the remote has and
// refs/heads does not, or "" when every candidate is either already local or
// absent from the remote too.
//
// "The remote has it" is asked of the remote, with ls-remote, and not of
// refs/remotes/<remote>/*. A remote-tracking ref is a local cache: it survives
// the branch being deleted upstream until someone prunes, and creating a local
// branch from one would resurrect a dead branch as a *local* trunk. Because
// heuristicBaseCandidates prefers main over master, a resurrected main would
// then win every subsequent ResolveBase in that repository — so `info`'s BASE
// column, MergedBranches and every cleanup candidate would from then on be
// computed against a branch that no longer exists anywhere, and nothing would
// ever look at the real trunk again.
//
// Asking the remote also fixes the case the flag was written for. A `clone
// --single-branch -b develop` has no origin/master ref at all, so a
// tracking-ref probe finds nothing to create in precisely the repositories that
// need it most.
//
// Config candidates are consulted before the heuristic list for the same reason
// ResolveBase does it: a repository that declares its trunk should not have one
// guessed for it.
//
// This is deliberately separate from firstExistingBranch rather than a flag on
// it: ResolveBase feeds `info`'s BASE column and MergedBranches, and widening
// its notion of "exists" would change what every base-relative reading in the
// tool means.
func (c *client) missingLocalBase(ctx context.Context, repoPath, remote string, candidates []string) string {
	onRemote := c.remoteBranches(ctx, repoPath, remote)
	if len(onRemote) == 0 {
		return ""
	}

	for _, list := range [][]string{candidates, heuristicBaseCandidates} {
		for _, candidate := range list {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			if c.refExists(ctx, repoPath, "refs/heads/"+candidate) {
				continue
			}
			if onRemote[candidate] {
				return candidate
			}
		}
	}

	return ""
}

// remoteBranches lists the branch names the remote actually has right now.
//
// Returns nil on any failure — an unreachable or unauthenticated remote must
// read as "we do not know", never as "the remote has no such branch", because
// the caller turns a positive answer into a new local branch.
func (c *client) remoteBranches(ctx context.Context, repoPath, remote string) map[string]bool {
	result, err := c.executor.RunWithEnv(ctx, repoPath, nonInteractiveEnv, "ls-remote", "--heads", remote)
	if err != nil || result.ExitCode != 0 {
		c.logger.Debug("SyncBase: ls-remote %s failed for %s: %v", remote, repoPath, err)
		return nil
	}

	names := make(map[string]bool)

	for _, line := range strings.Split(result.Stdout, "\n") {
		_, ref, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok {
			continue
		}

		if name := strings.TrimPrefix(ref, "refs/heads/"); name != ref && name != "" {
			names[name] = true
		}
	}

	return names
}

// refExists reports whether a fully-qualified ref resolves. A non-zero exit from
// rev-parse --verify --quiet is git's way of saying "absent", not "failed".
func (c *client) refExists(ctx context.Context, repoPath, ref string) bool {
	result, _ := c.executor.Run(ctx, repoPath, "rev-parse", "--verify", "--quiet", ref) //nolint:errcheck // exit≠0 means ref missing
	return result.ExitCode == 0
}

// shortSHA abbreviates a commit for display. Fixed-width rather than git's
// ambiguity-aware abbreviation: this appears in a table column, and a width that
// varies per repository would make the column ragged for no gain.
func shortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) > 8 {
		return sha[:8]
	}

	return sha
}
