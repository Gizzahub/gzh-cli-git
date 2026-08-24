<!-- size-limit: 1000 -->

<!-- A changelog is an append-only record, not a guide read front to back, so the
     default 500-line document budget models the wrong genre.

     1000 is a ceiling, not an exemption, and it is sized from measurement: the
     2026-08 backlog was 460 lines across 78 entries, so one release's worth of
     changes fits in the headroom above the current length. Hitting it therefore
     means a release is overdue, not that this file needs trimming — cut the
     release and move that line into docs/changelog/.

     Do not shrink the budget to a value a single batch can exhaust mid-write.
     Exceeding it is a hard error that blocks edits to this file, and the remedy
     is itself an edit to this file — that is how the 2026-08-07 to 2026-08-24
     gap happened, when 71 commits went unrecorded behind a 45KB limit. -->

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Entries explain *why* a change was made — that is the part a generator cannot produce,
and the reason this file is written by hand. Keep each entry to roughly 3-5 lines and
group related commits into one entry; the exhaustive per-commit list is generated into
the GitHub Release notes by `.goreleaser.yaml` (`changelog.use: github`) and does not
belong here. When `[Unreleased]` outgrows the repository's document size gate, the fix
is to cut a release and move that line into `docs/changelog/`, not to write less.

## [Unreleased]

<!-- 2026-08-07 .. 2026-08-24. Reconstructed from commit history after this file went
     unmaintained for two weeks; grouped by theme rather than by commit. -->

### Added

- `gz-git integrate check` and `integrate run` land a task branch on the configured
  integration branch: `check` is read-only and reports every gate, `run` performs the
  merge and then reclaims the worktree, the local branch and the remote branch in the
  same step, exiting 3 when reclaim is incomplete rather than reporting success.
  `integrate queue` lists what is waiting. Configuration gains `integrationBranch` and
  `taskPattern`, declared at the repository root so `dev/*/*/*` is recognized as a task
  namespace instead of an ordinary branch.
  - Gates fail closed: a repository with no check gate is refused rather than passed,
    `make` probe targets must be on an allowlist before launch, and a remote delete is
    leased to the SHA that actually landed, so a branch that moved after the merge is
    not deleted. Without a declared integration branch, `origin/HEAD` is followed.
- `gz-git pr create` opens a pull or merge request on GitHub, GitLab and Gitea.
- `gz-git update --sync-base` repairs local base refs that have fallen behind their
  remote, which is what makes the BASE divergence column trustworthy.
- `gz-git info` prints one line per repository as a table. `--audit` reports diagnostic
  codes with a per-code autofix policy, `--compact` drops columns (the default now keeps
  them), and the BRANCH column folds in upstream divergence. Both arrow columns name what
  they compare: BRANCH is HEAD against `@{upstream}`, BASE is HEAD against the local base
  branch — a repository fully pushed can still show BASE arrows, which is not a push
  failure.
- `gz-git cleanup branch` reclaims merged remote bot branches and classifies superseded
  bot remotes, so an abandoned automation branch is distinguishable from a live one.
- `gz-git install --audit` reports shadowed and duplicated binaries on the install path —
  the case where a fix is committed but the binary being run is still the old one.
- `repository.ResolveBase` resolves a base branch config-first, and
  `cliutil.ExitReclaimIncomplete` gives callers a distinct exit code for that state.

### Fixed

- `gz-git push` surfaces git's stderr on a bulk push failure. It was swallowed, so a
  rejected push reported only a count and looked like a silent no-op.
- `gz-git info` shows remote-only branches, prefers top-level remotes, and keeps a label
  ambiguous rather than guessing when two remotes could claim it.
- `gz-git handoff` no longer subtracts untracked files from the tracked count, and blocks
  on untracked-only work instead of calling the repository safe to leave.
- Repository reads are scoped to `os.Root`, with child roots anchored and automatic
  config reads isolated, so a path escaping the repository cannot be followed.
- Provider token failures keep their diagnosis instead of collapsing into a generic
  error; the GitLab client is migrated off its deprecated surface, and `NewProvider` for
  Gitea no longer makes a network version check just to construct.
- `make install` writes only to the configured bindir, fails closed, and `make clean` no
  longer removes installed binaries.
- SSH key paths are shell-quoted, and `internal/gitcmd` input validation is aligned with
  what git actually accepts for refs and URLs.
