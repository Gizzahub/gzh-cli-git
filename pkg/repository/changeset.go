// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ChangeScope names the two-endpoint comparison that defines a change set.
//
// Before this type existed, `diff` and `commit` disagreed about what "changed"
// meant and neither said so: diff compared worktree against the index, commit
// hand-assembled a union of both sides, and `executeCommit` then ran
// `git add -A`, whose actual scope (HEAD against the worktree, untracked
// included) was computed nowhere at all.
type ChangeScope string

const (
	// ScopeHead compares HEAD against the working tree including untracked
	// files. This is the scope `git add -A && git commit` actually records, and
	// therefore the default for both diff and commit.
	ScopeHead ChangeScope = "head"

	// ScopeStagedOnly compares HEAD against the index (`git diff --cached`).
	ScopeStagedOnly ChangeScope = "staged"

	// ScopeWorktreeOnly compares the index against the working tree
	// (`git diff`). This was diff's previous, undeclared behavior, which is why
	// a fully staged repository reported files but an empty diff body.
	ScopeWorktreeOnly ChangeScope = "worktree"
)

// ChangeEntry is one path in a change set.
type ChangeEntry struct {
	// Path is the repository-relative path as it exists on disk: never quoted,
	// never a collapsed directory.
	Path string

	// OldPath is the source path of a rename or copy, empty otherwise.
	OldPath string

	// Status is the normalized single-letter code (M, A, D, R, C, ?).
	Status string

	// Staged means the index differs from HEAD for this path.
	Staged bool

	// Untracked means the path is not in the index at all.
	Untracked bool

	// Conflicted means the path has unmerged index stages.
	Conflicted bool
}

// ChangeSet is the shared answer to "what changed in this repository", used by
// both BulkDiff and BulkCommit so the two can no longer disagree.
type ChangeSet struct {
	Entries []ChangeEntry

	// TrackedCount counts entries git already knows about; UntrackedCount counts
	// the rest. Their sum is len(Entries).
	TrackedCount   int
	UntrackedCount int

	// StagedCount and ConflictCount are subsets of Entries, not of each other.
	StagedCount   int
	ConflictCount int

	// Additions and Deletions are the line counts a commit of this change set
	// would record. Tracked lines come from `git diff --numstat`; untracked files
	// are counted separately, since git cannot diff what is not in the index.
	Additions int
	Deletions int

	// DiffFileCount is how many files actually differ from the scope's base.
	// It is not len(Entries): a path can be listed by status yet be identical to
	// HEAD (index and worktree both changed, canceling out), in which case there
	// is nothing to commit even though the repository looks dirty.
	DiffFileCount int

	Scope ChangeScope
}

// collectChangeSet is the single porcelain parser for this package.
//
// It runs `git status --porcelain -z -uall`, which differs from the plain
// `--porcelain` the two inline parsers used in three ways that all matter:
//
//   - -z terminates records with NUL instead of newline and, as a consequence,
//     disables C-quoting. Paths containing spaces or (under core.quotePath)
//     non-ASCII characters come back as the bytes actually on disk rather than
//     as an escaped display string that matches no real file.
//   - -uall lists untracked files individually instead of collapsing them to
//     `dir/`, so a preview count matches what a commit would record.
//   - Because records are not newline-delimited, there is no reason to
//     TrimSpace them, which restores the leading-space distinction between
//     index status (X) and worktree status (Y).
func (c *client) collectChangeSet(ctx context.Context, repoPath string, scope ChangeScope) (*ChangeSet, error) {
	stdout, err := c.runGit(ctx, repoPath, "status", "--porcelain", "-z", "-uall")
	if err != nil {
		return nil, fmt.Errorf("failed to read status: %w", err)
	}

	records, err := parsePorcelainZ(stdout)
	if err != nil {
		return nil, fmt.Errorf("failed to read status: %w", err)
	}

	cs := &ChangeSet{Scope: scope, Entries: make([]ChangeEntry, 0, 8)}

	for _, rec := range records {
		code := rec.Code

		entry := ChangeEntry{
			Path:       rec.Path,
			OldPath:    rec.OldPath,
			Status:     parseGitStatus(code),
			Untracked:  code == "??",
			Conflicted: isUnmergedCode(code),
		}
		// A conflicted path has index stages rather than a staged change;
		// counting it as staged would let it slip past a staged-only filter.
		entry.Staged = !entry.Untracked && !entry.Conflicted && code[0] != ' '

		if !entry.matchesScope(scope, code) {
			continue
		}

		cs.Entries = append(cs.Entries, entry)
		switch {
		case entry.Untracked:
			cs.UntrackedCount++
		default:
			cs.TrackedCount++
		}
		if entry.Staged {
			cs.StagedCount++
		}
		if entry.Conflicted {
			cs.ConflictCount++
		}
	}

	cs.Additions, cs.Deletions, cs.DiffFileCount = c.collectDiffStats(ctx, repoPath, scope)
	addUntrackedAdditions(repoPath, cs)

	return cs, nil
}

