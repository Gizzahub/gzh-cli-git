// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

// Retiring a non-canonical branch is the only bulk cleanup path allowed past the
// built-in protected-name list, so it is kept apart from the merged/stale/gone
// candidates: its authority comes from a declaration and a live ancestry proof,
// never from a name, and the evidence it rests on has to be current rather than
// merely cached.

import (
	"context"
	"fmt"
)

// appendNonCanonical adds branches that duplicate the repository's declared
// canonical branch, local and (when --remote is set) remote.
//
// This is the only candidate path allowed past isProtectedBranch's built-in
// name list, so it re-derives its own authority instead of inheriting one:
// the canonical branch must be declared for this repository, the branch must
// not be that branch, must not be the checked-out branch, must not match an
// operator --protect pattern, must not be a declared task branch, and must be
// an ancestor of the canonical branch. Nothing here is inferred from a name.
//
// A repository with no declaration contributes no candidates and no error —
// bulk mode runs across a whole tree, and one undeclared repository must not
// fail the run for the rest.
func (c *client) appendNonCanonical(
	ctx context.Context,
	repoPath, remote, currentBranch string,
	opts BulkCleanupOptions,
	result *RepositoryCleanupResult,
	toDelete []branchInfo,
) []branchInfo {
	if opts.CanonicalResolver == nil {
		return toDelete
	}
	canonical, taskPatterns, err := opts.CanonicalResolver(ctx, repoPath)
	if err != nil || canonical == "" {
		return toDelete
	}

	eligible := func(name string) bool {
		if name == canonical || name == currentBranch {
			return false
		}
		// Only a trunk name earns the bypass past the built-in protected list.
		// A merged release/1.2 or feature/x is an ancestor of the canonical
		// branch too, and sweeping those up here would quietly make
		// --non-canonical a superset of --merged.
		if !IsRetirableTrunkName(name) {
			return false
		}
		for _, pattern := range opts.ProtectPatterns {
			if matchBranchPattern(name, pattern) {
				return false
			}
		}
		return !MatchesAnyTaskPattern(name, taskPatterns)
	}

	// Refresh the canonical branch's remote-tracking ref before measuring
	// anything against it. That ref is a cache from the last fetch, and every
	// ancestry answer below is only as current as it is: a canonical branch
	// force-pushed to a rewritten history since that fetch still looks, locally,
	// like it contains the trunk about to be deleted. ls-remote cannot settle
	// this on its own — it returns an id without the object, so it can say the
	// tip moved but not whether it advanced or was rewound. Fetching is what
	// tells those apart. Best-effort: an unreachable remote leaves the cache in
	// place, and deleteCleanupRemoteBranch refuses rather than trusting it.
	c.refreshCanonicalTracking(ctx, repoPath, remote, canonical)

	// The declaration is a bare name, which is not yet a ref. A local candidate
	// may be measured against the local trunk, falling back to the remote one on
	// a fresh clone that has no local copy; a remote candidate may only ever be
	// measured against the remote trunk, since a local trunk that is ahead of
	// its own remote would otherwise authorize deleting commits the remote still
	// needs. No resolvable target means no candidates, not an unchecked delete.
	localRef := "refs/heads/" + canonical
	remoteRef := "refs/remotes/" + remote + "/" + canonical
	localTarget := c.firstExistingRef(ctx, repoPath, localRef, remoteRef)
	remoteTarget := c.firstExistingRef(ctx, repoPath, remoteRef)

	if localTarget != "" {
		// Only the fallback is cached evidence. Naming the canonical branch here
		// is what tells the delete path to re-check the live tip; measured
		// against the local ref there is nothing for a remote to invalidate, and
		// demanding the network there would break offline cleanup for nothing.
		cachedBasis := ""
		if localTarget == remoteRef {
			cachedBasis = canonical
		}
		toDelete = c.appendNonCanonicalLocals(
			ctx, repoPath, localTarget, cachedBasis, eligible, result, toDelete,
		)
	}

	if !opts.DeleteRemote || remoteTarget == "" {
		return toDelete
	}

	return c.appendNonCanonicalRemotes(ctx, repoPath, remote, canonical, remoteTarget, eligible, result, toDelete)
}

// appendNonCanonicalLocals collects local branches that are ancestors of target.
//
// git branch --merged <target> is the ancestry predicate itself, asked once
// instead of once per branch. isRefAncestor still re-asks it per candidate:
// this path deletes branches git protects by name, so the authorization is
// never delegated to a listing.
// cachedBasis is the canonical branch name when target is its remote-tracking
// copy, and empty when target is a ref this clone owns. It travels with each
// candidate so the delete path can tell an authorization that a remote can
// invalidate from one it cannot.
func (c *client) appendNonCanonicalLocals(
	ctx context.Context,
	repoPath, target, cachedBasis string,
	eligible func(string) bool,
	result *RepositoryCleanupResult,
	toDelete []branchInfo,
) []branchInfo {
	names, err := c.getMergedBranches(ctx, repoPath, target)
	if err != nil {
		return toDelete
	}

	canonicalSHA := ""
	if cachedBasis != "" {
		canonicalSHA = c.fullRefSHA(ctx, repoPath, target)
	}

	for _, name := range names {
		if !eligible(name) || containsBranch(toDelete, name, branchLocationLocal) {
			continue
		}
		if !c.isRefAncestor(ctx, repoPath, "refs/heads/"+name, target) {
			continue
		}
		toDelete = append(toDelete, branchInfo{
			name:         name,
			reason:       nonCanonicalReason,
			location:     branchLocationLocal,
			canonical:    cachedBasis,
			canonicalSHA: canonicalSHA,
		})
		result.NonCanonicalCount++
	}

	return toDelete
}

