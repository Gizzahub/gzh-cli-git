# CLAUDE.md

This file provides LLM-optimized guidance for Claude Code when working with this repository.

______________________________________________________________________

## Project Context

**Binary**: `gz-git`
**Module**: `github.com/gizzahub/gzh-cli-git`
**Go Version**: 1.23+
**Architecture**: Standard CLI (Cobra-based)

### Core Principles

- **Interface-driven design**: Use Go interfaces for abstraction
- **Direct constructors**: No DI containers, simple factory pattern
- **Safe git operations**: Sanitize inputs, prevent command injection
- **Modular packages**: Separation of git commands, parsing, and validation

______________________________________________________________________

## Shared Library (gzh-cli-core)

**IMPORTANT**: Use `gzh-cli-core` for common utilities. DO NOT create local duplicates.

| Package | Import | Purpose |
|---------|--------|---------|
| logger | `gzh-cli-core/logger` | Structured logging |
| testutil | `gzh-cli-core/testutil` | Test helpers (TempDir, Assert\*, Capture) |
| errors | `gzh-cli-core/errors` | Error types and wrapping |
| config | `gzh-cli-core/config` | Config loading utilities |
| cli | `gzh-cli-core/cli` | CLI flags and output |
| version | `gzh-cli-core/version` | Version info |

```go
import (
    "github.com/gizzahub/gzh-cli-core/logger"
    "github.com/gizzahub/gzh-cli-core/errors"
    "github.com/gizzahub/gzh-cli-core/testutil"
)
```

**Git-specific test helpers**: Use `internal/testutil` for `TempGitRepo()`, `TempGitRepoWithCommit()`, etc.

______________________________________________________________________

## Module-Specific Guides (AGENTS.md)

**Read these before modifying code:**

| Guide | Location | Purpose |
|-------|----------|---------|
| Common Rules | `cmd/AGENTS_COMMON.md` | Project-wide conventions |
| CLI Module | `cmd/gzh-git/AGENTS.md` | CLI-specific rules |

______________________________________________________________________

## Internal Packages

| Package | Purpose | Key Functions |
|---------|---------|---------------|
| `internal/gitcmd` | Git command execution | `Run()`, `RunWithOutput()` |
| `internal/parser` | Git output parsing | Status, log, diff parsing |
| `internal/testutil` | Git-specific test helpers | `TempGitRepo()`, `TempGitRepoWithCommit()` |

## Public Packages (pkg/)

| Package | Purpose |
|---------|---------|
| `pkg/branch` | Branch management |
| `pkg/commit` | Commit operations |
| `pkg/config` | Configuration handling |
| `pkg/history` | Git history analysis |
| `pkg/merge` | Merge strategies |
| `pkg/operations` | Complex git operations |
| `pkg/repository` | Repository abstraction |
| `pkg/watch` | File watching |

______________________________________________________________________

## Development Workflow

### Before Code Modification

1. **Read AGENTS.md** for the module you're modifying
1. Check existing patterns in `internal/` and `pkg/`
1. Review CONTRIBUTING.md for guidelines

### Code Modification Process

```bash
# 1. Write code + tests
# 2. Quality checks (CRITICAL)
make quality    # runs fmt + lint + test

# Quick development cycle
make dev-fast   # format + unit tests only

# Pre-PR verification
make pr-check
```

______________________________________________________________________

## Essential Commands Reference

### Development Workflow

```bash
# One-time setup
make deps
make install-tools

# Before every commit (CRITICAL)
make quality

# Build & install
make build
make install

# Quick development
make dev-fast   # format + unit tests
make dev        # format + lint + test
```

### Testing

```bash
make test           # All tests
make test-unit      # Unit tests only
make test-coverage  # With coverage report
make bench          # Benchmarks
```

### Code Quality

```bash
make fmt            # Format code
make lint           # Run linters
make fmt-diff       # Format changed files only
make lint-diff      # Lint changed files only
```

______________________________________________________________________

## Project Structure

```
.
├── cmd/
│   └── gzh-git/
│       ├── AGENTS.md           # Module-specific guide
│       ├── main.go             # Entry point
│       ├── root.go             # Root command
│       └── *.go                # Subcommands
├── internal/                    # Private packages
│   ├── gitcmd/                 # Git command executor
│   ├── parser/                 # Output parsing
│   └── validation/             # Input validation
├── pkg/                         # Public packages
│   ├── branch/                 # Branch management
│   ├── commit/                 # Commit operations
│   ├── config/                 # Configuration
│   ├── history/                # History analysis
│   ├── merge/                  # Merge strategies
│   ├── operations/             # Complex operations
│   ├── repository/             # Repository abstraction
│   └── watch/                  # File watching
├── docs/                        # Documentation
├── examples/                    # Usage examples
├── tests/                       # Integration tests
├── .make/                       # Modular Makefile
├── .golangci.yml               # Linter config
├── CLAUDE.md                   # This file
├── go.mod                      # Go module
├── Makefile                    # Build automation
└── README.md                   # Project documentation
```

______________________________________________________________________

## Important Rules

### Critical Requirements

- **Read AGENTS.md** before modifying any module
- Always run `make quality` before commit
- Test coverage: 80%+ for core logic
- **Sanitize git inputs** - prevent command injection

### Code Style

- **Binary name**: `gz-git`
- **Interface-driven**: Use interfaces for testability
- **Error handling**: Use structured errors with context
- **Git safety**: Always validate and sanitize user inputs

### Commit Format

```
{type}({scope}): {description}

{body}

Model: claude-{model}
Co-Authored-By: Claude <noreply@anthropic.com>
```

**Types**: feat, fix, docs, refactor, test, chore
**Scope**: REQUIRED (e.g., cmd, internal, pkg/branch, pkg/commit)

______________________________________________________________________

## FAQ

**Q: Where to add new git commands?**
A: `cmd/gzh-git/` - create new command file

**Q: Where to add git execution logic?**
A: `internal/gitcmd/` - safe command execution

**Q: Where to add output parsing?**
A: `internal/parser/` - git output parsing

**Q: Where to add public APIs?**
A: `pkg/{feature}/` directory

**Q: What files should AI not modify?**
A: See `.claudeignore` (create if needed)

______________________________________________________________________

**Last Updated**: 2024-12-05