- The `integrate` lint gate runs with a per-run `GOLANGCI_LINT_CACHE`, so a cached
  diagnostic from a deleted worktree is no longer reported against the current tree.
- Fail-closed corrections: protected branches are screened inside `Execute` (not only at
  the call site), and a failed stash-status git command is an error rather than "clean".

### Added

- `handoff check` and `doctor` now report the age of stash entries, not just their
  count. A stash never leaves the machine that made it, so an entry that outlived a
  week of handoff cycles is work nobody is coming back for on their own: `handoff check` gives it the new `stranded` reason instead of `stashed`, and `doctor` warns
  about it under `repo:{name}:stash`. Both commands stay quiet about a stash made
  today, which is what the command is for, and neither one touches any of them —
  restoring a stash is a decision, not a cleanup.
  - The oldest entry is found by comparing dates, not by taking the last line of
    `git stash list`: that order comes from the reflog, and pushing a stash on an older
    base leaves an entry whose date is not where its position suggests.
- `gz-git branch name <task>` builds the branch name a task should have here, from a
  template and the resolved identity: `--kind work` (`feat/{task}`), `--kind device`
  (`feat/{task}/{device}`) or `--kind agent` (`agent/{task}/{agent}`). It prints the
  name and creates nothing, so it composes with `gz-git switch --create` for bulk work
  and with plain git for one repository — `branch create` stays unexposed.
  - Templates are configurable per kind under `branch.naming`, merged one key at a time
    so overriding one leaves the other two at their defaults.
  - Every substituted value is slugified. The default device name is the hostname, and
    `Daves-MacBook.local` is not a legal branch name, so without this a template would
    work on one machine and fail on the next.
  - A `device` or `agent` branch whose segment is unnamed is refused rather than
    collapsed: the result would be the shared branch again under a longer name, which is
    the collision splitting the branch exists to avoid. A misspelled placeholder is
    reported instead of being baked into a name.
- `push.policy.foreignWork` refuses a force push that would discard remote commits whose
  identity trailers name a different device or agent, listing the commits at stake.
  `--force-with-lease` does not cover this: a lease compares the remote against the ref
  this machine last fetched, so it protects only until the next fetch — and a
  multi-device workflow fetches on arrival, after which the lease is satisfied and the
  other machine's work disappears silently. The check runs under `--dry-run` too, where
  finding out first is the point. `--foreign-work allow` overrides it.
  - It refuses only on positive evidence of another writer. An unsigned commit is
    unattributed, not attributed elsewhere, so rewriting your own hand-made commits and
    force pushing still works. The cost is that only commits made by `handoff end` can
    be attributed at all, and a machine that names no identity skips the check entirely.
- `gz-git handoff start` names the branches whose remote advanced under another device or
  agent while this machine was away. It reports rather than blocks — a rebase replays
  over those commits and loses nothing. It is the signal that a branch has two writers
  and should be split, which is the moment to do it rather than after a collision.
- `identity` config names the machine and the agent behind an automated commit, and
  `handoff end` writes them as `Device:` and `Agent:` git trailers on the checkpoint.
  A checkpoint is made with nobody watching, and git records only the author — the same
  person on every machine they own — so without a trailer the commit cannot say where
  the work is. `device` defaults to the hostname, so checkpoints are signed with no
  configuration at all; `agent` is empty unless something sets it. `GZ_GIT_DEVICE` and
  `GZ_GIT_AGENT` override the config, since an agent knows its own name at launch.
  `--no-trailers` omits them for one run.
  - Configure it globally rather than in a project's `.gz-git.yaml`: that file is
    committed, and every machine that cloned it would report the same device.
- `push.policy` config restricts what `push` and `handoff end` may write to a remote.
  `protected` lists branch names and trailing-`*` patterns that may not be pushed to at
  all — the **destination** decides, so `--refspec develop:main` is refused just as a
  direct push to `main` is. It is separate from `branch.protectedBranches`, which guards
  deletion, and is empty unless configured. `forceMode` picks how force pushes are
  treated: `lease-only`, `allow`, or `deny`. `--force-mode` overrides it for one
  invocation. A refused repository is reported as `blocked` with the rule named, the
  rest of the batch still runs, and the command exits non-zero.
  - `handoff end` applies the policy *before* committing, not only at the push, so an
    unattended checkpoint cannot land on a branch this workspace may not push to.
