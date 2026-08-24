# gz-git Command Reference

Curated reference and workflow guide for `gz-git`.
For the authoritative list of flags and defaults, use:

```bash
gz-git --help
gz-git <command> --help
```

## Global Flags

| Flag        | Short | Description               |
| ----------- | ----- | ------------------------- |
| `--quiet`   | `-q`  | Suppress non-error output |
| `--verbose` | `-v`  | Show detailed output      |
| `--help`    | `-h`  | Show command help         |
| `--version` |       | Show version              |

## Bulk-First Model

Most day-to-day commands are **bulk commands**: they scan a directory for Git repositories and run the operation in parallel.

### Common Bulk Flags

| Flag           | Short | Description                                        | Default   |
| -------------- | ----- | -------------------------------------------------- | --------- |
| `--scan-depth` | `-d`  | Directory depth to scan for repositories           | `1`       |
| `--parallel`   | `-j`  | Number of parallel operations                      | `10`      |
| `--dry-run`    | `-n`  | Preview without executing                          | `false`   |
| `--recursive`  | `-r`  | Recursively include nested repos and submodules    | `false`   |
| `--include`    |       | Include repositories matching regex                |           |
| `--exclude`    |       | Exclude repositories matching regex                |           |
| `--format`     | `-f`  | Output format: `default`, `compact`, `json`, `llm` | `default` |
| `--watch`      |       | Run continuously at intervals                      | `false`   |
| `--interval`   |       | Interval when watching                             | `5m`      |

### Output Formats

- `default`: human-readable, multi-line output
- `compact`: one-line summary per repository/event
- `json`: machine-readable output (CI / scripting)
- `llm`: prompt-friendly output for LLM tooling

## Core Operations (Bulk)

### status

Scan repositories and show working tree + ahead/behind summary.

```bash
gz-git status [directory] [flags]
gz-git status -d 2 ~/projects
gz-git status --format compact ~/projects
gz-git status --watch --interval 30s -d 2 ~/projects
```

### fetch

Fetch updates without touching your working tree.

```bash
gz-git fetch [directory] [flags]
gz-git fetch -d 2 ~/projects
gz-git fetch --all-remotes ~/projects
gz-git fetch --prune --tags ~/projects
gz-git fetch --watch --interval 1m -d 2 ~/projects
```

### pull

Fetch + integrate changes (merge/rebase/ff-only).

```bash
gz-git pull [directory] [flags]
gz-git pull -d 2 ~/projects
gz-git pull --strategy rebase ~/projects
gz-git pull --strategy ff-only ~/projects
gz-git pull --stash --prune --tags ~/projects
```

### push

Push commits to remotes.

```bash
gz-git push [directory] [flags]
gz-git push -d 2 ~/projects
gz-git push --set-upstream ~/projects
gz-git push --tags ~/projects
gz-git push --remote origin --remote backup --refspec develop:master ~/projects
gz-git push --all-remotes --ignore-dirty ~/projects
```

### update

Update repositories from remote using `git pull --rebase` (safe defaults).

```bash
gz-git update [directory] [flags]
gz-git update -d 2 ~/projects
gz-git update --no-fetch ~/projects
gz-git update --watch --interval 5m -d 2 ~/projects
gz-git update --sync-base -d 2 ~/projects
gz-git update --sync-base --create-missing-base -d 2 ~/projects
```

