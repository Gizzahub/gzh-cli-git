// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/identity"
)

// ForeignWorkMode decides what happens to a force push that would discard
// commits another machine or agent made.
type ForeignWorkMode string

// ForeignWorkMode values.
const (
	// ForeignWorkBlock refuses the push. It is the default.
	ForeignWorkBlock ForeignWorkMode = "block"
	// ForeignWorkAllow permits it, leaving --force-with-lease as the only guard.
	ForeignWorkAllow ForeignWorkMode = "allow"
)

// ValidateForeignWorkMode resolves a configured or flag-supplied mode. An empty
// value is the default rather than an error, so callers can pass an unset flag
// straight through.
func ValidateForeignWorkMode(value string) (ForeignWorkMode, error) {
	switch ForeignWorkMode(value) {
	case "":
		return ForeignWorkBlock, nil
	case ForeignWorkBlock:
		return ForeignWorkBlock, nil
	case ForeignWorkAllow:
		return ForeignWorkAllow, nil
	default:
		return "", fmt.Errorf("invalid foreign work mode %q: want block or allow", value)
	}
}

// ForeignCommit is a commit on the remote branch that a force push would throw
// away, signed by a machine or agent other than this one.
type ForeignCommit struct {
	Hash     string            `json:"hash"`
	Subject  string            `json:"subject"`
	Identity identity.Identity `json:"identity"`
}

// String renders the commit for an error message.
func (c ForeignCommit) String() string {
	return fmt.Sprintf("%s %s (%s)", shortHash(c.Hash), c.Subject, c.Identity.Name())
}

// findForeignCommits lists the commits reachable from remoteRef but not from
// localRef whose trailers name a different writer than mine.
//
// It answers a question --force-with-lease cannot. A lease compares the remote
// against the ref this machine last fetched, so it protects only until a fetch —
// and a multi-device workflow fetches on arrival. After that the lease is
// satisfied and a force push silently drops whatever the other machine wrote.
//
// Only signed commits can be attributed. A commit made by hand elsewhere has no
// trailer, so it reads as unknown and is not reported: the gate finds real
// conflicts in a workflow that checkpoints through this tool, and finds nothing
// in one that does not.
func findForeignCommits(
	ctx context.Context,
	executor *gitcmd.Executor,
	repoPath, localRef, remoteRef string,
	mine identity.Identity,
) ([]ForeignCommit, error) {
	if !mine.Known() || localRef == "" || remoteRef == "" {
		return nil, nil
	}

	// %x1f separates the fields of a record, %x1e the records, so a commit
	// message containing blank lines stays in one piece.
	result, err := executor.Run(ctx, repoPath, "log",
		"--format=%H%x1f%s%x1f%B%x1e", localRef+".."+remoteRef)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s..%s: %w", localRef, remoteRef, err)
	}
	if result.ExitCode != 0 {
		// A ref that does not resolve is not a conflict — the remote branch may
		// simply not exist yet, which the push itself handles.
		return nil, nil
	}

	var foreign []ForeignCommit
	for record := range strings.SplitSeq(result.Stdout, "\x1e") {
		record = strings.TrimLeft(record, "\n")
		if record == "" {
			continue
		}

		fields := strings.SplitN(record, "\x1f", 3)
		if len(fields) < 3 {
			continue
		}

		author := identity.FromMessage(fields[2])
		if !mine.DiffersFrom(author) {
			continue
		}
		foreign = append(foreign, ForeignCommit{
			Hash:     fields[0],
			Subject:  fields[1],
			Identity: author,
		})
	}

	return foreign, nil
}

// IncomingForeignWork lists the commits the upstream branch has that the local
// branch does not, written under a different device or agent than mine.
//
// Nothing here is unsafe on its own — a rebase replays local commits over them
// and loses nothing. It answers a different question: whether someone else is
// working on this branch, which is when the branch should be split rather than
// shared.
func (c *client) IncomingForeignWork(ctx context.Context, repoPath string, mine identity.Identity) ([]ForeignCommit, error) {
	repo, err := c.Open(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	info, err := c.GetInfo(ctx, repo)
	if err != nil {
		return nil, err
	}

	return findForeignCommits(ctx, c.executor, repoPath, info.Branch, info.Upstream, mine)
}

// checkForeignWork reports the commits a force push would discard that another
// writer made. It returns nothing unless the push actually forces.
func (c *client) checkForeignWork(
	ctx context.Context,
	repoPath string,
	info *Info,
	opts BulkPushOptions,
) ([]ForeignCommit, error) {
	if opts.Policy.foreignWorkMode() == ForeignWorkAllow {
		return nil, nil
	}

	localRef, remoteRef, forces := foreignWorkRefs(info, opts)
	if !forces {
		return nil, nil
	}

	return findForeignCommits(ctx, c.executor, repoPath, localRef, remoteRef, opts.Identity)
}

// foreignWorkRefs works out what a force push would overwrite: the local ref
// being sent, the remote-tracking ref it would replace, and whether the push
// forces at all.
//
// A branch with no upstream and no refspec has no remote counterpart to
// discard, so there is nothing to check.
func foreignWorkRefs(info *Info, opts BulkPushOptions) (localRef, remoteRef string, forces bool) {
	if opts.Refspec == "" {
		return info.Branch, info.Upstream, opts.Force
	}

	parsed, err := ValidateRefspec(opts.Refspec)
	if err != nil {
		return "", "", false
	}

	remote := "origin"
	if len(opts.Remotes) > 0 {
		remote = opts.Remotes[0]
	}

	return parsed.GetSourceBranch(),
		remote + "/" + parsed.GetDestinationBranch(),
		opts.Force || parsed.Force
}

// describeForeignWork summarizes the commits at stake, naming a couple of them
// so the refusal points at something a person can go look at.
func describeForeignWork(foreign []ForeignCommit) string {
	const shown = 2

	var b strings.Builder
	fmt.Fprintf(&b, "force push would discard %d commit(s) from another machine or agent: ", len(foreign))

	for i, commit := range foreign[:min(shown, len(foreign))] {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(commit.String())
	}
	if len(foreign) > shown {
		fmt.Fprintf(&b, ", and %d more", len(foreign)-shown)
	}

	return b.String()
}

// shortHash abbreviates a commit hash for display.
func shortHash(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}
