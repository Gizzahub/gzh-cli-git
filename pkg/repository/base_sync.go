// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"
	"strings"
)

// Why this exists at all: `git fetch` updates refs/remotes/*, and `git pull`
// updates whichever branch happens to be checked out. A base branch that is
// never checked out is updated by neither. It sits at whatever commit it held
// on clone day while the branch the work actually happens on moves daily, and
// every base-relative reading — `info`'s BASE column, MergedBranches, the
// cleanup candidates — is computed against that stale ref.
//
// The failure is silent and it compounds. A local base 1200 commits behind its
// own remote reports the whole repository as "1200 ahead of base", which reads
// as unlanded work when nothing is unlanded. The remote is fine; only the local
// pointer is a months-old photograph. SyncBase moves that pointer.

// BaseSyncAction is the decision SyncBase reached for one repository's local
// base ref. It is reported even when nothing was written, so a dry run and a
// real run describe themselves in the same vocabulary.
type BaseSyncAction string

const (
	// BaseSyncUpToDate means the local base ref already equals its
	// remote-tracking ref. Nothing to do.
	BaseSyncUpToDate BaseSyncAction = "up-to-date"

	// BaseSyncFastForward means the local base ref was a strict ancestor of the
	// remote and was advanced to it. No local commit was dropped, because there
	// was no local-only commit to drop.
	BaseSyncFastForward BaseSyncAction = "fast-forward"

	// BaseSyncAdopted means the local base ref held commits the remote base
	// lacks, and the policy still moved the ref to the remote tip. Only reached
	// when decideBaseDivergence judged those commits recoverable.
	BaseSyncAdopted BaseSyncAction = "adopted"

	// BaseSyncBlocked means the local base ref diverged and the policy refused
	// to move it. The ref is left exactly as it was; this is a report, not a
	// failure of the update.
	BaseSyncBlocked BaseSyncAction = "blocked"

	// BaseSyncCreated means no local base ref existed and one was created from
	// the remote. Only reachable with BaseSyncOptions.CreateMissing: a
	// repository deliberately kept without a local trunk is a legitimate
	// choice, so materializing one is opt-in rather than a repair.
	BaseSyncCreated BaseSyncAction = "created"

	// BaseSyncSkipped means there was nothing to sync: no base resolved, no
	// remote-tracking counterpart, or the base is the checked-out branch and
	// the normal pull path already owns it.
	BaseSyncSkipped BaseSyncAction = "skipped"
)

// BaseSyncOptions configures one SyncBase call.
//
// This is a struct rather than a parameter list because the flags are all
// booleans: SyncBase(ctx, path, remote, candidates, true, false, true) states
// nothing about what those positions mean, and the compiler cannot catch two of
// them being swapped.
type BaseSyncOptions struct {
	// Remote is the remote whose base ref is the target. Required.
	Remote string

	// Candidates is the base-branch search order, typically
	// EffectiveConfig.Branch.DefaultBranch. Empty falls back to the heuristic
	// list ResolveBase already uses.
	Candidates []string

	// Fetch refreshes the base's remote-tracking ref before comparing.
	Fetch bool

	// DryRun reports the decision without writing any ref.
	DryRun bool

	// CreateMissing materializes a local base ref from the remote when the
	// repository has none. Off by default: absence of a local trunk is often
	// intentional on a develop-only clone, and creating a branch the user never
	// asked for is a different act than repairing one that fell behind.
	CreateMissing bool
}

// BaseDivergence counts how a local base ref and its remote-tracking
// counterpart differ. The three numbers answer three different questions and
// the distinction between LocalOnly and Stranded is the whole safety argument.
type BaseDivergence struct {
	// RemoteOnly is the number of commits the remote base has that the local
	// ref lacks — how far the local pointer fell behind.
	RemoteOnly int

	// LocalOnly is the number of commits on the local base ref that the remote
	// base does not have. Non-zero means this is not a fast-forward.
	LocalOnly int

	// Stranded is how many of the LocalOnly commits are reachable from no
	// remote-tracking ref of this remote at all — not from the base, not from a
	// task branch, not from anything that was ever pushed.
	//
	// This is the number that decides whether local-only commits represent real
	// unpushed work or a stale pointer parked on a branch that was pushed
	// elsewhere. A local base sitting on the tip of a task branch that lives on
	// the remote has LocalOnly > 0 and Stranded == 0: the commits exist on
	// origin under another name, so moving the pointer loses no history. A base
	// carrying genuinely unpushed commits has Stranded > 0, and moving the
	// pointer would strand them in the reflog.
	Stranded int
}

