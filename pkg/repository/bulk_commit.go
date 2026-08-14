// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// BulkCommitOptions configures bulk commit operations.
type BulkCommitOptions struct {
	// Directory is the root directory to scan for repositories
	Directory string

	// Parallel is the number of concurrent workers (default: 10)
	Parallel int

	// MaxDepth is the maximum directory depth to scan (default: 1)
	MaxDepth int

	// DryRun shows what would be committed without actually committing
	DryRun bool

	// Message is a common message for all repositories (overrides auto-generation)
	Message string

	// Yes auto-approves without confirmation
	Yes bool

	// IncludePattern is a regex pattern to include repositories
	IncludePattern string

	// ExcludePattern is a regex pattern to exclude repositories
	ExcludePattern string

	// IncludeSubmodules includes git submodules in the scan
	IncludeSubmodules bool

	// AllowConflicted commits repositories that still have unmerged paths.
	// Off by default: `git add -A` marks conflicts as resolved, so committing
	// them writes conflict markers into history irreversibly.
	AllowConflicted bool

	// Verbose enables detailed logging
	Verbose bool

	// Logger for progress logging
	Logger Logger

	// ProgressCallback is called for each repository processed
	ProgressCallback func(current, total int, repo string)

	// MessageGenerator generates commit messages for repositories
	// If nil, a simple default message is generated
	MessageGenerator func(ctx context.Context, repoPath string, files []string) (string, error)
}

// BulkCommitResult contains the results of a bulk commit operation.
type BulkCommitResult struct {
	// TotalScanned is the number of repositories found
	TotalScanned int

	// TotalDirty is the number of repositories with uncommitted changes
	TotalDirty int

	// TotalCommitted is the number of repositories successfully committed
	TotalCommitted int

	// TotalFailed is the number of repositories that failed to commit
	TotalFailed int

	// TotalSkipped is the number of repositories skipped (clean or excluded)
	TotalSkipped int

	// TotalConflicted is the number of repositories left uncommitted because
	// they still have unmerged paths
	TotalConflicted int

	// Repositories contains individual repository results
	Repositories []RepositoryCommitResult

	// Duration is the total operation time
	Duration time.Duration

	// Summary contains status counts
	Summary map[string]int
}

// RepositoryCommitResult contains the result for a single repository commit.
type RepositoryCommitResult struct {
	// Path is the repository path
	Path string

	// RelativePath is the path relative to scan root
	RelativePath string

	// Branch is the current branch
	Branch string

	// Status is the operation status (success, skipped, error, would-commit)
	Status string

	// CommitHash is the commit hash if successful
	CommitHash string

	// Message is the commit message used
	Message string

	// SuggestedMessage is the auto-generated message (for preview)
	SuggestedMessage string

	// FilesChanged is the number of files changed
	FilesChanged int

	// TrackedFilesChanged, UntrackedFilesChanged and StagedFilesChanged break
	// FilesChanged down. The untracked count is the one that used to be wrong:
	// without -uall, `?? docs/` counted as a single file while the commit
	// recorded every file beneath it.
	TrackedFilesChanged   int
	UntrackedFilesChanged int
	StagedFilesChanged    int

	// Additions is the number of lines added
	Additions int

	// Deletions is the number of lines deleted
	Deletions int

	// ChangedFiles is the list of changed files
	ChangedFiles []string

	// ConflictedFiles is the list of unmerged files. Non-empty means the
	// repository was not committed (unless AllowConflicted was set).
	ConflictedFiles []string

	// Error if the operation failed
	Error error

	// Duration is the operation time for this repository
	Duration time.Duration
}

// GetStatus returns the status for summary calculation.
func (r RepositoryCommitResult) GetStatus() string { return r.Status }

