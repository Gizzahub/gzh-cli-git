# Phase 7.2: gzh-cli Integration Specification

**Phase**: 7.2
**Priority**: P0 (High)
**Status**: Pending
**Created**: 2025-11-30
**Dependencies**: Phase 7.1 (Library Publication)

______________________________________________________________________

## Overview

Phase 7.2 focuses on integrating the gzh-cli-git library into the gzh-cli unified CLI tool. This phase involves architectural planning, API integration, command mapping, and final v1.0.0 release preparation.

### Goals

1. **Integration Architecture** - Design how gzh-cli consumes gzh-cli-git library
1. **Command Integration** - Map gzh-git commands to gzh-cli command structure
1. **Shared Infrastructure** - Leverage gzh-cli's logging, config, and UI components
1. **Testing** - Validate integration through gzh-cli's test suite
1. **Documentation** - Update gzh-cli docs with Git functionality
1. **v1.0.0 Release** - Finalize both libraries for production

### Non-Goals

- Rewriting existing gzh-cli-git functionality
- GUI or web interface
- Plugin system (future enhancement)
- Cloud integrations

______________________________________________________________________

## Architecture

### Integration Layers

```
┌─────────────────────────────────────────┐
│         gzh-cli (Unified CLI)            │
├─────────────────────────────────────────┤
│                                          │
│  ┌────────────────────────────────────┐ │
│  │    Command Layer                    │ │
│  │  - gzh git <subcommand>            │ │
│  │  - Command routing                  │ │
│  │  - Argument parsing                 │ │
│  └────────────┬───────────────────────┘ │
│               │                          │
│  ┌────────────▼───────────────────────┐ │
│  │  Integration Layer (This Phase)     │ │
│  │  - API wrapper functions            │ │
│  │  - Error translation                │ │
│  │  - Progress reporting               │ │
│  └────────────┬───────────────────────┘ │
│               │                          │
└───────────────┼──────────────────────────┘
                │
        ┌───────▼────────┐
        │   gzh-cli-git  │
        │     Library    │
        └────────────────┘
```

### Data Flow

```
User Command Flow:
$ gzh git status
    ↓
gzh-cli Command Router
    ↓
Git Integration Handler (new)
    ↓
gzh-cli-git Library (repository.Client)
    ↓
Git CLI Execution
    ↓
Parse & Format Output
    ↓
Return to gzh-cli
    ↓
Display to User
```

______________________________________________________________________

## Component 1: Integration Architecture

### Purpose

Define how gzh-cli will integrate and consume the gzh-cli-git library.

### 1.1 Dependency Management

**Update gzh-cli go.mod:**

```go
module github.com/gizzahub/gzh-cli

go 1.24

require (
    github.com/gizzahub/gzh-cli-git v0.1.0  // Add this
    github.com/spf13/cobra v1.8.0
    // ... other dependencies
)
```

### 1.2 Integration Package Structure

**Create in gzh-cli repository:**

```
gzh-cli/
├── internal/
│   └── git/                    # Git integration (new)
│       ├── client.go          # Wrapper around gzh-cli-git
│       ├── logger.go          # Logger adapter
│       ├── progress.go        # Progress reporter adapter
│       ├── formatter.go       # Output formatter adapter
│       └── errors.go          # Error translation
└── cmd/
    └── git/                    # Git commands (new)
        ├── status.go
        ├── clone.go
        ├── commit.go
        ├── branch.go
        ├── history.go
        └── merge.go
```

### 1.3 Client Wrapper

**Example `internal/git/client.go`:**

```go
package git

import (
    "context"

    "github.com/gizzahub/gzh-cli/internal/config"
    "github.com/gizzahub/gzh-cli/internal/logger"
    "github.com/gizzahub/gzh-cli-git/pkg/repository"
)

// Client wraps gzh-cli-git library with gzh-cli infrastructure
type Client struct {
    repo       repository.Client
    logger     *logger.Logger  // gzh-cli logger
    config     *config.Config  // gzh-cli config
    progressUI UIProgress       // gzh-cli progress UI
}

// NewClient creates a Git client integrated with gzh-cli
func NewClient(cfg *config.Config, log *logger.Logger) *Client {
    // Create repository client with adapters
    repoClient := repository.NewClient(
        repository.WithClientLogger(newLoggerAdapter(log)),
    )

    return &Client{
        repo:   repoClient,
        logger: log,
        config: cfg,
    }
}

// Status gets repository status with gzh-cli formatting
func (c *Client) Status(ctx context.Context, path string) error {
    // Use gzh-cli-git library
    repo, err := c.repo.Open(ctx, path)
    if err != nil {
        return c.translateError(err)
    }

    status, err := c.repo.GetStatus(ctx, repo)
    if err != nil {
        return c.translateError(err)
    }

    // Format using gzh-cli formatters
    return c.displayStatus(status)
}
```

