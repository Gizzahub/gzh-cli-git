# Phase 7.1: Library Publication Specification

**Phase**: 7.1
**Priority**: P0 (High)
**Status**: Pending
**Created**: 2025-11-30
**Dependencies**: Phase 6 (Complete)

______________________________________________________________________

## Overview

Phase 7.1 focuses on preparing and publishing gzh-cli-git as a stable Go library for external consumption. This phase ensures the library meets production-quality standards, has clear versioning and release processes, and is properly documented for third-party developers.

### Goals

1. **Version Management** - Establish semantic versioning and release process
1. **Library Stability** - Finalize public APIs with backward compatibility guarantees
1. **Publication** - Publish to pkg.go.dev and GitHub with proper tagging
1. **Documentation Polish** - Complete API documentation and examples
1. **Release Artifacts** - Create changelog, migration guides, and release notes
1. **Quality Assurance** - Final validation before v0.1.0 release

### Non-Goals

- gzh-cli integration (Phase 7.2)
- v1.0.0 production release (requires gzh-cli integration first)
- Performance optimizations beyond current benchmarks
- New feature development

______________________________________________________________________

## Architecture

### Versioning Strategy

```
Version Format: vMAJOR.MINOR.PATCH[-PRERELEASE]

v0.1.0-alpha  → Initial library release (current target)
v0.1.x        → Bug fixes, documentation updates
v0.2.0        → New features (backward compatible)
v1.0.0        → Production-ready after gzh-cli integration

Semantic Versioning Rules:
- MAJOR: Breaking API changes
- MINOR: New features (backward compatible)
- PATCH: Bug fixes (backward compatible)
- PRERELEASE: alpha, beta, rc.1, rc.2, etc.
```

### Release Process

```
Release Workflow:
1. Version Bump → 2. Changelog Update → 3. Tag Creation →
4. GitHub Release → 5. pkg.go.dev Verification → 6. Announcement

Automated Checks:
- All tests passing (unit + integration + E2E)
- Test coverage ≥ 69% (current baseline)
- All linters passing (golangci-lint)
- Build successful on all platforms
- No critical security vulnerabilities
```

______________________________________________________________________

## Component 1: API Stability Review

### Purpose

Review and finalize all public APIs to ensure they meet production-quality standards and won't require breaking changes in the near future.

### 1.1 Public API Audit

**Packages to Review:**

```
pkg/repository/
├── interfaces.go      # Client, Logger, ProgressReporter
├── types.go          # Repository, Info, Status, CloneOptions
└── client.go         # NewClient, ClientOption

pkg/commit/
├── template.go       # TemplateManager, Template
├── validator.go      # Validator, ValidationResult
├── generator.go      # Generator, GenerateOptions
└── push.go           # PushManager, PushOptions

pkg/branch/
├── manager.go        # BranchManager, CreateOptions, DeleteOptions
├── worktree.go       # WorktreeManager, WorktreeInfo
├── cleanup.go        # CleanupService, CleanupOptions
└── parallel.go       # ParallelWorkflow, Context

pkg/history/
├── analyzer.go       # Analyzer, CommitStats
├── contributor.go    # ContributorAnalyzer, Contributor
├── file_history.go   # FileHistoryTracker, FileHistory
└── formatter.go      # Formatter interface

pkg/merge/
├── detector.go       # ConflictDetector, ConflictReport
├── strategy.go       # MergeManager, MergeOptions
└── rebase.go         # RebaseManager, RebaseOptions
```

**Review Checklist:**

- [ ] All exported types documented
- [ ] All exported functions documented
- [ ] Parameter names clear and consistent
- [ ] Return types appropriate (no unnecessary complexity)
- [ ] Error types exported and documented
- [ ] Interface contracts well-defined
- [ ] Breaking change risk assessed

### 1.2 API Compatibility Guarantees

**v0.1.x Promises:**

```go
// Guaranteed Stable APIs (v0.1.x):
- repository.Client interface
- repository.Repository type
- All pkg/*/Manager interfaces
- All option structs (CloneOptions, etc.)
- Error types and sentinel errors

// May Change in v0.2.x (with deprecation):
- Internal implementations
- Unexported helper functions
- Performance characteristics
- Error messages (not error types)

// Explicitly Unstable (may change any time):
- internal/* packages
- cmd/* implementation details
```

