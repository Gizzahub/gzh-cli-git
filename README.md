# gzh-cli-git

> Advanced Git automation CLI and Go library for developers

[![Go Version](https://img.shields.io/badge/go-1.24.0%2B-blue)](https://go.dev)
[![Version](https://img.shields.io/badge/version-v0.1.0--alpha-orange)](https://github.com/gizzahub/gzh-cli-git/releases/tag/v0.1.0-alpha)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Test Coverage](https://img.shields.io/badge/coverage-69.1%25-yellow)](docs/COVERAGE.md)
[![Tests](https://img.shields.io/badge/tests-141%20passing-brightgreen)](#testing)
[![GoDoc](https://pkg.go.dev/badge/github.com/gizzahub/gzh-cli-git.svg)](https://pkg.go.dev/github.com/gizzahub/gzh-cli-git)

**gzh-cli-git** is a Git-specialized CLI tool and Go library that provides advanced Git automation capabilities. It serves dual purposes: a powerful standalone CLI for developers and a reusable library for embedding in other Go projects.

---

## Features

### ✅ Currently Available (v0.1.0-alpha)

📦 **Repository Operations**
- Clone repositories with advanced options (branch, depth, single-branch, recursive)
- Check repository status (clean/dirty, modified/staged/untracked files)
- Get repository information (branch, remote, upstream, ahead/behind counts)
- Validate repository existence

📚 **Go Library API**
- Clean, stable public APIs (`pkg/repository`, `pkg/operations`)
- Zero CLI dependencies in library code
- Context-aware operations (cancellation, timeouts)
- Easy integration into other Go projects

🔧 **Developer Tools**
- Bulk repository operations (clone-or-update multiple repos in parallel)
- Flexible clone strategies (always-clone, update-if-exists, skip-if-exists)
- Comprehensive test coverage (141 tests, 69.1% coverage)

### ⏳ Planned Features (Coming Soon)

🚀 **Commit Automation** *(v0.2.0)*
- Template-based commit messages (Conventional Commits, Semantic Versioning)
- Auto-generated messages from code changes
- Smart push with safety checks

🌿 **Branch Management** *(v0.3.0)*
- Worktree-based parallel development
- Branch naming validation
- Automated cleanup of merged branches

📊 **History Analysis** *(v0.4.0)*
- Commit statistics and contributor insights
- File change tracking
- Multiple output formats (Table, JSON, CSV)

🔀 **Advanced Merge/Rebase** *(v0.5.0)*
- Pre-merge conflict detection
- Auto-resolution with multiple strategies
- Interactive assistance

See [Roadmap](#roadmap) for detailed timeline and [FAQ](docs/user/guides/faq.md) for more information.

---

## Quick Start

### Installation

**Via Go Install:**
```bash
go install github.com/gizzahub/gzh-cli-git/cmd/gzh-git@latest
```

**Via Homebrew (macOS/Linux):**
```bash
brew install gzh-git  # Coming soon
```

**From Source:**
```bash
git clone https://github.com/gizzahub/gzh-cli-git.git
cd gzh-cli-git
make build
make install
```

### Requirements

- Git 2.30+
- Go 1.24+ (for building from source)

---

## Usage

### As CLI Tool

**Check Repository Status:**
```bash
# Show working tree status
gzh-git status

# Show status for specific repository
gzh-git status /path/to/repo

# Quiet mode (exit code 1 if dirty)
gzh-git status -q
```

**View Repository Information:**
```bash
# Show detailed repository information
gzh-git info

# Displays: branch, remote URL, upstream, ahead/behind counts, dirty/clean status
gzh-git info /path/to/repo
```

**Clone Repositories:**
```bash
# Basic clone
gzh-git clone https://github.com/user/repo.git

# Clone specific branch
gzh-git clone -b develop https://github.com/user/repo.git

# Shallow clone (faster)
gzh-git clone --depth 1 https://github.com/user/repo.git

# Clone with submodules
gzh-git clone --recursive https://github.com/user/repo.git

# Clone to specific directory
gzh-git clone https://github.com/user/repo.git my-project
```

**Global Options:**
```bash
# Verbose output
gzh-git -v status

# Quiet mode (errors only)
gzh-git -q clone https://github.com/user/repo.git

# Show version
gzh-git --version

# Show help
gzh-git --help
```

---

### 🔮 Usage Examples for Planned Features

These features are not yet implemented. See [Roadmap](#roadmap) for timeline.

**Commit Automation** *(Coming in v0.2.0):*
```bash
# Use conventional commits template
gzh-git commit --template conventional --type feat --scope cli

# Auto-generate commit message from changes
gzh-git commit --auto

# Smart push with safety checks
gzh-git push --smart
```

**Branch & Worktree Management** *(Coming in v0.3.0):*
```bash
# Create worktree for parallel development
gzh-git worktree add ~/work/feature-auth feature/auth

# Clean up merged branches
gzh-git branch cleanup --merged --dry-run
```

**History Analysis** *(Coming in v0.4.0):*
```bash
# Commit statistics
gzh-git stats commits --since 2025-01-01 --format table

# Contributor analysis
gzh-git stats contributors --top 10

# File history
gzh-git history file src/main.go
```

**Advanced Merge/Rebase** *(Coming in v0.5.0):*
```bash
# Detect conflicts before merging
gzh-git merge --detect-conflicts feature/auth

# Auto-resolve conflicts with strategy
gzh-git merge --auto-resolve feature/auth --strategy theirs

# Interactive rebase assistance
gzh-git rebase --interactive main --assistant
```

### As Go Library

**Basic Repository Operations:**
```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/gizzahub/gzh-cli-git/pkg/repository"
)

func main() {
    ctx := context.Background()

    // Create repository client
    client := repository.NewClient()

    // Open repository
    repo, err := client.Open(ctx, ".")
    if err != nil {
        log.Fatalf("Failed to open repository: %v", err)
    }

    // Get repository info
    info, err := client.GetInfo(ctx, repo)
    if err != nil {
        log.Fatalf("Failed to get info: %v", err)
    }

    fmt.Printf("Repository: %s\n", repo.Path)
    fmt.Printf("Branch: %s\n", info.Branch)
    fmt.Printf("Remote URL: %s\n", info.RemoteURL)

    // Get repository status
    status, err := client.GetStatus(ctx, repo)
    if err != nil {
        log.Fatalf("Failed to get status: %v", err)
    }

    fmt.Printf("Clean: %v\n", status.IsClean)
    fmt.Printf("Modified files: %d\n", len(status.ModifiedFiles))
    fmt.Printf("Staged files: %d\n", len(status.StagedFiles))
}
```

**Clone Repository:**
```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/gizzahub/gzh-cli-git/pkg/repository"
)

func main() {
    ctx := context.Background()

    // Create repository client
    client := repository.NewClient()

    // Clone repository with options
    repo, err := client.Clone(ctx, repository.CloneOptions{
        URL:          "https://github.com/user/repo.git",
        Destination:  "/tmp/cloned-repo",
        Branch:       "main",
        Depth:        1,
        SingleBranch: true,
    })

    if err != nil {
        log.Fatalf("Failed to clone: %v", err)
    }

    fmt.Printf("Cloned to: %s\n", repo.Path)
}
```

**🔮 Planned Library Features**

These features are not yet implemented. See current library documentation at [docs/LIBRARY.md](docs/LIBRARY.md).

**Commit Automation** *(v0.2.0):*
```go
// Coming soon
package main

import (
    "context"
    "github.com/gizzahub/gzh-cli-git/pkg/repository"
    "github.com/gizzahub/gzh-cli-git/pkg/commit"
)

func main() {
    ctx := context.Background()
    repoClient := repository.NewClient()
    commitMgr := commit.NewManager()

    repo, _ := repoClient.Open(ctx, ".")

    // Auto-commit with smart message generation
    result, err := commitMgr.AutoCommit(ctx, repo, commit.AutoCommitOptions{
        MessageFormat: "conventional",
        Template:      "feat",
    })

    if err != nil {
        panic(err)
    }

    fmt.Printf("Committed: %s\n", result.Hash)
}
```

**For more examples, see [examples/](examples/) directory.**

---

## Documentation

- 📖 [User Guide](docs/00-overview/README.md)
- 🏗️ [Architecture Design](ARCHITECTURE.md)
- 📋 [Product Requirements](PRD.md)
- 🔧 [Technical Requirements](REQUIREMENTS.md)
- 📚 [API Reference](https://pkg.go.dev/github.com/gizzahub/gzh-cli-git)
- 🤝 [Contributing Guide](CONTRIBUTING.md)

### Feature Specifications

- [Commit Automation](specs/10-commit-automation.md) *(coming soon)*
- [Branch Management](specs/20-branch-management.md) *(coming soon)*
- [History Analysis](specs/30-history-analysis.md) *(coming soon)*
- [Advanced Merge/Rebase](specs/40-advanced-merge.md) *(coming soon)*

---

## Project Status

**Current Version**: v0.1.0-alpha
**Status**: Active Development

### Roadmap

- [x] **Phase 1**: Foundation & Infrastructure *(Completed)*
  - [x] Project structure
  - [x] Core documentation (PRD, REQUIREMENTS, ARCHITECTURE)
  - [x] Basic Git operations (clone, status, info)
  - [x] Test infrastructure (141 tests, 69.1% coverage)
  - [x] Library-first architecture
  - [x] Released: **v0.1.0-alpha** ✅

- [ ] **Phase 2**: Commit Automation *(v0.2.0 - Planned)*
  - [ ] Template system
  - [ ] Auto-commit with smart message generation
  - [ ] Smart push with safety checks
  - [ ] Commit message validation

- [ ] **Phase 3**: Branch Management *(v0.3.0 - Planned)*
  - [ ] Branch operations (create, delete, list)
  - [ ] Worktree management
  - [ ] Parallel development workflows
  - [ ] Branch cleanup automation

- [ ] **Phase 4**: History Analysis *(v0.4.0 - Planned)*
  - [ ] Commit statistics and trends
  - [ ] Contributor analysis
  - [ ] File history tracking
  - [ ] Multiple output formats (Table, JSON, CSV)

- [ ] **Phase 5**: Advanced Merge/Rebase *(v0.5.0 - Planned)*
  - [ ] Pre-merge conflict detection
  - [ ] Auto-resolution strategies
  - [ ] Interactive merge assistance
  - [ ] Rebase workflow support

- [ ] **Phase 6**: Production Readiness *(v1.0.0 - Target)*
  - [ ] 90%+ test coverage
  - [ ] Performance optimization
  - [ ] Complete documentation
  - [ ] API stability guarantees
  - [ ] Production-grade error handling

**See full roadmap in [PRD.md](PRD.md)**

---

## Architecture

```
┌─────────────────────────────────────────┐
│        gzh-cli-git Architecture          │
├─────────────────────────────────────────┤
│                                          │
│  ┌────────────────────────────────────┐ │
│  │    CLI Layer (cmd/)                 │ │
│  │  - Commands (Cobra)                 │ │
│  │  - Output Formatting                │ │
│  │  - User Interface                   │ │
│  └────────────┬───────────────────────┘ │
│               │                          │
│  ┌────────────▼───────────────────────┐ │
│  │  Public Library API (pkg/)          │ │
│  │  - Repository, Commit, Branch       │ │
│  │  - History, Merge Managers          │ │
│  │  - ZERO CLI dependencies           │ │
│  └────────────┬───────────────────────┘ │
│               │                          │
│  ┌────────────▼───────────────────────┐ │
│  │  Internal Implementation            │ │
│  │  - Git Command Executor             │ │
│  │  - Output Parsers                   │ │
│  │  - Input Validation                 │ │
│  └────────────┬───────────────────────┘ │
│               │                          │
│  ┌────────────▼───────────────────────┐ │
│  │  External Dependencies              │ │
│  │  - Git CLI (2.30+)                  │ │
│  │  - Filesystem                       │ │
│  └─────────────────────────────────────┘ │
│                                          │
└─────────────────────────────────────────┘
```

**Key Principles:**
- **Library-First**: Clean APIs with zero CLI dependencies
- **Interface-Driven**: All components via well-defined interfaces
- **Context Propagation**: Cancellation and timeout support
- **Testability**: 100% mockable components

**See detailed architecture in [ARCHITECTURE.md](ARCHITECTURE.md)**

---

## Development

### Prerequisites

- Go 1.24+
- Git 2.30+
- Make
- golangci-lint 1.55+

### Build

```bash
# Install dependencies
go mod download

# Build binary
make build

# Run tests
make test

# Run linters
make lint

# Format code
make fmt

# All quality checks
make quality
```

### Testing

```bash
# Unit tests only
make test-unit

# Integration tests (requires Git)
make test-integration

# E2E tests
make test-e2e

# Test coverage
make test-coverage

# Benchmarks
make bench
```

### Project Structure

```
gzh-cli-git/
├── pkg/                  # Public library API
├── internal/             # Internal implementation
├── cmd/gzh-git/          # CLI application
├── examples/             # Usage examples
├── test/                 # Integration & E2E tests
├── docs/                 # User documentation
├── specs/                # Feature specifications
└── .make/                # Modular Makefiles
```

---

## Integration with gzh-cli

This project is designed to be the Git engine for [gzh-cli](https://github.com/gizzahub/gzh-cli), a unified CLI for developers.

**Usage in gzh-cli:**

```go
import "github.com/gizzahub/gzh-cli-git/pkg/repository"

// gzh-cli can now leverage all gzh-cli-git functionality
client := repository.NewClient(logger)
repo, _ := client.Open(ctx, repoPath)
```

---

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines.

### Development Workflow

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests and linters (`make quality`)
5. Commit using conventional commits
6. Push to your fork
7. Open a Pull Request

### Commit Convention

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types**: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `perf`, `ci`

**Example:**
```
feat(commit): add auto-commit with template support

Implement auto-commit functionality that:
- Analyzes staged changes
- Generates commit message from template
- Validates message format

Closes #123
```

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## Acknowledgments

- Inspired by [gzh-cli](https://github.com/gizzahub/gzh-cli)
- Built with [Cobra](https://github.com/spf13/cobra) CLI framework
- Follows [Conventional Commits](https://www.conventionalcommits.org/) specification

---

## Support

- 📧 Email: support@gizzahub.com *(example)*
- 🐛 Issues: [GitHub Issues](https://github.com/gizzahub/gzh-cli-git/issues)
- 💬 Discussions: [GitHub Discussions](https://github.com/gizzahub/gzh-cli-git/discussions)

---

<p align="center">
  Made with ❤️ by the Gizzahub team
</p>
