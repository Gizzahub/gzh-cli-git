// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"
	"strings"
)

// What to do when a local base ref and its remote have genuinely diverged:
// how the divergence is measured, what is decided from it, and how the ref is
// written. Kept apart from the orchestration in base_sync.go because this is
// the part with a safety argument to defend.

// baseSyncRefs is what the adopt decision needs to know about one base ref,
// grouped so the four strings cannot be passed in the wrong order.
type baseSyncRefs struct {
	name      string
	localRef  string
	localSHA  string
	remoteSHA string
}

// baseBackupRefPrefix is where the previous tip of an adopted base is parked.
//
// Under refs/gz-git/ rather than refs/heads/ so a backup never becomes a branch
// the user has to notice, never appears in `git branch`, and can never be
// picked as a base by ResolveBase on a later run. It is still a ref, which is
// the only property that matters: it keeps the commits reachable, and reachable
// is what the reflog is not.
const baseBackupRefPrefix = "refs/gz-git/base-backup/"

// adoptOrBlock applies decideBaseDivergence and, when it says adopt, moves the
// ref — but only after parking the tip it is moving off.
//
// The backup is what makes adopting defensible, and it is not belt-and-braces.
// BaseDivergence.Stranded is computed with `rev-list --not --remotes=<remote>`,
// and refs/remotes/* is a local cache, not the remote: a tracking ref for a
// branch someone deleted upstream survives until the next prune, and until then
// it stands as evidence that a commit "exists on the remote" when it no longer
// does anywhere. Stranded can therefore read 0 for work that was never pushed.
//
// Making the count sound would mean an authoritative ls-remote per repository
// on the *repair* path, which runs everywhere by default. Parking the old tip
// costs one ref write and makes the question moot: adopt can be wrong about
// where a commit lives and still not lose it. Recover with
//
//	git -C <repo> branch recovered refs/gz-git/base-backup/<base>
func (c *client) adoptOrBlock(
	ctx context.Context, repoPath string, refs baseSyncRefs, div BaseDivergence,
	opts BaseSyncOptions, out BaseSyncResult,
) (BaseSyncResult, error) {
	action, reason := decideBaseDivergence(div)
	out.Reason = reason

	if action != BaseSyncAdopted {
		out.Action = BaseSyncBlocked
		return out, nil
	}

	backup := baseBackupRefPrefix + refs.name
	out.Action = BaseSyncAdopted
	out.Backup = backup

	if opts.DryRun {
		out.Reason = fmt.Sprintf("would adopt remote tip (%s)", reason)
		return out, nil
	}

	// Unconditional rather than compare-and-swap, unlike every other write
	// here: an earlier adopt in this repository leaves a backup behind, and
	// refusing to overwrite it would fail the second adopt rather than protect
	// anything. The ref being overwritten is one this tool wrote, holding a tip
	// that was already judged recoverable when it was written.
	if err := c.forceBaseRef(ctx, repoPath, backup, refs.localSHA); err != nil {
		return out, err
	}

	if err := c.moveBaseRef(ctx, repoPath, refs.localRef, refs.remoteSHA, refs.localSHA); err != nil {
		return out, err
	}

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
// silently relies on it has moved the cost of its decision onto the user. What
// adoption relies on instead is the backup ref adoptOrBlock writes, which is
// reachable and does not expire.
func decideBaseDivergence(d BaseDivergence) (action BaseSyncAction, reason string) {
	if d.Stranded > 0 {
		// Deliberately reports Stranded and not LocalOnly. LocalOnly is often much
		// larger and mostly harmless, and a message that says "47 local commits"
		// when 2 are actually at risk teaches the user to dismiss it.
		return BaseSyncBlocked, fmt.Sprintf("%d commit(s) exist only here", d.Stranded)
	}

	return BaseSyncAdopted, fmt.Sprintf("%d local commit(s) already pushed elsewhere", d.LocalOnly)
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
	//
	// Note the direction of every way this can be wrong. A commit held by a tag,
	// by another local branch, or by a different remote raises the count and so
	// blocks — conservative. The one direction that lowers it is a stale
	// tracking ref, which adoptOrBlock's backup covers.
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
//
// What update-ref does NOT guard is checkouts — it is plumbing, and it will
// rewind a branch a linked worktree is standing on where `git branch -f`
// refuses. SyncBase.checkedOutReason is what supplies that guard.
func (c *client) moveBaseRef(ctx context.Context, repoPath, ref, newSHA, oldSHA string) error {
	return c.writeRef(ctx, repoPath, ref, newSHA, oldSHA)
}

// forceBaseRef writes a ref with no compare-and-swap. Only for refs this tool
// owns outright; never for a branch.
func (c *client) forceBaseRef(ctx context.Context, repoPath, ref, newSHA string) error {
	return c.writeRef(ctx, repoPath, ref, newSHA)
}

func (c *client) writeRef(ctx context.Context, repoPath, ref, newSHA string, oldSHA ...string) error {
	args := append([]string{"update-ref", ref, newSHA}, oldSHA...)

	result, err := c.executor.Run(ctx, repoPath, args...)
	if err != nil {
		return fmt.Errorf("failed to update %s: %w", ref, err)
	}

	if result.ExitCode != 0 {
		return fmt.Errorf("failed to update %s: %s", ref, strings.TrimSpace(result.Stderr))
	}

	return nil
}