- `gz-git handoff` moves work between machines and agents. Where `status` reports how
  healthy a repository is and `sync` aligns the *set* of repositories against a config,
  `handoff` reports and moves the *work state*: whatever exists only on this machine.
  - `handoff check` gives one verdict — `SAFE TO LEAVE`, `NOT YET`, or `BLOCKED` —
    across every scanned repository, distinguishing what `handoff end` clears on its own
    (uncommitted files, unpushed commits, a missing upstream) from what needs a decision
    here (conflicts, an interrupted rebase, a detached HEAD, no remote, a stash). No
    network is used: unpushed commits are counted against the remote tracking ref, which
    only advances when this machine pushes.
  - `handoff end` commits and pushes everything movable, and leaves the rest untouched.
    Before committing, every repository is screened for credentials (by filename and by
    content — private key blocks, AWS/GitHub/GitLab/Slack/Google tokens), files over
    5 MiB, and untracked build output that `.gitignore` does not cover. Anything flagged
    is held back with the file named rather than swept into history; `--force` commits it
    anyway. `--no-push` checkpoints without a network.
  - `handoff start` pulls every repository with a rebase and prunes deleted remote
    branches, so commits that are still only here are replayed on top of what the remote
    gained instead of producing a merge commit. Repositories with uncommitted work are
    fetched but never rebased.
  - Stash entries are never treated as movable: they are invisible to every other machine,
    and turning one into a commit is a decision, not a cleanup step.
  - Exit codes follow the diagnostic convention: `0` nothing outstanding, `1` work remains,
    `2` the command could not run.
- `gz-git config recommended` audits the git configuration that a multi-device,
  multi-agent workflow depends on: `pull.rebase`, `rebase.autoStash`, `fetch.prune`,
  `push.autoSetupRemote`, `push.default`, `rerere.enabled`, `merge.conflictStyle`.
  It reports unset, mismatched, and (for settings needing a newer git than the one
  installed) unsupported values, and writes the missing ones with `--apply`.
  `--local` targets the current repository instead of `~/.gitconfig`. Boolean
  spellings git accepts (`yes`, `on`, `1`) are not reported as mismatches.
  Exit codes follow the diagnostic convention: `0` conforming, `1` drift found,
  `2` the audit could not run — so it works as a CI workstation gate.
- `gz-git doctor` reports the same audit as one aggregate `System` check, naming the
  keys that need changes; `--verbose` expands it to one check per setting.

### Changed (behavior change)

`gz-git push --refspec +local:remote` is now refused by default. `--force` has always
mapped to `--force-with-lease`, but a `+` refspec went straight to git as an unleased
force, so the two spellings of "force push" behaved differently and the safer one was
the longer one to type. The new `lease-only` default closes that, and applies with no
config file present. Set `push.policy.forceMode: allow`, or pass `--force-mode allow`,
to get the old behavior.

### Fixed (behavior change)

`diff` and `commit` parsed `git status --porcelain` independently and disagreed on what
counts as the change set, so a commit message written from a diff could omit files the
commit then recorded (reported case: `diff` said 4 files, `commit` recorded 7). Both now
share a single collector (`git status --porcelain -z -uall`) whose definition matches what
`git add -A` actually stages. The following values change for existing consumers:

- **`files_changed` grows on repositories with untracked directories.** Collapsed
  `?? docs/` entries are now expanded to their individual paths, so a preview that
  reported 1 reports N. The old number was wrong, not smaller — the commit always
  recorded N.
- **`additions` / `deletions` change on several shapes.** They are now derived from a
  single `git diff --numstat -z HEAD` instead of summing `git diff --stat --cached` and
  `git diff --stat`. Edits that were both staged and re-edited are no longer counted
  twice, untracked file lines are now counted at all, and file names are no longer
  mis-parsed as counts (`9-changed-deletions.txt` used to report `deletions: 9`).
- **`gz-git commit` exits non-zero when repositories are left uncommitted for unresolved
  merge conflicts** (`cliutil.ExitPartialFailed`, 2). It previously exited 0. A caller
  that only checks `$?` used to record the refusal as a clean success.
