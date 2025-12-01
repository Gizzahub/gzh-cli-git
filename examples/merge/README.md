# Merge & Conflict Detection Example

This example demonstrates gzh-git merge and conflict detection features using the CLI.

## Features Demonstrated

1. **Pre-Merge Conflict Detection**: Identify conflicts before attempting merge
2. **Merge Execution**: Execute merges with various strategies
3. **Merge Abort**: Safely abort in-progress merges
4. **Rebase Operations**: Rebase branches interactively or non-interactively

## Usage

### Detect Conflicts Before Merging

```bash
# Check for conflicts between branches
gzh-git merge detect feature/mybranch main

# Detailed conflict analysis
gzh-git merge detect feature/mybranch main --detailed
```

### Execute Merge

```bash
# Basic merge
gzh-git merge do feature/mybranch

# Merge with specific strategy
gzh-git merge do feature/mybranch --strategy recursive

# Merge without creating commit (for review)
gzh-git merge do feature/mybranch --no-commit
```

### Abort Merge

```bash
# If merge has conflicts, abort and return to pre-merge state
gzh-git merge abort
```

### Rebase Operations

```bash
# Rebase current branch onto main
gzh-git merge rebase main

# Interactive rebase
gzh-git merge rebase main --interactive
```

## Merge Strategies

gzh-git supports multiple merge strategies:

- **fast-forward**: Fast-forward only (no merge commit)
- **recursive**: Default 3-way merge (Git's default)
- **ours**: Prefer current branch on conflicts
- **theirs**: Prefer incoming branch on conflicts
- **octopus**: Merge multiple branches

## Library Usage

For library integration, see [Library Guide](../../docs/LIBRARY.md).

See [pkg/merge](../../pkg/merge) for complete API documentation.
