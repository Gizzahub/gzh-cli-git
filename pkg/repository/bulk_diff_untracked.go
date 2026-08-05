// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Reasons recorded in OmittedFile.Reason.
const (
	omitReasonNotRegular = "not-regular-file"
	omitReasonTooLarge   = "too-large"
	omitReasonReadError  = "read-error"
)

// File modes as git records them in "new file mode" lines.
const (
	gitModeFile       = "100644"
	gitModeExecutable = "100755"
	gitModeSymlink    = "120000"
)

// binarySniffLen mirrors git's own heuristic window: a blob is treated as binary
// if a NUL byte appears within the first 8000 bytes.
const binarySniffLen = 8000

// errNotRegular is returned when a path stops being a regular file between the
// Lstat check and the read.
var errNotRegular = errors.New("not a regular file")

// appendUntrackedDiffs appends synthetic "new file" hunks for untracked files to
// result.DiffContent, and records every file it could not include in
// result.OmittedFiles.
//
// The previous implementation called os.ReadFile in a loop, which
// (a) dereferenced symlinks and inlined content from outside the repository,
// (b) failed with EISDIR on collapsed directory entries and dropped them via a
// bare continue, and (c) loaded the entire file into memory before comparing
// against MaxDiffSize. Each file is now size-checked against the remaining
// budget first and read through a limited reader.
func (c *client) appendUntrackedDiffs(repoPath string, result *RepositoryDiffResult, opts BulkDiffOptions) {
	var body strings.Builder

	omit := func(path, reason string) {
		result.OmittedFiles = append(result.OmittedFiles, OmittedFile{Path: path, Reason: reason})
	}

	for _, file := range result.UntrackedFiles {
		remaining := opts.MaxDiffSize - (len(result.DiffContent) + body.Len())
		if remaining <= 0 {
			result.Truncated = true
			omit(file, omitReasonTooLarge)
			continue
		}

		abs := filepath.Join(repoPath, file)

		info, err := os.Lstat(abs)
		if err != nil {
			omit(file, omitReasonReadError)
			continue
		}

		// Symlinks are rendered the way git does: mode 120000 with the link
		// target as the single line of content. The target's *contents* are
		// never read, which is what previously leaked files such as /etc/hosts
		// or ~/.aws/credentials into diff output and LLM prompts.
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(abs)
			if err != nil {
				omit(file, omitReasonReadError)
				continue
			}
			body.WriteString(newFileHunk(file, gitModeSymlink, []string{target}, true))
			continue
		}

		if !info.Mode().IsRegular() {
			omit(file, omitReasonNotRegular)
			continue
		}

		if info.Size() > int64(remaining) {
			result.Truncated = true
			omit(file, omitReasonTooLarge)
			continue
		}

		content, err := readRegularFile(abs, int64(remaining))
		if err != nil {
			omit(file, omitReasonReadError)
			continue
		}

		mode := gitModeFile
		if info.Mode()&0o111 != 0 {
			mode = gitModeExecutable
		}

		if isBinaryContent(content) {
			body.WriteString(binaryFileHunk(file, mode))
			continue
		}

		lines, endsWithNewline := splitDiffLines(content)
		body.WriteString(newFileHunk(file, mode, lines, endsWithNewline))
	}

	result.DiffContent += body.String()
}

// readRegularFile reads at most limit bytes from path.
//
// The file type is re-checked through the open descriptor rather than trusting
// the earlier Lstat: that closes the window where a regular file is swapped for
// a symlink between the two calls.
func readRegularFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // path comes from `git ls-files` inside the scanned repository
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only handle

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errNotRegular
	}

	return io.ReadAll(io.LimitReader(f, limit))
}

// isBinaryContent applies git's NUL-byte heuristic.
func isBinaryContent(content []byte) bool {
	if len(content) > binarySniffLen {
		content = content[:binarySniffLen]
	}

	return bytes.IndexByte(content, 0) >= 0
}

// splitDiffLines splits file content into diff body lines and reports whether
// the file ended with a newline. A file that does not gets git's
// "\ No newline at end of file" marker.
func splitDiffLines(content []byte) (lines []string, endsWithNewline bool) {
	if len(content) == 0 {
		return nil, true
	}

	text := string(content)
	if trimmed, ok := strings.CutSuffix(text, "\n"); ok {
		return strings.Split(trimmed, "\n"), true
	}

	return strings.Split(text, "\n"), false
}

// newFileHunk renders a synthetic unified-diff hunk for an added file, matching
// the shape git emits for a newly tracked path.
func newFileHunk(path, mode string, lines []string, endsWithNewline bool) string {
	var b strings.Builder

	fmt.Fprintf(&b, "\ndiff --git a/%s b/%s\nnew file mode %s\n--- /dev/null\n+++ b/%s\n", path, path, mode, path)

	if len(lines) == 0 {
		b.WriteString("@@ -0,0 +0,0 @@\n")
		return b.String()
	}

	fmt.Fprintf(&b, "@@ -0,0 +1,%d @@\n", len(lines))
	for _, line := range lines {
		b.WriteString("+")
		b.WriteString(line)
		b.WriteString("\n")
	}
	if !endsWithNewline {
		b.WriteString("\\ No newline at end of file\n")
	}

	return b.String()
}

// binaryFileHunk renders the placeholder git uses for binary additions.
func binaryFileHunk(path, mode string) string {
	return fmt.Sprintf("\ndiff --git a/%s b/%s\nnew file mode %s\nBinary files /dev/null and b/%s differ\n",
		path, path, mode, path)
}
