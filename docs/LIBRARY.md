# Library Integration Guide

Use `gzh-git` as a Go library in your own applications.

## Installation

```bash
go get github.com/gizzahub/gzh-cli-git
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/gizzahub/gzh-cli-git/pkg/repository"
    "github.com/gizzahub/gzh-cli-git/pkg/commit"
)

func main() {
    ctx := context.Background()

    // Open repository
    client := repository.NewClient()
    repo, err := client.Open(ctx, "/path/to/repo")
    if err != nil {
        log.Fatal(err)
    }

    // Generate commit message
    generator := commit.NewGenerator()
    template, _ := commit.GetBuiltinTemplate("conventional")

    msg, err := generator.Generate(ctx, repo, template)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(msg.Format())
}
```

## Core Packages

### repository

Manage Git repositories.

```go
import "github.com/gizzahub/gzh-cli-git/pkg/repository"

// Create client
client := repository.NewClient()

// Check if path is a repository
isRepo := client.IsRepository(ctx, "/path/to/repo")

// Open repository
repo, err := client.Open(ctx, "/path/to/repo")

// Clone repository
repo, err := client.Clone(ctx, repository.CloneOptions{
    URL:          "https://github.com/user/repo.git",
    Path:         "/local/path",
    Branch:       "main",
    Depth:        1,
    SingleBranch: true,
})

// Get repository info
info, err := repo.Info(ctx)
fmt.Printf("Branch: %s\n", info.CurrentBranch)
fmt.Printf("Remote: %s\n", info.RemoteURL)
```

### commit

Automate commit message generation and validation.

```go
import "github.com/gizzahub/gzh-cli-git/pkg/commit"

// Get built-in template
template, err := commit.GetBuiltinTemplate("conventional")

// Or load custom template
template, err := commit.LoadTemplate("/path/to/template.yaml")

// Generate commit message
generator := commit.NewGenerator()
message, err := generator.Generate(ctx, repo, template)

// Validate commit message
validator := commit.NewValidator()
result := validator.Validate(ctx, repo, "feat: add new feature", template)

if result.Valid {
    fmt.Println("Valid commit message")
} else {
    for _, err := range result.Errors {
        fmt.Printf("Error: %s at line %d\n", err.Message, err.Line)
    }
}

// Create commit
err = generator.CreateCommit(ctx, repo, message.Format())
```

### branch

Manage branches and worktrees.

```go
import "github.com/gizzahub/gzh-cli-git/pkg/branch"

// Create branch manager
mgr := branch.NewManager()

// List branches
branches, err := mgr.List(ctx, repo, branch.ListOptions{
    All: true,
})

for _, b := range branches {
    fmt.Printf("%s (head: %v)\n", b.Name, b.IsHead)
}

// Create branch
err = mgr.Create(ctx, repo, branch.CreateOptions{
    Name:     "feature/new-feature",
    StartRef: "main",
    Track:    true,
})

// Delete branch
err = mgr.Delete(ctx, repo, branch.DeleteOptions{
    Name:  "feature/old",
    Force: false,
})

// Manage worktrees
wtMgr := branch.NewWorktreeManager()

// Add worktree
wt, err := wtMgr.Add(ctx, repo, branch.AddOptions{
    Path:   "/path/to/worktree",
    Branch: "feature/parallel",
    Force:  false,
})

// List worktrees
worktrees, err := wtMgr.List(ctx, repo)

// Remove worktree
err = wtMgr.Remove(ctx, repo, branch.RemoveOptions{
    Path:  "/path/to/worktree",
    Force: false,
})
```

### history

Analyze repository history.

