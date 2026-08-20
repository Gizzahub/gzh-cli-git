---
name: branch-cleanup
description: |
  Guide for cleaning up merged, stale, gone, and leftover bot remote
  branches with gz-git. Use when:
  - Cleaning up branches after PR merge
  - Removing stale feature branches
  - Bulk branch cleanup across multiple repos
  - Identifying gone branches (remote deleted)
  - Protecting important branches from deletion
  - Reclaiming leftover Dependabot/Renovate/github-actions remotes
  - Seeing REMOTE_BOT_BRANCH_RECLAIMABLE or REMOTE_BOT_BRANCH_PENDING
allowed-tools: Bash, Read, Grep
---

# Branch Cleanup with gz-git

This skill covers branch cleanup operations using `gz-git cleanup branch`.

## Overview

| Cleanup Type | Description                                    |
| ------------ | ---------------------------------------------- |
| `--merged`   | Branches fully merged into base branch         |
| `--stale`    | Branches with no activity for N days           |
| `--gone`     | Branches whose remote tracking branch deleted  |

`--bots` is a name filter (`dependabot/` `renovate/` `github-actions/`), not a fourth type.

Single-repo unless a directory argument is given (that path is bulk). Bulk non-interactive delete needs `--force --yes`.

---

## Safety First: Dry-Run Default

**gz-git cleanup branch is DRY-RUN by default.** Nothing is deleted until you add `--force`.

```bash
# Preview only (safe)
gz-git cleanup branch --merged

# Actually delete
gz-git cleanup branch --merged --force
```

---

## Remote bot reclaim (LLM)

Leftover Dependabot / Renovate / github-actions remotes after a squash-merge.
This is not `/git:dependabot-merge` (that *applies* the update).

```bash
gz-git info --audit
gz-git cleanup branch --bots --merged --remote --format json .
# only when the user asked to delete:
gz-git cleanup branch --bots --merged --remote --force --yes .
```

On this command `-r` is `--remote`, not recursive (recursive has no short flag).

| Audit code | Meaning | Autofix | Delete? |
| ---------- | ------- | ------- | ------- |
| `REMOTE_BOT_BRANCH_RECLAIMABLE` | tip is an ancestor of the base | false | yes, with `--force --yes` if the user asked |
| `REMOTE_BOT_BRANCH_PENDING` | unmerged; may be an open PR | false | no |

- JSON schema: `gz-git.cleanup.branch/v1`
- Remote delete only for ancestry-merged refs (`merge-base --is-ancestor`)
- `--stale` never deletes remote-only names
- `--merged --remote` without `--bots` deletes **every** merged remote, not just bots
- Protected: `main` `master` `develop` `development` `release/*` `hotfix/*`
- Dry-run until `--force`. Bulk non-interactive delete also needs `--yes`

---

## Quick Start

### Single Repository

```bash
# Preview merged branches
gz-git cleanup branch --merged

# Delete merged branches
gz-git cleanup branch --merged --force

# Preview stale branches (30+ days)
gz-git cleanup branch --stale

# Delete stale branches
gz-git cleanup branch --stale --force

# Preview gone branches
gz-git cleanup branch --gone

# Combine types
gz-git cleanup branch --merged --stale --gone --force
```

### Bulk Mode (Multiple Repos)

```bash
# Preview across all repos in current dir
gz-git cleanup branch --merged .

# Delete across all repos
gz-git cleanup branch --merged --force --yes .

# Scan deeper (2 levels)
gz-git cleanup branch --merged --force --yes -d 2 ~/repos

# Include/exclude patterns
gz-git cleanup branch --merged --force --yes --include "api-*" .
gz-git cleanup branch --merged --force --yes --exclude "legacy-*" .
```

---

## Cleanup Types

### Merged Branches (`--merged`)

Branches fully merged into the base branch (main/master/develop).

```bash
# Preview merged branches
gz-git cleanup branch --merged

# Output:
# Repository: project-api
#   feature/login       → merged into main
#   feature/signup      → merged into main
#   fix/typo            → merged into main
#
# Would delete 3 branches (dry-run)
```

