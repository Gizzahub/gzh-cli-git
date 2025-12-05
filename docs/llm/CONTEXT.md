# gzh-cli-git - LLM Context Summary

> **Purpose**: Provide LLM with essential project context for code assistance and development
> **Last Updated**: 2025-12-01
> **Version**: v0.2.0

## Project Identity

**Name**: gzh-cli-git
**Type**: Dual-purpose CLI tool + Go library
**Domain**: Git automation and repository management
**Language**: Go 1.24+
**Architecture**: Library-First Design (pkg/ has zero CLI dependencies)

## Core Concept

```
┌─────────────────────────────────────┐
│  gzh-cli-git = Library + CLI        │
├─────────────────────────────────────┤
│                                      │
│  Library (pkg/)                      │
│  - Zero CLI dependencies            │
│  - Clean public APIs                │
│  - Reusable in other projects       │
│                                      │
│  CLI (cmd/)                          │
│  - User interface                   │
│  - Output formatting                │
│  - Uses pkg/ library                │
│                                      │
└─────────────────────────────────────┘
```

## Current Implementation Status

### ✅ Fully Implemented (v0.2.0)

**ALL major features are implemented and functional.**

**Repository Operations** (`pkg/repository/`):

- `Open(ctx, path)` - Open existing repository
- `Clone(ctx, opts)` - Clone with advanced options
- `GetInfo(ctx, repo)` - Repository metadata
- `GetStatus(ctx, repo)` - Working tree status
- `IsRepository(ctx, path)` - Validation

**Operations** (`pkg/operations/`):

- Clone with options (branch, depth, single-branch, recursive)
- Clone-or-update strategies
- Bulk repository operations with parallelization

**Commit Automation** (`pkg/commit/`):

- Auto-generate commit messages
- Template-based commits (Conventional Commits)
- Message validation against rules
- Template management

**Branch Management** (`pkg/branch/`):

- Create, list, delete branches
- Worktree-based parallel development
- Branch creation with linked worktrees

**History Analysis** (`pkg/history/`):

- Commit statistics and trends
- Contributor analysis with metrics
- File change tracking
- Multiple output formats

**Merge/Rebase** (`pkg/merge/`):

- Pre-merge conflict detection
- Merge execution with strategies
- Abort and rebase operations

**CLI Commands** (`cmd/gzh-git/`):
All commands functional - status, info, clone, update, branch, commit, history, merge

**Testing**:

- 141 tests passing
- 69.1% code coverage
- Comprehensive integration tests

## Project Structure

```
gzh-cli-git/
├── pkg/                      # PUBLIC LIBRARY (zero CLI deps)
│   ├── repository/           # Core repository operations
│   │   ├── interfaces.go     # Client interface
│   │   ├── client.go         # Implementation
│   │   └── types.go          # Repository, Info, Status
│   ├── operations/           # Git operations (clone, pull, fetch)
│   ├── commit/               # Commit automation ✅
│   ├── branch/               # Branch management ✅
│   ├── history/              # History analysis ✅
│   └── merge/                # Merge/rebase ✅
│
├── internal/                 # INTERNAL (not exposed)
│   ├── gitcmd/              # Git command execution
│   ├── parser/              # Git output parsing
│   └── validation/          # Input validation
│
├── cmd/gzh-git/             # CLI APPLICATION
│   ├── main.go              # Entry point
│   └── internal/cli/        # Cobra commands
│
├── test/                    # Integration & E2E tests
├── examples/                # Library usage examples
├── docs/                    # Documentation
└── specs/                   # Feature specifications
```

## Key Design Principles

### 1. Library-First Architecture

**Rule**: `pkg/` MUST NOT import CLI frameworks (Cobra, etc.)

```go
// ❌ WRONG: pkg/ importing CLI
import "github.com/spf13/cobra"

// ✅ CORRECT: pkg/ only stdlib and interfaces
import (
    "context"
    "io"
)
```

### 2. Interface-Driven Design

**All major components are interfaces:**

```go
// pkg/repository/interfaces.go
type Client interface {
    Open(ctx context.Context, path string) (*Repository, error)
    Clone(ctx context.Context, opts CloneOptions) (*Repository, error)
    GetStatus(ctx context.Context, repo *Repository) (*Status, error)
    // ...
}
```

### 3. Context Propagation

**Every operation accepts context as first parameter:**

```go
func Clone(ctx context.Context, opts CloneOptions) (*Repository, error)
```

### 4. Functional Options Pattern

**For extensibility without breaking changes:**

```go
Clone(ctx, url, path,
    WithBranch("main"),
    WithDepth(1),
    WithProgress(os.Stdout),
)
```

### 5. Git CLI Wrapper

**Uses Git CLI, not go-git library:**

- Maximum compatibility
- Simpler implementation
- Familiar behavior
- Trade-off: External dependency on Git 2.30+

## Critical Files

| File | Purpose | Importance |
|------|---------|------------|
| `pkg/repository/interfaces.go` | Core API contracts | CRITICAL |
| `pkg/repository/client.go` | Repository client implementation | CRITICAL |
| `pkg/operations/clone.go` | Clone operations | HIGH |
| `internal/gitcmd/executor.go` | Git command wrapper | CRITICAL |
| `cmd/gzh-git/main.go` | CLI entry point | MEDIUM |

