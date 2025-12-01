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

### ✅ Fully Implemented & Available (v0.1.0-alpha)

📦 **Repository Operations**
- Clone repositories with advanced options (branch, depth, single-branch, recursive)
- Check repository status (clean/dirty, modified/staged/untracked files)
- Get repository information (branch, remote, upstream, ahead/behind counts)
- Bulk operations (clone-or-update multiple repos in parallel)
- Flexible clone strategies (always-clone, update-if-exists, skip-if-exists)

🚀 **Commit Automation**
- Template-based commit messages (Conventional Commits support)
- Auto-generate commit messages from code changes
- Validate commit messages against templates
- Built-in template management (list, show, validate)

🌿 **Branch Management**
- Create, list, and delete branches
- Worktree-based parallel development
- Branch creation with linked worktrees
- Local and remote branch operations

📊 **History Analysis**
- Commit statistics and trends
- Contributor analysis with metrics
- File change tracking and history
- Multiple output formats (Table, JSON, CSV)

🔀 **Advanced Merge/Rebase**
- Pre-merge conflict detection
- Execute merge with multiple strategies
- Abort and rebase operations
- Interactive conflict assistance

📚 **Go Library API**
- Clean, stable public APIs (all `pkg/*` packages)
- Zero CLI dependencies in library code
- Context-aware operations (cancellation, timeouts)
- Easy integration into other Go projects
- Full implementations: `repository`, `operations`, `commit`, `branch`, `history`, `merge`

🔧 **Quality & Testing**
- 141 tests passing
- 69.1% code coverage
- Comprehensive integration tests
- Well-documented codebase

> **Note**: Despite the v0.1.0-alpha version number, this project has substantially more features than typically expected from an alpha release. See [IMPLEMENTATION_STATUS.md](docs/IMPLEMENTATION_STATUS.md) for details.

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

### Advanced Features Usage

**Commit Automation:**
```bash
# Auto-generate and create commit
gzh-git commit auto

# Validate commit message
gzh-git commit validate "feat(cli): add new command"

# List available templates
gzh-git commit template list

# Show template details
gzh-git commit template show conventional
```

**Branch & Worktree Management:**
```bash
# List all branches
gzh-git branch list --all

# Create new branch
gzh-git branch create feature/new-feature

# Create branch with worktree
gzh-git branch create feature/auth --worktree ~/work/auth

# Delete branch
gzh-git branch delete old-feature
```

**History Analysis:**
```bash
# Show commit statistics
gzh-git history stats --since "1 month ago"

# Analyze contributors
gzh-git history contributors --top 10

# View file history
gzh-git history file src/main.go
```

**Advanced Merge/Rebase:**
```bash
# Detect conflicts before merging
gzh-git merge detect feature/new-feature main

# Execute merge
gzh-git merge do feature/new-feature

# Abort merge if needed
gzh-git merge abort

# Rebase current branch
gzh-git merge rebase main
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

**Advanced Library Features:**

All major packages are fully implemented. See [Library Documentation](docs/LIBRARY.md) for comprehensive examples.

**Available Packages:**
- `pkg/repository` - Repository operations
- `pkg/operations` - Clone, pull, fetch operations
- `pkg/commit` - Commit automation and validation
- `pkg/branch` - Branch and worktree management
- `pkg/history` - History analysis and statistics
- `pkg/merge` - Merge and rebase operations

**For detailed examples, see:**
- [Library Guide](docs/LIBRARY.md) - Complete library documentation
- [examples/](examples/) directory - Working code samples
- [API Reference](https://pkg.go.dev/github.com/gizzahub/gzh-cli-git) - Full API documentation

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

- [x] **Phase 1-5**: Core Features *(Completed - v0.1.0-alpha)*
  - [x] Project structure and architecture
  - [x] Core documentation (PRD, REQUIREMENTS, ARCHITECTURE)
  - [x] Repository operations (clone, status, info, update)
  - [x] Commit automation (auto-commit, templates, validation)
  - [x] Branch management (create, delete, list, worktrees)
  - [x] History analysis (stats, contributors, file tracking)
  - [x] Advanced merge/rebase (conflict detection, strategies)
  - [x] Library-first architecture with full pkg/ implementations
  - [x] Test infrastructure (141 tests, 69.1% coverage)

- [ ] **Phase 6**: Documentation & Examples *(In Progress)*
  - [x] Implementation status report
  - [ ] Comprehensive usage examples
  - [ ] Complete API documentation
  - [ ] Video tutorials and guides
  - [ ] Migration guides from other tools

- [ ] **Phase 7**: Production Readiness *(Target: v1.0.0)*
  - [ ] 90%+ test coverage
  - [ ] Performance benchmarks and optimization
  - [ ] Security audit
  - [ ] API stability guarantees
  - [ ] Production deployment guides
  - [ ] Official release announcement

> **Note**: Phases 2-5 were completed during initial development but documented as "planned". See [IMPLEMENTATION_STATUS.md](docs/IMPLEMENTATION_STATUS.md) for details on this discrepancy.

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
