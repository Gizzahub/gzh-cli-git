# Technical Requirements Document

**Project**: gzh-cli-git
**Version**: 1.0
**Last Updated**: 2025-11-27
**Status**: Draft

______________________________________________________________________

## 1. System Requirements

### 1.1 Runtime Requirements

**Minimum System Specifications:**
| Component | Requirement | Notes |
|-----------|-------------|-------|
| Operating System | Linux, macOS, Windows | Kernel 4.x+, macOS 11+, Windows 10+ |
| Architecture | amd64, arm64 | Native binaries for each platform |
| Memory | 256MB RAM | For typical operations |
| Disk Space | 50MB | Binary + config + templates |
| Git | 2.30+ | System Git CLI required |

**Recommended Specifications:**
| Component | Recommendation | Benefit |
|-----------|---------------|---------|
| Memory | 512MB+ RAM | Better performance for bulk operations |
| CPU | 2+ cores | Parallel operations |
| Disk | SSD | Faster repository access |
| Git | 2.40+ | Latest features and performance |

### 1.2 Development Requirements

**Build Environment:**

```yaml
Go: 1.24.0+
Make: GNU Make 4.0+
Git: 2.30+
golangci-lint: 1.55+
```

**Development Tools:**

```yaml
Required:
  - go: Go compiler and toolchain
  - git: Version control
  - make: Build automation

Optional:
  - gomock: Mock generation
  - gotestfmt: Test output formatting
  - pre-commit: Git hooks framework
```

______________________________________________________________________

## 2. Functional Requirements

### 2.1 Commit Automation (F1)

#### F1.1 Template System

**REQ-F1.1.1**: Template Loading

- MUST support YAML template format
- MUST include 2+ built-in templates (conventional, semantic)
- MUST allow custom template paths
- MUST validate template structure on load

**REQ-F1.1.2**: Template Variables

- MUST support variable substitution: `{type}`, `{scope}`, `{subject}`
- MUST provide variable validation (required vs. optional)
- MUST support default values

**REQ-F1.1.3**: Template Management

- MUST list all available templates
- MUST show template details with examples
- MUST support template search by name

#### F1.2 Auto-Commit

**REQ-F1.2.1**: Change Analysis

- MUST analyze staged files to suggest commit type
- MUST detect scope from directory structure
- MUST generate subject from file changes

**REQ-F1.2.2**: Message Generation

- MUST generate valid conventional commit messages
- MUST include breaking change markers if detected
- MUST limit subject line to 72 characters

**REQ-F1.2.3**: Validation

- MUST validate commit message format
- MUST enforce character limits (subject: 72, body: 100 per line)
- MUST check for required fields

#### F1.3 Smart Push

**REQ-F1.3.1**: Safety Checks

- MUST detect force push attempts to protected branches
- MUST warn if pushing to default branch (main/master)
- MUST check remote state before push

**REQ-F1.3.2**: Remote Validation

- MUST verify remote exists
- MUST check if local is ahead/behind remote
- MUST suggest rebase if diverged

**REQ-F1.3.3**: Dry-Run Mode

- MUST support `--dry-run` flag
- MUST show what would be pushed without executing

### 2.2 Branch Management (F2)

#### F2.1 Branch Operations

**REQ-F2.1.1**: Branch Creation

- MUST create branches from current HEAD or specified commit
- MUST validate branch names against patterns (e.g., `feature/*`, `fix/*`)
- MUST check for existing branch before creating

**REQ-F2.1.2**: Branch Deletion

- MUST support safe deletion (only merged branches)
- MUST support force deletion with confirmation
- MUST preserve deleted branch refs for 30 days

**REQ-F2.1.3**: Branch Validation

- MUST validate naming conventions
- MUST check against protected branch list
- MUST warn on invalid characters

#### F2.2 Worktree Management

**REQ-F2.2.1**: Worktree Creation

- MUST create linked worktrees
- MUST validate worktree path is not in use
- MUST support creating from branch or commit

**REQ-F2.2.2**: Worktree Listing

- MUST list all worktrees with status
- MUST show branch and commit for each worktree
- MUST indicate locked worktrees

