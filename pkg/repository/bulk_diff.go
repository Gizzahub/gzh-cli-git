// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// BulkDiffOptions configures bulk diff operations.
type BulkDiffOptions struct {
	// Directory is the root directory to scan for repositories
	Directory string

	// Parallel is the number of concurrent workers (default: 10)
	Parallel int

	// MaxDepth is the maximum directory depth to scan (default: 1)
	MaxDepth int

	// Staged shows only staged changes (git diff --cached)
	Staged bool

	// IncludeUntracked includes untracked files in the output
	IncludeUntracked bool

	// ContextLines is the number of context lines around changes (default: 3)
	ContextLines int

	// MaxDiffSize limits the diff size per repository in bytes (default: 100KB)
	MaxDiffSize int

	// IncludePattern is a regex pattern to include repositories
	IncludePattern string

	// ExcludePattern is a regex pattern to exclude repositories
	ExcludePattern string

	// IncludeSubmodules includes git submodules in the scan
	IncludeSubmodules bool

	// Verbose enables detailed logging
	Verbose bool

	// Logger for progress logging
	Logger Logger

	// ProgressCallback is called for each repository processed
	ProgressCallback func(current, total int, repo string)
}

// BulkDiffResult contains the results of a bulk diff operation.
type BulkDiffResult struct {
	// TotalScanned is the number of repositories found
	TotalScanned int

	// TotalWithChanges is the number of repositories with changes
	TotalWithChanges int

	// TotalClean is the number of repositories without changes
	TotalClean int

	// Repositories contains individual repository results
	Repositories []RepositoryDiffResult

	// Duration is the total operation time
	Duration time.Duration

	// Summary contains status counts
	Summary map[string]int
}

// RepositoryDiffResult contains the diff result for a single repository.
type RepositoryDiffResult struct {
	// Path is the repository path
	Path string

	// RelativePath is the path relative to scan root
	RelativePath string

	// Branch is the current branch
	Branch string

	// Status is the operation status (has-changes, clean, error)
	Status string

	// DiffContent is the actual diff output
	DiffContent string

	// DiffSummary is a short summary of changes
	DiffSummary string

	// FilesChanged is the number of tracked files that differ from the scope's
	// base. It equals TrackedFilesChanged and deliberately excludes untracked
	// files, preserving the meaning this key has always had for diff. The full
	// set a commit would record is TrackedFilesChanged + UntrackedFilesChanged.
	FilesChanged int

	// TrackedFilesChanged, UntrackedFilesChanged and StagedFilesChanged name the
	// three counts separately, so a caller can tell why diff and commit report
	// the numbers they do instead of having to guess which set each one means.
	// StagedFilesChanged overlaps the other two rather than partitioning them.
	TrackedFilesChanged   int
	UntrackedFilesChanged int
	StagedFilesChanged    int

	// Scope names the comparison these numbers describe ("head", "staged" or
	// "worktree"). See ChangeScope.
	Scope string

	// Additions is the number of lines added
	Additions int

	// Deletions is the number of lines deleted
	Deletions int

	// ChangedFiles is the list of changed files with their status
	ChangedFiles []ChangedFile

	// UntrackedFiles is the list of untracked files
	UntrackedFiles []string

	// OmittedFiles lists untracked files whose content was left out of
	// DiffContent, with the reason. Never silently empty: every skip in the
	// untracked reader is recorded here.
	OmittedFiles []OmittedFile

	// Truncated indicates if the diff was truncated due to size limits
	Truncated bool

	// Error if the operation failed
	Error error

	// Duration is the operation time for this repository
	Duration time.Duration
}

// GetStatus returns the status for summary calculation.
func (r RepositoryDiffResult) GetStatus() string { return r.Status }

// OmittedFile records an untracked file that was left out of the diff body.
type OmittedFile struct {
	// Path is the repository-relative file path
	Path string

	// Reason is why the content was omitted: "not-regular-file", "too-large",
	// or "read-error"
	Reason string
}

// ChangedFile represents a changed file with its status.
type ChangedFile struct {
	// Path is the file path
	Path string

	// Status is the change status (M=modified, A=added, D=deleted, R=renamed, etc.)
	Status string

	// OldPath is the old path for renamed files
	OldPath string
}

// BulkDiff scans for repositories and gets diffs in parallel.
func (c *client) BulkDiff(ctx context.Context, opts BulkDiffOptions) (*BulkDiffResult, error) {
	startTime := time.Now()

	// Set defaults
	if opts.ContextLines == 0 {
		opts.ContextLines = 3
	}
	if opts.MaxDiffSize == 0 {
		opts.MaxDiffSize = 100 * 1024 // 100KB default
	}

	// Initialize common settings
	common, err := initializeBulkOperation(
		opts.Directory,
		opts.Parallel,
		opts.MaxDepth,
		opts.IncludeSubmodules,
		opts.IncludePattern,
		opts.ExcludePattern,
		opts.Logger,
	)
	if err != nil {
		return nil, err
	}

	// Scan and filter repositories
	filteredRepos, totalScanned, err := c.scanAndFilterRepositories(ctx, common)
	if err != nil {
		return nil, err
	}

	result := &BulkDiffResult{
		TotalScanned: totalScanned,
		Repositories: make([]RepositoryDiffResult, 0, len(filteredRepos)),
		Summary:      make(map[string]int),
	}

	if len(filteredRepos) == 0 {
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Process repositories in parallel
	var mu sync.Mutex
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, common.Parallel)

	for i, repoPath := range filteredRepos {
		wg.Add(1)
		go func(idx int, path string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if opts.ProgressCallback != nil {
				opts.ProgressCallback(idx+1, len(filteredRepos), path)
			}

			repoResult := c.getRepositoryDiff(ctx, common.Directory, path, opts)

			mu.Lock()
			result.Repositories = append(result.Repositories, repoResult)
			switch repoResult.Status {
			case "has-changes":
				result.TotalWithChanges++
			case "clean":
				result.TotalClean++
			}
			mu.Unlock()
		}(i, repoPath)
	}
	wg.Wait()

	result.Duration = time.Since(startTime)
	c.updateDiffSummary(result)

	return result, nil
}

