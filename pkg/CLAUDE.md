# CLAUDE.md - pkg/

LLM-optimized guidance for the **gzh-cli-gitforge** package library.

______________________________________________________________________

## Overview

This directory contains the **public API packages** for gzh-cli-gitforge. Each package provides specific Git forge functionality.

**Parent Project**: See [../CLAUDE.md](../CLAUDE.md) for project-level guidance.
**Context Docs**: See [../docs/.claude-context/](../docs/.claude-context/) for detailed guides.

______________________________________________________________________

## Package Map (20 packages)

### Core Operations

- **branch/** - Branch management, cleanup, worktree operations
- **repository/** - Repository abstraction, bulk operations, state management
- **scanner/** - Local git repo scanning and discovery

### Git Forge Integration

- **provider/** - Forge provider interface (GitHub/GitLab/Gitea)
- **github/** - GitHub provider (github.com + GitHub Enterprise Server via custom base URL)
- **gitlab/** - GitLab provider implementation
- **gitea/** - Gitea provider implementation

### Sync & Workspace

- **reposync/** - Repository sync planner/executor (forge API → local)
- **reposynccli/** - Sync CLI commands (from, config generate)
- **workspacecli/** - Workspace CLI commands (init, scan, sync, status, add, validate)
- **sync/** - Legacy sync package (being phased out)

### Configuration

- **config/** - Configuration management (profiles, precedence, hierarchical config)
- **identity/** - Device/agent naming for automated commits (git trailers)

### Git Operations

- **history/** - Git history analysis, contributor stats
- **merge/** - Merge conflict detection and resolution
- **stash/** - Stash management
- **tag/** - Tag management, semantic versioning

### Monitoring & UI

- **watch/** - Repository monitoring, change detection
- **tui/** - Terminal UI components, formatters
- **wizard/** - Interactive wizards for complex workflows

### Utilities

- **cliutil/** - CLI utilities, formatters, helpers
- **ratelimit/** - Rate limiting for API calls

______________________________________________________________________

## Adding a New Package

1. **Create package directory**: `pkg/newpkg/`
1. **Add package doc**: `pkg/newpkg/doc.go` with package comment
1. **Add types**: `pkg/newpkg/types.go` for public types
1. **Add implementation**: `pkg/newpkg/*.go`
1. **Add tests**: `pkg/newpkg/*_test.go`
1. **Update this CLAUDE.md**: Add to package map above

**Standards**:

- Use `github.com/gizzahub/gzh-cli-core` for common utilities
- Follow security guide: [../docs/.claude-context/security-guide.md](../docs/.claude-context/security-guide.md)
- Add integration tests to `../tests/integration/` if needed

______________________________________________________________________

## Common Tasks

### Modify Existing Package

1. Read package documentation: `pkg/{package}/doc.go`
1. Review types: `pkg/{package}/types.go`
1. Check tests: `pkg/{package}/*_test.go`
1. Make changes
1. Run: `cd .. && make quality` (from project root)

### Add New Feature to Package

1. Add types to `types.go`
1. Implement in new file or extend existing
1. Add tests
1. Update package doc if API changes
1. Update this CLAUDE.md if new public API

### Testing

```bash
# From project root
make test              # All tests
go test ./pkg/...      # All pkg tests
go test ./pkg/branch/... -v  # Specific package
```

______________________________________________________________________

## Package Dependencies

```
High-level packages (depend on low-level):
  workspacecli, reposynccli, wizard
    ↓
  reposync, scanner, config
    ↓
  provider (github, gitlab, gitea)
    ↓
  repository, branch, history, merge
    ↓
  ../internal/gitcmd (safe git execution)
```

**Rule**: Packages should not have circular dependencies.

______________________________________________________________________

## Context Documents

| Guide                                                                                  | Purpose                            |
| -------------------------------------------------------------------------------------- | ---------------------------------- |
| [../docs/.claude-context/security-guide.md](../docs/.claude-context/security-guide.md) | Input sanitization, safe execution |
| [../docs/.claude-context/common-tasks.md](../docs/.claude-context/common-tasks.md)     | Adding commands, testing           |

______________________________________________________________________

## Quick Reference

| Task          | Command                       |
| ------------- | ----------------------------- |
| Format all    | `cd .. && make fmt`           |
| Lint all      | `cd .. && make lint`          |
| Test all      | `cd .. && make test`          |
| Quality check | `cd .. && make quality`       |
| Test coverage | `cd .. && make test-coverage` |

**Note**: Always run commands from project root (`..`), not from `pkg/`.

______________________________________________________________________

**Last Updated**: 2026-01-22