- **Repositories whose net change against HEAD is empty now preview as `clean`, not
  `would-commit`.** A `MM file` where the worktree happens to match HEAD would fail at
  commit time with "nothing to commit" — and, because the failure arrived after the
  preview, exit 0. Binary and mode-only changes are still previewed as committable:
  the decision uses the numstat record count, not the line totals, which are `0`/`0`
  for both.
- **Paths containing spaces or non-ASCII characters are emitted unquoted.** Porcelain
  C-quoting was previously passed through verbatim, producing paths that did not exist
  on disk and leaking quotes into generated commit subjects
  (`chore("dir with space): update 5 files`).

Key names and types in the JSON output are unchanged; only the values are corrected.

______________________________________________________________________

The status and health paths still parsed `git status --porcelain` the way `diff` and
`commit` used to, each with its own answer to "how many uncommitted files". All call
sites now share one parser over `git status --porcelain -z -uall`. File counts reported
by `gz-git status`, `gz-git info`, `fetch`, `pull` and `push` move in **three different
directions** — a consumer that only expects them to grow will read the shrinking cases as
a regression:

- **`untracked_files` grows on repositories with untracked directories** (values
  *increase*). Plain `--porcelain` collapses an untracked directory into one `docs/`
  entry, so two new files under it were reported as one. Observed on a fixture with two
  files in one untracked directory: `untracked_files` 1 → 2.
- **`uncommitted_files` shrinks in `status` and `info`** (values *decrease*). It was the
  raw porcelain line count, untracked entries included — which `untracked_files` then
  reported a second time. It now counts tracked paths only. Observed on this repository:
  `uncommitted_files` 21 → 18 against 18 tracked changes and 3 untracked files.
- **`uncommitted_files` shrinks where one path is both staged and modified** (values
  *decrease*). `fetch`/`pull`/`push` computed
  `len(StagedFiles) + len(ModifiedFiles)`, so an `MM` path counted twice.
- **`uncommitted_files` grows where a file was deleted in the working tree only** (values
  *increase*). The same sum omitted ` D` paths, which are recorded as deletions and
  neither staged nor modified, so deleting a tracked file could leave the count at 0.
- **The first entry of a status listing is no longer misclassified** (values *move*
  between fields). The porcelain payload was whitespace-trimmed before being parsed by
  column offset, so the leading space of the first record was eaten: ` M README.md` was
  read as `M EADME.md` — staged instead of modified, with the path shifted one byte.
  Only the first record was affected, which is why it survived a table of fixtures that
  checked later ones. Observed on a fixture with two working-tree deletions:
  `uncommitted_files` 1 → 2.
- **Paths containing spaces or non-ASCII characters are emitted unquoted**, matching the
  `diff`/`commit` fix above.
- **A conflicted repository reports its unmerged paths from the index** rather than from
  porcelain, so the paths are never C-quoted and do not depend on the working tree.
- **`checkRepositoryState` now fails instead of reporting a clean, conflict-free
  repository when git fails.** It checked only the error return, but the executor reports
  a failed git through an exit code and returns no error — so a repository whose
  `.git/index` was corrupt (git exit 128) read as having no conflicts, and the `push`
  conflict guard opened. Callers now receive the error.

API note: `UncommittedFiles` on `RepositoryFetchResult`, `RepositoryPullResult`,
`RepositoryPushResult` and `RepositoryStatusResult` is **deprecated** in favour of
`TrackedChangedFiles`, `StagedFiles` and `UnstagedFiles`. It still carries a value —
now the same as `TrackedChangedFiles`. `Status` gains `StagedCount`, `UnstagedCount` and
`TrackedChangedCount`; these cannot be derived from the existing slices, which keep only
the union of the two porcelain status characters. `internal/parser.ParseStatus`, an
unused duplicate of the same parser, is removed.

JSON key names are unchanged, including `uncommitted_files`.

Two further consequences of that shared parser, previously undocumented:

- **`gz-git status` and `gz-git info` classify a working-tree-only deletion as dirty.**
  The health check summed `len(ModifiedFiles) + len(StagedFiles)`, and a ` D` path is
  recorded in `DeletedFiles` alone — so `rm` on a tracked file left the repository
  reporting `clean` / `healthy`. It now uses `TrackedChangedCount`, which also stops a
  path that is both staged and modified from counting twice.
