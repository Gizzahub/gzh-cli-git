// Package gitcmd provides Git command execution and output handling.
// This package wraps the Git CLI and provides a safe, structured interface
// for executing Git commands with proper error handling and output parsing.
package gitcmd

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Executor executes Git commands and captures their output.
// It provides a safe wrapper around os/exec with input sanitization,
// timeout support, and structured result handling.
type Executor struct {
	// gitBinary is the path to the Git executable.
	// Defaults to "git" (searches PATH).
	gitBinary string

	// env contains environment variables to set for Git commands.
	// These are added to the inherited environment.
	env []string

	// timeout is the default timeout for Git commands.
	// Individual commands can override this.
	timeout time.Duration
}

// Result contains the result of a Git command execution.
type Result struct {
	// Stdout contains the command's standard output.
	Stdout string

	// Stderr contains the command's standard error output.
	Stderr string

	// ExitCode is the command's exit code.
	// 0 indicates success, non-zero indicates an error.
	ExitCode int

	// Duration is how long the command took to execute.
	Duration time.Duration

	// Error is the error returned by exec, if any.
	// This may be nil even if ExitCode is non-zero.
	Error error
}

// Option configures an Executor.
type Option func(*Executor)

// WithGitBinary sets a custom Git binary path.
func WithGitBinary(path string) Option {
	return func(e *Executor) {
		e.gitBinary = path
	}
}

// WithEnv sets environment variables for Git commands.
func WithEnv(env []string) Option {
	return func(e *Executor) {
		e.env = env
	}
}

// WithTimeout sets the default timeout for Git commands.
func WithTimeout(timeout time.Duration) Option {
	return func(e *Executor) {
		e.timeout = timeout
	}
}

// NewExecutor creates a new Git command executor.
func NewExecutor(opts ...Option) *Executor {
	e := &Executor{
		gitBinary: "git",
		timeout:   5 * time.Minute, // Default 5 minute timeout
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// Run executes a Git command in the specified directory.
// The args are sanitized before execution to prevent command injection.
//
// Example:
//
//	result, err := executor.Run(ctx, "/path/to/repo", "status", "--porcelain")
func (e *Executor) Run(ctx context.Context, dir string, args ...string) (*Result, error) {
	start := time.Now()

	// Sanitize arguments to prevent command injection
	sanitizedArgs, err := SanitizeArgs(args)
	if err != nil {
		return &Result{
			Error:    err,
			ExitCode: -1,
		}, fmt.Errorf("argument sanitization failed: %w", err)
	}

	// Create context with timeout
	cmdCtx := ctx
	if e.timeout > 0 {
		var cancel context.CancelFunc
		cmdCtx, cancel = context.WithTimeout(ctx, e.timeout)
		defer cancel()
	}

	// Build command
	cmd := exec.CommandContext(cmdCtx, e.gitBinary, sanitizedArgs...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Env, e.env...)

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	execErr := cmd.Run()

	// Calculate duration
	duration := time.Since(start)

	// Determine exit code
	exitCode := 0
	if execErr != nil {
		if exitError, ok := execErr.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			// Non-exit error (e.g., command not found)
			exitCode = -1
		}
	}

	result := &Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Duration: duration,
		Error:    execErr,
	}

	return result, nil
}

// RunQuiet executes a Git command and returns only success/failure.
// This is useful for commands where you only care about the exit code.
//
// Example:
//
//	success, err := executor.RunQuiet(ctx, "/path/to/repo", "rev-parse", "--git-dir")
func (e *Executor) RunQuiet(ctx context.Context, dir string, args ...string) (bool, error) {
	result, err := e.Run(ctx, dir, args...)
	if err != nil {
		return false, err
	}

	return result.ExitCode == 0, nil
}

// RunOutput executes a Git command and returns only stdout.
// This is useful for commands where you need to parse the output.
// Returns an error if the command fails (non-zero exit code).
//
// Example:
//
//	branch, err := executor.RunOutput(ctx, "/path/to/repo", "rev-parse", "--abbrev-ref", "HEAD")
func (e *Executor) RunOutput(ctx context.Context, dir string, args ...string) (string, error) {
	result, err := e.Run(ctx, dir, args...)
	if err != nil {
		return "", err
	}

	if result.ExitCode != 0 {
		return "", &GitError{
			Command:  "git " + strings.Join(args, " "),
			ExitCode: result.ExitCode,
			Stderr:   result.Stderr,
		}
	}

	return strings.TrimSpace(result.Stdout), nil
}

// RunLines executes a Git command and returns stdout as a slice of lines.
// Empty lines are filtered out. Returns an error if the command fails.
//
// Example:
//
//	files, err := executor.RunLines(ctx, "/path/to/repo", "ls-files")
func (e *Executor) RunLines(ctx context.Context, dir string, args ...string) ([]string, error) {
	output, err := e.RunOutput(ctx, dir, args...)
	if err != nil {
		return nil, err
	}

	if output == "" {
		return []string{}, nil
	}

	lines := strings.Split(output, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			filtered = append(filtered, line)
		}
	}

	return filtered, nil
}

// IsGitRepository checks if the directory is a Git repository.
// This is a convenience wrapper around git rev-parse --git-dir.
func (e *Executor) IsGitRepository(ctx context.Context, dir string) bool {
	success, _ := e.RunQuiet(ctx, dir, "rev-parse", "--git-dir")
	return success
}

// GetGitVersion returns the Git version string.
// Example: "2.40.0"
func (e *Executor) GetGitVersion(ctx context.Context) (string, error) {
	output, err := e.RunOutput(ctx, "", "version")
	if err != nil {
		return "", err
	}

	// Parse "git version 2.40.0" -> "2.40.0"
	parts := strings.Fields(output)
	if len(parts) >= 3 {
		return parts[2], nil
	}

	return output, nil
}

// GitError represents a Git command execution error.
type GitError struct {
	// Command is the Git command that failed.
	Command string

	// ExitCode is the Git exit code.
	ExitCode int

	// Stderr is the error output from Git.
	Stderr string

	// Cause is the underlying error, if any.
	Cause error
}

// Error implements the error interface.
func (e *GitError) Error() string {
	msg := fmt.Sprintf("git command failed: %s (exit code %d)", e.Command, e.ExitCode)
	if e.Stderr != "" {
		msg += "\n" + e.Stderr
	}
	return msg
}

// Unwrap implements error unwrapping.
func (e *GitError) Unwrap() error {
	return e.Cause
}

// Is implements error comparison.
func (e *GitError) Is(target error) bool {
	_, ok := target.(*GitError)
	return ok
}
