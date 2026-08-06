# Configuration Guide

Detailed configuration examples for gz-git profiles, workspaces, and hierarchical configs.

______________________________________________________________________

## Config File Locations

```
~/.config/gz-git/
├── config.yaml              # Global config
├── profiles/
│   ├── default.yaml        # Default profile
│   ├── work.yaml           # User profiles
│   └── personal.yaml
└── state/
    └── active-profile.txt  # Currently active profile

# Project config (auto-detected)
~/myproject/.gz-git.yaml
```

______________________________________________________________________

## Profile Management Commands

```bash
# Initialize config directory
gz-git config init

# Create profile (interactive)
gz-git config profile create work

# Create profile (with flags)
gz-git config profile create work \
  --provider gitlab \
  --base-url https://gitlab.company.com \
  --token ${WORK_TOKEN} \
  --clone-proto ssh \
  --ssh-port 2224

# List profiles
gz-git config profile list

# Set active profile
gz-git config profile use work

# Show profile details
gz-git config profile show work

# Delete profile
gz-git config profile delete work

# Show project config (default)
gz-git config show

# Show effective config with precedence sources
gz-git config show --effective

# Show config hierarchy tree
gz-git config hierarchy
```

______________________________________________________________________

## Profile Example (work.yaml)

```yaml
name: work
provider: gitlab
baseURL: https://gitlab.company.com
token: ${WORK_GITLAB_TOKEN}  # Environment variable
cloneProto: ssh
sshPort: 2224
parallel: 10
includeSubgroups: true
subgroupMode: flat

# Command-specific overrides
sync:
  strategy: reset
  maxRetries: 3
```

______________________________________________________________________

## Project Config Example (.gz-git.yaml)

```yaml
profile: work  # Use work profile for this project

# Project-specific overrides
sync:
  strategy: pull  # Override profile's reset
  parallel: 3     # Lower parallelism
branch:
  defaultBranch: main
  protectedBranches: [main, develop]  # protected from deletion

push:
  policy:
    protected: [main]      # refuse to push to these at all
    forceMode: lease-only  # lease-only | allow | deny

metadata:
  team: backend
  repository: https://gitlab.company.com/backend/myproject
```

______________________________________________________________________

## Usage Example

```bash
# One-time setup
gz-git config profile create work \
  --provider gitlab \
  --base-url https://gitlab.company.com \
  --token ${WORK_TOKEN}
gz-git config profile use work

# Now all commands use work profile automatically
gz-git sync from-forge --org backend  # Uses gitlab, token, etc.
gz-git status                         # Uses work profile defaults

# Switch context
gz-git config profile use personal
gz-git sync from-forge --org my-projects  # Now uses personal profile

# One-off override
gz-git sync from-forge --profile work --org backend  # Temporarily use work
```

______________________________________________________________________

## Hierarchical Configuration

### Unified Config Structure

All levels use the same `.gz-git.yaml` format:

```yaml
# ~/.gz-git.yaml (workstation level)
parallel: 10
cloneProto: ssh

workspaces:
  mydevbox:
    path: ~/mydevbox
    type: config              # Has config file (recursive)
    profile: opensource
    parallel: 10

  mywork:
    path: ~/mywork
    type: config
    profile: work

  single-repo:
    path: ~/single-repo
    type: git                 # Plain git repo (no config)
```

```yaml
# ~/mydevbox/.gz-git.yaml (workspace level - same structure!)
profile: opensource

sync:
  strategy: reset  # reset, pull, rebase, fetch, skip, clone
  parallel: 10

workspaces:
  gzh-cli:
    path: gzh-cli
    type: git               # Plain repo

  gzh-cli-gitforge:
    path: gzh-cli-gitforge
    type: config           # Has config file
    sync:
      strategy: pull       # Inline override
```

### Workspace Types

- **`type: config`**: Directory with config file (enables recursive nesting)
- **`type: git`**: Plain Git repository (leaf node, no config)

### Discovery Modes

```bash
# Explicit: Only use workspaces defined in config
gz-git status --discovery-mode explicit

# Auto: Scan directories, ignore explicit workspaces
gz-git status --discovery-mode auto

# Hybrid: Use workspaces if defined, otherwise scan (DEFAULT)
gz-git status --discovery-mode hybrid
```

