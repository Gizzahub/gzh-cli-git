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

The base is resolved to a **local** ref, and nothing moves it on its own:
`git fetch` updates `refs/remotes/*`, `git pull` updates the branch you are
standing on, and a base you never check out is neither. On a develop-based
workflow the local `master` stays wherever it was at clone time, so
`BASE ↑1275` means "develop holds 1275 commits your stale local master lacks",
not "1275 unpushed commits".

Repair it in bulk, without checking it out and without touching the branch you
are on:

```bash
gz-git update --sync-base -d 2 ~/projects
```

This fast-forwards each stale base ref and reports which ones moved. A base
that holds commits its remote lacks is **not** fast-forwarded — it is reported
as `base <name> blocked: ...` and left alone, because that is either work that
was never pushed or a ref parked on a task branch, and neither is something a
bulk command should decide for you. Preview with `--dry-run` first.

For a single repository the equivalent by hand is:

```bash
git -C <repo> fetch origin master:master
```

The missing `+` is the safety net: that refspec refuses a non-fast-forward.

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

## `integrate check` reports a lint baseline that is not there

The lint gate can report hundreds of issues under a directory that no longer
exists — typically the worktree a previous `integrate run` just reclaimed:

```text
../../../worktrees/.../claude__mst__fix__push-stderr/pkg/repository/bulk_switch.go:318:92: ...
237 issues:
```

golangci-lint caches analysis results keyed by absolute path, and the cache
outlives the directory.

The gate no longer does this. `integrate check` runs the lint target with a
private `GOLANGCI_LINT_CACHE` that it creates and removes per run, and any
diagnostic pointing outside the repository now fails the check by name instead
of being counted as a pre-existing baseline. If you still see it, your
installed binary predates that fix — see the next section.

Running `make lint` by hand is a different matter: it uses the shared cache,
so a reclaimed worktree can still haunt it. Clear the cache and re-run:

```bash
golangci-lint cache clean && GOWORK=off make lint
```

`GOWORK=off` matters inside a worktree: the tracked `go.work` names sibling
modules by relative path, and those do not resolve from a worktree checkout.

## A gate fix does not take effect until `make install`

`integrate check` and `integrate run` are executed by the `gz-git` on your
PATH, not by the source tree you are standing in. A fix to the gate itself is
therefore invisible to the gate until it is installed:

```bash
GOWORK=off make install
```

This is easy to miss precisely because the change looks landed — it is
committed, it is on the integration branch, and its tests pass. The judgment
still comes from the binary. When a gate behaves in a way the current source
cannot explain, check what is actually running before investigating the code:

```bash
gz-git --version
strings "$(command -v gz-git)" | grep -c "<a string your fix introduced>"
```

The same applies in reverse: after installing a gate change, an unexpected new
failure may be the fix working rather than a regression.

## See also

- [FAQ](faq.md) — what `gz-git` is and how it differs from `git`
- [Command reference](../../commands/README.md) — flags and workflows
- [Error handling design](../../10-architecture/50-data-flow-and-error-handling.md)