### 1.3 Deprecation Strategy

**Deprecation Process:**

```go
// Step 1: Mark as deprecated with migration path
// Deprecated: Use NewClientWithOptions instead. This will be removed in v0.3.0.
func NewClient(logger Logger) Client {
    return NewClientWithOptions(WithClientLogger(logger))
}

// Step 2: Add to CHANGELOG.md under "Deprecated"
// Step 3: Keep for at least 2 minor versions
// Step 4: Remove in next major version
```

______________________________________________________________________

## Component 2: Version Management

### Purpose

Establish clear versioning practices and tooling for managing library releases.

### 2.1 Version File

**Create `version.go`:**

```go
package gzhcligit

// Version information
const (
    // Version is the current library version
    Version = "0.1.0-alpha"

    // GitCommit is the git commit SHA (set by build)
    GitCommit = "unknown"

    // BuildDate is the build date (set by build)
    BuildDate = "unknown"
)

// VersionInfo returns detailed version information
func VersionInfo() map[string]string {
    return map[string]string{
        "version":    Version,
        "gitCommit":  GitCommit,
        "buildDate":  BuildDate,
        "goVersion":  runtime.Version(),
    }
}
```

### 2.2 Git Tagging Strategy

**Tag Creation:**

```bash
# Version tags (required)
git tag -a v0.1.0-alpha -m "Release v0.1.0-alpha: Initial library publication"
git tag -a v0.1.0 -m "Release v0.1.0: Production-ready library"

# Tag format: vX.Y.Z[-PRERELEASE]
# Always use annotated tags (-a)
# Include release notes in tag message
```

**Tag Protection:**

```
Protected Tag Patterns:
- v*.*.* (all version tags)
- Must be signed (GPG)
- Require PR review before creation
- Cannot be deleted or force-pushed
```

### 2.3 Go Module Versioning

**Ensure `go.mod` is clean:**

```go
module github.com/gizzahub/gzh-cli-git

go 1.24

require (
    github.com/spf13/cobra v1.8.0
    gopkg.in/yaml.v3 v3.0.1
)
```

**Version Compatibility:**

```
v0.1.x → Compatible with Go 1.24+
v0.2.x → May require Go 1.25+
v1.0.x → Target Go 1.24+ (LTS support)
```

______________________________________________________________________

## Component 3: Release Artifacts

### Purpose

Create comprehensive release artifacts for each version, including changelogs, migration guides, and release notes.

### 3.1 CHANGELOG.md

**Format:**

```markdown
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- New features that are in development

### Changed
- Changes to existing functionality

### Deprecated
- Features that will be removed in future versions

### Removed
- Features that have been removed

### Fixed
- Bug fixes

### Security
- Security fixes and improvements

## [0.1.0-alpha] - 2025-11-30

### Added
- Initial library publication
- Repository operations (open, clone, status, info)
- Commit automation (templates, validation, auto-generation)
- Branch management (create, delete, list, worktrees)
- History analysis (stats, contributors, file tracking)
- Merge/rebase operations (conflict detection, strategies)
- CLI tool with 7 command groups
- 51 integration tests (100% passing)
- 90 E2E test scenarios (100% passing)
- 11 performance benchmarks (all targets met)
- Comprehensive documentation (user + contributor guides)

### Performance
- 69.1% test coverage overall
- Sub-5ms validation operations
- < 1MB memory usage per operation
- 95% operations < 100ms

### Documentation
- Complete API documentation (pkg.go.dev)
- User guide (QUICKSTART, INSTALL, TROUBLESHOOTING)
- Library integration guide
- Contributor guidelines
- 80+ code examples

[Unreleased]: https://github.com/gizzahub/gzh-cli-git/compare/v0.1.0-alpha...HEAD
[0.1.0-alpha]: https://github.com/gizzahub/gzh-cli-git/releases/tag/v0.1.0-alpha
```

### 3.2 Release Notes Template

**GitHub Release Notes:**