______________________________________________________________________

## Component 2: Command Integration

### Purpose

Map gzh-git commands to gzh-cli command structure and integrate with existing gzh-cli features.

### 2.1 Command Mapping

**gzh-git → gzh-cli mapping:**

```
gzh-git status              → gzh git status
gzh-git clone <url>         → gzh git clone <url>
gzh-git info                → gzh git info
gzh-git commit auto         → gzh git commit auto
gzh-git commit validate     → gzh git commit validate
gzh-git branch list         → gzh git branch list
gzh-git branch create       → gzh git branch create
gzh-git history stats       → gzh git history stats
gzh-git history contributors → gzh git history contributors
gzh-git merge do            → gzh git merge do
gzh-git merge detect        → gzh git merge detect
```

### 2.2 Command Implementation

**Example `cmd/git/status.go`:**

```go
package git

import (
    "github.com/spf13/cobra"

    "github.com/gizzahub/gzh-cli/internal/git"
)

// NewStatusCmd creates the 'gzh git status' command
func NewStatusCmd(gitClient *git.Client) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "status [path]",
        Short: "Show working tree status",
        Long:  "Display the working tree status of a Git repository",
        Args:  cobra.MaximumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            path := "."
            if len(args) > 0 {
                path = args[0]
            }

            return gitClient.Status(cmd.Context(), path)
        },
    }

    // Add flags
    cmd.Flags().BoolP("quiet", "q", false, "Only show errors")
    cmd.Flags().StringP("format", "f", "table", "Output format (table|json|csv)")

    return cmd
}
```

### 2.3 Root Git Command

**Example `cmd/git/git.go`:**

```go
package git

import (
    "github.com/spf13/cobra"

    "github.com/gizzahub/gzh-cli/internal/git"
)

// NewGitCmd creates the 'gzh git' root command
func NewGitCmd(gitClient *git.Client) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "git",
        Short: "Git operations and automation",
        Long:  "Advanced Git operations powered by gzh-cli-git library",
    }

    // Add subcommands
    cmd.AddCommand(NewStatusCmd(gitClient))
    cmd.AddCommand(NewCloneCmd(gitClient))
    cmd.AddCommand(NewInfoCmd(gitClient))
    cmd.AddCommand(NewCommitCmd(gitClient))
    cmd.AddCommand(NewBranchCmd(gitClient))
    cmd.AddCommand(NewHistoryCmd(gitClient))
    cmd.AddCommand(NewMergeCmd(gitClient))

    return cmd
}
```

______________________________________________________________________

## Component 3: Shared Infrastructure

### Purpose

Leverage gzh-cli's existing logging, configuration, and UI components instead of duplicating functionality.

### 3.1 Logger Adapter

**`internal/git/logger.go`:**

```go
package git

import (
    "github.com/gizzahub/gzh-cli/internal/logger"
    "github.com/gizzahub/gzh-cli-git/pkg/repository"
)

// loggerAdapter adapts gzh-cli logger to gzh-cli-git Logger interface
type loggerAdapter struct {
    logger *logger.Logger
}

func newLoggerAdapter(log *logger.Logger) repository.Logger {
    return &loggerAdapter{logger: log}
}

func (l *loggerAdapter) Debug(msg string, args ...interface{}) {
    l.logger.Debugf(msg, args...)
}

func (l *loggerAdapter) Info(msg string, args ...interface{}) {
    l.logger.Infof(msg, args...)
}

func (l *loggerAdapter) Warn(msg string, args ...interface{}) {
    l.logger.Warnf(msg, args...)
}

func (l *loggerAdapter) Error(msg string, args ...interface{}) {
    l.logger.Errorf(msg, args...)
}
```

### 3.2 Progress Reporter Adapter

**`internal/git/progress.go`:**

```go
package git

import (
    "github.com/gizzahub/gzh-cli/internal/ui"
    "github.com/gizzahub/gzh-cli-git/pkg/repository"
)

// progressAdapter adapts gzh-cli progress UI to gzh-cli-git ProgressReporter
type progressAdapter struct {
    progressBar *ui.ProgressBar
}

func newProgressAdapter(pb *ui.ProgressBar) repository.ProgressReporter {
    return &progressAdapter{progressBar: pb}
}

func (p *progressAdapter) Start(total int64) {
    p.progressBar.Start(total)
}

func (p *progressAdapter) Update(current int64) {
    p.progressBar.Update(current)
}

func (p *progressAdapter) Done() {
    p.progressBar.Finish()
}
```

### 3.3 Error Translation

**`internal/git/errors.go`:**