`--sync-base` also fast-forwards each repository's local base ref (the one
`info` reports `BASE` against) without checking it out. A base that diverged
from its remote is moved only when every commit it would move off already
exists on the remote under some other branch; otherwise it is reported and left
alone. `--create-missing-base` additionally creates the base branch in
repositories that have none, and requires `--sync-base`. See
[troubleshooting](../user/guides/troubleshooting.md#why-base-is-often-large).

### diff

Show diffs across repositories with uncommitted changes.

```bash
gz-git diff [directory] [flags]
gz-git diff -d 2 ~/projects
gz-git diff --staged ~/projects
gz-git diff --include-untracked --context 5 ~/projects
gz-git diff --no-content --format compact ~/projects
```

## Commit (Bulk)

### commit

Scan repositories and commit changes in parallel.
Default is **preview**; use `--yes` to actually commit.

```bash
gz-git commit [directory] [flags]

# Preview
gz-git commit --dry-run -d 2 ~/projects

# Apply
gz-git commit --yes -d 2 ~/projects

# Same message for all repos
gz-git commit --all "chore: sync all repos" --yes -d 2 ~/projects

# Per-repo messages (format: repo:message)
gz-git commit -m "frontend:feat: add login" -m "backend:fix: handle null" --yes -d 2 ~/projects

# Load per-repo messages via pipe (JSON or YAML)
cat /tmp/messages.json | gz-git commit --yes -d 2 ~/projects
```

## Branch & Conflict

### branch list (Bulk)

List branches across repositories.

```bash
gz-git branch list [directory] [flags]
gz-git branch list -a -d 2 ~/projects
gz-git branch list --include "gzh-cli.*" ~/projects
```

### switch (Bulk)

Switch branches across repositories.

```bash
gz-git switch <branch> [directory] [flags]
gz-git switch develop -d 2 ~/projects
gz-git switch feature/new --create -d 2 ~/projects
gz-git switch main --dry-run -d 2 ~/projects
gz-git switch main --force -d 2 ~/projects
```

### conflict detect (Single repo)

Detect potential conflicts between branches without modifying the working tree.

```bash
gz-git conflict detect <source> <target> [flags]
gz-git conflict detect feature/new-feature main
```

## Cleanup

### cleanup branch

Clean up merged, stale, or gone branches. **Dry-run is the default**; use `--force` to delete.

```bash
gz-git cleanup branch [directory] [flags]
gz-git cleanup branch --merged                 # single repo (cwd)
gz-git cleanup branch --stale --stale-days 30  # single repo (cwd)
gz-git cleanup branch --merged --force .       # bulk mode
gz-git cleanup branch --merged --remote --force -d 2 ~/projects
gz-git cleanup branch --bots --merged --remote --format json .
gz-git cleanup branch --bots --superseded --remote --format json .
gz-git cleanup branch --bots --merged --remote --force --yes .
```

On this command `-r` is `--remote`; nested repos use `--recursive` (no short flag). `--bots` limits to `dependabot/` `renovate/` `github-actions/`. `--merged --remote` without `--bots` deletes every merged remote. `--superseded` only considers bot names (version already caught up; Autofix false). Dry-run until `--force`; bulk non-interactive delete also needs `--yes`. Audit codes: `REMOTE_BOT_BRANCH_RECLAIMABLE`, `REMOTE_BOT_BRANCH_SUPERSEDED`, `REMOTE_BOT_BRANCH_PENDING`.

## Clone (Bulk)

### clone

Bulk clone one or more repositories via `--url` (repeatable) or `--file`.

```bash
gz-git clone [directory] [flags]

# Clone into current directory
gz-git clone --url https://github.com/user/repo1.git --url https://github.com/user/repo2.git

# Clone into a target directory
gz-git clone ~/projects --url https://github.com/user/repo.git

# Clone from a file (one URL per line)
gz-git clone --file repos.txt

# Pull existing repositories instead of skipping
gz-git clone --update --file repos.txt
```

## Monitoring

### watch

Monitor repositories for changes in real-time.

```bash
gz-git watch [paths...] [flags]
gz-git watch
gz-git watch /path/to/repo1 /path/to/repo2
gz-git watch --interval 5s --format compact
```

More details: [docs/commands/watch.md](watch.md).

## Forge (Sync / Config)

### forge from

Sync repositories from a forge provider (GitHub, GitLab, Gitea).

```bash
gz-git forge from --provider github --org myorg --path ./repos --token $GITHUB_TOKEN
gz-git forge from --provider gitlab --org mygroup --path ./repos --base-url https://gitlab.company.com
```

### forge config generate

Generate a workspace config from a forge provider (GitHub, GitLab, Gitea).

```bash
gz-git forge config generate --provider gitlab --org mygroup -o .gz-git.yaml
gz-git workspace sync
```

## Stash

```bash
gz-git stash save [directory] -m "WIP: before refactor"
gz-git stash list [directory]
gz-git stash pop [directory]
```

## Tags

```bash
gz-git tag list [directory]
gz-git tag status [directory]
gz-git tag create v1.0.0 [directory] -m "Release 1.0.0"
gz-git tag auto [directory] --bump=patch
gz-git tag push [directory]
```

## Info (Bulk)

### info

One row per repository: branch, divergence, working-tree state, remotes and
stray branches. Bulk like the rest — `-d/--scan-depth` applies.

```bash
gz-git info                       # current directory, depth 1
gz-git info -d 3 ~/projects       # scan 3 levels deep
gz-git info --compact             # drop columns nothing has anything to report in
gz-git info --full                # per-repository detail block instead of the table
gz-git info --audit               # machine-readable branch audit (JSON)
```

#### Two divergence axes

`BRANCH` and `BASE` both render as `↑ahead ↓behind`, but they compare against
different refs. Reading one as the other is the usual source of "`info` says I
have unpushed commits but `push` says up-to-date":

| Column   | Compares HEAD against                        | Moved by           |
| -------- | -------------------------------------------- | ------------------ |
| `BRANCH` | its upstream (`@{upstream}`)                 | `push` / `pull`    |
| `BASE`   | the **local** base branch (`master`, `main`)  | `merge` / `rebase` |

A legend under the summary line names both. `push` acts on the `BRANCH` axis
only, so a repository with a blank `BRANCH` and a large `BASE` is genuinely
up-to-date with its remote. The base is a local ref that `gz-git` never moves —
a stale local `master` inflates the column on its own. Base resolution order:
`branch.defaultBranch` candidates from config, first that exists locally, else
`main`, `master`, `develop`, `development`.

Other columns: `WT` linked worktrees · `DIRTY` as `+staged ~unstaged ?untracked`
(or `REBASING` / `MERGING` / `conflict:N`, which outrank the counts) · `REMOTE`
· `OTHER BRANCHES` · `REMOTE ONLY`. Row markers: `🔸` needs attention, `🔻`
blocked (mid-rebase/merge, conflicts, or a scan error).

See [troubleshooting](../user/guides/troubleshooting.md) for the diagnostic
recipes.

## History (Single repo)

### history

```bash
gz-git history stats --since "1 month ago"
gz-git history contributors --top 10
gz-git history file README.md --follow
gz-git history blame README.md
```

## Diagnostics

### doctor

Check system dependencies, config integrity, authentication, and forge connectivity.

```bash
gz-git doctor                # Run all checks
gz-git doctor -v             # Verbose (per-profile details)
gz-git doctor --skip-forge   # Skip forge API connectivity checks
gz-git doctor --format json  # Machine-readable output
```

## Misc

```bash
gz-git version --short
gz-git completion zsh > _gz-git
```
