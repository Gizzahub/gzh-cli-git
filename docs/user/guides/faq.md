# Frequently Asked Questions (FAQ)

Common questions and answers about gzh-cli-git.

## General Questions

### What is gzh-cli-git?

gzh-cli-git is a dual-purpose tool:
1. **CLI Application**: A standalone command-line tool for Git automation
2. **Go Library**: A reusable library for integrating Git operations into other Go projects

It's designed with a library-first architecture, meaning the core functionality is available as clean Go APIs without any CLI dependencies.

### Why use gzh-cli-git instead of standard Git?

gzh-cli-git doesn't replace Git—it enhances it:
- **Automation**: Auto-generate commit messages, detect conflicts before merging
- **Safety**: Smart push with pre-flight checks, conflict detection
- **Productivity**: Parallel development with worktrees, bulk repository operations
- **Library Integration**: Use Git operations programmatically in your Go applications

### What features are currently available?

**All Major Features Implemented (v0.2.0):**

✅ **Repository Operations**
- Clone with advanced options (branch, depth, single-branch, recursive)
- Status checking (clean/dirty, modified/staged/untracked files)
- Repository information (branch, remote, upstream, ahead/behind)
- Bulk clone-or-update operations

✅ **Commit Automation**
- Auto-generate commit messages from changes
- Template-based commits (Conventional Commits support)
- Commit message validation
- Template management (list, show, validate)

✅ **Branch & Worktree Management**
- Create, list, and delete branches
- Worktree-based parallel development
- Branch creation with linked worktrees

✅ **History Analysis**
- Commit statistics and trends
- Contributor analysis with metrics
- File change tracking and history
- Multiple output formats (Table, JSON, CSV)

✅ **Advanced Merge/Rebase**
- Pre-merge conflict detection
- Merge execution with strategies
- Abort and rebase operations

✅ **Go Library API**
- All 6 pkg/ packages fully implemented
- Clean APIs with zero CLI dependencies
- Context-aware operations

> **Note**: Version v0.2.0 accurately reflects feature completeness. See [IMPLEMENTATION_STATUS](../../IMPLEMENTATION_STATUS.md) for historical context.

## Installation & Setup

### How do I install gzh-git?

**Option 1: Using Go (Recommended)**
```bash
go install github.com/gizzahub/gzh-cli-git/cmd/gzh-git@latest
```

**Option 2: From Source**
```bash
git clone https://github.com/gizzahub/gzh-cli-git.git
cd gzh-cli-git
make build
sudo make install
```

See [Installation Guide](../INSTALL.md) for more options.

### What are the system requirements?

- **Go**: 1.24 or later (for building from source)
- **Git**: 2.30 or later (required for all operations)
- **OS**: Linux, macOS, or Windows

### Why does gzh-git require Git to be installed?

gzh-cli-git uses the Git CLI under the hood rather than reimplementing Git functionality. This approach provides:
- Maximum compatibility with all Git features
- Consistent behavior with standard Git
- Easier debugging (same commands you'd run manually)
- Simpler implementation and maintenance

### Command not found after installation

If you see "command not found: gzh-git":

1. **Check if Go bin is in PATH:**
   ```bash
   echo $PATH | grep go/bin
   ```

2. **Add Go bin to PATH:**
   ```bash
   # For bash
   echo 'export PATH=$PATH:$HOME/go/bin' >> ~/.bashrc
   source ~/.bashrc

   # For zsh
   echo 'export PATH=$PATH:$HOME/go/bin' >> ~/.zshrc
   source ~/.zshrc
   ```

3. **Verify installation:**
   ```bash
   which gzh-git
   gzh-git --version
   ```

## Usage Questions

### How do I check repository status?

```bash
# In current directory
gzh-git status

# Specific repository
gzh-git status /path/to/repo

# Quiet mode (exit code only)
gzh-git status -q
```

Exit codes:
- `0`: Repository is clean
- `1`: Repository has changes

### How do I clone a repository?

```bash
# Basic clone
gzh-git clone https://github.com/user/repo.git

# Clone specific branch
gzh-git clone -b develop https://github.com/user/repo.git

# Shallow clone (faster, saves disk space)
gzh-git clone --depth 1 https://github.com/user/repo.git

# Clone to specific directory
gzh-git clone https://github.com/user/repo.git my-project
```

### What's the difference between gzh-git and regular git?

For basic operations like `status` and `clone`, gzh-git provides:
- Cleaner, more structured output
- Additional validation and safety checks
- Better error messages
- Library API for programmatic access

Advanced features (coming soon) will add automation and intelligence not available in standard Git.

### Can I use gzh-git alongside regular git?

Yes! gzh-git works with standard Git repositories. You can:
- Use gzh-git for some operations
- Use regular git for others
- Mix both in the same workflow

They operate on the same `.git` directory and are fully compatible.

## Library Usage

### How do I use gzh-git as a library?

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
    client := repository.NewClient()

    // Open repository
    repo, err := client.Open(ctx, ".")
    if err != nil {
        log.Fatal(err)
    }

    // Get status
    status, err := client.GetStatus(ctx, repo)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Clean: %v\n", status.IsClean)
}
```

See [Library Integration Guide](../LIBRARY.md) for more examples.

### Is the library API stable?

Current status (v0.2.0):
- **All major APIs**: Implemented and functional
- **Core packages**: repository, operations, commit, branch, history, merge
- **Stability**: Pre-release - API may change before v1.0.0

API stability guarantees:
- Patch versions (0.1.x): No breaking changes
- Minor versions (0.x.0): May have breaking changes
- Major versions (v1.0.0+): Full stability guarantee

All packages work correctly with comprehensive test coverage (69.1%).

See [API Stability Policy](../API_STABILITY.md) for details.

### Can I use this in production?

**Current version (v0.2.0)**: Use with caution
- ✅ All major features implemented and functional
- ✅ Good test coverage (69.1%, 141 tests passing)
- ✅ Comprehensive error handling
- ⚠️ API may change before v1.0.0
- ⚠️ Pre-release version indicates ongoing stabilization

**Considerations:**
- **For personal/internal projects**: Generally safe to use
- **For production systems**: Wait for v1.0.0 or pin to specific version
- **For libraries**: Pin exact version to avoid breaking changes

**When fully production-ready (v1.0.0):**
- API stability guarantees
- 90%+ test coverage
- Security audit completion
- Production deployment guides

### How do I integrate with gzh-cli?

gzh-cli-git is designed to be the Git engine for [gzh-cli](https://github.com/gizzahub/gzh-cli):

```go
import "github.com/gizzahub/gzh-cli-git/pkg/repository"

