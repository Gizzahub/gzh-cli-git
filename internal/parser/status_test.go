package parser

import (
	"reflect"
	"testing"

	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

// TestParseStatus tests the main status parsing function
func TestParseStatus(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    *repository.Status
		wantErr bool
	}{
		{
			name:   "empty output (clean repo)",
			output: "",
			want: &repository.Status{
				IsClean:        true,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []repository.RenamedFile{},
			},
			wantErr: false,
		},
		{
			name:   "modified file in worktree",
			output: " M README.md",
			want: &repository.Status{
				IsClean:        false,
				ModifiedFiles:  []string{"README.md"},
				StagedFiles:    []string{},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []repository.RenamedFile{},
			},
			wantErr: false,
		},
		{
			name:   "modified file in index (staged)",
			output: "M  README.md",
			want: &repository.Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{"README.md"},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []repository.RenamedFile{},
			},
			wantErr: false,
		},
		{
			name:   "modified in both index and worktree",
			output: "MM README.md",
			want: &repository.Status{
				IsClean:        false,
				ModifiedFiles:  []string{"README.md"},
				StagedFiles:    []string{"README.md"},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []repository.RenamedFile{},
			},
			wantErr: false,
		},
		{
			name:   "added file (staged)",
			output: "A  newfile.go",
			want: &repository.Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{"newfile.go"},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []repository.RenamedFile{},
			},
			wantErr: false,
		},
		{
			name:   "deleted file (staged)",
			output: "D  oldfile.go",
			want: &repository.Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{"oldfile.go"},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{"oldfile.go"},
				RenamedFiles:   []repository.RenamedFile{},
			},
			wantErr: false,
		},
		{
			name:   "deleted file in worktree",
			output: " D oldfile.go",
			want: &repository.Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{"oldfile.go"},
				RenamedFiles:   []repository.RenamedFile{},
			},
			wantErr: false,
		},
		{
			name:   "renamed file",
			output: "R  old.txt -> new.txt",
			want: &repository.Status{
				IsClean:       false,
				ModifiedFiles: []string{},
				StagedFiles:   []string{"new.txt"},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles: []repository.RenamedFile{
					{OldPath: "old.txt", NewPath: "new.txt"},
				},
			},
			wantErr: false,
		},
		{
			name:   "copied file",
			output: "C  original.txt",
			want: &repository.Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{"original.txt"},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []repository.RenamedFile{},
			},
			wantErr: false,
		},
		{
			name:   "untracked file",
			output: "?? untracked.txt",
			want: &repository.Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{},
				UntrackedFiles: []string{"untracked.txt"},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []repository.RenamedFile{},
			},
			wantErr: false,
		},
		{
			name:   "conflict file (unmerged)",
			output: "UU conflict.txt",
			want: &repository.Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{"conflict.txt", "conflict.txt"},
				DeletedFiles:   []string{},
				RenamedFiles:   []repository.RenamedFile{},
			},
			wantErr: false,
		},
		{
			name:   "ignored file",
			output: "!! ignored.log",
			want: &repository.Status{
				IsClean:        true,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []repository.RenamedFile{},
			},
			wantErr: true, // '!' not handled in worktree status
		},
		{
			name: "multiple files",
			output: `M  staged.txt
 M modified.txt
?? untracked.txt
D  deleted.txt`,
			want: &repository.Status{
				IsClean:        false,
				ModifiedFiles:  []string{"modified.txt"},
				StagedFiles:    []string{"staged.txt", "deleted.txt"},
				UntrackedFiles: []string{"untracked.txt"},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{"deleted.txt"},
				RenamedFiles:   []repository.RenamedFile{},
			},
			wantErr: false,
		},
		{
			name:    "line too short",
			output:  "XY",
			wantErr: true,
		},
		{
			name:    "unknown index status",
			output:  "X  file.txt",
			wantErr: true,
		},
		{
			name:    "unknown worktree status",
			output:  " X file.txt",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseStatus(tt.output)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseStatus() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseStatus() unexpected error: %v", err)
				return
			}

			if got.IsClean != tt.want.IsClean {
				t.Errorf("IsClean = %v, want %v", got.IsClean, tt.want.IsClean)
			}

			if !reflect.DeepEqual(got.ModifiedFiles, tt.want.ModifiedFiles) {
				t.Errorf("ModifiedFiles = %v, want %v", got.ModifiedFiles, tt.want.ModifiedFiles)
			}

			if !reflect.DeepEqual(got.StagedFiles, tt.want.StagedFiles) {
				t.Errorf("StagedFiles = %v, want %v", got.StagedFiles, tt.want.StagedFiles)
			}

			if !reflect.DeepEqual(got.UntrackedFiles, tt.want.UntrackedFiles) {
				t.Errorf("UntrackedFiles = %v, want %v", got.UntrackedFiles, tt.want.UntrackedFiles)
			}

			if !reflect.DeepEqual(got.ConflictFiles, tt.want.ConflictFiles) {
				t.Errorf("ConflictFiles = %v, want %v", got.ConflictFiles, tt.want.ConflictFiles)
			}

			if !reflect.DeepEqual(got.DeletedFiles, tt.want.DeletedFiles) {
				t.Errorf("DeletedFiles = %v, want %v", got.DeletedFiles, tt.want.DeletedFiles)
			}

			if !reflect.DeepEqual(got.RenamedFiles, tt.want.RenamedFiles) {
				t.Errorf("RenamedFiles = %v, want %v", got.RenamedFiles, tt.want.RenamedFiles)
			}
		})
	}
}

