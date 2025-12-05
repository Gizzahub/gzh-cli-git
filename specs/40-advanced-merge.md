# Phase 5: Advanced Merge & Rebase Specification

**Phase**: 5
**Priority**: P0 (High)
**Status**: In Progress
**Created**: 2025-11-27
**Dependencies**: Phase 3 (Branch Management), Phase 4 (History Analysis)

______________________________________________________________________

## Overview

Phase 5 implements advanced merge and rebase capabilities with intelligent conflict detection, multiple merge strategies, and interactive rebase support. This phase focuses on making complex Git operations safer, more predictable, and easier to execute.

### Goals

1. **Conflict Detection** - Proactively detect merge conflicts before attempting merge
1. **Merge Strategies** - Support multiple merge strategies with intelligent selection
1. **Interactive Rebase** - Safe, guided interactive rebase with conflict resolution
1. **Auto-Resolution** - Automatically resolve simple conflicts when safe
1. **Rollback Support** - Easy rollback mechanisms for failed operations

### Non-Goals

- GUI-based conflict resolution
- Advanced cherry-pick operations
- Stash management (deferred to Phase 6)
- Patch operations

______________________________________________________________________

## Architecture

### Package Structure

```
pkg/merge/
├── types.go           # Core types and interfaces
├── errors.go          # Merge-specific errors
├── detector.go        # Conflict detection
├── strategy.go        # Merge strategy implementation
├── rebase.go          # Rebase operations
├── resolver.go        # Auto-conflict resolution
├── detector_test.go
├── strategy_test.go
├── rebase_test.go
└── resolver_test.go
```

### Core Interfaces

```go
// ConflictDetector detects potential merge conflicts
type ConflictDetector interface {
    Detect(ctx context.Context, repo *repository.Repository, opts DetectOptions) (*ConflictReport, error)
    Preview(ctx context.Context, repo *repository.Repository, source, target string) (*MergePreview, error)
}

// MergeStrategyManager manages merge strategies
type MergeStrategyManager interface {
    Merge(ctx context.Context, repo *repository.Repository, opts MergeOptions) (*MergeResult, error)
    SelectStrategy(ctx context.Context, repo *repository.Repository, source, target string) (MergeStrategy, error)
    Abort(ctx context.Context, repo *repository.Repository) error
}

// RebaseManager manages rebase operations
type RebaseManager interface {
    Rebase(ctx context.Context, repo *repository.Repository, opts RebaseOptions) (*RebaseResult, error)
    Continue(ctx context.Context, repo *repository.Repository) error
    Skip(ctx context.Context, repo *repository.Repository) error
    Abort(ctx context.Context, repo *repository.Repository) error
}

// ConflictResolver attempts automatic conflict resolution
type ConflictResolver interface {
    Resolve(ctx context.Context, repo *repository.Repository, conflicts []*Conflict) (*ResolutionResult, error)
    CanResolve(conflict *Conflict) bool
}
```

______________________________________________________________________

## Component 1: Conflict Detector

### Purpose

Detect potential merge conflicts before attempting a merge, allowing users to review and prepare for conflicts in advance.

### Features

1. **Pre-Merge Analysis**

   - Analyze changes in source and target branches
   - Identify files that have diverged
   - Detect overlapping modifications
   - Calculate conflict probability

1. **Conflict Classification**

   - Content conflicts (same lines modified)
   - Rename conflicts (file renamed in both branches)
   - Delete conflicts (file modified in one, deleted in other)
   - Binary conflicts (binary files modified in both)

1. **Merge Preview**

   - Show what will happen during merge
   - List files that will be merged
   - Highlight potential conflict areas
   - Estimate merge difficulty

### Data Types

