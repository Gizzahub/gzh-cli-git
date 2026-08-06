# Frequently Asked Questions (FAQ)

## What is gzh-cli-gitforge?

`gzh-cli-gitforge` is a dual-purpose project:

1. **CLI (`gz-git`)**: bulk-first Git workflows across many repositories
1. **Go library**: reusable packages under `pkg/` for integrating Git operations and repo sync into Go programs

## Does gz-git replace git?

No. `gz-git` runs the **Git CLI** under the hood and focuses on safer defaults, better bulk workflows, and structured output.

## What does “bulk-first” mean?

Most commands scan a directory for repositories and process them in parallel.

- Default scan depth: `--scan-depth 1`
- Default parallelism: `--parallel 10`

Control scope with:

```bash
gz-git status -d 2 ~/projects
gz-git fetch --include "gzh-cli.*" --exclude ".*-deprecated" ~/projects
```

## How do I install gz-git?

```bash
go install github.com/gizzahub/gzh-cli-gitforge/cmd/gz-git@latest
gz-git --version
```

If your shell can’t find `gz-git`, ensure `$(go env GOPATH)/bin` is on `PATH`.

## How do I clone repositories with gz-git?

`clone` is bulk-oriented: pass URLs via `--url` (repeatable) or `--file`.

```bash
gz-git clone --url https://github.com/user/repo.git
gz-git clone ~/projects --file repos.txt
gz-git clone --update --file repos.txt
```

## Why is there no `branch create/delete` command?

Basic branch creation/deletion is intentionally left to native `git`.
`gz-git` currently focuses on:

- `gz-git branch list` (bulk)
- `gz-git branch name` (build a task's branch name; prints it, creates nothing)
- `gz-git cleanup branch` (merged/stale/gone cleanup; dry-run by default)
- `gz-git switch` (bulk branch switching, `--create` to create)

`branch name` is the one addition that is not creation: it works out the branch
name a task should have on this device or under this agent, which plain `git`
cannot, and leaves creating it to `switch --create`.

## Where can I see all flags for a command?

```bash
gz-git --help
gz-git <command> --help
```

For curated workflows and examples: `docs/commands/README.md`.

## How do I sync all repos from GitHub/GitLab/Gitea?

Use `forge from`:

```bash
gz-git forge from --provider github --org myorg --path ./repos --token $GITHUB_TOKEN
```

Or generate a workspace config and sync via `workspace`:

```bash
gz-git forge config generate --provider github --org myorg -o .gz-git.yaml
gz-git workspace sync
```

## How do I use it as a Go library?

See `docs/user/getting-started/library-usage.md` and the GoDoc:

- https://pkg.go.dev/github.com/gizzahub/gzh-cli-gitforge
