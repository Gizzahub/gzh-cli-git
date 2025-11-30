// Package merge provides advanced merge and rebase operations.
// This package includes conflict detection, merge strategy management,
// automatic conflict resolution, and rebase operations with safety checks.
//
// Example usage:
//
//	// Detect conflicts before merging
//	detector := merge.NewConflictDetector()
//	report, err := detector.Detect(ctx, repo, "feature-branch")
//
//	// Execute merge with strategy
//	mgr := merge.NewMergeManager()
//	result, err := mgr.Merge(ctx, repo, merge.MergeOptions{
//	    Branch:   "feature-branch",
//	    Strategy: merge.StrategyOurs,
//	})
package merge

import (
	"context"
	"fmt"
	"strings"

	"github.com/gizzahub/gzh-cli-git/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

// GitExecutor defines the interface for running git commands
type GitExecutor interface {
	Run(ctx context.Context, repoPath string, args ...string) (*gitcmd.Result, error)
}

// ConflictDetector analyzes potential merge conflicts
type ConflictDetector interface {
	// Detect analyzes potential conflicts between source and target
	Detect(ctx context.Context, repo *repository.Repository, opts DetectOptions) (*ConflictReport, error)

	// Preview shows what will happen during merge
	Preview(ctx context.Context, repo *repository.Repository, source, target string) (*MergePreview, error)

	// CanFastForward checks if fast-forward merge is possible
	CanFastForward(ctx context.Context, repo *repository.Repository, source, target string) (bool, error)
}

type conflictDetector struct {
	executor GitExecutor
}

// NewConflictDetector creates a new conflict detector
func NewConflictDetector(executor GitExecutor) ConflictDetector {
	return &conflictDetector{executor: executor}
}

// Detect analyzes potential conflicts between source and target
func (d *conflictDetector) Detect(ctx context.Context, repo *repository.Repository, opts DetectOptions) (*ConflictReport, error) {
	// Validate branches
	if err := d.validateBranch(ctx, repo, opts.Source); err != nil {
		return nil, fmt.Errorf("invalid source branch: %w", err)
	}
	if err := d.validateBranch(ctx, repo, opts.Target); err != nil {
		return nil, fmt.Errorf("invalid target branch: %w", err)
	}

	// Find merge base
	mergeBase, err := d.findMergeBase(ctx, repo, opts.Source, opts.Target)
	if err != nil {
		return nil, err
	}

	// Get changed files in both branches
	sourceChanges, err := d.getChangedFiles(ctx, repo, mergeBase, opts.Source)
	if err != nil {
		return nil, fmt.Errorf("failed to get source changes: %w", err)
	}

	targetChanges, err := d.getChangedFiles(ctx, repo, mergeBase, opts.Target)
	if err != nil {
		return nil, fmt.Errorf("failed to get target changes: %w", err)
	}

	// Detect conflicts
	conflicts := d.detectConflicts(sourceChanges, targetChanges, opts.IncludeBinary)

	// Calculate difficulty and auto-resolve count
	canAutoResolve := 0
	for _, c := range conflicts {
		if c.AutoResolvable {
			canAutoResolve++
		}
	}

	difficulty := d.calculateDifficulty(len(conflicts), canAutoResolve)

	return &ConflictReport{
		Source:         opts.Source,
		Target:         opts.Target,
		MergeBase:      mergeBase,
		TotalConflicts: len(conflicts),
		Conflicts:      conflicts,
		CanAutoResolve: canAutoResolve,
		Difficulty:     difficulty,
	}, nil
}

// Preview shows what will happen during merge
func (d *conflictDetector) Preview(ctx context.Context, repo *repository.Repository, source, target string) (*MergePreview, error) {
	// Check if fast-forward is possible
	canFF, err := d.CanFastForward(ctx, repo, source, target)
	if err != nil {
		return nil, err
	}

	// Detect conflicts
	report, err := d.Detect(ctx, repo, DetectOptions{
		Source:        source,
		Target:        target,
		IncludeBinary: true,
	})
	if err != nil {
		return nil, err
	}

	// Count file changes
	mergeBase := report.MergeBase
	changes, err := d.getChangedFiles(ctx, repo, mergeBase, source)
	if err != nil {
		return nil, err
	}

	filesToAdd := 0
	filesToDelete := 0
	filesToChange := 0

	for _, change := range changes {
		switch change.ChangeType {
		case ChangeAdded, ChangeCopied:
			filesToAdd++
		case ChangeDeleted:
			filesToDelete++
		case ChangeModified, ChangeRenamed:
			filesToChange++
		}
	}

	return &MergePreview{
		Source:         source,
		Target:         target,
		CanFastForward: canFF,
		FilesToChange:  filesToChange,
		FilesToAdd:     filesToAdd,
		FilesToDelete:  filesToDelete,
		Conflicts:      report.Conflicts,
		Difficulty:     report.Difficulty,
	}, nil
}

// CanFastForward checks if fast-forward merge is possible
func (d *conflictDetector) CanFastForward(ctx context.Context, repo *repository.Repository, source, target string) (bool, error) {
	// Check if target is ancestor of source
	result, err := d.executor.Run(ctx, repo.Path, "merge-base", "--is-ancestor", target, source)
	if err != nil {
		return false, nil // Not an error, just can't fast-forward
	}

	return result.ExitCode == 0, nil
}

// validateBranch checks if a branch reference exists
func (d *conflictDetector) validateBranch(ctx context.Context, repo *repository.Repository, ref string) error {
	if ref == "" {
		return ErrInvalidBranch
	}

	result, err := d.executor.Run(ctx, repo.Path, "rev-parse", "--verify", ref)
	if err != nil || result.ExitCode != 0 {
		return ErrBranchNotFound
	}

	return nil
}

// findMergeBase finds the common ancestor of two branches
func (d *conflictDetector) findMergeBase(ctx context.Context, repo *repository.Repository, source, target string) (string, error) {
	result, err := d.executor.Run(ctx, repo.Path, "merge-base", source, target)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNoMergeBase, err)
	}

	if result.ExitCode != 0 {
		return "", ErrNoMergeBase
	}

	return strings.TrimSpace(result.Stdout), nil
}