```go
// ConflictReport contains detected conflicts
type ConflictReport struct {
    Source        string
    Target        string
    TotalConflicts int
    Conflicts     []*Conflict
    CanAutoResolve int
    Difficulty    MergeDifficulty
}

// Conflict represents a single merge conflict
type Conflict struct {
    FilePath     string
    ConflictType ConflictType
    SourceChange ChangeType
    TargetChange ChangeType
    Severity     ConflictSeverity
    AutoResolvable bool
    Description  string
}

// ConflictType defines the type of conflict
type ConflictType string

const (
    ConflictContent ConflictType = "content"    // Content conflicts
    ConflictRename  ConflictType = "rename"     // Rename conflicts
    ConflictDelete  ConflictType = "delete"     // Delete/modify conflicts
    ConflictBinary  ConflictType = "binary"     // Binary file conflicts
)

// MergeDifficulty indicates merge complexity
type MergeDifficulty string

const (
    DifficultyTrivial MergeDifficulty = "trivial"  // No conflicts
    DifficultyEasy    MergeDifficulty = "easy"     // Auto-resolvable
    DifficultyMedium  MergeDifficulty = "medium"   // Some manual work
    DifficultyHard    MergeDifficulty = "hard"     // Many conflicts
)

// DetectOptions configures conflict detection
type DetectOptions struct {
    Source      string
    Target      string
    BaseCommit  string
    IncludeBinary bool
}
```

### Git Commands

```bash
# Find merge base
git merge-base source target

# Get changes in source
git diff merge-base..source --name-status

# Get changes in target
git diff merge-base..target --name-status

# Simulate merge to detect conflicts
git merge-tree merge-base source target

# Check for conflicts without merging
git merge --no-commit --no-ff source
git merge --abort
```

### Implementation

```go
type conflictDetector struct {
    executor GitExecutor
}

func (c *conflictDetector) Detect(ctx context.Context, repo *repository.Repository, opts DetectOptions) (*ConflictReport, error) {
    // Find merge base
    baseResult, err := c.executor.Run(ctx, repo.Path, "merge-base", opts.Source, opts.Target)
    if err != nil {
        return nil, fmt.Errorf("failed to find merge base: %w", err)
    }
    mergeBase := strings.TrimSpace(baseResult.Stdout)

    // Get changes in source branch
    sourceChanges, err := c.getChanges(ctx, repo, mergeBase, opts.Source)
    if err != nil {
        return nil, err
    }

    // Get changes in target branch
    targetChanges, err := c.getChanges(ctx, repo, mergeBase, opts.Target)
    if err != nil {
        return nil, err
    }

    // Detect conflicts
    conflicts := c.detectConflicts(sourceChanges, targetChanges)

    // Build report
    report := &ConflictReport{
        Source:         opts.Source,
        Target:         opts.Target,
        TotalConflicts: len(conflicts),
        Conflicts:      conflicts,
        Difficulty:     c.calculateDifficulty(conflicts),
    }

    // Count auto-resolvable conflicts
    for _, conflict := range conflicts {
        if conflict.AutoResolvable {
            report.CanAutoResolve++
        }
    }

    return report, nil
}
```

______________________________________________________________________

## Component 2: Merge Strategy Manager

### Purpose

Execute merges using appropriate strategies based on branch structure and conflict analysis.

### Merge Strategies

1. **Fast-Forward (ff)**

   - No divergence, just move pointer
   - Cleanest history
   - Use when: Target is ancestor of source

1. **Recursive (default)**

   - Three-way merge with conflict detection
   - Creates merge commit
   - Use when: Standard merge needed

1. **Ours**

   - Prefer source changes in conflicts
   - Use when: Source has priority

1. **Theirs**

   - Prefer target changes in conflicts
   - Use when: Target has priority

1. **Octopus**

   - Merge multiple branches at once
   - Use when: Merging feature branches together

### Data Types

```go
// MergeStrategy defines merge approach
type MergeStrategy string

const (
    StrategyFastForward MergeStrategy = "fast-forward"
    StrategyRecursive   MergeStrategy = "recursive"
    StrategyOurs        MergeStrategy = "ours"
    StrategyTheirs      MergeStrategy = "theirs"
    StrategyOctopus     MergeStrategy = "octopus"
)

// MergeOptions configures merge operation
type MergeOptions struct {
    Source         string
    Target         string
    Strategy       MergeStrategy
    AllowFastForward bool
    CommitMessage  string
    NoCommit       bool
    Squash         bool
}

// MergeResult contains merge outcome
type MergeResult struct {
    Success      bool
    Strategy     MergeStrategy
    CommitHash   string
    Conflicts    []*Conflict
    FilesChanged int
    Message      string
}
```

