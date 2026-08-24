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

This fast-forwards each stale base ref and reports which ones moved. Preview
with `--dry-run` first.

### When the base holds commits the remote lacks

That is not a fast-forward, so the ref cannot simply catch up — and the two
situations it can mean are opposites:

- The base is parked on the tip of a task branch that **was** pushed. Every
  commit is reachable from some remote-tracking ref, so moving the pointer
  loses nothing. This is adopted: `base master +12 (adopted: 2 local commit(s)
  already pushed elsewhere; old tip at refs/gz-git/base-backup/master)`.
- The base carries commits that exist **nowhere else** — never pushed, on no
  branch the remote has. Moving the pointer would leave them reachable only
  from the reflog. This is refused: `base master blocked: 2 commit(s) exist
  only here`.

The count that decides is "commits on the local base reachable from no
remote-tracking ref", not "commits the remote base lacks". A blocked base is
reported and left exactly as it was; resolve it by pushing the commits
somewhere, then run again.

#### The backup ref, and why adopting needs one

Note the exact wording above: *reachable from a remote-tracking ref*, not *on
the remote*. Those are not the same claim. `refs/remotes/origin/*` is a local
cache — a tracking ref for a branch someone deleted upstream survives until the
next `fetch --prune`, and until then it stands as evidence for a commit that is
no longer anywhere. The decision can therefore be wrong in exactly one
direction, and it is the expensive one.

So an adopt parks the old tip before it moves the ref:

```bash
git -C <repo> for-each-ref refs/gz-git/base-backup/   # what was moved off
git -C <repo> branch recovered refs/gz-git/base-backup/master
```

They live under `refs/gz-git/` rather than `refs/heads/`, so they never show up
in `git branch` and can never be picked as a base on a later run. Delete one
with `git update-ref -d refs/gz-git/base-backup/<base>` once you are satisfied.

An adopt that only rewinds — the local base was strictly *ahead* of its remote
— says so rather than reporting a distance of zero:

```text
base master rewound to origin (adopted: 2 local commit(s) already pushed elsewhere; old tip at refs/gz-git/base-backup/master)
```

### The base is checked out somewhere

A base a linked worktree is standing on is reported and left alone, the same as
one checked out here: `base master is checked out in worktree celee__mbp__feat__x`.

This is not caution for its own sake. `git update-ref` is plumbing and enforces
no checkout rule — unlike `git branch -f`, it will move a branch out from under
a worktree, leaving that worktree's index disagreeing with its HEAD, so every
file the moved-off commits added reads as a staged deletion and the next commit
made there quietly reverts them. Finish or remove the worktree, then run again.

### Repositories with no local base branch at all

A clone that was switched to `develop` on day one may have no `refs/heads/
master`. The base then resolves to `develop` — the branch you are standing on —
and the sync hands off to the normal pull path, so nothing is ever repaired and
`info`'s `BASE` column stays meaningless. Opt in to fixing it:

```bash
gz-git update --sync-base --create-missing-base -d 2 ~/projects
```

This creates the branch locally at the remote's tip. It requires `--sync-base`
and is off by default, because a repository deliberately kept without a local
trunk is a legitimate choice and creating a branch is not a repair.

Two things it deliberately will not do:

- **Retarget a base it could repair.** The create path only runs when the
  resolved base leaves nothing to fix — nothing resolved, or it resolved to the
  branch you are standing on. A repository with a stale local `master` gets that
  `master` repaired, not an unrelated `main` invented beside it. A base your
  config declares is never overridden either way.
- **Resurrect a branch deleted upstream.** Which branches exist is asked of the
  remote with `ls-remote`, not read out of `refs/remotes/`, so a tracking ref
  left behind by a deleted branch cannot become a local trunk. This is also why
  it finds `master` in a `clone --single-branch -b develop`, which has no
  `origin/master` ref to probe. It needs the network, so it does nothing under
  `--skip-fetch`.

### Doing it by hand

For a single repository:

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
