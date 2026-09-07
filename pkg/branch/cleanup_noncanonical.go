// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package branch

// Retiring a non-canonical branch is the one classification allowed past the
// built-in protected-name list, so its authorization lives here, apart from the
// merged/stale/gone vocabulary: every helper in this file exists to answer one
// question — does the declared canonical branch demonstrably already hold this
// branch, right now, on evidence that is not a stale cache.

import (
	"context"
	"fmt"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// authorizeRetire re-verifies, at deletion time, that a non-canonical candidate
// is safe to delete. It fails closed on every uncertainty: no declared canonical
// branch, a candidate that is the canonical branch, an operator --protect match,
// or an ancestry check that does not come back clean.
func (c *cleanupService) authorizeRetire(ctx context.Context, repo *repository.Repository, branch *Branch, opts ExecuteOptions) bool {
	canonical := strings.TrimSpace(opts.CanonicalBranch)
	if canonical == "" || branch == nil {
		return false
	}

	if branch.Name == canonical || !repository.IsRetirableTrunkName(branch.Name) {
		return false
	}

	for _, pattern := range opts.Exclude {
		if matchPattern(branch.Name, pattern) {
			return false
		}
	}

	target := c.canonicalTargetRef(ctx, repo, branch, canonical, opts.CanonicalRemote)
	return c.isAncestorOf(ctx, repo, cleanupBranchRef(branch), target)
}

// isNonCanonical reports whether branch duplicates the declared canonical branch
// and may therefore be retired.
//
// The four conditions are an allow-list, not a deny-list, and that ordering is
// the safety property: a deny-list ("delete what is not canonical") would sweep
// up release lines, other people's work, and anything the declaration simply
// failed to mention. Every condition must hold.
//
//  1. A canonical branch was declared. Without it there is no baseline, so the
//     answer is always false — the command does nothing rather than guessing.
//  2. The branch is not the canonical branch itself, under any of the spellings
//     git reports it (local name, remotes/<remote>/<name>).
//  3. The branch is not a declared task branch. Those belong to the reclaim
//     path, which has its own lifecycle and its own gate.
//  4. The branch carries no commit the canonical branch lacks. This is the gate
//     that makes deletion lossless by construction, and it is asked of git, not
//     inferred from names or dates.
func (c *cleanupService) isNonCanonical(ctx context.Context, repo *repository.Repository, branch *Branch, opts AnalyzeOptions) bool {
	canonical := strings.TrimSpace(opts.CanonicalBranch)
	if canonical == "" {
		return false
	}

	if branch.Name == canonical {
		return false
	}

	// The bypass is only ever granted to a trunk name. Everything else that is
	// an ancestor of the canonical branch is what --merged is for, and the
	// operator has to ask for it there.
	if !repository.IsRetirableTrunkName(branch.Name) {
		return false
	}

	// A user-supplied --protect pattern is honored even here. Built-in name
	// protection is what this classification is allowed to override; an explicit
	// instruction from the operator is not.
	for _, pattern := range opts.Exclude {
		if matchPattern(branch.Name, pattern) {
			return false
		}
	}

	if repository.MatchesAnyTaskPattern(branch.Name, opts.TaskPatterns) {
		return false
	}

	target := c.canonicalTargetRef(ctx, repo, branch, canonical, opts.CanonicalRemote)
	return c.isAncestorOf(ctx, repo, cleanupBranchRef(branch), target)
}

// cleanupBranchRef returns the ref to hand to git for a classified branch.
//
// normalizeCleanupBranch shortens Name to the bare branch name so that local and
// remote copies compare and print alike, which means a remote branch's Name no
// longer resolves once its local namesake is deleted: git looks up a bare name
// under refs/heads and refs/tags, never under refs/remotes/<remote>/. Ref keeps
// the full path, so ancestry must be asked of Ref, not Name.
func cleanupBranchRef(branch *Branch) string {
	if branch == nil {
		return ""
	}
	if branch.Ref != "" {
		return branch.Ref
	}
	return branch.Name
}

// refreshCanonicalTracking updates the one remote-tracking ref the
// non-canonical classification depends on. It is deliberately narrow: one ref
// from one remote, not a whole fetch.
//
// It is not conditioned on a remote retirement being on the table. A local
// retirement in a clone with no local canonical branch is measured against the
// same cache and deletes the last ref holding a commit just as permanently, so
// tying the refresh to --remote would leave the safer-looking invocation the
// only unsafe one.
func (c *cleanupService) refreshCanonicalTracking(
	ctx context.Context, repo *repository.Repository, opts AnalyzeOptions,
) {
	canonical := strings.TrimSpace(opts.CanonicalBranch)
	if !opts.IncludeNonCanonical || canonical == "" {
		return
	}
	governed := governedRemote(opts.CanonicalRemote)
	refspec := "+refs/heads/" + canonical + ":refs/remotes/" + governed + "/" + canonical
	// Errors are not propagated: a repository that cannot reach its remote must
	// still be able to run the local half of cleanup. What must not happen is a
	// remote deletion on stale evidence, and that is Execute's gate, not this.
	_, _ = c.executor.RunWithEnv( //nolint:errcheck // best-effort refresh; Execute refuses on a stale or unreachable tip
		ctx, repo.Path, repository.NonInteractiveEnv(), "fetch", "--quiet", governed, refspec,
	)
}

// requireCurrentCanonicalTip confirms the remote's canonical branch still points
// where the classification measured against.
//
// It fails closed on every uncertainty — an unreadable cache, an unreachable
// remote, a canonical branch missing from the remote — because the alternative
// is deleting the last ref holding a commit on the strength of a stale answer.
// A tip that merely advanced is refused too: this compares identity, not
// ancestry, and re-running after a fetch is the cheap, safe way to say yes.
// retirementRestsOnCache reports whether this candidate's authorization comes
// from a remote-tracking ref rather than from a ref this clone owns.
//
// It mirrors canonicalTargetRef's resolution instead of re-deriving it, because
// the two must never disagree: the gate has to fire on exactly the cases the
// target resolution decided to trust a cache for. A remote candidate always
// rests on the cache. A local one rests on it only when this clone has no
// refs/heads copy of the canonical branch — with one, the ancestry is measured
// against a local ref that no remote can move behind our backs, and requiring
// the network there would break offline cleanup for no safety gain.
func (c *cleanupService) retirementRestsOnCache(
	ctx context.Context, repo *repository.Repository, branch *Branch, opts ExecuteOptions,
) bool {
	if branch == nil {
		return false
	}
	if branch.IsRemote {
		return true
	}
	canonical := strings.TrimSpace(opts.CanonicalBranch)
	if canonical == "" {
		return false
	}
	return c.firstExistingRef(ctx, repo, "refs/heads/"+canonical) == ""
}

func (c *cleanupService) requireCurrentCanonicalTip(
	ctx context.Context, repo *repository.Repository, branch *Branch, opts ExecuteOptions,
) error {
	canonical := strings.TrimSpace(opts.CanonicalBranch)
	governed := governedRemote(opts.CanonicalRemote)

	cached, err := c.run(ctx, repo.Path, "rev-parse", "--verify", "--quiet",
		"refs/remotes/"+governed+"/"+canonical)
	if err != nil || cached == nil || cached.ExitCode != 0 {
		return fmt.Errorf(
			"retire %s: no cached tip for %s/%s to confirm the ancestry against",
			branch.Name, governed, canonical,
		)
	}
	cachedSHA := strings.TrimSpace(cached.Stdout)

	live, err := c.executor.RunWithEnv(ctx, repo.Path, repository.NonInteractiveEnv(),
		"ls-remote", governed, "refs/heads/"+canonical)
	if err != nil || live == nil || live.ExitCode != 0 {
		return fmt.Errorf(
			"retire %s: could not reach %s to confirm %s still points where the ancestry was measured",
			branch.Name, governed, canonical,
		)
	}
	liveSHA := firstField(live.Stdout)
	if liveSHA == "" {
		return fmt.Errorf(
			"retire %s: %s/%s no longer exists on the remote;"+
				" the branch that authorized this deletion is gone",
			branch.Name, governed, canonical,
		)
	}

	if liveSHA != cachedSHA {
		return fmt.Errorf(
			"retire %s: %s/%s moved on the remote since the last fetch (cached %s, remote %s);"+
				" the ancestry that authorized this deletion was measured against the cached tip"+
				" — run `git fetch %s` and re-run",
			branch.Name, governed, canonical, shortSHA(cachedSHA), shortSHA(liveSHA), governed,
		)
	}
	return nil
}

// firstField returns the first whitespace-delimited field of the first non-empty
// line, which for ls-remote output is the object id.
func firstField(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 {
			return fields[0]
		}
	}
	return ""
}

