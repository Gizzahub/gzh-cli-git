package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UpdateStrategy defines how to handle existing repositories during clone-or-update operations
type UpdateStrategy string

const (
	// StrategyRebase rebases local changes on top of remote changes
	StrategyRebase UpdateStrategy = "rebase"
	// StrategyReset performs a hard reset to match remote state (discards local changes)
	StrategyReset UpdateStrategy = "reset"
	// StrategyClone removes existing directory and performs fresh clone
	StrategyClone UpdateStrategy = "clone"
	// StrategySkip leaves the existing repository unchanged
	StrategySkip UpdateStrategy = "skip"
	// StrategyPull performs a standard git pull (merge remote changes)
	StrategyPull UpdateStrategy = "pull"
	// StrategyFetch only fetches remote changes without updating working directory
	StrategyFetch UpdateStrategy = "fetch"
)

// CloneOrUpdateOptions configures the clone-or-update operation
type CloneOrUpdateOptions struct {
	// URL is the repository URL to clone (required)
	URL string

	// Destination is the local path where the repository will be cloned (required)
	Destination string

	// Strategy defines how to handle an existing repository
	// Default: StrategyRebase
	Strategy UpdateStrategy

	// Branch is the branch to check out after cloning
	// If empty, the remote's default branch is used
	Branch string

	// Depth limits the clone depth (number of commits)
	// 0 means full clone, 1 means shallow clone with only the latest commit
	Depth int

	// Force allows destructive operations even when not normally allowed
	Force bool

	// Logger is an optional logger for operation feedback
	Logger Logger

	// Progress is an optional progress reporter
	Progress ProgressReporter
}

// CloneOrUpdateResult contains the result of a clone-or-update operation
type CloneOrUpdateResult struct {
	// Repository is the opened repository (nil if skipped)
	Repository *Repository

	// Action describes what action was taken
	Action string // "cloned", "updated", "skipped", "reset", etc.

	// StrategyUsed is the strategy that was actually used
	StrategyUsed UpdateStrategy

	// Success indicates if the operation succeeded
	Success bool

	// Message contains a human-readable result message
	Message string
}

// CloneOrUpdate clones a repository if it doesn't exist, or updates it using the specified strategy.
// This is a high-level convenience method that intelligently handles existing repositories.
//
// The behavior depends on whether the destination exists:
// - If destination doesn't exist: Clone the repository
// - If destination exists but is not a git repo: Error (unless Strategy is Clone or Force is true)
// - If destination exists and is a git repo: Apply the specified update strategy
//
// Example:
//
//	opts := CloneOrUpdateOptions{
//	    URL: "https://github.com/user/repo.git",
//	    Destination: "/path/to/repo",
//	    Strategy: StrategyRebase,
//	}
//	result, err := client.CloneOrUpdate(ctx, opts)
func (c *client) CloneOrUpdate(ctx context.Context, opts CloneOrUpdateOptions) (*CloneOrUpdateResult, error) {
	// Validate options
	if opts.URL == "" {
		return nil, &ValidationError{
			Field:  "URL",
			Value:  opts.URL,
			Reason: "URL cannot be empty",
		}
	}
	if opts.Destination == "" {
		return nil, &ValidationError{
			Field:  "Destination",
			Value:  opts.Destination,
			Reason: "Destination cannot be empty",
		}
	}

	// Default strategy to rebase
	if opts.Strategy == "" {
		opts.Strategy = StrategyRebase
	}

	// Validate strategy
	if !isValidUpdateStrategy(opts.Strategy) {
		return nil, &ValidationError{
			Field:  "Strategy",
			Value:  string(opts.Strategy),
			Reason: fmt.Sprintf("invalid strategy, must be one of: %s", getValidStrategies()),
		}
	}

	// Use provided logger or noop
	logger := opts.Logger
	if logger == nil {
		logger = &noopLogger{}
	}

	// Resolve absolute path
	absPath, err := filepath.Abs(opts.Destination)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path: %w", err)
	}
	opts.Destination = absPath

	logger.Debug("clone-or-update starting", "url", opts.URL, "destination", opts.Destination, "strategy", opts.Strategy)

	// Check if target directory exists and is a git repository
	exists, isGitRepo, err := checkTargetDirectory(opts.Destination)
	if err != nil {
		return nil, fmt.Errorf("failed to check target directory: %w", err)
	}

	logger.Debug("target directory check", "exists", exists, "isGitRepo", isGitRepo)

	// Decide action based on existence and strategy
	switch {
	case !exists:
		// Directory doesn't exist - perform clone
		logger.Info("target directory does not exist, cloning", "destination", opts.Destination)
		return c.performCloneOperation(ctx, opts, logger)

	case exists && !isGitRepo:
		// Directory exists but is not a git repo
		if opts.Strategy == StrategyClone || opts.Force {
			logger.Info("removing non-git directory and cloning", "destination", opts.Destination)
			if err := os.RemoveAll(opts.Destination); err != nil {
				return nil, fmt.Errorf("failed to remove existing directory: %w", err)
			}
			return c.performCloneOperation(ctx, opts, logger)
		}
		return nil, fmt.Errorf("target directory exists but is not a git repository (use Strategy=Clone or Force=true to replace)")

	case exists && isGitRepo:
		// Directory exists and is a git repo - apply update strategy
		logger.Info("applying update strategy to existing repository", "strategy", opts.Strategy)
		return c.applyUpdateStrategy(ctx, opts, logger)

	default:
		return nil, fmt.Errorf("unexpected state in target directory analysis")
	}
}

