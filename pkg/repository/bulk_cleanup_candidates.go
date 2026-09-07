// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"
	"strings"
)

const (
	branchLocationLocal  = "local"
	branchLocationRemote = "remote"
)

// collectCleanupCandidates gathers local merged/stale/gone refs and, when
// DeleteRemote && IncludeMerged, remote-only merged refs. DeleteRemote &&
// IncludeSuperseded adds unmerged bot remotes whose version target already
// landed. Stale never considers remote-only names: an unmerged remote bot
// branch may be an open PR.
//
//nolint:gocognit,gocyclo // three cleanup types plus remote-merged pairing
func (c *client) collectCleanupCandidates(
	ctx context.Context,
	repoPath, baseBranch, remote, currentBranch string,
	opts BulkCleanupOptions,
	result *RepositoryCleanupResult,
) []branchInfo {
	var toDelete []branchInfo

	if opts.IncludeMerged {
		merged, err := c.getMergedBranches(ctx, repoPath, baseBranch)
		if err == nil {
			for _, b := range merged {
				if c.isProtectedBranch(b, currentBranch, opts.ProtectPatterns) {
					result.ProtectedCount++
					continue
				}
				if opts.BotsOnly && !IsBotBranch(b) {
					continue
				}
				toDelete = append(toDelete, branchInfo{name: b, reason: "merged", location: branchLocationLocal})
				result.MergedCount++
			}
		}
	}

	if opts.IncludeStale {
		stale, err := c.getStaleBranches(ctx, repoPath, opts.StaleThreshold)
		if err == nil {
			for _, b := range stale {
				if c.isProtectedBranch(b, currentBranch, opts.ProtectPatterns) {
					if !containsBranch(toDelete, b, branchLocationLocal) {
						result.ProtectedCount++
					}
					continue
				}
				if opts.BotsOnly && !IsBotBranch(b) {
					continue
				}
				if containsBranch(toDelete, b, branchLocationLocal) {
					continue
				}
				toDelete = append(toDelete, branchInfo{name: b, reason: "stale", location: branchLocationLocal})
				result.StaleCount++
			}
		}
	}

	if opts.IncludeGone {
		gone, err := c.getGoneBranches(ctx, repoPath)
		if err == nil {
			for _, b := range gone {
				if c.isProtectedBranch(b, currentBranch, opts.ProtectPatterns) {
					if !containsBranch(toDelete, b, branchLocationLocal) {
						result.ProtectedCount++
					}
					continue
				}
				if opts.BotsOnly && !IsBotBranch(b) {
					continue
				}
				if containsBranch(toDelete, b, branchLocationLocal) {
					continue
				}
				toDelete = append(toDelete, branchInfo{name: b, reason: "gone", location: branchLocationLocal})
				result.GoneCount++
			}
		}
	}

	if opts.IncludeNonCanonical {
		toDelete = c.appendNonCanonical(ctx, repoPath, remote, currentBranch, opts, result, toDelete)
	}

	if opts.DeleteRemote && opts.IncludeMerged {
		toDelete = c.appendRemoteMerged(ctx, repoPath, remote, baseBranch, currentBranch, opts, result, toDelete)
	}

	if opts.DeleteRemote && opts.IncludeSuperseded {
		toDelete = c.appendRemoteSuperseded(ctx, repoPath, remote, baseBranch, currentBranch, opts, result, toDelete)
	}

	return toDelete
}

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
	c.refreshCanonicalTracking(ctx, repoPath, remote, canonical, opts)

	// The declaration is a bare name, which is not yet a ref. A local candidate
	// may be measured against the local trunk, falling back to the remote one on
	// a fresh clone that has no local copy; a remote candidate may only ever be
	// measured against the remote trunk, since a local trunk that is ahead of
	// its own remote would otherwise authorize deleting commits the remote still
	// needs. No resolvable target means no candidates, not an unchecked delete.
	localTarget := c.firstExistingRef(
		ctx, repoPath,
		"refs/heads/"+canonical,
		"refs/remotes/"+remote+"/"+canonical,
	)
	remoteTarget := c.firstExistingRef(ctx, repoPath, "refs/remotes/"+remote+"/"+canonical)

	if localTarget != "" {
		toDelete = c.appendNonCanonicalLocals(ctx, repoPath, localTarget, eligible, result, toDelete)
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
func (c *client) appendNonCanonicalLocals(
	ctx context.Context,
	repoPath, target string,
	eligible func(string) bool,
	result *RepositoryCleanupResult,
	toDelete []branchInfo,
) []branchInfo {
	names, err := c.getMergedBranches(ctx, repoPath, target)
	if err != nil {
		return toDelete
	}

	for _, name := range names {
		if !eligible(name) || containsBranch(toDelete, name, branchLocationLocal) {
			continue
		}
		if !c.isRefAncestor(ctx, repoPath, "refs/heads/"+name, target) {
			continue
		}
		toDelete = append(toDelete, branchInfo{name: name, reason: nonCanonicalReason, location: branchLocationLocal})
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

// firstExistingRef returns the first candidate git can resolve, or "".
func (c *client) firstExistingRef(ctx context.Context, repoPath string, candidates ...string) string {
	for _, candidate := range candidates {
		if c.refExists(ctx, repoPath, candidate) {
			return candidate
		}
	}
	return ""
}

// remoteRef builds the remote-tracking ref name used for ancestry probes.
func remoteRef(remote, name string) string {
	if remote == "" {
		remote = DefaultRemoteName
	}
	return remote + "/" + name
}

// appendRemoteMerged adds remote-tracking names whose tips are ancestors of
// base. Local merged names that already have a same-named remote-tracking
// ref are scheduled for remote delete too — that is the advertised --remote
// behavior that previously did nothing.
func (c *client) appendRemoteMerged(
	ctx context.Context,
	repoPath, remote, baseBranch, currentBranch string,
	opts BulkCleanupOptions,
	result *RepositoryCleanupResult,
	toDelete []branchInfo,
) []branchInfo {
	remoteNames, err := c.listRemoteTrackingNames(ctx, repoPath, remote)
	if err != nil {
		return toDelete
	}
	remoteSet := make(map[string]bool, len(remoteNames))
	for _, name := range remoteNames {
		remoteSet[name] = true
	}

	for _, b := range toDelete {
		if b.location != branchLocationLocal || b.reason != "merged" {
			continue
		}
		if !remoteSet[b.name] {
			continue
		}
		if c.isProtectedBranch(b.name, currentBranch, opts.ProtectPatterns) {
			continue
		}
		if containsBranch(toDelete, b.name, branchLocationRemote) {
			continue
		}
		if !c.isRefAncestor(ctx, repoPath, remote+"/"+b.name, baseBranch) {
			continue
		}
		info, ok := c.remoteDeleteCandidate(ctx, repoPath, remote, b.name, "merged")
		if !ok {
			continue
		}
		toDelete = append(toDelete, info)
		result.MergedCount++
	}

	for _, name := range remoteNames {
		if c.isProtectedBranch(name, currentBranch, opts.ProtectPatterns) {
			continue
		}
		if opts.BotsOnly && !IsBotBranch(name) {
			continue
		}
		if containsBranch(toDelete, name, branchLocationRemote) {
			continue
		}
		if !c.isRefAncestor(ctx, repoPath, remote+"/"+name, baseBranch) {
			continue
		}
		info, ok := c.remoteDeleteCandidate(ctx, repoPath, remote, name, "merged")
		if !ok {
			continue
		}
		toDelete = append(toDelete, info)
		result.MergedCount++
	}

	return toDelete
}

// appendRemoteSuperseded adds remote-tracking bot names whose tips are not
// ancestors of base but whose version target is already on base. Human
// topic branches are never included: BotRemoteBranches only returns bot names.
func (c *client) appendRemoteSuperseded(
	ctx context.Context,
	repoPath, remote, baseBranch, currentBranch string,
	opts BulkCleanupOptions,
	result *RepositoryCleanupResult,
	toDelete []branchInfo,
) []branchInfo {
	repo := &Repository{Path: repoPath}
	_, superseded, _, err := c.BotRemoteBranches(ctx, repo, baseBranch)
	if err != nil {
		return toDelete
	}
	for _, name := range superseded {
		if c.isProtectedBranch(name, currentBranch, opts.ProtectPatterns) {
			continue
		}
		if containsBranch(toDelete, name, branchLocationRemote) {
			continue
		}
		info, ok := c.remoteDeleteCandidate(ctx, repoPath, remote, name, "superseded")
		if !ok {
			continue
		}
		toDelete = append(toDelete, info)
		result.SupersededCount++
	}
	return toDelete
}

// nonCanonicalReason tags candidates retired for duplicating the declared
// canonical branch. It reaches the JSON output as the entry's Reason.
const nonCanonicalReason = "non-canonical"

// executeCleanupDeletes attempts every candidate and returns both outcomes.
//
// It returns the failures rather than only logging them because the caller
// reports a count to the operator: a delete that git refused must not be
// counted as one that happened.
func (c *client) executeCleanupDeletes(
	ctx context.Context,
	repoPath, remote string,
	toDelete []branchInfo,
	logger Logger,
	relPath string,
) (deleted []branchInfo, failed []CleanupFailureEntry) {
	for _, b := range toDelete {
		err := c.deleteCleanupBranch(ctx, repoPath, remote, b)
		if err == nil {
			deleted = append(deleted, b)
			continue
		}
		logger.Warn("failed to delete branch", "repo", relPath, "branch", b.name, "location", b.location, "error", err)
		failed = append(failed, CleanupFailureEntry{
			Name:     b.name,
			Reason:   b.reason,
			Location: b.location,
			Error:    err.Error(),
		})
	}

	return deleted, failed
}

func (c *client) remoteDeleteCandidate(
	ctx context.Context, repoPath, remote, name, reason string,
) (branchInfo, bool) {
	if remote == "" {
		remote = DefaultRemoteName
	}
	sha := c.fullRefSHA(ctx, repoPath, "refs/remotes/"+remote+"/"+name)
	if sha == "" {
		sha = c.fullRefSHA(ctx, repoPath, remote+"/"+name)
	}
	if sha == "" {
		return branchInfo{}, false
	}
	return branchInfo{name: name, reason: reason, location: branchLocationRemote, sha: sha}, true
}

func (c *client) fullRefSHA(ctx context.Context, repoPath, ref string) string {
	sha, err := c.executor.RunOutput(ctx, repoPath, "rev-parse", "--verify", ref)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(sha)
}

// refreshCanonicalTracking updates one remote-tracking ref: the declared
// canonical branch, and only when remote deletions are actually on the table.
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
	ctx context.Context, repoPath, remote, canonical string, opts BulkCleanupOptions,
) {
	if !opts.DeleteRemote || canonical == "" {
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

// firstField returns the first whitespace-separated token of the first
// non-empty line, which for ls-remote is the object id.
func firstField(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 {
			return fields[0]
		}
	}

	return ""
}

// IsRemoteHeadRefusal recognizes the one remote-delete failure that is not a
// fault to report but a step the operator has not taken yet: the branch is still
// where the remote's HEAD points. A duplicate default branch is the common shape
// of the problem --non-canonical exists to clean up, so the raw git text —
// several lines about receive.denyDeleteCurrent, or a forge's own wording —
// would bury the only thing worth acting on.
//
// It lives here rather than in pkg/branch because both cleanup paths need the
// same wording and the dependency runs branch → repository.
func IsRemoteHeadRefusal(stderr string) bool {
	lowered := strings.ToLower(stderr)

	return strings.Contains(lowered, "refusing to delete the current branch") ||
		strings.Contains(lowered, "deletion of the current branch prohibited") ||
		strings.Contains(lowered, "default branch")
}

func (c *client) deleteCleanupBranch(ctx context.Context, repoPath, remote string, b branchInfo) error {
	if b.location == branchLocationRemote {
		return c.deleteCleanupRemoteBranch(ctx, repoPath, remote, b)
	}

	deleteArgs := []string{"branch", "-d", b.name}
	if b.reason == "stale" || b.reason == "gone" || b.reason == nonCanonicalReason {
		// -D for non-canonical is not a relaxation. git's -d asks whether the
		// branch is merged into HEAD or its upstream, which is a different
		// question from the one already answered: is it an ancestor of the
		// declared canonical branch. Leaving -d here would reject the retirement
		// of a duplicate trunk while sitting on a third branch.
		deleteArgs = []string{"branch", "-D", b.name}
	}

	result, err := c.executor.Run(ctx, repoPath, deleteArgs...)
	if err != nil {
		return fmt.Errorf("delete local %s: %w", b.name, err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("delete local %s: %s", b.name, firstLine(result.Stderr))
	}

	return nil
}

// deleteCleanupRemoteBranch pushes a leased deletion, then re-checks the remote.
//
// The re-check exists because a lease can fail for a reason that is not a
// refusal: someone else may have deleted the branch first, which is the outcome
// we wanted. Anything else is reported, with the default-branch refusal given
// the one wording an operator can act on.
func (c *client) deleteCleanupRemoteBranch(ctx context.Context, repoPath, remote string, b branchInfo) error {
	if remote == "" {
		remote = DefaultRemoteName
	}
	if b.sha == "" {
		return fmt.Errorf("delete %s/%s: no remote-tracking SHA to lease against", remote, b.name)
	}

	if b.reason == nonCanonicalReason {
		// The one classification allowed past the built-in protected-name list
		// is also the one whose authorization lives on another branch. Confirm
		// that branch has not moved before firing.
		if err := c.requireCurrentCanonicalTip(ctx, repoPath, remote, b); err != nil {
			return err
		}
	}

	ref := "refs/heads/" + b.name
	lease := "--force-with-lease=" + ref + ":" + b.sha

	result, err := c.executor.RunWithEnv(ctx, repoPath, nonInteractiveEnv, "push", lease, remote, ":"+ref)
	if err == nil && result != nil && result.ExitCode == 0 {
		return nil
	}

	// The refusal is classified from the whole of git's output, not from its
	// first line: the phrase worth matching ("default branch",
	// receive.denyDeleteCurrent) arrives several lines into the block. Only the
	// fallback message is shortened.
	stderr := ""
	if result != nil {
		stderr = result.Stderr
	}
	if strings.TrimSpace(stderr) == "" && err != nil {
		stderr = err.Error()
	}

	heads, lsErr := c.executor.RunWithEnv(ctx, repoPath, nonInteractiveEnv, "ls-remote", "--heads", remote, b.name)
	if lsErr == nil && heads != nil && heads.ExitCode == 0 && strings.TrimSpace(heads.Stdout) == "" {
		// The branch is gone from the remote, whoever removed it.
		return nil
	}

	if IsRemoteHeadRefusal(stderr) {
		return fmt.Errorf(
			"leased remote delete %s/%s: refused because it is still %s's default branch"+
				" — repoint the default branch first, then re-run",
			remote, b.name, remote,
		)
	}

	return fmt.Errorf("leased remote delete %s/%s: %s", remote, b.name, firstLine(stderr))
}

// firstLine keeps a git error to the one line worth showing in a bulk summary.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown error"
	}
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}

	return s
}

func recordCleanupBranches(result *RepositoryCleanupResult, branches []branchInfo) {
	seen := make(map[string]bool)
	for _, b := range branches {
		result.Branches = append(result.Branches, b.entry())
		if seen[b.name] {
			continue
		}
		seen[b.name] = true
		result.DeletedBranches = append(result.DeletedBranches, b.name)
	}
}

// branchInfo holds branch name, deletion reason, and where the ref lives.
type branchInfo struct {
	name     string
	reason   string // "merged", "stale", "gone", "superseded", "non-canonical"
	location string // "local", "remote"
	sha      string // full classified tip; required to lease a remote delete

	// canonical and canonicalSHA record what a non-canonical retirement was
	// justified by: the declared branch, and the cached tip the ancestry was
	// actually measured against. A remote delete re-checks that tip against the
	// remote before it fires, because --force-with-lease covers only the branch
	// being deleted and says nothing about the one that authorized it.
	canonical    string
	canonicalSHA string
}

func (b branchInfo) entry() CleanupBranchEntry {
	return CleanupBranchEntry{
		Name:     b.name,
		Reason:   b.reason,
		Location: b.location,
		Kind:     BotKind(b.name),
	}
}

// containsBranch reports whether name is already scheduled at location.
func containsBranch(list []branchInfo, name, location string) bool {
	for _, b := range list {
		if b.name == name && b.location == location {
			return true
		}
	}
	return false
}