```go
package git

import (
    "errors"
    "fmt"

    giterrors "github.com/gizzahub/gzh-cli-git/pkg/repository"
    "github.com/gizzahub/gzh-cli/internal/errors"
)

// translateError converts gzh-cli-git errors to gzh-cli error format
func (c *Client) translateError(err error) error {
    if err == nil {
        return nil
    }

    // Check for specific error types
    var valErr *giterrors.ValidationError
    if errors.As(err, &valErr) {
        return errors.NewValidationError(
            fmt.Sprintf("Git validation failed: %s", valErr.Reason),
        )
    }

    // Wrap unknown errors
    return errors.Wrap(err, "Git operation failed")
}
```

### 3.4 Output Formatter Adapter

**`internal/git/formatter.go`:**

```go
package git

import (
    "io"

    "github.com/gizzahub/gzh-cli/internal/output"
    "github.com/gizzahub/gzh-cli-git/pkg/repository"
)

// formatStatus formats repository status using gzh-cli formatters
func (c *Client) displayStatus(status *repository.Status) error {
    formatter := output.NewFormatter(c.config.OutputFormat)

    data := map[string]interface{}{
        "clean":        status.IsClean,
        "staged":       len(status.StagedFiles),
        "modified":     len(status.ModifiedFiles),
        "untracked":    len(status.UntrackedFiles),
        "ahead":        status.Ahead,
        "behind":       status.Behind,
    }

    return formatter.Print(data)
}
```

______________________________________________________________________

## Component 4: Testing Integration

### Purpose

Validate the integration through gzh-cli's existing test infrastructure.

### 4.1 Integration Tests

**Create `internal/git/client_test.go`:**

```go
package git

import (
    "context"
    "testing"

    "github.com/gizzahub/gzh-cli/internal/config"
    "github.com/gizzahub/gzh-cli/internal/logger"
)

func TestClient_Status(t *testing.T) {
    // Setup
    cfg := config.NewDefault()
    log := logger.NewTest(t)
    client := NewClient(cfg, log)

    // Test
    err := client.Status(context.Background(), ".")
    if err != nil {
        t.Fatalf("Status failed: %v", err)
    }
}

func TestClient_Clone(t *testing.T) {
    // Test clone with temporary directory
    // ...
}
```

### 4.2 E2E Tests

**Add to gzh-cli E2E test suite:**

```go
func TestGitCommands(t *testing.T) {
    tests := []struct {
        name    string
        command string
        want    string
    }{
        {
            name:    "git status",
            command: "git status",
            want:    "Clean:",
        },
        {
            name:    "git info",
            command: "git info",
            want:    "Branch:",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            output, err := runCommand(t, tt.command)
            if err != nil {
                t.Fatalf("Command failed: %v", err)
            }

            if !strings.Contains(output, tt.want) {
                t.Errorf("Output does not contain %q: %s", tt.want, output)
            }
        })
    }
}
```

______________________________________________________________________

## Component 5: Documentation Updates

### Purpose

Update gzh-cli documentation to include Git functionality.

### 5.1 User Documentation

**Update gzh-cli README.md:**

```markdown
## Features

- **Project Management**: Streamlined project initialization and configuration
- **Git Operations**: Advanced Git automation powered by gzh-cli-git ⭐ NEW
  - Smart commit messages with templates
  - Branch management and worktrees
  - History analysis and statistics
  - Merge/rebase with conflict detection
- **Development Tools**: Code generation, testing, and deployment automation
```

### 5.2 Command Reference

**Create `docs/commands/git.md` in gzh-cli:**

```markdown
# Git Commands

All Git operations in gzh-cli are powered by the [gzh-cli-git](https://github.com/gizzahub/gzh-cli-git) library.

## Available Commands

### Repository Operations
- `gzh git status` - Show working tree status
- `gzh git clone <url>` - Clone a repository
- `gzh git info` - Show repository information

### Commit Operations
- `gzh git commit auto` - Auto-generate and create commit
- `gzh git commit validate <message>` - Validate commit message
- `gzh git commit template list` - List available templates

### Branch Operations
- `gzh git branch list` - List branches
- `gzh git branch create <name>` - Create a branch
- `gzh git branch delete <name>` - Delete a branch

### History Operations
- `gzh git history stats` - Show commit statistics
- `gzh git history contributors` - Show contributor statistics
- `gzh git history file <path>` - Show file history

### Merge Operations
- `gzh git merge do <branch>` - Merge a branch
- `gzh git merge detect <src> <target>` - Detect merge conflicts
- `gzh git merge abort` - Abort in-progress merge

## Examples

See [gzh-cli-git documentation](https://pkg.go.dev/github.com/gizzahub/gzh-cli-git) for detailed examples.
```

______________________________________________________________________

## Component 6: Release Preparation

### Purpose

Prepare both libraries for v1.0.0 production release.

### 6.1 Version Coordination

**gzh-cli-git versions:**