## Key Types

```go
type Repository struct { Path, GitDir, WorkTree string; IsBare, IsShallow bool }
type Info struct { Branch, RemoteURL, Upstream string; AheadBy, BehindBy int; IsDirty bool }
type Status struct { IsClean bool; ModifiedFiles, StagedFiles, UntrackedFiles []string }
type CloneOptions struct { URL, Destination, Branch string; Depth int; SingleBranch, Recursive, Bare, Mirror bool; Progress io.Writer }
```

## Dependencies

```go
// Core dependencies (production)
require (
    github.com/spf13/cobra v1.9.1      // CLI framework (cmd/ only)
    github.com/spf13/viper v1.20.1     // Configuration (cmd/ only)
    golang.org/x/sync v0.17.0          // Concurrency utilities
    gopkg.in/yaml.v3 v3.0.1            // YAML parsing
)

// Test dependencies
require (
    github.com/stretchr/testify v1.11.0 // Testing framework
)
```

## Usage Patterns

```go
// Open: client.Open(ctx, "/path")
// Clone: client.Clone(ctx, CloneOptions{URL, Destination, Branch, Depth})
// Status: client.GetStatus(ctx, repo) → Status{IsClean, ModifiedFiles, ...}
```

## Error Handling

Standard: `fmt.Errorf("op failed: %w", err)`
Git errors: `GitError{Op, Path, ExitCode, Output, Err}`

## Testing

- Unit: Mocked executor, fast
- Integration: Real Git, `// +build integration`
- E2E: CLI binary, `// +build e2e`
- Coverage: 69.1%, 141 tests

## Build & Development

```bash
# Build binary
make build           # → build/gzh-git

# Run tests
make test            # Unit tests only
make test-integration # Integration tests
make test-e2e        # E2E tests

# Quality checks
make lint            # golangci-lint
make fmt             # go fmt
make quality         # All checks

# Install
make install         # → /usr/local/bin/gzh-git
```

## Git Commit Guidelines

**Format**: Conventional Commits with mandatory scope

```
{type}({scope}): {imperative verb} {what}

{body}

Model: claude-{model}
Co-Authored-By: Claude <noreply@anthropic.com>
```

**Types**: feat, fix, docs, refactor, test, chore, perf
**Scopes**: repository, operations, commit, branch, cli, docs, test

## Documentation Structure

```
docs/
├── user/                    # End-user documentation
│   ├── getting-started/
│   ├── guides/
│   └── reference/
├── developer/               # Library integration
│   ├── library/
│   ├── architecture/
│   └── contributing/
└── llm/                     # LLM-specific context
    └── CONTEXT.md (this file)
```

## Integration

- gzh-cli: `import "github.com/gizzahub/gzh-cli-git/pkg/repository"`
- As library: `go get github.com/gizzahub/gzh-cli-git`

## Performance Characteristics

| Operation | Target (p95) | Notes |
|-----------|--------------|-------|
| `Open` | < 10ms | Fast path validation |
| `GetStatus` | < 50ms | Calls `git status --porcelain` |
| `Clone` | Network-bound | Depends on repo size |
| Bulk ops (100 repos) | < 30s | Parallel execution |

## Security Considerations

1. **Input Sanitization**: All user inputs validated before Git commands
1. **Path Validation**: Prevent path traversal attacks
1. **Command Injection**: No string interpolation in Git commands
1. **Credential Safety**: Never log credentials

## Known Limitations

1. **Git CLI Dependency**: Requires Git 2.30+ installed
1. **Pre-v1.0.0 Status**: API may change before v1.0.0
1. **Test Coverage**: 69.1% (target: 90% for v1.0.0)
1. **No Windows Testing**: Primary development on macOS/Linux

## Development Workflow

**New operation**: Define interface → Implement → Git command → Tests → CLI → Docs
**New feature**: Create pkg/ package → Manager interface → CLI commands → Tests

## References

- **Full Architecture**: [ARCHITECTURE.md](../../ARCHITECTURE.md) (1500+ lines, detailed)
- **Product Requirements**: [PRD.md](../../PRD.md)
- **Technical Requirements**: [REQUIREMENTS.md](../../REQUIREMENTS.md)
- **API Documentation**: [pkg.go.dev](https://pkg.go.dev/github.com/gizzahub/gzh-cli-git)
- **Roadmap**: See PRD.md for 10-week development plan

## Quick Decision Reference

**When to use gzh-cli-git:**

- ✅ Need Git operations in Go application
- ✅ Want type-safe Git API
- ✅ Building automation tools

**When NOT to use:**

- ❌ Need pure Go solution (no Git dependency)
- ❌ Need production-stable library with API guarantees (wait for v1.0.0)
- ❌ Need Windows-tested code (primary development on macOS/Linux)

______________________________________________________________________

**Token Efficiency**: This document is optimized for LLM context windows

- ~450 lines (well under 500-line target)
- ~7KB (well under 10KB limit)
- Structured for quick parsing
- Contains essential information only