```go
import (
    "github.com/gizzahub/gzh-cli-git/pkg/history"
    "github.com/gizzahub/gzh-cli-git/internal/gitcmd"
)

executor := gitcmd.NewExecutor()

// Analyze commit statistics
analyzer := history.NewHistoryAnalyzer(executor)
stats, err := analyzer.Analyze(ctx, repo, history.AnalyzeOptions{
    Since:  time.Now().AddDate(0, -1, 0), // Last month
    Branch: "main",
})

fmt.Printf("Total commits: %d\n", stats.TotalCommits)
fmt.Printf("Contributors: %d\n", stats.UniqueAuthors)

// Analyze contributors
contribAnalyzer := history.NewContributorAnalyzer(executor)
contributors, err := contribAnalyzer.GetTopContributors(ctx, repo, 10)

for _, c := range contributors {
    fmt.Printf("%s: %d commits\n", c.Name, c.TotalCommits)
}

// Get file history
tracker := history.NewFileHistoryTracker(executor)
fileHistory, err := tracker.GetHistory(ctx, repo, "src/main.go", history.HistoryOptions{
    MaxCount: 10,
    Follow:   true,
})

// Get blame information
blameInfo, err := tracker.GetBlame(ctx, repo, "src/main.go")

for _, line := range blameInfo.Lines {
    fmt.Printf("%s: %s\n", line.Author, line.Content)
}
```

### merge

Perform merge and rebase operations.

```go
import (
    "github.com/gizzahub/gzh-cli-git/pkg/merge"
    "github.com/gizzahub/gzh-cli-git/internal/gitcmd"
)

executor := gitcmd.NewExecutor()

// Detect conflicts
detector := merge.NewConflictDetector(executor)
report, err := detector.Detect(ctx, repo, merge.DetectOptions{
    Source: "feature/new-feature",
    Target: "main",
})

if report.TotalConflicts > 0 {
    fmt.Printf("Found %d conflicts\n", report.TotalConflicts)
    for _, conflict := range report.Conflicts {
        fmt.Printf("  %s: %s\n", conflict.ConflictType, conflict.FilePath)
    }
}

// Perform merge
mgr := merge.NewMergeManager(executor, detector)
result, err := mgr.Merge(ctx, repo, merge.MergeOptions{
    Source:           "feature/new-feature",
    Target:           "main",
    Strategy:         merge.StrategyRecursive,
    AllowFastForward: true,
})

if result.Success {
    fmt.Printf("Merge successful: %s\n", result.CommitHash)
} else {
    fmt.Printf("Merge has conflicts in %d files\n", len(result.Conflicts))
}

// Rebase
rebaseMgr := merge.NewRebaseManager(executor)
rebaseResult, err := rebaseMgr.Rebase(ctx, repo, merge.RebaseOptions{
    Branch: "main",
    Onto:   "",
})

if rebaseResult.Success {
    fmt.Printf("Rebased %d commits\n", rebaseResult.CommitsRebased)
}
```

## Advanced Usage

### Custom Commit Templates

```go
import "github.com/gizzahub/gzh-cli-git/pkg/commit"

// Define custom template
template := &commit.Template{
    Name:        "my-template",
    Description: "My custom template",
    Format:      "{type}({scope}): {subject}\n\n{body}",
    Variables: []commit.TemplateVariable{
        {
            Name:        "type",
            Description: "Commit type",
            Required:    true,
            Options:     []string{"feat", "fix", "docs", "chore"},
        },
        {
            Name:        "scope",
            Description: "Scope of changes",
            Required:    true,
        },
        {
            Name:        "subject",
            Description: "Short description",
            Required:    true,
            MaxLength:   50,
        },
    },
    Rules: []commit.ValidationRule{
        {
            Type:    "max_length",
            Field:   "subject",
            Value:   50,
            Message: "Subject must be 50 characters or less",
        },
    },
}

// Use custom template
generator := commit.NewGenerator()
message, err := generator.Generate(ctx, repo, template)
```

### Working with Worktrees