// TestParseBranchInfo tests branch name parsing
func TestParseBranchInfo(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "main branch",
			output: "main",
			want:   "main",
		},
		{
			name:   "feature branch",
			output: "feature/new-feature",
			want:   "feature/new-feature",
		},
		{
			name:   "branch with whitespace",
			output: "  develop  \n",
			want:   "develop",
		},
		{
			name:   "empty (detached HEAD)",
			output: "",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseBranchInfo(tt.output)
			if got != tt.want {
				t.Errorf("ParseBranchInfo() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestParseRemoteInfo tests remote URL parsing
func TestParseRemoteInfo(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "HTTPS URL",
			output: "https://github.com/user/repo.git",
			want:   "https://github.com/user/repo.git",
		},
		{
			name:   "SSH URL",
			output: "git@github.com:user/repo.git",
			want:   "git@github.com:user/repo.git",
		},
		{
			name:   "URL with whitespace",
			output: "  https://github.com/user/repo.git  \n",
			want:   "https://github.com/user/repo.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseRemoteInfo(tt.output)
			if got != tt.want {
				t.Errorf("ParseRemoteInfo() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestParseUpstreamInfo tests upstream branch parsing
func TestParseUpstreamInfo(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "origin/main",
			output: "origin/main",
			want:   "origin/main",
		},
		{
			name:   "upstream/develop",
			output: "upstream/develop",
			want:   "upstream/develop",
		},
		{
			name:   "upstream with whitespace",
			output: "  origin/master  \n",
			want:   "origin/master",
		},
		{
			name:   "no upstream",
			output: "",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseUpstreamInfo(tt.output)
			if got != tt.want {
				t.Errorf("ParseUpstreamInfo() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestParseAheadBehind tests ahead/behind count parsing
func TestParseAheadBehind(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		wantAhead  int
		wantBehind int
		wantErr    bool
	}{
		{
			name:       "ahead and behind",
			output:     "2\t3",
			wantAhead:  2,
			wantBehind: 3,
			wantErr:    false,
		},
		{
			name:       "only ahead",
			output:     "5\t0",
			wantAhead:  5,
			wantBehind: 0,
			wantErr:    false,
		},
		{
			name:       "only behind",
			output:     "0\t4",
			wantAhead:  0,
			wantBehind: 4,
			wantErr:    false,
		},
		{
			name:       "in sync",
			output:     "0\t0",
			wantAhead:  0,
			wantBehind: 0,
			wantErr:    false,
		},
		{
			name:       "empty output",
			output:     "",
			wantAhead:  0,
			wantBehind: 0,
			wantErr:    false,
		},
		{
			name:    "invalid format (no tab)",
			output:  "2 3",
			wantErr: true,
		},
		{
			name:    "invalid format (too many parts)",
			output:  "2\t3\t4",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ahead, behind, err := ParseAheadBehind(tt.output)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseAheadBehind() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseAheadBehind() unexpected error: %v", err)
				return
			}

			if ahead != tt.wantAhead {
				t.Errorf("ahead = %d, want %d", ahead, tt.wantAhead)
			}

			if behind != tt.wantBehind {
				t.Errorf("behind = %d, want %d", behind, tt.wantBehind)
			}
		})
	}
}

// TestParseCommitInfo tests commit info parsing
func TestParseCommitInfo(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		wantHash      string
		wantAuthor    string
		wantEmail     string
		wantSubject   string
		wantTimestamp int64
		wantErr       bool
	}{
		{
			name:          "valid commit",
			line:          "abc1234|John Doe|john@example.com|1609459200|feat: add feature",
			wantHash:      "abc1234",
			wantAuthor:    "John Doe",
			wantEmail:     "john@example.com",
			wantSubject:   "feat: add feature",
			wantTimestamp: 1609459200,
			wantErr:       false,
		},
		{
			name:          "commit with spaces",
			line:          "  def4567  |  Jane Smith  |  jane@example.com  |  1609459300  |  fix: bug fix  ",
			wantHash:      "def4567",
			wantAuthor:    "Jane Smith",
			wantEmail:     "jane@example.com",
			wantSubject:   "fix: bug fix",
			wantTimestamp: 1609459300,
			wantErr:       false,
		},
		{
			name:    "invalid format (too few parts)",
			line:    "abc123|John Doe|john@example.com",
			wantErr: true,
		},
		{
			name:    "invalid hash",
			line:    "|John Doe|john@example.com|1609459200|subject",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, author, email, subject, timestamp, err := ParseCommitInfo(tt.line)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseCommitInfo() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseCommitInfo() unexpected error: %v", err)
				return
			}

			if hash != tt.wantHash {
				t.Errorf("hash = %q, want %q", hash, tt.wantHash)
			}

			if author != tt.wantAuthor {
				t.Errorf("author = %q, want %q", author, tt.wantAuthor)
			}

			if email != tt.wantEmail {
				t.Errorf("email = %q, want %q", email, tt.wantEmail)
			}

			if subject != tt.wantSubject {
				t.Errorf("subject = %q, want %q", subject, tt.wantSubject)
			}

			if timestamp != tt.wantTimestamp {
				t.Errorf("timestamp = %d, want %d", timestamp, tt.wantTimestamp)
			}
		})
	}
}

// TestParseFileList tests file list parsing
func TestParseFileList(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name:   "empty output",
			output: "",
			want:   []string{},
		},
		{
			name:   "single file",
			output: "README.md",
			want:   []string{"README.md"},
		},
		{
			name:   "multiple files",
			output: "README.md\nLICENSE\nsrc/main.go",
			want:   []string{"README.md", "LICENSE", "src/main.go"},
		},
		{
			name:   "files with whitespace",
			output: "  README.md  \n  LICENSE  \n  src/main.go  ",
			want:   []string{"README.md", "LICENSE", "src/main.go"},
		},
		{
			name:   "files with empty lines",
			output: "README.md\n\nLICENSE\n\n\nsrc/main.go",
			want:   []string{"README.md", "LICENSE", "src/main.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseFileList(tt.output)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseFileList() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestParseIsClean tests clean status detection
func TestParseIsClean(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "clean repo",
			output: "",
			want:   true,
		},
		{
			name:   "clean repo with whitespace",
			output: "   \n\t  ",
			want:   true,
		},
		{
			name:   "dirty repo",
			output: "M  README.md",
			want:   false,
		},
		{
			name:   "dirty repo with untracked",
			output: "?? file.txt",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseIsClean(tt.output)
			if got != tt.want {
				t.Errorf("ParseIsClean() = %v, want %v", got, tt.want)
			}
		})
	}
}
