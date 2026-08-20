---
name: gz-git
description: |
  gz-git CLI for safe Git operations. Use when:
  - Managing Git repositories (single or multiple)
  - Bulk status/fetch/pull/push/update/diff/commit/switch across repos
  - Syncing repos from GitHub/GitLab/Gitea (sync from-forge/from-config)
  - Generating config from local scan or forge API (sync config scan/generate)
  - Watching repos for changes (watch)
  gz-git operates in BULK MODE by default (scans directories for repos).
allowed-tools: Bash, Read, Grep, Glob
---

# gz-git CLI Reference

This is a lightweight, stable reference for the `gz-git` CLI.
For the full and authoritative flag list, prefer:

```bash
gz-git --help
gz-git <command> --help
```

Curated docs in this repo:

- `docs/commands/README.md`: `../../../docs/commands/README.md`
- `docs/commands/watch.md`: `../../../docs/commands/watch.md`

## Core Concept: Bulk-First Design

Most `gz-git` commands scan a directory for repositories and process them in parallel.

### Defaults

| Setting        | Default | Description                      |
| -------------- | ------- | -------------------------------- |
| `--scan-depth` | `1`     | Current directory + 1 level deep |
| `--parallel`   | `10`    | Process 5 repos concurrently     |

### Common Bulk Flags

```bash
-d, --scan-depth INT     # Directory depth (default: 1)
-j, --parallel INT       # Parallel workers (default: 10)
-n, --dry-run            # Preview without executing
-r, --recursive          # Include nested repos/submodules
    --include REGEX      # Include repos matching pattern
    --exclude REGEX      # Exclude repos matching pattern
-f, --format FORMAT      # default, compact, json, llm
    --watch              # Run continuously at intervals
    --interval DURATION  # Interval when watching
```

## Common Commands

### Status / Fetch / Pull / Push / Update

```bash
gz-git status -d 2 ~/projects
gz-git fetch  -d 2 ~/projects --all-remotes --prune --tags
gz-git pull   -d 2 ~/projects --strategy rebase --stash
gz-git push   -d 2 ~/projects --set-upstream
gz-git update -d 2 ~/projects --watch --interval 5m
```

### Diff

```bash
gz-git diff -d 2 ~/projects
gz-git diff --staged --format compact ~/projects
gz-git diff --include-untracked --context 5 ~/projects
```

### Commit (Bulk)

Default is preview; use `--yes` to commit.
**CRITICAL FOR LLMs**: When generating commit messages automatically, ALWAYS use the `--json` flag. It is the most robust and efficient method for AI pipelines.

```bash
gz-git commit --dry-run -d 2 ~/projects
gz-git commit --yes -d 2 ~/projects
gz-git commit --all "chore: sync all repos" --yes -d 2 ~/projects
gz-git commit -m "frontend:feat: add login" -m "backend:fix: null check" --yes -d 2 ~/projects
gz-git commit --json '{"repo1":"feat: one", "repo2":"fix: two"}' --yes -d 2 ~/projects  # BEST for LLMs
gz-git commit --file /tmp/messages.json --yes -d 2 ~/projects
```

### Branch Helpers

Basic branch creation/deletion remains native `git`. `gz-git` provides:

```bash
gz-git branch list -a -d 2 ~/projects
gz-git switch feature/new --create -d 2 ~/projects
gz-git cleanup branch --merged --force -d 2 ~/projects
gz-git cleanup branch --bots --merged --remote --format json .
# leftover Dependabot/Renovate remotes; details: skill:branch-cleanup
# cleanup -r is --remote (not recursive). not /git:dependabot-merge
gz-git conflict detect feature/new main
```

### Clone (Bulk)

```bash
gz-git clone --url https://github.com/user/repo1.git --url https://github.com/user/repo2.git
gz-git clone ~/projects --file repos.txt
gz-git clone --update --file repos.txt
```

### Sync (Multi-Repo Management)

```bash
# From Git Forge (GitHub/GitLab/Gitea)
gz-git sync from-forge --provider github --org myorg --target ./repos --token $GITHUB_TOKEN
gz-git sync from-forge --provider gitlab --org devbox --include-subgroups --subgroup-mode flat

# From YAML Config
gz-git sync from-config -c sync.yaml --dry-run

# Config Management
gz-git sync config scan ~/mydevbox --strategy unified -o sync.yaml
gz-git sync config scan ~/mydevbox --strategy per-directory --no-gitignore
gz-git sync config generate --provider gitlab --org devbox --token $TOKEN -o sync.yaml
gz-git sync config merge --provider gitlab --org another-group --into sync.yaml --mode append
gz-git sync config init -o sample.yaml
gz-git sync config validate -c sync.yaml
```

### Watch