// matchesScope reports whether an entry belongs to the requested comparison.
func (e ChangeEntry) matchesScope(scope ChangeScope, code string) bool {
	switch scope {
	case ScopeStagedOnly:
		return e.Staged
	case ScopeWorktreeOnly:
		// Y == ' ' means the worktree agrees with the index. Untracked entries
		// carry "??", so they are included here by construction.
		return code[1] != ' '
	case ScopeHead:
		return true
	default:
		return true
	}
}

// scopeDiffArgs returns the trailing revision arguments for `git diff`.
func scopeDiffArgs(scope ChangeScope) []string {
	switch scope {
	case ScopeStagedOnly:
		return []string{"--cached"}
	case ScopeWorktreeOnly:
		return nil
	case ScopeHead:
		return []string{"HEAD"}
	default:
		return []string{"HEAD"}
	}
}

// runScopedDiff runs `git diff <flags...> <scope revisions>`.
//
// On an unborn HEAD (a repository with no commits) `git diff HEAD` fails, since
// there is nothing to compare against. Everything staged there is by definition
// an addition, so --cached is the correct equivalent.
func (c *client) runScopedDiff(ctx context.Context, repoPath string, scope ChangeScope, flags ...string) (string, error) {
	args := append([]string{"diff"}, flags...)

	stdout, err := c.runGit(ctx, repoPath, concat(args, scopeDiffArgs(scope))...)
	if err == nil {
		return stdout, nil
	}
	if scope != ScopeHead {
		return "", err
	}

	stdout, fallbackErr := c.runGit(ctx, repoPath, concat(args, []string{"--cached"})...)
	if fallbackErr != nil {
		return "", err
	}

	return stdout, nil
}

// runGit collapses gitcmd's two-part failure signal into a single error.
//
// Executor.Run reports a failed git command through Result.ExitCode and returns
// a nil error, so checking only err turns "git failed" into "git found nothing":
// a broken status reads as a clean repository and a broken conflict probe reads
// as a conflict-free one. Both are the wrong way to fail.
func (c *client) runGit(ctx context.Context, repoPath string, args ...string) (string, error) {
	res, err := c.executor.Run(ctx, repoPath, args...)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("git %s exited %d: %s",
			strings.Join(args, " "), res.ExitCode, strings.TrimSpace(res.Stderr))
	}

	return res.Stdout, nil
}

// concat joins argument slices without aliasing either one. Appending directly
// to a shared prefix would let the retry in runScopedDiff overwrite the first
// attempt's backing array.
func concat(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	out = append(out, a...)

	return append(out, b...)
}

// collectDiffStats returns line counts and the number of differing files.
//
// `--numstat -z` is used rather than parsing `--stat` prose. The prose form is
// localized, elides file lists past a width limit, and renders counts as a
// proportional +/- bar, so the numbers recovered from it are approximations of
// an approximation. numstat emits exact per-file integers.
func (c *client) collectDiffStats(ctx context.Context, repoPath string, scope ChangeScope) (additions, deletions, files int) {
	out, err := c.runScopedDiff(ctx, repoPath, scope, "--numstat", "-z")
	if err != nil {
		return 0, 0, 0
	}

	return parseNumstat(out)
}

// parseNumstat sums `git diff --numstat -z` output and counts the files in it.
//
// Records are "<added>\t<deleted>\t<path>\0". Renames and copies instead emit
// "<added>\t<deleted>\t\0<source>\0<destination>\0" — an empty path field
// followed by two extra records that must be consumed. Binary files report "-"
// for both counts.
//
// files is counted separately from the line sums because the two answer
// different questions. A binary edit reports "-"/"-" and a mode-only change
// reports 0/0, so both net zero lines while still being real changes; only the
// file count distinguishes them from a diff that is genuinely empty.
func parseNumstat(out string) (additions, deletions, files int) {
	records := strings.Split(out, "\x00")
	for i := 0; i < len(records); i++ {
		if records[i] == "" {
			continue
		}

		fields := strings.SplitN(records[i], "\t", 3)
		if len(fields) < 3 {
			continue
		}
		if fields[2] == "" {
			i += 2
		}

		files++

		if n, err := strconv.Atoi(fields[0]); err == nil {
			additions += n
		}
		if n, err := strconv.Atoi(fields[1]); err == nil {
			deletions += n
		}
	}

	return additions, deletions, files
}

