# Branch Management Example

This example demonstrates gzh-git branch management features using the CLI.

## Features Demonstrated

1. **List Branches**: Show all local and remote branches
2. **Create Branches**: Create new branches with various options
3. **Delete Branches**: Remove branches safely
4. **Worktree Management**: Create branches with linked worktrees

## Usage

### List All Branches

```bash
# List local branches
gzh-git branch list

# List all branches (including remote)
gzh-git branch list --all
```

### Create a New Branch

```bash
# Create from current HEAD
gzh-git branch create feature/new-feature

# Create from specific commit/branch
gzh-git branch create feature/new-feature --from main
```

### Create Branch with Worktree

```bash
# Create branch in separate working directory
gzh-git branch create feature/parallel --worktree /tmp/parallel-work
```

### Delete a Branch

```bash
# Delete local branch
gzh-git branch delete feature/old-feature

# Force delete (if not fully merged)
gzh-git branch delete feature/experimental --force
```

## Library Usage

For library integration, see [Library Guide](../../docs/LIBRARY.md).

Basic library example:

```go
import "github.com/gizzahub/gzh-cli-git/pkg/branch"

// Branch manager provides branch operations
// Worktree manager handles worktree operations
```

See [pkg/branch](../../pkg/branch) for complete API documentation.
