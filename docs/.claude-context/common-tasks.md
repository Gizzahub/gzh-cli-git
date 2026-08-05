# Common Tasks - gzh-cli-gitforge

## Core Design: Bulk-First Defaults

gz-git은 **기본적으로 bulk 모드**로 동작합니다. 모든 주요 명령어는 디렉토리를 스캔하여 여러 repository를 병렬 처리합니다.

### 기본값 (pkg/repository/defaults.go)

```go
// Local operations (status, fetch, pull, push, etc.)
DefaultLocalScanDepth = 1   // 현재 디렉토리 + 1레벨
DefaultLocalParallel  = 10  // 10개 병렬 처리

// Forge API operations (sync from-forge, config generate)
DefaultForgeParallel  = 4   // API rate limit 고려

// Legacy aliases (하위 호환성)
DefaultBulkMaxDepth = DefaultLocalScanDepth
DefaultBulkParallel = DefaultLocalParallel
```

### 스캔 깊이 설명

```
depth=0: 현재 디렉토리만 (단일 repo 동작)
depth=1: 현재 + 1레벨 (기본값)
depth=2: 현재 + 2레벨
```

### 단일 repo 작업

경로를 직접 지정하면 해당 repo만 처리:

```bash
gz-git status /path/to/repo
gz-git fetch /path/to/repo
```

### bulk 명령어 플래그 등록 (cmd/gz-git/cmd/bulk_common.go)

```go
func addBulkFlags(cmd *cobra.Command, flags *BulkFlags) {
    cmd.Flags().IntVarP(&flags.Depth, "scan-depth", "d",
        repository.DefaultBulkMaxDepth, "directory depth to scan")
    cmd.Flags().IntVarP(&flags.Parallel, "parallel", "j",
        repository.DefaultBulkParallel, "parallel workers")
    cmd.Flags().BoolVarP(&flags.DryRun, "dry-run", "n",
        false, "preview without executing")
    cmd.Flags().StringVar(&flags.Include, "include", "", "include pattern")
    cmd.Flags().StringVar(&flags.Exclude, "exclude", "", "exclude pattern")
}
```

______________________________________________________________________

## Adding New Git Commands

### Where to add

`cmd/gz-git/` - create new command file

### Example

```go
// cmd/gz-git/status.go
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

`internal/porcelain` — the single `git status --porcelain -z` parser for the whole
module. Do not add a second one; the defect class it exists to close was every
package that read git status re-parsing it their own way, each rediscovering the
same bugs (collapsed untracked directories, C-quoted paths naming no real file,
renames flattened into one nonexistent path).

It stops at the record level on purpose. Projecting records onto a domain type is
the caller's job, because the projections disagree: `pkg/repository`'s `Status`
folds the XY pair into a union of file lists, while a conflict check needs the
pair intact.

### Example

```go
// Any package holding a *gitcmd.Executor
stdout, err := c.runGit(ctx, repoPath, "status", "--porcelain", "-z", "-uall")
if err != nil {
    return nil, fmt.Errorf("failed to read status: %w", err)
}
records, err := porcelain.Parse(stdout)
if err != nil {
    return nil, fmt.Errorf("failed to read status: %w", err)
}

// Inside pkg/repository, parseStatusZ composes both steps into a Status.
```

### Rules that are not optional

- **`-z`, never plain `--porcelain`.** Without it git C-quotes any path holding a
  space or non-ASCII byte, so the string names no real file, and a path
  containing a newline splits one entry into two.
- **`runGit`, never `Executor.Run` or `RunOutput`.** `Run` reports a failed git
  through `Result.ExitCode` and returns a nil error, so `err != nil` alone turns
  "git failed" into "git found nothing". `RunOutput` trims its result, which eats
  the leading space of the first record — the load-bearing distinction between
  ` M` (unstaged) and `M ` (staged).
- **`-uall` when counting untracked files**, or git collapses a directory into
  one `dir/` entry and N new files are reported as one. Omit it (prefer `-uno`)
  when the caller only reads tracked state, since it forces a full recursive walk.
- **Never split on `\n` and never `TrimSpace` a record.** Both columns of the XY
  code carry meaning, and `X` is `' '` for every worktree-only change.

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
import "github.com/gizzahub/gzh-cli-gitforge/internal/testutil"

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
import "github.com/gizzahub/gzh-cli-gitforge/pkg/branch"

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
import "github.com/gizzahub/gzh-cli-gitforge/pkg/commit"

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

______________________________________________________________________

## Repository Health Diagnostics (sync status)

### Overview

`gz-git sync status` provides comprehensive health checks for multiple repositories:

- Fetches from all remotes (with timeout)
- Detects network connectivity issues
- Compares local vs remote branches
- Identifies potential conflicts
- Provides actionable recommendations

### Architecture

```
DiagnosticExecutor (pkg/reposync/diagnostic_executor.go)
    ├── CheckHealth() - Main entry point
    ├── checkOne() - Per-repository health check
    ├── fetchWithTimeout() - Remote fetch with timeout
    ├── classifyDivergence() - Analyze local vs remote
    ├── classifyHealth() - Determine overall health
    └── generateRecommendation() - Create actionable guidance
```

### Type Hierarchy

```go
// Health classification
type HealthStatus string
const (
    HealthHealthy      // up-to-date, clean
    HealthWarning      // diverged, can be resolved
    HealthError        // conflicts, dirty + behind
    HealthUnreachable  // network timeout, auth failed
)