**REQ-F2.2.3**: Worktree Cleanup

- MUST remove worktree and clean up files
- MUST detect orphaned worktrees
- MUST support pruning stale worktrees

#### F2.3 Parallel Workflows

**REQ-F2.3.1**: Workflow Configuration

- MUST define workflow with multiple branches
- MUST support dependency ordering
- MUST track workflow state

**REQ-F2.3.2**: Synchronization

- MUST sync changes across related worktrees
- MUST detect conflicts between worktrees
- MUST provide status overview

### 2.3 History Analysis (F3)

#### F3.1 Commit Statistics

**REQ-F3.1.1**: Commit Queries

- MUST support time-based queries (`--since`, `--until`)
- MUST filter by author, committer
- MUST filter by file/directory path

**REQ-F3.1.2**: Statistics Calculation

- MUST calculate commit frequency (daily/weekly/monthly)
- MUST calculate lines added/removed
- MUST identify most active files

**REQ-F3.1.3**: Aggregation

- MUST group by author, date, file
- MUST support custom aggregation periods
- MUST handle large repositories (10K+ commits)

#### F3.2 Contributor Analysis

**REQ-F3.2.1**: Contributor Identification

- MUST identify all contributors (author + committer)
- MUST resolve email aliases
- MUST rank by contributions

**REQ-F3.2.2**: Activity Patterns

- MUST analyze time-of-day patterns
- MUST identify day-of-week patterns
- MUST show contribution trends

**REQ-F3.2.3**: Code Ownership

- MUST identify file owners by commit count
- MUST show ownership percentage
- MUST track ownership changes over time

#### F3.3 Reporting

**REQ-F3.3.1**: Output Formats

- MUST support table format (human-readable)
- MUST support JSON format (machine-readable)
- MUST support CSV format (spreadsheets)

**REQ-F3.3.2**: Report Content

- MUST include summary statistics
- MUST include detailed breakdowns
- MUST include visualizable data structures

### 2.4 Advanced Merge/Rebase (F4)

#### F4.1 Conflict Detection

**REQ-F4.1.1**: Pre-Merge Analysis

- MUST detect potential conflicts before merge
- MUST identify conflicting files
- MUST classify conflict types (content, binary, deletion)

**REQ-F4.1.2**: Conflict Reporting

- MUST report conflict count and locations
- MUST show conflict context (surrounding lines)
- MUST estimate resolution difficulty

#### F4.2 Auto-Resolution

**REQ-F4.2.1**: Strategy Support

- MUST support `ours` strategy
- MUST support `theirs` strategy
- MUST support `union` strategy
- MUST support `patience` algorithm

**REQ-F4.2.2**: Policy Enforcement

- MUST support safe policies (only trivial conflicts)
- MUST support pattern-based policies (e.g., `.gitignore` = ours)
- MUST require explicit confirmation for risky resolutions

**REQ-F4.2.3**: Rollback

- MUST support undo after auto-resolution
- MUST preserve original state
- MUST allow manual resolution fallback

#### F4.3 Interactive Assistance

**REQ-F4.3.1**: Rebase Helper

- MUST guide through interactive rebase steps
- MUST show commit context
- MUST support skip/edit/squash operations

**REQ-F4.3.2**: Merge Strategy Selection

- MUST suggest best merge strategy
- MUST explain trade-offs
- MUST allow override

### 2.5 Library API (F5)

#### F5.1 Public API Design

**REQ-F5.1.1**: Interface Contracts

- MUST define interfaces for all core operations
- MUST use `context.Context` for all operations
- MUST return rich error types with context

**REQ-F5.1.2**: Dependency Injection

- MUST accept `Logger` interface
- MUST accept `ProgressReporter` interface
- MUST accept `Config` struct

**REQ-F5.1.3**: Functional Options

- MUST use functional options pattern for extensibility
- MUST provide sensible defaults
- MUST document all options

#### F5.2 Error Handling

**REQ-F5.2.1**: Error Types

- MUST define custom error types: `GitError`, `ConflictError`, `ValidationError`
- MUST implement `error` interface
- MUST support `errors.Is()` and `errors.As()`

