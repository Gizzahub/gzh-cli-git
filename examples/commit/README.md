# Commit Automation Example

This example demonstrates gzh-git commit automation features using the CLI.

## Features Demonstrated

1. **Validate Commit Messages**: Check if messages follow Conventional Commits format
1. **List Templates**: Show available commit message templates
1. **Show Template Details**: Display template format and rules
1. **Auto-Generate Messages**: Create commit messages from staged changes

## Usage

### Validate a Commit Message

```bash
gzh-git commit validate "feat(cli): add status command"
```

### List Available Templates

```bash
gzh-git commit template list
```

### Show Template Details

```bash
gzh-git commit template show conventional
```

### Auto-Generate Commit Message

```bash
# Stage some changes first
git add file.txt

# Generate commit message
gzh-git commit auto
```

## Library Usage

For library integration, see [Library Guide](../../docs/LIBRARY.md).

Basic library example:

```go
import "github.com/gizzahub/gzh-cli-git/pkg/commit"

generator := commit.NewGenerator()
template Mgr := commit.NewTemplateManager()
validator := commit.NewValidator()
```

See [pkg/commit](../../pkg/commit) for complete API documentation.
