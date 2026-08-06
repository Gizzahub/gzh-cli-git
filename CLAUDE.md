# CLAUDE.md — gzh-cli-gitforge

LLM-optimized guidance for gzh-cli-gitforge.

**Binary**: `gz-git` | **Module**: `github.com/gizzahub/gzh-cli-gitforge` | **Go**: 1.26+

## Top Commands

| Command              | Purpose             | When                  |
| -------------------- | ------------------- | --------------------- |
| `make quality`       | fmt + lint + test   | Pre-commit (CRITICAL) |
| `make dev-fast`      | format + unit tests | Quick dev cycle       |
| `make build`         | Build binary        | After changes         |
| `make pr-check`      | Pre-PR verification | Before PR             |
| `make test-coverage` | Coverage report     | Check coverage        |

## Absolute Rules

**DO**: Use `gzh-cli-core` for utilities · Read `cmd/AGENTS_COMMON.md` before modifying · Run `make quality` before every commit · Sanitize all git inputs · 80%+ test coverage for core logic

**DON'T**: Use `sh -c` (command injection) · Concatenate user input into commands · Log credentials · Commit without security tests

## Directory Structure

```
cmd/gz-git/          # CLI commands (AGENTS.md inside)
internal/
  gitcmd/            # Git command executor
  parser/            # Output parsing
  testutil/          # Git test helpers
pkg/
  repository/        # Repository abstraction + bulk ops
  config/            # Configuration management
  provider/          # Forge providers (github/gitlab/gitea)
  reposync/          # Repo sync planner/executor
  reposynccli/       # Sync CLI commands
  workspacecli/      # Workspace CLI commands
  scanner/           # Local git repo scanner
  branch/            # Branch utilities + cleanup
docs/.claude-context/ # Context docs
```

## Main Commands

| Command                | Description                                            |
| ---------------------- | ------------------------------------------------------ |
| **`sync` / `s`**       | **Smart sync: auto-init + sync (most used)**           |
| `clone`                | Parallel clone (--url, --file, -c config)              |
| `status`               | Health check (fetch + divergence + recommend)          |
| `fetch` / `pull`       | Fetch/pull all repos                                   |
| `push`                 | Push all repos (refspec: `develop:master`)             |
| `commit`               | Commit all dirty repos (**ALWAYS use `--json`**)       |
| `handoff check`        | Can I walk away? Reports work that exists only here    |
| `handoff end`          | Commit + push all movable work (screens for secrets)   |
| `handoff start`        | Pull --rebase + prune all repos on arrival             |
| `cleanup branch`       | Clean merged/stale/gone branches                       |
| `forge from`           | Sync from GitHub/GitLab/Gitea org                      |
| `forge config generate`| Generate config from Forge API                         |
| `workspace init`       | Scan directory → generate config                       |
| `workspace sync`       | Clone/update from config (detailed preview)            |
| `config profile`       | Profile management (create/use/list)                   |
| `config recommended`   | Audit/apply git settings for multi-device work (`--apply`) |
| `doctor`               | Diagnose system, config, auth, forge health            |

## Configuration System

**5-Layer Precedence** (highest → lowest):
1. Command flags (`--provider gitlab`)
2. Project config (`.gz-git.yaml`)
3. Active profile (`~/.config/gz-git/profiles/{active}.yaml`)
4. Global config (`~/.config/gz-git/config.yaml`)
5. Built-in defaults

**Two formats**: `repositories` array (simple) or `workspaces` map (hierarchical)

**Branch config**: `defaultBranch: develop,master` — tries in order, falls back to repo default

```bash
gz-git config init && gz-git config profile create work && gz-git config profile use work
gz-git config show --effective    # Show effective config
```

Details: [config-guide.md](docs/.claude-context/config-guide.md)

## Core Design: Bulk-First

All commands operate in bulk mode by default.

```go
// pkg/repository/defaults.go
DefaultLocalScanDepth = 1   // local ops (status, fetch, pull...)
DefaultLocalParallel  = 10
DefaultForgeParallel  = 4   // lower for API rate limits
```

