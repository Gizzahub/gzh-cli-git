# Troubleshooting

Symptom-first guide for `gz-git`. For what a command does, see the
[command reference](../../commands/README.md); for what this project is, see the
[FAQ](faq.md).

## `info` shows `↑N` but `push` reports "up-to-date"

Both are right — they measure different things. `info` prints divergence in two
columns and both use the same `↑N ↓N` glyphs:

| Column   | Compares HEAD against                       | Moved by         |
| -------- | ------------------------------------------- | ---------------- |
| `BRANCH` | its upstream (`@{upstream}`)                | `push` / `pull`  |
| `BASE`   | the **local** base branch (`master`, `main`) | `merge` / `rebase` |

`push` only ever acts on the `BRANCH` axis. A repository that is fully pushed —
`BRANCH` blank — can still show a large `BASE` count, and `push` reporting
`up-to-date` for it is accurate.

The table prints a legend under the summary line naming both comparisons:

```text
126 repositories  ·  3.06s  ·  12 need attention
BRANCH ↑↓ = vs upstream  ·  BASE ↑↓ = vs base branch
```

Confirm either axis by hand — left count is ahead, right is behind:

```bash
git -C <repo> rev-list --left-right --count HEAD...@{upstream}   # BRANCH axis
git -C <repo> rev-list --left-right --count HEAD...master        # BASE axis
```

### Why `BASE` is often large

The base is resolved to a **local** ref and `gz-git` never moves it. On a
develop-based workflow the local `master` stays wherever it was at clone time,
so `BASE ↑1275` means "develop holds 1275 commits your stale local master
lacks", not "1275 unpushed commits". Fast-forward it without checking it out:

```bash
git -C <repo> fetch origin master:master
```

Resolution order: the `branch.defaultBranch` candidates from config, first one
that exists as a local ref; otherwise `main`, `master`, `develop`,
`development`. Sitting on the base branch prints nothing — HEAD cannot diverge
from itself.

## `push` failed but the message doesn't say why

Older builds reported only `push exited with code 128: exit status 128`,
discarding the stderr git had already produced. Current builds carry git's own
diagnosis on the same row:

```text
✗ tasuku-repo  origin: push exited with code 128: ERROR: Permission to iheanyi/tasuku.git denied to archmagece.
```

stderr is collapsed onto one line so the bulk table stays aligned — nothing is
truncated. On an older binary, reproduce the single repository by hand to see
the full text:

```bash
git -C <repo> push origin HEAD
```

## A repository pushes to a remote you don't own

A `permission denied` on push usually means `origin` points at an upstream you
only have read access to — common for a repository cloned directly rather than
from your own fork. Check where it actually points:

```bash
gz-git info -d3 | grep <repo>     # REMOTE column
git -C <repo> remote -v
```

Repoint it at your fork, or leave it read-only and expect `push` to keep
reporting it as failed.

## A bulk command printed nothing

Only `--quiet` fully silences a bulk command; `--format json` still prints. If
output is missing without `-q`, check the exit code before assuming the command
was skipped:

```bash
gz-git pull -d3; echo "EXIT=$?"
```

| Exit | Meaning                                  |
| ---- | ---------------------------------------- |
| 0    | all repositories processed successfully  |
| 1    | tool or configuration error (nothing ran)|
| 2    | one or more repositories failed          |

`--dry-run` reports the same scan without touching anything, which is the
cheapest way to confirm the command is finding your repositories at all.

## See also

- [FAQ](faq.md) — what `gz-git` is and how it differs from `git`
- [Command reference](../../commands/README.md) — flags and workflows
- [Error handling design](../../10-architecture/50-data-flow-and-error-handling.md)