- **`gz-git switch`'s skip message names both counts.** `Has uncommitted changes (%d files) - skipping` became `Has uncommitted changes (%d tracked, %d untracked) - skipping`. The single number was `len(ModifiedFiles) + len(StagedFiles)` while the
  skip gate itself is `IsClean`, so a repository held back for untracked files alone
  announced `(0 files)`. Scripts matching on this string need updating.

______________________________________________________________________

A rename whose destination is intent-to-added made `gz-git status` report a repository
holding uncommitted work as **healthy, "No action needed", exit 0**. Three defects had to
line up, and all three are fixed:

- **`git status -z` rename records are now paired on either status column.** `-z` drops
  the `->` separator and moves the source path into the *next* record; the parser
  claimed that record only when the rename letter sat on the index side (`R `, `RM`,
  `RD`). git also emits it on the worktree side — ` R` — when the destination is
  intent-to-added while the source deletion stays unstaged (`mv a b && git add -N b`).
  The unclaimed source path was then re-read as a status line, so `handler.go` became
  XY code `ha` and the whole status read failed with `unknown index status code: h`.
  Affects `status`, `info`, `fetch`, `pull`, `push`, `diff` and `commit`, which share
  the parser. Copies (`C`) are paired the same way.
- **Intent-to-add (` A`, `git add -N`) is reported as an unstaged modification.** It was
  a silent no-op: the path raised `TrackedChangedCount` while appearing in no file list,
  so a consumer cross-checking the count against the lists saw them disagree. It is
  filed under `ModifiedFiles`, matching git's own reading — the index holds the path
  with empty content, so every byte is unstaged.
- **`gz-git status` no longer reports a repository whose working tree it could not read
  as clean.** The health check set `WorkTreeClean` on a failed `GetStatus` — promoting
  "the read failed" to "there is nothing to commit". This is the amplifier that turned
  the parse failure above into a silent wrong answer, and it predates it: a corrupt
  `.git/index` already produced `✓ All 1 repositories are healthy`, exit 0. Such a
  repository is now `HealthError`, exit 2, with the underlying git error preserved.
- **Unparseable status output is an error again rather than a dropped record.** Folding
  the two porcelain parsers into one silently relaxed the contract: the parser this
  replaced (`internal/parser.ParseStatus`) rejected a line it could not read, while the
  merged one skipped any record shorter than `XY P`. That guard could not tell the empty
  element every `-z` payload ends with — a structural artifact of NUL-terminating each
  record — from the one signal that says the output is not the format the parser assumes,
  such as a truncated read. Commands now fail with the offending record quoted instead of
  reporting a file list that is short by exactly the records they could not read. Two
  narrower cases went the same way: a status code with no path, and a rename whose source
  record is missing, which was passed through as a rename with an empty origin — a shape
  git never emits and a caller cannot distinguish from a real one.

______________________________________________________________________

`gz-git cleanup branch` reported deletions that never happened. Three defects stacked, and
a run in which git refused every single deletion still printed `✓ Deleted N branch(es)`
and exited 0:

- **`branch.Manager` no longer reports a failed git as completed work.** The same defect
  fixed in `WorktreeManager` above ran through `manager.go` and `cleanup.go`: only the
  ability to *start* git was checked, while a failed git is signalled through the exit
  code. `Create` returned success for a branch that was never created, `Create` with
  `Checkout` returned success with HEAD never moved — so the caller's next commit landed
  on the old branch — `Delete` reported `git branch -d`'s refusal to remove an unmerged
  branch as a deletion, `List` returned a repository with no branches, and `Current`
  produced an empty branch name. `Exists` is unchanged and still answers `false, nil` for
  a branch that does not exist: there the exit code *is* the answer.
- **`Create` with an unresolvable start ref returns `ErrInvalidRef`.** The guard that
  verifies the start ref could not fire, so callers matching on the sentinel saw
  `git branch`'s own wording instead.
- **`✓ Deleted N branch(es)` counts branches actually deleted.** N was
  `report.CountBranches()` — the number of *candidates* the analysis found — printed
  whether or not any deletion succeeded. Failures are now listed on stderr with git's
  reason, and a run in which any deletion failed exits `cliutil.ExitPartialFailed` (2)
  instead of 0.

API note: `branch.CleanupService.Execute` now returns `(*ExecuteResult, error)` instead of
`error`. It previously discarded every per-branch deletion error, which left no way to
tell a run that deleted everything from one that deleted nothing. The new `ExecuteResult`
carries `Deleted []string` and `Failed []DeleteFailure`. The policy is unchanged — one
branch failing does not stop the rest — but the failures are now reported rather than
dropped. A non-nil error still means the run never started.

