package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gizzahub/gzh-cli-git/internal/gitcmd"
)

// client implements the Client interface.
// It wraps the Git CLI executor and provides high-level repository operations.
type client struct {
	executor *gitcmd.Executor
	logger   Logger
}

// NewClient creates a new repository client with the given options.
// The client provides access to all repository operations defined in the Client interface.
//
// Example:
//
//	client := repository.NewClient(
//	    repository.WithLogger(myLogger),
//	    repository.WithTimeout(30 * time.Second),
//	)
func NewClient(opts ...ClientOption) Client {
	c := &client{
		executor: gitcmd.NewExecutor(),
		logger:   &noopLogger{},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// ClientOption configures a Client.
type ClientOption func(*client)

// WithClientLogger sets a custom logger for the client.
func WithClientLogger(logger Logger) ClientOption {
	return func(c *client) {
		if logger != nil {
			c.logger = logger
		}
	}
}

// WithExecutor sets a custom Git executor for the client.
// This is primarily useful for testing with a mock executor.
func WithExecutor(executor *gitcmd.Executor) ClientOption {
	return func(c *client) {
		if executor != nil {
			c.executor = executor
		}
	}
}

// Open opens an existing Git repository at the specified path.
// Returns an error if the path is not a valid Git repository.
//
// Example:
//
//	repo, err := client.Open(ctx, "/path/to/repo")
//	if err != nil {
//	    log.Fatal(err)
//	}
func (c *client) Open(ctx context.Context, path string) (*Repository, error) {
	c.logger.Debug("Opening repository at %s", path)

	// Validate path
	if path == "" {
		return nil, &ValidationError{
			Field:  "path",
			Value:  path,
			Reason: "path cannot be empty",
		}
	}

	// Check if path exists
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	if _, err := os.Stat(absPath); err != nil {
		return nil, fmt.Errorf("path does not exist: %w", err)
	}

	// Check if it's a Git repository
	if !c.executor.IsGitRepository(ctx, absPath) {
		return nil, fmt.Errorf("not a Git repository: %s", absPath)
	}

	c.logger.Info("Opened repository at %s", absPath)

	return &Repository{
		Path: absPath,
	}, nil
}

// Clone clones a Git repository from the specified URL.
// The repository is cloned into the directory specified in opts.Destination.
//
// Example:
//
//	repo, err := client.Clone(ctx, repository.CloneOptions{
//	    URL:         "https://github.com/user/repo.git",
//	    Destination: "/path/to/clone",
//	    Options: []repository.CloneOption{
//	        repository.WithBranch("main"),
//	        repository.WithDepth(1),
//	    },
//	})
func (c *client) Clone(ctx context.Context, opts CloneOptions) (*Repository, error) {
	c.logger.Debug("Cloning repository from %s to %s", opts.URL, opts.Destination)

	// Validate options
	if opts.URL == "" {
		return nil, &ValidationError{
			Field:  "URL",
			Value:  opts.URL,
			Reason: "URL is required",
		}
	}
	if opts.Destination == "" {
		return nil, &ValidationError{
			Field:  "Destination",
			Value:  opts.Destination,
			Reason: "Destination is required",
		}
	}

	// Build Git clone command arguments
	args := []string{"clone"}

	if opts.Branch != "" {
		args = append(args, "--branch", opts.Branch)
	}

	if opts.Depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", opts.Depth))
	}

	if opts.SingleBranch {
		args = append(args, "--single-branch")
	}

	if opts.Bare {
		args = append(args, "--bare")
	}

	if opts.Mirror {
		args = append(args, "--mirror")
	}

	if opts.Quiet {
		args = append(args, "--quiet")
	}

	args = append(args, opts.URL, opts.Destination)

	// Execute clone command
	result, err := c.executor.Run(ctx, "", args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute clone command: %w", err)
	}

	if result.ExitCode != 0 {
		// Check if the error is due to branch not found
		branchNotFound := strings.Contains(result.Stderr, "does not have any commits yet") ||
			strings.Contains(result.Stderr, "Remote branch") && strings.Contains(result.Stderr, "not found") ||
			strings.Contains(result.Stderr, "리모트의") && strings.Contains(result.Stderr, "브랜치가") && strings.Contains(result.Stderr, "없습니다")

		if branchNotFound && opts.Branch != "" {
			// Branch doesn't exist
			if opts.CreateBranch {
				c.logger.Info("Branch %s not found, will create it after cloning default branch", opts.Branch)
				// Clone without branch specification (will use default branch)
				argsWithoutBranch := []string{"clone"}
				if opts.Depth > 0 {
					argsWithoutBranch = append(argsWithoutBranch, "--depth", fmt.Sprintf("%d", opts.Depth))
				}
				if opts.Bare {
					argsWithoutBranch = append(argsWithoutBranch, "--bare")
				}
				if opts.Mirror {
					argsWithoutBranch = append(argsWithoutBranch, "--mirror")
				}
				if opts.Quiet {
					argsWithoutBranch = append(argsWithoutBranch, "--quiet")
				}
				argsWithoutBranch = append(argsWithoutBranch, opts.URL, opts.Destination)

				result, err = c.executor.Run(ctx, "", argsWithoutBranch...)
				if err != nil {
					return nil, fmt.Errorf("failed to clone repository without branch: %w", err)
				}
				if result.ExitCode != 0 {
					return nil, &gitcmd.GitError{
						Command:  "git " + strings.Join(argsWithoutBranch, " "),
						ExitCode: result.ExitCode,
						Stderr:   result.Stderr,
					}
				}

				// Now create and checkout the new branch
				c.logger.Info("Creating new branch %s", opts.Branch)
				checkoutResult, err := c.executor.Run(ctx, opts.Destination, "checkout", "-b", opts.Branch)
				if err != nil {
					return nil, fmt.Errorf("failed to create branch %s: %w", opts.Branch, err)
				}
				if checkoutResult.ExitCode != 0 {
					return nil, fmt.Errorf("failed to create branch %s: %s", opts.Branch, checkoutResult.Stderr)
				}
			} else {
				// Provide clear error message
				return nil, fmt.Errorf("branch '%s' does not exist on remote repository. Use --create-branch flag to create it after cloning", opts.Branch)
			}
		} else {
			// Other git error
			return nil, &gitcmd.GitError{
				Command:  "git " + strings.Join(args, " "),
				ExitCode: result.ExitCode,
				Stderr:   result.Stderr,
			}
		}
	}

	// Report progress if available
	if opts.Progress != nil {
		opts.Progress.Done()
	}

	c.logger.Info("Cloned repository from %s to %s", opts.URL, opts.Destination)

	// Open the cloned repository
	return c.Open(ctx, opts.Destination)
}

// IsRepository checks if the specified path is a valid Git repository.
// This is a lightweight check that only verifies the repository structure.
//
// Example:
//
//	if client.IsRepository(ctx, "/path/to/repo") {
//	    fmt.Println("Valid Git repository")
//	}
func (c *client) IsRepository(ctx context.Context, path string) bool {
	if path == "" {
		return false
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		c.logger.Debug("Failed to resolve path: %v", err)
		return false
	}

	return c.executor.IsGitRepository(ctx, absPath)
}

// GetInfo retrieves information about the repository.
// This includes the current branch, remote URL, and upstream tracking information.
//
// Example:
//
//	info, err := client.GetInfo(ctx, repo)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Branch: %s\n", info.CurrentBranch)
func (c *client) GetInfo(ctx context.Context, repo *Repository) (*Info, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository cannot be nil")
	}

	c.logger.Debug("Getting repository info for %s", repo.Path)

	info := &Info{}

	// Get current branch
	output, err := c.executor.RunOutput(ctx, repo.Path, "branch", "--show-current")
	if err != nil {
		// Not an error if in detached HEAD state
		c.logger.Debug("Failed to get current branch: %v", err)
	} else {
		info.Branch = strings.TrimSpace(output)
	}

	// Get remote URL (default to "origin")
	output, err = c.executor.RunOutput(ctx, repo.Path, "remote", "get-url", "origin")
	if err != nil {
		c.logger.Debug("Failed to get remote URL: %v", err)
	} else {
		info.RemoteURL = strings.TrimSpace(output)
	}

	// Get upstream branch
	output, err = c.executor.RunOutput(ctx, repo.Path, "rev-parse", "--abbrev-ref", "@{upstream}")
	if err != nil {
		c.logger.Debug("Failed to get upstream branch: %v", err)
	} else {
		info.Upstream = strings.TrimSpace(output)
	}

	// Get ahead/behind counts
	if info.Upstream != "" {
		output, err = c.executor.RunOutput(ctx, repo.Path, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
		if err != nil {
			c.logger.Debug("Failed to get ahead/behind counts: %v", err)
		} else {
			ahead, behind, err := parseAheadBehind(output)
			if err != nil {
				c.logger.Warn("Failed to parse ahead/behind counts: %v", err)
			} else {
				info.AheadBy = ahead
				info.BehindBy = behind
			}
		}
	}

	c.logger.Info("Retrieved repository info for %s", repo.Path)

	return info, nil
}

// GetStatus retrieves the current status of the repository.
// This includes information about modified, staged, and untracked files.
//
// Example:
//
//	status, err := client.GetStatus(ctx, repo)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if status.IsClean {
//	    fmt.Println("Working tree is clean")
//	}
func (c *client) GetStatus(ctx context.Context, repo *Repository) (*Status, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository cannot be nil")
	}

	c.logger.Debug("Getting repository status for %s", repo.Path)

	// Execute git status --porcelain
	output, err := c.executor.RunOutput(ctx, repo.Path, "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("failed to get repository status: %w", err)
	}

	// Parse status output
	status, err := parseStatus(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse status output: %w", err)
	}

	c.logger.Info("Retrieved repository status for %s (clean: %v)", repo.Path, status.IsClean)

	return status, nil
}