**REQ-F5.2.2**: Error Context

- MUST include operation name in error
- MUST include repository path in error
- MUST include underlying error cause

**REQ-F5.2.3**: Actionable Messages

- MUST provide clear error descriptions
- MUST suggest remediation steps
- MUST include relevant command output

______________________________________________________________________

## 3. Non-Functional Requirements

### 3.1 Performance Requirements

#### 3.1.1 Latency

**REQ-NFR-P1**: Basic Operation Latency

- MUST complete `status` operation \<50ms (p95)
- MUST complete `commit` operation \<100ms (p95)
- MUST complete `branch create` \<100ms (p95)

**REQ-NFR-P2**: Bulk Operation Throughput

- MUST process 100 repositories in \<30 seconds
- MUST support parallel operations (up to 10 concurrent)
- MUST maintain \<50MB memory per repository

**REQ-NFR-P3**: History Analysis Performance

- MUST analyze 10K commits \<5 seconds
- MUST analyze 100K commits \<30 seconds
- MUST support streaming for large result sets

#### 3.1.2 Resource Usage

**REQ-NFR-P4**: Memory Constraints

- MUST use \<50MB for typical CLI operations
- MUST use \<200MB for bulk operations
- MUST NOT leak memory (verified by profiling)

**REQ-NFR-P5**: Binary Size

- MUST produce binary \<15MB (compressed)
- MUST NOT include unnecessary dependencies
- MUST support static linking option

**REQ-NFR-P6**: Disk I/O

- MUST minimize unnecessary Git operations
- MUST cache repository state when appropriate
- MUST respect Git's object cache

### 3.2 Reliability Requirements

#### 3.2.1 Data Safety

**REQ-NFR-R1**: Destructive Operations

- MUST require explicit confirmation for destructive operations
- MUST support `--dry-run` for all destructive operations
- MUST create backups before destructive operations

**REQ-NFR-R2**: Atomic Operations

- MUST ensure operations complete fully or rollback
- MUST NOT leave repository in inconsistent state
- MUST handle interruptions gracefully (Ctrl+C)

**REQ-NFR-R3**: Error Recovery

- MUST recover from temporary network failures (3 retries)
- MUST handle disk full gracefully
- MUST detect corrupted repositories

#### 3.2.2 API Stability

**REQ-NFR-R4**: Backward Compatibility

- MUST maintain backward compatibility within major version
- MUST NOT break existing library consumers
- MUST deprecate APIs before removal (1 minor version notice)

**REQ-NFR-R5**: Semantic Versioning

- MUST follow SemVer 2.0.0 strictly
- MUST document breaking changes in changelog
- MUST provide migration guides for major versions

### 3.3 Usability Requirements

#### 3.3.1 CLI User Experience

**REQ-NFR-U1**: Command Discoverability

- MUST provide `--help` for all commands
- MUST show usage examples in help text
- MUST support tab completion (bash, zsh, fish)

**REQ-NFR-U2**: Error Messages

- MUST provide clear, actionable error messages
- MUST suggest next steps on error
- MUST avoid technical jargon when possible

**REQ-NFR-U3**: Progress Feedback

- MUST show progress for long-running operations (>2s)
- MUST support `-v/--verbose` flag for details
- MUST support `-q/--quiet` flag for scripts

#### 3.3.2 Library User Experience

**REQ-NFR-U4**: Documentation

- MUST provide GoDoc for 100% of public APIs
- MUST include runnable examples for common use cases
- MUST document all configuration options

**REQ-NFR-U5**: Examples

- MUST provide basic usage example
- MUST provide advanced usage examples
- MUST provide integration examples (gzh-cli)

### 3.4 Security Requirements

#### 3.4.1 Input Validation

**REQ-NFR-S1**: Command Injection Prevention

- MUST sanitize all user inputs before passing to Git CLI
- MUST validate file paths stay within repository
- MUST reject suspicious patterns (e.g., `&&`, `;`, `|`)

**REQ-NFR-S2**: Path Traversal Prevention