______________________________________________________________________

`gz-git cleanup branch --gone` did nothing at all in a single repository. The flag was
accepted, no gone branches were ever reported, and the command exited 0 — which reads as
"there are none". Three independent defects each produced that outcome on their own:

- **"Gone" now means what git means by it.** The single-repository path asked whether a
  *remote-tracking* branch's remote was still registered in `git remote`. That is a
  different question from the one the flag names: git marks a **local** branch `[gone]`
  when the upstream it tracks no longer resolves, and that is what the bulk path
  (`gz-git cleanup branch <dir> --gone`) has always used. The two implementations had
  drifted onto different concepts; the single-repository one is replaced by the same
  `for-each-ref --format=%(refname:short) %(upstream:track)` read, preceded by a
  best-effort `fetch --prune` so the marker is current.
  The check it replaces could never fire regardless: it required a `remotes/` name prefix
  that `git branch -vv` parsing strips before the check ever sees it. It also failed open
  — a failed `git remote` read reported *every* branch as orphaned — which this removes.
- **`AnalyzeOptions` gained `IncludeGone`.** There was no field for the flag to reach.
  Gone detection was gated on `IncludeRemote`, so `--gone` without `--remote` did not even
  list the branches it would have judged.
- **The single-repository command passes `--gone` to the analysis.** Its `AnalyzeOptions`
  omitted the field entirely. The bulk path always passed it, which is why the flag looked
  implemented.

`--gone` deletes the local ref and never the branch on the server: a local ref is
recoverable from the reflog, a shared remote branch is not. Passing `--remote` alongside
`--gone` does not change that — gone branches are local, and the remote deletion path is
reached only for remote branches.

### Fixed

- `branch.WorktreeManager` finds worktrees whose path goes through a symlink. `Get`
  compared paths normalized with `filepath.Abs`, which is string arithmetic and never
  resolves links, against the paths `git worktree list` reports with every link already
  resolved — so on macOS, where `/var` is a symlink to `/private/var`, no worktree under
  a temp directory ever matched. `Add` was the worst of it: it created the worktree, then
  failed to describe it, and reported the whole operation as an error while leaving the
  worktree on disk. `Remove` and `Exists` reported such a worktree as absent, which left
  the CLI with no way to undo what `Add` had done.
- `branch.WorktreeManager` no longer reports failed git commands as completed work.
  Every command in the file checked only whether git could be *started*, while a failed
  git is signalled through the exit code — so `Remove` and `Prune` returned success
  without doing anything, `List` returned a repository with no worktrees, and the
  uncommitted-changes check that guards an unforced `Remove` returned "clean". The guard
  additionally discarded its own error, so a check that could not run counted as a check
  that passed; `Remove` now refuses and names `--force` as the way past it.
- The `workspace sync` preview lists repositories whose working tree it could not read.
  The block is the preview's whole account of what a sync may overwrite, and a failed
  check left the repository unlisted — among the ones that were checked and found safe.
- `gz-git doctor` no longer reports a repository it could not read as healthy. Every
  repository check signals "nothing wrong" by producing no findings, so discarding the
  error from `git status` filed an unreadable repository under the same heading as one
  that was examined and found clean — in the command whose whole purpose is telling those
  apart. Such a repository now produces a `worktree-unreadable` warning naming the git
  error, and its conflict and dirty-worktree checks are reported as skipped.
- The post-sync badge in `gz-git sync` and `gz-git workspace sync` recognizes every
  conflicted state. It tested each porcelain status letter against `U`, which covers five
  of git's seven unmerged codes; `AA` (both added) and `DD` (both deleted) contain no `U`,
  so a repository left in either state rendered as `dirty` rather than `conflict`.
- The post-sync badge no longer renders an unreadable repository as a healthy one. A
  failed `git status` was discarded, leaving `IsDirty` and `HasConflicts` at `false` —
  the exact shape of a clean, conflict-free repository, so the badge showed neither
  `dirty` nor `conflict`. Such a repository now renders as `unknown`.
