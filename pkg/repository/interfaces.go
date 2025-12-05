// Package repository provides core Git repository operations and interfaces.
// This package defines the public API for interacting with Git repositories
// and is designed to be consumed by both the CLI and external Go applications.
package repository

import (
	"context"
	"fmt"
	"io"
	"time"
)

// Client defines core repository operations.
// This is the primary interface for interacting with Git repositories.
// All methods accept context.Context for cancellation and timeout support.
type Client interface {
	// Repository lifecycle operations

	// Open opens an existing Git repository at the specified path.
	// Returns an error if the path is not a valid Git repository.
	Open(ctx context.Context, path string) (*Repository, error)

	// Clone clones a repository from the specified URL to the destination path.
	// Returns the opened repository on success.
	Clone(ctx context.Context, opts CloneOptions) (*Repository, error)

	// CloneOrUpdate intelligently clones a repository if it doesn't exist,
	// or updates it using the specified strategy if it does.
	// This is a high-level convenience method for repository synchronization.
	CloneOrUpdate(ctx context.Context, opts CloneOrUpdateOptions) (*CloneOrUpdateResult, error)

	// BulkUpdate scans for repositories and updates them in parallel.
	// This is useful for updating multiple repositories at once.
	BulkUpdate(ctx context.Context, opts BulkUpdateOptions) (*BulkUpdateResult, error)

	// BulkFetch scans for repositories and fetches them in parallel.
	// This is useful for fetching updates from multiple repositories at once.
	BulkFetch(ctx context.Context, opts BulkFetchOptions) (*BulkFetchResult, error)

	// BulkPull scans for repositories and pulls them in parallel.
	// This is useful for pulling updates (fetch + merge/rebase) from multiple repositories at once.
	BulkPull(ctx context.Context, opts BulkPullOptions) (*BulkPullResult, error)

	// BulkPush scans for repositories and pushes them in parallel.
	// This is useful for pushing local commits from multiple repositories at once.
	BulkPush(ctx context.Context, opts BulkPushOptions) (*BulkPushResult, error)

	// IsRepository checks if the path points to a valid Git repository.
	// Returns true if the path contains a .git directory or is a bare repository.
	IsRepository(ctx context.Context, path string) bool

	// Repository inspection operations

	// GetInfo retrieves detailed information about a repository.
	// This includes configuration, remote URLs, and other metadata.
	GetInfo(ctx context.Context, repo *Repository) (*Info, error)

	// GetStatus retrieves the current working tree status.
	// This shows modified, staged, untracked files, etc.
	GetStatus(ctx context.Context, repo *Repository) (*Status, error)
}

// Logger provides a logging interface for library consumers.
// This allows the library to integrate with any logging framework
// without taking a hard dependency on a specific logger.
//
// Library code should accept Logger via dependency injection.
// CLI code can provide a concrete logger implementation.
type Logger interface {
	// Debug logs a debug-level message with optional key-value pairs.
	Debug(msg string, args ...interface{})

	// Info logs an info-level message with optional key-value pairs.
	Info(msg string, args ...interface{})

	// Warn logs a warning-level message with optional key-value pairs.
	Warn(msg string, args ...interface{})

	// Error logs an error-level message with optional key-value pairs.
	Error(msg string, args ...interface{})
}

// ProgressReporter provides progress feedback for long-running operations.
// This allows library consumers to display progress bars or other UI feedback.
type ProgressReporter interface {
	// Start initializes the progress reporter with the total amount of work.
	// The total may be in bytes, number of files, or other units depending on the operation.
	Start(total int64)

	// Update reports current progress.
	// The current value should be <= total.
	Update(current int64)

	// Done signals that the operation is complete.
	// This should be called even if the operation fails.
	Done()
}

// Repository represents a Git repository handle.
// This is returned by Open and Clone operations and passed to other methods.
type Repository struct {
	// Path is the absolute path to the repository root.
	// This is the directory containing the .git directory (or the repository itself if bare).
	Path string

	// GitDir is the path to the .git directory.
	// For normal repositories, this is Path/.git
	// For bare repositories, this is the same as Path
	// For worktrees, this points to the linked .git file
	GitDir string

	// WorkTree is the working tree path.
	// For normal repositories, this is the same as Path
	// For bare repositories, this is empty
	// For worktrees, this may differ from Path
	WorkTree string

	// IsBare indicates if this is a bare repository (no working tree).
	IsBare bool

	// IsShallow indicates if this is a shallow clone (partial history).
	IsShallow bool
}

// Info contains detailed repository information.
type Info struct {
	// Branch is the current branch name (e.g., "main", "master").
	// Empty if in detached HEAD state.
	Branch string

	// Commit is the current HEAD commit hash (full SHA-1).
	Commit string

	// Remote is the default remote name (usually "origin").
	Remote string

	// RemoteURL is the URL of the default remote.
	RemoteURL string

	// IsDirty indicates if there are uncommitted changes.
	IsDirty bool

	// Upstream is the upstream branch (e.g., "origin/main").
	// Empty if no upstream is configured.
	Upstream string

	// AheadBy is the number of commits ahead of upstream.
	AheadBy int

	// BehindBy is the number of commits behind upstream.
	BehindBy int
}

// Status represents the working tree and staging area status.
type Status struct {
	// IsClean is true if there are no changes (working tree matches HEAD).
	IsClean bool

	// ModifiedFiles are files with unstaged changes.
	ModifiedFiles []string

	// StagedFiles are files staged for commit.
	StagedFiles []string

	// UntrackedFiles are files not tracked by Git.
	UntrackedFiles []string

	// ConflictFiles are files with merge conflicts.
	ConflictFiles []string

	// DeletedFiles are files deleted but not staged.
	DeletedFiles []string

	// RenamedFiles are files that have been renamed.
	RenamedFiles []RenamedFile
}

