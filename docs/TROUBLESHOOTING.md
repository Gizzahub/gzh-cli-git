# Troubleshooting Guide

Common issues and solutions for `gzh-git`.

## Installation Issues

### Command Not Found

**Problem:** `gzh-git: command not found`

**Solutions:**

```bash
# Check if binary exists
which gzh-git

# Check Go bin directory
ls -la $HOME/go/bin

# Add to PATH
echo 'export PATH=$PATH:$HOME/go/bin' >> ~/.bashrc
source ~/.bashrc

# Or for Zsh
echo 'export PATH=$PATH:$HOME/go/bin' >> ~/.zshrc
source ~/.zshrc
```

### Permission Denied During Install

**Problem:** `permission denied` when running `make install`

**Solutions:**

```bash
# Use sudo
sudo make install

# Or install to user directory
make install PREFIX=$HOME/.local

# Add to PATH if using PREFIX
export PATH=$PATH:$HOME/.local/bin
```

### Build Failures

**Problem:** Build errors with `make build`

**Solutions:**

```bash
# Check Go version (must be 1.21+)
go version

# Clean build cache
go clean -cache
go clean -modcache

# Download dependencies
go mod download
go mod tidy

# Rebuild
make clean
make build
```

## Repository Issues

### Not a Git Repository

**Problem:** `Error: not a git repository`

**Solutions:**

```bash
# Verify you're in a Git repository
git status

# Initialize if needed
git init

# Or navigate to repository
cd /path/to/your/repo
```

### Unable to Open Repository

**Problem:** `failed to open repository`

**Solutions:**

```bash
# Check repository validity
git fsck

# Check file permissions
ls -la .git/

# Repair if needed
git gc
git prune
```

## Commit Issues

### Auto-Commit Fails

**Problem:** `gzh-git commit auto` doesn't create commit

**Solutions:**

```bash
# Check if there are staged changes
git status

# Stage changes first
git add <files>

# Then auto-commit
gzh-git commit auto

# Or use dry-run to see what would happen
gzh-git commit auto --dry-run
```

### Validation Errors

**Problem:** Commit message validation fails

**Solutions:**

```bash
# Check template requirements
gzh-git commit template show conventional

# Validate message format
gzh-git commit validate "your message"

# Common issues:
# - Missing type: "feat:", "fix:", etc.
# - Missing scope: "feat(api):"
# - Subject too long (>72 chars)
# - Body lines too long (>100 chars)
```

### Template Not Found

**Problem:** `unknown template: my-template`

**Solutions:**

```bash
# List available templates
gzh-git commit template list

# Check template location
ls ~/.config/gzh-git/templates/

# Use correct template name
gzh-git commit auto --template conventional
```

## Branch Issues

### Cannot Create Branch

**Problem:** Branch creation fails

**Solutions:**

```bash
# Check if branch already exists
git branch -a | grep <name>

# Use force flag if needed
gzh-git branch delete <name> --force
gzh-git branch create <name>

# Check base reference
git show <base-ref>
```

### Worktree Creation Fails

**Problem:** Cannot create worktree

**Solutions:**

```bash
# Check if path already exists
ls -la /path/to/worktree

# Remove if necessary
rm -rf /path/to/worktree

# Ensure parent directory exists
mkdir -p /parent/directory

# Create with absolute path
gzh-git branch create feature --worktree /absolute/path
```

### Cannot Delete Branch

**Problem:** Branch deletion fails

**Solutions:**

```bash
# Check if branch is checked out
git branch --show-current

# Switch to different branch first
git checkout main

# Force delete if needed
gzh-git branch delete <name> --force

# Delete remote branch
gzh-git branch delete <name> --remote
```

## History Issues

### No History Returned

**Problem:** `gzh-git history stats` returns empty

**Solutions:**

```bash
# Check if repository has commits
git log

# Check date range
gzh-git history stats --since "1 year ago"

# Check branch exists
git branch -a | grep <branch-name>
```

### File History Not Found

**Problem:** `file not found in repository history`

**Solutions:**

```bash
# Check file exists
ls -la <file-path>

# Check if file was ever committed
git log --all -- <file-path>

# Use --follow for renames
gzh-git history file --follow <file-path>

# Check correct path
git ls-files | grep <filename>
```

## Merge Issues

### Merge Conflicts

**Problem:** Merge results in conflicts

**Solutions:**