````markdown
# gzh-cli-git v0.1.0-alpha

> Initial library publication - Advanced Git automation CLI and Go library

## 🎉 Highlights

- **Complete Library API**: Repository, commit, branch, history, and merge operations
- **Full-Featured CLI**: 7 command groups with 20+ subcommands
- **Production-Ready Testing**: 51 integration + 90 E2E tests (100% passing)
- **Excellent Performance**: Sub-5ms validations, < 1MB memory usage
- **Comprehensive Docs**: Complete API docs + user guides + 80+ examples

## 📦 Installation

### As a Library

```bash
go get github.com/gizzahub/gzh-cli-git@v0.1.0-alpha
````

### As a CLI Tool

```bash
go install github.com/gizzahub/gzh-cli-git/cmd/gzh-git@v0.1.0-alpha
```

## ✨ Features

### Repository Operations

- Open, clone, and query repositories
- Check status and get repository info
- Multiple output formats (table, JSON, CSV, markdown)

### Commit Automation

- Template-based commit messages (Conventional Commits, Semantic Versioning)
- Auto-generate commits from staged changes
- Validate commit messages against templates
- Smart push with safety checks

### Branch Management

- Create, delete, and list branches
- Worktree support for parallel development
- Branch cleanup (merged, stale, orphaned)
- Parallel workflow coordination

### History Analysis

- Commit statistics and trends
- Contributor analysis
- File history tracking with blame support
- Multiple output formatters

### Merge/Rebase

- Pre-merge conflict detection
- Multiple merge strategies
- Rebase operations with continue/skip/abort
- Conflict resolution assistance

## 📊 Quality Metrics

- **Test Coverage**: 69.1% overall (3,333/4,823 statements)
- **Integration Tests**: 51 tests (100% passing in 5.7s)
- **E2E Tests**: 90 test runs (100% passing in 4.5s)
- **Performance**: 95% ops < 100ms, 100% ops < 500ms
- **Memory**: < 1MB per operation

## 📚 Documentation

- [Quick Start Guide](docs/QUICKSTART.md)
- [Installation Guide](docs/INSTALL.md)
- [Library Integration](docs/LIBRARY.md)
- [API Documentation](https://pkg.go.dev/github.com/gizzahub/gzh-cli-git)
- [Contributing Guide](CONTRIBUTING.md)

## 🔄 What's Next

- v0.1.0 stable release after community testing
- Integration with gzh-cli
- v1.0.0 production release

## ⚠️ Pre-release Notice

This is an **alpha release** for early adopters and testing. APIs may change before v1.0.0. See [CONTRIBUTING.md](CONTRIBUTING.md) for how to report issues.

## 🙏 Acknowledgments

Built with [Cobra](https://github.com/spf13/cobra) CLI framework.

______________________________________________________________________

**Full Changelog**: https://github.com/gizzahub/gzh-cli-git/blob/master/CHANGELOG.md

````

### 3.3 Migration Guides

**Create `docs/MIGRATING.md` (for future versions):**

```markdown
# Migration Guide

## Migrating to v0.2.0 from v0.1.x

(To be filled when v0.2.0 is released)

## Migrating to v1.0.0 from v0.x

(To be filled when v1.0.0 is released)
````

______________________________________________________________________

## Component 4: Documentation Polish

### Purpose

Ensure all documentation is complete, accurate, and ready for external developers.

### 4.1 pkg.go.dev Preparation

**Package Documentation Checklist:**

- [ ] All packages have package-level documentation ✅ (Done in Phase 6)
- [ ] All exported types documented
- [ ] All exported functions documented
- [ ] Examples for main APIs
- [ ] README.md is comprehensive
- [ ] LICENSE file present
- [ ] go.mod is clean and correct

**Example Documentation:**

```go
// Example usage in package docs
package repository

// Example of opening a repository and getting status
func ExampleClient_Open() {
    client := NewClient()
    repo, err := client.Open(context.Background(), ".")
    if err != nil {
        log.Fatal(err)
    }

    status, err := client.GetStatus(context.Background(), repo)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Clean: %v\n", status.IsClean)
    // Output: Clean: true
}
```

