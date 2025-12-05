# Quick Start Guide

Get started with `gzh-git` in 5 minutes.

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/gizzahub/gzh-cli-git.git
cd gzh-cli-git

# Build and install
make build
sudo make install

# Verify installation
gzh-git --version
```

### Using Go

```bash
go install github.com/gizzahub/gzh-cli-git/cmd/gzh-git@latest
```

## Basic Usage

### 1. Check Repository Status

```bash
# Navigate to your Git repository
cd /path/to/your/repo

# Check status
gzh-git status
```

**Output:**

```
Repository Status
=================

Branch: main
Upstream: origin/main
Status: Clean

Modified Files: 3
  - src/main.go (10 additions, 2 deletions)
  - README.md (5 additions, 0 deletions)
  - go.mod (1 additions, 0 deletions)

Untracked Files: 1
  - docs/new-feature.md
```

### 2. Auto-Generate Commits

Instead of manually writing commit messages, let `gzh-git` generate them:

```bash
# Stage your changes
git add src/main.go

# Auto-generate and commit
gzh-git commit auto
```

**What happens:**

1. Analyzes your staged changes
1. Generates a commit message following best practices
1. Validates the message
1. Creates the commit

**Example generated message:**

```
feat(main): add configuration validation

Add validation for config file parsing with proper error handling.
Ensures config values are within expected ranges.
```

### 3. Create and Manage Branches

```bash
# Create a new feature branch
gzh-git branch create feature/user-authentication

# List all branches
gzh-git branch list --all

# Delete old branch
gzh-git branch delete feature/old-implementation
```

### 4. Analyze Repository History

```bash
# Show commit statistics
gzh-git history stats --since "1 month ago"

# List top contributors
gzh-git history contributors --top 5

# View file history
gzh-git history file src/main.go
```

### 5. Safe Merging with Conflict Detection

```bash
# Check for conflicts before merging
gzh-git merge detect feature/new-feature main

# If no conflicts, merge
gzh-git merge do feature/new-feature

# If there are conflicts, you'll see:
⚠️  Found 2 potential conflicts:

  content: src/config.go
  content: README.md

Difficulty: medium
Auto-resolvable: 0/2
```

## Common Workflows

### Feature Development Workflow

```bash
# 1. Create feature branch
gzh-git branch create feature/add-caching

# 2. Make changes and commit
git add .
gzh-git commit auto

# 3. Check for merge conflicts
gzh-git merge detect feature/add-caching main

# 4. Merge when ready
gzh-git merge do feature/add-caching --squash
```

### Code Review Workflow

```bash
# 1. Check branch history
gzh-git history stats --branch feature/new-api

# 2. View contributor activity
gzh-git history contributors --since "1 week ago"

# 3. Analyze file changes
gzh-git history file src/api/handler.go

# 4. Review specific lines
gzh-git history blame src/api/handler.go
```

### Hotfix Workflow

```bash
# 1. Create hotfix branch from production
gzh-git branch create hotfix/critical-bug --base production

# 2. Fix and commit
git add .
gzh-git commit auto --template conventional

# 3. Validate commit message
gzh-git commit validate "fix(auth): prevent null pointer dereference"

# 4. Fast-forward merge to production
gzh-git merge do hotfix/critical-bug --ff-only
```

## Tips and Best Practices

### 1. Use Templates for Consistency

```bash
# List available templates
gzh-git commit template list

# View template details
gzh-git commit template show conventional

# Use specific template
gzh-git commit auto --template conventional
```

### 2. Prevent Merge Conflicts

Always check for conflicts before merging:

```bash
gzh-git merge detect <your-branch> <target-branch>
```

### 3. Track File History

When debugging, check file history and blame:

```bash
# See all changes to a file
gzh-git history file --follow src/problematic.go

# Find who wrote specific lines
gzh-git history blame src/problematic.go
```

### 4. Work in Parallel with Worktrees

Create linked worktrees for parallel development:

```bash
# Create branch with worktree
gzh-git branch create feature/parallel --worktree ../parallel-work

# Now you can work in both directories simultaneously
cd ../parallel-work  # Main repo stays on current branch
```

### 5. Export Data for Analysis

Use JSON output for custom analysis:

```bash
# Export statistics
gzh-git history stats --format json > stats.json

# Export contributors
gzh-git history contributors --format json > contributors.json

# Process with jq
cat stats.json | jq '.total_commits'
```

## Troubleshooting

### "Not a git repository"

```bash
# Make sure you're in a Git repository
git status

# Or initialize a new one
git init
```

### "Unknown template"

```bash
# List available templates
gzh-git commit template list

# Use a valid template name
gzh-git commit auto --template conventional
```

### "Merge conflicts detected"

```bash
# Abort the merge if needed
gzh-git merge abort

# Or resolve conflicts manually and continue
git add <resolved-files>
git commit
```

## Next Steps

- Read the [Command Reference](commands/README.md) for detailed command documentation
- Learn about [Library Integration](LIBRARY.md) to use gzh-git as a Go library
- Check [Troubleshooting Guide](TROUBLESHOOTING.md) for common issues
- See [Examples](examples/) for real-world usage patterns

## Getting Help

```bash
# Get help for any command
gzh-git --help
gzh-git commit --help
gzh-git merge do --help

# Check version
gzh-git --version
```

For more help:

- GitHub Issues: https://github.com/gizzahub/gzh-cli-git/issues
- Documentation: https://github.com/gizzahub/gzh-cli-git/tree/main/docs