// BulkCommit scans for repositories and commits changes in parallel.
//
//nolint:gocognit // TODO: Refactor into smaller helper functions
func (c *client) BulkCommit(ctx context.Context, opts BulkCommitOptions) (*BulkCommitResult, error) {
	startTime := time.Now()

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

	result := &BulkCommitResult{
		TotalScanned: totalScanned,
		Repositories: make([]RepositoryCommitResult, 0, len(filteredRepos)),
		Summary:      make(map[string]int),
	}

	if len(filteredRepos) == 0 {
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Phase 1: Collect status for all dirty repositories
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

			repoResult := c.analyzeRepositoryForCommit(ctx, common.Directory, path, opts)

			mu.Lock()
			result.Repositories = append(result.Repositories, repoResult)
			mu.Unlock()
		}(i, repoPath)
	}
	wg.Wait()

	// Count dirty repositories. "conflicted" repos are deliberately excluded:
	// they are never handed to Phase 2, so counting them as dirty would make the
	// committed/dirty ratio look like a failure rather than a refusal.
	for _, repo := range result.Repositories {
		switch repo.Status {
		case "dirty", "would-commit":
			result.TotalDirty++
		case "conflicted":
			result.TotalConflicted++
		}
	}

	// Skipped counts repositories that were examined and left alone, so it is
	// measured against the filtered set. TotalScanned is the pre-filter total, so
	// using it counted every repository excluded by --include/--exclude as
	// "skipped" — a repository that was never a candidate was reported as one
	// that had nothing to do.
	//
	// This is computed before the dry-run return below; when it lived after it,
	// a dry run always reported 0 skipped.
	result.TotalSkipped = len(filteredRepos) - result.TotalDirty - result.TotalConflicted

	// If dry-run, we're done with analysis
	if opts.DryRun {
		for i := range result.Repositories {
			if result.Repositories[i].Status == "dirty" {
				result.Repositories[i].Status = "would-commit"
			}
		}
		result.Duration = time.Since(startTime)
		c.updateCommitSummary(result)
		return result, nil
	}

	// Phase 2: Commit dirty repositories (if not dry-run)
	for i, repoPath := range filteredRepos {
		// Find the corresponding result
		var repoResult *RepositoryCommitResult
		for j := range result.Repositories {
			if result.Repositories[j].Path == repoPath {
				repoResult = &result.Repositories[j]
				break
			}
		}

		if repoResult == nil || repoResult.Status != "dirty" {
			continue
		}

		wg.Add(1)
		go func(_ int, path string, res *RepositoryCommitResult) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			commitStart := time.Now()

			// Determine message
			message := opts.Message
			if message == "" {
				message = res.SuggestedMessage
			}
			if message == "" {
				message = fmt.Sprintf("chore: update %d files", res.FilesChanged)
			}

			// Execute commit
			hash, err := c.executeCommit(ctx, path, message)
			if err != nil {
				mu.Lock()
				res.Status = "error"
				res.Error = err
				result.TotalFailed++
				mu.Unlock()
			} else {
				mu.Lock()
				res.Status = "success"
				res.CommitHash = hash
				res.Message = message
				result.TotalCommitted++
				mu.Unlock()
			}

			res.Duration = time.Since(commitStart)
		}(i, repoPath, repoResult)
	}
	wg.Wait()

	result.Duration = time.Since(startTime)
	c.updateCommitSummary(result)

	return result, nil
}

// analyzeRepositoryForCommit analyzes a repository for potential commit.
func (c *client) analyzeRepositoryForCommit(ctx context.Context, rootDir, repoPath string, opts BulkCommitOptions) RepositoryCommitResult {
	startTime := time.Now()

	relPath, err := filepath.Rel(rootDir, repoPath)
	if err != nil {
		relPath = repoPath
	}
	if relPath == "." {
		relPath = filepath.Base(rootDir)
	}

	result := RepositoryCommitResult{
		Path:         repoPath,
		RelativePath: relPath,
		Status:       "clean",
		ChangedFiles: []string{},
	}

	// Get branch
	branchResult, err := c.executor.Run(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err == nil {
		result.Branch = strings.TrimSpace(branchResult.Stdout)
	}

	// Collect the change set at HEAD scope — the exact set `git add -A` stages
	// and the following commit records. Sharing collectChangeSet with BulkDiff is
	// what makes `diff` and `commit --dry-run` agree; previously each parsed
	// porcelain independently and reported different file counts for the same
	// repository.
	changes, err := c.collectChangeSet(ctx, repoPath, ScopeHead)
	if err != nil {
		result.Status = "error"
		result.Error = err
		result.Duration = time.Since(startTime)
		return result
	}

	var porcelainConflicts []string
	for _, entry := range changes.Entries {
		result.ChangedFiles = append(result.ChangedFiles, entry.Path)
		if entry.Conflicted {
			porcelainConflicts = append(porcelainConflicts, entry.Path)
		}
	}

	result.FilesChanged = len(result.ChangedFiles)
	result.TrackedFilesChanged = changes.TrackedCount
	result.UntrackedFilesChanged = changes.UntrackedCount
	result.StagedFilesChanged = changes.StagedCount
	result.Additions = changes.Additions
	result.Deletions = changes.Deletions

	if result.FilesChanged == 0 {
		result.Status = "clean"
		result.Duration = time.Since(startTime)
		return result
	}

	// A repository can look dirty and still have nothing to commit. `MM f.txt`
	// means the index differs from HEAD and the worktree differs from the index;
	// if the worktree happens to match HEAD, the net change against HEAD is empty
	// and `git add -A && git commit` fails with "nothing to commit". Predicting
	// "would-commit" for it turned a deterministic failure into a surprise —
	// and, because the failure arrived in phase 2, one that exited 0.
	//
	// Conflicts are exempt: an unmerged path has no HEAD delta until it is
	// resolved, and the guard below must still refuse it rather than call it
	// clean.
	if changes.DiffFileCount == 0 && changes.UntrackedCount == 0 && changes.ConflictCount == 0 {
		result.Status = "clean"
		result.Duration = time.Since(startTime)
		return result
	}

	// Guard against committing an unresolved merge. `executeCommit` runs
	// `git add -A`, which git treats as "conflict resolved": the <<<<<<< markers
	// would be staged as ordinary content and, because .git/MERGE_HEAD still
	// exists, recorded in a two-parent merge commit. The repository then reports
	// clean, so the damage is effectively undetectable after the fact.
	if len(porcelainConflicts) > 0 {
		// Porcelain paths may be C-quoted; ask the index for the real ones.
		result.ConflictedFiles = c.collectConflictedPaths(ctx, repoPath)
		if len(result.ConflictedFiles) == 0 {
			result.ConflictedFiles = porcelainConflicts
		}
	}

	if len(result.ConflictedFiles) > 0 && !opts.AllowConflicted {
		result.Status = "conflicted"
		result.Error = fmt.Errorf(
			"%d unmerged path(s) — resolve the merge first, or re-run with --allow-conflicted: %s",
			len(result.ConflictedFiles), strings.Join(result.ConflictedFiles, ", "),
		)
		result.Duration = time.Since(startTime)
		return result
	}

	result.Status = "dirty"

	// Line counts came from the change set above. The previous code summed
	// `--stat --cached` and `--stat`, which double-counts any file that is both
	// staged and modified again in the worktree.

	// Generate suggested message
	if opts.MessageGenerator != nil {
		msg, err := opts.MessageGenerator(ctx, repoPath, result.ChangedFiles)
		if err == nil {
			result.SuggestedMessage = msg
		}
	}

	if result.SuggestedMessage == "" {
		result.SuggestedMessage = c.generateSimpleCommitMessage(result.ChangedFiles)
	}

	result.Duration = time.Since(startTime)
	return result
}

// executeCommit executes git add and commit.
func (c *client) executeCommit(ctx context.Context, repoPath, message string) (string, error) {
	// Stage all changes
	_, err := c.executor.Run(ctx, repoPath, "add", "-A")
	if err != nil {
		return "", fmt.Errorf("failed to stage changes: %w", err)
	}

	// Create commit
	cmd := exec.CommandContext(ctx, "git", "commit", "-m", message) // #nosec G204 -- git executable and fixed flags are used; message is one argv value.
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to commit: %w\nOutput: %s", err, string(output))
	}

	// Get commit hash
	hashResult, err := c.executor.Run(ctx, repoPath, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", nil //nolint:nilerr // commit succeeded; hash retrieval is best-effort, empty string is acceptable
	}

	return strings.TrimSpace(hashResult.Stdout), nil
}

