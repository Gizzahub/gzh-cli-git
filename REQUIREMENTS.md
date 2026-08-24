# Requirements (Current)

This repository keeps detailed, per-feature requirements in `docs/specs/` and product intent in `docs/00-product/`.
This file is a short index so requirements stay discoverable without duplicating specs.

## Compatibility

- Go: `1.26+` (see `go.mod`)
- Git: `2.30+`

## Safety / Security

- Never execute via shell (`sh -c`); always pass arguments as a slice.
- Sanitize Git arguments before execution (see `internal/gitcmd/` and `docs/.claude-context/security-guide.md`).
- Avoid logging credentials (strip tokens from URLs before logging).

## Canonical Sources

- Feature specifications: `docs/specs/00-overview.md`
- Product docs: `docs/00-product/`
- Architecture overview: `ARCHITECTURE.md` (details: `docs/10-architecture/`)

## Archived

- Historical TRD (v1.0, 2025-11-27): removed from the tree in `4e8325b`. Read it with
  `git show 4e8325b^:docs/_deprecated/2026-01/REQUIREMENTS_v1.0_2025-11-27.md`.