// countAddedLines returns the number of lines path contributes as insertions.
//
// Binary files count as zero, matching git: --numstat reports "-" for them and a
// commit records no insertions. Content is streamed in fixed-size chunks rather
// than buffered, because untracked trees are unbounded and a bulk run may walk
// many of them.
func countAddedLines(path string) (int, error) {
	f, err := os.Open(path) //nolint:gosec // path comes from git status inside the scanned repository
	if err != nil {
		return 0, err
	}
	defer f.Close() //nolint:errcheck // read-only handle

	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, errNotRegular
	}

	var (
		buf      = make([]byte, 32*1024)
		lines    int
		sniffed  int
		read     int
		lastByte byte
	)

	for {
		n, readErr := f.Read(buf)
		if n > 0 {
			chunk := buf[:n]

			if sniffed < binarySniffLen {
				window := chunk
				if room := binarySniffLen - sniffed; len(window) > room {
					window = window[:room]
				}
				if bytes.IndexByte(window, 0) >= 0 {
					return 0, nil
				}
				sniffed += len(window)
			}

			lines += bytes.Count(chunk, []byte{'\n'})
			lastByte = chunk[len(chunk)-1]
			read += n
		}

		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return 0, readErr
		}
	}

	// A trailing line with no newline still counts as a line.
	if read > 0 && lastByte != '\n' {
		lines++
	}

	return lines, nil
}

// addUntrackedAdditions adds untracked files' line counts to the change set.
//
// `git diff HEAD` cannot see untracked files — they are not in the index — yet
// `git add -A && git commit` records every one of their lines as an insertion.
// Leaving them out is what made a new-files-only repository preview as
// "0 insertions" and then commit twelve.
//
// Symlinks are counted as the single line git stores (the target path) without
// being opened, so the link target's size and contents stay irrelevant.
func addUntrackedAdditions(repoPath string, cs *ChangeSet) {
	for _, entry := range cs.Entries {
		if !entry.Untracked {
			continue
		}

		abs := filepath.Join(repoPath, entry.Path)

		info, err := os.Lstat(abs)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			cs.Additions++
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}

		n, err := countAddedLines(abs)
		if err != nil {
			continue
		}
		cs.Additions += n
	}
}

// isUnmergedCode reports whether a porcelain v1 "XY" status code denotes an
// unmerged (conflicted) path.
//
// Git defines exactly seven unmerged combinations:
//
//	DD  both deleted     AU  added by us      UD  deleted by them
//	UA  added by them    DU  deleted by us    AA  both added
//	UU  both modified
//
// Every one of them carries a 'U' on at least one side except AA and DD, so the
// set collapses to the two checks below.
//
// This matters because `git add -A` marks a conflicted path as *resolved*: once
// staged, the conflict markers become ordinary content and the following commit
// records them permanently. Detecting the state before staging is the only
// reliable guard.
func isUnmergedCode(code string) bool {
	if len(code) < 2 {
		return false
	}

	x, y := code[0], code[1]
	if x == 'U' || y == 'U' {
		return true
	}

	return (x == 'A' && y == 'A') || (x == 'D' && y == 'D')
}

// collectConflictedPaths returns the unmerged paths recorded in the index.
//
// `git ls-files --unmerged` is preferred over `git status --porcelain` for the
// path values: it reads index stages 1-3 directly, so the output is never
// subject to core.quotePath escaping and does not depend on how the working
// tree currently looks. Porcelain remains useful as a cheap detector (see
// isUnmergedCode), but its paths can be C-quoted.
//
// Output is one NUL-terminated record per stage:
//
//	<mode> <object> <stage>\t<path>\0
//
// A single conflicted path therefore appears up to three times and must be
// de-duplicated. Returns nil when the repository has no unmerged entries.
//
// A nil return on failure does not weaken the commit guard: this is only called
// once porcelain has already found conflicts, and the caller falls back to the
// porcelain paths when this yields nothing. Detection stays with porcelain;
// failure here costs path fidelity, not the gate.
func (c *client) collectConflictedPaths(ctx context.Context, repoPath string) []string {
	stdout, err := c.runGit(ctx, repoPath, "ls-files", "--unmerged", "-z")
	if err != nil || stdout == "" {
		return nil
	}

	seen := make(map[string]struct{})
	paths := make([]string, 0, 4)

	for record := range strings.SplitSeq(stdout, "\x00") {
		_, path, ok := strings.Cut(record, "\t")
		if !ok || path == "" {
			continue
		}
		if _, dup := seen[path]; dup {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}

	if len(paths) == 0 {
		return nil
	}

	return paths
}
