# Architecture Design Document

**Project**: gzh-cli-git
**Version**: 1.0
**Last Updated**: 2025-11-27
**Status**: Draft

______________________________________________________________________

## Table of Contents

1. [Executive Summary](#1-executive-summary)
1. [Architectural Overview](#2-architectural-overview)
1. [Design Principles](#3-design-principles)
1. [Component Design](#4-component-design)
1. [Interface Contracts](#5-interface-contracts)
1. [Data Flow](#6-data-flow)
1. [Error Handling Strategy](#7-error-handling-strategy)
1. [Testing Architecture](#8-testing-architecture)
1. [Performance Considerations](#9-performance-considerations)
1. [Security Architecture](#10-security-architecture)
1. [Deployment Architecture](#11-deployment-architecture)
1. [Design Decisions](#12-design-decisions)

______________________________________________________________________

## 1. Executive Summary

### 1.1 Architectural Goals

gzh-cli-git adopts a **Library-First Architecture** with the following goals:

1. **Dual-Purpose Design**: Function as both standalone CLI and reusable Go library
1. **Clean Separation**: Zero coupling between library code (`pkg/`) and CLI code (`cmd/`)
1. **Maximum Reusability**: Enable easy integration into gzh-cli and other projects
1. **Interface-Driven**: All core functionality via well-defined interfaces
1. **Testability**: 100% mockable components for comprehensive testing

### 1.2 Key Architectural Decisions

| Decision | Rationale | Trade-offs |
|----------|-----------|------------|
| Library-First over CLI-First | Enables reuse in gzh-cli; better API design | More upfront design effort |
| Git CLI over go-git library | Maximum compatibility; simpler | External dependency on Git |
| Interfaces over concrete types | Testability; extensibility | More files, indirection |
| Functional options pattern | API extensibility without breaking changes | More boilerplate |
| Context propagation | Cancellation, timeouts, request-scoped values | Every function signature includes ctx |

______________________________________________________________________

## 2. Architectural Overview

### 2.1 High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    gzh-cli-git System                        │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌────────────────────────────────────────────────────┐     │
│  │              CLI Layer (cmd/)                       │     │
│  │  ┌──────────┐  ┌─────────┐  ┌──────────┐          │     │
│  │  │ Commands │  │ Output  │  │    UI    │          │     │
│  │  │  (Cobra) │  │ Format  │  │ Progress │          │     │
│  │  └────┬─────┘  └────┬────┘  └─────┬────┘          │     │
│  └───────┼─────────────┼──────────────┼───────────────┘     │
│          │             │              │                      │
│  ┌───────┼─────────────┼──────────────┼───────────────┐     │
│  │       ▼             ▼              ▼               │     │
│  │          Public Library API (pkg/)                 │     │
│  │  ┌──────────┐  ┌────────┐  ┌────────┐  ┌────────┐ │     │
│  │  │Repository│  │ Commit │  │ Branch │  │ History│ │     │
│  │  │  Client  │  │Manager │  │Manager │  │Analyzer│ │     │
│  │  └────┬─────┘  └───┬────┘  └───┬────┘  └───┬────┘ │     │
│  └───────┼────────────┼───────────┼───────────┼──────┘     │
│          │            │           │           │             │
│  ┌───────┼────────────┼───────────┼───────────┼──────┐     │
│  │       ▼            ▼           ▼           ▼      │     │
│  │      Internal Implementation (internal/)          │     │
│  │  ┌─────────┐  ┌─────────┐  ┌────────────┐        │     │
│  │  │ Git CMD │  │ Parsers │  │ Validation │        │     │
│  │  │Executor │  │ (status,│  │  (input)   │        │     │
│  │  │         │  │log,diff)│  │            │        │     │
│  │  └────┬────┘  └────┬────┘  └─────┬──────┘        │     │
│  └───────┼────────────┼──────────────┼───────────────┘     │
│          │            │              │                      │
│  ┌───────┼────────────┼──────────────┼───────────────┐     │
│  │       ▼            ▼              ▼               │     │
│  │              External Dependencies                │     │
│  │  ┌─────────┐  ┌────────────┐  ┌──────────┐      │     │
│  │  │ Git CLI │  │ Filesystem │  │   OS     │      │     │
│  │  │ (2.30+) │  │   (I/O)    │  │(exec,env)│      │     │
│  │  └─────────┘  └────────────┘  └──────────┘      │     │
│  └───────────────────────────────────────────────────┘     │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 Layer Responsibilities

**CLI Layer (`cmd/`):**

- User interaction (prompts, confirmations)
- Command parsing (Cobra)
- Output formatting (table, JSON)
- Progress reporting
- Configuration file management

**Library Layer (`pkg/`):**

- Core Git operations
- Business logic (commit automation, conflict resolution)
- Public APIs for external consumers
- NO CLI dependencies (no Cobra, no fmt.Println)

**Internal Layer (`internal/`):**

- Git command execution
- Output parsing (status, log, diff)
- Input validation and sanitization
- Shared utilities (not exposed)

**External Layer:**

- Git CLI binary (system installation)
- Filesystem I/O
- Operating system (exec, environment)

______________________________________________________________________

## 3. Design Principles

### 3.1 SOLID Principles

**Single Responsibility:**

- Each package has one clear purpose
- `pkg/commit/` only handles commits
- `internal/gitcmd/` only executes Git commands

**Open/Closed:**

- Interfaces open for extension
- Functional options allow new parameters without breaking API

**Liskov Substitution:**

- All interface implementations are substitutable
- Mocks can replace real implementations

**Interface Segregation:**

- Small, focused interfaces (not god interfaces)
- `CommitManager`, `BranchManager` separate, not combined

**Dependency Inversion:**

- Depend on interfaces, not concretions
- Accept `Logger` interface, not `*zap.Logger`

### 3.2 Library-First Principles

**P1: Zero CLI Dependencies in pkg/**

```go
// ❌ WRONG: pkg/ code importing CLI framework
import "github.com/spf13/cobra"

// ✅ CORRECT: pkg/ only uses stdlib and interfaces
import (
    "context"
    "io"
)
```

**P2: Dependency Injection via Interfaces**

```go
// ❌ WRONG: Hard-coded logger
func Process() {
    log.Println("processing...")
}

// ✅ CORRECT: Logger injected via interface
func Process(ctx context.Context, logger Logger) {
    logger.Info("processing...")
}
```

**P3: Context Propagation**

```go
// ❌ WRONG: No context
func Clone(url, path string) error

// ✅ CORRECT: Context as first parameter
func Clone(ctx context.Context, url, path string) error
```

### 3.3 Go Idioms

**Functional Options Pattern:**

```go
type CloneOption func(*CloneConfig)

func WithBranch(b string) CloneOption {
    return func(c *CloneConfig) { c.Branch = b }
}

// Usage allows evolution without breaking changes
Clone(ctx, url, path, WithBranch("main"), WithDepth(1))
```

**Error Wrapping:**

```go
if err != nil {
    return fmt.Errorf("failed to clone repository: %w", err)
}
```

**Interface Satisfaction:**

```go
var _ CommitManager = (*commitManager)(nil) // Compile-time check
```

______________________________________________________________________

## 4. Component Design

### 4.1 Directory Structure

```
gzh-cli-git/
├── pkg/                          # PUBLIC API
│   ├── repository/               # Core repo operations
│   │   ├── interfaces.go         # Repository, Client interfaces
│   │   ├── client.go             # Client implementation
│   │   ├── types.go              # Repository, Info, Status types
│   │   └── options.go            # Functional options
│   ├── operations/               # Basic Git operations
│   │   ├── clone.go              # Clone with options
│   │   ├── pull.go               # Pull with strategies
│   │   ├── fetch.go              # Fetch operations
│   │   └── bulk.go               # Bulk operations
│   ├── commit/                   # Commit automation
│   │   ├── interfaces.go         # CommitManager interface
│   │   ├── manager.go            # Manager implementation
│   │   ├── template.go           # Template system
│   │   ├── auto.go               # Auto-commit logic
│   │   ├── smart_push.go         # Smart push operations
│   │   └── types.go              # CommitOptions, Result, etc.
│   ├── branch/                   # Branch management
│   │   ├── interfaces.go         # BranchManager interface
│   │   ├── manager.go            # Manager implementation
│   │   ├── worktree.go           # Worktree operations
│   │   ├── workflow.go           # Parallel workflows
│   │   └── types.go              # Branch, Worktree, etc.
│   ├── history/                  # Git history analysis
│   │   ├── interfaces.go         # Analyzer interface
│   │   ├── analyzer.go           # Analyzer implementation
│   │   ├── stats.go              # Statistics calculation
│   │   ├── contributors.go       # Contributor analysis
│   │   └── types.go              # Commit, Statistics, etc.
│   ├── merge/                    # Advanced merge/rebase
│   │   ├── interfaces.go         # MergeManager interface
│   │   ├── manager.go            # Manager implementation
│   │   ├── conflict.go           # Conflict detection
│   │   ├── strategies.go         # Resolution strategies
│   │   └── types.go              # ConflictReport, etc.
│   └── config/                   # Configuration
│       ├── config.go             # Config struct
│       └── validation.go         # Config validation
│
├── internal/                     # INTERNAL (not exposed)
│   ├── gitcmd/                   # Git command execution
│   │   ├── executor.go           # Command executor
│   │   ├── sanitize.go           # Input sanitization
│   │   └── errors.go             # Error types
│   ├── parser/                   # Git output parsing
│   │   ├── status.go             # Parse git status
│   │   ├── log.go                # Parse git log
│   │   ├── diff.go               # Parse git diff
│   │   └── common.go             # Shared parsing utilities
│   └── validation/               # Input validation
│       ├── validator.go          # Validation logic
│       └── patterns.go           # Regex patterns
│
├── cmd/                          # CLI APPLICATION
│   └── gzh-git/                  # Binary: gzh-git
│       ├── main.go               # Entry point
│       ├── root.go               # Root command
│       └── internal/             # CLI-specific (not reusable)
│           ├── cli/              # Cobra commands
│           │   ├── commit/       # Commit commands
│           │   │   ├── commit.go
│           │   │   ├── auto.go
│           │   │   └── template.go
│           │   ├── branch/       # Branch commands
│           │   ├── history/      # History commands
│           │   └── merge/        # Merge commands
│           ├── output/           # Output formatting
│           │   ├── table.go      # Table renderer
│           │   ├── json.go       # JSON formatter
│           │   └── formatter.go  # Common interface
│           └── ui/               # User interface
│               ├── progress.go   # Progress bars
│               └── prompt.go     # User prompts
│
├── examples/                     # Library usage examples
│   ├── basic/                    # Basic usage
│   ├── commit_automation/        # Commit features
│   └── gzh_cli_integration/      # gzh-cli integration
│
├── test/                         # Integration & E2E tests
│   ├── integration/              # Integration tests
│   └── e2e/                      # End-to-end tests
│
└── configs/                      # Default configurations
    └── templates/                # Commit templates
        ├── conventional.yaml
        └── semantic.yaml
```

### 4.2 Component Diagram

```
┌─────────────────────────────────────────────────────────┐
│                     pkg/repository                       │
│  ┌─────────────────────────────────────────────────┐   │
│  │ Client (interface)                               │   │
│  │  - Open(ctx, path) (*Repository, error)         │   │
│  │  - Clone(ctx, opts) (*Repository, error)        │   │
│  │  - GetStatus(ctx, repo) (*Status, error)        │   │
│  └──────────────────┬───────────────────────────────┘   │
│                     │ implements                         │
│  ┌──────────────────▼───────────────────────────────┐   │
│  │ client (struct)                                   │   │
│  │  - executor: *gitcmd.Executor                    │   │
│  │  - logger: Logger                                │   │
│  └───────────────────────────────────────────────────┘   │
└──────────────────────┬────────────────────────────────┘
                       │ uses
        ┌──────────────▼────────────────┐
        │   internal/gitcmd/Executor    │
        │  - Run(ctx, dir, args)        │
        └───────────────┬────────────────┘
                        │ executes
            ┌───────────▼───────────┐
            │   Git CLI (external)   │
            └────────────────────────┘
```

______________________________________________________________________

## 5. Interface Contracts

### 5.1 Core Repository Interface

```go
// pkg/repository/interfaces.go
package repository

import (
    "context"
    "io"
)

// Client defines core repository operations
type Client interface {
    // Repository lifecycle
    Open(ctx context.Context, path string) (*Repository, error)
    Clone(ctx context.Context, opts CloneOptions) (*Repository, error)
    IsRepository(ctx context.Context, path string) bool

    // Repository inspection
    GetInfo(ctx context.Context, repo *Repository) (*Info, error)
    GetStatus(ctx context.Context, repo *Repository) (*Status, error)

    // Basic operations (delegated to specific managers)
    // Use CommitManager, BranchManager, etc. for advanced features
}

// Repository represents a Git repository handle
type Repository struct {
    Path      string // Absolute path to repository
    GitDir    string // Path to .git directory
    WorkTree  string // Working tree path (may differ for worktrees)
    IsBare    bool   // True if bare repository
    IsShallow bool   // True if shallow clone
}

// Logger interface for dependency injection
// Library consumers provide their own logger implementation
type Logger interface {
    Debug(msg string, args ...interface{})
    Info(msg string, args ...interface{})
    Warn(msg string, args ...interface{})
    Error(msg string, args ...interface{})
}

// ProgressReporter for long-running operations
type ProgressReporter interface {
    Start(total int64)
    Update(current int64)
    Done()
}
```

### 5.2 Commit Manager Interface

```go
// pkg/commit/interfaces.go
package commit

import (
    "context"
    "github.com/gizzahub/gzh-cli-git/pkg/repository"
)

// Manager handles commit operations
type Manager interface {
    // Manual commit operations
    Create(ctx context.Context, repo *repository.Repository, opts CommitOptions) (*Result, error)
    Amend(ctx context.Context, repo *repository.Repository, opts AmendOptions) (*Result, error)

    // Message generation
    GenerateMessage(ctx context.Context, repo *repository.Repository, template Template) (string, error)
    ValidateMessage(ctx context.Context, message string, rules ValidationRules) error

    // Automation
    AutoCommit(ctx context.Context, repo *repository.Repository, policy AutoCommitPolicy) (*Result, error)

    // Smart push
    SmartPush(ctx context.Context, repo *repository.Repository, opts PushOptions) (*Result, error)
}

// Template represents a commit message template
type Template struct {
    Name        string            `yaml:"name"`
    Type        string            `yaml:"type"`     // feat, fix, docs, etc.
    Scope       string            `yaml:"scope"`
    Subject     string            `yaml:"subject"`
    Body        string            `yaml:"body"`
    Footer      string            `yaml:"footer"`
    Variables   map[string]string `yaml:"variables"`
}

// CommitOptions configure commit creation
type CommitOptions struct {
    Message      string
    AllFiles     bool              // --all
    Amend        bool              // --amend
    NoVerify     bool              // --no-verify
    Template     *Template
    Variables    map[string]string // Template variable values
}

// Result contains commit operation result
type Result struct {
    Hash      string    // Commit SHA
    Message   string    // Commit message
    Author    string    // Author name <email>
    Timestamp time.Time // Commit timestamp
    Files     []string  // Modified files
}
```

### 5.3 Branch Manager Interface

```go
// pkg/branch/interfaces.go
package branch

import (
    "context"
    "github.com/gizzahub/gzh-cli-git/pkg/repository"
)

// Manager handles branch and worktree operations
type Manager interface {
    // Branch operations
    List(ctx context.Context, repo *repository.Repository, opts ListOptions) ([]Branch, error)
    Create(ctx context.Context, repo *repository.Repository, name string, opts CreateOptions) (*Branch, error)
    Delete(ctx context.Context, repo *repository.Repository, name string, force bool) error
    Checkout(ctx context.Context, repo *repository.Repository, name string, opts CheckoutOptions) error

    // Worktree operations
    ListWorktrees(ctx context.Context, repo *repository.Repository) ([]Worktree, error)
    AddWorktree(ctx context.Context, repo *repository.Repository, path string, opts WorktreeOptions) (*Worktree, error)
    RemoveWorktree(ctx context.Context, repo *repository.Repository, path string) error

    // Parallel workflow support
    CreateParallelWorkflow(ctx context.Context, repo *repository.Repository, config WorkflowConfig) (*Workflow, error)
}

// Branch represents a Git branch
type Branch struct {
    Name       string
    Hash       string
    IsHead     bool   // Current branch
    IsRemote   bool
    Upstream   string // Upstream branch (if any)
    AheadBy    int    // Commits ahead of upstream
    BehindBy   int    // Commits behind upstream
}

// Worktree represents a Git worktree
type Worktree struct {
    Path       string
    Branch     string
    Hash       string
    IsLocked   bool
    Reason     string // Lock reason
}
```

### 5.4 History Analyzer Interface

```go
// pkg/history/interfaces.go
package history

import (
    "context"
    "time"
    "github.com/gizzahub/gzh-cli-git/pkg/repository"
)

// Analyzer handles Git history analysis
type Analyzer interface {
    // Commit analysis
    GetCommits(ctx context.Context, repo *repository.Repository, opts QueryOptions) ([]Commit, error)
    GetCommitStats(ctx context.Context, repo *repository.Repository, opts StatsOptions) (*Statistics, error)

    // Contributor analysis
    GetContributors(ctx context.Context, repo *repository.Repository, opts ContributorOptions) ([]Contributor, error)
    GetContributorStats(ctx context.Context, repo *repository.Repository, email string) (*ContributorStats, error)

    // File history
    GetFileHistory(ctx context.Context, repo *repository.Repository, path string) ([]FileChange, error)
    GetDiff(ctx context.Context, repo *repository.Repository, opts DiffOptions) (*Diff, error)
}

// Commit represents a Git commit
type Commit struct {
    Hash      string
    Author    string
    Email     string
    Timestamp time.Time
    Message   string
    Files     []string
    Stats     CommitStats
}

// Statistics aggregates commit data
type Statistics struct {
    TotalCommits   int
    TotalAuthors   int
    CommitsByDay   map[string]int // Date -> count
    CommitsByAuthor map[string]int
    LinesAdded     int
    LinesRemoved   int
    TopFiles       []FileStats
}

// Contributor represents a repository contributor
type Contributor struct {
    Name        string
    Email       string
    Commits     int
    LinesAdded  int
    LinesRemoved int
    FirstCommit time.Time
    LastCommit  time.Time
}
```

### 5.5 Merge Manager Interface

```go
// pkg/merge/interfaces.go
package merge

import (
    "context"
    "github.com/gizzahub/gzh-cli-git/pkg/repository"
)

// Manager handles merge and rebase operations
type Manager interface {
    // Merge/rebase operations
    Merge(ctx context.Context, repo *repository.Repository, branch string, opts MergeOptions) (*Result, error)
    Rebase(ctx context.Context, repo *repository.Repository, upstream string, opts RebaseOptions) (*Result, error)

    // Conflict handling
    DetectConflicts(ctx context.Context, repo *repository.Repository, source, target string) (*ConflictReport, error)
    ResolveConflict(ctx context.Context, repo *repository.Repository, file string, strategy ResolutionStrategy) error
    AutoResolve(ctx context.Context, repo *repository.Repository, policy AutoResolvePolicy) (*ResolutionResult, error)
}

// ResolutionStrategy defines conflict resolution method
type ResolutionStrategy string

const (
    StrategyOurs     ResolutionStrategy = "ours"     // Keep our version
    StrategyTheirs   ResolutionStrategy = "theirs"   // Take their version
    StrategyUnion    ResolutionStrategy = "union"    // Combine both
    StrategyPatience ResolutionStrategy = "patience" // Patience algorithm
)

// ConflictReport contains conflict analysis
type ConflictReport struct {
    HasConflicts   bool
    ConflictCount  int
    ConflictingFiles []ConflictFile
    Resolvable     bool // True if auto-resolvable
}

// ConflictFile represents a file with conflicts
type ConflictFile struct {
    Path           string
    ConflictType   ConflictType
    OurVersion     string
    TheirVersion   string
    BaseVersion    string
    Difficulty     DifficultyLevel
}
```

______________________________________________________________________

## 6. Data Flow

### 6.1 Commit Automation Flow

```
User CLI Command
       │
       ▼
┌──────────────────┐
│ cmd/cli/commit   │  Parse CLI args, read config
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ pkg/commit       │  Validate options, prepare operation
│ Manager.Auto    │
│ Commit()         │
└────────┬─────────┘
         │
         ├──────────▶ Get Repository Status
         │            (pkg/repository/Client.GetStatus)
         │
         ├──────────▶ Analyze Staged Changes
         │            (internal logic)
         │
         ├──────────▶ Generate Commit Message
         │            (template + variables)
         │
         ├──────────▶ Validate Message
         │            (validation rules)
         │
         ▼
┌──────────────────┐
│ internal/gitcmd  │  Execute: git commit -m "message"
│ Executor.Run()   │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Git CLI          │  System git binary
│ (external)       │
└────────┬─────────┘
         │
         ▼
     Success/Error
         │
         ▼
┌──────────────────┐
│ pkg/commit       │  Parse output, create Result
│ Manager returns  │
│ Result           │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ cmd/cli/commit   │  Format output, display to user
│ Display Result   │
└──────────────────┘
```

### 6.2 Repository Open Flow

```
Library Consumer (gzh-cli)
         │
         ▼
┌──────────────────────┐
│ pkg/repository       │
│ Client.Open(ctx,path)│
└───────────┬──────────┘
            │
            ├──▶ Validate path exists
            │    (internal/validation)
            │
            ├──▶ Check .git directory
            │    (filesystem check)
            │
            ├──▶ Execute: git rev-parse --git-dir
            │    (internal/gitcmd/Executor)
            │
            ├──▶ Parse Git output
            │    (internal/parser)
            │
            ▼
  ┌───────────────────┐
  │ Return Repository │
  │  - Path           │
  │  - GitDir         │
  │  - WorkTree       │
  │  - IsBare         │
  └───────────────────┘
```

### 6.3 Error Flow

```
Error Occurs in Git CLI
         │
         ▼
┌──────────────────────┐
│ internal/gitcmd      │  Capture stderr, exit code
│ Executor.Run()       │
└───────────┬──────────┘
            │
            ▼
┌──────────────────────┐
│ pkg/repository       │  Wrap error with context
│ Client method        │  return GitError{Op, Path, Err}
└───────────┬──────────┘
            │
            ▼
┌──────────────────────┐
│ cmd/cli              │  Extract error details
│ Command handler      │  Format user-friendly message
└───────────┬──────────┘
            │
            ▼
      Display to User
   "Failed to clone repository at /path:
    remote: repository not found

    Suggestions:
    - Check repository URL
    - Verify access permissions"
```

______________________________________________________________________

## 7. Error Handling Strategy

### 7.1 Error Types

```go
// pkg/errors/errors.go
package errors

// GitError wraps Git operation errors
type GitError struct {
    Op       string // Operation: "clone", "commit", etc.
    Path     string // Repository path
    Message  string // Human-readable message
    Err      error  // Underlying error
    ExitCode int    // Git exit code
    Output   string // Git stderr output
}

func (e *GitError) Error() string {
    return fmt.Sprintf("git %s failed at %s: %v", e.Op, e.Path, e.Err)
}

func (e *GitError) Unwrap() error {
    return e.Err
}

// ConflictError indicates merge conflict
type ConflictError struct {
    Files []string
    Msg   string
}

// ValidationError indicates invalid input
type ValidationError struct {
    Field   string
    Value   string
    Reason  string
}
```

### 7.2 Error Handling Pattern

```go
// Example: pkg/repository/client.go
func (c *client) Clone(ctx context.Context, opts CloneOptions) (*Repository, error) {
    // Validate inputs
    if err := validateCloneOptions(opts); err != nil {
        return nil, &ValidationError{
            Field:  "url",
            Value:  opts.URL,
            Reason: err.Error(),
        }
    }

    // Execute Git command
    result, err := c.executor.Run(ctx, opts.Destination, "clone", opts.URL, ".")
    if err != nil {
        return nil, &GitError{
            Op:       "clone",
            Path:     opts.Destination,
            Message:  "failed to clone repository",
            Err:      err,
            ExitCode: result.ExitCode,
            Output:   result.Stderr,
        }
    }

    // Parse and return
    repo, err := c.Open(ctx, opts.Destination)
    if err != nil {
        return nil, fmt.Errorf("failed to open cloned repository: %w", err)
    }

    return repo, nil
}
```

### 7.3 Error Inspection Helpers

```go
// pkg/errors/helpers.go
package errors

// IsNotRepository checks if error indicates "not a git repository"
func IsNotRepository(err error) bool {
    var gitErr *GitError
    if errors.As(err, &gitErr) {
        return gitErr.ExitCode == 128 &&
               strings.Contains(gitErr.Output, "not a git repository")
    }
    return false
}

// IsConflict checks if error indicates merge conflict
func IsConflict(err error) bool {
    var conflictErr *ConflictError
    return errors.As(err, &conflictErr)
}

// IsNetworkError checks for network-related errors
func IsNetworkError(err error) bool {
    var gitErr *GitError
    if errors.As(err, &gitErr) {
        return strings.Contains(gitErr.Output, "Could not resolve host") ||
               strings.Contains(gitErr.Output, "Connection refused")
    }
    return false
}
```

______________________________________________________________________

## 8. Testing Architecture

### 8.1 Test Pyramid

```
         ┌─────────┐
         │   E2E   │  10% - Full workflows, real Git repos
         │  Tests  │
         └─────────┘
       ┌───────────────┐
       │  Integration  │  30% - Real Git, test repos
       │     Tests     │
       └───────────────┘
     ┌───────────────────┐
     │    Unit Tests     │  60% - Mocked dependencies
     │                   │
     └───────────────────┘
```

### 8.2 Unit Testing Strategy

**Package**: `pkg/commit/`

```go
// pkg/commit/manager_test.go
package commit

import (
    "context"
    "testing"
    "github.com/golang/mock/gomock"
    "github.com/stretchr/testify/assert"
)

func TestManager_AutoCommit(t *testing.T) {
    tests := []struct {
        name       string
        setupMock  func(*gomock.Controller) *mockExecutor
        want       *Result
        wantErr    bool
    }{
        {
            name: "successful auto-commit",
            setupMock: func(ctrl *gomock.Controller) *mockExecutor {
                m := NewMockExecutor(ctrl)
                m.EXPECT().
                    Run(gomock.Any(), gomock.Any(), "commit", "-m", gomock.Any()).
                    Return(&gitcmd.Result{ExitCode: 0}, nil)
                return m
            },
            want: &Result{
                Hash: "abc123",
                Message: "feat: add feature",
            },
            wantErr: false,
        },
        // More test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ctrl := gomock.NewController(t)
            defer ctrl.Finish()

            executor := tt.setupMock(ctrl)
            mgr := NewManager(executor, nil)

            got, err := mgr.AutoCommit(context.Background(), testRepo, testPolicy)

            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.want.Hash, got.Hash)
            }
        })
    }
}
```

### 8.3 Integration Testing Strategy

**Package**: `test/integration/`

```go
// test/integration/commit_test.go
// +build integration

package integration

import (
    "context"
    "os"
    "path/filepath"
    "testing"
    "github.com/gizzahub/gzh-cli-git/pkg/repository"
    "github.com/gizzahub/gzh-cli-git/pkg/commit"
)

func TestCommit_Integration(t *testing.T) {
    // Create temporary Git repository
    repoPath := t.TempDir()
    setupGitRepo(t, repoPath)

    // Initialize clients
    repoClient := repository.NewClient(nil)
    commitMgr := commit.NewManager(nil)

    repo, err := repoClient.Open(context.Background(), repoPath)
    if err != nil {
        t.Fatalf("failed to open repo: %v", err)
    }

    // Test commit creation
    result, err := commitMgr.Create(context.Background(), repo, commit.CommitOptions{
        Message: "test: integration test commit",
    })

    if err != nil {
        t.Fatalf("commit failed: %v", err)
    }

    // Verify commit
    if result.Hash == "" {
        t.Error("expected commit hash, got empty")
    }

    // Cleanup
    cleanupGitRepo(t, repoPath)
}

func setupGitRepo(t *testing.T, path string) {
    t.Helper()
    exec.Command("git", "-C", path, "init").Run()
    exec.Command("git", "-C", path, "config", "user.name", "Test").Run()
    exec.Command("git", "-C", path, "config", "user.email", "test@example.com").Run()
    // Create test file
    os.WriteFile(filepath.Join(path, "README.md"), []byte("# Test"), 0644)
    exec.Command("git", "-C", path, "add", ".").Run()
}
```

### 8.4 E2E Testing Strategy

**Package**: `test/e2e/`

```go
// test/e2e/workflow_test.go
// +build e2e

package e2e

import (
    "os/exec"
    "testing"
)

func TestWorkflow_CommitToPush(t *testing.T) {
    // Setup: Create repo, make changes
    repoPath := setupTestRepo(t)
    defer cleanup(t, repoPath)

    // Execute CLI commands
    steps := []struct {
        cmd  string
        args []string
    }{
        {"gzh-git", []string{"commit", "--auto"}},
        {"gzh-git", []string{"push", "--smart"}},
    }

    for _, step := range steps {
        cmd := exec.Command(step.cmd, step.args...)
        cmd.Dir = repoPath

        output, err := cmd.CombinedOutput()
        if err != nil {
            t.Fatalf("command failed: %s\n%s", step.cmd, output)
        }

        t.Logf("%s output:\n%s", step.cmd, output)
    }

    // Verify: Check Git log
    verifyCommitExists(t, repoPath)
}
```

______________________________________________________________________

## 9. Performance Considerations

### 9.1 Performance Requirements

| Operation | Target (p95) | Strategy |
|-----------|--------------|----------|
| `status` | \<50ms | Cached repository state |
| `commit` | \<100ms | Minimal validation |
| `branch create` | \<100ms | Direct Git execution |
| Bulk update (100 repos) | \<30s | Parallel execution (goroutines) |
| History analysis (10K commits) | \<5s | Streaming, pagination |

### 9.2 Optimization Strategies

**Parallel Execution:**

```go
// pkg/operations/bulk.go
func (b *bulkOperator) UpdateAll(ctx context.Context, repos []*Repository) error {
    semaphore := make(chan struct{}, 10) // Limit to 10 concurrent
    errChan := make(chan error, len(repos))

    var wg sync.WaitGroup
    for _, repo := range repos {
        wg.Add(1)
        go func(r *Repository) {
            defer wg.Done()
            semaphore <- struct{}{}        // Acquire
            defer func() { <-semaphore }() // Release

            if err := b.update(ctx, r); err != nil {
                errChan <- err
            }
        }(repo)
    }

    wg.Wait()
    close(errChan)

    // Collect errors
    var errs []error
    for err := range errChan {
        errs = append(errs, err)
    }

    if len(errs) > 0 {
        return fmt.Errorf("bulk update failed: %d errors", len(errs))
    }
    return nil
}
```

**Caching:**

```go
// pkg/repository/client.go
type client struct {
    executor *gitcmd.Executor
    cache    *ttlcache.Cache // Time-based cache
}

func (c *client) GetStatus(ctx context.Context, repo *Repository) (*Status, error) {
    // Check cache
    if cached, ok := c.cache.Get(repo.Path); ok {
        return cached.(*Status), nil
    }

    // Execute Git command
    status, err := c.getStatusUncached(ctx, repo)
    if err != nil {
        return nil, err
    }

    // Cache for 5 seconds
    c.cache.Set(repo.Path, status, 5*time.Second)
    return status, nil
}
```

**Streaming for Large Results:**

```go
// pkg/history/analyzer.go
func (a *analyzer) GetCommits(ctx context.Context, repo *Repository, opts QueryOptions) (<-chan Commit, error) {
    commitChan := make(chan Commit, 100)

    go func() {
        defer close(commitChan)

        // Stream commits incrementally
        cmd := exec.CommandContext(ctx, "git", "log", "--format=%H|%an|%ae|%at|%s")
        stdout, _ := cmd.StdoutPipe()
        cmd.Start()

        scanner := bufio.NewScanner(stdout)
        for scanner.Scan() {
            commit := parseCommitLine(scanner.Text())
            select {
            case commitChan <- commit:
            case <-ctx.Done():
                return
            }
        }
    }()

    return commitChan, nil
}
```

______________________________________________________________________

## 10. Security Architecture

### 10.1 Security Principles

1. **Input Validation**: Sanitize all user inputs before passing to Git CLI
1. **Path Validation**: Ensure paths stay within repository boundaries
1. **Command Injection Prevention**: No direct string interpolation in commands
1. **Credential Safety**: Never log or expose credentials
1. **Least Privilege**: Run with minimal necessary permissions

### 10.2 Input Sanitization

```go
// internal/gitcmd/sanitize.go
package gitcmd

import (
    "regexp"
    "strings"
)

var (
    // Dangerous patterns that could enable command injection
    dangerousPatterns = []*regexp.Regexp{
        regexp.MustCompile(`[;&|]`),     // Command separators
        regexp.MustCompile(`\$\(`),      // Command substitution
        regexp.MustCompile("`"),         // Backticks
        regexp.MustCompile(`\.\./`),     // Path traversal
    }
)

// SanitizeArgs validates and sanitizes Git command arguments
func SanitizeArgs(args []string) ([]string, error) {
    sanitized := make([]string, 0, len(args))

    for _, arg := range args {
        // Check for dangerous patterns
        for _, pattern := range dangerousPatterns {
            if pattern.MatchString(arg) {
                return nil, fmt.Errorf("potentially dangerous argument: %s", arg)
            }
        }

        // Additional validation for specific arg types
        if strings.HasPrefix(arg, "-") {
            // Flag validation
            if !isValidGitFlag(arg) {
                return nil, fmt.Errorf("invalid Git flag: %s", arg)
            }
        }

        sanitized = append(sanitized, arg)
    }

    return sanitized, nil
}

func isValidGitFlag(flag string) bool {
    // Whitelist of known safe Git flags
    safeFlags := []string{
        "--all", "--amend", "--no-verify", "--dry-run",
        "--force", "--quiet", "--verbose",
        // ... more safe flags
    }

    for _, safe := range safeFlags {
        if flag == safe || strings.HasPrefix(flag, safe+"=") {
            return true
        }
    }

    return false
}
```

### 10.3 Path Validation

```go
// internal/validation/paths.go
package validation

import (
    "path/filepath"
    "strings"
)

// ValidateRepositoryPath ensures path is valid and safe
func ValidateRepositoryPath(path string) error {
    // Convert to absolute path
    absPath, err := filepath.Abs(path)
    if err != nil {
        return fmt.Errorf("invalid path: %w", err)
    }

    // Check for path traversal attempts
    if strings.Contains(absPath, "..") {
        return errors.New("path traversal not allowed")
    }

    // Ensure path is within allowed directories
    // (e.g., not system directories)
    disallowed := []string{
        "/etc/", "/usr/", "/bin/", "/sbin/",
        "C:\\Windows\\", "C:\\Program Files\\",
    }

    for _, dis := range disallowed {
        if strings.HasPrefix(absPath, dis) {
            return fmt.Errorf("access to %s not allowed", dis)
        }
    }

    return nil
}
```

______________________________________________________________________

## 11. Deployment Architecture

### 11.1 Build Pipeline

```
┌───────────────┐
│ Source Code   │
│ (main branch) │
└───────┬───────┘
        │
        ▼
┌───────────────┐
│ GitHub Actions│
│ CI/CD         │
└───────┬───────┘
        │
        ├───▶ golangci-lint
        ├───▶ go test (unit, integration)
        ├───▶ go test -race (race detection)
        ├───▶ gosec (security scan)
        │
        ▼
┌───────────────┐
│ Build Binary  │
│ (multi-platform)
└───────┬───────┘
        │
        ├───▶ linux/amd64
        ├───▶ linux/arm64
        ├───▶ darwin/amd64
        ├───▶ darwin/arm64
        ├───▶ windows/amd64
        │
        ▼
┌───────────────┐
│ Create Release│
│ (GitHub)      │
└───────┬───────┘
        │
        ├───▶ Upload binaries
        ├───▶ Generate checksums
        ├───▶ Sign with GPG
        │
        ▼
┌───────────────┐
│ Publish       │
│ - Go module   │
│ - Homebrew    │
│ - APT/YUM     │
└───────────────┘
```

### 11.2 Deployment Targets

**Go Module (Primary):**

```bash
go get github.com/gizzahub/gzh-cli-git@latest
```

**Homebrew (macOS/Linux):**

```bash
brew install gzh-git
```

**Direct Download:**

```bash
curl -sL https://github.com/gizzahub/gzh-cli-git/releases/latest/download/gzh-git-linux-amd64 -o gzh-git
chmod +x gzh-git
```

______________________________________________________________________

## 12. Design Decisions

### 12.1 Decision Log

#### D1: Git CLI vs. go-git Library

**Decision**: Use Git CLI
**Rationale**:

- Maximum compatibility with all Git features
- Simpler implementation (no need to reimplement Git logic)
- Users already have Git installed
- Easier to debug (same commands users run manually)

**Trade-offs**:

- External dependency on Git binary
- Slower than pure Go (process spawning overhead)
- Parsing text output vs. structured API

**Alternatives Considered**:

- go-git/v5: Pure Go, no external deps, but incomplete feature set
- Hybrid: Use go-git for simple ops, Git CLI for complex (too complex)

#### D2: Library-First Architecture

**Decision**: Design library (`pkg/`) with zero CLI dependencies
**Rationale**:

- Enables reuse in gzh-cli and other projects
- Better API design (forced to think about interfaces)
- Easier testing (no CLI framework mocks)
- Clear separation of concerns

**Trade-offs**:

- More upfront design effort
- Indirection layer between CLI and logic
- Some code duplication (CLI and library versions)

**Alternatives Considered**:

- CLI-first, extract library later (risky, usually doesn't happen)
- Monolithic design (violates single responsibility)

#### D3: Functional Options Pattern

**Decision**: Use functional options for all complex operations
**Rationale**:

- API extensibility without breaking changes
- Sensible defaults
- Self-documenting (option names are clear)
- Idiomatic Go pattern

**Trade-offs**:

- More verbose (but clearer)
- Slightly more allocations (usually negligible)

**Example**:

```go
// Instead of:
Clone(ctx, url, path, branch, depth, progress, recursive)

// Use:
Clone(ctx, url, path,
    WithBranch("main"),
    WithDepth(1),
    WithProgress(os.Stdout),
)
```

#### D4: Context Propagation

**Decision**: All operations accept `context.Context` as first parameter
**Rationale**:

- Cancellation support (user can Ctrl+C)
- Timeout support (prevent infinite hangs)
- Request-scoped values (trace IDs, etc.)
- Idiomatic Go concurrency pattern

**Trade-offs**:

- Every function signature includes ctx
- Must remember to pass context through

#### D5: Interface-Driven Design

**Decision**: Define interfaces for all major components
**Rationale**:

- Testability (easy to mock)
- Extensibility (consumers can provide implementations)
- Decoupling (depend on interfaces, not concretions)

**Trade-offs**:

- More files (interface + implementation)
- Indirection (but worth it for benefits)

______________________________________________________________________

## 13. Future Considerations

### 13.1 Potential Enhancements (v2.0+)

**Plugin Architecture:**

- Allow custom commit templates from plugins
- Custom conflict resolution strategies
- Extensible history analyzers

**Performance:**

- libgit2 integration for performance-critical paths
- Persistent cache (disk-based)
- Incremental updates

**Features:**

- Git hooks automation
- Submodule management
- Advanced visualizations (TUI)
- Team collaboration features (code review integration)

### 13.2 Scalability

**Large Repositories (100K+ commits):**

- Streaming APIs (don't load all commits into memory)
- Pagination for queries
- Parallel processing for bulk operations

**High Concurrency:**

- Connection pooling for Git operations
- Rate limiting for external APIs (GitHub, GitLab)
- Circuit breakers for error handling

______________________________________________________________________

## Appendix

### A.1 Key Files Summary

| File | Purpose | Criticality |
|------|---------|-------------|
| `pkg/repository/interfaces.go` | Core repository API | CRITICAL |
| `pkg/commit/manager.go` | Commit automation | HIGH |
| `internal/gitcmd/executor.go` | Git command wrapper | CRITICAL |
| `cmd/gzh-git/main.go` | CLI entry point | MEDIUM |

### A.2 Dependencies

```go
// go.mod
module github.com/gizzahub/gzh-cli-git

go 1.24

require (
    github.com/spf13/cobra v1.9.1       // CLI framework
    github.com/spf13/viper v1.20.1      // Configuration
    golang.org/x/sync v0.17.0           // Concurrency utilities
    gopkg.in/yaml.v3 v3.0.1             // YAML parsing
)

require ( // Test dependencies
    github.com/golang/mock v1.6.0
    github.com/stretchr/testify v1.11.0
)
```

### A.3 References

- [Effective Go](https://golang.org/doc/effective_go.html)
- [Go Project Layout](https://github.com/golang-standards/project-layout)
- [Conventional Commits](https://www.conventionalcommits.org/)
- [gzh-cli Architecture](https://github.com/gizzahub/gzh-cli/blob/main/ARCHITECTURE.md)

______________________________________________________________________

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-11-27 | Claude (AI) | Initial architecture design |

______________________________________________________________________

**End of Document**