// Divergence analysis
type DivergenceType string
const (
    DivergenceNone         // local == remote
    DivergenceFastForward  // can fast-forward pull
    DivergenceDiverged     // merge/rebase needed
    DivergenceAhead        // can push
    DivergenceConflict     // merge conflicts exist
    DivergenceNoUpstream   // no upstream configured
)

// Network status
type NetworkStatus string
const (
    NetworkOK           // fetch succeeded
    NetworkTimeout      // fetch timed out
    NetworkUnreachable  // DNS/connection failed
    NetworkAuthFailed   // authentication failed
)
```

### Adding New Health Checks

```go
// pkg/reposync/diagnostic_executor.go
func (e DiagnosticExecutor) checkOne(ctx context.Context, ...) RepoHealth {
    // ... existing checks ...

    // Add custom health check
    if opts.CheckCustom {
        customStatus := e.checkCustomCondition(ctx, r)
        health.CustomStatus = customStatus
    }

    return health
}
```

### Customizing Recommendations

```go
// pkg/reposync/diagnostic_executor.go
func generateRecommendation(health RepoHealth) string {
    switch health.HealthStatus {
    case HealthWarning:
        if health.CustomCondition {
            return "Custom recommendation for your case"
        }
    }
    // ... existing logic ...
}
```

### Testing Health Checks

```go
// Integration test example
func TestCustomHealthCheck(t *testing.T) {
    repo := testutil.TempGitRepoWithCommit(t)
    defer os.RemoveAll(filepath.Dir(repo))

    executor := DiagnosticExecutor{
        Client: repo.NewClient(),
    }

    opts := DiagnosticOptions{
        SkipFetch: true, // Fast test without network
        CheckWorkTree: true,
    }

    report, err := executor.CheckHealth(ctx, repos, opts)
    // Assert on report.Results
}
```

### CLI Integration

```go
// pkg/reposynccli/status_command.go
func (f CommandFactory) runStatus(cmd *cobra.Command, opts *StatusOptions) error {
    // Load repositories from config or scan directory
    repos := loadRepos(opts)

    // Execute health check
    executor := reposync.DiagnosticExecutor{}
    report, err := executor.CheckHealth(ctx, repos, diagOpts)

    // Display results
    f.printHealthReport(cmd, report, opts.Verbose)
}
```

### Performance Considerations

- **Parallel execution**: Default 4 workers, configurable with `--parallel`
- **Timeout handling**: Default 30s per fetch, configurable with `--timeout`
- **Skip fetch**: Use `--skip-fetch` for fast checks (may show stale data)
- **Network classification**: Errors are parsed from git stderr for specific guidance

______________________________________________________________________

## Workspace Commands

### Workspace Init (Merged with Scan)

`workspace init` now combines initialization and scanning:

```bash
# Show usage (no arguments)
gz-git workspace init

# Scan directory and create config
gz-git workspace init .                    # Current dir
gz-git workspace init ~/mydevbox           # Specific dir
gz-git workspace init . -d 3               # Depth 3
gz-git workspace init . --exclude "vendor,tmp"

# Options
gz-git workspace init . --force            # Overwrite existing config
gz-git workspace init . --template         # Empty template only (no scan)
```

### Workspace Sync Workflow

```bash
# 1. Initialize workspace config
gz-git workspace init ~/mydevbox

# 2. (Optional) Edit config
vi ~/mydevbox/.gz-git.yaml

# 3. Sync repositories
gz-git workspace sync

# 4. Check status
gz-git workspace status
```

### Child Config Generation

When syncing hierarchical configs, control child config format:

```yaml
# Parent config
childConfigMode: repositories  # Simple format (default)
# childConfigMode: workspaces  # Map format
# childConfigMode: none        # No config generation
```

### Config Format Detection

gz-git uses content-based detection:

```go
// pkg/workspacecli/config_loader.go
func detectConfigKind(cfg *config.Config) ConfigKind {
    if cfg.Kind != "" {
        return cfg.Kind  // Explicit takes priority
    }
    if len(cfg.Workspaces) > 0 || len(cfg.Profiles) > 0 {
        return ConfigKindWorkspace
    }
    return ConfigKindRepositories  // Default
}
```

### Adding Workspace CLI Commands

Location: `pkg/workspacecli/`

```go
// pkg/workspacecli/init_command.go
func (f *CommandFactory) newInitCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "init [path]",
        Short: "Initialize workspace configuration",
        RunE: func(cmd *cobra.Command, args []string) error {
            // Handle no args → show usage
            // Handle path arg → scan and generate
        },
    }
}
```

### Testing Workspace Commands

```go
func TestWorkspaceInit(t *testing.T) {
    tmpDir := t.TempDir()

    // Create test git repos
    testutil.TempGitRepoAt(t, filepath.Join(tmpDir, "repo1"))
    testutil.TempGitRepoAt(t, filepath.Join(tmpDir, "repo2"))

    // Run init
    cmd := workspacecli.NewCommandFactory().NewInitCommand()
    cmd.SetArgs([]string{tmpDir})
    err := cmd.Execute()

    // Verify config created
    configPath := filepath.Join(tmpDir, ".gz-git.yaml")
    assert.FileExists(t, configPath)
}
```
