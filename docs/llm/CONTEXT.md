# gzh-cli-gitforge - LLM Context (Current)

This document is intentionally short. Canonical agent guidance lives in `CLAUDE.md` and `docs/.claude-context/`.

## Quick Facts

- **Binary**: `gz-git`
- **Module**: `github.com/gizzahub/gzh-cli-gitforge`
- **Go**: `1.26` (see `go.mod`)
- **Core rule**: no shell execution; sanitize Git args (command injection prevention)

## Read Order (Recommended)

1. `CLAUDE.md`
1. `cmd/AGENTS_COMMON.md`
1. `cmd/gz-git/AGENTS.md`
1. `docs/.claude-context/security-guide.md`
1. `docs/.claude-context/common-tasks.md`

## Repo Map

- `cmd/gz-git/cmd/`: Cobra commands and CLI output formatting
- `pkg/`: public library packages (reusable, no Cobra deps)
- `internal/`: internal helpers (`internal/gitcmd`, `internal/porcelain`, `internal/testutil`)
- `tests/`: integration + e2e tests for CLI workflows

## CLI Docs

- Command workflows and examples: `docs/commands/README.md`
- `watch` deep dive: `docs/commands/watch.md`

## Archived Context

- Historical v0.3.0 snapshot: removed from the tree in `4e8325b`. Read it with
  `git show 4e8325b^:docs/_deprecated/2026-01/LLM_CONTEXT_v0.3.0_2025-12-01.md`.
