// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
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
//
//nolint:gocognit,gocyclo // TODO: Refactor clone logic into smaller functions
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

	// Option-injection defense: the URL and branch are external (forge API,
	// config, or CLI) and flow to git as positional/flag values. Reject values
	// that git could parse as options (e.g. --upload-pack=…) before use.
	if err := gitcmd.SanitizeURL(opts.URL); err != nil {
		return nil, &ValidationError{Field: "URL", Value: opts.URL, Reason: err.Error()}
	}
	if opts.Branch != "" {
		if err := gitcmd.SanitizeBranchName(opts.Branch); err != nil {
			return nil, &ValidationError{Field: "Branch", Value: opts.Branch, Reason: err.Error()}
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

	if opts.Recursive {
		args = append(args, "--recurse-submodules")
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

	// Execute clone command with environment variables (for auth)
	result, err := c.executor.RunWithEnv(ctx, "", opts.Env, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute clone command: %w", err)
	}

	if result.ExitCode != 0 {
		// Check if the error is due to branch not found
		branchNotFound := strings.Contains(result.Stderr, "does not have any commits yet") ||
			strings.Contains(result.Stderr, "Remote branch") && strings.Contains(result.Stderr, "not found") ||
			strings.Contains(result.Stderr, "리모트의") && strings.Contains(result.Stderr, "브랜치가") && strings.Contains(result.Stderr, "없습니다")

		if !branchNotFound || opts.Branch == "" {
			// Other git error
			return nil, &gitcmd.GitError{
				Command:  "git " + strings.Join(args, " "),
				ExitCode: result.ExitCode,
				Stderr:   result.Stderr,
			}
		}
		// Branch doesn't exist
		if !opts.CreateBranch {
			// Provide clear error message
			return nil, fmt.Errorf("branch '%s' does not exist on remote repository. Use --create-branch flag to create it after cloning", opts.Branch)
		}
		c.logger.Info("Branch %s not found, will create it after cloning default branch", opts.Branch)
		// Clone without branch specification (will use default branch)
		argsWithoutBranch := []string{"clone"}
		if opts.Depth > 0 {
			argsWithoutBranch = append(argsWithoutBranch, "--depth", fmt.Sprintf("%d", opts.Depth))
		}
		if opts.Recursive {
			argsWithoutBranch = append(argsWithoutBranch, "--recurse-submodules")
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

		result, err = c.executor.RunWithEnv(ctx, "", opts.Env, argsWithoutBranch...)
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
//
//nolint:gocognit // GetInfo collects many git fields in one pass; splitting would require multiple round-trips
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

	// Get all remotes
	info.Remotes = make(map[string]string)
	output, err = c.executor.RunOutput(ctx, repo.Path, "remote", "-v")
	if err != nil {
		c.logger.Debug("Failed to get configured remotes: %v", err)
	} else {
		lines := strings.SplitSeq(output, "\n")
		for line := range lines {
			// Format: name\turl (purpose)
			// Example: origin  https://github.com/user/repo.git (fetch)
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				name := parts[0]
				url := parts[1]
				info.Remotes[name] = url
			}
		}
	}

	// Set primary remote URL (prefer origin, fallback to random first)
	info.Remote = "origin"
	if url, ok := info.Remotes["origin"]; ok {
		info.RemoteURL = url
	} else if len(info.Remotes) > 0 {
		for name, url := range info.Remotes {
			info.Remote = name
			info.RemoteURL = url
			break
		}
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

	// Get additional details (HeadSHA, Describe, LastCommit)
	// Consolidate into one command interaction if possible? git log -1 --format is useful
	// Format: %h|%s|%cr|%an
	output, err = c.executor.RunOutput(ctx, repo.Path, "log", "-1", "--format=%h|%s|%cr|%an")
	if err == nil {
		parts := strings.Split(strings.TrimSpace(output), "|")
		if len(parts) >= 4 {
			info.HeadSHA = parts[0]
			info.LastCommitMsg = parts[1]
			info.LastCommitDate = parts[2]
			info.LastCommitAuthor = parts[3]
		}
	} else {
		// Try just getting hash if log fails (e.g. empty repo)
		output, err = c.executor.RunOutput(ctx, repo.Path, "rev-parse", "--short", "HEAD")
		if err == nil {
			info.HeadSHA = strings.TrimSpace(output)
		}
	}

	// Get Describe (version)
	output, err = c.executor.RunOutput(ctx, repo.Path, "describe", "--tags", "--always", "--dirty")
	if err == nil {
		info.Describe = strings.TrimSpace(output)
	}

	// Read full ref names: refname:short becomes ambiguous when a local branch
	// is named like a remote-tracking branch (for example origin/develop).
	output, err = c.executor.RunOutput(ctx, repo.Path, "for-each-ref", "--format=%(refname)", "refs/heads")
	if err == nil {
		info.LocalBranches = parseLocalBranches(output)
	}

	// Remote-tracking refs are intentionally collected separately from local
	// branches. A remote's symbolic HEAD (for example origin/HEAD) is not a
	// branch a user can check out, so %(symref) lets us leave it out here.
	// The final literal keeps RunOutput's whitespace trimming from consuming an
	// empty symref field on the last ordinary ref.
	output, err = c.executor.RunOutput(ctx, repo.Path, "for-each-ref", "--format=%(refname)%09%(symref)%09x", "refs/remotes")
	if err == nil {
		info.RemoteBranches = parseRemoteBranches(output)
	} else {
		// Remote refs are optional display metadata, like describe and stash
		// facts above. Keep GetInfo best-effort, but make a probe failure visible
		// in debug logs rather than indistinguishable from an empty result there.
		c.logger.Debug("Failed to get remote-tracking branches: %v", err)
	}

	// Get stash count and the age of the oldest entry. The age is what separates
	// "I am mid-task" from work that has been invisible to every other machine
	// for weeks, and git offers no cheaper way to ask than reading the dates.
	output, err = c.executor.RunOutput(ctx, repo.Path, "stash", "list", "--format=%ct")
	if err == nil {
		info.StashCount, info.OldestStash = ParseStashDates(output)
	}

	c.logger.Info("Retrieved repository info for %s", repo.Path)

	return info, nil
}

// parseRemoteBranches accepts for-each-ref's ref, symref, marker records.
// Symbolic refs (origin/HEAD) have a non-empty symref and are not branches.
func parseRemoteBranches(output string) []string {
	branches := make([]string, 0)
	for line := range strings.SplitSeq(output, "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 || parts[1] != "" || parts[2] != "x" {
			continue
		}
		if ref, ok := strings.CutPrefix(parts[0], "refs/remotes/"); ok && ref != "" {
			branches = append(branches, ref)
		}
	}
	sort.Strings(branches)
	return branches
}

func parseLocalBranches(output string) []string {
	branches := make([]string, 0)
	for line := range strings.SplitSeq(output, "\n") {
		if name, ok := strings.CutPrefix(line, "refs/heads/"); ok && name != "" {
			branches = append(branches, name)
		}
	}
	sort.Strings(branches)
	return branches
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

	// -z -uall, and runGit rather than RunOutput: see porcelain.Parse for why all
	// three matter. RunOutput trims its result, which used to strip the leading
	// space off the first record and reclassify an unstaged edit as a staged one.
	output, err := c.runGit(ctx, repo.Path, "status", "--porcelain", "-z", "-uall")
	if err != nil {
		return nil, fmt.Errorf("failed to get repository status: %w", err)
	}

	// Parse status output
	status, err := parseStatusZ(output)
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
	_, _ = fmt.Sscanf(parts[0], "%d", &ahead)  //nolint:errcheck // Sscanf on a known numeric string is best-effort; zero value on failure is acceptable
	_, _ = fmt.Sscanf(parts[1], "%d", &behind) //nolint:errcheck // Sscanf on a known numeric string is best-effort; zero value on failure is acceptable

	return ahead, behind, nil
}

// Porcelain status parsing is split across two places: internal/porcelain owns
// the wire format, shared with the packages outside this one that read the same
// output, and porcelain.go here projects records onto Status. collectChangeSet
// and checkRepositoryState go through the same pair.
