// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"fmt"
	"strings"
)

// porcelainRecord is one entry of `git status --porcelain -z`.
//
// Code is kept as the raw two-character XY pair rather than a normalized single
// letter, because the two sides answer different questions: X is what the index
// holds against HEAD, Y is what the working tree holds against the index. A
// caller that only needs "what happened to this path" can collapse the pair, but
// one that has to separate staged from unstaged changes cannot recover it once
// collapsed — which is why ChangeSet cannot serve as the shared representation
// and this lower-level record exists instead.
type porcelainRecord struct {
	Code    string
	Path    string
	OldPath string
}

// parsePorcelainZ splits `git status --porcelain -z` output into records.
//
// This is the single place in the package that knows the porcelain v1 wire
// format. Three properties make -z mandatory rather than a preference:
//
//   - Without it, git C-quotes any path containing a space or (under
//     core.quotePath) a non-ASCII byte, so the string emitted names no real file.
//     -z disables quoting entirely.
//   - Records are NUL-terminated, so a path may itself contain a newline without
//     splitting one entry into two.
//   - Because records are not newline-delimited, nothing may be trimmed. The
//     leading space in " M file" is the load-bearing distinction between "index
//     unchanged" and "index modified"; trimming it reclassifies an unstaged edit
//     as a staged one and shifts the path by one byte. Executor.RunOutput trims
//     its result, so callers must use runGit here rather than RunOutput.
//
// Pair -uall with it at the call site to stop git from collapsing an untracked
// directory into a single `dir/` entry, which would report N new files as one.
func parsePorcelainZ(stdout string) []porcelainRecord {
	records := strings.Split(stdout, "\x00")
	out := make([]porcelainRecord, 0, len(records))

	for i := 0; i < len(records); i++ {
		// "XY PATH": two status characters, a space, then at least one byte of
		// path. The trailing NUL leaves an empty final record that lands here too.
		if len(records[i]) < 4 {
			continue
		}

		rec := porcelainRecord{Code: records[i][:2], Path: records[i][3:]}

		// -z drops the " -> " separator and reverses the field order: the
		// destination stays in this record and the source becomes the next one.
		if isRenameOrCopyCode(rec.Code) {
			if i+1 < len(records) {
				i++
				rec.OldPath = records[i]
			}
		}

		out = append(out, rec)
	}

	return out
}

// isRenameOrCopyCode reports whether an entry is followed by a source-path record.
//
// The R/C letter can sit on either side, so keying on X alone is not enough:
//
//	R  / RM / RD   rename staged in the index (the familiar case)
//	 R / _C        rename or copy detected in the working tree — git emits this
//	               when the destination is intent-to-added while the deletion of
//	               the source is still unstaged (`mv a b && git add -N b`)
//
// Missing the second form does not merely lose OldPath: the unconsumed source
// path stays in the record stream and is re-read as a status line, so its first
// two bytes become an XY code. For a source named handler.go that is "ha", which
// fails applyStatusCode and takes the whole status read down with it.
//
// git emits exactly one source record per entry, so one lookahead covers all
// forms, including RM and RD where both columns are set.
func isRenameOrCopyCode(code string) bool {
	return code[0] == 'R' || code[0] == 'C' || code[1] == 'R' || code[1] == 'C'
}

// statusFromRecords projects porcelain records onto the public Status view.
func statusFromRecords(records []porcelainRecord) (*Status, error) {
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
		case isUnmergedCode(rec.Code):
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
func applyStatusCode(status *Status, rec porcelainRecord) error {
	index, worktree := rune(rec.Code[0]), rune(rec.Code[1])

	switch index {
	case 'M', 'A', 'C', 'T':
		status.StagedFiles = append(status.StagedFiles, rec.Path)
	case 'D':
		status.StagedFiles = append(status.StagedFiles, rec.Path)
		status.DeletedFiles = append(status.DeletedFiles, rec.Path)
	case 'R':
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