// appendNonCanonicalRemotes collects remote-tracking branches that are
// ancestors of target, which is always the remote copy of the canonical branch.
func (c *client) appendNonCanonicalRemotes(
	ctx context.Context,
	repoPath, remote, canonical, target string,
	eligible func(string) bool,
	result *RepositoryCleanupResult,
	toDelete []branchInfo,
) []branchInfo {
	names, err := c.listRemoteTrackingNames(ctx, repoPath, remote)
	if err != nil {
		return toDelete
	}

	for _, name := range names {
		if !eligible(name) || containsBranch(toDelete, name, branchLocationRemote) {
			continue
		}
		if !c.isRefAncestor(ctx, repoPath, remoteRef(remote, name), target) {
			continue
		}
		info, ok := c.remoteDeleteCandidate(ctx, repoPath, remote, name, nonCanonicalReason)
		if !ok {
			continue
		}
		info.canonical = canonical
		info.canonicalSHA = c.fullRefSHA(ctx, repoPath, target)
		toDelete = append(toDelete, info)
		result.NonCanonicalCount++
	}

	return toDelete
}

// refreshCanonicalTracking updates one remote-tracking ref: the declared
// canonical branch.
//
// It does not wait for --remote to be set. A clone with no local copy of the
// canonical branch measures its local candidates against this same cache, and
// `branch -D` takes the branch reflog with it, so the invocation without
// --remote is not the safe one — it is merely the one that looks safe.
//
// It is deliberately narrow. A full fetch --prune would change what every other
// classification sees, and bulk mode runs this across a whole tree; this path
// needs exactly one ref to be current, so it asks for exactly that one.
//
// --dry-run is not exempt. A preview that lists a remote deletion the real run
// would refuse is worse than a preview that touched the network: the operator
// confirms one thing and gets another. Answering "what would you delete on the
// remote" truthfully requires knowing where the remote actually is.
func (c *client) refreshCanonicalTracking(
	ctx context.Context, repoPath, remote, canonical string,
) {
	if canonical == "" {
		return
	}
	if remote == "" {
		remote = DefaultRemoteName
	}
	refspec := "+refs/heads/" + canonical + ":refs/remotes/" + remote + "/" + canonical
	_, _ = c.executor.RunWithEnv( //nolint:errcheck // best-effort; the delete path refuses on a stale or unreachable tip
		ctx, repoPath, nonInteractiveEnv, "fetch", "--quiet", remote, refspec,
	)
}

// requireCurrentCanonicalTip refuses a non-canonical remote deletion whose
// authorizing evidence may already be void.
//
// The ancestry that authorized this delete was measured against a cached
// remote-tracking ref. --force-with-lease does not cover that: the lease names
// the branch being deleted, never the branch the ancestry was measured against.
// So if the canonical branch moved on the remote since the ref was cached, the
// deletion is authorized by something no longer true — and the commits it drops
// exist nowhere else, since being an ancestor of the canonical branch was the
// whole argument for their being safe to lose.
func (c *client) requireCurrentCanonicalTip(ctx context.Context, repoPath, remote string, b branchInfo) error {
	if b.canonical == "" || b.canonicalSHA == "" {
		return fmt.Errorf(
			"retire %s/%s: no record of the canonical tip this deletion was measured against",
			remote, b.name,
		)
	}

	live, err := c.executor.RunWithEnv(
		ctx, repoPath, nonInteractiveEnv, "ls-remote", remote, "refs/heads/"+b.canonical,
	)
	if err != nil || live == nil || live.ExitCode != 0 {
		return fmt.Errorf(
			"retire %s/%s: could not reach %s to confirm %s still points where the ancestry was measured",
			remote, b.name, remote, b.canonical,
		)
	}

	liveSHA := firstField(live.Stdout)
	if liveSHA == "" {
		return fmt.Errorf(
			"retire %s/%s: %s no longer exists on %s; the branch that authorized this deletion is gone",
			remote, b.name, b.canonical, remote,
		)
	}
	if liveSHA != b.canonicalSHA {
		return fmt.Errorf(
			"retire %s/%s: %s moved on %s since the ancestry was measured (measured %s, remote now %s);"+
				" run `git fetch %s` and re-run",
			remote, b.name, b.canonical, remote, shortSHA(b.canonicalSHA), shortSHA(liveSHA), remote,
		)
	}

	return nil
}
