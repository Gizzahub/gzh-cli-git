# ISSUE: Forge event stream / hosting timeline as gz-git product surface

- status: todo
- priority: P3
- category: product / forge
- created_at: 2026-08-07
- transferred_from: gzh-cli issue 26 residual (Provider `ListEvents` / `StreamEvents`)
- owning_product: **gz-git** (gzh-cli-gitforge)

## Background

gzh-cli (`gz git`) already implements:

- Repo mutation CLI surface: create/delete/archive/search/update/fork
- Webhook CRUD on providers
- **Process-local** in-memory storage for `gz git event list|get|metrics` after
  `event server` receives webhooks

What was left on gzh-cli issue 26 was **Provider-level** remote timeline APIs:

- `ListEvents` / `GetEvent` / `ProcessEvent` / `RegisterEventHandler` / `StreamEvents`

Those are **not** the same as the CLI memory store. They pull event streams from
GitHub/GitLab/Gitea hosting APIs.

Product direction: **git forge sync / advanced forge surfaces live in gz-git**,
not expand further on gzh-cli. gzh-cli 26 is closed for the CLI mutation surface;
any remote event-stream product work continues here.

## Why not implement on gzh-cli

- gz-git is the SSoT for forge/workspace operations in this monorepo family.
- gzh-cli `GitProvider.Event*` methods are unused by current gz-git codepaths.
- Duplicating remote event streaming on both CLIs increases maintenance without
  a shared product surface.

## Scope (when product prioritizes)

1. Decide UX: `gz-git events …` (or equivalent) vs bulk/LLM pipeline input.
2. Reuse or extract provider event listing from forge APIs (prefer gitforge
   `pkg/github|gitlab|gitea` clients, not gzh-cli `pkg/git/provider` stubs).
3. Persistence policy (none / file / sqlite) if events must outlive a process.
4. Do **not** reintroduce fabricated demo data (see gzh-cli issue 44 history).

## Out of scope

- gzh-cli `gz git event` memory storage (already honest; leave as-is or deprecate later)
- Package-manager cleanup (issue 25 family)

## Acceptance (future)

- [ ] Product decision recorded (command shape + persistence)
- [ ] Implementation in this repo with tests (httptest / fake API)
- [ ] Docs under `docs/usage/`
- [ ] No silent fake rows on empty storage

## References

- gzh-cli issue 26 resolution notes
- gzh-cli `cmd/git/event` (memory storage — different layer)
- This repo: workspace/forge commands under `cmd/gz-git`