// RenamedFile represents a file that has been renamed.
type RenamedFile struct {
	// OldPath is the original file path.
	OldPath string

	// NewPath is the new file path.
	NewPath string
}

// CloneOptions configures repository cloning.
// Use the With* functions to set options (functional options pattern).
type CloneOptions struct {
	// URL is the repository URL to clone (required).
	URL string

	// Destination is the local path where the repository will be cloned (required).
	Destination string

	// Branch is the branch to check out after cloning.
	// If empty, the remote's default branch is used.
	Branch string

	// Depth limits the clone depth (number of commits).
	// 0 means full clone, 1 means shallow clone with only the latest commit.
	Depth int

	// SingleBranch clones only the specified branch.
	// If true, other branches are not fetched.
	SingleBranch bool

	// Recursive clones submodules recursively.
	Recursive bool

	// Bare creates a bare repository (no working directory).
	Bare bool

	// Mirror creates a mirror repository (all refs are copied).
	Mirror bool

	// Quiet suppresses progress output.
	Quiet bool

	// CreateBranch creates the branch if it doesn't exist on the remote.
	// If true and the specified branch doesn't exist, it will be created after cloning.
	// Only effective when Branch is specified.
	CreateBranch bool

	// Progress is an optional progress reporter.
	// If provided, clone progress will be reported.
	Progress ProgressReporter

	// Logger is an optional logger.
	// If provided, clone operations will be logged.
	Logger Logger
}

// CloneOption is a functional option for configuring clone operations.
type CloneOption func(*CloneOptions)

// WithBranch sets the branch to check out after cloning.
func WithBranch(branch string) CloneOption {
	return func(opts *CloneOptions) {
		opts.Branch = branch
	}
}

// WithDepth sets the clone depth (for shallow clones).
// A depth of 1 creates a shallow clone with only the latest commit.
func WithDepth(depth int) CloneOption {
	return func(opts *CloneOptions) {
		opts.Depth = depth
	}
}

// WithSingleBranch enables single-branch mode (only clone specified branch).
func WithSingleBranch() CloneOption {
	return func(opts *CloneOptions) {
		opts.SingleBranch = true
	}
}

// WithRecursive enables recursive submodule cloning.
func WithRecursive() CloneOption {
	return func(opts *CloneOptions) {
		opts.Recursive = true
	}
}

// WithProgress sets a progress reporter for the clone operation.
func WithProgress(progress ProgressReporter) CloneOption {
	return func(opts *CloneOptions) {
		opts.Progress = progress
	}
}

// WithLogger sets a logger for the clone operation.
func WithLogger(logger Logger) CloneOption {
	return func(opts *CloneOptions) {
		opts.Logger = logger
	}
}

// Result represents the result of a Git operation.
type Result struct {
	// Success indicates if the operation succeeded.
	Success bool

	// Output contains the operation's stdout output.
	Output string

	// Error contains the operation's stderr output or error message.
	Error string

	// ExitCode is the Git command's exit code.
	// 0 indicates success, non-zero indicates an error.
	ExitCode int

	// Duration is how long the operation took.
	Duration time.Duration

	// Timestamp is when the operation completed.
	Timestamp time.Time
}

// noopLogger is a default logger that does nothing.
// This is used when no logger is provided by the consumer.
type noopLogger struct{}

func (n *noopLogger) Debug(msg string, args ...interface{}) {}
func (n *noopLogger) Info(msg string, args ...interface{})  {}
func (n *noopLogger) Warn(msg string, args ...interface{})  {}
func (n *noopLogger) Error(msg string, args ...interface{}) {}

// noopProgress is a default progress reporter that does nothing.
// This is used when no progress reporter is provided by the consumer.
type noopProgress struct{}

func (n *noopProgress) Start(total int64)    {}
func (n *noopProgress) Update(current int64) {}
func (n *noopProgress) Done()                {}

// NewNoopLogger creates a no-op logger.
// This is useful for testing or when logging is not needed.
func NewNoopLogger() Logger {
	return &noopLogger{}
}

// NewNoopProgress creates a no-op progress reporter.
// This is useful for testing or when progress reporting is not needed.
func NewNoopProgress() ProgressReporter {
	return &noopProgress{}
}

// WriterLogger wraps an io.Writer as a simple logger.
// All log levels write to the same writer with level prefixes.
type WriterLogger struct {
	w io.Writer
}

// NewWriterLogger creates a logger that writes to an io.Writer.
// This is useful for simple logging to stdout/stderr.
func NewWriterLogger(w io.Writer) Logger {
	return &WriterLogger{w: w}
}

func (l *WriterLogger) Debug(msg string, args ...interface{}) {
	l.log("DEBUG", msg, args...)
}

func (l *WriterLogger) Info(msg string, args ...interface{}) {
	l.log("INFO", msg, args...)
}

func (l *WriterLogger) Warn(msg string, args ...interface{}) {
	l.log("WARN", msg, args...)
}

func (l *WriterLogger) Error(msg string, args ...interface{}) {
	l.log("ERROR", msg, args...)
}

func (l *WriterLogger) log(level, msg string, args ...interface{}) {
	// Simple format: [LEVEL] message key=value key=value
	if l.w == nil {
		return
	}

	output := "[" + level + "] " + msg
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			output += " " + args[i].(string) + "=" + formatValue(args[i+1])
		}
	}
	output += "\n"

	l.w.Write([]byte(output))
}

func formatValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", val)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", val)
	case float32, float64:
		return fmt.Sprintf("%f", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
