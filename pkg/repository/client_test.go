package repository

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestNewClient verifies that NewClient creates a client with default settings.
func TestNewClient(t *testing.T) {
	client := NewClient()
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}

	// Find project root by looking for .git directory
	projectRoot := "."
	for range 5 {
		if _, err := os.Stat(filepath.Join(projectRoot, ".git")); err == nil {
			break
		}
		projectRoot = filepath.Join("..", projectRoot)
	}

	// Verify client can perform basic operations
	ctx := context.Background()
	result := client.IsRepository(ctx, projectRoot)
	if !result {
		t.Errorf("expected %s to be a git repository", projectRoot)
	}
}

// TestNewClientWithOptions verifies that client options are applied correctly.
func TestNewClientWithOptions(t *testing.T) {
	testLogger := &testLogger{}

	client := NewClient(
		WithClientLogger(testLogger),
	)

	// Find project root by looking for .git directory
	projectRoot := "."
	for range 5 {
		if _, err := os.Stat(filepath.Join(projectRoot, ".git")); err == nil {
			break
		}
		projectRoot = filepath.Join("..", projectRoot)
	}

	// Verify client works with custom logger
	ctx := context.Background()

	// Open will call logger.Debug and logger.Info on success
	_, err := client.Open(ctx, projectRoot)
	if err != nil {
		t.Skipf("Skipping test: %s is not a git repo: %v", projectRoot, err)
	}

	// Verify logger received messages
	if len(testLogger.messages) < 2 {
		t.Errorf("expected logger to receive at least 2 messages, got %d", len(testLogger.messages))
	}
}

// TestIsRepository verifies repository validation.
func TestIsRepository(t *testing.T) {
	client := NewClient()

	// Find project root by looking for .git directory
	projectRoot := "."
	for range 5 {
		if _, err := os.Stat(filepath.Join(projectRoot, ".git")); err == nil {
			break
		}
		projectRoot = filepath.Join("..", projectRoot)
	}

	tests := []struct {
		name     string
		path     string
		wantBool bool
	}{
		{
			name:     "empty path",
			path:     "",
			wantBool: false,
		},
		{
			name:     "project root directory (git repo)",
			path:     projectRoot,
			wantBool: true,
		},
		{
			name:     "subdirectory without .git",
			path:     ".", // pkg/repository/ has no .git
			wantBool: false,
		},
		{
			name:     "non-existent path",
			path:     "/nonexistent/path/to/repo",
			wantBool: false,
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.IsRepository(ctx, tt.path)
			if got != tt.wantBool {
				t.Errorf("IsRepository() = %v, want %v", got, tt.wantBool)
			}
		})
	}
}

