package repository

import (
	"context"
	"testing"
)

// TestNewClient verifies that NewClient creates a client with default settings.
func TestNewClient(t *testing.T) {
	client := NewClient()
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}

	// Verify client can perform basic operations
	ctx := context.Background()
	result := client.IsRepository(ctx, ".")
	if !result {
		t.Error("expected current directory to be a git repository")
	}
}

// TestNewClientWithOptions verifies that client options are applied correctly.
func TestNewClientWithOptions(t *testing.T) {
	testLogger := &testLogger{}

	client := NewClient(
		WithClientLogger(testLogger),
	)

	// Verify client works with custom logger
	ctx := context.Background()

	// Open will call logger.Debug and logger.Info on success
	_, err := client.Open(ctx, ".")
	if err != nil {
		t.Skipf("Skipping test: current directory is not a git repo: %v", err)
	}

	// Verify logger received messages
	if len(testLogger.messages) < 2 {
		t.Errorf("expected logger to receive at least 2 messages, got %d", len(testLogger.messages))
	}
}

// TestIsRepository verifies repository validation.
func TestIsRepository(t *testing.T) {
	client := NewClient()

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
			name:     "current directory (likely a git repo)",
			path:     ".",
			wantBool: true, // This test assumes we're in gzh-cli-git repo
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

// TestParseStatus verifies status parsing logic.
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
			name:   "modified file",
			output: " M README.md",
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{"README.md"},
				StagedFiles:    []string{},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []RenamedFile{},
			},
			wantErr: false,
		},
		{
			name:   "staged file",
			output: "M  README.md",
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{"README.md"},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []RenamedFile{},
			},
			wantErr: false,
		},
		{
			name:   "added file",
			output: "A  newfile.go",
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{"newfile.go"},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []RenamedFile{},
			},
			wantErr: false,
		},
		{
			name:   "untracked file",
			output: "?? untracked.txt",
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{},
				UntrackedFiles: []string{"untracked.txt"},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []RenamedFile{},
			},
			wantErr: false,
		},
		{
			name:   "renamed file",
			output: "R  old.txt -> new.txt",
			want: &Status{
				IsClean:       false,
				ModifiedFiles: []string{},
				StagedFiles:   []string{"new.txt"},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles: []RenamedFile{
					{OldPath: "old.txt", NewPath: "new.txt"},
				},
			},
			wantErr: false,
		},
		{
			name:   "deleted file (staged)",
			output: "D  removed.go",
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{"removed.go"},
				UntrackedFiles: []string{},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{"removed.go"},
				RenamedFiles:   []RenamedFile{},
			},
			wantErr: false,
		},
		{
			name:   "multiple files",
			output: "M  file1.go\nA  file2.go\n?? file3.go",
			want: &Status{
				IsClean:        false,
				ModifiedFiles:  []string{},
				StagedFiles:    []string{"file1.go", "file2.go"},
				UntrackedFiles: []string{"file3.go"},
				ConflictFiles:  []string{},
				DeletedFiles:   []string{},
				RenamedFiles:   []RenamedFile{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseStatus(tt.output)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseStatus() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
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

func (l *testLogger) Debug(msg string, args ...interface{}) {
	l.messages = append(l.messages, msg)
}

func (l *testLogger) Info(msg string, args ...interface{}) {
	l.messages = append(l.messages, msg)
}

func (l *testLogger) Warn(msg string, args ...interface{}) {
	l.messages = append(l.messages, msg)
}

func (l *testLogger) Error(msg string, args ...interface{}) {
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