// generateSimpleCommitMessage generates a simple commit message from file changes.
func (c *client) generateSimpleCommitMessage(files []string) string {
	if len(files) == 0 {
		return "chore: update files"
	}

	// Analyze files to infer type
	testFiles := 0
	docFiles := 0
	configFiles := 0

	for _, file := range files {
		lower := strings.ToLower(file)
		switch {
		case strings.Contains(lower, "test"):
			testFiles++
		case strings.HasSuffix(lower, ".md"), strings.Contains(lower, "readme"):
			docFiles++
		case strings.HasSuffix(lower, ".yaml"), strings.HasSuffix(lower, ".yml"),
			strings.HasSuffix(lower, ".json"), strings.HasSuffix(lower, ".toml"):
			configFiles++
		}
	}

	total := len(files)

	// Determine type
	commitType := "chore"
	if testFiles > total/2 {
		commitType = "test"
	} else if docFiles > total/2 {
		commitType = "docs"
	}

	// Infer scope from common directory
	scope := inferScopeFromFiles(files)

	// Generate description
	var desc string
	if len(files) == 1 {
		desc = fmt.Sprintf("update %s", filepath.Base(files[0]))
	} else {
		desc = fmt.Sprintf("update %d files", len(files))
	}

	if scope != "" {
		return fmt.Sprintf("%s(%s): %s", commitType, scope, desc)
	}
	return fmt.Sprintf("%s: %s", commitType, desc)
}

// inferScopeFromFiles infers a scope from file paths.
func inferScopeFromFiles(files []string) string {
	if len(files) == 0 {
		return ""
	}

	// Count first-level directories
	dirCount := make(map[string]int)
	for _, file := range files {
		parts := strings.Split(filepath.Dir(file), string(filepath.Separator))
		if len(parts) > 0 && parts[0] != "." && parts[0] != "" {
			dirCount[parts[0]]++
		}
	}

	// Find most common directory
	maxCount := 0
	scope := ""
	for dir, count := range dirCount {
		if count > maxCount {
			maxCount = count
			scope = dir
		}
	}

	// Clean up scope
	scope = strings.TrimPrefix(scope, "pkg/")
	scope = strings.TrimPrefix(scope, "internal/")
	scope = strings.TrimPrefix(scope, "cmd/")

	return scope
}

// updateCommitSummary updates the summary counts.
func (c *client) updateCommitSummary(result *BulkCommitResult) {
	for _, repo := range result.Repositories {
		result.Summary[repo.Status]++
	}
}
