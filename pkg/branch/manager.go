// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package branch

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// BranchManager manages Git branch operations.
type BranchManager interface {
	// Create creates a new branch.
	Create(ctx context.Context, repo *repository.Repository, opts CreateOptions) error

	// Delete deletes a branch.
	Delete(ctx context.Context, repo *repository.Repository, opts DeleteOptions) error

	// List lists branches.
	List(ctx context.Context, repo *repository.Repository, opts ListOptions) ([]*Branch, error)

	// Get retrieves a specific branch by name.
	Get(ctx context.Context, repo *repository.Repository, name string) (*Branch, error)

	// Current returns the currently checked out branch.
	Current(ctx context.Context, repo *repository.Repository) (*Branch, error)

	// Exists checks if a branch exists.
	Exists(ctx context.Context, repo *repository.Repository, name string) (bool, error)
}

// manager implements BranchManager.
type manager struct {
	executor *gitcmd.Executor
}

// NewManager creates a new BranchManager.
func NewManager() BranchManager {
	return &manager{
		executor: gitcmd.NewExecutor(),
	}
}

// NewManagerWithExecutor creates a new BranchManager with custom executor.
func NewManagerWithExecutor(executor *gitcmd.Executor) BranchManager {
	return &manager{
		executor: executor,
	}
}

// run executes a git command and turns a non-zero exit into an error.
//
// Exists is the one method here that must not use it: its whole answer is the
// exit code of `rev-parse --verify`, so it calls the executor directly. Every
// other command in this file either changes the repository or produces output to
// parse, and for those a non-zero exit means the result is not there.
func (m *manager) run(ctx context.Context, dir string, args ...string) (*gitcmd.Result, error) {
	return runGit(ctx, m.executor, dir, args...)
}

// Create creates a new branch.
func (m *manager) Create(ctx context.Context, repo *repository.Repository, opts CreateOptions) error {
	if repo == nil {
		return fmt.Errorf("repository cannot be nil")
	}

	if opts.Name == "" {
		return fmt.Errorf("branch name is required")
	}

	// Validate branch name
	if opts.Validate {
		if err := validateBranchName(opts.Name); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidName, err)
		}
	}

	// Check if branch already exists
	exists, err := m.Exists(ctx, repo, opts.Name)
	if err != nil {
		return fmt.Errorf("failed to check branch existence: %w", err)
	}

	if exists && !opts.Force {
		return fmt.Errorf("%w: %s (use --force to overwrite)", ErrBranchExists, opts.Name)
	}

	// Set default start ref
	startRef := opts.StartRef
	if startRef == "" {
		startRef = "HEAD"
	}

	// Verify start ref exists. A ref that does not resolve is reported by the
	// exit code, so checking only err let every bad start ref through this guard
	// and down to `git branch`, which failed with its own wording — ErrInvalidRef
	// was unreachable.
	if _, err := m.run(ctx, repo.Path, "rev-parse", "--verify", startRef); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidRef, startRef)
	}

	// Build git branch command
	args := []string{"branch"}

	if opts.Force {
		args = append(args, "--force")
	}

	if opts.Track {
		args = append(args, "--track")
	}

	args = append(args, opts.Name, startRef)

	// Create branch
	if _, err := m.run(ctx, repo.Path, args...); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	// Checkout if requested. This one is the reason Create must not report a
	// failed git as success: a caller told the branch was created and checked out
	// goes on to commit, and HEAD never moved.
	if opts.Checkout {
		if _, err := m.run(ctx, repo.Path, "checkout", opts.Name); err != nil {
			return fmt.Errorf("failed to checkout branch: %w", err)
		}
	}

	return nil
}

// Delete deletes a branch.
func (m *manager) Delete(ctx context.Context, repo *repository.Repository, opts DeleteOptions) error {
	if repo == nil {
		return fmt.Errorf("repository cannot be nil")
	}

	if opts.Name == "" {
		return fmt.Errorf("branch name is required")
	}

	// Check if branch exists
	exists, err := m.Exists(ctx, repo, opts.Name)
	if err != nil {
		return fmt.Errorf("failed to check branch existence: %w", err)
	}

	if !exists {
		return fmt.Errorf("%w: %s", ErrBranchNotFound, opts.Name)
	}

	// Get branch info
	branch, err := m.Get(ctx, repo, opts.Name)
	if err != nil {
		return fmt.Errorf("failed to get branch info: %w", err)
	}

	// Safety checks
	if !opts.Force {
		// Cannot delete current branch
		if branch.IsHead {
			return fmt.Errorf("%w: %s", ErrBranchIsHead, opts.Name)
		}

		// Cannot delete protected branch
		if IsProtected(opts.Name) {
			return fmt.Errorf("%w: %s (use --force to override)", ErrProtectedBranch, opts.Name)
		}

		// Warn if branch is unmerged
		if !branch.IsMerged && !opts.Confirm {
			return fmt.Errorf("%w: %s (use --force to delete anyway)", ErrBranchUnmerged, opts.Name)
		}
	}

	// Dry run - just return
	if opts.DryRun {
		return nil
	}

	// Delete local branch
	deleteFlag := "-d"
	if opts.Force {
		deleteFlag = "-D"
	}

	// `git branch -d` refuses to delete an unmerged branch — that refusal arrives
	// as a non-zero exit, so checking only err turned git's last safety net into a
	// reported deletion.
	if _, err := m.run(ctx, repo.Path, "branch", deleteFlag, opts.Name); err != nil {
		return fmt.Errorf("failed to delete branch: %w", err)
	}

	// Delete remote branch if requested
	if opts.Remote {
		// Parse remote from upstream
		if branch.Upstream != "" {
			parts := strings.Split(branch.Upstream, "/")
			if len(parts) >= 2 {
				remote := parts[0]
				remoteBranch := strings.Join(parts[1:], "/")

				if _, err := m.run(ctx, repo.Path, "push", remote, "--delete", remoteBranch); err != nil {
					return fmt.Errorf("failed to delete remote branch: %w", err)
				}
			}
		}
	}

	return nil
}

