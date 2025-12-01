# gzh-cli-git Examples

This directory contains runnable examples demonstrating how to use the gzh-cli-git library in your Go applications.

## Quick Start

All examples can be run from the examples directory:

```bash
cd examples/<example-name>
go run main.go [arguments]
```

## Available Examples

### 1. Basic Operations (`basic/`)

Demonstrates basic repository operations: opening a repository, getting info, and checking status.

```bash
cd examples/basic
go run main.go
```

**What it does:**
- Opens the gzh-cli-git repository
- Displays branch, remote URL, and upstream information
- Shows working tree status (clean/dirty, file counts)

**API Demonstrated:**
- `repository.NewClient()`
- `client.Open(ctx, path)`
- `client.GetInfo(ctx, repo)`
- `client.GetStatus(ctx, repo)`

---

### 2. Clone Repository (`clone/`)

Demonstrates cloning a repository with advanced options.

```bash
cd examples/clone
go run main.go <repository-url> [destination]
```

**Examples:**
```bash
# Clone to default location (/tmp/cloned-repo)
go run main.go https://github.com/golang/example.git

# Clone to specific location
go run main.go https://github.com/golang/example.git /tmp/my-repo
```

**What it does:**
- Clones a repository with shallow clone (depth=1) for speed
- Uses single-branch mode to clone only the default branch
- Displays clone progress and repository information

**API Demonstrated:**
- `repository.CloneOptions`
- `client.Clone(ctx, options)`

---

### 3. Commit Automation (`commit/`)

Demonstrates commit message automation, validation, and templates.

```bash
cd examples/commit
go run main.go [repository-path]
```

**What it does:**
- Validates commit messages against Conventional Commits format
- Lists available commit message templates
- Shows template details
- Auto-generates commit messages from staged changes

**API Demonstrated:**
- `commit.NewManager()`
- `manager.ValidateMessage(ctx, repo, message)`
- `manager.ListTemplates(ctx)`
- `manager.GetTemplate(ctx, name)`
- `manager.AutoGenerateMessage(ctx, repo)`

---

### 4. Branch Management (`branch/`)

Demonstrates branch operations and worktree management.

```bash
cd examples/branch
go run main.go [repository-path]
```

**What it does:**
- Lists all branches (local and remote)
- Gets current branch information
- Checks branch existence
- Lists active worktrees
- Shows examples of branch creation

**API Demonstrated:**
- `branch.NewManager()`
- `manager.List(ctx, repo, options)`
- `manager.GetCurrent(ctx, repo)`
- `manager.Exists(ctx, repo, name)`
- `manager.ListWorktrees(ctx, repo)`

---

### 5. History Analysis (`history/`)

Demonstrates commit history analysis and statistics.

```bash
cd examples/history
go run main.go [repository-path]
```

**What it does:**
- Gets commit statistics (last 30 days)
- Analyzes top contributors
- Shows file history
- Lists recent commits

**API Demonstrated:**
- `history.NewAnalyzer()`
- `analyzer.GetStats(ctx, repo, options)`
- `analyzer.GetContributors(ctx, repo, options)`
- `analyzer.GetFileHistory(ctx, repo, options)`
- `analyzer.GetCommits(ctx, repo, options)`

---

### 6. Merge & Conflict Detection (`merge/`)

Demonstrates merge operations and pre-merge conflict detection.

```bash
cd examples/merge
go run main.go [repository-path]
```

**What it does:**
- Checks if merge is in progress
- Detects potential conflicts before merging
- Shows available merge strategies
- Demonstrates merge execution (example only, doesn't execute)
- Shows rebase operations

**API Demonstrated:**
- `merge.NewManager()`
- `manager.InProgress(ctx, repo)`
- `manager.DetectConflicts(ctx, repo, options)`
- `manager.Execute(ctx, repo, options)` (example)
- `manager.Rebase(ctx, repo, options)` (example)

---

## Running All Examples

You can test all examples from the project root:

```bash
# Basic operations (current repository)
cd examples/basic && go run main.go && cd ../..

# Clone example (requires internet)
cd examples/clone && go run main.go https://github.com/golang/example.git /tmp/test-clone && cd ../..

# Commit automation
cd examples/commit && go run main.go && cd ../..

# Branch management
cd examples/branch && go run main.go && cd ../..

# History analysis
cd examples/history && go run main.go && cd ../..

# Merge and conflict detection
cd examples/merge && go run main.go && cd ../..
```

## Building Examples

To build standalone binaries:

```bash
# Create bin directory
mkdir -p bin

# Build all examples
go build -o bin/basic examples/basic/main.go
go build -o bin/clone examples/clone/main.go
go build -o bin/commit examples/commit/main.go
go build -o bin/branch examples/branch/main.go
go build -o bin/history examples/history/main.go
go build -o bin/merge examples/merge/main.go
```

Then run them:

```bash
./bin/basic
./bin/commit .
./bin/branch .
./bin/history .
./bin/merge .
```

## Example Categories

### Repository Operations
- `basic/` - Core repository operations (open, info, status)
- `clone/` - Advanced cloning with options

### Commit Management
- `commit/` - Commit automation, validation, templates

### Branch & Worktree
- `branch/` - Branch management and worktrees

### Analysis & History
- `history/` - Commit statistics and contributor analysis

### Merge & Rebase
- `merge/` - Conflict detection and merge operations

## Learning More

### API Documentation
- [Repository Client API](../pkg/repository/interfaces.go)
- [Commit Manager API](../pkg/commit/interfaces.go)
- [Branch Manager API](../pkg/branch/interfaces.go)
- [History Analyzer API](../pkg/history/interfaces.go)
- [Merge Manager API](../pkg/merge/interfaces.go)
- [Complete GoDoc](https://pkg.go.dev/github.com/gizzahub/gzh-cli-git)

### User Documentation
- [Main README](../README.md)
- [Quick Start Guide](../docs/QUICKSTART.md)
- [Library Integration Guide](../docs/LIBRARY.md)
- [FAQ](../docs/user/guides/faq.md)

## Adding More Examples

Want to contribute an example? Create a new directory under `examples/` with:
- `main.go` - Your example code
- `README.md` - Description and usage (optional but recommended)

Make sure your example:
- Has a clear, focused purpose
- Includes comprehensive error handling
- Uses `context.Context` properly
- Has helpful output messages
- Demonstrates real-world usage
- Includes comments explaining key concepts
