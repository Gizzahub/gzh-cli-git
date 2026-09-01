# Controller-config integration

`integrate check`, `integrate run`, and `integrate queue` normally use only
the repository-root declaration. A devbox/controller config is opt-in:

```sh
gz-git integrate check --controller-config /path/to/devbox/.gz-git.yaml
gz-git integrate run --controller-config /path/to/devbox/.gz-git.yaml
gz-git integrate queue --controller-config /path/to/devbox/.gz-git.yaml
```

The file is never searched for in parent directories. It must contain exactly
one `workspaces` entry whose `url` canonically equals the selected repository
remote. That entry must declare exactly one `branch.integrationBranch`; its
`branch.taskPattern` is the only reclaim policy used by this mode. The file's
resolved path and content digest are rechecked before push and reclaim.

`integrate queue` uses the same explicit binding without searching for a
controller. Its comparison base is always the selected controller remote plus
the declared integration branch. `--base` is accepted only when it normalizes
to that same branch (for example `origin/develop` or `develop`). When
`branch.taskPattern` is non-empty, queue lists only matching task refs; an
empty pattern deliberately leaves the queue unfiltered. Controller resolution
errors remain exit 2 even with `--quiet`.

The only preparation profile is the closed `familybook-ent-v1` value:

```yaml
workspaces:
  engine:
    url: https://example.invalid/familybook-engine.git
    branch:
      integrationBranch: develop
      taskPattern: dev/*
    integration:
      prepareProfile: familybook-ent-v1
```

It runs `go generate ./ent` in isolated detached source and target worktrees
with isolated Go caches before the existing Make gates. Generated output must
be untracked and below `ent/generated`; any tracked mutation or other output
fails the check. Each preparation receives a fixed five-minute budget so a
cold isolated module cache can be populated. No shell command or timeout is
configurable.

This is not a sandbox: as with the legacy Make gate, the selected task branch
is trusted to execute repository-owned code. The command receives an isolated
HOME/XDG/Git configuration and no inherited credential or SSH-agent variables;
the target worktree is measured and removed before source code is created.
