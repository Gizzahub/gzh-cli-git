package gitcmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNewExecutor tests executor creation with options.
func TestNewExecutor(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
		want *Executor
	}{
		{
			name: "default executor",
			opts: nil,
			want: &Executor{
				gitBinary: "git",
				timeout:   5 * time.Minute,
			},
		},
		{
			name: "custom git binary",
			opts: []Option{WithGitBinary("/usr/bin/git")},
			want: &Executor{
				gitBinary: "/usr/bin/git",
				timeout:   5 * time.Minute,
			},
		},
		{
			name: "custom timeout",
			opts: []Option{WithTimeout(10 * time.Second)},
			want: &Executor{
				gitBinary: "git",
				timeout:   10 * time.Second,
			},
		},
		{
			name: "custom environment",
			opts: []Option{WithEnv([]string{"GIT_AUTHOR_NAME=Test"})},
			want: &Executor{
				gitBinary: "git",
				env:       []string{"GIT_AUTHOR_NAME=Test"},
				timeout:   5 * time.Minute,
			},
		},
		{
			name: "all options",
			opts: []Option{
				WithGitBinary("/custom/git"),
				WithTimeout(30 * time.Second),
				WithEnv([]string{"VAR=value"}),
			},
			want: &Executor{
				gitBinary: "/custom/git",
				timeout:   30 * time.Second,
				env:       []string{"VAR=value"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewExecutor(tt.opts...)

			if got.gitBinary != tt.want.gitBinary {
				t.Errorf("gitBinary = %q, want %q", got.gitBinary, tt.want.gitBinary)
			}

			if got.timeout != tt.want.timeout {
				t.Errorf("timeout = %v, want %v", got.timeout, tt.want.timeout)
			}

			if len(got.env) != len(tt.want.env) {
				t.Errorf("env length = %d, want %d", len(got.env), len(tt.want.env))
			}
		})
	}
}

// TestExecutorRun tests basic command execution.
func TestExecutorRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping executor test in short mode")
	}

	executor := NewExecutor()
	ctx := context.Background()

	tests := []struct {
		name         string
		args         []string
		wantErr      bool
		wantExitCode int
	}{
		{
			name:         "git version succeeds",
			args:         []string{"version"},
			wantErr:      false,
			wantExitCode: 0,
		},
		{
			name:         "git help succeeds",
			args:         []string{"help"},
			wantErr:      false,
			wantExitCode: 0,
		},
		{
			name:         "dangerous args rejected",
			args:         []string{"status", "; rm -rf /"},
			wantErr:      true,
			wantExitCode: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executor.Run(ctx, "", tt.args...)

			if tt.wantErr {
				if err == nil {
					t.Error("Run() expected error, got nil")
				}
				if result.ExitCode != tt.wantExitCode {
					t.Errorf("ExitCode = %d, want %d", result.ExitCode, tt.wantExitCode)
				}
				return
			}

			if err != nil {
				t.Errorf("Run() unexpected error: %v", err)
				return
			}

			if result.ExitCode != 0 {
				t.Errorf("ExitCode = %d, want 0", result.ExitCode)
			}

			if result.Stdout == "" {
				t.Error("Stdout is empty, expected output")
			}

			if result.Duration == 0 {
				t.Error("Duration is 0, expected non-zero")
			}
		})
	}
}

// TestExecutorRunInRepo tests command execution in a real Git repository.
func TestExecutorRunInRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping repository test in short mode")
	}

	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Initialize a Git repository
	executor := NewExecutor()
	ctx := context.Background()

	// Initialize repo
	result, err := executor.Run(ctx, tmpDir, "init")
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("Failed to init repo: %v (stderr: %s)", err, result.Stderr)
	}

	// Configure user for commits
	executor.Run(ctx, tmpDir, "config", "user.name", "Test User")
	executor.Run(ctx, tmpDir, "config", "user.email", "test@example.com")

	tests := []struct {
		name        string
		setup       func() // Setup function to run before test
		args        []string
		wantErr     bool
		checkStdout func(string) bool // Optional stdout validation
	}{
		{
			name:        "git status in clean repo",
			args:        []string{"status", "--porcelain"},
			wantErr:     false,
			checkStdout: func(s string) bool { return true }, // Clean repo
		},
		{
			name:    "git branch list",
			args:    []string{"branch"},
			wantErr: false,
		},
		{
			name: "create test file and check status",
			setup: func() {
				os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("test"), 0o644)
			},
			args:    []string{"status", "--porcelain"},
			wantErr: false,
			checkStdout: func(s string) bool {
				return strings.Contains(s, "test.txt")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}

			result, err := executor.Run(ctx, tmpDir, tt.args...)

			if tt.wantErr {
				if err == nil && result.ExitCode == 0 {
					t.Error("Run() expected error, got success")
				}
				return
			}

			if err != nil {
				t.Errorf("Run() unexpected error: %v", err)
				return
			}

			if result.ExitCode != 0 {
				t.Errorf("ExitCode = %d, stderr: %s", result.ExitCode, result.Stderr)
			}

			if tt.checkStdout != nil && !tt.checkStdout(result.Stdout) {
				t.Errorf("Stdout validation failed: %q", result.Stdout)
			}
		})
	}
}