// BaseSyncResult reports what SyncBase decided and did for one repository.
type BaseSyncResult struct {
	// Base is the resolved base branch name, empty when none resolved.
	Base string

	// BaseSource echoes BaseBranchInfo.Source so a caller can explain which
	// policy picked this branch.
	BaseSource string

	// Remote is the remote whose base ref was used as the target.
	Remote string

	// Action is the decision reached.
	Action BaseSyncAction

	// Advanced is how many commits the local base ref gained. Zero for every
	// action except FastForward and Adopted, and zero on a dry run.
	Advanced int

	// Divergence holds the counts the decision was made from.
	Divergence BaseDivergence

	// Reason explains a Skipped or Blocked action in one clause. Empty when the
	// action speaks for itself.
	Reason string
}

// SyncBase fast-forwards a repository's local base ref to its remote-tracking
// counterpart without checking it out.
//
// The base branch is resolved with ResolveBase, deliberately: `info` reports
// divergence against that branch, and a command that repaired a *different*
// branch than the one being reported on would leave the report unchanged and
// look broken. opts.Candidates is the same list `info` passes — typically
// EffectiveConfig.Branch.DefaultBranch.
//
// When opts.Fetch is true the base ref is refreshed from the remote first. That
// is not redundant with the pull the caller may have just run: the pull path
// exits early on a dirty tree, on an already-current branch, and on a missing
// upstream, so in exactly the repositories most likely to have a stale base
// there was no fetch at all.
//
// Nothing here touches the working tree or the checked-out branch. A dirty
// worktree is not a reason to skip: updating a ref that nothing is checked out
// on cannot disturb uncommitted work.
func (c *client) SyncBase(ctx context.Context, repoPath string, opts BaseSyncOptions) (BaseSyncResult, error) {
	remote := strings.TrimSpace(opts.Remote)
	out := BaseSyncResult{Remote: remote, Action: BaseSyncSkipped}

	if remote == "" {
		out.Reason = "no remote configured"
		return out, nil
	}

	repo, err := c.Open(ctx, repoPath)
	if err != nil {
		return out, fmt.Errorf("failed to open repository: %w", err)
	}

	base, err := c.resolveSyncBase(ctx, repo, repoPath, remote, opts)
	if err != nil {
		return out, err
	}
	if base.Name == "" {
		out.Reason = "no base branch resolved"
		return out, nil
	}
	out.Base = base.Name
	out.BaseSource = base.Source

	current, err := c.executor.RunOutput(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return out, fmt.Errorf("failed to read current branch: %w", err)
	}
	if strings.TrimSpace(current) == base.Name {
		// Not a no-op we are hiding: the pull path already advanced this ref,
		// and re-pointing a checked-out branch behind its own working tree is
		// how you produce a repository that reports phantom deletions.
		out.Reason = "base is checked out; pull owns it"
		return out, nil
	}

	if opts.Fetch && !opts.DryRun {
		// Refresh only this one ref rather than the whole remote. The leading +
		// is not a force in any meaningful sense — a remote-tracking ref exists
		// to mirror the remote, and that is precisely what the default fetch
		// refspec does.
		refspec := fmt.Sprintf("+refs/heads/%s:refs/remotes/%s/%s", base.Name, remote, base.Name)
		if _, err := c.executor.RunWithEnv(ctx, repoPath, nonInteractiveEnv, "fetch", remote, refspec); err != nil {
			c.logger.Debug("SyncBase: fetch of %s failed for %s: %v", refspec, repoPath, err)
			// A failed fetch is not fatal. Fall through and compare against
			// whatever remote-tracking ref is already present; the worst case is
			// that we conclude "up-to-date" against slightly old data, which is
			// the status quo, not a regression.
		}
	}

	remoteRef := "refs/remotes/" + remote + "/" + base.Name
	remoteSHA, err := c.executor.RunOutput(ctx, repoPath, "rev-parse", "--verify", "--quiet", remoteRef)
	if err != nil || remoteSHA == "" {
		out.Reason = "no " + remote + "/" + base.Name
		return out, nil //nolint:nilerr // a base that was never pushed is a normal state, not an error
	}

	localRef := "refs/heads/" + base.Name
	localSHA, err := c.executor.RunOutput(ctx, repoPath, "rev-parse", "--verify", "--quiet", localRef)
	if err != nil || localSHA == "" { //nolint:nilerr // an absent ref is a state to report, not an error to raise
		if !opts.CreateMissing {
			out.Reason = "no local " + base.Name
			return out, nil
		}
		action, reason, err := c.createLocalBase(ctx, repoPath, localRef, base.Name, remoteSHA, opts.DryRun)
		if err != nil {
			return out, err
		}
		out.Action, out.Reason = action, reason
		return out, nil
	}

	if localSHA == remoteSHA {
		out.Action = BaseSyncUpToDate
		return out, nil
	}

	div, err := c.baseDivergence(ctx, repoPath, localRef, remoteRef, remote)
	if err != nil {
		return out, err
	}
	out.Divergence = div

	if div.LocalOnly == 0 {
		// Strict ancestor: the only possible outcome of moving the pointer is
		// that it catches up. This is the case that covers essentially every
		// repository where the base was simply never checked out.
		if opts.DryRun {
			out.Action = BaseSyncFastForward
			out.Reason = fmt.Sprintf("would advance %d commits", div.RemoteOnly)
			return out, nil
		}
		if err := c.moveBaseRef(ctx, repoPath, localRef, remoteSHA, localSHA); err != nil {
			return out, err
		}
		out.Action = BaseSyncFastForward
		out.Advanced = div.RemoteOnly
		return out, nil
	}

	action, reason := decideBaseDivergence(div)
	out.Reason = reason

	if action != BaseSyncAdopted {
		out.Action = BaseSyncBlocked
		return out, nil
	}

	if opts.DryRun {
		out.Action = BaseSyncAdopted
		out.Reason = fmt.Sprintf("would adopt remote tip (%s)", reason)
		return out, nil
	}
	if err := c.moveBaseRef(ctx, repoPath, localRef, remoteSHA, localSHA); err != nil {
		return out, err
	}
	out.Action = BaseSyncAdopted
	out.Advanced = div.RemoteOnly
	return out, nil
}