// getChangedFiles gets list of changed files between two commits
func (d *conflictDetector) getChangedFiles(ctx context.Context, repo *repository.Repository, base, head string) ([]*FileChange, error) {
	result, err := d.executor.Run(ctx, repo.Path, "diff", "--name-status", base+".."+head)
	if err != nil {
		return nil, err
	}

	var changes []*FileChange
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		status := parts[0]
		path := parts[1]

		change := &FileChange{
			Path:       path,
			ChangeType: d.parseChangeType(status),
		}

		// Handle renames (R100 oldpath newpath)
		if strings.HasPrefix(status, "R") && len(parts) >= 3 {
			change.OldPath = path
			change.Path = parts[2]
		}

		changes = append(changes, change)
	}

	return changes, nil
}

// detectConflicts finds conflicts between two sets of changes
func (d *conflictDetector) detectConflicts(sourceChanges, targetChanges []*FileChange, includeBinary bool) []*Conflict {
	var conflicts []*Conflict

	// Create maps for quick lookup
	sourceMap := make(map[string]*FileChange)
	for _, change := range sourceChanges {
		sourceMap[change.Path] = change
	}

	targetMap := make(map[string]*FileChange)
	for _, change := range targetChanges {
		targetMap[change.Path] = change
	}

	// Check for conflicts
	for path, sourceChange := range sourceMap {
		targetChange, exists := targetMap[path]
		if !exists {
			continue // No conflict if only changed in one branch
		}

		conflict := d.analyzeConflict(path, sourceChange, targetChange)
		if conflict != nil && (includeBinary || conflict.ConflictType != ConflictBinary) {
			conflicts = append(conflicts, conflict)
		}
	}

	return conflicts
}

// analyzeConflict analyzes a potential conflict between two changes
func (d *conflictDetector) analyzeConflict(path string, sourceChange, targetChange *FileChange) *Conflict {
	// Both modified same file
	if sourceChange.ChangeType == ChangeModified && targetChange.ChangeType == ChangeModified {
		return &Conflict{
			FilePath:       path,
			ConflictType:   ConflictContent,
			SourceChange:   ChangeModified,
			TargetChange:   ChangeModified,
			Severity:       SeverityMedium,
			AutoResolvable: false,
			Description:    "File modified in both branches",
		}
	}

	// One deleted, one modified
	if sourceChange.ChangeType == ChangeDeleted && targetChange.ChangeType == ChangeModified {
		return &Conflict{
			FilePath:       path,
			ConflictType:   ConflictDelete,
			SourceChange:   ChangeDeleted,
			TargetChange:   ChangeModified,
			Severity:       SeverityHigh,
			AutoResolvable: false,
			Description:    "File deleted in source but modified in target",
		}
	}

	if sourceChange.ChangeType == ChangeModified && targetChange.ChangeType == ChangeDeleted {
		return &Conflict{
			FilePath:       path,
			ConflictType:   ConflictDelete,
			SourceChange:   ChangeModified,
			TargetChange:   ChangeDeleted,
			Severity:       SeverityHigh,
			AutoResolvable: false,
			Description:    "File modified in source but deleted in target",
		}
	}

	// Both renamed
	if sourceChange.ChangeType == ChangeRenamed && targetChange.ChangeType == ChangeRenamed {
		if sourceChange.OldPath == targetChange.OldPath {
			return &Conflict{
				FilePath:       path,
				ConflictType:   ConflictRename,
				SourceChange:   ChangeRenamed,
				TargetChange:   ChangeRenamed,
				Severity:       SeverityLow,
				AutoResolvable: true,
				Description:    "File renamed differently in both branches",
			}
		}
	}

	return nil
}

// parseChangeType converts git status code to ChangeType
func (d *conflictDetector) parseChangeType(status string) ChangeType {
	switch {
	case status == "A":
		return ChangeAdded
	case status == "D":
		return ChangeDeleted
	case status == "M":
		return ChangeModified
	case strings.HasPrefix(status, "R"):
		return ChangeRenamed
	case status == "C":
		return ChangeCopied
	default:
		return ChangeModified
	}
}

// calculateDifficulty determines merge difficulty based on conflicts
func (d *conflictDetector) calculateDifficulty(totalConflicts, canAutoResolve int) MergeDifficulty {
	if totalConflicts == 0 {
		return DifficultyTrivial
	}

	if canAutoResolve == totalConflicts {
		return DifficultyEasy
	}

	if totalConflicts <= 5 {
		return DifficultyMedium
	}

	return DifficultyHard
}