// TestOpenValidation verifies input validation for Open.
func TestOpenValidation(t *testing.T) {
	client := NewClient()
	ctx := context.Background()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
		{
			name:    "non-existent path",
			path:    "/nonexistent/path",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.Open(ctx, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("Open() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestCloneValidation verifies input validation for Clone.
func TestCloneValidation(t *testing.T) {
	client := NewClient()
	ctx := context.Background()

	tests := []struct {
		name    string
		opts    CloneOptions
		wantErr bool
	}{
		{
			name: "empty URL",
			opts: CloneOptions{
				URL:         "",
				Destination: "/tmp/test",
			},
			wantErr: true,
		},
		{
			name: "empty Destination",
			opts: CloneOptions{
				URL:         "https://github.com/test/repo.git",
				Destination: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.Clone(ctx, tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("Clone() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// porcelainZ builds a `git status --porcelain -z` payload from records.
//
// git terminates every record with a NUL, including the last, so the fixture
// does too — the trailing empty split element it produces is part of what the
// parser has to tolerate.
func porcelainZ(records ...string) string {
	out := ""
	for _, rec := range records {
		out += rec + "\x00"
	}

	return out
}

// TestParseStatus verifies status parsing logic.
//
// Fixtures are in -z record form, not newline-delimited: several of the cases
// below (renames, paths with spaces) have no faithful newline representation,
// and the "first record" cases exist specifically because the previous
// implementation read newline-delimited, whitespace-trimmed output.
func TestParseStatus(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    *Status
		wantErr bool
	}{
		{
			name:   "empty output (clean)",
			output: "",
			want: &Status{
				IsClean:        true,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []RenamedFile{},
			},
			wantErr: false,
		},
		{
			// Regression: RunOutput trimmed the whole payload, so the leading
			// space of the FIRST record was eaten and " M" was read as "M " —
			// an unstaged edit reported as staged, with the path shifted a byte
			// to "EADME.md". A case in second position cannot catch this.
			name:   "unstaged modify as first record",
			output: porcelainZ(" M README.md", " M second.md"),
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{"README.md", "second.md"},
				StagedFiles:    []string{},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []RenamedFile{},

				StagedCount:         0,
				UnstagedCount:       2,
				TrackedChangedCount: 2,
			},
			wantErr: false,
		},
		{
			// Same regression, deletion flavor: this is the shape that made
			// v0.7.0 report uncommitted_files=1 for two worktree-only deletes.
			name:   "worktree-only delete as first record",
			output: porcelainZ(" D a.txt", " D b.txt"),
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{"a.txt", "b.txt"},
				RenamedFiles:   []RenamedFile{},

				StagedCount:         0,
				UnstagedCount:       2,
				TrackedChangedCount: 2,
			},
			wantErr: false,
		},
		{
			name:   "staged file",
			output: porcelainZ("M  README.md"),
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{"README.md"},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []RenamedFile{},

				StagedCount:         1,
				UnstagedCount:       0,
				TrackedChangedCount: 1,
			},
			wantErr: false,
		},
		{
			// One path, both sides changed: it counts once as a changed path but
			// once on each side. len(StagedFiles)+len(ModifiedFiles) reported 2.
			name:   "staged and modified counts once",
			output: porcelainZ("MM README.md"),
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{"README.md"},
				StagedFiles:    []string{"README.md"},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []RenamedFile{},

				StagedCount:         1,
				UnstagedCount:       1,
				TrackedChangedCount: 1,
			},
			wantErr: false,
		},
		{
			name:   "added file",
			output: porcelainZ("A  newfile.go"),
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{"newfile.go"},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []RenamedFile{},

				StagedCount:         1,
				UnstagedCount:       0,
				TrackedChangedCount: 1,
			},
			wantErr: false,
		},
		{
			name:   "untracked file",
			output: porcelainZ("?? untracked.txt"),
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{},
				UntrackedFiles: []string{"untracked.txt"},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []RenamedFile{},

				StagedCount:         0,
				UnstagedCount:       0,
				TrackedChangedCount: 0,
			},
			wantErr: false,
		},
		{
			// -z disables C-quoting, so a path with a space or a non-ASCII byte
			// arrives verbatim. Without it these came back as "\"docs/한글 파일.md\""
			// with the inner bytes escaped — a string naming no file on disk.
			name:   "paths with spaces and non-ASCII survive intact",
			output: porcelainZ(" M docs/한글 파일.md", "?? my notes.txt"),
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{"docs/한글 파일.md"},
				StagedFiles:    []string{},
				UntrackedFiles: []string{"my notes.txt"},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []RenamedFile{},

				StagedCount:         0,
				UnstagedCount:       1,
				TrackedChangedCount: 1,
			},
			wantErr: false,
		},
		{
			// -z drops " -> " and puts the source in the NEXT record.
			name:   "renamed file",
			output: porcelainZ("R  new.txt", "old.txt"),
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{"new.txt"},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles: []RenamedFile{
					{OldPath: "old.txt", NewPath: "new.txt"},
				},

				StagedCount:         1,
				UnstagedCount:       0,
				TrackedChangedCount: 1,
			},
			wantErr: false,
		},
		{
			// The source record must not be mistaken for a record of its own.
			name:   "rename followed by another record",
			output: porcelainZ("R  new.txt", "old.txt", "?? extra.txt"),
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{"new.txt"},
				UntrackedFiles: []string{"extra.txt"},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles: []RenamedFile{
					{OldPath: "old.txt", NewPath: "new.txt"},
				},

				StagedCount:         1,
				UnstagedCount:       0,
				TrackedChangedCount: 1,
			},
			wantErr: false,
		},
		{
			// The rename letter on the WORKTREE side, emitted for
			// `mv a b && git add -N b`. Pairing keyed on the index column alone
			// left "old.txt" loose in the stream, where the next iteration read
			// it as a status line and took "ol" for an XY code — failing the
			// whole parse on `unknown index status code: o`.
			name:   "worktree-side rename pairs its source",
			output: porcelainZ(" R new.txt", "old.txt", "?? extra.txt"),
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{"new.txt"},
				StagedFiles:    []string{},
				UntrackedFiles: []string{"extra.txt"},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles: []RenamedFile{
					{OldPath: "old.txt", NewPath: "new.txt"},
				},

				StagedCount:         0,
				UnstagedCount:       1,
				TrackedChangedCount: 1,
			},
			wantErr: false,
		},
		{
			// Both columns set: git still emits exactly one source record, so one
			// lookahead is right for RM as much as for "R " and " R".
			name:   "rename staged then modified again",
			output: porcelainZ("RM new.txt", "old.txt"),
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{"new.txt"},
				StagedFiles:    []string{"new.txt"},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles: []RenamedFile{
					{OldPath: "old.txt", NewPath: "new.txt"},
				},

				StagedCount:         1,
				UnstagedCount:       1,
				TrackedChangedCount: 1,
			},
			wantErr: false,
		},
		{
			// Intent-to-add (`git add -N`). Treating worktree 'A' as a no-op let
			// this raise TrackedChangedCount while landing in no list at all, so
			// the counts and the lists disagreed about the same file.
			name:   "intent-to-add is an unstaged addition",
			output: porcelainZ(" A staged-later.txt"),
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{"staged-later.txt"},
				StagedFiles:    []string{},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []RenamedFile{},

				StagedCount:         0,
				UnstagedCount:       1,
				TrackedChangedCount: 1,
			},
			wantErr: false,
		},
		{
			name:   "deleted file (staged)",
			output: porcelainZ("D  removed.go"),
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{"removed.go"},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{"removed.go"},
				RenamedFiles:   []RenamedFile{},

				StagedCount:         1,
				UnstagedCount:       0,
				TrackedChangedCount: 1,
			},
			wantErr: false,
		},
		{
			// AA and DD are unmerged too. The old code filed them as staged
			// adds/deletes and left ConflictFiles empty.
			name:   "all unmerged codes are conflicts, not changes",
			output: porcelainZ("UU both.txt", "AA added.txt", "DD gone.txt", "AU au.txt", "UD ud.txt"),
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{"both.txt", "added.txt", "gone.txt", "au.txt", "ud.txt"},
				DeletedFiles:   []string{},
				RenamedFiles:   []RenamedFile{},

				StagedCount:         0,
				UnstagedCount:       0,
				TrackedChangedCount: 5,
			},
			wantErr: false,
		},
		{
			// T (typechange, e.g. file to symlink) used to hit the default
			// branch and fail the whole parse.
			name:   "typechange",
			output: porcelainZ("T  link.txt", " T other.txt"),
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{"other.txt"},
				StagedFiles:    []string{"link.txt"},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []RenamedFile{},

				StagedCount:         1,
				UnstagedCount:       1,
				TrackedChangedCount: 2,
			},
			wantErr: false,
		},
		{
			name:   "ignored entries are not changes",
			output: porcelainZ("!! build/out.bin", " M real.go"),
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{"real.go"},
				StagedFiles:    []string{},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []RenamedFile{},

				StagedCount:         0,
				UnstagedCount:       1,
				TrackedChangedCount: 1,
			},
			wantErr: false,
		},
		{
			name:   "multiple files",
			output: porcelainZ("M  file1.go", "A  file2.go", "?? file3.go"),
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{"file1.go", "file2.go"},
				UntrackedFiles: []string{"file3.go"},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []RenamedFile{},

				StagedCount:         2,
				UnstagedCount:       0,
				TrackedChangedCount: 2,
			},
			wantErr: false,
		},
		{
			name:    "unknown index status code",
			output:  porcelainZ("X  weird.txt"),
			wantErr: true,
		},
		{
			// Worktree default branch (applyStatusCode second switch). Index-side
			// "X " already covers the first switch; without this case a broken
			// worktree letter would pass silently if the default only lived on X.
			name:    "unknown worktree status code",
			output:  porcelainZ(" X weird.txt"),
			wantErr: true,
		},
		{
			// Index-side copy: -z pairs the source into OldPath. Must land in
			// RenamedFiles, not only StagedFiles — otherwise callers lose the
			// origin path that distinguishes a copy from a plain add.
			name:   "copied file",
			output: porcelainZ("C  new-copy.txt", "original.txt"),
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{"new-copy.txt"},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles: []RenamedFile{
					{OldPath: "original.txt", NewPath: "new-copy.txt"},
				},

				StagedCount:         1,
				UnstagedCount:       0,
				TrackedChangedCount: 1,
			},
			wantErr: false,
		},
		{
			// Worktree-side copy (rare but legal): same pairing contract as " R".
			name:   "worktree-side copy pairs its source",
			output: porcelainZ(" C new-copy.txt", "original.txt"),
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{"new-copy.txt"},
				StagedFiles:    []string{},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles: []RenamedFile{
					{OldPath: "original.txt", NewPath: "new-copy.txt"},
				},

				StagedCount:         0,
				UnstagedCount:       1,
				TrackedChangedCount: 1,
			},
			wantErr: false,
		},
		{
			// The trailing NUL every payload ends with produces one empty split
			// element. It is the only input the parser may discard, so the case
			// that proves it is tolerated has to be stated rather than left
			// implicit in the other fixtures.
			name:   "trailing NUL does not become a phantom record",
			output: porcelainZ("M  only.go"),
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{"only.go"},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []RenamedFile{},

				StagedCount:         1,
				UnstagedCount:       0,
				TrackedChangedCount: 1,
			},
			wantErr: false,
		},
		{
			// Shorter than "XY P". git has no way to emit this, so it means the
			// format is not what the parser believes or stdout was truncated.
			// Skipping it would hide both.
			name:    "record too short to hold a status code",
			output:  porcelainZ("M"),
			wantErr: true,
		},
		{
			// Three bytes: an XY pair and its separator with the path cut off.
			// This is the length that the previous `len < 4 { continue }` guard
			// swallowed, and it is indistinguishable from a truncated read.
			name:    "record with status code but no path",
			output:  porcelainZ("?? "),
			wantErr: true,
		},
		{
			// A rename destination whose source record was cut off. The split's
			// trailing empty element makes this look like "a next record exists",
			// so a length-only lookahead reports a rename with an empty origin.
			name:    "rename with no source record",
			output:  porcelainZ("R  renamed.go"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseStatusZ(tt.output)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseStatusZ() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			if got.StagedCount != tt.want.StagedCount {
				t.Errorf("StagedCount = %v, want %v", got.StagedCount, tt.want.StagedCount)
			}
			if got.UnstagedCount != tt.want.UnstagedCount {
				t.Errorf("UnstagedCount = %v, want %v", got.UnstagedCount, tt.want.UnstagedCount)
			}
			if got.TrackedChangedCount != tt.want.TrackedChangedCount {
				t.Errorf("TrackedChangedCount = %v, want %v", got.TrackedChangedCount, tt.want.TrackedChangedCount)
			}

			// Verify IsClean
			if got.IsClean != tt.want.IsClean {
				t.Errorf("IsClean = %v, want %v", got.IsClean, tt.want.IsClean)
			}

			// Verify file lists
			if !stringSliceEqual(got.ModifiedFiles, tt.want.ModifiedFiles) {
				t.Errorf("ModifiedFiles = %v, want %v", got.ModifiedFiles, tt.want.ModifiedFiles)
			}
			if !stringSliceEqual(got.StagedFiles, tt.want.StagedFiles) {
				t.Errorf("StagedFiles = %v, want %v", got.StagedFiles, tt.want.StagedFiles)
			}
			if !stringSliceEqual(got.UntrackedFiles, tt.want.UntrackedFiles) {
				t.Errorf("UntrackedFiles = %v, want %v", got.UntrackedFiles, tt.want.UntrackedFiles)
			}
			if !stringSliceEqual(got.ConflictFiles, tt.want.ConflictFiles) {
				t.Errorf("ConflictFiles = %v, want %v", got.ConflictFiles, tt.want.ConflictFiles)
			}
			if !stringSliceEqual(got.DeletedFiles, tt.want.DeletedFiles) {
				t.Errorf("DeletedFiles = %v, want %v", got.DeletedFiles, tt.want.DeletedFiles)
			}

			// Verify renamed files
			if len(got.RenamedFiles) != len(tt.want.RenamedFiles) {
				t.Errorf("RenamedFiles length = %v, want %v", len(got.RenamedFiles), len(tt.want.RenamedFiles))
			} else {
				for i := range got.RenamedFiles {
					if got.RenamedFiles[i] != tt.want.RenamedFiles[i] {
						t.Errorf("RenamedFiles[%d] = %v, want %v", i, got.RenamedFiles[i], tt.want.RenamedFiles[i])
					}
				}
			}
		})
	}
}

