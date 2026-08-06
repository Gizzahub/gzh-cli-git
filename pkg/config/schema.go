// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package config

// ExampleConfig is a documented example configuration showing all available options.
const ExampleConfig = `# gz-git Configuration Reference
# ==================================
# This file serves as a reference for all available configuration options.
# You can use this structure in:
#   - Project config: .gz-git.yaml
#   - Global config:  ~/.config/gz-git/config.yaml
#   - Profile config: ~/.config/gz-git/profiles/*.yaml

# -----------------------------------------------------------------------------
# Core Settings
# -----------------------------------------------------------------------------

# Profile to use for this configuration scope
# Type: string
# Default: "default"
profile: work

# -----------------------------------------------------------------------------
# Forge Provider Settings (Source)
# -----------------------------------------------------------------------------

# Provider type (required for forge sync)
# Type: string ("gitlab" | "github" | "gitea")
provider: gitlab

# API Base URL (required for self-hosted instances)
# Type: string (url)
baseURL: https://gitlab.company.com

# Authentication Token
# Type: string (recommended: use ${ENV_VAR} syntax)
token: ${GITLAB_TOKEN}

# -----------------------------------------------------------------------------
# Clone & Network Settings
# -----------------------------------------------------------------------------

# Protocol to use for cloning
# Type: string ("ssh" | "https")
# Default: "ssh"
cloneProto: ssh

# Custom SSH port (if non-standard)
# Type: integer
sshPort: 2222

# SSH Key path preference (optional)
# Type: string (path)
sshKeyPath: ~/.ssh/id_ed25519_work

# -----------------------------------------------------------------------------
# Bulk Operation Settings
# -----------------------------------------------------------------------------

# Number of parallel operations
# Type: integer
# Default: 10
parallel: 10

# Include subgroups when syncing from GitLab
# Type: boolean
# Default: false
includeSubgroups: true

# Directory structure for subgroups
# Type: string ("flat" | "nested")
# Default: "flat"
# - flat:  group-subgroup-project
# - nested: group/subgroup/project
subgroupMode: flat

# -----------------------------------------------------------------------------
# Command-Specific Overrides
# -----------------------------------------------------------------------------

# Sync Command Settings
sync:
  # Strategy for updating existing repos
  # Type: string ("pull" | "reset" | "rebase" | "skip" | "clone" | "fetch")
  # - pull:   git pull (fast-forward merge)
  # - reset:  git fetch --all && git reset --hard @{u} (destructive!)
  # - rebase: git fetch && git rebase origin/branch (preserves local commits)
  # - skip:   skip updating existing repos
  # - clone:  remove existing directory and perform fresh clone (destructive!)
  # - fetch:  only fetch remote changes (no workspace update)
  strategy: pull

  # Max retry attempts for network operations
  # Type: integer
  maxRetries: 3

  # Operation timeout
  # Type: string (duration, e.g. "30s", "1m")
  timeout: 60s

# Branch Command Settings
branch:
  # Default branch name to expect
  # Type: string
  defaultBranch: main

  # Protected branches (prevent accidental deletion/overwrite)
  # Type: list of strings
  protectedBranches:
    - main
    - master
    - develop
    - release/*

# Fetch Command Settings
fetch:
  # Fetch all remotes, not just origin
  # Type: boolean
  allRemotes: true

  # Prune deleted remote branches
  # Type: boolean
  prune: true

# Pull Command Settings
pull:
  # Use rebase instead of merge
  # Type: boolean
  rebase: false

  # Only allow fast-forward
  # Type: boolean
  ffOnly: false

# Push Command Settings
push:
  # Automatically set upstream on push
  # Type: boolean
  setUpstream: true

  # Restrict what push (and handoff end) may write
  policy:
    # Branches that may not be pushed to at all. Trailing * matches a prefix.
    # Type: list of strings (default: none)
    protected: [main, master]

    # How force pushes are treated
    #   lease-only  --force is allowed (it uses --force-with-lease);
    #               a "+" refspec, which has no lease, is refused
    #   allow       every force push is permitted
    #   deny        no force push is permitted
    # Type: string (default: lease-only)
    forceMode: lease-only

    # What happens to a force push that would discard commits whose trailers
    # name a different device or agent than this one
    #   block   the push is refused and the commits are listed
    #   allow   the push runs, with --force-with-lease as the only guard
    # Only commits signed by handoff end can be attributed; a commit made by
    # hand elsewhere carries no trailer and is never counted as foreign.
    # Type: string (default: block)
    foreignWork: block

# Identity recorded on automated commits (handoff end)
#
# Belongs in the global config, not a project's .gz-git.yaml: that file is
# committed, and every machine that clones it would report the same device.
# GZ_GIT_DEVICE and GZ_GIT_AGENT override these.
identity:
  # This machine
  # Type: string (default: hostname)
  device: dave-office

  # The automation driving this machine, if any
  # Type: string (default: none, meaning a person is driving)
  agent: hermes-01

# -----------------------------------------------------------------------------
# Workspace Structure (Recursive)
# -----------------------------------------------------------------------------
# Define child workspaces or repositories.
# Discovery Mode controls how workspaces are found:
# - hybrid: Use defined workspaces + scan directories (default)
# - auto:   Scan directories only
# - explicit: Use defined workspaces only

discovery:
  mode: hybrid

# Workspace definitions (Map format: name -> config)
workspaces:
  # 1. Recursive Config Workspace
  backend:
    path: backend
    type: config        # Load .gz-git.yaml from this dir

  # 2. Simple Git Repo Workspace (with URL for sync)
  frontend:
    path: frontend
    type: git           # Treat as single git repo
    url: git@github.com:myorg/frontend.git
    profile: personal   # Override profile for this workspace

  # 3. Git Repo with Additional Remotes (fork workflow)
  my-fork:
    path: my-fork
    type: git
    url: git@github.com:myuser/project.git
    additionalRemotes:
      upstream: https://github.com/original/project.git
      backup: git@gitlab.com:myuser/project.git

# -----------------------------------------------------------------------------
# Metadata (Optional)
# -----------------------------------------------------------------------------
metadata:
  team: backend-team
  owner: archmagece
  description: "Main backend services"
`