### API (pkg/config)

```go
// Load config recursively
config, err := config.LoadConfigRecursive(
    "/home/user/mydevbox",
    ".gz-git.yaml",
)

// Apply discovery mode
err = config.LoadWorkspaces(
    "/home/user/mydevbox",
    config,
    config.HybridMode,
)

// Find nearest config file
configDir, err := config.FindConfigRecursive(
    "/home/user/mydevbox/project",
    ".gz-git.yaml",
)
```

______________________________________________________________________

## Config Systems: Two Formats

gz-git supports two config formats (both work with `workspace sync`):

### Simple Format (`repositories` array)

```yaml
# .gz-git.yaml - Simple format
strategy: pull
parallel: 4
repositories:
  # name omitted - auto-extracted from URL (proxynd-core)
  - url: ssh://git@gitlab.polypia.net:2224/scripton-open/proxynd/proxynd-core.git
    branch: develop
  # name specified - custom directory name
  - name: enterprise
    url: ssh://git@gitlab.polypia.net:2224/scripton-open/proxynd/proxynd-enterprise.git
    branch: develop
  # path for subdirectory clone
  - url: https://github.com/discourse/discourse.git
    path: subdir/discourse
```

| Field    | Required | Description                        |
| -------- | -------- | ---------------------------------- |
| `url`    | Yes      | Git clone URL (HTTPS, SSH, git@)   |
| `name`   | No       | Directory name (auto from URL)     |
| `path`   | No       | Target path (defaults to name)     |
| `branch` | No       | Branch to checkout                 |

### Hierarchical Format (`workspaces` map)

```yaml
# .gz-git.yaml - Hierarchical format
parallel: 10
cloneProto: ssh

profiles:
  polypia:
    provider: gitlab
    baseURL: https://gitlab.polypia.net
    token: ${GITLAB_TOKEN}
    sshPort: 2224

workspaces:
  mydevbox:
    path: ~/mydevbox
    profile: polypia
    source:
      provider: gitlab
      org: devbox
      includeSubgroups: true
    sync:
      strategy: pull
```

### Format Selection

| Scenario                    | Recommended Format         |
| --------------------------- | -------------------------- |
| Simple repo list            | Simple (`repositories`)    |
| Forge org sync              | Hierarchical (`workspaces`)|
| Multiple profiles           | Hierarchical (`workspaces`)|
| Quick setup                 | Simple (`repositories`)    |

### Child Config Generation Mode

```yaml
childConfigMode: repositories  # Default - flat array format
# childConfigMode: workspaces  # Map-based format for nested management
# childConfigMode: none        # Directory only, no config generation
```

### Format Detection (Content-Based)

1. **Explicit `kind:` field** (highest priority)
2. **Content inspection**: `workspaces`/`profiles` → hierarchical; `repositories` → simple
3. **Default**: Falls back to `repositories` format

______________________________________________________________________

## Push Policy

`push.policy` restricts what `push` and `handoff end` may write. It is distinct
from `branch.protectedBranches`, which guards deletion.

```yaml
push:
  policy:
    protected: [main, master, "release/*"]
    forceMode: lease-only
```

| Key | Meaning |
| --- | ------- |
| `protected` | Branch names and trailing-`*` patterns that may not be pushed to. Empty by default. The **destination** decides, so `--refspec develop:main` is refused. |
| `forceMode` | `lease-only` (default) allows `--force`, which pushes with `--force-with-lease`, and refuses a `+` refspec, which has no lease. `allow` permits both. `deny` permits neither. |

`--force-mode` overrides `forceMode` for one invocation. A refused repository is
reported as `blocked`, the rest of the batch still runs, and the command exits
non-zero.

`lease-only` applies even with no config file: without it a `+` refspec would be
a silent way around `--force-with-lease`.

______________________________________________________________________

## Security Notes

- Profile files: 0600 permissions (user read/write only)
- Config directory: 0700 permissions (user access only)
- Use environment variables for tokens: `token: ${GITLAB_TOKEN}`
- No shell command execution (only `${VAR}` expansion)

______________________________________________________________________

**Last Updated**: 2026-01-23