// List lists branches.
func (m *manager) List(ctx context.Context, repo *repository.Repository, opts ListOptions) ([]*Branch, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository cannot be nil")
	}

	// Build git branch command
	args := []string{"branch", "--list", "--verbose", "--verbose"}

	if opts.All {
		args = append(args, "--all")
	}

	if opts.Merged {
		args = append(args, "--merged")
	} else if opts.Unmerged {
		args = append(args, "--no-merged")
	}

	// Add pattern if specified
	if opts.Pattern != "" {
		args = append(args, opts.Pattern)
	}

	// Run command. A failed `git branch` leaves Stdout empty, which parses into
	// an empty slice — a repository with no branches, which is not a state git
	// can actually be in and is exactly what cleanup reads as "nothing to do".
	result, err := m.run(ctx, repo.Path, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}

	// Parse output
	branches, err := m.parseBranchList(result.Stdout)
	if err != nil {
		return nil, fmt.Errorf("failed to parse branch list: %w", err)
	}

	// Filter remote if specified
	if opts.Remote != "" {
		filtered := make([]*Branch, 0)
		for _, b := range branches {
			if b.IsRemote && strings.HasPrefix(b.Name, opts.Remote+"/") {
				filtered = append(filtered, b)
			}
		}
		branches = filtered
	}

	// Apply limit
	if opts.Limit > 0 && len(branches) > opts.Limit {
		branches = branches[:opts.Limit]
	}

	return branches, nil
}

// Get retrieves a specific branch by name.
func (m *manager) Get(ctx context.Context, repo *repository.Repository, name string) (*Branch, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository cannot be nil")
	}

	if name == "" {
		return nil, fmt.Errorf("branch name is required")
	}

	// List branches with pattern
	branches, err := m.List(ctx, repo, ListOptions{Pattern: name})
	if err != nil {
		return nil, err
	}

	// Find exact match
	for _, b := range branches {
		if b.Name == name {
			return b, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", ErrBranchNotFound, name)
}

// Current returns the currently checked out branch.
func (m *manager) Current(ctx context.Context, repo *repository.Repository) (*Branch, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository cannot be nil")
	}

	// Get current branch name. A failed rev-parse leaves Stdout empty, and an
	// empty name is not "HEAD", so the detached-HEAD branch below is skipped and
	// Get is asked for a branch called "".
	result, err := m.run(ctx, repo.Path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}

	branchName := strings.TrimSpace(result.Stdout)

	// Check for detached HEAD
	if branchName == "HEAD" {
		return nil, ErrDetachedHead
	}

	// Get full branch info
	return m.Get(ctx, repo, branchName)
}

// Exists checks if a branch exists.
func (m *manager) Exists(ctx context.Context, repo *repository.Repository, name string) (bool, error) {
	if repo == nil {
		return false, fmt.Errorf("repository cannot be nil")
	}

	if name == "" {
		return false, fmt.Errorf("branch name is required")
	}

	// Try to get branch ref.
	//
	// Deliberately not m.run: here the exit code IS the answer, so turning a
	// non-zero exit into an error would make "branch does not exist" a failure.
	// This is the one place in the file where checking err alone is correct,
	// because err genuinely means only that git could not be run.
	result, err := m.executor.Run(ctx, repo.Path, "rev-parse", "--verify", fmt.Sprintf("refs/heads/%s", name))
	if err != nil {
		// Sanitization or other error
		return false, err
	}

	// Check exit code - non-zero means branch doesn't exist
	return result.ExitCode == 0, nil
}

// parseBranchList parses git branch -vv output.
func (m *manager) parseBranchList(output string) ([]*Branch, error) { //nolint:unparam // error return kept for future use and consistent interface
	lines := strings.Split(strings.TrimSpace(output), "\n")
	branches := make([]*Branch, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}

		branch, err := m.parseBranchLine(line)
		if err != nil {
			// Skip unparseable lines
			continue
		}

		branches = append(branches, branch)
	}

	return branches, nil
}

