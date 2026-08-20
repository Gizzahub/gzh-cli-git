// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"
	"strings"
)

const defaultRemoteName = "origin"

// BotRemoteBranches partitions origin remote-tracking refs with bot prefixes
// into those whose tips are ancestors of base vs those that are not.
// Names are returned without the remote prefix. Skips origin/HEAD and
// IsProtected names. Empty base returns nil, nil, nil.
func (c *client) BotRemoteBranches(ctx context.Context, repo *Repository, base string) (merged, pending []string, err error) {
	if repo == nil {
		return nil, nil, fmt.Errorf("repository cannot be nil")
	}
	base = strings.TrimSpace(base)
	if base == "" {
		return nil, nil, nil
	}

	names, err := c.listRemoteTrackingNames(ctx, repo.Path, defaultRemoteName)
	if err != nil {
		return nil, nil, err
	}

	for _, name := range names {
		if IsProtected(name) || !IsBotBranch(name) {
			continue
		}
		tip := defaultRemoteName + "/" + name
		if c.isRefAncestor(ctx, repo.Path, tip, base) {
			merged = append(merged, name)
		} else {
			pending = append(pending, name)
		}
	}
	return merged, pending, nil
}

// listRemoteTrackingNames returns branch names under refs/remotes/<remote>/,
// without the remote prefix. origin/HEAD is skipped.
func (c *client) listRemoteTrackingNames(ctx context.Context, repoPath, remote string) ([]string, error) {
	if remote == "" {
		remote = defaultRemoteName
	}
	output, err := c.executor.RunOutput(ctx, repoPath, "for-each-ref", "--format=%(refname:short)", "refs/remotes/"+remote+"/")
	if err != nil {
		return nil, fmt.Errorf("failed to list remote-tracking branches: %w", err)
	}

	remotePrefix := remote + "/"
	var names []string
	for _, line := range strings.Split(output, "\n") {
		short := strings.TrimSpace(line)
		if short == "" {
			continue
		}
		name, ok := strings.CutPrefix(short, remotePrefix)
		if !ok || name == "" || name == "HEAD" {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

// isRefAncestor reports whether tip is an ancestor of base. A missing ref or
// a failed probe is treated as "not an ancestor" — the same reading
// MergedBranches uses for merge-base --is-ancestor exit 1.
func (c *client) isRefAncestor(ctx context.Context, repoPath, tip, base string) bool {
	result, err := c.executor.Run(ctx, repoPath, "merge-base", "--is-ancestor", tip, base)
	if err != nil {
		return false
	}
	return result.ExitCode == 0
}