// performCloneOperation executes a fresh clone
func (c *client) performCloneOperation(ctx context.Context, opts CloneOrUpdateOptions, logger Logger) (*CloneOrUpdateResult, error) {
	cloneOpts := CloneOptions{
		URL:         opts.URL,
		Destination: opts.Destination,
		Branch:      opts.Branch,
		Depth:       opts.Depth,
		Logger:      logger,
		Progress:    opts.Progress,
	}

	repo, err := c.Clone(ctx, cloneOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	return &CloneOrUpdateResult{
		Repository:   repo,
		Action:       "cloned",
		StrategyUsed: opts.Strategy,
		Success:      true,
		Message:      fmt.Sprintf("Successfully cloned %s to %s", opts.URL, opts.Destination),
	}, nil
}

// applyUpdateStrategy applies the specified update strategy to an existing repository
func (c *client) applyUpdateStrategy(ctx context.Context, opts CloneOrUpdateOptions, logger Logger) (*CloneOrUpdateResult, error) {
	switch opts.Strategy {
	case StrategySkip:
		logger.Info("skipping existing repository", "destination", opts.Destination)
		return &CloneOrUpdateResult{
			Repository:   nil,
			Action:       "skipped",
			StrategyUsed: StrategySkip,
			Success:      true,
			Message:      fmt.Sprintf("Skipped existing repository at %s", opts.Destination),
		}, nil

	case StrategyClone:
		// Remove and re-clone
		logger.Info("removing existing repository and cloning fresh", "destination", opts.Destination)
		if err := os.RemoveAll(opts.Destination); err != nil {
			return nil, fmt.Errorf("failed to remove existing repository: %w", err)
		}
		return c.performCloneOperation(ctx, opts, logger)

	case StrategyFetch:
		// Only fetch remote changes
		return c.applyFetchStrategy(ctx, opts, logger)

	case StrategyPull:
		// Standard git pull (merge)
		return c.applyPullStrategy(ctx, opts, logger)

	case StrategyReset:
		// Hard reset to remote
		return c.applyResetStrategy(ctx, opts, logger)

	case StrategyRebase:
		// Rebase local changes on remote
		return c.applyRebaseStrategy(ctx, opts, logger)

	default:
		return nil, fmt.Errorf("unsupported update strategy: %s", opts.Strategy)
	}
}

// applyFetchStrategy fetches remote changes without updating working directory
func (c *client) applyFetchStrategy(ctx context.Context, opts CloneOrUpdateOptions, logger Logger) (*CloneOrUpdateResult, error) {
	args := []string{"fetch", "origin"}
	if opts.Branch != "" {
		args = append(args, opts.Branch)
	}

	result, err := c.executor.Run(ctx, opts.Destination, args...)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("fetch failed: %s", result.Error)
	}

	repo, err := c.Open(ctx, opts.Destination)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository after fetch: %w", err)
	}

	return &CloneOrUpdateResult{
		Repository:   repo,
		Action:       "fetched",
		StrategyUsed: StrategyFetch,
		Success:      true,
		Message:      fmt.Sprintf("Successfully fetched updates for %s", opts.Destination),
	}, nil
}