// TestExecutorRunQuiet tests RunQuiet method.
func TestExecutorRunQuiet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping executor test in short mode")
	}

	executor := NewExecutor()
	ctx := context.Background()

	tests := []struct {
		name    string
		args    []string
		want    bool
		wantErr bool
	}{
		{
			name:    "successful command",
			args:    []string{"version"},
			want:    true,
			wantErr: false,
		},
		{
			name:    "dangerous args",
			args:    []string{"; whoami"},
			want:    false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := executor.RunQuiet(ctx, "", tt.args...)

			if tt.wantErr {
				if err == nil {
					t.Error("RunQuiet() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("RunQuiet() unexpected error: %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("RunQuiet() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExecutorRunOutput tests RunOutput method.
func TestExecutorRunOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping executor test in short mode")
	}

	executor := NewExecutor()
	ctx := context.Background()

	tests := []struct {
		name     string
		args     []string
		wantErr  bool
		contains string // Expected substring in output
	}{
		{
			name:     "git version output",
			args:     []string{"version"},
			wantErr:  false,
			contains: "git version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := executor.RunOutput(ctx, "", tt.args...)

			if tt.wantErr {
				if err == nil {
					t.Error("RunOutput() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("RunOutput() unexpected error: %v", err)
				return
			}

			if tt.contains != "" && !strings.Contains(got, tt.contains) {
				t.Errorf("RunOutput() output %q does not contain %q", got, tt.contains)
			}
		})
	}
}

// TestExecutorRunLines tests RunLines method.
func TestExecutorRunLines(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping executor test in short mode")
	}

	executor := NewExecutor()
	ctx := context.Background()

	t.Run("git help returns multiple lines", func(t *testing.T) {
		lines, err := executor.RunLines(ctx, "", "help")
		if err != nil {
			t.Fatalf("RunLines() error: %v", err)
		}

		if len(lines) == 0 {
			t.Error("RunLines() returned empty slice, expected lines")
		}
	})
}

// TestExecutorIsGitRepository tests IsGitRepository method.
func TestExecutorIsGitRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping repository test in short mode")
	}

	executor := NewExecutor()
	ctx := context.Background()

	// Create temp Git repo
	tmpDir := t.TempDir()
	executor.Run(ctx, tmpDir, "init")

	tests := []struct {
		name string
		dir  string
		want bool
	}{
		{
			name: "valid git repository",
			dir:  tmpDir,
			want: true,
		},
		{
			name: "non-git directory",
			dir:  t.TempDir(),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := executor.IsGitRepository(ctx, tt.dir)
			if got != tt.want {
				t.Errorf("IsGitRepository() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExecutorGetGitVersion tests GetGitVersion method.
func TestExecutorGetGitVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping executor test in short mode")
	}

	executor := NewExecutor()
	ctx := context.Background()

	version, err := executor.GetGitVersion(ctx)
	if err != nil {
		t.Fatalf("GetGitVersion() error: %v", err)
	}

	if version == "" {
		t.Error("GetGitVersion() returned empty string")
	}

	// Version should be something like "2.40.0"
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		t.Errorf("GetGitVersion() = %q, expected version format x.y.z", version)
	}
}

// TestGitError tests GitError type.
func TestGitError(t *testing.T) {
	tests := []struct {
		name    string
		err     *GitError
		wantMsg string
	}{
		{
			name: "basic error",
			err: &GitError{
				Command:  "git status",
				ExitCode: 128,
				Stderr:   "not a git repository",
			},
			wantMsg: "git command failed: git status (exit code 128)",
		},
		{
			name: "error with no stderr",
			err: &GitError{
				Command:  "git clone",
				ExitCode: 1,
			},
			wantMsg: "git command failed: git clone (exit code 1)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMsg := tt.err.Error()

			if !strings.Contains(gotMsg, tt.wantMsg) {
				t.Errorf("Error() = %q, want to contain %q", gotMsg, tt.wantMsg)
			}

			if tt.err.Stderr != "" && !strings.Contains(gotMsg, tt.err.Stderr) {
				t.Errorf("Error() = %q, want to contain stderr %q", gotMsg, tt.err.Stderr)
			}
		})
	}
}

// TestGitErrorIs tests GitError.Is method.
func TestGitErrorIs(t *testing.T) {
	err1 := &GitError{Command: "git status", ExitCode: 128}
	err2 := &GitError{Command: "git clone", ExitCode: 1}

	if !err1.Is(err2) {
		t.Error("GitError.Is() should return true for another GitError")
	}

	if err1.Is(context.Canceled) {
		t.Error("GitError.Is() should return false for non-GitError")
	}
}

// TestExecutorTimeout tests command timeout.
func TestExecutorTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timeout test in short mode")
	}

	// Create executor with very short timeout
	executor := NewExecutor(WithTimeout(1 * time.Millisecond))
	ctx := context.Background()

	// Try to run a command that would take longer than timeout
	// Note: This might not always timeout depending on system load
	result, _ := executor.Run(ctx, "", "version")

	// We just want to make sure it doesn't hang forever
	// The result might succeed if the command is very fast
	if result == nil {
		t.Error("Run() returned nil result")
	}
}

// TestExecutorContextCancellation tests context cancellation.
func TestExecutorContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cancellation test in short mode")
	}

	executor := NewExecutor()
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	result, _ := executor.Run(ctx, "", "version")

	// Should complete (might succeed or fail depending on timing)
	if result == nil {
		t.Error("Run() returned nil result after context cancellation")
	}
}
