// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package branch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/internal/porcelain"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// ParallelWorkflow manages parallel development workflows.
type ParallelWorkflow interface {
	// GetActiveContexts returns all active worktree contexts.
	GetActiveContexts(ctx context.Context, repo *repository.Repository) ([]*WorkContext, error)

	// SwitchContext provides information for switching to a worktree.
	SwitchContext(ctx context.Context, repo *repository.Repository, path string) (*SwitchInfo, error)

	// DetectConflicts detects potential conflicts across worktrees.
	DetectConflicts(ctx context.Context, repo *repository.Repository) ([]*Conflict, error)

	// GetStatus gets status across all worktrees.
	GetStatus(ctx context.Context, repo *repository.Repository) (*ParallelStatus, error)
}

// WorkContext represents a development context (worktree).
type WorkContext struct {
	Path          string   // Worktree path
	Branch        string   // Current branch
	IsMain        bool     // Is main worktree
	HasChanges    bool     // Has uncommitted changes
	ModifiedFiles []string // List of modified files
}

// SwitchInfo provides information for context switching.
type SwitchInfo struct {
	FromPath   string // Current location
	ToPath     string // Target worktree path
	ToBranch   string // Target branch
	Command    string // Suggested command (cd)
	HasChanges bool   // Target has uncommitted changes
}

// Conflict represents a potential conflict across worktrees.
type Conflict struct {
	File      string   // File path
	Worktrees []string // Worktrees modifying this file
	Severity  ConflictSeverity
}

// ConflictSeverity indicates conflict severity.
type ConflictSeverity string

// ConflictSeverity values indicate how severe a branch conflict is.
const (
	SeverityLow    ConflictSeverity = "low"    // Different files
	SeverityMedium ConflictSeverity = "medium" // Same directory
	SeverityHigh   ConflictSeverity = "high"   // Same file
)

// ParallelStatus represents status across all worktrees.
type ParallelStatus struct {
	TotalWorktrees  int            // Total number of worktrees
	ActiveWorktrees int            // Worktrees with changes
	Conflicts       int            // Number of conflicts
	Contexts        []*WorkContext // All contexts
}

// parallelWorkflow implements ParallelWorkflow.
type parallelWorkflow struct {
	executor        *gitcmd.Executor
	worktreeManager WorktreeManager
}

// NewParallelWorkflow creates a new ParallelWorkflow.
func NewParallelWorkflow() ParallelWorkflow {
	return &parallelWorkflow{
		executor:        gitcmd.NewExecutor(),
		worktreeManager: NewWorktreeManager(),
	}
}

// NewParallelWorkflowWithDeps creates a new ParallelWorkflow with custom dependencies.
func NewParallelWorkflowWithDeps(executor *gitcmd.Executor, worktreeManager WorktreeManager) ParallelWorkflow {
	return &parallelWorkflow{
		executor:        executor,
		worktreeManager: worktreeManager,
	}
}

// GetActiveContexts returns all active worktree contexts.
func (p *parallelWorkflow) GetActiveContexts(ctx context.Context, repo *repository.Repository) ([]*WorkContext, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository cannot be nil")
	}

	// Get all worktrees
	worktrees, err := p.worktreeManager.List(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	// Build contexts.
	//
	// A worktree that cannot be read is not dropped from the list. Every
	// consumer of these contexts asks a question whose reassuring answer is the
	// empty one — DetectConflicts reports the files two worktrees both touch,
	// GetStatus counts the worktrees holding changes — so omitting a worktree
	// silently is indistinguishable from that worktree being clean and
	// conflict-free.
	contexts := make([]*WorkContext, 0, len(worktrees))
	for _, wt := range worktrees {
		workCtx, err := p.buildWorkContext(ctx, wt)
		if err != nil {
			return nil, fmt.Errorf("failed to read worktree %s: %w", wt.Path, err)
		}

		contexts = append(contexts, workCtx)
	}

	return contexts, nil
}

