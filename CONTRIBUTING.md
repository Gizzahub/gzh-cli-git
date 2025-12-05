# Contributing to gzh-cli-git

Thank you for your interest in contributing to **gzh-cli-git**! This guide will help you get started with development, testing, and submitting contributions.

______________________________________________________________________

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Project Structure](#project-structure)
- [Coding Standards](#coding-standards)
- [Testing Guidelines](#testing-guidelines)
- [Commit Convention](#commit-convention)
- [Pull Request Process](#pull-request-process)
- [Documentation](#documentation)
- [Release Process](#release-process)

______________________________________________________________________

## Code of Conduct

This project adheres to a code of conduct that all contributors are expected to follow:

- Be respectful and inclusive
- Focus on constructive feedback
- Accept differing viewpoints gracefully
- Prioritize project goals over personal preferences

______________________________________________________________________

## Getting Started

### Prerequisites

Before contributing, ensure you have the following installed:

- **Go 1.24+** - [Download Go](https://go.dev/dl/)
- **Git 2.30+** - Required for development and testing
- **Make** - For build automation
- **golangci-lint 1.55+** - For code linting

### Fork and Clone

1. Fork the repository on GitHub
1. Clone your fork locally:

```bash
git clone https://github.com/YOUR_USERNAME/gzh-cli-git.git
cd gzh-cli-git
```

3. Add the upstream remote:

```bash
git remote add upstream https://github.com/gizzahub/gzh-cli-git.git
```

### Install Dependencies

```bash
# Install Go dependencies
go mod download

# Verify setup
make test
```

______________________________________________________________________

## Development Workflow

### 1. Create a Feature Branch

Always create a new branch for your work:

```bash
git checkout -b feature/your-feature-name
# or
git checkout -b fix/your-bug-fix
```

**Branch Naming Convention:**

- `feature/*` - New features
- `fix/*` - Bug fixes
- `docs/*` - Documentation updates
- `refactor/*` - Code refactoring
- `test/*` - Test improvements
- `chore/*` - Maintenance tasks

### 2. Make Your Changes

- Write clean, well-documented code
- Follow the [Coding Standards](#coding-standards)
- Add tests for new functionality
- Update documentation as needed

### 3. Run Quality Checks

Before committing, ensure all quality checks pass:

```bash
# Run all quality checks (formatting, linting, tests)
make quality

# Or run individually:
make fmt        # Format code
make lint       # Run linters
make test       # Run all tests
make test-coverage  # Check coverage
```

### 4. Commit Your Changes

Follow our [Commit Convention](#commit-convention):

```bash
git add .
git commit -m "feat(commit): add auto-commit with template support"
```

### 5. Push and Create Pull Request

```bash
git push origin feature/your-feature-name
```

Then open a Pull Request on GitHub.

______________________________________________________________________

## Project Structure

Understanding the project structure will help you navigate the codebase:

```
gzh-cli-git/
├── pkg/                     # Public library API (importable by other projects)
│   ├── repository/          # Repository management
│   ├── commit/              # Commit automation
│   ├── branch/              # Branch management
│   ├── history/             # History analysis
│   └── merge/               # Merge/rebase operations
│
├── internal/                # Internal implementation (not importable)
│   ├── gitcmd/              # Git command execution & security
│   └── parser/              # Output parsing
│
├── cmd/gzh-git/             # CLI application
│   └── cmd/                 # CLI commands (Cobra)
│
├── tests/                   # Test suites
│   ├── integration/         # Integration tests
│   └── e2e/                 # End-to-end tests
│
├── benchmarks/              # Performance benchmarks
├── examples/                # Usage examples
├── docs/                    # User documentation
└── specs/                   # Feature specifications
```

### Key Principles

1. **Library-First Design**: `pkg/` has ZERO CLI dependencies
1. **Interface-Driven**: All components use well-defined interfaces
1. **Context Propagation**: All operations support `context.Context`
1. **Testability**: 100% mockable components

______________________________________________________________________

## Coding Standards

### Go Code Style

Follow standard Go conventions:

```bash
# Format code automatically
make fmt

# or manually
gofmt -s -w .
goimports -w .
```

### Linting

We use `golangci-lint` with strict settings:

```bash
make lint
```

**Key rules:**

- No unused variables or imports
- Error handling required
- Cyclomatic complexity < 15
- Function length < 60 lines (where reasonable)

### Code Organization

**Package Layout:**

```go
// 1. Package declaration and doc comment
// Package commit provides commit message automation.
package commit

// 2. Imports (grouped: stdlib, external, internal)
import (
    "context"
    "fmt"

    "github.com/spf13/cobra"

    "github.com/gizzahub/gzh-cli-git/internal/gitcmd"
)

// 3. Constants and variables
const (
    DefaultTemplate = "conventional"
)

// 4. Type definitions
type Manager struct {
    executor gitcmd.Executor
}

// 5. Constructor functions
func NewManager(executor gitcmd.Executor) *Manager {
    return &Manager{executor: executor}
}

// 6. Methods (grouped by receiver)
func (m *Manager) AutoCommit(ctx context.Context, opts AutoCommitOptions) error {
    // Implementation
}
```

### Naming Conventions

- **Packages**: Short, lowercase, singular (`commit`, not `commits`)
- **Files**: Lowercase with underscores (`template_manager.go`)
- **Types**: PascalCase (`TemplateManager`)
- **Functions/Methods**: PascalCase for exported, camelCase for private
- **Constants**: PascalCase or UPPER_SNAKE_CASE for package-level

### Error Handling

Always handle errors explicitly:

```go
// ❌ Bad
result, _ := doSomething()

// ✅ Good
result, err := doSomething()
if err != nil {
    return fmt.Errorf("failed to do something: %w", err)
}
```

**Use `fmt.Errorf` with `%w` for error wrapping:**

```go
if err := validateInput(input); err != nil {
    return fmt.Errorf("input validation failed: %w", err)
}
```

### Context Usage

All long-running operations must accept `context.Context`:

```go
func (m *Manager) Clone(ctx context.Context, opts CloneOptions) (*Repository, error) {
    // Check context cancellation
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    // Perform operation with context
    return m.executor.ExecuteContext(ctx, "clone", opts.URL)
}
```

### Documentation

**Public APIs require documentation:**

```go
// Manager handles commit message automation and validation.
// It provides methods for generating, validating, and creating commits
// with template-based message formatting.
type Manager struct {
    executor gitcmd.Executor
}

// NewManager creates a new commit Manager with the provided executor.
// The executor is used to run git commands and should be properly
// configured with security settings.
func NewManager(executor gitcmd.Executor) *Manager {
    return &Manager{executor: executor}
}

// AutoCommit generates a commit message from staged changes and creates
// a commit. It analyzes the diff, infers the commit type and scope,
// and applies the specified template.
//
// Options:
//   - Template: Name of template to use (default: "conventional")
//   - AllowEmpty: Allow commits with no changes (default: false)
//   - NoVerify: Skip pre-commit hooks (default: false)
//
// Returns the commit hash on success or an error if validation fails.
func (m *Manager) AutoCommit(ctx context.Context, opts AutoCommitOptions) (*CommitResult, error) {
    // Implementation
}
```

______________________________________________________________________

## Testing Guidelines

### Test Coverage Targets

- **Overall**: 85% minimum
- **pkg/**: 85% minimum
- **internal/**: 80% minimum
- **Security-critical code**: 100% required

Current coverage: **69.1%** - See [docs/COVERAGE.md](docs/COVERAGE.md)

### Running Tests

```bash
# All tests
make test

# Unit tests only
make test-unit

# Integration tests (requires Git)
make test-integration

# E2E tests
make test-e2e

# With coverage report
make test-coverage

# View HTML coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Writing Tests

**Test File Naming:**

- Unit tests: `<file>_test.go` in same package
- Integration tests: `tests/integration/<feature>_test.go`
- E2E tests: `tests/e2e/<workflow>_test.go`

**Test Structure:**

```go
func TestManager_AutoCommit(t *testing.T) {
    tests := []struct {
        name    string
        setup   func(t *testing.T) *Manager
        opts    AutoCommitOptions
        want    *CommitResult
        wantErr bool
    }{
        {
            name: "success with conventional template",
            setup: func(t *testing.T) *Manager {
                executor := &mockExecutor{
                    // Mock setup
                }
                return NewManager(executor)
            },
            opts: AutoCommitOptions{
                Template: "conventional",
            },
            want: &CommitResult{
                Hash: "abc123",
            },
            wantErr: false,
        },
        // More test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mgr := tt.setup(t)
            got, err := mgr.AutoCommit(context.Background(), tt.opts)

            if (err != nil) != tt.wantErr {
                t.Errorf("AutoCommit() error = %v, wantErr %v", err, tt.wantErr)
                return
            }

            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("AutoCommit() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Test Helpers

Use `t.Helper()` for helper functions:

```go
func setupTestRepo(t *testing.T) *Repository {
    t.Helper()

    tempDir := t.TempDir()
    // Setup repository
    return repo
}
```

### Integration Tests

Integration tests should:

- Use temporary directories (`t.TempDir()`)
- Clean up resources
- Be independent (no shared state)
- Test real Git operations

### Benchmarks

Add benchmarks for performance-critical code:

```go
func BenchmarkAutoCommit(b *testing.B) {
    mgr := setupManager(b)
    opts := AutoCommitOptions{Template: "conventional"}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := mgr.AutoCommit(context.Background(), opts)
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

______________________________________________________________________

## Commit Convention

We follow [Conventional Commits](https://www.conventionalcommits.org/) specification:

### Format

```
<type>(<scope>): <subject>

<body>

<footer>
```

### Type

- **feat**: New feature
- **fix**: Bug fix
- **docs**: Documentation changes
- **refactor**: Code refactoring (no behavior change)
- **test**: Test additions or improvements
- **chore**: Build process, dependencies, tooling
- **perf**: Performance improvements
- **ci**: CI/CD configuration changes

### Scope

The scope should indicate the affected component:

- `repository` - Repository operations
- `commit` - Commit automation
- `branch` - Branch management
- `history` - History analysis
- `merge` - Merge/rebase operations
- `cli` - CLI commands
- `docs` - Documentation
- `deps` - Dependencies
- `cmd` - CLI implementation

### Examples

**Feature:**

```
feat(commit): add auto-commit with template support

Implement auto-commit functionality that:
- Analyzes staged changes using git diff
- Infers commit type and scope automatically
- Generates message from template
- Validates message format

Closes #123
```

**Bug Fix:**

```
fix(merge): prevent panic on nil conflict resolution

Add nil check before accessing conflict resolution result.
Fixes issue where unresolved conflicts caused panic.

Fixes #456
```

**Documentation:**

```
docs(readme): update installation instructions

Add Homebrew installation option and clarify
prerequisites for building from source.
```

**Refactoring:**

```
refactor(parser): simplify diff parsing logic

Extract common parsing logic into helper functions
to reduce code duplication and improve readability.
No behavior changes.
```

### Breaking Changes

Mark breaking changes clearly:

```
feat(api): change signature of AutoCommit method

BREAKING CHANGE: AutoCommit now returns (*CommitResult, error)
instead of (string, error) to provide more commit information.

Migration guide:
- Old: hash, err := mgr.AutoCommit(ctx, opts)
- New: result, err := mgr.AutoCommit(ctx, opts); hash := result.Hash
```

______________________________________________________________________

## Pull Request Process

### Before Submitting

1. **Sync with upstream:**

   ```bash
   git fetch upstream
   git rebase upstream/master
   ```

1. **Run quality checks:**

   ```bash
   make quality
   ```

1. **Update documentation** if needed

1. **Add tests** for new functionality

### PR Description Template

```markdown
## Description
Brief description of what this PR does.

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Related Issues
Closes #123

## Testing
Describe how you tested this change:
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] Manual testing performed

## Checklist
- [ ] Code follows project style guidelines
- [ ] Self-review completed
- [ ] Comments added for complex logic
- [ ] Documentation updated
- [ ] Tests added/updated
- [ ] All tests passing
- [ ] No new warnings from linters
```

### Review Process

1. **Automated checks** must pass (CI/CD)
1. **At least one reviewer** approval required
1. **Maintainer review** for significant changes
1. **Address feedback** promptly and professionally

### Merging

- Maintainers will merge approved PRs
- We use **squash and merge** for clean history
- Your commits will be squashed into one commit

______________________________________________________________________

## Documentation

### Types of Documentation

1. **Code Documentation** - GoDoc comments
1. **User Documentation** - `docs/` directory
1. **API Reference** - pkg.go.dev (auto-generated)
1. **Specifications** - `specs/` directory

### Writing Documentation

**User Documentation:**

- Write in Markdown
- Include examples with code snippets
- Keep it concise and practical
- Use proper headings and structure

**GoDoc Comments:**

- Start with the name being documented
- Be clear and concise
- Include examples for complex APIs
- Document parameters and return values

**Example:**

```go
// AutoCommit generates a commit message from staged changes and creates a commit.
//
// It performs the following steps:
//  1. Retrieves staged changes using git diff
//  2. Analyzes changes to infer commit type and scope
//  3. Generates message using the specified template
//  4. Validates the generated message
//  5. Creates the commit
//
// Example:
//
//	mgr := commit.NewManager(executor)
//	result, err := mgr.AutoCommit(ctx, commit.AutoCommitOptions{
//	    Template: "conventional",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println("Created commit:", result.Hash)
//
// Returns an error if there are no staged changes, validation fails,
// or the commit operation fails.
func (m *Manager) AutoCommit(ctx context.Context, opts AutoCommitOptions) (*CommitResult, error) {
    // Implementation
}
```

______________________________________________________________________

## Release Process

### Versioning

We use [Semantic Versioning](https://semver.org/):

- **Major** (v1.0.0): Breaking changes
- **Minor** (v0.1.0): New features (backward compatible)
- **Patch** (v0.0.1): Bug fixes (backward compatible)

### Release Checklist

Maintainers will:

1. Update version in relevant files
1. Update CHANGELOG.md
1. Tag release: `git tag -a v0.2.0 -m "Release v0.2.0"`
1. Push tag: `git push origin v0.2.0`
1. Create GitHub release with notes
1. Publish to pkg.go.dev (automatic)

______________________________________________________________________

## Getting Help

### Questions and Support

- **Documentation**: Check [docs/](docs/) first
- **Issues**: Search existing [GitHub Issues](https://github.com/gizzahub/gzh-cli-git/issues)
- **Discussions**: Use [GitHub Discussions](https://github.com/gizzahub/gzh-cli-git/discussions)
- **Chat**: Join our community chat (coming soon)

### Reporting Bugs

When reporting bugs, include:

1. **Environment**: Go version, OS, Git version
1. **Steps to reproduce**: Minimal example
1. **Expected behavior**: What should happen
1. **Actual behavior**: What actually happens
1. **Logs**: Relevant error messages or logs

### Feature Requests

For feature requests:

1. Check existing issues first
1. Describe the use case clearly
1. Explain why it's valuable
1. Consider implementation complexity
1. Be open to alternative solutions

______________________________________________________________________

## Project Conventions

### File Size Limits

Keep files manageable:

- **Ideal**: < 500 lines
- **Maximum**: < 1000 lines
- **If larger**: Split into multiple files

### Directory Structure

Follow these conventions:

- `pkg/` - Public library API only
- `internal/` - Internal implementation
- `cmd/` - CLI application
- `tests/` - All test suites
- `docs/` - User documentation
- `specs/` - Technical specifications
- `examples/` - Working code examples

### Build Artifacts

Never commit build artifacts:

- Binaries: `gzh-git`, `*.exe`
- Coverage: `coverage.out`, `coverage.html`
- Temporary: `tmp/`, `*.tmp`

These are in `.gitignore` - if you need to add new artifacts, update `.gitignore` first.

______________________________________________________________________

## Recognition

Contributors are recognized in:

- GitHub contributors page
- CHANGELOG.md (for significant contributions)
- Release notes

______________________________________________________________________

## License

By contributing to gzh-cli-git, you agree that your contributions will be licensed under the MIT License.

______________________________________________________________________

## Thank You!

Thank you for contributing to **gzh-cli-git**! Your efforts help make this project better for everyone.

**Questions?** Feel free to ask in [GitHub Discussions](https://github.com/gizzahub/gzh-cli-git/discussions).

______________________________________________________________________

**Last Updated**: 2025-11-29
