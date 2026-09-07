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

### Added

- `gz-git observe` reports the D6 four-state `.gz-git-context.yaml` matrix
  and aggregates CE v2 `ce.task.gate-doctor/v2` (origin tag `v0.8.3` /
  `ac744597`) without an apply surface. Probe:
  `gz-git capability context-reference-observe-v1`.
- `gz-git capability integrate-readiness-v1` now provides a fail-closed machine
  probe for wrappers and hooks that must reject clients which would ignore the
  target-owned readiness contract.
- `integrate check` can now use a target-owned `branch.readiness` V1 contract
  instead of executing a task branch's Makefile. The contract binds an executable
  runner tree to both checked commits, runs it in detached worktrees with bounded
  output and time, and makes `integrate run` revalidate the target SHA and push it
  with an exact lease before reclaiming anything.
- `gz-git cleanup branch --non-canonical` retires trunk branches that duplicate the
  canonical branch a repository declares in `.gz-git.yaml`. It refuses to act on an
  undeclared repository, scopes remote deletion to the declared remote, and re-measures
  the canonical tip at delete time rather than trusting a cached one — the flag exists
  for repositories carrying a second trunk nobody deletes because deleting it by hand
  is unnerving, so being wrong once is worse than not shipping.
- The `--non-canonical` preview says what it is proposing and what it turned down.
  Each candidate carries the ref that authorized it (`→ contained in <ref> @ <sha>`),
  which since the local→remote fallback can be another machine's branch as of the last
  fetch; each trunk the gate examined and declined is reported with its reason on
  stderr, ungated by `--quiet`. "Nothing to clean up" and "checked, and refused" are
  different facts for an operator deciding whether to pass `--force`.

______________________________________________________________________

## Source-version milestones

Earlier 0.x files record source-version milestones; the current remote has no matching
stable tags or GitHub Releases. v0.8.0 is intended to be the first stable published tag.
A row marked `publication pending` is release preparation, not evidence that a tag or
artifact exists. This file carries only unreleased changes.

| Line                           | Milestones                                       |
| ------------------------------ | ------------------------------------------------ |
| [0.8.x](docs/changelog/0.8.md) | 0.8.0 (prepared 2026-08-25; publication pending) |
| [0.7.x](docs/changelog/0.7.md) | 0.7.0 (2026-07-02)                               |
| [0.6.x](docs/changelog/0.6.md) | 0.6.1 (2026-01-25), 0.6.0 (2026-01-21)           |
| [0.4.x](docs/changelog/0.4.md) | 0.4.0 (2026-01-02)                               |
| [0.3.x](docs/changelog/0.3.md) | 0.3.1 (2025-12-02), 0.3.0 (2025-12-01)           |
| [0.2.x](docs/changelog/0.2.md) | 0.2.0 (2025-12-01)                               |
| [0.1.x](docs/changelog/0.1.md) | 0.1.0-alpha (2025-12-01)                         |

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
