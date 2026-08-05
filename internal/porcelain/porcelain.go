// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

// Package porcelain parses `git status --porcelain -z` output.
//
// It exists because four packages need to read the same wire format and none of
// them may depend on the others: pkg/repository owns the Status projection,
// pkg/branch and pkg/doctor hold only a *gitcmd.Executor, and pkg/reposync is
// under a standing rule not to call pkg/repository from its executor. Every
// previous attempt to satisfy that produced another hand-rolled line splitter,
// and each one rediscovered the same defects — collapsed untracked directories,
// C-quoted paths that name no real file, renames flattened into a single
// nonexistent path.
//
// The package deliberately stops at the record level. Projecting records onto a
// domain type is the caller's business, and the two projections that exist
// disagree: Status folds the XY pair into a union of file lists, while a conflict
// check needs the pair intact. Sharing the projection would force one of them to
// lose information; sharing the parse costs nothing.
package porcelain

import (
	"fmt"
	"strings"
)

// Record is one entry of `git status --porcelain -z`.
//
// Code is kept as the raw two-character XY pair rather than a normalized single
// letter, because the two sides answer different questions: X is what the index
// holds against HEAD, Y is what the working tree holds against the index. A
// caller that only needs "what happened to this path" can collapse the pair, but
// one that has to separate staged from unstaged changes cannot recover it once
// collapsed.
type Record struct {
	Code    string
	Path    string
	OldPath string
}

// Parse splits `git status --porcelain -z` output into records.
//
// Three properties make -z mandatory rather than a preference:
//
//   - Without it, git C-quotes any path containing a space or (under
//     core.quotePath) a non-ASCII byte, so the string emitted names no real file.
//     -z disables quoting entirely.
//   - Records are NUL-terminated, so a path may itself contain a newline without
//     splitting one entry into two.
//   - Because records are not newline-delimited, nothing may be trimmed. The
//     leading space in " M file" is the load-bearing distinction between "index
//     unchanged" and "index modified"; trimming it reclassifies an unstaged edit
//     as a staged one and shifts the path by one byte. gitcmd.Executor.RunOutput
//     trims its result, so callers must read stdout untrimmed — Run, not
//     RunOutput.
//
// Pair -uall with it at the call site to stop git from collapsing an untracked
// directory into a single `dir/` entry, which would report N new files as one.
//
// Anything that is not a well-formed record is an error rather than a skip. The
// only input this function may discard is the empty string, which -z produces by
// construction: it terminates every record with a NUL, so the final split always
// yields one trailing empty field. A non-empty record shorter than "XY P" is not
// something git emits, so it means either that the format is not what this parser
// believes it to be or that stdout was truncated — and that is the one signal
// worth keeping, not the one worth swallowing.
func Parse(stdout string) ([]Record, error) {
	records := strings.Split(stdout, "\x00")
	out := make([]Record, 0, len(records))

	for i := 0; i < len(records); i++ {
		if records[i] == "" {
			continue
		}

		// "XY PATH": two status characters, a space, then at least one byte of path.
		if len(records[i]) < 4 {
			return nil, fmt.Errorf("malformed porcelain record %q: want at least 4 bytes (XY, space, path)", records[i])
		}

		rec := Record{Code: records[i][:2], Path: records[i][3:]}

		// -z drops the " -> " separator and reverses the field order: the
		// destination stays in this record and the source becomes the next one.
		if isRenameOrCopy(rec.Code) {
			// A missing source record is malformed, not an empty OldPath. Passing
			// it through would report a rename with no origin, and the caller has
			// no way to tell that from a rename genuinely lacking one — which git
			// never emits.
			//
			// Checking i+1 against the length is not enough on its own: the split
			// of a well-formed payload always ends in an empty element, so a
			// truncated rename entry — the destination present, its source cut off
			// — reads as "a next record exists" and adopts "" as the source. That
			// is the exact silent pass this guard is here to stop, so the source
			// must be present *and* non-empty.
			if i+1 >= len(records) || records[i+1] == "" {
				return nil, fmt.Errorf("porcelain record %q has status %q but no source path record follows", rec.Path, rec.Code)
			}

			i++
			rec.OldPath = records[i]
		}

		out = append(out, rec)
	}

	return out, nil
}

// isRenameOrCopy reports whether an entry is followed by a source-path record.
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
// no status-code switch accepts, taking the whole status read down with it.
//
// git emits exactly one source record per entry, so one lookahead covers all
// forms, including RM and RD where both columns are set.
func isRenameOrCopy(code string) bool {
	return code[0] == 'R' || code[0] == 'C' || code[1] == 'R' || code[1] == 'C'
}

// IsUnmerged reports whether an XY code marks a path with unresolved conflicts.
//
// There are seven such codes and only five contain a U:
//
//	DD  both deleted      AU  added by us     UD  deleted by them
//	UA  added by them     DU  deleted by us   AA  both added
//	UU  both modified
//
// Testing for U alone is the recurring bug — it silently passes AA and DD, the
// two shapes a conflicted rename or a delete/delete merge produces, so a
// repository mid-conflict reads as merely dirty.
//
// Callers may hold a code from an arbitrary source, so a short string is
// answered false rather than indexed.
func IsUnmerged(code string) bool {
	if len(code) < 2 {
		return false
	}

	x, y := code[0], code[1]
	if x == 'U' || y == 'U' {
		return true
	}

	return (x == 'A' && y == 'A') || (x == 'D' && y == 'D')
}