// decideBaseDivergence decides what to do when the local base ref is NOT a
// fast-forward of its remote counterpart — the local ref holds commits the
// remote base does not have. The fast-forward case never reaches here.
//
// Return BaseSyncAdopted to move the local ref to the remote tip anyway, or
// BaseSyncBlocked to leave it alone and report. The returned string is shown to
// the user as the reason and should read as one clause, e.g.
// "3 unpushed commits" or "all local commits exist on origin".
//
// The trade-off, concretely:
//
//   - Always blocking is safe but the report never clears. A base ref parked on
//     a task-branch tip stays flagged on every run forever, and a warning that
//     never goes away stops being read.
//   - Always adopting keeps every repository clean but silently moves the ref
//     off genuinely unpushed commits. They survive in the reflog for 90 days,
//     which is recovery, not safety.
//   - d.Stranded separates the two: it counts local-only commits reachable from
//     no remote ref at all. Stranded == 0 means every local-only commit was
//     already pushed under some other branch name, so moving the pointer loses
//     no history.
//
// The policy is therefore: adopt when nothing would be stranded, block when
// anything would. It is not a heuristic about how likely the commits are to
// matter — it is the difference between a pointer that can be recreated from
// origin and one that cannot.
//
// Note what this does NOT do: it never inspects the reflog, and it never treats
// "recoverable from the reflog" as safe. A 90-day expiry that the user has to
// know to reach into is a recovery procedure, not a guarantee, and a tool that
// silently relies on it has moved the cost of its decision onto the user.
func decideBaseDivergence(d BaseDivergence) (action BaseSyncAction, reason string) {
	if d.Stranded > 0 {
		// Deliberately reports Stranded and not LocalOnly. LocalOnly is often much
		// larger and mostly harmless, and a message that says "47 local commits"
		// when 2 are actually at risk teaches the user to dismiss it.
		return BaseSyncBlocked, fmt.Sprintf("%d commit(s) exist only here", d.Stranded)
	}
	return BaseSyncAdopted, fmt.Sprintf("%d local commit(s) already pushed elsewhere", d.LocalOnly)
}

// resolveSyncBase picks the branch this sync will act on.
//
// ResolveBase looks only at refs/heads, which is right for reporting: a branch
// that does not exist locally cannot be what local work diverged from. The
// consequence is that a clone which only ever checked out develop does not
// resolve to "no base" — it falls through ResolveBase's heuristic list and
// lands on develop, the branch that *is* checked out, so the sync hands off to
// the pull path and never repairs anything. Those are exactly the repositories
// a base sync would help most.
//
// So the create path is not conditioned on "nothing resolved". It asks a
// different question: is a preferred base present on the remote and absent
// here? Config-declared bases that do resolve locally are left alone —
// overriding an explicit configuration would be this flag deciding it knows the
// repository's trunk better than the repository does.
func (c *client) resolveSyncBase(ctx context.Context, repo *Repository, repoPath, remote string, opts BaseSyncOptions) (BaseBranchInfo, error) {
	base, err := c.ResolveBase(ctx, repo, opts.Candidates)
	if err != nil {
		return base, fmt.Errorf("failed to resolve base branch: %w", err)
	}
	if !opts.CreateMissing || strings.HasPrefix(base.Source, baseSourceConfigPrefix) {
		return base, nil
	}
	if name := c.missingLocalBase(ctx, repoPath, remote, opts.Candidates); name != "" {
		base.Name = name
		base.Source = baseSourceRemote
	}
	return base, nil
}