- `branch.ParallelWorkflow` reports worktree changes as paths that exist. Its file list
  is compared across worktrees to find files two branches touch at once, so a value
  naming nothing on disk cannot match and the overlap it should reveal goes unreported.
  Four shapes produced such values: a rename came back as the single string
  `old.txt -> new.txt`; a path holding a space or a non-ASCII byte came back C-quoted,
  escapes included; untracked files were listed despite the name; and an untracked
  directory collapsed to one `dir/` entry, counting N new files as one path that is not
  a file. It now reads `-z` through the shared parser and drops untracked entries.
- `branch.ParallelWorkflow` no longer reports a worktree it could not read as clean.
  A failed `git status` became `HasChanges: false`, an unreadable file list became an
  empty one, a worktree that failed to build was dropped from the list without a signal,
  and a failed conflict scan became `Conflicts: 0` — four separate fallbacks that all
  land on the answer a caller acts on by doing nothing. Each is now an error.
- `gz-git commit` no longer commits repositories with unresolved merge conflicts.
  `git add -A` marks conflicts as resolved, so `<<<<<<<` markers were staged as ordinary
  content and recorded into a two-parent merge commit — after which the repository
  reported clean, making the damage effectively undetectable. Conflicted repositories are
  now reported with an `⊗` status, their unmerged paths listed regardless of `--verbose`.
- `gz-git diff --include-untracked` no longer dereferences symlinks. It read untracked
  files with `os.ReadFile`, which follows links, so a convenience symlink such as
  `ln -s ~/.aws/credentials creds` inlined the target's plaintext into diff output and
  therefore into LLM prompts and CI artifacts. Symlinks are now reported the way git
  reports them: `mode 120000` plus the link path.
- `gz-git diff --include-untracked` no longer reads a file before checking it against
  `--max-size`. A 191 MB untracked file under `--max-size 1` took RSS from 20 MB to
  1.21 GB (4.17 GB at `--parallel 3`; the default is `--parallel 10`) and then discarded
  the result. Size is now checked from `Lstat` before opening, and reads stream through
  `io.LimitReader`.
- `gz-git diff --include-untracked` is no longer a silent no-op on untracked
  directories, and no longer drops files without a signal. Every skip is now recorded in
  the new `omitted_files` key and surfaced in the human-readable formats.
- `gz-git diff` reports content and line counts for changes that are already staged.
  It ran `git diff` (worktree vs index), so a fully staged repository listed its files
  but reported empty content and zero lines.
- `git` command failures are no longer read as "no changes". `gitcmd.Executor.Run`
  signals failure through `Result.ExitCode` and returns a nil error unless the process
  could not be started, so code that checked only the error inverted a broken `status`
  into a clean repository and a broken `ls-files --unmerged` into a conflict-free one.
- `gz-git commit`'s `Total skipped` is computed from the filtered repository set and
  before the `--dry-run` early return. It previously counted repositories excluded by
  `--include`/`--exclude` as skipped, and always reported 0 under `--dry-run`.
- Untracked files ending without a trailing newline no longer gain a spurious blank
  `+` line; git's `\ No newline at end of file` marker is emitted instead.

### Added

- `gz-git commit --allow-conflicted` commits repositories that still have unmerged paths,
  for the rare case where writing conflict markers into history is intended.
- JSON output gains `tracked_files_changed`, `untracked_files_changed` and
  `staged_files_changed` on both `diff` and `commit`, so a consumer can tell which set
  each count describes rather than inferring it. `staged_files_changed` overlaps the
  other two rather than partitioning them.
- `gz-git diff --format json` gains `scope` (`head` | `staged` | `worktree`), naming the
  comparison the numbers describe, and `omitted_files` (`{path, reason}`; reason is
  `not-regular-file`, `too-large` or `read-error`).
- `gz-git commit --format json` gains `total_conflicted` and per-repository
  `conflicted_files`.
- `gz-git diff`'s default and compact formats show untracked counts. Default prints
  `4 files (+3 untracked)`; compact grows an `Untracked` column and renames `Files` to
  `Tracked`, both only when some repository has untracked files. Files omitted from the
  diff body are flagged in every human-readable format.
- `reposync.WorkTreeStatus` gains `WorkTreeUnknown` (`"unknown"`), distinct from
  `WorkTreeClean`: the question was asked and went unanswered. The empty zero value still
  means "not checked", which is what `CheckWorkTree: false` leaves behind. It renders as
  `state-unreadable` in the TUI formatter and `UNREADABLE` in `gz-git status`, and carries
  the recommendation *"Working tree state could not be read. Run 'git status' in the
  repository to see the underlying error"*. A consumer switching on `WorkTreeStatus`
  without a `default` will silently render nothing for it.

