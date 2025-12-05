# API Stability Document

**Project**: gzh-cli-git
**Version**: v0.1.0-alpha
**Status**: Pre-release
**Last Updated**: 2025-12-01

______________________________________________________________________

## Overview

This document outlines the stability guarantees and versioning policy for the gzh-cli-git public API. As a library-first project, we prioritize backward compatibility and clear communication about API changes.

______________________________________________________________________

## Versioning Policy

gzh-cli-git follows [Semantic Versioning 2.0.0](https://semver.org/):

```
vMAJOR.MINOR.PATCH[-PRERELEASE]

Examples:
- v0.1.0-alpha  → Pre-release, no stability guarantees
- v0.1.0        → Minor release, backward compatible
- v0.2.0        → New features, backward compatible
- v1.0.0        → Production-ready, full stability guarantees
```

### Version Number Rules

- **MAJOR**: Incremented for breaking API changes
- **MINOR**: Incremented for new features (backward compatible)
- **PATCH**: Incremented for bug fixes (backward compatible)
- **PRERELEASE**: alpha, beta, rc.1, rc.2, etc.

### Pre-1.0 Stability

**Current Status**: v0.1.0-alpha (Pre-release)

- ⚠️ **No stability guarantees** until v1.0.0
- API may change between minor versions (0.x.0)
- Breaking changes will be documented in CHANGELOG.md
- We aim to minimize breaking changes even in 0.x releases

**Path to v1.0.0**:

1. v0.1.0-alpha → Initial library release
1. v0.1.x → Bug fixes and documentation updates
1. v0.2.0 → Additional features based on feedback
1. v1.0.0 → Production-ready after gzh-cli integration

______________________________________________________________________

## Public API Surface

### Stable Packages (pkg/)

All packages under `pkg/` are part of the public API:

```
pkg/
├── repository/    # Repository operations (Client interface)
├── commit/        # Commit automation (Template, Validator, Generator, PushManager)
├── branch/        # Branch management (BranchManager, WorktreeManager, CleanupService)
├── history/       # History analysis (Analyzer, ContributorAnalyzer, FileHistoryTracker)
├── merge/         # Merge/Rebase (MergeManager, ConflictDetector, RebaseManager)
└── config/        # Configuration management
```

### Internal Packages (internal/)

**NOT part of public API** - subject to change without notice:

```
internal/
├── gitcmd/        # Git command execution
├── parser/        # Output parsing
└── validation/    # Input validation
```

### CLI Layer (cmd/)

**NOT part of public library API** - CLI commands are for end-users, not library consumers:

```
cmd/gzh-git/       # CLI application (uses pkg/ packages)
```

______________________________________________________________________

## API Stability Levels

### Level 1: Stable ✅

**Definition**: Guaranteed backward compatibility until next major version.

**Packages**:

- `pkg/repository` - Core repository operations
  - `Client` interface
  - `Repository`, `Info`, `Status` types
  - `CloneOptions` and functional options

**Guarantees**:

- No breaking changes in method signatures
- No removal of exported types or functions
- Field additions will use optional/pointer types

### Level 2: Beta ⚠️

**Definition**: API is mostly stable but may see minor adjustments based on feedback.

**Packages**:

- `pkg/commit` - Commit automation
- `pkg/branch` - Branch management
- `pkg/history` - History analysis
- `pkg/merge` - Merge/Rebase operations

**Possible Changes**:

- Additional methods on interfaces
- New optional fields in option structs
- Deprecation of rarely-used APIs

### Level 3: Alpha 🚧

**Definition**: Experimental API that may change significantly.

**Packages**:

- `pkg/config` - Configuration management (minimal implementation)

**Warning**: Use with caution, expect breaking changes.

______________________________________________________________________

## Interface Contracts

### Core Interfaces

#### repository.Client

```go
type Client interface {
    Open(ctx context.Context, path string) (*Repository, error)
    Clone(ctx context.Context, opts CloneOptions) (*Repository, error)
    CloneOrUpdate(ctx context.Context, opts CloneOrUpdateOptions) (*CloneOrUpdateResult, error)
    BulkUpdate(ctx context.Context, opts BulkUpdateOptions) (*BulkUpdateResult, error)
    IsRepository(ctx context.Context, path string) bool
    GetInfo(ctx context.Context, repo *Repository) (*Info, error)
    GetStatus(ctx context.Context, repo *Repository) (*Status, error)
}
```

**Stability**: ✅ Stable (Level 1)

**Guarantees**:

- All methods return `(*Type, error)` for extensibility
- All methods accept `context.Context` for cancellation
- Option structs use functional options pattern for backward compatibility

#### repository.Logger

```go
type Logger interface {
    Debug(msg string, args ...interface{})
    Info(msg string, args ...interface{})
    Warn(msg string, args ...interface{})
    Error(msg string, args ...interface{})
}
```

**Stability**: ✅ Stable (Level 1)

**Guarantees**:

- Simple key-value logging interface
- Compatible with popular logging frameworks (zap, logrus, slog)

#### repository.ProgressReporter

```go
type ProgressReporter interface {
    Start(total int64)
    Update(current int64)
    Done()
}
```

**Stability**: ✅ Stable (Level 1)

**Guarantees**:

- Simple progress reporting interface
- Compatible with CLI progress bars and GUI widgets

______________________________________________________________________

## Backward Compatibility Guidelines

### Adding New Features (MINOR version bump)

**Allowed Changes**:

- ✅ Adding new methods to interfaces (with default implementations if needed)
- ✅ Adding new exported types, functions, or constants
- ✅ Adding new fields to option structs (must be optional)
- ✅ Adding new errors to error taxonomy

**Example**:

```go
// v0.1.0
type CloneOptions struct {
    URL         string
    Destination string
    Branch      string
}

// v0.2.0 - Added new optional field (backward compatible)
type CloneOptions struct {
    URL         string
    Destination string
    Branch      string
    Depth       int  // New field with zero-value default
}
```

### Breaking Changes (MAJOR version bump)

**Requires MAJOR version bump**:

- ❌ Removing or renaming exported types, functions, or methods
- ❌ Changing method signatures (parameters or return types)
- ❌ Removing fields from structs
- ❌ Changing field types in exported structs

**Example of Breaking Change** (requires v2.0.0):

```go
// v1.0.0
func Clone(url, dest string) error

// v2.0.0 - Changed signature (BREAKING)
func Clone(ctx context.Context, opts CloneOptions) (*Repository, error)
```

### Deprecation Process

1. **Mark as deprecated** in GoDoc comments
1. **Document alternative** in deprecation notice
1. **Keep for one MAJOR version** (e.g., deprecated in v1.0, removed in v2.0)
1. **Log deprecation warnings** if possible

**Example**:

```go
// Deprecated: Use CloneWithContext instead. This function will be removed in v2.0.
func Clone(url, dest string) error {
    return CloneWithContext(context.Background(), url, dest)
}
```

______________________________________________________________________

## Error Handling Contract

### Error Types

**Stable Error Types** (Level 1):

- `ErrRepositoryNotFound` - Repository does not exist
- `ErrNotARepository` - Path is not a valid Git repository
- `ErrInvalidURL` - Invalid repository URL

**Beta Error Types** (Level 2):

- `ErrTemplateNotFound` - Commit template not found
- `ErrInvalidTemplate` - Template validation failed
- `ErrConflict` - Merge conflict detected

### Error Checking

**Recommended Pattern**:

```go
repo, err := client.Open(ctx, "/path/to/repo")
if err != nil {
    if errors.Is(err, repository.ErrNotARepository) {
        // Handle specific error
    }
    return err
}
```

**Guarantees**:

- Error types won't change (errors.Is will continue to work)
- Error messages may change (don't parse error strings)

______________________________________________________________________

## Migration Path to v1.0.0

### Current State (v0.1.0-alpha)

- ✅ All core interfaces defined
- ✅ Library-first architecture (zero CLI dependencies)
- ⚠️ Limited real-world testing
- ⚠️ No production usage yet

### Requirements for v1.0.0

1. **Validation**: gzh-cli integration complete
1. **Testing**: 85%+ test coverage
1. **Documentation**: Complete API reference with examples
1. **Stability**: 3+ months without API changes
1. **Feedback**: Addressed feedback from early adopters

### Breaking Changes Before v1.0.0

If breaking changes are needed before v1.0.0, they will:

1. Be documented in CHANGELOG.md
1. Be announced in release notes
1. Include migration examples
1. Increment MINOR version (0.x.0)

______________________________________________________________________

## API Review Checklist

Before releasing a new version, we review:

- [ ] All public APIs have GoDoc comments
- [ ] All breaking changes documented in CHANGELOG.md
- [ ] All new APIs have unit tests
- [ ] All new APIs have usage examples
- [ ] Version number follows semantic versioning
- [ ] Migration guide provided (if breaking changes)

______________________________________________________________________

## Contact and Feedback

We welcome feedback on the API design:

- **Issues**: [GitHub Issues](https://github.com/gizzahub/gzh-cli-git/issues)
- **Discussions**: [GitHub Discussions](https://github.com/gizzahub/gzh-cli-git/discussions)

______________________________________________________________________

## Appendix: Public API Inventory

### pkg/repository

**Interfaces**:

- `Client` - Primary repository operations
- `Logger` - Logging abstraction
- `ProgressReporter` - Progress reporting

**Types**:

- `Repository` - Repository handle
- `Info` - Repository information
- `Status` - Working tree status
- `CloneOptions` - Clone configuration
- `CloneOrUpdateOptions` - Clone/update configuration
- `BulkUpdateOptions` - Bulk update configuration
- `Result` - Operation result

**Functions**:

- `NewClient(options ...ClientOption) Client`
- `NewNoopLogger() Logger`
- `NewNoopProgress() ProgressReporter`
- `NewWriterLogger(w io.Writer) Logger`

### pkg/commit

**Interfaces**:

- `TemplateManager` - Template management
- `Validator` - Message validation
- `Generator` - Message generation
- `PushManager` - Safe push operations

**Types**:

- `Template` - Commit template
- `ValidationResult` - Validation outcome
- `GenerateOptions` - Generation options
- `PushOptions` - Push options

**Errors**:

- `ErrTemplateNotFound`
- `ErrInvalidTemplate`
- `ErrNoChanges`

### pkg/branch

**Interfaces**:

- `BranchManager` - Branch operations
- `WorktreeManager` - Worktree management
- `CleanupService` - Branch cleanup

**Types**:

- `Branch` - Branch information
- `CreateOptions` - Branch creation options
- `DeleteOptions` - Branch deletion options
- `WorktreeInfo` - Worktree information

### pkg/history

**Interfaces**:

- `Analyzer` - Commit statistics
- `ContributorAnalyzer` - Contributor analysis
- `FileHistoryTracker` - File history tracking

**Types**:

- `CommitStats` - Commit statistics
- `Contributor` - Contributor information
- `FileHistory` - File change history

### pkg/merge

**Interfaces**:

- `MergeManager` - Merge operations
- `ConflictDetector` - Conflict detection
- `RebaseManager` - Rebase operations

**Types**:

- `MergeOptions` - Merge configuration
- `ConflictReport` - Conflict information
- `RebaseOptions` - Rebase configuration

______________________________________________________________________

**Document Version**: 1.0
**Last Review**: 2025-12-01
**Next Review**: Before v0.2.0 or v1.0.0