```go
import "github.com/gizzahub/gzh-cli-git/pkg/branch"

mgr := branch.NewWorktreeManager()

// Create multiple worktrees for parallel development
worktrees := []struct {
    branch string
    path   string
}{
    {"feature/ui", "../ui-work"},
    {"feature/api", "../api-work"},
    {"feature/db", "../db-work"},
}

for _, wt := range worktrees {
    _, err := mgr.Add(ctx, repo, branch.AddOptions{
        Path:   wt.path,
        Branch: wt.branch,
    })
    if err != nil {
        log.Printf("Failed to create worktree %s: %v", wt.path, err)
    }
}

// List all worktrees
list, err := mgr.List(ctx, repo)
for _, w := range list {
    fmt.Printf("%s -> %s\n", w.Path, w.Branch)
}
```

### Batch Operations

```go
import (
    "github.com/gizzahub/gzh-cli-git/pkg/branch"
    "sync"
)

// Delete multiple merged branches in parallel
mgr := branch.NewManager()

// Get merged branches
branches, err := mgr.List(ctx, repo, branch.ListOptions{All: true})
merged := []string{}
for _, b := range branches {
    if b.IsMerged && !b.IsHead {
        merged = append(merged, b.Name)
    }
}

// Delete in parallel
var wg sync.WaitGroup
for _, name := range merged {
    wg.Add(1)
    go func(branchName string) {
        defer wg.Done()
        err := mgr.Delete(ctx, repo, branch.DeleteOptions{
            Name: branchName,
        })
        if err != nil {
            log.Printf("Failed to delete %s: %v", branchName, err)
        }
    }(name)
}
wg.Wait()
```

### Error Handling

```go
import (
    "errors"
    "github.com/gizzahub/gzh-cli-git/pkg/repository"
    "github.com/gizzahub/gzh-cli-git/pkg/merge"
)

// Check specific error types
repo, err := client.Open(ctx, "/path/to/repo")
if err != nil {
    if errors.Is(err, repository.ErrNotRepository) {
        fmt.Println("Not a Git repository")
    } else {
        fmt.Printf("Error: %v\n", err)
    }
    return
}

// Handle merge conflicts
result, err := mgr.Merge(ctx, repo, opts)
if err != nil {
    return err
}

if !result.Success {
    fmt.Println("Merge has conflicts:")
    for _, conflict := range result.Conflicts {
        fmt.Printf("  %s\n", conflict.FilePath)
        if conflict.AutoResolvable {
            fmt.Println("    (auto-resolvable)")
        }
    }
}
```

## Testing Your Integration

```go
import (
    "testing"
    "github.com/gizzahub/gzh-cli-git/pkg/repository"
)

func TestMyFunction(t *testing.T) {
    // Create test repository
    tmpDir := t.TempDir()

    client := repository.NewClient()
    repo, err := client.Init(context.Background(), repository.InitOptions{
        Path: tmpDir,
        Bare: false,
    })
    if err != nil {
        t.Fatal(err)
    }

    // Your test code here
    // ...
}
```

## Performance Considerations

### Use Context for Cancellation

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

stats, err := analyzer.Analyze(ctx, repo, opts)
```

### Limit Data Retrieval

```go
// Limit commits analyzed
stats, err := analyzer.Analyze(ctx, repo, history.AnalyzeOptions{
    Since: time.Now().AddDate(0, -3, 0), // Last 3 months only
})

// Limit file history
history, err := tracker.GetHistory(ctx, repo, "file.go", history.HistoryOptions{
    MaxCount: 100, // Only last 100 commits
})
```

### Reuse Executors

```go
// Create once, reuse multiple times
executor := gitcmd.NewExecutor()

analyzer := history.NewHistoryAnalyzer(executor)
detector := merge.NewConflictDetector(executor)
rebaseMgr := merge.NewRebaseManager(executor)
```

## See Also

- [API Documentation](https://pkg.go.dev/github.com/gizzahub/gzh-cli-git)
- [Examples](examples/)
- [Architecture](../ARCHITECTURE.md)
- [Contributing](../CONTRIBUTING.md)
