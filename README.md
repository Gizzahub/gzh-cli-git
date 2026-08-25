# gzh-cli-gitforge

> Bulk-first Git operations CLI (`gz-git`) + Go library

[![Go Version](https://img.shields.io/badge/go-1.26%2B-blue)](https://go.dev)
[![Release candidate](https://img.shields.io/badge/release-v0.8.0_candidate-orange)](docs/changelog/0.8.md)
[![CI](https://github.com/gizzahub/gzh-cli-gitforge/actions/workflows/ci.yml/badge.svg)](https://github.com/gizzahub/gzh-cli-gitforge/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![GoDoc](https://pkg.go.dev/badge/github.com/gizzahub/gzh-cli-gitforge.svg)](https://pkg.go.dev/github.com/gizzahub/gzh-cli-gitforge)

`gzh-cli-gitforge` provides:

- **`gz-git` (CLI)**: scan directories and run safe Git operations across many repositories in parallel.
- **Go library**: reusable packages under `pkg/` for repository operations, bulk workflows, and forge sync.

### Two engines (role separation)

| Engine                    | Commands                                                          | Role                                                    |
| ------------------------- | ----------------------------------------------------------------- | ------------------------------------------------------- |
| **Ad-hoc scan**           | `status`, `fetch`, `pull`, `push`, `update`, `clone`, `commit`, … | Instant scan of a directory tree; no config required    |
| **Declarative workspace** | `workspace sync` / `sync`, `forge from`                           | Converge repos from `.gz-git.yaml` or forge org listing |

Scan flags like `--recursive` mean “include nested git repos”. Workspace recursion uses
`--recurse-workspaces` (deprecated alias: `--recursive` on workspace/sync only).

______________________________________________________________________

## CLI Highlights (`gz-git`)

- Bulk operations with `--scan-depth` and `--parallel`: `status`, `fetch`, `pull`, `push`, `update`, `diff`, `commit`, `switch`
- Bulk clone: `clone --url ...` / `clone --file ...` (+ `--update`)
- Git forge operations:
  - `forge from` (GitHub/GitLab/Gitea org/group/user)
  - `forge config generate` → then `workspace sync` (YAML config workflow)
- Maintenance: `cleanup branch` (dry-run by default)
- Monitoring: `watch` (default/compact/json/llm)
- Insights: `history` (stats/contributors/file/blame), `info`, `conflict detect`
- Diagnostics: `doctor` (system, config, auth, forge health checks)
- Tag/stash/worktree helpers: `tag`, `stash`, `worktree`

______________________________________________________________________

## Quick Start

### Install

```bash
go install github.com/gizzahub/gzh-cli-gitforge/cmd/gz-git@latest
gz-git --version
```

### Common Workflows

```bash
# Bulk status (current directory + 1 level)
gz-git status

# Fetch everything under ~/projects (2 levels deep)
gz-git fetch -d 2 ~/projects

# Update repos (pull --rebase), continuously
gz-git update --watch --interval 5m -d 2 ~/projects

# Bulk commit (preview → apply)
gz-git commit -d 2 ~/projects
gz-git commit --yes -d 2 ~/projects

# Switch branch across repos (create if missing)
gz-git switch feature/foo --create -d 2 ~/projects

# Bulk clone into ~/projects
gz-git clone ~/projects --url https://github.com/user/repo1.git --url https://github.com/user/repo2.git

# Sync all repos from a GitHub org
gz-git forge from --provider github --org myorg --path ./repos --token $GITHUB_TOKEN

# Filter repos by language, stars, activity (NEW)
gz-git forge from --provider github --org kubernetes --path ./k8s \
  --language go --min-stars 100 --last-push-within 30d

# Filter repos by name or full path regex
gz-git forge from --provider github --org myorg --path ./repos \
  --include "api|web" --exclude "archive"

# Workspace sync with detailed preview (NEW: shows file changes, conflicts)
gz-git workspace sync  # Preview then auto-proceed
gz-git workspace sync --dry-run  # Preview only
gz-git workspace sync --interactive  # Ask before executing
gz-git workspace sync --format json  # Machine-readable output
```

______________________________________________________________________

## Requirements

- Git 2.30+
- Go 1.26+ (building from source / using as a library)

______________________________________________________________________

## Documentation

- [Docs index](docs/README.md)
- [5-minute quick start (Korean)](QUICK_START.md)
- [Command reference (curated)](docs/commands/README.md)
- [Watch command guide](docs/commands/watch.md)
- [Go library usage](docs/user/getting-started/library-usage.md)
- API reference: https://pkg.go.dev/github.com/gizzahub/gzh-cli-gitforge

______________________________________________________________________

## License

MIT. See `LICENSE`.