// shortSHA abbreviates an object id for an operator-facing message.
func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// canonicalTargetRef resolves the declared canonical branch to the concrete ref
// this candidate's ancestry must be measured against, or "" when no such ref
// exists.
//
// The declaration is a bare name, and a bare name is not one ref. Handing it
// straight to git gets both halves of the safety property wrong:
//
//   - For a remote candidate, a bare name resolves to refs/heads/<canonical>.
//     A local trunk that is ahead of its own remote would then authorize
//     deleting a remote branch holding commits the remote trunk does not have.
//     A remote deletion must be justified by the remote trunk, so that is the
//     only ref accepted here.
//   - For a local candidate on a fresh clone there may be no local trunk at all
//     — the clone checked out one branch. The bare name resolves to nothing,
//     every probe fails closed, and the command reports "nothing to clean up"
//     in precisely the repository it exists to clean. So it falls back to the
//     remote-tracking trunk — but that ref is a cache, not the remote, and the
//     fallback is lossless only while the two agree. It does not hold on its
//     own: a local branch ahead of its own remote, measured against a canonical
//     branch rewound since the last fetch, is deleted with its commits on no
//     other ref. retirementRestsOnCache marks that case so the deletion is
//     gated on the live tip rather than on this assumption.
func (c *cleanupService) canonicalTargetRef(
	ctx context.Context, repo *repository.Repository, branch *Branch, canonical, governed string,
) string {
	governed = governedRemote(governed)
	if branch.IsRemote {
		remote, _ := remoteAndBranch(branch)
		// A candidate on any other remote is not this declaration's business.
		// Returning no target fails the ancestry probe closed, which is the
		// same refusal every other unresolvable case gets.
		if remote == "" || remote != governed {
			return ""
		}
		return c.firstExistingRef(ctx, repo, "refs/remotes/"+remote+"/"+canonical)
	}
	return c.firstExistingRef(
		ctx, repo,
		"refs/heads/"+canonical,
		"refs/remotes/"+governed+"/"+canonical,
	)
}

// governedRemote resolves the remote a declaration speaks for, defaulting to
// origin. The default is deliberately a name and not "any remote": a repository
// whose only remote is named something else simply yields no remote candidates,
// which is the safe direction to be wrong in.
func governedRemote(name string) string {
	if trimmed := strings.TrimSpace(name); trimmed != "" {
		return trimmed
	}
	return repository.DefaultRemoteName
}

// firstExistingRef returns the first candidate git can resolve, or "".
func (c *cleanupService) firstExistingRef(ctx context.Context, repo *repository.Repository, candidates ...string) string {
	for _, candidate := range candidates {
		result, err := c.run(ctx, repo.Path, "rev-parse", "--verify", "--quiet", candidate)
		if err == nil && result != nil && result.ExitCode == 0 {
			return candidate
		}
	}
	return ""
}
