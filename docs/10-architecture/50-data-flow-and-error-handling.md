# 6-7. Data Flow and Error Handling

> gzh-cli-gitforge 아키텍처 문서 · [인덱스](README.md) · [ARCHITECTURE.md](../../ARCHITECTURE.md)

## 6. Data Flow

### 6.1 Repository Open Flow

```
Library Consumer (gzh-cli)
         │
         ▼
┌──────────────────────┐
│ pkg/repository       │
│ Client.Open(ctx,path)│
└───────────┬──────────┘
            │
            ├──▶ Validate path exists
            │    (stdlib + pkg/repository checks)
            │
            ├──▶ Check .git directory
            │    (filesystem check)
            │
            ├──▶ Execute: git rev-parse --git-dir
            │    (internal/gitcmd/Executor)
            │
            ├──▶ Parse Git output
            │    (pkg/repository)
            │
            ▼
  ┌───────────────────┐
  │ Return Repository │
  │  - Path           │
  │  - GitDir         │
  │  - WorkTree       │
  │  - IsBare         │
  └───────────────────┘
```

### 6.2 Error Flow

```
Error Occurs in Git CLI
         │
         ▼
┌──────────────────────┐
│ internal/gitcmd      │  Capture stderr, exit code
│ Executor.Run()       │
└───────────┬──────────┘
            │
            ▼
┌──────────────────────┐
│ pkg/repository       │  Wrap error with context
│ Client method        │  return GitError{Op, Path, Err}
└───────────┬──────────┘
            │
            ▼
┌──────────────────────┐
│ cmd/gz-git/cmd       │  Cobra command layer
│ Command handler      │  Format user-friendly message
└───────────┬──────────┘
            │
            ▼
      Display to User
   "Failed to clone repository at /path:
    remote: repository not found

    Suggestions:
    - Check repository URL
    - Verify access permissions"
```

______________________________________________________________________

## 7. Error Handling Strategy

### 7.1 Error Types

```go
// internal/gitcmd/executor.go
type GitError struct {
    Command  string
    ExitCode int
    Stderr   string
    Cause    error
}

// pkg/repository/types.go
type ValidationError struct {
    Field  string
    Value  string
    Reason string
}

// Domain-specific "error-like" results are modeled as types, e.g.:
//   pkg/merge.ConflictReport
//   pkg/branch.CleanupReport
```

### 7.2 Error Handling Pattern

```go
// pkg/repository/client.go (simplified)
if opts.URL == "" {
    return nil, &ValidationError{
        Field:  "URL",
        Value:  opts.URL,
        Reason: "URL is required",
    }
}

// RunOutput/RunLines return *gitcmd.GitError on non-zero exit codes.
branch, err := c.executor.RunOutput(ctx, repo.Path, "rev-parse", "--abbrev-ref", "HEAD")
if err != nil {
    return nil, fmt.Errorf("failed to resolve current branch: %w", err)
}

_ = branch
```

### 7.3 Error Inspection Helpers

```go
// Example helper when you want to branch on git exit codes/output.
func IsNotRepository(err error) bool {
    var gitErr *gitcmd.GitError
    if errors.As(err, &gitErr) {
        return gitErr.ExitCode == 128 &&
            strings.Contains(gitErr.Stderr, "not a git repository")
    }
    return false
}
```

### 7.4 Bulk Operation Error Surfacing

Every bulk operation reduces a failed repository to **one table row**, so the
error it stores is the only thing the user will ever see about that failure.
The contract for building it:

1. **Prefer git's stderr over the exit status.** `exit status 128` is the same
   string for a permission denial, a rejected non-fast-forward and a missing
   remote. git already wrote the distinguishing text; discarding it makes the
   three indistinguishable.
1. **Collapse, never truncate.** stderr is joined onto a single line because a
   newline would break the row alignment of the bulk table. No content is
   dropped.
1. **Fall back in a fixed order** — execution error → stderr summary → wrapped
   command error → bare exit code — so a repository always reports *something*
   more specific than "failed".

All bulk git invocations share one builder rather than formatting errors at
each call site, which is what let `push` drift from `pull` and report only its
exit status:

```go
// pkg/repository/bulk.go
func buildGitCommandError(op string, execErr error, exitCode int, stderr string, cmdErr error) error
func buildPullError(execErr error, exitCode int, stderr string, cmdErr error) error // op = "pull"
func buildPushError(execErr error, exitCode int, stderr string, cmdErr error) error // op = "push"
```

**Boundary with auth detection.** `isAuthenticationError(stderr)` /
`ErrAuthRequired` exist so bulk ops never block on a credential prompt
(`GIT_TERMINAL_PROMPT=0`). It answers "can we authenticate at all", not "may
this identity write here". `Permission ... denied` is an *authorization*
failure against a valid identity, so it stays on the generic path and reaches
the user carrying git's own text.
