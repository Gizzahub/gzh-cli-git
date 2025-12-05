# AGENTS_COMMON.md - Common Development Rules

Project-wide conventions for all gzh-cli-git modules.

______________________________________________________________________

## Naming Conventions

### Files

- **Commands**: `{command}.go` (e.g., `clone.go`, `status.go`)
- **Tests**: `{name}_test.go`
- **Mocks**: `{name}_mock.go` or in `mocks/` directory

### Code

- **Interfaces**: Noun (e.g., `Executor`, `Parser`, `Repository`)
- **Implementations**: Adjective+Noun (e.g., `DefaultExecutor`, `SafeParser`)
- **Constructors**: `New{Type}()` pattern

______________________________________________________________________

## Error Handling

### Standard Pattern

```go
if err != nil {
    return fmt.Errorf("{operation} failed: %w", err)
}
```

### Git-Specific Errors

```go
// For git command failures
if err != nil {
    return fmt.Errorf("git %s failed: %w", cmd, err)
}
```

______________________________________________________________________

## Git Safety Rules

### CRITICAL: Input Sanitization

```go
// Always sanitize user inputs before git commands
sanitized := sanitize.Path(userInput)
sanitized := sanitize.BranchName(userInput)
sanitized := sanitize.RemoteName(userInput)
```

### Forbidden Patterns

- ❌ Direct shell execution with user input
- ❌ `exec.Command("sh", "-c", userInput)`
- ❌ Unsanitized paths in git commands

### Safe Patterns

- ✅ Use `internal/gitcmd` for execution
- ✅ Validate inputs before execution
- ✅ Use argument arrays, not string concatenation

______________________________________________________________________

## Testing Patterns

### Unit Test Structure

```go
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name    string
        input   InputType
        want    OutputType
        wantErr bool
    }{
        // test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := FunctionName(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
            }
            if got != tt.want {
                t.Errorf("got = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Git Test Helpers

```go
// Use t.TempDir() for git repository tests
dir := t.TempDir()
// Initialize test repo
exec.Command("git", "init", dir).Run()
```

______________________________________________________________________

## Logging

### Levels

- **Debug**: Detailed git command output
- **Info**: Operation progress
- **Warn**: Non-fatal issues (dirty working tree, etc.)
- **Error**: Operation failures

### Format

```go
log.Info("cloning repository", "url", url, "path", path)
log.Error("clone failed", "error", err)
```

______________________________________________________________________

## Commit Message Format

```
{type}({scope}): {imperative verb} {description}

{optional body}

Model: claude-{model}
Co-Authored-By: Claude <noreply@anthropic.com>
```

### Types

- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation
- `refactor`: Code restructuring
- `test`: Test additions/changes
- `chore`: Maintenance

### Scopes

- `cmd`: CLI commands
- `internal`: Internal packages
- `pkg/branch`: Branch operations
- `pkg/commit`: Commit operations
- `pkg/merge`: Merge operations
- etc.

______________________________________________________________________

## Forbidden Patterns

### Code

- ❌ Global state
- ❌ `init()` with side effects
- ❌ Hardcoded paths
- ❌ Unsanitized shell execution

### Git Operations

- ❌ Force push without confirmation
- ❌ Destructive operations without backup
- ❌ Operations on detached HEAD without warning

______________________________________________________________________

## Quality Standards

### Coverage Targets

| Package | Target |
|---------|--------|
| internal/ | 80%+ |
| pkg/ | 85%+ |
| cmd/ | 70%+ |

### Before Commit

```bash
make quality  # REQUIRED
```

______________________________________________________________________

**Last Updated**: 2024-12-05
