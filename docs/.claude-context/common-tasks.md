# Common Tasks - gzh-cli-git

## Adding New Git Commands

### Where to add
`cmd/gzh-git/` - create new command file

### Example
```go
// cmd/gzh-git/status.go
var statusCmd = &cobra.Command{
    Use:   "status",
    Short: "Show working tree status",
    RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
    // Implementation
    return nil
}

func init() {
    rootCmd.AddCommand(statusCmd)
}
```

## Adding Git Execution Logic

### Where to add
`internal/gitcmd/` - safe command execution

### Example
```go
// internal/gitcmd/executor.go
func (e *Executor) Run(ctx context.Context, args ...string) error {
    // Sanitize inputs
    sanitized := sanitize(args)

    cmd := exec.CommandContext(ctx, "git", sanitized...)
    return cmd.Run()
}
```

## Adding Output Parsing

### Where to add
`internal/parser/` - git output parsing

### Example
```go
// internal/parser/status.go
func ParseStatus(output string) (*Status, error) {
    lines := strings.Split(output, "\n")
    // Parse git status output
    return &Status{}, nil
}
```

## Adding Public APIs

### Where to add
`pkg/{feature}/` directory

### Example
```go
// pkg/branch/branch.go
package branch

type Manager struct {
    executor gitcmd.Executor
}

func (m *Manager) List(ctx context.Context) ([]string, error) {
    // Implementation
    return nil, nil
}
```

## Input Sanitization (Critical for Security)

### Always sanitize user inputs

```go
func sanitize(args []string) []string {
    var sanitized []string
    for _, arg := range args {
        // Remove dangerous characters
        if !containsDangerousChars(arg) {
            sanitized = append(sanitized, arg)
        }
    }
    return sanitized
}

func containsDangerousChars(s string) bool {
    dangerous := []string{";", "|", "&", "$", "`"}
    for _, d := range dangerous {
        if strings.Contains(s, d) {
            return true
        }
    }
    return false
}
```

## Testing Git Operations

### Use git-specific test helpers

```go
import "github.com/gizzahub/gzh-cli-git/internal/testutil"

func TestGitOperation(t *testing.T) {
    // Create temp git repo
    repo := testutil.TempGitRepo(t)

    // Create repo with commit
    repoWithCommit := testutil.TempGitRepoWithCommit(t)

    // Test operations
}
```

## Working with Branches

```go
import "github.com/gizzahub/gzh-cli-git/pkg/branch"

manager := branch.NewManager(executor)

// List branches
branches, err := manager.List(ctx)

// Create branch
err = manager.Create(ctx, "feature-branch")

// Switch branch
err = manager.Switch(ctx, "main")
```

## Working with Commits

```go
import "github.com/gizzahub/gzh-cli-git/pkg/commit"

handler := commit.NewHandler(executor)

// Create commit
err = handler.Commit(ctx, "commit message")

// Amend commit
err = handler.Amend(ctx, "new message")
```

## Error Handling Best Practices

```go
func runGitCommand(ctx context.Context, args ...string) error {
    if err := executor.Run(ctx, args...); err != nil {
        // Wrap with context
        return fmt.Errorf("git %s failed: %w",
            strings.Join(args, " "), err)
    }
    return nil
}
```
