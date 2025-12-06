# CLAUDE.md

This file provides LLM-optimized guidance for Claude Code when working with this repository.

______________________________________________________________________

## Quick Start (30s scan)

**Binary**: `gz-git`
**Module**: `github.com/gizzahub/gzh-cli-git`
**Go Version**: 1.23+
**Architecture**: Safe Git operations CLI

Interface-driven design with strict input sanitization for security.

---

## Top 10 Commands

| Command | Purpose | When to Use |
|---------|---------|-------------|
| `make quality` | fmt + lint + test | Pre-commit (CRITICAL) |
| `make dev-fast` | format + unit tests | Quick dev cycle |
| `make build` | Build binary | After changes |
| `make test` | All tests | Validation |
| `make test-coverage` | Coverage report | Check coverage |
| `make fmt` | Format code | Fix formatting |
| `make lint` | Run linters | Fix lint issues |
| `make pr-check` | Pre-PR verification | Before PR |
| `make install` | Install binary | Local testing |
| `make clean` | Clean artifacts | Fresh start |

---

## Absolute Rules (DO/DON'T)

### DO
- ✅ Use `gzh-cli-core` for common utilities
- ✅ Read `cmd/AGENTS_COMMON.md` before ANY modification
- ✅ Run `make quality` before every commit
- ✅ **ALWAYS sanitize git inputs** (prevent command injection)
- ✅ Test coverage: 80%+ for core logic
- ✅ Use git-specific test helpers from `internal/testutil`

### DON'T
- ❌ Use shell execution (`sh -c`) - command injection risk
- ❌ Concatenate user input into commands
- ❌ Skip input validation
- ❌ Log credentials or sensitive data
- ❌ Commit without security tests

---

## Directory Structure

```
.
├── cmd/gzh-git/            # CLI commands
│   ├── AGENTS.md           # Module guide (READ THIS!)
│   └── *.go                # Subcommands
├── internal/               # Private packages
│   ├── gitcmd/             # Git command executor
│   ├── parser/             # Output parsing
│   └── testutil/           # Git test helpers
├── pkg/                    # Public packages
│   ├── branch/             # Branch management
│   ├── commit/             # Commit operations
│   ├── operations/         # Complex operations
│   └── repository/         # Repository abstraction
└── docs/.claude-context/   # Context docs
```

---

## Context Documentation

| Guide | Purpose |
|-------|---------|
| [Common Tasks](docs/.claude-context/common-tasks.md) | Adding commands, testing git ops |
| [Security Guide](docs/.claude-context/security-guide.md) | Input sanitization, safe execution |

**CRITICAL**: Read before modifying:
- `cmd/AGENTS_COMMON.md` - Project-wide conventions
- `cmd/gzh-git/AGENTS.md` - CLI-specific rules
- [Security Guide](docs/.claude-context/security-guide.md) - Security requirements

---

## Common Mistakes (Top 3)

1. **Not sanitizing git inputs**
   - ⚠️ Command injection vulnerability
   - ✅ Always validate inputs, use `internal/gitcmd`

2. **Using shell execution**
   - ⚠️ Security risk (`sh -c`)
   - ✅ Use `exec.Command("git", args...)` with separate args

3. **Logging credentials**
   - ⚠️ URLs may contain credentials
   - ✅ Strip credentials before logging

______________________________________________________________________

## Shared Library (gzh-cli-core)

**IMPORTANT**: Use for common utilities. DO NOT duplicate.

```go
import (
    "github.com/gizzahub/gzh-cli-core/logger"
    "github.com/gizzahub/gzh-cli-core/errors"
    "github.com/gizzahub/gzh-cli-core/testutil"
)
```

**Git-specific test helpers**:
```go
import "github.com/gizzahub/gzh-cli-git/internal/testutil"

repo := testutil.TempGitRepo(t)
repoWithCommit := testutil.TempGitRepoWithCommit(t)
```

______________________________________________________________________

## Security (CRITICAL)

### Safe Command Execution

```go
// ✅ SAFE - Arguments passed separately
cmd := exec.Command("git", "clone", url)

// ❌ DANGEROUS - Shell execution
cmd := exec.Command("sh", "-c", "git clone " + url)
```

### Input Validation

```go
// Always validate before executing
if !isValidBranchName(branch) {
    return errors.New("invalid branch name")
}
```

See [Security Guide](docs/.claude-context/security-guide.md) for details.

______________________________________________________________________

## Git Commit Format

```
{type}({scope}): {description}

{body}

Model: claude-{model}
Co-Authored-By: Claude <noreply@anthropic.com>
```

**Types**: feat, fix, docs, refactor, test, chore
**Scope**: REQUIRED (cmd, internal, pkg/branch, pkg/commit)

______________________________________________________________________

## FAQ

**Q: Where to add new git commands?**
A: `cmd/gzh-git/` - create new command file

**Q: Where to add git execution logic?**
A: `internal/gitcmd/` - safe command execution

**Q: Where to add output parsing?**
A: `internal/parser/` - git output parsing

---

**Last Updated**: 2025-12-06
**Previous**: 234 lines → **Current**: 153 lines (35% reduction)