// parseBranchLine parses a single line from git branch -vv output.
func (m *manager) parseBranchLine(line string) (*Branch, error) {
	// Format: "* main  abc1234 [origin/main] Commit message"
	// Format: "* main  abc1234 [origin/main: ahead 2, behind 3] Commit message"
	// Format: "  feature/x abc1234 Commit message"

	branch := &Branch{}

	// Check if this is the current branch (starts with *)
	if strings.HasPrefix(line, "*") {
		branch.IsHead = true
		line = strings.TrimPrefix(line, "*")
	}

	line = strings.TrimSpace(line)

	// Parse name, SHA, upstream, and message
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid branch line format")
	}

	branch.Name = parts[0]
	branch.SHA = parts[1]
	branch.Ref = fmt.Sprintf("refs/heads/%s", branch.Name)

	// Check if remote branch
	if strings.HasPrefix(branch.Name, "remotes/") {
		branch.IsRemote = true
		branch.Name = strings.TrimPrefix(branch.Name, "remotes/")
		branch.Ref = fmt.Sprintf("refs/remotes/%s", branch.Name)
	}

	// Parse upstream (if present, in brackets)
	if len(parts) > 2 && strings.HasPrefix(parts[2], "[") {
		// Find the complete bracket content
		bracketContent := ""
		for i := 2; i < len(parts); i++ {
			if bracketContent != "" {
				bracketContent += " "
			}
			bracketContent += parts[i]
			if strings.HasSuffix(parts[i], "]") {
				break
			}
		}

		// Remove brackets
		bracketContent = strings.Trim(bracketContent, "[]")

		// Parse upstream and ahead/behind info
		// Format: "origin/main" or "origin/main: ahead 2" or "origin/main: ahead 2, behind 3"
		if before, after, ok := strings.Cut(bracketContent, ":"); ok {
			branch.Upstream = strings.TrimSpace(before)
			statusPart := after

			// Parse ahead/behind counts
			branch.AheadBy, branch.BehindBy = parseAheadBehindFromStatus(statusPart)
		} else {
			branch.Upstream = bracketContent
		}
	}

	return branch, nil
}

// parseAheadBehindFromStatus parses "ahead 2, behind 3" or "ahead 2" or "behind 3".
func parseAheadBehindFromStatus(status string) (ahead, behind int) {
	status = strings.TrimSpace(status)

	// Parse "ahead N" (defaults to 0 on parse failure)
	if strings.Contains(status, "ahead") {
		_, _ = fmt.Sscanf(extractNumber(status, "ahead"), "%d", &ahead) //nolint:errcheck // default 0 on parse failure is correct behavior
	}

	// Parse "behind N" (defaults to 0 on parse failure)
	if strings.Contains(status, "behind") {
		_, _ = fmt.Sscanf(extractNumber(status, "behind"), "%d", &behind) //nolint:errcheck // default 0 on parse failure is correct behavior
	}

	return ahead, behind
}

// extractNumber extracts the number following a keyword.
func extractNumber(s, keyword string) string {
	_, after, ok := strings.Cut(s, keyword)
	if !ok {
		return "0"
	}

	// Skip the keyword and any spaces
	rest := strings.TrimSpace(after)

	// Extract digits
	var num strings.Builder
	for _, c := range rest {
		if c >= '0' && c <= '9' {
			num.WriteRune(c)
		} else if num.Len() > 0 {
			break
		}
	}

	if num.Len() == 0 {
		return "0"
	}
	return num.String()
}

// validateBranchName validates branch name against Git rules.
func validateBranchName(name string) error {
	if name == "" {
		return fmt.Errorf("branch name cannot be empty")
	}

	// Git branch naming rules
	// See: https://git-scm.com/docs/git-check-ref-format

	// Cannot start with .
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("branch name cannot start with '.'")
	}

	// Cannot end with .lock
	if strings.HasSuffix(name, ".lock") {
		return fmt.Errorf("branch name cannot end with '.lock'")
	}

	// Cannot contain certain characters
	invalidChars := []string{" ", "~", "^", ":", "?", "*", "[", "\\", ".."}
	for _, char := range invalidChars {
		if strings.Contains(name, char) {
			return fmt.Errorf("branch name cannot contain '%s'", char)
		}
	}

	// Cannot start or end with /
	if strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") {
		return fmt.Errorf("branch name cannot start or end with '/'")
	}

	// Cannot contain consecutive slashes
	if strings.Contains(name, "//") {
		return fmt.Errorf("branch name cannot contain consecutive slashes")
	}

	// Must match pattern (alphanumeric, dash, underscore, slash)
	validPattern := regexp.MustCompile(`^[a-zA-Z0-9/_-]+$`)
	if !validPattern.MatchString(name) {
		return fmt.Errorf("branch name must match pattern: [a-zA-Z0-9/_-]+")
	}

	return nil
}