- MUST validate paths are within repository
- MUST reject `../` patterns outside repo
- MUST check symlink targets

#### 3.4.2 Credential Management

**REQ-NFR-S3**: Credential Handling

- MUST NOT log credentials
- MUST NOT expose credentials in error messages
- MUST delegate credential storage to Git credential manager

**REQ-NFR-S4**: Sensitive Data

- MUST NOT log file contents
- MUST sanitize output before logging
- MUST respect `.gitignore` for privacy

### 3.5 Compatibility Requirements

#### 3.5.1 Platform Support

**REQ-NFR-C1**: Operating Systems

- MUST support Linux (kernel 4.x+)
- MUST support macOS (11.0+)
- MUST support Windows (10+)

**REQ-NFR-C2**: Architectures

- MUST support amd64
- MUST support arm64
- SHOULD support 32-bit platforms

#### 3.5.2 Git Compatibility

**REQ-NFR-C3**: Git Versions

- MUST support Git 2.30+
- SHOULD support Git 2.40+ (recommended)
- MUST test against Git 2.30, 2.35, 2.40, 2.45

**REQ-NFR-C4**: Git Features

- MUST use only stable Git CLI commands
- MUST NOT rely on Git porcelain changes
- MUST fall back gracefully if feature unavailable

#### 3.5.3 Go Compatibility

**REQ-NFR-C5**: Go Versions

- MUST support Go 1.22+ (for building)
- SHOULD support Go 1.24+ (recommended)
- MUST maintain compatibility with gzh-cli's Go version

**REQ-NFR-C6**: Go Modules

- MUST use Go modules (go.mod)
- MUST NOT require `GOPATH`
- MUST support vendoring

### 3.6 Maintainability Requirements

#### 3.6.1 Code Quality

**REQ-NFR-M1**: Test Coverage

- MUST achieve ≥85% coverage in `pkg/`
- MUST achieve ≥80% coverage in `internal/`
- MUST achieve ≥70% coverage in `cmd/`

**REQ-NFR-M2**: Linting

- MUST pass golangci-lint with project config
- MUST pass `go vet`
- MUST pass `gofumpt` formatting

**REQ-NFR-M3**: Code Complexity

- SHOULD limit cyclomatic complexity \<15 per function
- SHOULD limit file size \<500 lines
- SHOULD limit function size \<100 lines

#### 3.6.2 Documentation

**REQ-NFR-M4**: Code Documentation

- MUST document all exported functions (GoDoc)
- MUST document all exported types
- MUST document all configuration options

**REQ-NFR-M5**: User Documentation

- MUST provide user guide for each feature
- MUST provide API reference
- MUST provide migration guides

**REQ-NFR-M6**: Developer Documentation

- MUST provide CONTRIBUTING.md
- MUST document build process
- MUST document testing strategy

______________________________________________________________________

## 4. Technical Constraints

### 4.1 Technology Stack

**REQ-TC-1**: Programming Language

- MUST use Go 1.24.0+
- MUST use Go standard library where possible
- MUST minimize external dependencies

**REQ-TC-2**: CLI Framework

- MUST use Cobra v1.9+ for CLI
- MUST use Viper v1.20+ for configuration
- SHOULD use standard library for core logic

**REQ-TC-3**: Testing Framework

- MUST use standard `testing` package
- MAY use testify for assertions
- MUST use gomock for interface mocking

### 4.2 Architecture Constraints

**REQ-TC-4**: Library-First Design

- MUST have ZERO CLI dependencies in `pkg/`
- MUST NOT import Cobra in `pkg/`
- MUST NOT use `fmt.Println` in `pkg/`

**REQ-TC-5**: Interface-Driven

- MUST define interfaces for all core operations
- MUST use dependency injection
- MUST support mock implementations for testing

**REQ-TC-6**: Context Propagation

- MUST accept `context.Context` as first parameter
- MUST respect context cancellation
- MUST propagate context to sub-operations

### 4.3 Integration Constraints

**REQ-TC-7**: gzh-cli Integration

- MUST be importable as Go module
- MUST NOT conflict with gzh-cli dependencies
- MUST maintain compatible Go version