```bash
# Check conflicts first
gzh-git merge detect <source> <target>

# If conflicts exist, resolve them
git status
# Edit conflicted files
git add <resolved-files>
git commit

# Or abort merge
gzh-git merge abort
```

### Fast-Forward Fails

**Problem:** `--ff-only` fails

**Solutions:**

```bash
# Check if fast-forward is possible
gzh-git merge detect <source> <target>

# Use regular merge instead
gzh-git merge do <source>

# Or rebase first
gzh-git merge rebase <target>
```

### Rebase Conflicts

**Problem:** Rebase stops with conflicts

**Solutions:**

```bash
# Resolve conflicts
git status
# Edit conflicted files
git add <resolved-files>

# Continue rebase
gzh-git merge rebase --continue

# Or skip commit
gzh-git merge rebase --skip

# Or abort
gzh-git merge rebase --abort
```

## Performance Issues

### Slow Operations

**Problem:** Commands take too long

**Solutions:**

```bash
# For large repositories, use depth limit
gzh-git history stats --max-count 1000

# Use specific date range
gzh-git history stats --since "1 month ago"

# For file history, limit commits
gzh-git history file --max 100 <file>

# Optimize repository
git gc --aggressive
git prune
```

### High Memory Usage

**Problem:** High memory consumption

**Solutions:**

```bash
# Use smaller date ranges
gzh-git history stats --since "3 months ago"

# Limit output with --top
gzh-git history contributors --top 10

# Use streaming output formats
gzh-git history stats --format json | jq .
```

## Output Issues

### Garbled Output

**Problem:** Output contains strange characters

**Solutions:**

```bash
# Set UTF-8 encoding
export LC_ALL=en_US.UTF-8
export LANG=en_US.UTF-8

# Use --quiet to suppress formatting
gzh-git --quiet <command>

# Use machine-readable format
gzh-git history stats --format json
```

### Missing Colors

**Problem:** No colored output

**Solutions:**

```bash
# Check terminal color support
echo $TERM

# Force color output
FORCE_COLOR=1 gzh-git status

# Or use simpler format
gzh-git status --porcelain
```

## Common Error Messages

### "argument sanitization failed"

**Cause:** Command contains potentially unsafe arguments

**Solution:** The gitcmd sanitization system detected unsafe patterns. This is a security feature.

```bash
# Check command arguments
# Avoid special characters in branch names: ; | & > <
# Use standard branch naming: feature/my-branch

# If you see this with standard commands, please report it
```

### "invalid source branch: branch not found"

**Cause:** Specified branch doesn't exist

**Solution:**

```bash
# List available branches
git branch -a

# Use correct branch name
gzh-git merge detect feature/existing-branch main
```

### "failed to get current branch"

**Cause:** Detached HEAD state or corrupted repository

**Solution:**

```bash
# Check current state
git status

# Checkout a branch
git checkout main

# Or repair repository
git fsck
```

## Debugging

### Enable Verbose Output

```bash
# Use verbose flag
gzh-git --verbose <command>

# Or set environment variable
export GZH_GIT_VERBOSE=1
gzh-git <command>
```

### Check Git Operations

```bash
# Enable Git tracing
GIT_TRACE=1 gzh-git <command>

# Check Git config
git config --list

# Verify Git works
git status
```

### Generate Debug Report

```bash
# System information
uname -a
go version
git --version
gzh-git --version

# Repository information
git status
git log --oneline -5
git config --list

# Test command
gzh-git --verbose <command> 2>&1 | tee debug.log
```

## Getting Help

If you can't resolve the issue:

1. **Check documentation**: Read the [Command Reference](commands/README.md)
2. **Search issues**: https://github.com/gizzahub/gzh-cli-git/issues
3. **Ask for help**: https://github.com/gizzahub/gzh-cli-git/discussions
4. **Report a bug**: https://github.com/gizzahub/gzh-cli-git/issues/new

### Bug Report Template

When reporting issues, include:

```markdown
**Environment:**
- OS: [e.g., Ubuntu 22.04, macOS 14.0]
- Go version: [output of `go version`]
- Git version: [output of `git --version`]
- gzh-git version: [output of `gzh-git --version`]

**Command:**
```bash
gzh-git <command>
```

**Expected behavior:**
[What you expected to happen]

**Actual behavior:**
[What actually happened]

**Output/Error:**
```
[Paste full error output]
```

**Debug log:**
[Attach debug.log if generated]
```

## See Also

- [Installation Guide](INSTALL.md)
- [Quick Start](QUICKSTART.md)
- [Command Reference](commands/README.md)