// SwitchContext provides information for switching to a worktree.
func (p *parallelWorkflow) SwitchContext(ctx context.Context, repo *repository.Repository, path string) (*SwitchInfo, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository cannot be nil")
	}

	if path == "" {
		return nil, fmt.Errorf("worktree path is required")
	}

	// Get target worktree
	targetWt, err := p.worktreeManager.Get(ctx, repo, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	// Get current directory
	currentDir, err := os.Getwd()
	if err != nil {
		currentDir = ""
	}

	// Check if target has changes
	hasChanges, err := p.hasUncommittedChanges(ctx, path)
	if err != nil {
		hasChanges = false
	}

	// Build switch info
	info := &SwitchInfo{
		FromPath:   currentDir,
		ToPath:     targetWt.Path,
		ToBranch:   targetWt.Branch,
		Command:    fmt.Sprintf("cd %s", targetWt.Path),
		HasChanges: hasChanges,
	}

	return info, nil
}

// DetectConflicts detects potential conflicts across worktrees.
func (p *parallelWorkflow) DetectConflicts(ctx context.Context, repo *repository.Repository) ([]*Conflict, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository cannot be nil")
	}

	// Get all contexts
	contexts, err := p.GetActiveContexts(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to get contexts: %w", err)
	}

	// Build file -> worktrees map
	fileWorktrees := make(map[string][]string)
	for _, context := range contexts {
		if !context.HasChanges {
			continue
		}

		for _, file := range context.ModifiedFiles {
			fileWorktrees[file] = append(fileWorktrees[file], context.Path)
		}
	}

	// Find conflicts
	conflicts := make([]*Conflict, 0)
	for file, worktrees := range fileWorktrees {
		if len(worktrees) > 1 {
			conflict := &Conflict{
				File:      file,
				Worktrees: worktrees,
				Severity:  p.determineConflictSeverity(file, worktrees),
			}
			conflicts = append(conflicts, conflict)
		}
	}

	return conflicts, nil
}

// GetStatus gets status across all worktrees.
func (p *parallelWorkflow) GetStatus(ctx context.Context, repo *repository.Repository) (*ParallelStatus, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository cannot be nil")
	}

	// Get all contexts
	contexts, err := p.GetActiveContexts(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to get contexts: %w", err)
	}

	// Detect conflicts.
	//
	// Falling back to an empty slice here reported Conflicts: 0, which is the
	// same value a genuinely conflict-free set of worktrees produces and the one
	// a caller reads as "nothing to look at".
	conflicts, err := p.DetectConflicts(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to detect conflicts: %w", err)
	}

	// Count active worktrees
	activeCount := 0
	for _, context := range contexts {
		if context.HasChanges {
			activeCount++
		}
	}

	status := &ParallelStatus{
		TotalWorktrees:  len(contexts),
		ActiveWorktrees: activeCount,
		Conflicts:       len(conflicts),
		Contexts:        contexts,
	}

	return status, nil
}

// buildWorkContext builds a WorkContext from a Worktree.
//
// Both reads below used to mask their failure — a broken status became
// HasChanges=false, and an unreadable file list became an empty one. Together
// they turned any git failure into "this worktree is clean", which is precisely
// the state the callers act on by doing nothing.
func (p *parallelWorkflow) buildWorkContext(ctx context.Context, wt *Worktree) (*WorkContext, error) {
	hasChanges, err := p.hasUncommittedChanges(ctx, wt.Path)
	if err != nil {
		return nil, err
	}

	// Get modified files if there are changes
	var modifiedFiles []string
	if hasChanges {
		modifiedFiles, err = p.getModifiedFiles(ctx, wt.Path)
		if err != nil {
			return nil, err
		}
	}

	workCtx := &WorkContext{
		Path:          wt.Path,
		Branch:        wt.Branch,
		IsMain:        wt.IsMain,
		HasChanges:    hasChanges,
		ModifiedFiles: modifiedFiles,
	}

	return workCtx, nil
}

