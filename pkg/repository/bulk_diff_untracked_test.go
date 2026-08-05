// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// untrackedFixture builds a repository with one commit plus a set of untracked
// entries that the old os.ReadFile loop handled incorrectly.
func untrackedFixture(t *testing.T) (rootDir, repoPath string) {
	t.Helper()

	rootDir = t.TempDir()
	repoPath = filepath.Join(rootDir, "repo")

	if err := os.MkdirAll(repoPath, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := initGitRepoWithCommit(repoPath); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	write := func(rel, content string) {
		t.Helper()
		abs := filepath.Join(repoPath, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	// Nested untracked directory: porcelain collapses this to "docs/", so the
	// old reader hit EISDIR and dropped both files without a word.
	write("docs/adr/0005-scope.md", "# ADR 0005\n")
	write("docs/adr/0006-parsing.md", "# ADR 0006\n")

	// Path with a space: porcelain C-quotes this, and nothing unquoted it.
	write("has space.txt", "spaced\n")

	// Binary content.
	write("blob.bin", "\x00\x01\x02binary\x00")

	// No trailing newline.
	write("nonewline.txt", "last line without newline")

	return rootDir, repoPath
}

// secretOutsideRepo writes a file outside the repository and returns its path.
func secretOutsideRepo(t *testing.T, rootDir string) string {
	t.Helper()

	secret := filepath.Join(rootDir, "outside-secret.txt")
	if err := os.WriteFile(secret, []byte("AWS_SECRET_ACCESS_KEY=leaked\n"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	return secret
}

func diffOne(t *testing.T, rootDir string, opts BulkDiffOptions) RepositoryDiffResult {
	t.Helper()

	opts.Directory = rootDir
	if opts.Parallel == 0 {
		opts.Parallel = 1
	}
	if opts.MaxDepth == 0 {
		opts.MaxDepth = 1
	}

	result, err := NewClient().BulkDiff(context.Background(), opts)
	if err != nil {
		t.Fatalf("BulkDiff: %v", err)
	}
	if len(result.Repositories) != 1 {
		t.Fatalf("got %d repositories, want 1", len(result.Repositories))
	}

	return result.Repositories[0]
}

// TestUntrackedSymlinkIsNotDereferenced is the regression guard for the
// information leak: os.ReadFile followed the link and inlined the target's
// contents into diff output (and from there into LLM prompts and CI artifacts).
func TestUntrackedSymlinkIsNotDereferenced(t *testing.T) {
	rootDir, repoPath := untrackedFixture(t)
	secret := secretOutsideRepo(t, rootDir)

	if err := os.Symlink(secret, filepath.Join(repoPath, "leaked-link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	repo := diffOne(t, rootDir, BulkDiffOptions{IncludeUntracked: true, MaxDiffSize: 1 << 20})

	if strings.Contains(repo.DiffContent, "AWS_SECRET_ACCESS_KEY") {
		t.Error("diff body contains the symlink target's contents")
	}
	if !strings.Contains(repo.DiffContent, "new file mode 120000") {
		t.Error("symlink was not recorded with mode 120000")
	}
	if !strings.Contains(repo.DiffContent, "+"+secret) {
		t.Errorf("symlink target path missing from diff body:\n%s", repo.DiffContent)
	}
}

// TestUntrackedDirectoryIsExpanded guards the silent no-op: porcelain reported
// "docs/" and the reader failed with EISDIR, so --include-untracked produced
// zero bytes for an entire new subtree.
func TestUntrackedDirectoryIsExpanded(t *testing.T) {
	rootDir, _ := untrackedFixture(t)

	repo := diffOne(t, rootDir, BulkDiffOptions{IncludeUntracked: true, MaxDiffSize: 1 << 20})

	for _, want := range []string{"docs/adr/0005-scope.md", "docs/adr/0006-parsing.md"} {
		if !containsPath(repo.UntrackedFiles, want) {
			t.Errorf("UntrackedFiles missing %q: %v", want, repo.UntrackedFiles)
		}
		if !strings.Contains(repo.DiffContent, "+++ b/"+want) {
			t.Errorf("diff body missing content for %q", want)
		}
	}
	if containsPath(repo.UntrackedFiles, "docs/") {
		t.Error("UntrackedFiles still contains the collapsed directory entry")
	}
	if !strings.Contains(repo.DiffContent, "# ADR 0005") {
		t.Error("nested file content missing from diff body")
	}
}

// TestUntrackedSpacedPathIncluded guards the quoted-path drop.
func TestUntrackedSpacedPathIncluded(t *testing.T) {
	rootDir, _ := untrackedFixture(t)

	repo := diffOne(t, rootDir, BulkDiffOptions{IncludeUntracked: true, MaxDiffSize: 1 << 20})

	if !containsPath(repo.UntrackedFiles, "has space.txt") {
		t.Errorf("UntrackedFiles missing unquoted spaced path: %v", repo.UntrackedFiles)
	}
	if !strings.Contains(repo.DiffContent, "+spaced") {
		t.Error("spaced-path content missing from diff body")
	}
}

// TestUntrackedBinaryNotInlined checks binaries get git's placeholder rather
// than raw bytes.
func TestUntrackedBinaryNotInlined(t *testing.T) {
	rootDir, _ := untrackedFixture(t)

	repo := diffOne(t, rootDir, BulkDiffOptions{IncludeUntracked: true, MaxDiffSize: 1 << 20})

	if !strings.Contains(repo.DiffContent, "Binary files /dev/null and b/blob.bin differ") {
		t.Errorf("binary placeholder missing:\n%s", repo.DiffContent)
	}
	if strings.Contains(repo.DiffContent, "+\x00") {
		t.Error("raw binary bytes were inlined into the diff body")
	}
}

// TestUntrackedNoNewlineMarker checks the synthetic hunk matches git's shape.
func TestUntrackedNoNewlineMarker(t *testing.T) {
	rootDir, _ := untrackedFixture(t)

	repo := diffOne(t, rootDir, BulkDiffOptions{IncludeUntracked: true, MaxDiffSize: 1 << 20})

	if !strings.Contains(repo.DiffContent, "+last line without newline\n\\ No newline at end of file") {
		t.Errorf("missing no-newline marker:\n%s", repo.DiffContent)
	}
	// The old builder split on "\n" and emitted a trailing bare "+" for every
	// file that did end with a newline.
	if strings.Contains(repo.DiffContent, "+\n\ndiff --git") {
		t.Error("spurious empty '+' line emitted at end of a file")
	}
}

// TestUntrackedOversizeFileIsNotRead is the memory guard: a file larger than
// the remaining budget must be skipped by size, never read first.
func TestUntrackedOversizeFileIsNotRead(t *testing.T) {
	rootDir, repoPath := untrackedFixture(t)

	big := strings.Repeat("x", 256*1024)
	if err := os.WriteFile(filepath.Join(repoPath, "huge.log"), []byte(big), 0o600); err != nil {
		t.Fatalf("write huge.log: %v", err)
	}

	repo := diffOne(t, rootDir, BulkDiffOptions{IncludeUntracked: true, MaxDiffSize: 4096})

	if !repo.Truncated {
		t.Error("Truncated not set despite an over-budget file")
	}
	if strings.Contains(repo.DiffContent, big[:1024]) {
		t.Error("over-budget file content was inlined")
	}
	if !hasOmission(repo.OmittedFiles, "huge.log", omitReasonTooLarge) {
		t.Errorf("huge.log not reported as too-large: %v", repo.OmittedFiles)
	}
	if len(repo.DiffContent) > 4096+4096 {
		t.Errorf("diff body is %d bytes, far beyond the %d budget", len(repo.DiffContent), 4096)
	}
}

// TestUntrackedUnreadableFileIsReported checks that no skip is silent: a file
// git enumerated but that cannot be opened must show up in OmittedFiles rather
// than vanishing through a bare continue, which is how the old loop lost files.
func TestUntrackedUnreadableFileIsReported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits do not deny access")
	}

	rootDir, repoPath := untrackedFixture(t)

	locked := filepath.Join(repoPath, "unreadable.txt")
	if err := os.WriteFile(locked, []byte("secret\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o600) })

	repo := diffOne(t, rootDir, BulkDiffOptions{IncludeUntracked: true, MaxDiffSize: 1 << 20})

	if !containsPath(repo.UntrackedFiles, "unreadable.txt") {
		t.Fatalf("git did not enumerate the file: %v", repo.UntrackedFiles)
	}
	if !hasOmission(repo.OmittedFiles, "unreadable.txt", omitReasonReadError) {
		t.Errorf("unreadable file not reported: %v", repo.OmittedFiles)
	}
	// One bad file must not abort the rest of the repository.
	if !strings.Contains(repo.DiffContent, "# ADR 0005") {
		t.Error("a single read failure suppressed the remaining files")
	}
}

// TestReadRegularFileRejectsDirectory exercises the fstat recheck performed on
// the open descriptor. The Lstat in appendUntrackedDiffs is advisory: between it
// and the open, the path can be swapped for something that is not a regular
// file. Re-checking through the descriptor closes that window, so the swap
// cannot redirect the read.
//
// Note that git's own untracked listing already filters out FIFOs, sockets and
// device nodes, so the not-regular branch is reachable only through that race —
// a directory is the one type that can be opened and then rejected without the
// test blocking on an open() that waits for a writer.
func TestReadRegularFileRejectsDirectory(t *testing.T) {
	dir := t.TempDir()

	if _, err := readRegularFile(dir, 1024); !errors.Is(err, errNotRegular) {
		t.Errorf("readRegularFile(dir) error = %v, want errNotRegular", err)
	}
}

// TestReadRegularFileRespectsLimit confirms the read is bounded, so a file that
// grows between the size check and the read cannot exhaust memory.
func TestReadRegularFileRespectsLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("y", 10_000)), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	content, err := readRegularFile(path, 100)
	if err != nil {
		t.Fatalf("readRegularFile: %v", err)
	}
	if len(content) != 100 {
		t.Errorf("read %d bytes, want 100", len(content))
	}
}

// TestUntrackedNotReadWithoutFlag confirms enumeration stays accurate but no
// content is read when --include-untracked is off.
func TestUntrackedNotReadWithoutFlag(t *testing.T) {
	rootDir, _ := untrackedFixture(t)

	repo := diffOne(t, rootDir, BulkDiffOptions{MaxDiffSize: 1 << 20})

	if !containsPath(repo.UntrackedFiles, "docs/adr/0005-scope.md") {
		t.Errorf("untracked enumeration should be accurate regardless of the flag: %v", repo.UntrackedFiles)
	}
	if strings.Contains(repo.DiffContent, "# ADR 0005") {
		t.Error("untracked content was inlined without --include-untracked")
	}
	if len(repo.OmittedFiles) != 0 {
		t.Errorf("OmittedFiles should be empty when content is not read: %v", repo.OmittedFiles)
	}
}

func TestSplitDiffLines(t *testing.T) {
	tests := []struct {
		name            string
		content         string
		wantLines       []string
		endsWithNewline bool
	}{
		{"empty", "", nil, true},
		{"single line with newline", "a\n", []string{"a"}, true},
		{"single line without newline", "a", []string{"a"}, false},
		{"two lines", "a\nb\n", []string{"a", "b"}, true},
		{"blank line preserved", "a\n\nb\n", []string{"a", "", "b"}, true},
		{"only newline", "\n", []string{""}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines, ends := splitDiffLines([]byte(tt.content))
			if ends != tt.endsWithNewline {
				t.Errorf("endsWithNewline = %v, want %v", ends, tt.endsWithNewline)
			}
			if len(lines) != len(tt.wantLines) {
				t.Fatalf("lines = %q, want %q", lines, tt.wantLines)
			}
			for i := range lines {
				if lines[i] != tt.wantLines[i] {
					t.Errorf("line %d = %q, want %q", i, lines[i], tt.wantLines[i])
				}
			}
		})
	}
}

func containsPath(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func hasOmission(omitted []OmittedFile, path, reason string) bool {
	for _, o := range omitted {
		if o.Path == path && o.Reason == reason {
			return true
		}
	}
	return false
}