### Git Commands

```bash
# Fast-forward merge
git merge --ff-only source

# Recursive merge
git merge --no-ff source -m "Merge message"

# Merge with strategy
git merge -s ours source
git merge -s theirs source

# Squash merge
git merge --squash source

# No-commit merge (preview)
git merge --no-commit source
```

### Implementation

```go
type mergeStrategyManager struct {
    executor GitExecutor
    detector ConflictDetector
}

func (m *mergeStrategyManager) Merge(ctx context.Context, repo *repository.Repository, opts MergeOptions) (*MergeResult, error) {
    // Validate options
    if err := m.validateOptions(opts); err != nil {
        return nil, err
    }

    // Select strategy if not specified
    strategy := opts.Strategy
    if strategy == "" {
        var err error
        strategy, err = m.SelectStrategy(ctx, repo, opts.Source, opts.Target)
        if err != nil {
            return nil, err
        }
    }

    // Execute merge based on strategy
    result, err := m.executeMerge(ctx, repo, opts, strategy)
    if err != nil {
        return nil, err
    }

    return result, nil
}

func (m *mergeStrategyManager) SelectStrategy(ctx context.Context, repo *repository.Repository, source, target string) (MergeStrategy, error) {
    // Check if fast-forward is possible
    canFF, err := m.canFastForward(ctx, repo, source, target)
    if err != nil {
        return "", err
    }

    if canFF {
        return StrategyFastForward, nil
    }

    // Default to recursive for normal merges
    return StrategyRecursive, nil
}
```

______________________________________________________________________

## Component 3: Rebase Manager

### Purpose

Perform interactive and non-interactive rebases with conflict handling and rollback support.

### Features

1. **Standard Rebase**

   - Rebase branch onto another
   - Preserve commit history
   - Handle conflicts interactively

1. **Interactive Rebase**

   - Pick commits to include
   - Reorder commits
   - Squash commits
   - Edit commit messages

1. **Conflict Resolution**

   - Detect conflicts during rebase
   - Pause for manual resolution
   - Continue after resolution
   - Skip problematic commits

1. **Safety Features**

   - Backup original branch
   - Easy abort mechanism
   - Preserve uncommitted changes
   - Validate before starting

### Data Types

```go
// RebaseOptions configures rebase operation
type RebaseOptions struct {
    Branch       string
    Onto         string
    Interactive  bool
    AutoSquash   bool
    PreserveMerges bool
    UpstreamName string
}

// RebaseResult contains rebase outcome
type RebaseResult struct {
    Success       bool
    CommitsRebased int
    ConflictsFound int
    CurrentCommit  string
    Status        RebaseStatus
    Message       string
}

// RebaseStatus indicates rebase state
type RebaseStatus string

const (
    RebaseComplete    RebaseStatus = "complete"
    RebaseInProgress  RebaseStatus = "in_progress"
    RebaseConflict    RebaseStatus = "conflict"
    RebaseAborted     RebaseStatus = "aborted"
)

// RebaseAction defines interactive rebase action
type RebaseAction string

const (
    ActionPick   RebaseAction = "pick"
    ActionReword RebaseAction = "reword"
    ActionEdit   RebaseAction = "edit"
    ActionSquash RebaseAction = "squash"
    ActionFixup  RebaseAction = "fixup"
    ActionDrop   RebaseAction = "drop"
)
```

### Git Commands

```bash
# Standard rebase
git rebase onto

# Interactive rebase
git rebase -i HEAD~5

# Continue after conflict resolution
git rebase --continue

# Skip current commit
git rebase --skip

# Abort rebase
git rebase --abort

# Check rebase status
git status --porcelain
```

### Implementation

