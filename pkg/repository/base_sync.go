// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"
	"path/filepath"
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

	// BaseSyncFailed means the sync could not reach a decision at all — a git
	// command failed, the repository is unreadable. It is deliberately not
	// BaseSyncBlocked. Blocked is a verdict the policy reached on evidence and
	// carries an instruction to the user ("push these commits, then run
	// again"); failed carries none, because nothing was judged. Folding the two
	// together puts rows nobody can act on into the one list whose entire value
	// is that every row needs action.
	BaseSyncFailed BaseSyncAction = "failed"

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

	// Backup is the ref the previous base tip was parked at before an adopt
	// rewound it, empty when nothing was parked. Only Adopted sets it.
	Backup string

	// DryRun echoes BaseSyncOptions.DryRun.
	//
	// Advanced is zero both on a dry run and on a real adopt that only rewinds,
	// so a renderer cannot tell "nothing happened yet" from "a ref moved
	// backwards" without being told which run this was.
	DryRun bool
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
	out := BaseSyncResult{Remote: remote, Action: BaseSyncSkipped, DryRun: opts.DryRun}

	if remote == "" {
		out.Reason = "no remote configured"
		return out, nil
	}

	repo, err := c.Open(ctx, repoPath)
	if err != nil {
		return out, fmt.Errorf("failed to open repository: %w", err)
	}

	current, err := c.executor.RunOutput(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		// A repository with no commits has an unborn HEAD, which this call
		// cannot resolve. That is not a failure worth reporting: such a
		// repository has no base ref to sync and cannot have one until
		// something is committed. Probed only on the error path, so the 113
		// repositories that do have commits pay nothing for it.
		if !c.hasCommits(ctx, repoPath) {
			out.Reason = "repository has no commits"
			return out, nil
		}
		return out, fmt.Errorf("failed to read current branch: %w", err)
	}
	current = strings.TrimSpace(current)

	base, err := c.resolveSyncBase(ctx, repo, repoPath, remote, current, opts)
	if err != nil {
		return out, err
	}
	if base.Name == "" {
		out.Reason = "no base branch resolved"
		return out, nil
	}
	out.Base = base.Name
	out.BaseSource = base.Source

	if reason := c.checkedOutReason(ctx, repoPath, base.Name, current); reason != "" {
		out.Reason = reason
		return out, nil
	}

	return c.syncResolvedBase(ctx, repoPath, remote, base, opts, out)
}

// checkedOutReason reports why the base must be left alone because something
// has it checked out, or "" when nothing does.
//
// The HEAD of this repository is only half the question. `git update-ref` is
// plumbing and enforces no checkout rule at all — it will happily rewind a
// branch that a *linked* worktree is standing on, where the porcelain `git
// branch -f` refuses outright. The worktree is then left with an index that
// disagrees with HEAD, so every file the moved-off commits added shows as a
// staged deletion and the next commit made there quietly reverts them.
//
// That matters more since the adopt policy landed: fast-forwarding a ref under
// a worktree is confusing, but rewinding one off commits is destructive.
func (c *client) checkedOutReason(ctx context.Context, repoPath, baseName, current string) string {
	if current == baseName {
		// Not a no-op we are hiding: the pull path already advanced this ref,
		// and re-pointing a checked-out branch behind its own working tree is
		// how you produce a repository that reports phantom deletions.
		return "base is checked out; pull owns it"
	}
	if path := c.branchWorktree(ctx, repoPath, baseName); path != "" {
		return "base is checked out in worktree " + filepath.Base(path)
	}
	return ""
}

// branchWorktree returns the path of the worktree that has branch checked out,
// or "" when none does. A detached worktree reports `detached` instead of a
// branch line and so cannot match, which is correct: a detached HEAD owns no
// branch name and nothing breaks when that branch moves.
func (c *client) branchWorktree(ctx context.Context, repoPath, branch string) string {
	output, err := c.executor.RunOutput(ctx, repoPath, "worktree", "list", "--porcelain")
	if err != nil {
		// Best effort. A repository whose worktree list cannot be read is not a
		// reason to abort the sync; the HEAD check above still covers the
		// common single-worktree case.
		c.logger.Debug("SyncBase: worktree list failed for %s: %v", repoPath, err)
		return ""
	}

	want := "branch refs/heads/" + branch

	var path string

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "worktree "):
			path = strings.TrimPrefix(line, "worktree ")
		case line == want:
			return path
		}
	}

	return ""
}

// hasCommits reports whether any ref in the repository reaches a commit.
//
// The question is "does this repository have any history", not "does HEAD
// resolve", and the two come apart exactly where it matters. An unborn HEAD is
// not proof of an empty repository: `git checkout --orphan gh-pages` leaves a
// repository full of commits whose HEAD points at a branch that does not exist
// yet, and it answers `rev-parse --verify HEAD` identically to a clone of an
// empty remote. Reading that as "no commits" would skip a base ref that is
// genuinely stale, silently — the one outcome this whole change exists to
// prevent.
//
// Walking refs answers the real question. An empty `--all` also means no base
// branch can exist, which is the same conclusion by a route that cannot be
// wrong for the orphan case.
func (c *client) hasCommits(ctx context.Context, repoPath string) bool {
	result, err := c.executor.Run(ctx, repoPath, "rev-list", "-n", "1", "--all")
	if err != nil || result.ExitCode != 0 {
		// The probe itself could not run — a canceled context, an expired
		// timeout, an unreadable object store. That is not evidence of an empty
		// repository, and claiming it here would convert a real failure into a
		// silent skip. Report "has commits" so the caller's original error
		// surfaces as the failure it is.
		return true
	}
	return strings.TrimSpace(result.Stdout) != ""
}

// syncResolvedBase is the half of SyncBase that runs once a base branch is
// known: it exists, it is the one to act on, and nothing has it checked out.
func (c *client) syncResolvedBase(
	ctx context.Context, repoPath, remote string, base BaseBranchInfo, opts BaseSyncOptions, out BaseSyncResult,
) (BaseSyncResult, error) {
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

	return c.adoptOrBlock(ctx, repoPath, baseSyncRefs{
		name:      base.Name,
		localRef:  localRef,
		localSHA:  localSHA,
		remoteSHA: remoteSHA,
	}, div, opts, out)
}