**Common flags**: `-d/--scan-depth` · `-j/--parallel` · `-n/--dry-run` · `--include/--exclude` · `-f/--format` (default|compact|json|llm) · `--full`

## Sync & Workspace Usage

```bash
# sync: all-in-one command
gz-git sync                    # auto-init if no .gz-git.yaml, else sync
gz-git sync --dry-run          # preview
gz-git sync --check            # sync + status after
gz-git sync -c config.yaml     # explicit config

# workspace detail
gz-git workspace init . -d 3   # scan depth 3
gz-git workspace sync --dry-run
gz-git workspace add https://github.com/user/repo.git
```

## Handoff Usage (multi-device / multi-agent)

`sync` aligns the **set** of repositories against a config. `handoff` moves the **work
state** of the repositories already present — do not confuse the two.

```bash
gz-git handoff check              # verdict: SAFE TO LEAVE / NOT YET / BLOCKED (no network)
gz-git handoff end                # commit + push everything movable, before leaving
gz-git handoff end --dry-run      # what would be committed, and what the guard flags
gz-git handoff end --no-push      # checkpoint offline
gz-git handoff start              # pull --rebase + prune, on arrival
```

`handoff end` screens every repository before committing: credential filenames and
contents, files over 5 MiB, untracked build output missing from `.gitignore`. Flagged
repositories are held back, not committed — `--force` overrides. Stash entries are never
moved automatically; they are invisible to every other machine by design.

## Forge Usage

```bash
gz-git forge from --provider gitlab --org mygroup --path ~/repos \
  --base-url https://gitlab.com --token $GITLAB_TOKEN

# Filter: --language go --min-stars 100 --last-push-within 30d
# Subgroups: --include-subgroups --subgroup-mode flat

gz-git forge config generate --provider gitlab --org devbox -o .gz-git.yaml
gz-git forge status -c sync.yaml --verbose
```

**Health symbols**: `✓` healthy · `⚠` warning · `✗` error · `⊘` unreachable

## Push with Refspec

```bash
gz-git push --refspec develop:master         # local:remote
gz-git push --refspec +develop:master        # raw force — refused unless forceMode: allow
```

## Push Policy

`push.policy` gates `push` and `handoff end`. Separate from
`branch.protectedBranches`, which only guards deletion.

```yaml
push:
  policy:
    protected: [main, master]   # never push here; the destination decides
    forceMode: lease-only       # lease-only (default) | allow | deny
```

`lease-only` allows `--force` (which uses `--force-with-lease`) and refuses a
`+` refspec, which has none — it applies even with no config file, so the two
force paths behave the same. `--force-mode` overrides it per invocation.
Refused repositories are reported as `blocked`; the rest of the batch runs.

## Security (CRITICAL)

```go
// SAFE
cmd := exec.Command("git", "clone", url)

// DANGEROUS — NEVER
cmd := exec.Command("sh", "-c", "git clone "+url)
```

See [security-guide.md](docs/.claude-context/security-guide.md)

## Shared Library

```go
import (
    "github.com/gizzahub/gzh-cli-core/logger"
    "github.com/gizzahub/gzh-cli-core/errors"
    "github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)
// testutil.TempGitRepo(t) / testutil.TempGitRepoWithCommit(t)
```

## Context Docs

| Guide | Purpose |
| ----- | ------- |
| [config-guide.md](docs/.claude-context/config-guide.md) | Profiles, hierarchical config |
| [common-tasks.md](docs/.claude-context/common-tasks.md) | Adding commands, testing |
| [security-guide.md](docs/.claude-context/security-guide.md) | Input sanitization |

**Read before modifying**: `cmd/AGENTS_COMMON.md` · `cmd/gz-git/AGENTS.md`

## Common Mistakes

1. **Not sanitizing git inputs** → Use `internal/gitcmd`
2. **Shell execution** → Use `exec.Command("git", args...)`
3. **Logging credentials** → Strip URLs before logging

## Git Commit Format

```
{type}({scope}): {description}
Model: claude-{model}
Co-Authored-By: Claude <noreply@anthropic.com>
```

**Types**: feat, fix, docs, refactor, test, chore | **Scope**: REQUIRED