// TestParseAheadBehind verifies ahead/behind parsing logic.
func TestParseAheadBehind(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		wantAhead  int
		wantBehind int
		wantErr    bool
	}{
		{
			name:       "empty output",
			output:     "",
			wantAhead:  0,
			wantBehind: 0,
			wantErr:    false,
		},
		{
			name:       "ahead only",
			output:     "2\t0",
			wantAhead:  2,
			wantBehind: 0,
			wantErr:    false,
		},
		{
			name:       "behind only",
			output:     "0\t3",
			wantAhead:  0,
			wantBehind: 3,
			wantErr:    false,
		},
		{
			name:       "both ahead and behind",
			output:     "5\t2",
			wantAhead:  5,
			wantBehind: 2,
			wantErr:    false,
		},
		{
			name:       "invalid format",
			output:     "invalid",
			wantAhead:  0,
			wantBehind: 0,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAhead, gotBehind, err := parseAheadBehind(tt.output)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseAheadBehind() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if gotAhead != tt.wantAhead {
				t.Errorf("ahead = %v, want %v", gotAhead, tt.wantAhead)
			}
			if gotBehind != tt.wantBehind {
				t.Errorf("behind = %v, want %v", gotBehind, tt.wantBehind)
			}
		})
	}
}

// TestGetStatus verifies GetStatus functionality.
func TestGetStatus(t *testing.T) {
	client := NewClient()
	ctx := context.Background()

	// Open current directory (should be a git repo)
	repo, err := client.Open(ctx, ".")
	if err != nil {
		t.Skipf("Skipping test: current directory is not a git repo: %v", err)
	}

	// Get status
	status, err := client.GetStatus(ctx, repo)
	if err != nil {
		t.Fatalf("GetStatus() failed: %v", err)
	}

	// Verify status is not nil
	if status == nil {
		t.Fatal("GetStatus() returned nil status")
	}

	// Verify all slices are initialized (even if empty)
	if status.ModifiedFiles == nil {
		t.Error("ModifiedFiles should not be nil")
	}
	if status.StagedFiles == nil {
		t.Error("StagedFiles should not be nil")
	}
	if status.UntrackedFiles == nil {
		t.Error("UntrackedFiles should not be nil")
	}
	if status.ConflictFiles == nil {
		t.Error("ConflictFiles should not be nil")
	}
	if status.DeletedFiles == nil {
		t.Error("DeletedFiles should not be nil")
	}
	if status.RenamedFiles == nil {
		t.Error("RenamedFiles should not be nil")
	}
}