**Auto-detection**: Base branch is auto-detected from:
1. `main` (if exists)
2. `master` (if exists)
3. Default branch from remote

```bash
# Override base branch
gz-git cleanup branch --merged --base develop
```

### Stale Branches (`--stale`)

Branches with no commits for N days (default: 30).

```bash
# Preview stale branches (30+ days)
gz-git cleanup branch --stale

# Custom threshold (60 days)
gz-git cleanup branch --stale --stale-days 60

# Output:
# Repository: project-api
#   feature/old-idea    → last commit 45 days ago
#   experiment/test     → last commit 120 days ago
#
# Would delete 2 branches (dry-run)
```

### Gone Branches (`--gone`)

Branches whose upstream tracking branch was deleted (usually after PR merge).

```bash
# Preview gone branches
gz-git cleanup branch --gone

# Output:
# Repository: project-api
#   feature/merged-pr   → upstream gone (origin/feature/merged-pr deleted)
#
# Would delete 1 branch (dry-run)
```

**Common Scenario**: After merging a PR on GitHub/GitLab, the remote branch is deleted, but your local branch remains. `--gone` cleans these up.

---

## Protected Branches

Some branches are **always protected** and never deleted:

| Protected by Default |
| -------------------- |
| `main`               |
| `master`             |
| `develop`            |
| `development`        |
| `release/*`          |
| `hotfix/*`           |

### Add Custom Protected Branches

Add names with `--protect`. Cleanup does not read `branch.protectedBranches`
from config (that field is schema-only for this command).

```bash
# Protect additional branches
gz-git cleanup branch --merged --protect "staging,qa,prod" --force

# Protect with patterns
gz-git cleanup branch --merged --protect "env/*,deploy/*" --force
```

---

## Remote Branch Deletion

By default, only **local branches** are deleted. To also delete remote branches:

```bash
# Delete local + remote branches
gz-git cleanup branch --merged --remote --force

# Output:
# Repository: project-api
#   feature/login       → deleted (local)
#   feature/login       → deleted (origin)
```

**Warning**: Remote deletion is permanent. Use dry-run first.
`--merged --remote` without `--bots` deletes every merged remote, including human topic branches.

```bash
# Preview remote deletion
gz-git cleanup branch --merged --remote

# Preview leftover bot remotes only
gz-git cleanup branch --bots --merged --remote --format json .
```

---

## Bulk Mode Options

| Flag           | Default | Description                        |
| -------------- | ------- | ---------------------------------- |
| `-d, --scan-depth` | `1`  | Directory depth to scan            |
| `-j, --parallel`   | `10` | Parallel operations                |
| `--include`    | -       | Regex pattern to include repos     |
| `--exclude`    | -       | Regex pattern to exclude repos     |
| `--recursive`  | `false` | Include nested repos/submodules    |

```bash
# Scan 2 levels deep
gz-git cleanup branch --merged --force --yes -d 2 ~/repos

# Only frontend repos
gz-git cleanup branch --merged --force --yes --include "frontend-*" .

# Exclude vendor repos
gz-git cleanup branch --merged --force --yes --exclude "vendor|third-party" .

# Include submodules
gz-git cleanup branch --merged --force --yes --recursive .
```

---

## Interactive Wizard

For guided cleanup:

```bash
gz-git cleanup wizard

# Interactive prompts:
# ? Select cleanup type: [merged/stale/gone/all]
# ? Include remote branches? [y/N]
# ? Protect additional branches: staging, qa
# ? Proceed with deletion? [y/N]
```

---

## Common Workflows

### After PR Merge

```bash
# 1. Fetch to update remote refs
gz-git fetch --prune

# 2. Clean up gone branches
gz-git cleanup branch --gone --force
```

### Weekly Maintenance

```bash
# Preview all cleanup types
gz-git cleanup branch --merged --stale --gone .

# Review output, then execute
gz-git cleanup branch --merged --stale --gone --force --yes .
```