// applyPullStrategy performs a standard git pull (merge)
func (c *client) applyPullStrategy(ctx context.Context, opts CloneOrUpdateOptions, logger Logger) (*CloneOrUpdateResult, error) {
	args := []string{"pull", "origin"}
	if opts.Branch != "" {
		args = append(args, opts.Branch)
	}

	result, err := c.executor.Run(ctx, opts.Destination, args...)
	if err != nil {
		return nil, fmt.Errorf("pull failed: %w", err)
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("pull failed: %s", result.Error)
	}

	repo, err := c.Open(ctx, opts.Destination)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository after pull: %w", err)
	}

	return &CloneOrUpdateResult{
		Repository:   repo,
		Action:       "pulled",
		StrategyUsed: StrategyPull,
		Success:      true,
		Message:      fmt.Sprintf("Successfully pulled updates for %s", opts.Destination),
	}, nil
}

// applyResetStrategy performs a hard reset to match remote state
func (c *client) applyResetStrategy(ctx context.Context, opts CloneOrUpdateOptions, logger Logger) (*CloneOrUpdateResult, error) {
	// First fetch to get latest remote state
	fetchResult, err := c.executor.Run(ctx, opts.Destination, "fetch", "origin")
	if err != nil {
		return nil, fmt.Errorf("fetch before reset failed: %w", err)
	}
	if fetchResult.ExitCode != 0 {
		return nil, fmt.Errorf("fetch before reset failed: %s", fetchResult.Error)
	}

	// Determine reset target
	resetTarget := "origin/HEAD"
	if opts.Branch != "" {
		resetTarget = fmt.Sprintf("origin/%s", opts.Branch)
	}

	// Hard reset to remote
	resetResult, err := c.executor.Run(ctx, opts.Destination, "reset", "--hard", resetTarget)
	if err != nil {
		return nil, fmt.Errorf("reset failed: %w", err)
	}
	if resetResult.ExitCode != 0 {
		return nil, fmt.Errorf("reset failed: %s", resetResult.Error)
	}

	repo, err := c.Open(ctx, opts.Destination)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository after reset: %w", err)
	}

	return &CloneOrUpdateResult{
		Repository:   repo,
		Action:       "reset",
		StrategyUsed: StrategyReset,
		Success:      true,
		Message:      fmt.Sprintf("Successfully reset %s to %s", opts.Destination, resetTarget),
	}, nil
}

// applyRebaseStrategy rebases local changes on top of remote changes
func (c *client) applyRebaseStrategy(ctx context.Context, opts CloneOrUpdateOptions, logger Logger) (*CloneOrUpdateResult, error) {
	// Fetch latest changes
	fetchResult, err := c.executor.Run(ctx, opts.Destination, "fetch", "origin")
	if err != nil {
		return nil, fmt.Errorf("fetch before rebase failed: %w", err)
	}
	if fetchResult.ExitCode != 0 {
		return nil, fmt.Errorf("fetch before rebase failed: %s", fetchResult.Error)
	}

	// Pull with rebase
	args := []string{"pull", "--rebase", "origin"}
	if opts.Branch != "" {
		args = append(args, opts.Branch)
	}

	rebaseResult, err := c.executor.Run(ctx, opts.Destination, args...)
	if err != nil {
		return nil, fmt.Errorf("rebase failed: %w", err)
	}
	if rebaseResult.ExitCode != 0 {
		return nil, fmt.Errorf("rebase failed: %s", rebaseResult.Error)
	}

	repo, err := c.Open(ctx, opts.Destination)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository after rebase: %w", err)
	}

	return &CloneOrUpdateResult{
		Repository:   repo,
		Action:       "rebased",
		StrategyUsed: StrategyRebase,
		Success:      true,
		Message:      fmt.Sprintf("Successfully rebased %s", opts.Destination),
	}, nil
}

