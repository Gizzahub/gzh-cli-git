# gzh-git Command Reference

Complete reference for all `gzh-git` commands.

## Quick Navigation

- [Repository Commands](#repository-commands) - Initialize and manage repositories
- [Commit Commands](#commit-commands) - Automate commit operations
- [Branch Commands](#branch-commands) - Branch and worktree management
- [History Commands](#history-commands) - Analyze repository history
- [Merge Commands](#merge-commands) - Merge and rebase operations

## Global Flags

All commands support these global flags:

| Flag | Short | Description |
|------|-------|-------------|
| `--quiet` | `-q` | Suppress non-error output |
| `--verbose` | `-v` | Show detailed output |
| `--help` | `-h` | Show command help |

## Repository Commands

### status

Show repository status with detailed change information.

```bash
gzh-git status [flags]
```

**Flags:**

- `--porcelain`: Machine-readable output

**Examples:**

```bash
# Show status
gzh-git status

# Machine-readable format
gzh-git status --porcelain
```

### clone

Clone a repository with advanced options.

```bash
gzh-git clone <url> [directory] [flags]
```

**Flags:**

- `--branch <name>`: Clone specific branch
- `--depth <n>`: Create shallow clone
- `--single-branch`: Clone only one branch

**Examples:**

```bash
# Clone repository
gzh-git clone https://github.com/user/repo.git

# Clone specific branch
gzh-git clone --branch develop https://github.com/user/repo.git

# Shallow clone
gzh-git clone --depth 1 https://github.com/user/repo.git
```

### info

Display repository information.

```bash
gzh-git info [flags]
```

**Examples:**

```bash
# Show repository info
gzh-git info
```

## Commit Commands

### commit auto

Automatically generate and create commits from staged changes.

```bash
gzh-git commit auto [flags]
```

**Flags:**

- `--template <name>`: Use specific commit template
- `--dry-run`: Show generated message without committing

**Examples:**

```bash
# Auto-generate and commit
gzh-git commit auto

# Use specific template
gzh-git commit auto --template conventional

# Preview without committing
gzh-git commit auto --dry-run
```

**How it works:**

1. Analyzes staged changes
1. Generates commit message using template
1. Validates message against rules
1. Creates commit if validation passes

### commit validate

Validate commit messages against templates.

```bash
gzh-git commit validate <message> [flags]
```

**Flags:**

- `--template <name>`: Validate against specific template

**Examples:**

```bash
# Validate message
gzh-git commit validate "feat: add new feature"

# Validate with template
gzh-git commit validate "feat(api): add endpoint" --template conventional
```

### commit template

Manage commit message templates.

```bash
gzh-git commit template <command> [args]
```

**Subcommands:**

- `list`: List available templates
- `show <name>`: Show template details
- `validate <file>`: Validate custom template file

**Examples:**

```bash
# List templates
gzh-git commit template list

# Show template
gzh-git commit template show conventional

# Validate custom template
gzh-git commit template validate my-template.yaml
```

## Branch Commands

### branch list

List local and remote branches.

```bash
gzh-git branch list [flags]
```

**Flags:**

- `--all`, `-a`: Show both local and remote branches
- `--remote`, `-r`: Show only remote branches
- `--merged`: Show only merged branches
- `--no-merged`: Show only unmerged branches

**Examples:**

```bash
# List local branches
gzh-git branch list

# List all branches
gzh-git branch list --all

# Show merged branches
gzh-git branch list --merged
```

### branch create

Create a new branch with optional worktree.

```bash
gzh-git branch create <name> [flags]
```

**Flags:**

- `--base <ref>`: Create from specific ref (default: HEAD)
- `--track`: Set up tracking
- `--worktree <path>`: Create linked worktree

**Examples:**

```bash
# Create branch
gzh-git branch create feature/new-feature

# Create from specific commit
gzh-git branch create hotfix/bug --base abc123

# Create with worktree
gzh-git branch create feature/parallel --worktree ../parallel-work
```

### branch delete

Delete local or remote branches.

```bash
gzh-git branch delete <name> [flags]
```

**Flags:**

- `--force`, `-f`: Force delete even if not merged
- `--remote`, `-r`: Delete remote branch

**Examples:**

```bash
# Delete branch
gzh-git branch delete feature/old

# Force delete
gzh-git branch delete experimental --force

# Delete remote branch
gzh-git branch delete feature/done --remote
```

## History Commands

### history stats

Show commit statistics and trends.

```bash
gzh-git history stats [flags]
```

**Flags:**

- `--since <date>`: Start date (e.g., '2024-01-01', '1 month ago')
- `--until <date>`: End date
- `--branch <name>`: Specific branch
- `--author <name>`: Filter by author
- `--format <type>`: Output format (table|json|csv|markdown)

**Examples:**

```bash
# Overall statistics
gzh-git history stats

# Last month
gzh-git history stats --since "1 month ago"

# Export as JSON
gzh-git history stats --format json > stats.json
```

### history contributors

Analyze repository contributors.

```bash
gzh-git history contributors [flags]
```

**Flags:**

- `--top <n>`: Show only top N contributors
- `--since <date>`: Start date
- `--until <date>`: End date
- `--min-commits <n>`: Minimum commits threshold
- `--sort <field>`: Sort by (commits|additions|deletions|recent)
- `--format <type>`: Output format

**Examples:**

```bash
# List all contributors
gzh-git history contributors

# Top 10 contributors
gzh-git history contributors --top 10

# Contributors with at least 5 commits
gzh-git history contributors --min-commits 5
```

### history file

Show file change history.

```bash
gzh-git history file <path> [flags]
```

**Flags:**

- `--since <date>`: Start date
- `--until <date>`: End date
- `--max <n>`: Maximum number of commits
- `--follow`: Follow file renames
- `--author <name>`: Filter by author
- `--format <type>`: Output format

**Examples:**

```bash
# Show file history
gzh-git history file src/main.go

# Follow renames
gzh-git history file --follow src/main.go

# Limit to 10 commits
gzh-git history file --max 10 README.md
```

### history blame

Show line-by-line authorship.

```bash
gzh-git history blame <file> [flags]
```

**Examples:**

```bash
# Show blame
gzh-git history blame src/main.go
```

## Merge Commands

### merge do

Execute a merge operation.

```bash
gzh-git merge do <source-branch> [flags]
```

**Flags:**

- `--strategy <type>`: Merge strategy (auto|ours|theirs|recursive)
- `--ff-only`: Only allow fast-forward merge
- `--no-commit`: Merge but don't commit
- `--squash`: Squash all commits into one
- `--message <text>`: Custom merge commit message

**Examples:**

```bash
# Merge branch
gzh-git merge do feature/new-feature

# Fast-forward only
gzh-git merge do feature/new-feature --ff-only

# Squash merge
gzh-git merge do feature/new-feature --squash

# Custom message
gzh-git merge do feature/new-feature --message "Merge feature X"
```

**Merge Strategies:**

- `auto` (default): Let Git choose the best strategy
- `recursive`: Standard three-way merge
- `ours`: Keep our version in conflicts
- `theirs`: Keep their version in conflicts

### merge detect

Detect potential merge conflicts before merging.

```bash
gzh-git merge detect <source> <target> [flags]
```

**Flags:**

- `--include-binary`: Include binary file conflicts
- `--base <commit>`: Specify base commit

**Examples:**

```bash
# Detect conflicts
gzh-git merge detect feature/new-feature main

# Include binary files
gzh-git merge detect feature/new-feature main --include-binary
```

### merge abort

Abort an in-progress merge.

```bash
gzh-git merge abort
```

**Examples:**

```bash
# Abort merge
gzh-git merge abort
```

### merge rebase

Rebase current branch onto another.

```bash
gzh-git merge rebase [branch] [flags]
```

**Flags:**

- `--onto <commit>`: Rebase onto specific commit
- `--continue`: Continue after resolving conflicts
- `--skip`: Skip current commit
- `--abort`: Abort rebase

**Examples:**

```bash
# Rebase onto main
gzh-git merge rebase main

# Continue after conflicts
gzh-git merge rebase --continue

# Abort rebase
gzh-git merge rebase --abort
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Invalid arguments |
| 3 | Not a git repository |
| 4 | Operation failed (e.g., conflicts) |

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `GZH_GIT_TEMPLATE` | Default commit template | `conventional` |
| `GZH_GIT_EDITOR` | Editor for interactive operations | `$EDITOR` or `vim` |

## See Also

- [Installation Guide](../INSTALL.md)
- [Quick Start](../QUICKSTART.md)
- [Troubleshooting](../TROUBLESHOOTING.md)
- [Library Integration](../LIBRARY.md)