// getRepositoryDiff gets the diff for a single repository.
//
//nolint:gocognit // TODO: Refactor diff logic into smaller functions
func (c *client) getRepositoryDiff(ctx context.Context, rootDir, repoPath string, opts BulkDiffOptions) RepositoryDiffResult {
	startTime := time.Now()

	relPath, err := filepath.Rel(rootDir, repoPath)
	if err != nil {
		relPath = repoPath
	}
	if relPath == "." {
		relPath = filepath.Base(rootDir)
	}

	result := RepositoryDiffResult{
		Path:           repoPath,
		RelativePath:   relPath,
		Status:         "clean",
		ChangedFiles:   []ChangedFile{},
		UntrackedFiles: []string{},
	}

	// Get branch
	branchResult, err := c.executor.Run(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err == nil {
		result.Branch = strings.TrimSpace(branchResult.Stdout)
	}

	// Collect the change set. Default scope is HEAD → worktree (untracked
	// included), which is the set `commit` actually records; --staged narrows it
	// to the index. Previously this path compared the index against the worktree
	// and never said so, which is why a fully staged repository listed files but
	// produced an empty diff body.
	scope := ScopeHead
	if opts.Staged {
		scope = ScopeStagedOnly
	}

	changes, err := c.collectChangeSet(ctx, repoPath, scope)
	if err != nil {
		result.Status = "error"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	for _, entry := range changes.Entries {
		if entry.Untracked {
			result.UntrackedFiles = append(result.UntrackedFiles, entry.Path)
			continue
		}
		result.ChangedFiles = append(result.ChangedFiles, ChangedFile{
			Path:    entry.Path,
			OldPath: entry.OldPath,
			Status:  entry.Status,
		})
	}

	result.Scope = string(scope)
	result.FilesChanged = changes.TrackedCount
	result.TrackedFilesChanged = changes.TrackedCount
	result.UntrackedFilesChanged = changes.UntrackedCount
	result.StagedFilesChanged = changes.StagedCount
	result.Additions = changes.Additions
	result.Deletions = changes.Deletions

	// If no changes, mark as clean
	if len(changes.Entries) == 0 {
		result.Status = "clean"
		result.Duration = time.Since(startTime)
		return result
	}

	result.Status = "has-changes"

	// Get diff content for the same scope the change set was collected in
	diffContent, err := c.runScopedDiff(ctx, repoPath, scope, fmt.Sprintf("--unified=%d", opts.ContextLines))
	if err != nil {
		result.Error = fmt.Errorf("failed to get diff: %w", err)
		result.Duration = time.Since(startTime)
		return result
	}

	result.DiffContent = diffContent

	// Truncate if too large
	if len(result.DiffContent) > opts.MaxDiffSize {
		result.DiffContent = result.DiffContent[:opts.MaxDiffSize]
		result.Truncated = true
	}

	result.DiffSummary = formatDiffSummary(changes)

	// Include untracked file contents if requested
	if opts.IncludeUntracked && len(result.UntrackedFiles) > 0 {
		c.appendUntrackedDiffs(repoPath, &result, opts)
	}

	result.Duration = time.Since(startTime)
	return result
}

// parseGitStatus converts git status codes to readable status.
func parseGitStatus(code string) string {
	// First character is index status, second is worktree status
	indexStatus := code[0]
	worktreeStatus := code[1]

	// Prioritize index status for staged files
	switch indexStatus {
	case 'M':
		return "M" // Modified
	case 'A':
		return "A" // Added
	case 'D':
		return "D" // Deleted
	case 'R':
		return "R" // Renamed
	case 'C':
		return "C" // Copied
	}

	// Fall back to worktree status
	switch worktreeStatus {
	case 'M':
		return "M"
	case 'D':
		return "D"
	}

	return "?" // Unknown
}

// formatDiffSummary renders git's familiar one-line stat summary from exact
// numstat counts, rather than scraping it back out of `--stat` prose. The prose
// form is localized and width-limited, so parsing it was never reliable.
func formatDiffSummary(cs *ChangeSet) string {
	if cs.TrackedCount == 0 && cs.UntrackedCount == 0 {
		return ""
	}

	parts := []string{fmt.Sprintf("%s changed", pluralize(cs.TrackedCount+cs.UntrackedCount, "file"))}
	if cs.Additions > 0 {
		parts = append(parts, pluralize(cs.Additions, "insertion")+"(+)")
	}
	if cs.Deletions > 0 {
		parts = append(parts, pluralize(cs.Deletions, "deletion")+"(-)")
	}

	return strings.Join(parts, ", ")
}

// pluralize renders "1 file" / "2 files" the way git does.
func pluralize(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}

	return fmt.Sprintf("%d %ss", n, noun)
}

// updateDiffSummary updates the summary counts.
func (c *client) updateDiffSummary(result *BulkDiffResult) {
	for _, repo := range result.Repositories {
		result.Summary[repo.Status]++
	}
}