// checkTargetDirectory checks if target directory exists and is a git repository
func checkTargetDirectory(path string) (exists bool, isGitRepo bool, err error) {
	// Check if directory exists
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if !info.IsDir() {
		return false, false, fmt.Errorf("target path exists but is not a directory")
	}

	// Check if it's a git repository
	gitDir := filepath.Join(path, ".git")
	_, err = os.Stat(gitDir)
	if os.IsNotExist(err) {
		return true, false, nil
	}
	if err != nil {
		return true, false, err
	}

	return true, true, nil
}

// isValidUpdateStrategy validates if the strategy is supported
func isValidUpdateStrategy(strategy UpdateStrategy) bool {
	switch strategy {
	case StrategyRebase, StrategyReset, StrategyClone, StrategySkip, StrategyPull, StrategyFetch:
		return true
	default:
		return false
	}
}

// getValidStrategies returns a comma-separated list of valid strategies
func getValidStrategies() string {
	return "rebase, reset, clone, skip, pull, fetch"
}

// ExtractRepoNameFromURL extracts the repository name from a Git URL
// This is useful for determining a default destination directory name
//
// Supports various URL formats:
// - https://github.com/user/repo.git → repo
// - git@github.com:user/repo.git → repo
// - ssh://git@server.com/user/repo.git → repo
//
// Example:
//
//	name, err := ExtractRepoNameFromURL("https://github.com/user/my-repo.git")
//	// name == "my-repo"
func ExtractRepoNameFromURL(repoURL string) (string, error) {
	if repoURL == "" {
		return "", fmt.Errorf("repository URL cannot be empty")
	}

	// Remove common Git URL prefixes and suffixes
	url := strings.TrimSpace(repoURL)

	// Remove .git suffix if present
	if strings.HasSuffix(url, ".git") {
		url = strings.TrimSuffix(url, ".git")
	}

	var repoPath string

	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		// HTTP/HTTPS URLs: https://github.com/user/repo
		parts := strings.Split(url, "/")
		if len(parts) < 2 {
			return "", fmt.Errorf("invalid HTTP/HTTPS URL format: %s", repoURL)
		}
		repoPath = parts[len(parts)-1]
	} else if strings.Contains(url, "@") && strings.Contains(url, ":") {
		// SSH URLs: git@github.com:user/repo
		if strings.HasPrefix(url, "ssh://") {
			// ssh://git@server.com/user/repo
			parts := strings.Split(url, "/")
			if len(parts) < 2 {
				return "", fmt.Errorf("invalid SSH URL format: %s", repoURL)
			}
			repoPath = parts[len(parts)-1]
		} else {
			// git@github.com:user/repo
			colonIndex := strings.LastIndex(url, ":")
			if colonIndex == -1 {
				return "", fmt.Errorf("invalid SSH URL format: %s", repoURL)
			}
			pathPart := url[colonIndex+1:]
			parts := strings.Split(pathPart, "/")
			repoPath = parts[len(parts)-1]
		}
	} else {
		// Fallback: try to extract from the last part of the path
		parts := strings.Split(url, "/")
		if len(parts) < 1 {
			return "", fmt.Errorf("unable to extract repository name from URL: %s", repoURL)
		}
		repoPath = parts[len(parts)-1]
	}

	// Clean up the repository name
	repoPath = strings.TrimSpace(repoPath)
	if repoPath == "" {
		return "", fmt.Errorf("extracted repository name is empty from URL: %s", repoURL)
	}

	// Validate repository name (basic validation)
	if strings.Contains(repoPath, " ") || strings.Contains(repoPath, "\t") {
		return "", fmt.Errorf("invalid repository name extracted: %s", repoPath)
	}

	return repoPath, nil
}
