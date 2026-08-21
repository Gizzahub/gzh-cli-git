// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
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

	if opts.DeleteRemote && opts.IncludeMerged {
		toDelete = c.appendRemoteMerged(ctx, repoPath, remote, baseBranch, currentBranch, opts, result, toDelete)
	}

	if opts.DeleteRemote && opts.IncludeSuperseded {
		toDelete = c.appendRemoteSuperseded(ctx, repoPath, remote, baseBranch, currentBranch, opts, result, toDelete)
	}

	return toDelete
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

func (c *client) executeCleanupDeletes(
	ctx context.Context,
	repoPath, remote string,
	toDelete []branchInfo,
	logger Logger,
	relPath string,
) []branchInfo {
	var deleted []branchInfo
	for _, b := range toDelete {
		if c.deleteCleanupBranch(ctx, repoPath, remote, b) {
			deleted = append(deleted, b)
			continue
		}
		logger.Warn("failed to delete branch", "repo", relPath, "branch", b.name, "location", b.location)
	}
	return deleted
}

func (c *client) remoteDeleteCandidate(ctx context.Context, repoPath, remote, name, reason string) (branchInfo, bool) {
	if remote == "" {
		remote = defaultRemoteName
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

func (c *client) deleteCleanupBranch(ctx context.Context, repoPath, remote string, b branchInfo) bool {
	if b.location == branchLocationRemote {
		if remote == "" {
			remote = defaultRemoteName
		}
		if b.sha == "" {
			return false
		}
		ref := "refs/heads/" + b.name
		lease := "--force-with-lease=" + ref + ":" + b.sha
		result, err := c.executor.RunWithEnv(ctx, repoPath, nonInteractiveEnv, "push", lease, remote, ":"+ref)
		if err == nil && result != nil && result.ExitCode == 0 {
			return true
		}
		heads, lsErr := c.executor.RunWithEnv(ctx, repoPath, nonInteractiveEnv, "ls-remote", "--heads", remote, b.name)
		if lsErr != nil || heads == nil || heads.ExitCode != 0 || strings.TrimSpace(heads.Stdout) != "" {
			return false
		}
		return true
	}

	deleteArgs := []string{"branch", "-d", b.name}
	if b.reason == "stale" || b.reason == "gone" {
		deleteArgs = []string{"branch", "-D", b.name}
	}
	result, err := c.executor.Run(ctx, repoPath, deleteArgs...)
	return err == nil && result.ExitCode == 0
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
	reason   string // "merged", "stale", "gone", "superseded"
	location string // "local", "remote"
	sha      string // full classified tip; required to lease a remote delete
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