// TestGetInfo verifies GetInfo functionality.
func TestGetInfo(t *testing.T) {
	client := NewClient()
	ctx := context.Background()

	// Open current directory (should be a git repo)
	repo, err := client.Open(ctx, ".")
	if err != nil {
		t.Skipf("Skipping test: current directory is not a git repo: %v", err)
	}

	// Get info
	info, err := client.GetInfo(ctx, repo)
	if err != nil {
		t.Fatalf("GetInfo() failed: %v", err)
	}

	// Verify info is not nil
	if info == nil {
		t.Fatal("GetInfo() returned nil info")
	}

	// Log the retrieved info for debugging
	t.Logf("Branch: %s", info.Branch)
	t.Logf("Remote: %s", info.Remote)
	t.Logf("RemoteURL: %s", info.RemoteURL)
	t.Logf("Upstream: %s", info.Upstream)
	t.Logf("AheadBy: %d", info.AheadBy)
	t.Logf("BehindBy: %d", info.BehindBy)
}

// TestGetStatusNilRepo verifies error handling for nil repository.
func TestGetStatusNilRepo(t *testing.T) {
	client := NewClient()
	ctx := context.Background()

	_, err := client.GetStatus(ctx, nil)
	if err == nil {
		t.Error("GetStatus() should return error for nil repository")
	}
}

