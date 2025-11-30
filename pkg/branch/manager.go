// Package branch provides Git branch management operations.
// This package includes branch creation, deletion, listing, and worktree management.
//
// Example usage:
//
//	mgr := branch.NewManager()
//	err := mgr.Create(ctx, repo, branch.CreateOptions{
//	    Name:     "feature/new-feature",
//	    StartRef: "main",
//	    Checkout: true,
//	})
package branch

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/gizzahub/gzh-cli-git/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-git/pkg/repository"
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
			return fmt.Errorf("%w: %v", ErrInvalidName, err)
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

	// Verify start ref exists
	if _, err := m.executor.Run(ctx, repo.Path, "rev-parse", "--verify", startRef); err != nil {
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
	if _, err := m.executor.Run(ctx, repo.Path, args...); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	// Checkout if requested
	if opts.Checkout {
		if _, err := m.executor.Run(ctx, repo.Path, "checkout", opts.Name); err != nil {
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

	if _, err := m.executor.Run(ctx, repo.Path, "branch", deleteFlag, opts.Name); err != nil {
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

				if _, err := m.executor.Run(ctx, repo.Path, "push", remote, "--delete", remoteBranch); err != nil {
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

	// Run command
	result, err := m.executor.Run(ctx, repo.Path, args...)
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

	// Get current branch name
	result, err := m.executor.Run(ctx, repo.Path, "rev-parse", "--abbrev-ref", "HEAD")
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

	// Try to get branch ref
	_, err := m.executor.Run(ctx, repo.Path, "rev-parse", "--verify", fmt.Sprintf("refs/heads/%s", name))
	if err != nil {
		// Branch doesn't exist
		return false, nil
	}

	return true, nil
}

// parseBranchList parses git branch -vv output.
func (m *manager) parseBranchList(output string) ([]*Branch, error) {
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
		upstream := parts[2]
		if strings.HasSuffix(upstream, "]") {
			branch.Upstream = strings.Trim(upstream, "[]")
		} else {
			// Upstream might span multiple parts
			for i := 3; i < len(parts); i++ {
				upstream += " " + parts[i]
				if strings.HasSuffix(parts[i], "]") {
					branch.Upstream = strings.Trim(upstream, "[]")
					break
				}
			}
		}
	}

	return branch, nil
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
