# AGENTS.md - gzh-git CLI Module Guide

Module-specific guidelines for the gzh-git CLI module.

**Parent**: See `cmd/AGENTS_COMMON.md` for common rules.

______________________________________________________________________

## Module Overview

**Purpose**: Git operations CLI for repository management
**Binary**: `gz-git`
**Entry Point**: `main.go`

______________________________________________________________________

## File Structure

```
cmd/gzh-git/
├── AGENTS.md       # This file
├── main.go         # Entry point (calls Execute())
├── root.go         # Root command and subcommand registration
├── version.go      # Version information
├── clone.go        # Clone command
├── status.go       # Status command
├── branch.go       # Branch commands
├── commit.go       # Commit commands
└── ...             # Other git commands
```

______________________________________________________________________

## Command Structure

### Root Command (`root.go`)

- Defines the root Cobra command
- Registers all subcommands in `init()`
- Handles global flags (--verbose, --debug)

### Adding New Commands

1. Create `{command}.go` file:

```go
var myCmd = &cobra.Command{
    Use:   "mycommand",
    Short: "Brief description",
    Long:  `Detailed description...`,
    RunE: func(cmd *cobra.Command, args []string) error {
        // Use pkg/ for business logic
        return pkg.DoSomething(args)
    },
}

func init() {
    rootCmd.AddCommand(myCmd)
    myCmd.Flags().StringP("flag", "f", "", "Flag description")
}
```

2. Register in `init()` of the command file

______________________________________________________________________

## Key Patterns

### Flag Handling

```go
func init() {
    myCmd.Flags().StringP("branch", "b", "", "Branch name")
    myCmd.Flags().BoolP("force", "f", false, "Force operation")
    myCmd.Flags().StringSliceP("include", "i", nil, "Patterns to include")
}
```

### Using Internal Packages

```go
import (
    "github.com/gizzahub/gzh-cli-git/internal/gitcmd"
    "github.com/gizzahub/gzh-cli-git/internal/parser"
)

func runClone(url, path string) error {
    // Use gitcmd for safe execution
    output, err := gitcmd.Run("clone", url, path)
    if err != nil {
        return fmt.Errorf("clone failed: %w", err)
    }

    // Use parser for output processing
    result := parser.ParseCloneOutput(output)
    return nil
}
```

### Using Public Packages

```go
import (
    "github.com/gizzahub/gzh-cli-git/pkg/repository"
    "github.com/gizzahub/gzh-cli-git/pkg/branch"
)

func runBranch(name string) error {
    repo, err := repository.Open(".")
    if err != nil {
        return err
    }

    return branch.Create(repo, name)
}
```

### Error Handling

- Use `RunE` instead of `Run`
- Return errors, don't `os.Exit()` directly
- Wrap errors with context

```go
RunE: func(cmd *cobra.Command, args []string) error {
    if len(args) < 1 {
        return fmt.Errorf("repository URL required")
    }

    if err := doOperation(args[0]); err != nil {
        return fmt.Errorf("operation failed: %w", err)
    }

    return nil
},
```

### Output

- Use `fmt.Println` for normal output
- Use `fmt.Fprintln(os.Stderr, ...)` for errors
- Consider `--json` flag for structured output

______________________________________________________________________

## Testing

```go
func TestCloneCommand(t *testing.T) {
    // Create temporary directory
    dir := t.TempDir()

    // Test command execution
    cmd := rootCmd
    cmd.SetArgs([]string{"clone", "https://github.com/test/repo", dir})

    err := cmd.Execute()
    if err != nil {
        t.Errorf("unexpected error: %v", err)
    }
}
```

______________________________________________________________________

## Dependencies

- `github.com/spf13/cobra` - CLI framework
- `internal/gitcmd` - Safe git execution
- `internal/parser` - Output parsing
- `pkg/*` - Business logic packages

______________________________________________________________________

**Last Updated**: 2024-12-05