### Pre-Release Cleanup

```bash
# Clean up merged feature branches
gz-git cleanup branch --merged --force --yes .

# Clean up stale branches (90+ days)
gz-git cleanup branch --stale --stale-days 90 --force --yes .
```

### Monorepo Cleanup

```bash
# Scan entire monorepo (deep)
gz-git cleanup branch --merged --force --yes -d 3 --recursive ~/monorepo

# Exclude certain paths
gz-git cleanup branch --merged --force --yes --exclude "vendor|node_modules" .
```

---

## Output Examples

### Dry-Run Output

```
Scanning repositories...

Repository: project-core (main)
  Merged branches:
    feature/auth        → merged into main (2 days ago)
    feature/api         → merged into main (5 days ago)
  Stale branches:
    experiment/old      → last commit 45 days ago

Repository: project-api (main)
  Merged branches:
    fix/bug-123         → merged into main (1 day ago)
  Gone branches:
    feature/pr-456      → upstream deleted

Summary:
  Repositories scanned: 2
  Branches to delete:   5

[DRY-RUN] Use --force to actually delete branches
```

### Force Output

```
Scanning repositories...

Repository: project-core (main)
  ✓ Deleted: feature/auth
  ✓ Deleted: feature/api
  ✓ Deleted: experiment/old

Repository: project-api (main)
  ✓ Deleted: fix/bug-123
  ✓ Deleted: feature/pr-456

Summary:
  Repositories processed: 2
  Branches deleted:       5
  Errors:                 0
```

---

## Troubleshooting

### "branch is not fully merged"

The branch has commits not in the base branch.

```bash
# Check if really merged
git log main..feature/branch

# Force delete (careful!)
git branch -D feature/branch
```

### Protected name was skipped

Built-in protected names are skipped, not erroring. Extra names use `--protect`.
Cleanup does not read `branch.protectedBranches` from config.

### "failed to delete remote branch"

Cleanup leases the classified SHA
(`--force-with-lease=refs/heads/<name>:<sha>`). Empty SHA refuses the
delete. Do not fall back to unleased `git push origin --delete`.

```bash
gz-git fetch --prune
gz-git cleanup branch --bots --merged --remote --format json .
```

If the tip moved, the lease refuses on purpose. Re-run dry-run. Push
permission is still required.

### "no branches to clean up"

```bash
# Ensure fetch is up-to-date
gz-git fetch --prune .

# Check with verbose
gz-git cleanup branch --merged -v .

# Try different types
gz-git cleanup branch --stale --stale-days 7 .
```

---

## Quick Reference

| Task                        | Command                                      |
| --------------------------- | -------------------------------------------- |
| Preview merged              | `cleanup branch --merged`                    |
| Delete merged               | `cleanup branch --merged --force`            |
| Preview stale (30d)         | `cleanup branch --stale`                     |
| Delete stale (60d)          | `cleanup branch --stale --stale-days 60 --force` |
| Preview gone                | `cleanup branch --gone`                      |
| Delete gone                 | `cleanup branch --gone --force`              |
| Preview leftover bot remotes | `cleanup branch --bots --merged --remote --format json .` |
| Delete leftover bot remotes | `cleanup branch --bots --merged --remote --force --yes .` |
| All types                   | `cleanup branch --merged --stale --gone`     |
| Bulk mode                   | `cleanup branch --merged --force --yes .`          |
| With remote                 | `cleanup branch --merged --remote --force`   |
| Protect branches            | `--protect "staging,qa"`                     |
| Custom base                 | `--base develop`                             |
| Interactive                 | `cleanup wizard`                             |

---

## Branch Types Summary

| Type     | Condition                          | Safe to Delete?       |
| -------- | ---------------------------------- | --------------------- |
| Merged   | All commits in base branch         | ✓ Yes                 |
| Stale    | No commits for N days              | ⚠ Review first        |
| Gone     | Remote tracking deleted            | ✓ Usually yes         |
| Protected| In protected list                  | ✗ Never auto-deleted  |
