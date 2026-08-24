# Documentation

This folder contains the canonical documentation for **gzh-cli-gitforge**.
Root-level docs (`README.md`, `QUICK_START.md`, `CONTRIBUTING.md`, `ARCHITECTURE.md`) remain the GitHub entrypoints; everything else is organized here.

## Start Here

- Project overview: [README.md](../README.md)
- 5-minute quick start (Korean): [QUICK_START.md](../QUICK_START.md)
- Command reference (curated): [docs/commands/README.md](commands/README.md)
- FAQ: [docs/user/guides/faq.md](user/guides/faq.md)
- Troubleshooting (symptom-first): [docs/user/guides/troubleshooting.md](user/guides/troubleshooting.md)
- Go library usage: [docs/user/getting-started/library-usage.md](user/getting-started/library-usage.md)

## Documentation Map

### CLI (gz-git)

- Command reference and examples: [docs/commands/README.md](commands/README.md)
- Watch command guide: [docs/commands/watch.md](commands/watch.md)
- Watch output design notes: [docs/design/WATCH_OUTPUT_FORMATS.md](design/WATCH_OUTPUT_FORMATS.md)
- Watch output improvement notes: [docs/design/WATCH_OUTPUT_IMPROVEMENTS.md](design/WATCH_OUTPUT_IMPROVEMENTS.md)

### Users

- User docs index: [docs/user/README.md](user/README.md)
- Getting started: [docs/user/getting-started/](user/getting-started/)
- Guides: [docs/user/guides/](user/guides/)

### Development

- Contributing: [CONTRIBUTING.md](../CONTRIBUTING.md)
- Architecture: [ARCHITECTURE.md](../ARCHITECTURE.md) (overview) · [10-architecture/](10-architecture/README.md) (details)
- Specifications (design/requirements-by-feature): [specs/](../specs/)
- Release notes: [CHANGELOG.md](../CHANGELOG.md)
- Homebrew distribution notes: [docs/homebrew-setup.md](homebrew-setup.md)

### Product

- Product docs: [docs/00-product/](00-product/)

### LLM / Agent Context (Internal)

- Agent instructions (root): [CLAUDE.md](../CLAUDE.md) (symlinked as `AGENTS.md`)
- Expanded context docs: [docs/.claude-context/](.claude-context/)

### Archives

- Deprecated / historical docs (not maintained): [docs/\_deprecated/](./_deprecated/)

## Maintenance Policy

- **CLI flags and subcommands**: Prefer `gz-git --help` / `gz-git <cmd> --help` as the source of truth; keep `docs/commands/README.md` focused on workflows, examples, and stable concepts to reduce drift.
- **When behavior changes**: Update (1) Cobra help text, (2) `docs/commands/README.md`, then (3) root `README.md` / `QUICK_START.md` if they include affected examples.