```
v0.1.0-alpha → Initial library release (Phase 7.1)
v0.1.0       → Stable after alpha testing
v1.0.0       → Production-ready after gzh-cli integration (Phase 7.2)
```

**gzh-cli versions:**

```
v0.x.x       → Pre-Git integration
v1.0.0       → With Git integration (Phase 7.2)
```

### 6.2 Release Checklist

**For gzh-cli-git v1.0.0:**

- [ ] All integration tests passing in gzh-cli
- [ ] No breaking API changes since v0.1.0
- [ ] Performance benchmarks maintained
- [ ] Documentation complete and accurate
- [ ] CHANGELOG.md updated
- [ ] Migration guide from v0.x to v1.0
- [ ] GitHub release created
- [ ] pkg.go.dev verified

**For gzh-cli v1.0.0:**

- [ ] Git integration complete and tested
- [ ] All existing features still working
- [ ] New Git commands documented
- [ ] E2E tests passing
- [ ] CHANGELOG.md updated
- [ ] User migration guide created
- [ ] GitHub release created

### 6.3 Coordinated Release

**Release Process:**

1. **Week 1**: gzh-cli-git v0.1.0-alpha (Phase 7.1)
1. **Week 2-3**: Alpha testing, bug fixes
1. **Week 4**: gzh-cli-git v0.1.0 stable
1. **Week 5-6**: gzh-cli integration (Phase 7.2)
1. **Week 7**: Integration testing and refinement
1. **Week 8**: Coordinated v1.0.0 release

______________________________________________________________________

## Acceptance Criteria

### Phase 7.2 Complete When:

- [ ] gzh-cli integration package created (internal/git/)
- [ ] All Git commands integrated (status, clone, commit, branch, history, merge)
- [ ] Logger adapter implemented
- [ ] Progress reporter adapter implemented
- [ ] Error translation implemented
- [ ] Output formatter adapter implemented
- [ ] Integration tests passing in gzh-cli
- [ ] E2E tests passing in gzh-cli
- [ ] gzh-cli documentation updated
- [ ] Command reference created
- [ ] Both libraries ready for v1.0.0
- [ ] Release preparation complete

______________________________________________________________________

## Quality Metrics

### Integration Testing

- All gzh-cli-git tests still passing
- All gzh-cli tests still passing
- New Git integration tests passing
- E2E tests covering Git workflows

### Performance

- No performance regression in gzh-cli
- Git operations maintain gzh-cli-git benchmarks
- Memory usage acceptable

### Documentation

- Complete command reference
- Integration examples
- Migration guide for users

______________________________________________________________________

## Risks and Mitigation

### Risk 1: API Incompatibility

**Risk**: gzh-cli-git API may not fit gzh-cli architecture
**Impact**: High (requires refactoring)
**Mitigation**:

- Use adapter pattern for flexibility
- Keep integration layer thin
- Review integration architecture early

### Risk 2: Dependency Conflicts

**Risk**: gzh-cli and gzh-cli-git may have conflicting dependencies
**Impact**: Medium
**Mitigation**:

- Review dependencies before integration
- Use compatible versions
- Test dependency resolution

### Risk 3: Integration Complexity

**Risk**: Integration may be more complex than anticipated
**Impact**: Medium (delays v1.0.0 release)
**Mitigation**:

- Start with simple commands (status, info)
- Gradually add more complex features
- Allocate buffer time in schedule

______________________________________________________________________

## Timeline

### Estimated Duration: 2-3 weeks

**Week 1** (After Phase 7.1 complete):

- Integration package setup (2 days)
- Adapter implementation (2 days)
- Basic commands (status, info, clone) (1 day)

**Week 2**:

- Advanced commands (commit, branch, history, merge) (3 days)
- Integration testing (2 days)

**Week 3**:

- Documentation (2 days)
- E2E testing (1 day)
- Release preparation (2 days)

______________________________________________________________________

## Success Metrics

- All 7 command groups integrated successfully
- Zero regression in existing gzh-cli features
- Integration tests: 100% passing
- E2E tests: 100% passing
- User documentation: Complete
- v1.0.0 release: Ready

______________________________________________________________________

## Next Steps

After Phase 7.2 completion:

1. **v1.0.0 Release** - Coordinated release of both libraries
1. **Community Adoption** - Support early adopters
1. **Feature Enhancements** - Plan v1.1.0 features based on feedback
1. **Performance Optimization** - Improve based on real-world usage

______________________________________________________________________

## References

- gzh-cli repository: https://github.com/gizzahub/gzh-cli
- gzh-cli-git library: https://github.com/gizzahub/gzh-cli-git
- Phase 7.1 Spec: `specs/60-library-publication.md`
- Integration best practices: [Go Library Integration](https://go.dev/doc/modules/managing-dependencies)