### Changed

- `checkRepositoryState` reads `git status --porcelain -z -uno` instead of `-uall`. It
  consumes only `ConflictFiles`, and a conflicted path is by definition tracked, so
  `-uall` was forcing a full recursive walk of every untracked directory to produce
  entries nothing read. No output value changes.

### Removed

- The `internal/parser` package is deleted. Nothing imported it: after the change-set
  work removed `ParseStatus`, the remaining 20 exported functions had zero callers in
  the module, and 21KB of tests were validating 8.9KB of code no command could reach.
  It is under `internal/`, so this is **not a public API change** — no importer outside
  this module could have depended on it.

  The three live `parseAheadBehind` implementations (`pkg/repository/client.go`,
  `pkg/branch/manager.go`, `pkg/doctor/repo_checks.go`) are untouched. Removing dead
  code and consolidating live parsers are separate jobs; only the first is done here.

### Internal

- New `pkg/repository/changeset.go`: `collectChangeSet` is the single change-set
  collector shared by `BulkDiff` and `BulkCommit`. The two inline porcelain parsers are
  gone; `parseDiffStats` and `extractDiffSummaryLine` are deleted.
- New `pkg/repository/bulk_diff_untracked.go` holds the read-only untracked enumeration
  and defensive file reading (`Lstat` pre-check, re-`Stat` on the open descriptor to
  close the TOCTOU window, chunked streaming).
- Golden tests for `gz-git diff`'s default, compact, json and llm formats
  (`cmd/gz-git/cmd/testdata/*.golden`, regenerate with `-update-golden`).
- New `pkg/reposync/diagnostic_worktree_test.go` covers `pkg/repository` and
  `pkg/reposync` together. It is deliberately untagged: the rename defect above was
  only ever visible at that join, and the existing cross-package coverage sits behind
  `//go:build integration`, so it never ran.

> One follow-up is tracked but not fixed here: `--format llm` emits map keys in random
> order because the formatter lives in `gzh-cli-core`. See `tasks/issue/07-*`.
> Four issues opened during review remain open: porcelain parsers outside
> `pkg/repository` (`10-*`), the parser's silent skip of malformed records (`09-*`),
> the remaining `Executor.Run` fail-open sites (`08-*`), and status-consumer test
> coverage (`11-*`).

______________________________________________________________________

## Past releases

Released versions are archived one file per release line. This file carries only
unreleased changes.

| Line                           | Releases                               |
| ------------------------------ | -------------------------------------- |
| [0.7.x](docs/changelog/0.7.md) | 0.7.0 (2026-07-02)                     |
| [0.6.x](docs/changelog/0.6.md) | 0.6.1 (2026-01-25), 0.6.0 (2026-01-21) |
| [0.4.x](docs/changelog/0.4.md) | 0.4.0 (2026-01-02)                     |
| [0.3.x](docs/changelog/0.3.md) | 0.3.1 (2025-12-02), 0.3.0 (2025-12-01) |
| [0.2.x](docs/changelog/0.2.md) | 0.2.0 (2025-12-01)                     |
| [0.1.x](docs/changelog/0.1.md) | 0.1.0-alpha (2025-12-01)               |

The mechanical per-commit list is not copied here; `.goreleaser.yaml`
(`changelog.use: github`) generates it into the GitHub Release notes.

## Links

- **Repository**: https://github.com/gizzahub/gzh-cli-gitforge
- **Documentation**: https://pkg.go.dev/github.com/gizzahub/gzh-cli-gitforge
- **Issues**: https://github.com/gizzahub/gzh-cli-gitforge/issues
- **Discussions**: https://github.com/gizzahub/gzh-cli-gitforge/discussions

______________________________________________________________________

## Acknowledgments

- Built with [Cobra](https://github.com/spf13/cobra) CLI framework
- Follows [Conventional Commits](https://www.conventionalcommits.org/) specification
- Inspired by [gzh-cli](https://github.com/gizzahub/gzh-cli)

______________________________________________________________________

[unreleased]: https://github.com/gizzahub/gzh-cli-gitforge/compare/v0.3.0...HEAD