```go
type rebaseManager struct {
    executor GitExecutor
}

func (r *rebaseManager) Rebase(ctx context.Context, repo *repository.Repository, opts RebaseOptions) (*RebaseResult, error) {
    // Validate repository state
    if err := r.validateState(ctx, repo); err != nil {
        return nil, err
    }

    // Build rebase command
    args := []string{"rebase"}

    if opts.Interactive {
        args = append(args, "-i")
    }

    if opts.PreserveMerges {
        args = append(args, "--preserve-merges")
    }

    if opts.Onto != "" {
        args = append(args, "--onto", opts.Onto)
    }

    args = append(args, opts.UpstreamName)

    // Execute rebase
    result, err := r.executor.Run(ctx, repo.Path, args...)
    if err != nil {
        return r.handleRebaseError(err, result)
    }

    return &RebaseResult{
        Success: true,
        Status:  RebaseComplete,
        Message: "Rebase completed successfully",
    }, nil
}
```

______________________________________________________________________

## Component 4: Conflict Resolver

### Purpose

Automatically resolve simple, safe conflicts to reduce manual intervention.

### Auto-Resolution Rules

1. **Whitespace-Only Conflicts**

   - Different whitespace, same content
   - Safe to normalize

1. **Comment-Only Conflicts**

   - Only comments differ
   - Can merge both

1. **Import/Dependency Conflicts**

   - Different imports added
   - Can merge both lists

1. **Non-Overlapping Changes**

   - Changes in different parts of file
   - Can merge both

### Data Types

```go
// ResolutionResult contains resolution outcome
type ResolutionResult struct {
    TotalConflicts int
    Resolved       int
    Failed         int
    Resolutions    []*Resolution
}

// Resolution represents a single conflict resolution
type Resolution struct {
    FilePath     string
    ConflictType ConflictType
    Strategy     ResolutionStrategy
    Success      bool
    Error        error
}

// ResolutionStrategy defines how to resolve
type ResolutionStrategy string

const (
    StrategyKeepBoth      ResolutionStrategy = "keep_both"
    StrategyKeepOurs      ResolutionStrategy = "keep_ours"
    StrategyKeepTheirs    ResolutionStrategy = "keep_theirs"
    StrategyMergeLines    ResolutionStrategy = "merge_lines"
    StrategyNormalize     ResolutionStrategy = "normalize"
)
```

______________________________________________________________________

## Error Handling

### Custom Errors

```go
var (
    ErrConflictsDetected  = errors.New("merge conflicts detected")
    ErrMergeInProgress    = errors.New("merge already in progress")
    ErrRebaseInProgress   = errors.New("rebase already in progress")
    ErrDirtyWorkingTree   = errors.New("working tree has uncommitted changes")
    ErrNoMergeBase        = errors.New("no common merge base found")
    ErrInvalidStrategy    = errors.New("invalid merge strategy")
    ErrCannotFastForward  = errors.New("cannot fast-forward merge")
    ErrRebaseConflict     = errors.New("rebase conflict encountered")
)
```

______________________________________________________________________

## Testing Strategy

### Unit Tests

1. **Conflict Detector Tests**

   - Detect content conflicts
   - Detect rename conflicts
   - Detect delete conflicts
   - Calculate merge difficulty
   - Handle no conflicts case

1. **Merge Strategy Tests**

   - Fast-forward merge
   - Recursive merge
   - Strategy selection
   - Abort functionality
   - Invalid state handling

1. **Rebase Manager Tests**

   - Standard rebase
   - Interactive rebase
   - Continue/skip/abort
   - Conflict handling
   - State validation

1. **Conflict Resolver Tests**

   - Auto-resolve safe conflicts
   - Skip unsafe conflicts
   - Resolution strategies
   - Error handling

### Integration Tests

Deferred to Phase 6 (requires real Git repositories with conflicts).

### Coverage Target

- Unit tests: ≥85%
- Integration tests: ≥80%
- Overall: ≥85%

______________________________________________________________________

## Safety Mechanisms

### Pre-Flight Checks

```go
// Before any destructive operation
func (m *mergeStrategyManager) validateState(ctx context.Context, repo *repository.Repository) error {
    // Check for uncommitted changes
    status, err := m.executor.Run(ctx, repo.Path, "status", "--porcelain")
    if err != nil {
        return err
    }

    if status.Stdout != "" {
        return ErrDirtyWorkingTree
    }

    // Check for ongoing merge/rebase
    if m.isMergeInProgress(ctx, repo) {
        return ErrMergeInProgress
    }

    if m.isRebaseInProgress(ctx, repo) {
        return ErrRebaseInProgress
    }

    return nil
}
```