// parseAheadBehind parses the output of "git rev-list --left-right --count HEAD...@{upstream}".
// Format: "AHEAD\tBEHIND"
// Example: "2\t3" means 2 commits ahead, 3 commits behind.
func parseAheadBehind(output string) (ahead, behind int, err error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return 0, 0, nil
	}

	parts := strings.Split(output, "\t")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid ahead-behind format: %s", output)
	}

	// Simple integer parsing (ignoring errors returns 0)
	fmt.Sscanf(parts[0], "%d", &ahead)
	fmt.Sscanf(parts[1], "%d", &behind)

	return ahead, behind, nil
}

// parseStatus parses the output of "git status --porcelain".
// The porcelain format is designed to be easy for scripts to parse.
//
// Format:
// XY PATH
// where X = index status, Y = worktree status
//
// Status codes:
// ' ' = unmodified
// M = modified
// A = added
// D = deleted
// R = renamed
// C = copied
// U = updated but unmerged
// ? = untracked
// ! = ignored
func parseStatus(output string) (*Status, error) {
	status := &Status{
		IsClean:        true,
		ModifiedFiles:  []string{},
		StagedFiles:    []string{},
		UntrackedFiles: []string{},
		ConflictFiles:  []string{},
		DeletedFiles:   []string{},
		RenamedFiles:   []RenamedFile{},
	}

	if output == "" {
		// Empty output means clean working tree
		return status, nil
	}

	lines := strings.Split(output, "\n")
	for i, line := range lines {
		// Don't trim the line itself as git status --porcelain has specific format
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Minimum length: "XY PATH" = 3 characters + space + path
		if len(line) < 4 {
			return nil, fmt.Errorf("line %d too short for status format: %q", i, line)
		}

		indexStatus := rune(line[0])
		worktreeStatus := rune(line[1])
		filePath := strings.TrimSpace(line[3:])

		// Handle renamed files (format: "old -> new")
		if indexStatus == 'R' || worktreeStatus == 'R' {
			parts := strings.Split(filePath, " -> ")
			if len(parts) == 2 {
				status.RenamedFiles = append(status.RenamedFiles, RenamedFile{
					OldPath: strings.TrimSpace(parts[0]),
					NewPath: strings.TrimSpace(parts[1]),
				})
				status.StagedFiles = append(status.StagedFiles, parts[1])
				status.IsClean = false
				continue
			}
		}

		// Parse status codes
		if err := parseStatusCode(status, indexStatus, worktreeStatus, filePath); err != nil {
			return nil, fmt.Errorf("line %d: %w (content: %q)", i, err, line)
		}
	}

	return status, nil
}