// hasUncommittedChanges checks if a worktree has uncommitted changes.
//
// Plain --porcelain is adequate here and only here: the answer is whether the
// output is empty, and neither directory collapsing nor C-quoting can turn an
// empty result into a non-empty one or the reverse. Going through
// internal/porcelain would unify the code without fixing anything.
func (p *parallelWorkflow) hasUncommittedChanges(ctx context.Context, path string) (bool, error) {
	result, err := p.executor.Run(ctx, path, "status", "--porcelain")
	if err != nil {
		return false, err
	}

	if result.ExitCode != 0 {
		return false, fmt.Errorf("git status failed in %s: %s", path, strings.TrimSpace(result.Stderr))
	}

	return strings.TrimSpace(result.Stdout) != "", nil
}

// getModifiedFiles lists the tracked files a worktree has changed.
//
// Every path returned here is compared against paths from other worktrees to
// find files two branches touch at once, so a value that names no file on disk
// does not merely look wrong — it can never match, and the conflict it was
// supposed to reveal goes unreported. The previous implementation produced three
// such values:
//
//   - A rename came back as the single string "old.txt -> new.txt", because
//     plain --porcelain renders both paths in one line. -z splits them into
//     separate records, and the destination is the path that exists.
//   - A path holding a space or a non-ASCII byte came back C-quoted
//     (`"\303\251.md"`), quotes and escapes included. -z disables quoting.
//   - Untracked files were included despite the name, and an untracked
//     directory collapsed to one `dir/` entry, so N new files counted as one
//     path that is not a file.
//
// Untracked files are now excluded outright rather than listed correctly: two
// worktrees independently creating a file git does not track yet is not the
// overlap this detector reports on. That is also why -uall is absent while the
// rest of the module pairs it with -z — it only affects how untracked entries
// are listed, so here it would buy a full recursive walk of every ignored tree
// for entries that are dropped on the next line.
func (p *parallelWorkflow) getModifiedFiles(ctx context.Context, path string) ([]string, error) {
	result, err := p.executor.Run(ctx, path, "status", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}

	// Run reports a failed git through ExitCode and leaves err nil unless the
	// process could not start, so checking err alone would read a broken status
	// as a worktree with nothing changed.
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("git status failed in %s: %s", path, strings.TrimSpace(result.Stderr))
	}

	records, err := porcelain.Parse(result.Stdout)
	if err != nil {
		return nil, fmt.Errorf("failed to read status in %s: %w", path, err)
	}

	files := make([]string, 0, len(records))
	for _, rec := range records {
		if rec.Code == "??" || rec.Code == "!!" {
			continue
		}

		// Path is the destination for a rename or copy; OldPath holds the source,
		// which no longer exists and cannot collide with anything.
		files = append(files, rec.Path)
	}

	return files, nil
}

// determineConflictSeverity determines conflict severity based on file paths.
func (p *parallelWorkflow) determineConflictSeverity(file string, worktrees []string) ConflictSeverity {
	// If same file modified in multiple worktrees, high severity
	if len(worktrees) > 1 {
		return SeverityHigh
	}

	// Check if files in same directory
	dir := filepath.Dir(file)
	for _, wt := range worktrees {
		wtDir := filepath.Dir(wt)
		if dir == wtDir {
			return SeverityMedium
		}
	}

	return SeverityLow
}

// HasConflicts checks if there are any conflicts.
func (s *ParallelStatus) HasConflicts() bool {
	return s.Conflicts > 0
}

// IsActive checks if any worktree has uncommitted changes.
func (s *ParallelStatus) IsActive() bool {
	return s.ActiveWorktrees > 0
}

// GetMainContext returns the main worktree context.
func (s *ParallelStatus) GetMainContext() *WorkContext {
	for _, ctx := range s.Contexts {
		if ctx.IsMain {
			return ctx
		}
	}
	return nil
}

// GetActiveContexts returns contexts with uncommitted changes.
func (s *ParallelStatus) GetActiveContexts() []*WorkContext {
	active := make([]*WorkContext, 0)
	for _, ctx := range s.Contexts {
		if ctx.HasChanges {
			active = append(active, ctx)
		}
	}
	return active
}