// Use in gzh-cli commands
client := repository.NewClient()
repo, _ := client.Open(ctx, repoPath)
```

Full integration planned for v1.0.0 release.

## Troubleshooting

### "Not a git repository" error

This error means you're trying to run gzh-git in a directory that isn't a Git repository.

**Solution:**
```bash
# Check if current directory is a Git repo
git status

# Initialize new repository
git init

# Or navigate to existing repository
cd /path/to/git/repo
```

### Clone fails with "Permission denied"

**Possible causes:**
1. **SSH key not configured** (for SSH URLs)
2. **No access to repository** (private repo)
3. **Network issues**

**Solutions:**
```bash
# For SSH: Check SSH key
ssh -T git@github.com

# For HTTPS: Use personal access token
git clone https://username:token@github.com/user/repo.git

# Check network connectivity
ping github.com
```

### Performance: Why is gzh-git slower than git?

gzh-git adds a thin layer on top of Git for:
- Input validation
- Output parsing
- Safety checks

Overhead is typically 10-50ms. For large operations (clone, fetch), the overhead is negligible compared to network time.

**Optimization tips:**
- Use `--depth 1` for faster clones
- Use `-q` flag to reduce output processing
- For bulk operations, use library API with parallelization

### How do I report bugs or request features?

1. **Check existing issues**: [GitHub Issues](https://github.com/gizzahub/gzh-cli-git/issues)
2. **Search documentation**: [docs/](../)
3. **Create new issue**: Use issue templates

**Include in bug reports:**
- gzh-git version (`gzh-git --version`)
- Git version (`git --version`)
- Operating system
- Steps to reproduce
- Expected vs actual behavior

## Advanced Questions

### Does gzh-git support Git hooks?

Git hooks work normally with gzh-git since it uses standard Git repositories. Future versions may add hook automation features.

### Can I use custom commit templates?

Yes! gzh-git supports custom commit templates:

```bash
# List available templates
gzh-git commit template list

# Show template details
gzh-git commit template show conventional

# Use auto-commit (uses default template)
gzh-git commit auto
```

You can create custom templates in `~/.config/gzh-git/templates/` directory.

### What about submodules?

Submodule support is planned for future releases. Current version focuses on core repository operations.

### Is there a GUI or TUI?

Current version is CLI-only. Terminal UI (TUI) is being considered for future versions.

### How does gzh-git handle credentials?

gzh-git uses Git's credential system:
- SSH keys (via SSH agent)
- HTTPS credentials (via Git credential helper)
- Personal access tokens

No credentials are stored or logged by gzh-git.

### Can I extend gzh-git with plugins?

Plugin architecture is planned for v2.0+. Current version focuses on core stability.

## Project & Community

### Who maintains gzh-cli-git?

Currently maintained by the Gizzahub team as part of the gzh-cli ecosystem.

### How can I contribute?

See [Contributing Guide](../../CONTRIBUTING.md) for:
- Setting up development environment
- Code style and standards
- Pull request process
- Testing requirements

### What's the license?

MIT License - see [LICENSE](../../LICENSE) for details.

### Where can I get help?

- **Documentation**: [docs/](../)
- **GitHub Issues**: [Report bugs](https://github.com/gizzahub/gzh-cli-git/issues)
- **GitHub Discussions**: [Ask questions](https://github.com/gizzahub/gzh-cli-git/discussions)

## See Also

- [Quick Start Guide](../QUICKSTART.md)
- [Installation Guide](../INSTALL.md)
- [Troubleshooting Guide](../TROUBLESHOOTING.md)
- [Library Integration](../LIBRARY.md)
- [Command Reference](../../commands/README.md)
