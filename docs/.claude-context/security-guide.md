# Security Guide - gzh-cli-git

## Input Sanitization (Critical)

### Why It Matters
Git commands can be exploited through command injection if inputs aren't sanitized.

### Dangerous Patterns to Avoid

```go
// ❌ DANGEROUS - Command injection risk
func badExample(userInput string) error {
    cmd := exec.Command("sh", "-c", "git clone " + userInput)
    return cmd.Run()
}

// ✅ SAFE - Proper argument passing
func goodExample(url string) error {
    cmd := exec.Command("git", "clone", url)
    return cmd.Run()
}
```

### Sanitization Rules

1. **Never use shell execution** (`sh -c`)
2. **Always pass arguments separately** (not concatenated)
3. **Validate input format** before execution
4. **Use allowlists** for known-safe values

### Validation Functions

```go
func isValidURL(s string) bool {
    u, err := url.Parse(s)
    if err != nil {
        return false
    }
    return u.Scheme == "https" || u.Scheme == "git" || u.Scheme == "ssh"
}

func isValidBranchName(s string) bool {
    // Git branch name rules
    if strings.Contains(s, "..") {
        return false
    }
    if strings.HasPrefix(s, "/") || strings.HasSuffix(s, "/") {
        return false
    }
    // Check for control characters
    for _, r := range s {
        if r < 32 || r == 127 {
            return false
        }
    }
    return true
}

func isValidPath(s string) bool {
    // Prevent directory traversal
    if strings.Contains(s, "..") {
        return false
    }
    // Only allow relative paths
    if filepath.IsAbs(s) {
        return false
    }
    return true
}
```

## Safe Command Execution

### Using internal/gitcmd

```go
import "github.com/gizzahub/gzh-cli-git/internal/gitcmd"

executor := gitcmd.NewExecutor()

// Executor handles sanitization internally
err := executor.Run(ctx, "clone", url, destination)
```

### Custom Execution

```go
func safeGitCommand(ctx context.Context, args ...string) error {
    // 1. Validate all arguments
    for _, arg := range args {
        if !isSafeArgument(arg) {
            return errors.New("invalid argument")
        }
    }

    // 2. Use exec.CommandContext (respects context cancellation)
    cmd := exec.CommandContext(ctx, "git", args...)

    // 3. Set safe environment
    cmd.Env = []string{
        "GIT_TERMINAL_PROMPT=0", // Disable prompts
    }

    return cmd.Run()
}

func isSafeArgument(arg string) bool {
    // Check for dangerous characters
    dangerous := []string{";", "|", "&", "$", "`", "\n", "\r"}
    for _, d := range dangerous {
        if strings.Contains(arg, d) {
            return false
        }
    }
    return true
}
```

## Credentials Handling

### Never log credentials

```go
func clone(ctx context.Context, url string) error {
    // ❌ WRONG - URL might contain credentials
    log.Printf("Cloning from %s", url)

    // ✅ CORRECT - Strip credentials before logging
    safeURL := stripCredentials(url)
    log.Printf("Cloning from %s", safeURL)

    return executor.Run(ctx, "clone", url)
}

func stripCredentials(urlStr string) string {
    u, err := url.Parse(urlStr)
    if err != nil {
        return "[invalid-url]"
    }
    u.User = nil
    return u.String()
}
```

### Credential storage

```go
// Use git credential helper, never store in code
// Configure via git config:
// git config --global credential.helper osxkeychain  // macOS
// git config --global credential.helper manager      // Windows
// git config --global credential.helper cache        // Linux
```

## File Path Safety

### Prevent directory traversal

```go
func safeJoinPath(base, userPath string) (string, error) {
    // Clean the user path
    cleaned := filepath.Clean(userPath)

    // Join with base
    fullPath := filepath.Join(base, cleaned)

    // Verify it's still under base directory
    if !strings.HasPrefix(fullPath, filepath.Clean(base)) {
        return "", errors.New("path traversal detected")
    }

    return fullPath, nil
}
```

## Testing Security

### Security test checklist

```go
func TestInputSanitization(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {
            name:    "command injection attempt",
            input:   "repo; rm -rf /",
            wantErr: true,
        },
        {
            name:    "valid input",
            input:   "my-repo",
            wantErr: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validateInput(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("wanted error: %v, got: %v", tt.wantErr, err)
            }
        })
    }
}
```

## Security Checklist

Before committing git command code:

- [ ] All user inputs are validated
- [ ] No shell execution (`sh -c`)
- [ ] Arguments passed separately (not concatenated)
- [ ] Credentials never logged
- [ ] Paths validated against traversal
- [ ] Context cancellation respected
- [ ] Security tests added