**REQ-TC-8**: Git CLI Integration

- MUST use native Git CLI (not go-git)
- MUST detect Git binary at runtime
- MUST validate Git version compatibility

______________________________________________________________________

## 5. Testing Requirements

### 5.1 Unit Testing

**REQ-TEST-U1**: Coverage Requirements

- MUST achieve ≥85% line coverage in `pkg/`
- MUST achieve ≥80% line coverage in `internal/`
- MUST achieve ≥70% line coverage in `cmd/`

**REQ-TEST-U2**: Test Organization

- MUST colocate tests with code (`file.go` → `file_test.go`)
- MUST use table-driven tests for multiple scenarios
- MUST use descriptive test names

**REQ-TEST-U3**: Mocking

- MUST mock external dependencies (Git CLI)
- MUST provide mock implementations of interfaces
- MUST test error conditions

### 5.2 Integration Testing

**REQ-TEST-I1**: Real Git Repos

- MUST test against real Git repositories
- MUST create temporary test repositories
- MUST clean up test repositories after tests

**REQ-TEST-I2**: Multi-Platform Testing

- MUST test on Linux
- MUST test on macOS
- MUST test on Windows

**REQ-TEST-I3**: Git Version Matrix

- MUST test against Git 2.30
- MUST test against Git 2.40
- SHOULD test against latest Git

### 5.3 End-to-End Testing

**REQ-TEST-E1**: CLI Scenarios

- MUST test complete user workflows
- MUST test error scenarios
- MUST test edge cases

**REQ-TEST-E2**: Library Integration

- MUST test gzh-cli integration
- MUST test external library usage
- MUST verify API stability

### 5.4 Performance Testing

**REQ-TEST-P1**: Benchmarks

- MUST provide benchmarks for core operations
- MUST track performance regressions
- MUST test with various repository sizes

**REQ-TEST-P2**: Load Testing

- MUST test bulk operations (100+ repos)
- MUST test memory usage
- MUST test concurrent operations

### 5.5 Security Testing

**REQ-TEST-S1**: Input Validation

- MUST test command injection scenarios
- MUST test path traversal scenarios
- MUST test malformed input handling

**REQ-TEST-S2**: Credential Safety

- MUST verify credentials not logged
- MUST verify credentials not in errors
- MUST test credential manager integration

______________________________________________________________________

## 6. Deployment Requirements

### 6.1 Build Requirements

**REQ-DEP-B1**: Build System

- MUST use `make` for build automation
- MUST support `make build`, `make test`, `make install`
- MUST generate static binaries option

**REQ-DEP-B2**: Cross-Compilation

- MUST support cross-compilation to all platforms
- MUST provide pre-built binaries (Linux, macOS, Windows)
- MUST support amd64 and arm64

**REQ-DEP-B3**: Reproducible Builds

- MUST support reproducible builds
- MUST pin dependency versions
- MUST document build environment

### 6.2 Distribution Requirements

**REQ-DEP-D1**: Go Module

- MUST publish to Go module proxy
- MUST tag releases with semantic versions
- MUST provide stable import path

**REQ-DEP-D2**: Binary Releases

- MUST provide GitHub Releases
- MUST include checksums (SHA256)
- MUST sign releases (GPG)

**REQ-DEP-D3**: Package Managers

- SHOULD support Homebrew (macOS/Linux)
- SHOULD support apt (Debian/Ubuntu)
- SHOULD support yum (RHEL/CentOS)

### 6.3 CI/CD Requirements

**REQ-DEP-CI1**: Continuous Integration

- MUST run tests on every PR
- MUST run linters on every PR
- MUST check test coverage

**REQ-DEP-CI2**: Continuous Deployment

- MUST automate release process
- MUST publish on Git tag push
- MUST update package managers

______________________________________________________________________

## 7. Documentation Requirements

### 7.1 Code Documentation

**REQ-DOC-C1**: GoDoc

- MUST document 100% of public APIs
- MUST include examples for complex APIs
- MUST document error conditions

**REQ-DOC-C2**: Inline Comments

- MUST comment complex logic
- SHOULD document why, not what
- MUST explain non-obvious decisions

