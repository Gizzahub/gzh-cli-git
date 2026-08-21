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
// into merged (tip is an ancestor of base), superseded (not an ancestor, but
// base already satisfies the bot's version target), and pending (not an
// ancestor and still newer or not comparable). Names are returned without
// the remote prefix. Skips origin/HEAD and IsProtected names. Empty base
// returns nil, nil, nil, nil.
func (c *client) BotRemoteBranches(ctx context.Context, repo *Repository, base string) (merged, superseded, pending []string, err error) {
	if repo == nil {
		return nil, nil, nil, fmt.Errorf("repository cannot be nil")
	}
	base = strings.TrimSpace(base)
	if base == "" {
		return nil, nil, nil, nil
	}

	names, err := c.listRemoteTrackingNames(ctx, repo.Path, defaultRemoteName)
	if err != nil {
		return nil, nil, nil, err
	}

	var files botVersionFiles
	baseLoaded := false
	loadBase := func() {
		if baseLoaded {
			return
		}
		files.baseGoMod = c.fileAtRef(ctx, repo.Path, base, "go.mod")
		files.baseWorkflows = c.workflowContentsAt(ctx, repo.Path, base)
		baseLoaded = true
	}

	for _, name := range names {
		if IsProtected(name) || !IsBotBranch(name) {
			continue
		}
		tip := defaultRemoteName + "/" + name
		if c.isRefAncestor(ctx, repo.Path, tip, base) {
			merged = append(merged, name)
			continue
		}
		if !botKindComparable(name) {
			pending = append(pending, name)
			continue
		}
		loadBase()
		files.botGoMod = ""
		if strings.HasPrefix(botMatchName(name), dependabotGoModulesPrefix) {
			files.botGoMod = c.fileAtRef(ctx, repo.Path, tip, "go.mod")
		}
		if botTargetSuperseded(name, files) {
			superseded = append(superseded, name)
		} else {
			pending = append(pending, name)
		}
	}
	return merged, superseded, pending, nil
}

func (c *client) fileAtRef(ctx context.Context, repoPath, ref, file string) string {
	out, err := c.executor.RunOutput(ctx, repoPath, "show", ref+":"+file)
	if err != nil {
		return ""
	}
	return out
}

func (c *client) workflowContentsAt(ctx context.Context, repoPath, ref string) []string {
	names, err := c.executor.RunLines(ctx, repoPath, "ls-tree", "-r", "--name-only", ref, "--", ".github/workflows")
	if err != nil {
		return nil
	}
	var out []string
	for _, name := range names {
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		body := c.fileAtRef(ctx, repoPath, ref, name)
		if body != "" {
			out = append(out, body)
		}
	}
	return out
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