### 4.2 README Updates

**Ensure README.md includes:**

- [ ] Clear project description
- [ ] Installation instructions
- [ ] Quick start examples
- [ ] Feature highlights
- [ ] Link to full documentation
- [ ] Badge for pkg.go.dev
- [ ] Badge for build status
- [ ] Badge for test coverage
- [ ] Contributing guidelines link
- [ ] License information

**Add badges:**

```markdown
[![Go Reference](https://pkg.go.dev/badge/github.com/gizzahub/gzh-cli-git.svg)](https://pkg.go.dev/github.com/gizzahub/gzh-cli-git)
[![Go Report Card](https://goreportcard.com/badge/github.com/gizzahub/gzh-cli-git)](https://goreportcard.com/report/github.com/gizzahub/gzh-cli-git)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
```

### 4.3 API Examples

**Create `examples/` directory structure:**

```
examples/
├── basic/              # Already exists
├── advanced/
│   ├── custom-templates/
│   │   └── main.go
│   ├── parallel-workflows/
│   │   └── main.go
│   └── conflict-resolution/
│       └── main.go
└── integration/
    ├── github-pr-workflow/
    │   └── main.go
    └── ci-cd-automation/
        └── main.go
```

______________________________________________________________________

## Component 5: Publication Process

### Purpose

Execute the actual publication of the library to GitHub and pkg.go.dev.

### 5.1 Pre-Publication Checklist

**Quality Gates:**

```bash
# 1. All tests passing
go test ./... -v
# Expected: 100% passing

# 2. Linters passing
golangci-lint run
# Expected: No issues

# 3. Build successful
go build ./...
# Expected: Success

# 4. Coverage check
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total
# Expected: ≥ 69.1%

# 5. go.mod tidy
go mod tidy
git diff go.mod go.sum
# Expected: No changes

# 6. License file present
cat LICENSE
# Expected: MIT license text

# 7. Security audit
go list -json -m all | nancy sleuth
# Expected: No critical vulnerabilities
```

### 5.2 GitHub Release Process

**Steps:**

1. **Create and push tag:**

   ```bash
   git tag -a v0.1.0-alpha -m "Release v0.1.0-alpha: Initial library publication"
   git push origin v0.1.0-alpha
   ```

1. **Create GitHub Release:**

   - Go to GitHub Releases page
   - Click "Draft a new release"
   - Select tag: v0.1.0-alpha
   - Release title: "gzh-cli-git v0.1.0-alpha"
   - Copy release notes from template
   - Mark as "pre-release"
   - Publish release

1. **Attach artifacts (optional):**

   ```bash
   # Build binaries for multiple platforms
   make build-all
   # Attach to GitHub release
   ```

### 5.3 pkg.go.dev Publication

**Automatic Publication:**

```
When a new tag is pushed to GitHub:
1. pkg.go.dev automatically detects the new version
2. Fetches the module and generates documentation
3. Makes it available at https://pkg.go.dev/github.com/gizzahub/gzh-cli-git@v0.1.0-alpha

Manual trigger (if needed):
https://pkg.go.dev/github.com/gizzahub/gzh-cli-git@v0.1.0-alpha?tab=overview
(Visit URL to trigger indexing)
```

**Verification:**

```bash
# Check that pkg.go.dev has indexed the version
curl -s https://pkg.go.dev/github.com/gizzahub/gzh-cli-git@v0.1.0-alpha | grep "v0.1.0-alpha"

# Verify go get works
go get github.com/gizzahub/gzh-cli-git@v0.1.0-alpha
```

______________________________________________________________________

## Component 6: Post-Publication

### Purpose

Validate publication success and prepare for community adoption.

### 6.1 Verification Checklist

**After Publication:**

- [ ] pkg.go.dev shows correct documentation
- [ ] GitHub release is visible and downloadable
- [ ] `go get` installs successfully
- [ ] CLI binary can be installed via `go install`
- [ ] All documentation links work
- [ ] License is correctly displayed
- [ ] README renders properly on GitHub
- [ ] Examples run successfully

### 6.2 Announcement

**Channels:**