// parseStatusCode interprets the two-character status code.
func parseStatusCode(status *Status, index, worktree rune, path string) error {
	// Index status (staged changes)
	switch index {
	case 'M': // Modified in index
		status.StagedFiles = append(status.StagedFiles, path)
		status.IsClean = false
	case 'A': // Added to index
		status.StagedFiles = append(status.StagedFiles, path)
		status.IsClean = false
	case 'D': // Deleted from index
		status.StagedFiles = append(status.StagedFiles, path)
		status.DeletedFiles = append(status.DeletedFiles, path)
		status.IsClean = false
	case 'R': // Renamed in index
		status.StagedFiles = append(status.StagedFiles, path)
		status.IsClean = false
	case 'C': // Copied in index
		status.StagedFiles = append(status.StagedFiles, path)
		status.IsClean = false
	case 'U': // Unmerged (conflict)
		status.ConflictFiles = append(status.ConflictFiles, path)
		status.IsClean = false
	case '?': // Untracked
		status.UntrackedFiles = append(status.UntrackedFiles, path)
		status.IsClean = false
	case '!': // Ignored
		// We typically don't track ignored files in status
	case ' ': // Unchanged in index
		// No action needed for index
	default:
		return fmt.Errorf("unknown index status code: %c", index)
	}

	// Worktree status (unstaged changes)
	switch worktree {
	case 'M': // Modified in worktree
		status.ModifiedFiles = append(status.ModifiedFiles, path)
		status.IsClean = false
	case 'D': // Deleted from worktree
		status.DeletedFiles = append(status.DeletedFiles, path)
		status.IsClean = false
	case 'U': // Unmerged (conflict)
		status.ConflictFiles = append(status.ConflictFiles, path)
		status.IsClean = false
	case '?': // Untracked (second character for untracked files)
		// Already handled by index status
	case ' ': // Unchanged in worktree
		// No action needed
	default:
		// Some status codes only appear in index, not worktree
		if worktree != 'A' && worktree != 'R' && worktree != 'C' {
			return fmt.Errorf("unknown worktree status code: %c", worktree)
		}
	}

	return nil
}