### 7.2 User Documentation

**REQ-DOC-U1**: README

- MUST include project overview
- MUST include installation instructions
- MUST include quick start guide

**REQ-DOC-U2**: User Guides

- MUST provide guide for each feature
- MUST include CLI examples
- MUST include troubleshooting

**REQ-DOC-U3**: API Reference

- MUST provide complete API reference
- MUST include library usage examples
- MUST document configuration options

### 7.3 Developer Documentation

**REQ-DOC-D1**: Contributing Guide

- MUST provide CONTRIBUTING.md
- MUST document development workflow
- MUST explain testing strategy

**REQ-DOC-D2**: Architecture Documentation

- MUST provide ARCHITECTURE.md
- MUST document design decisions
- MUST include diagrams

______________________________________________________________________

## 8. Compliance & Standards

### 8.1 Coding Standards

**REQ-STD-C1**: Go Style Guide

- MUST follow [Effective Go](https://golang.org/doc/effective_go.html)
- MUST follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- MUST pass `gofumpt` formatting

**REQ-STD-C2**: Naming Conventions

- MUST use camelCase for Go identifiers
- MUST use descriptive names (avoid abbreviations)
- MUST follow Go interface naming (`-er` suffix)

### 8.2 API Standards

**REQ-STD-A1**: Semantic Versioning

- MUST follow [SemVer 2.0.0](https://semver.org/)
- MUST document breaking changes
- MUST provide migration guides

**REQ-STD-A2**: Git Conventions

- MUST support [Conventional Commits](https://www.conventionalcommits.org/)
- SHOULD support other commit conventions
- MUST validate commit format

### 8.3 Licensing

**REQ-STD-L1**: Open Source License

- MUST use permissive license (MIT, Apache 2.0)
- MUST include LICENSE file
- MUST attribute dependencies

______________________________________________________________________

## 9. Acceptance Criteria

### 9.1 Feature Completeness

- ✅ All F1-F5 features implemented
- ✅ CLI commands working for all features
- ✅ Library API stable and documented

### 9.2 Quality Gates

- ✅ Test coverage ≥85% (pkg/), ≥80% (internal/), ≥70% (cmd/)
- ✅ All linters passing (golangci-lint, go vet)
- ✅ Performance benchmarks met (\<100ms operations)
- ✅ Security scans passing (gosec, nancy)

### 9.3 Documentation Completeness

- ✅ 100% API coverage (GoDoc)
- ✅ User guides for all features
- ✅ Migration guide for gzh-cli
- ✅ CONTRIBUTING.md, ARCHITECTURE.md complete

### 9.4 Integration Success

- ✅ gzh-cli successfully using library
- ✅ 3+ alpha users validated workflows
- ✅ No critical bugs reported

______________________________________________________________________

## 10. Traceability Matrix

| Requirement ID | Feature | Priority | Verification Method | Status |
|----------------|---------|----------|---------------------|--------|
| REQ-F1.1.1 | Template Loading | P0 | Unit Test | ⏳ Pending |
| REQ-F1.2.1 | Change Analysis | P0 | Integration Test | ⏳ Pending |
| REQ-F1.3.1 | Safety Checks | P0 | E2E Test | ⏳ Pending |
| REQ-F2.1.1 | Branch Creation | P0 | Unit Test | ⏳ Pending |
| REQ-F2.2.1 | Worktree Creation | P0 | Integration Test | ⏳ Pending |
| REQ-F3.1.1 | Commit Queries | P0 | Unit Test | ⏳ Pending |
| REQ-F4.1.1 | Pre-Merge Analysis | P0 | Integration Test | ⏳ Pending |
| REQ-NFR-P1 | Basic Operation Latency | P0 | Benchmark | ⏳ Pending |
| REQ-NFR-R1 | Destructive Operations | P0 | Manual Test | ⏳ Pending |
| REQ-NFR-S1 | Command Injection Prevention | P0 | Security Test | ⏳ Pending |

______________________________________________________________________

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-11-27 | Claude (AI) | Initial technical requirements |

______________________________________________________________________

**End of Document**