### Backup Mechanisms

```go
// Create safety backup before risky operations
func (m *mergeStrategyManager) createBackup(ctx context.Context, repo *repository.Repository) (string, error) {
    // Get current HEAD
    result, err := m.executor.Run(ctx, repo.Path, "rev-parse", "HEAD")
    if err != nil {
        return "", err
    }

    headRef := strings.TrimSpace(result.Stdout)

    // Create backup branch
    backupName := fmt.Sprintf("backup/%s/%d", headRef[:7], time.Now().Unix())
    _, err = m.executor.Run(ctx, repo.Path, "branch", backupName)
    if err != nil {
        return "", err
    }

    return backupName, nil
}
```

______________________________________________________________________

## CLI Integration (Deferred to Phase 6)

### Command Structure

```bash
# Detect conflicts
gzh-git merge detect <source> <target>

# Merge with strategy
gzh-git merge <source> [--strategy=<strategy>] [--no-ff]

# Abort merge
gzh-git merge abort

# Rebase
gzh-git rebase <upstream> [--interactive]

# Continue rebase
gzh-git rebase continue

# Abort rebase
gzh-git rebase abort
```

______________________________________________________________________

## Dependencies

### Internal

- `pkg/repository` - Repository operations
- `pkg/branch` - Branch management
- `internal/gitcmd` - Git command execution

### External

- Standard library only (no external dependencies)

______________________________________________________________________

## Success Criteria

1. ✅ All core interfaces implemented
1. ✅ Comprehensive unit tests (≥85% coverage)
1. ✅ Conflict detection accurate (>95%)
1. ✅ Safe auto-resolution (zero data loss)
1. ✅ Merge strategies work correctly
1. ✅ Rebase operations safe and reliable
1. ✅ Documentation and examples complete

______________________________________________________________________

## Implementation Checklist

### Phase 5.1: Conflict Detector

- [ ] Define types.go (Conflict, ConflictReport, DetectOptions)
- [ ] Define errors.go (merge-specific errors)
- [ ] Implement detector.go (ConflictDetector interface)
- [ ] Write detector_test.go (unit tests)
- [ ] Validation: All tests passing, ≥85% coverage

### Phase 5.2: Merge Strategy Manager

- [ ] Add MergeStrategy types to types.go
- [ ] Implement strategy.go (MergeStrategyManager interface)
- [ ] Write strategy_test.go (unit tests)
- [ ] Validation: All tests passing, ≥85% coverage

### Phase 5.3: Rebase Manager

- [ ] Add Rebase types to types.go
- [ ] Implement rebase.go (RebaseManager interface)
- [ ] Write rebase_test.go (unit tests)
- [ ] Validation: All tests passing, ≥85% coverage

### Phase 5.4: Conflict Resolver (Optional)

- [ ] Add Resolution types to types.go
- [ ] Implement resolver.go (ConflictResolver interface)
- [ ] Write resolver_test.go (unit tests)
- [ ] Validation: All tests passing, ≥85% coverage

### Phase 5.5: Integration

- [ ] Update specs/00-overview.md
- [ ] Update PROJECT_STATUS.md
- [ ] Create docs/phase-5-completion.md
- [ ] Run full test suite
- [ ] Validation: All tests passing, documentation complete

______________________________________________________________________

## Timeline

- **Phase 5.1**: 1-2 days (Conflict Detector)
- **Phase 5.2**: 1-2 days (Merge Strategy Manager)
- **Phase 5.3**: 1-2 days (Rebase Manager)
- **Phase 5.4**: 1 day (Conflict Resolver - optional)
- **Phase 5.5**: 1 day (Integration & Documentation)

**Total Estimated**: 5-8 days

______________________________________________________________________

## References

- [Git Merge Documentation](https://git-scm.com/docs/git-merge)
- [Git Rebase Documentation](https://git-scm.com/docs/git-rebase)
- [Git Merge Strategies](https://git-scm.com/docs/merge-strategies)
- Phase 3: Branch Management (Complete)
- Phase 4: History Analysis (Complete)

______________________________________________________________________

**Last Updated**: 2025-11-27
**Version**: 1.0