// TestGetInfoNilRepo verifies error handling for nil repository.
func TestGetInfoNilRepo(t *testing.T) {
	client := NewClient()
	ctx := context.Background()

	_, err := client.GetInfo(ctx, nil)
	if err == nil {
		t.Error("GetInfo() should return error for nil repository")
	}
}

// Helper functions

// testLogger is a simple logger implementation for testing.
type testLogger struct {
	messages []string
}

func (l *testLogger) Debug(msg string, args ...any) {
	l.messages = append(l.messages, msg)
}

func (l *testLogger) Info(msg string, args ...any) {
	l.messages = append(l.messages, msg)
}

func (l *testLogger) Warn(msg string, args ...any) {
	l.messages = append(l.messages, msg)
}

func (l *testLogger) Error(msg string, args ...any) {
	l.messages = append(l.messages, msg)
}

// stringSliceEqual compares two string slices for equality.
func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestCloneOptions tests all CloneOption functions.
func TestCloneOptions(t *testing.T) {
	tests := []struct {
		name string
		opt  CloneOption
	}{
		{"WithBranch", WithBranch("main")},
		{"WithDepth", WithDepth(1)},
		{"WithSingleBranch", WithSingleBranch()},
		{"WithRecursive", WithRecursive()},
		{"WithProgress", WithProgress(&testProgressReporter{})},
		{"WithLogger", WithLogger(&testLogger{})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &CloneOptions{}
			tt.opt(opts)
			// Just verify the option can be applied without error
		})
	}
}

// TestWithExecutor tests WithExecutor client option.
func TestWithExecutor(t *testing.T) {
	// This is a simple test to just cover the function
	client := NewClient(WithExecutor(nil))
	if client == nil {
		t.Error("NewClient() with WithExecutor returned nil")
	}
}

// TestNoopLogger tests NoopLogger.
func TestNoopLogger(t *testing.T) {
	logger := NewNoopLogger()

	// These should not panic
	logger.Debug("test")
	logger.Info("test")
	logger.Warn("test")
	logger.Error("test")
}

// testProgressReporter is a simple progress reporter for testing.
type testProgressReporter struct{}

func (p *testProgressReporter) Start(total int64)    {}
func (p *testProgressReporter) Update(current int64) {}
func (p *testProgressReporter) Done()                {}