```bash
gz-git watch
gz-git watch /path/to/repo1 /path/to/repo2 --interval 5s --format llm
```

### Stash / Tag

```bash
gz-git stash save . -m "WIP: before refactor"
gz-git stash list .
gz-git stash pop .

gz-git tag list .
gz-git tag create v1.0.0 . -m "Release 1.0.0"
gz-git tag auto . --bump=patch
gz-git tag push .
```

## Push with Refspec

Push local branches to different remote branches using refspec syntax.

### Syntax

```bash
# Push develop → master
gz-git push --refspec develop:master

# Force push (uses --force-with-lease)
gz-git push --refspec +develop:master

# Multiple remotes
gz-git push --refspec develop:master --remote origin --remote backup

# Preview first
gz-git push --refspec develop:master --dry-run
```

### Automatic Validation

gz-git validates refspecs **before** executing:

1. **Format check** - Git branch name rules
2. **Source branch check** - Local branch must exist
3. **Commit count** - Calculates actual push commits
4. **Remote branch check** - Verifies remote branch exists

### Valid/Invalid Formats

| Valid                                   | Invalid (auto-error)     |
| --------------------------------------- | ------------------------ |
| `branch`                                | `develop::master` (::)   |
| `local:remote`                          | `branch name` (space)    |
| `+local:remote` (force)                 | `-invalid` (starts -)    |
| `refs/heads/main:refs/heads/master`     | `branch.` (ends .)       |

### Error Messages

```bash
# Source branch not found
✗ agent-mesh-cli (master)  failed  10ms
  ⚠  refspec source branch 'develop' not found in repository (current branch: master)

# Invalid format
Error: invalid refspec: refspec contains invalid character
```

## Discovery Modes

When using hierarchical config, control how workspaces are discovered.

### Modes

| Mode       | Behavior                                          |
| ---------- | ------------------------------------------------- |
| `explicit` | Only use workspaces defined in config             |
| `auto`     | Scan directories, ignore config workspaces        |
| `hybrid`   | Use config if defined, otherwise scan **(default)** |

### Usage

```bash
gz-git status --discovery-mode explicit
gz-git status --discovery-mode auto
gz-git status --discovery-mode hybrid  # default
```

### Config Example

```yaml
# .gz-git.yaml
workspaces:
  gzh-cli:
    path: gzh-cli
    type: git
  gzh-cli-gitforge:
    path: gzh-cli-gitforge
    type: config  # has its own .gz-git.yaml
```

## Sync Status (Health Check)

Diagnose repository health with `sync status`.

### Commands

```bash
# Config-based health check
gz-git sync status -c sync.yaml

# Directory scan + health check
gz-git sync status --target ~/repos --depth 2

# Fast check (skip remote fetch)
gz-git sync status -c sync.yaml --skip-fetch

# Custom timeout (slow network)
gz-git sync status -c sync.yaml --timeout 60s

# Verbose output
gz-git sync status -c sync.yaml --verbose
```

### Output Example

```
✓ gzh-cli (master)              healthy     up-to-date
⚠ gzh-cli-gitforge (develop)   warning     3↓ 2↑ diverged
  → Use 'git pull --rebase' or 'git merge' to reconcile
✗ gzh-cli-quality (main)        error       dirty + 5↓ behind
  → Commit or stash 3 modified files, then pull
⊘ gzh-cli-template (master)     timeout     fetch failed (30s timeout)
  → Check network connection

Summary: 1 healthy, 1 warning, 1 error, 1 unreachable
```

### Health Status Legend

- `✓ healthy` - Up-to-date, clean working tree
- `⚠ warning` - Diverged/behind/ahead (resolvable)
- `✗ error` - Conflicts, dirty + behind (manual fix needed)
- `⊘ unreachable` - Network timeout, auth failed

See: `skill:sync-troubleshooting` for detailed diagnostics.

## Config Commands (Quick Reference)

```bash
# Show current config
gz-git config show

# Show effective config with precedence sources
gz-git config show --effective

# Show config hierarchy tree
gz-git config hierarchy

# Profile management
gz-git config profile list
gz-git config profile use work
gz-git config profile show work
gz-git config profile create work --provider gitlab --base-url https://gitlab.com
```

See: `skill:config-profiles` for profile setup guide.

## Related Skills

| Skill                    | Purpose                               |
| ------------------------ | ------------------------------------- |
| `workspace-management`   | Local workspace CLI operations        |
| `config-profiles`        | Profile creation and management       |
| `forge-sync`             | GitHub/GitLab/Gitea sync              |
| `branch-cleanup`         | Merged/stale/gone and leftover bot remotes |
| `sync-troubleshooting`   | Sync diagnostics and error resolution |
| `devbox-setup`           | Multi-repo Makefile setup             |
