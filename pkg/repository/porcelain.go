// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"fmt"

	"github.com/gizzahub/gzh-cli-gitforge/internal/porcelain"
)

// parseStatusZ turns raw `git status --porcelain -z` output into a Status.
//
// The two stages behind it are separate because collectChangeSet needs the
// records themselves — it keeps the XY pair per path, which Status collapses —
// but every caller that only wants a Status runs both stages back to back and
// wraps either failure identically. Composing them here keeps that pairing in
// one place rather than repeating it, correctly, at each call site.
func parseStatusZ(stdout string) (*Status, error) {
	records, err := porcelain.Parse(stdout)
	if err != nil {
		return nil, err
	}

	return statusFromRecords(records)
}

// statusFromRecords projects porcelain records onto the public Status view.
func statusFromRecords(records []porcelain.Record) (*Status, error) {
	status := &Status{
		IsClean:        true,
		ModifiedFiles:  []string{},
		StagedFiles:    []string{},
		UntrackedFiles: []string{},
		ConflictFiles:  []string{},
		DeletedFiles:   []string{},
		RenamedFiles:   []RenamedFile{},
	}

	for _, rec := range records {
		// Ignored entries only appear under --ignored, and they are not changes.
		if rec.Code == "!!" {
			continue
		}

		status.IsClean = false

		switch {
		case porcelain.IsUnmerged(rec.Code):
			// An unmerged path is neither staged nor merely modified: its index
			// holds three stages instead of one content. Filing it in those
			// buckets as well is what let AA and DD miss conflict detection
			// entirely while UU was recorded in ConflictFiles twice. For the same
			// reason it raises TrackedChangedCount but neither side count.
			status.ConflictFiles = append(status.ConflictFiles, rec.Path)
			status.TrackedChangedCount++
		case rec.Code == "??":
			status.UntrackedFiles = append(status.UntrackedFiles, rec.Path)
		default:
			if err := applyStatusCode(status, rec); err != nil {
				return nil, err
			}

			status.TrackedChangedCount++

			if rec.Code[0] != ' ' {
				status.StagedCount++
			}

			if rec.Code[1] != ' ' {
				status.UnstagedCount++
			}
		}
	}

	return status, nil
}

// applyStatusCode files a non-conflicted, tracked record into Status by side.
//
// Unmerged and untracked codes are handled by the caller and never reach here,
// so both switches can treat X and Y as independent one-sided verdicts.
func applyStatusCode(status *Status, rec porcelain.Record) error {
	index, worktree := rune(rec.Code[0]), rune(rec.Code[1])

	switch index {
	case 'M', 'A', 'T':
		status.StagedFiles = append(status.StagedFiles, rec.Path)
	case 'D':
		status.StagedFiles = append(status.StagedFiles, rec.Path)
		status.DeletedFiles = append(status.DeletedFiles, rec.Path)
	case 'R', 'C':
		// Index-side rename or copy: both ship a source path in the next -z
		// record (already paired into rec.OldPath). Copies used to land only in
		// StagedFiles and drop OldPath, so RenamedFiles could not recover them.
		status.StagedFiles = append(status.StagedFiles, rec.Path)
		status.RenamedFiles = append(status.RenamedFiles, RenamedFile{
			OldPath: rec.OldPath,
			NewPath: rec.Path,
		})
	case ' ':
		// Index agrees with HEAD; the worktree switch below carries the change.
	default:
		return fmt.Errorf("unknown index status code: %c", index)
	}

	switch worktree {
	case 'M', 'T':
		status.ModifiedFiles = append(status.ModifiedFiles, rec.Path)
	case 'D':
		status.DeletedFiles = append(status.DeletedFiles, rec.Path)
	case 'R', 'C':
		// A rename or copy seen on the worktree side, not the index side. Mirror
		// the index branch above, which files a rename under both its side bucket
		// and RenamedFiles, so the pair stays recoverable either way.
		status.ModifiedFiles = append(status.ModifiedFiles, rec.Path)
		status.RenamedFiles = append(status.RenamedFiles, RenamedFile{
			OldPath: rec.OldPath,
			NewPath: rec.Path,
		})
	case 'A':
		// Intent-to-add (`git add -N`, git >= 2.16): the index carries the path
		// with empty content, so every byte of the file is an unstaged addition.
		// Treating this as a no-op let the path raise TrackedChangedCount while
		// appearing in no list, so a caller that cross-checked the count against
		// the lists saw them disagree.
		status.ModifiedFiles = append(status.ModifiedFiles, rec.Path)
	case ' ':
		// Index-only change; the switch above already carried it.
	default:
		return fmt.Errorf("unknown worktree status code: %c", worktree)
	}

	return nil
}