// createLocalBase materializes a base ref the repository does not have.
//
// Empty oldSHA is update-ref's "the ref must not exist" guard, the same
// compare-and-swap the move path uses and for the same reason: between the read
// that found no ref and this write, another process may have created the
// branch, and creating it here would then overwrite a ref this run never
// looked at.
func (c *client) createLocalBase(ctx context.Context, repoPath, localRef, baseName, remoteSHA string, dryRun bool) (BaseSyncAction, string, error) {
	if dryRun {
		return BaseSyncCreated, fmt.Sprintf("would create %s at %s", baseName, shortSHA(remoteSHA)), nil
	}
	if err := c.moveBaseRef(ctx, repoPath, localRef, remoteSHA, ""); err != nil {
		return BaseSyncSkipped, "", err
	}
	return BaseSyncCreated, "created at " + shortSHA(remoteSHA), nil
}

// missingLocalBase returns the first base candidate that exists on remote but
// not in refs/heads, or "" when every candidate is either already local or not
// on the remote either.
//
// Config candidates are consulted before the heuristic list for the same reason
// ResolveBase does it: a repository that declares its trunk should not have one
// guessed for it. The heuristic pass is what covers the repositories that
// declare nothing, which is most of them.
//
// This is deliberately separate from firstExistingBranch rather than a flag on
// it: ResolveBase feeds `info`'s BASE column and MergedBranches, and widening
// its notion of "exists" would change what every base-relative reading in the
// tool means.
func (c *client) missingLocalBase(ctx context.Context, repoPath, remote string, candidates []string) string {
	for _, list := range [][]string{candidates, heuristicBaseCandidates} {
		for _, candidate := range list {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			if c.refExists(ctx, repoPath, "refs/heads/"+candidate) {
				continue
			}
			if c.refExists(ctx, repoPath, "refs/remotes/"+remote+"/"+candidate) {
				return candidate
			}
		}
	}
	return ""
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

// baseDivergence counts how the local and remote base refs differ.
func (c *client) baseDivergence(ctx context.Context, repoPath, localRef, remoteRef, remote string) (BaseDivergence, error) {
	var div BaseDivergence

	// rev-list --left-right --count A...B yields "<left>\t<right>": commits in A
	// not in B, then commits in B not in A. Same shape as ResolveBase's probe.
	output, err := c.executor.RunOutput(ctx, repoPath, "rev-list", "--left-right", "--count", localRef+"..."+remoteRef)
	if err != nil {
		return div, fmt.Errorf("failed to compare %s against %s: %w", localRef, remoteRef, err)
	}
	localOnly, remoteOnly, err := parseAheadBehind(output)
	if err != nil {
		return div, fmt.Errorf("unparseable divergence %q: %w", output, err)
	}
	div.LocalOnly = localOnly
	div.RemoteOnly = remoteOnly

	if div.LocalOnly == 0 {
		// A fast-forward has nothing to strand, and --not --remotes over a large
		// repository is the expensive probe here. Skip it when the answer is
		// already known.
		return div, nil
	}

	// "Reachable from no remote-tracking ref" is the question, not "reachable
	// from the remote base". A commit pushed on a task branch is safe on the
	// remote even though the remote base has never seen it, and treating it as
	// unpushed work is what makes an always-block policy unusable.
	stranded, err := c.executor.RunOutput(ctx, repoPath, "rev-list", "--count", localRef, "--not", "--remotes="+remote)
	if err != nil {
		return div, fmt.Errorf("failed to count stranded commits on %s: %w", localRef, err)
	}
	if _, err := fmt.Sscanf(strings.TrimSpace(stranded), "%d", &div.Stranded); err != nil {
		return div, fmt.Errorf("unparseable stranded count %q: %w", stranded, err)
	}

	return div, nil
}

// moveBaseRef points a local branch ref at a new commit without checking it out.
//
// The three-argument form of update-ref is a compare-and-swap: git refuses the
// write unless the ref still points at oldSHA. Bulk update runs many
// repositories concurrently and a user may be working in one of them; without
// the guard, a ref that moved between the read and the write would be silently
// overwritten with a decision made about a state that no longer exists.
//
// An empty oldSHA is git's spelling of "and the ref must not exist yet", so the
// create path gets the same guarantee without a second code path.
func (c *client) moveBaseRef(ctx context.Context, repoPath, ref, newSHA, oldSHA string) error {
	result, err := c.executor.Run(ctx, repoPath, "update-ref", ref, newSHA, oldSHA)
	if err != nil {
		return fmt.Errorf("failed to update %s: %w", ref, err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to update %s: %s", ref, strings.TrimSpace(result.Stderr))
	}
	return nil
}