1. **GitHub Discussions** - Announce in project discussions
1. **Go Forum** - Post in golang-nuts (if appropriate)
1. **Social Media** - Tweet/share (if applicable)
1. **Internal Team** - Notify gzh-cli team

**Announcement Template:**

````markdown
🎉 **gzh-cli-git v0.1.0-alpha is now available!**

We're excited to announce the first alpha release of gzh-cli-git, an advanced Git automation CLI tool and Go library.

✨ **Features:**
- Complete Git operation library (repository, commit, branch, history, merge)
- Full-featured CLI with 20+ commands
- 69.1% test coverage with 141 tests passing
- Comprehensive documentation

📦 **Installation:**
```bash
go get github.com/gizzahub/gzh-cli-git@v0.1.0-alpha
````

📚 **Documentation:**
https://pkg.go.dev/github.com/gizzahub/gzh-cli-git

This is an alpha release for early testing. Feedback welcome!

```

### 6.3 Community Support

**Support Channels:**

```

Issues: GitHub Issues for bugs and feature requests
Discussions: GitHub Discussions for questions
Documentation: Complete guides in docs/ directory
Examples: Working examples in examples/ directory

```

---

## Acceptance Criteria

### Phase 7.1 Complete When:

- [ ] All public APIs reviewed and finalized
- [ ] API compatibility guarantees documented
- [ ] version.go created with v0.1.0-alpha
- [ ] CHANGELOG.md created with full history
- [ ] Release notes prepared
- [ ] All quality gates passing (tests, lints, build)
- [ ] v0.1.0-alpha tag created and pushed
- [ ] GitHub release published
- [ ] pkg.go.dev indexed successfully
- [ ] `go get` verification successful
- [ ] Documentation complete and accurate
- [ ] README badges added
- [ ] Post-publication announcement made

---

## Quality Metrics

### Coverage Requirements

- Maintain ≥ 69.1% overall coverage (current baseline)
- No reduction in test coverage
- All tests passing (141 tests)

### Performance Baseline

- Maintain current performance benchmarks
- All operations < 500ms
- Memory usage < 1MB per operation

### Documentation

- 100% GoDoc coverage (already achieved)
- All examples working
- All links valid

---

## Risks and Mitigation

### Risk 1: API Changes After Publication

**Risk**: May discover API issues after v0.1.0-alpha release
**Impact**: High (breaking changes in alpha are acceptable)
**Mitigation**:
- Mark as alpha release (expect changes)
- Document deprecation process
- Keep alpha period short (2-4 weeks)
- Gather feedback before v0.1.0 stable

### Risk 2: pkg.go.dev Indexing Delay

**Risk**: pkg.go.dev may take time to index new version
**Impact**: Low (users can still `go get` directly)
**Mitigation**:
- Wait 15-30 minutes after tag push
- Manually trigger by visiting pkg.go.dev URL
- Document direct `go get` as alternative

### Risk 3: Dependency Conflicts

**Risk**: Library dependencies may conflict with user projects
**Impact**: Medium
**Mitigation**:
- Minimize dependencies (only 2 external deps)
- Use stable, well-maintained dependencies
- Document dependency versions clearly
- Test with various Go versions

---

## Timeline

### Estimated Duration: 1-2 days

**Day 1** (4-6 hours):
- API stability review (2 hours)
- Version management setup (1 hour)
- CHANGELOG.md creation (1 hour)
- Documentation polish (2 hours)

**Day 2** (2-4 hours):
- Final quality checks (1 hour)
- Tag and release creation (1 hour)
- pkg.go.dev verification (30 min)
- Announcement (30 min)

---

## Next Phase

After Phase 7.1 completion, proceed to **Phase 7.2: gzh-cli Integration** (see `specs/70-gzh-cli-integration.md`).

---

## References

- [Semantic Versioning](https://semver.org/)
- [Keep a Changelog](https://keepachangelog.com/)
- [Go Module Reference](https://go.dev/ref/mod)
- [pkg.go.dev Best Practices](https://go.dev/blog/godoc)
- Phase 6 Completion: `docs/phase-6-completion.md`
- Current Status: `PROJECT_STATUS.md`
```
