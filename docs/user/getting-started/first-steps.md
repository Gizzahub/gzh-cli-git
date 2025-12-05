# First Steps with gzh-git

Welcome! This tutorial will guide you through your first 10 minutes with gzh-git.

## What You'll Learn

By the end of this tutorial, you'll know how to:

- ✅ Install gzh-git
- ✅ Clone a repository
- ✅ Check repository status
- ✅ View repository information
- ✅ Use gzh-git as a Go library

**Time Required**: ~10 minutes

**Prerequisites**:

- Git 2.30+ installed
- Go 1.24+ installed (optional, for library usage)

______________________________________________________________________

## Step 1: Install gzh-git (2 minutes)

### Quick Install

```bash
go install github.com/gizzahub/gzh-cli-git/cmd/gzh-git@latest
```

### Verify Installation

```bash
gzh-git --version
```

**Expected Output**:

```
gzh-git version v0.1.0-alpha
```

**Troubleshooting**: If you see "command not found", add Go's bin directory to your PATH:

```bash
# For bash
echo 'export PATH=$PATH:$HOME/go/bin' >> ~/.bashrc
source ~/.bashrc

# For zsh
echo 'export PATH=$PATH:$HOME/go/bin' >> ~/.zshrc
source ~/.zshrc
```

______________________________________________________________________

## Step 2: Clone Your First Repository (3 minutes)

Let's clone a public repository to practice with.

### Basic Clone

```bash
# Clone to current directory
gzh-git clone https://github.com/gizzahub/gzh-cli-git.git

# Or clone to specific directory
gzh-git clone https://github.com/gizzahub/gzh-cli-git.git my-test-repo
```

**What happens:**

1. gzh-git validates the URL
1. Creates the destination directory
1. Clones the repository
1. Shows progress information

**Expected Output**:

```
Cloning into 'gzh-cli-git'...
✓ Repository cloned successfully
Path: /current/directory/gzh-cli-git
```

### Advanced Clone Options

Try these variations:

**Clone specific branch:**

```bash
gzh-git clone -b develop https://github.com/user/repo.git
```

**Shallow clone (faster, saves space):**

```bash
gzh-git clone --depth 1 https://github.com/user/repo.git
```

**Clone only one branch:**

```bash
gzh-git clone --single-branch -b main https://github.com/user/repo.git
```

______________________________________________________________________

## Step 3: Check Repository Status (2 minutes)

Navigate into your cloned repository:

```bash
cd gzh-cli-git  # or my-test-repo
```

### View Status

```bash
gzh-git status
```

**Example Output (Clean Repository)**:

```
Repository Status
=================

Branch: main
Upstream: origin/main
Status: Clean ✓

No changes detected.
```

### Make Some Changes

Let's create a test file to see how status changes:

```bash
echo "# Test File" > test.md
```

Now check status again:

```bash
gzh-git status
```

**Example Output (With Changes)**:

```
Repository Status
=================

Branch: main
Upstream: origin/main
Status: Dirty ✗

Untracked Files: 1
  - test.md
```

### Quiet Mode

Use `-q` flag for scripts (exits with code 1 if repository is dirty):

```bash
gzh-git status -q
echo $?  # Prints exit code (1 = dirty, 0 = clean)
```

______________________________________________________________________

## Step 4: View Repository Information (1 minute)

Get detailed information about your repository:

```bash
gzh-git info
```

**Example Output**:

```
Repository Information
=====================

Path: /Users/you/projects/gzh-cli-git
Git Directory: /Users/you/projects/gzh-cli-git/.git

Branch: main
Remote URL: https://github.com/gizzahub/gzh-cli-git.git
Upstream: origin/main

Ahead: 0 commits
Behind: 0 commits

Status: Dirty (1 untracked file)
```

**What this tells you:**

- Current branch name
- Remote repository URL
- How many commits you're ahead/behind
- Repository cleanliness status

______________________________________________________________________

## Step 5: Use gzh-git as a Library (Optional, 3 minutes)

If you're a Go developer, you can use gzh-git in your own projects.

### Create a Test Project

```bash
mkdir gzh-test
cd gzh-test
go mod init example.com/gzh-test
```

### Install Library

```bash
go get github.com/gizzahub/gzh-cli-git
```

### Write Simple Code

Create `main.go`:

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

    // Open current directory
    repo, err := client.Open(ctx, ".")
    if err != nil {
        log.Fatal(err)
    }

    // Get repository info
    info, err := client.GetInfo(ctx, repo)
    if err != nil {
        log.Fatal(err)
    }

    // Print information
    fmt.Printf("Repository: %s\n", repo.Path)
    fmt.Printf("Branch: %s\n", info.Branch)
    fmt.Printf("Remote: %s\n", info.RemoteURL)

    // Get status
    status, err := client.GetStatus(ctx, repo)
    if err != nil {
        log.Fatal(err)
    }

    if status.IsClean {
        fmt.Println("Status: Clean ✓")
    } else {
        fmt.Printf("Status: Dirty ✗ (%d modified, %d staged, %d untracked)\n",
            len(status.ModifiedFiles),
            len(status.StagedFiles),
            len(status.UntrackedFiles))
    }
}
```

### Run Your Code

```bash
go run main.go
```

**Expected Output**:

```
Repository: /Users/you/projects/gzh-test
Branch: main
Remote: https://github.com/your/repo.git
Status: Clean ✓
```

______________________________________________________________________

## Common Workflows

### Workflow 1: Check Multiple Repositories

```bash
#!/bin/bash
# check-repos.sh - Check status of multiple repositories

repos=(
    ~/projects/repo1
    ~/projects/repo2
    ~/projects/repo3
)

for repo in "${repos[@]}"; do
    echo "Checking $repo..."
    gzh-git status "$repo" -q
    if [ $? -eq 1 ]; then
        echo "  ✗ Repository has changes"
    else
        echo "  ✓ Clean"
    fi
    echo
done
```

### Workflow 2: Clone Multiple Repositories

```bash
#!/bin/bash
# clone-projects.sh - Clone multiple related repositories

repos=(
    "https://github.com/org/frontend.git"
    "https://github.com/org/backend.git"
    "https://github.com/org/shared.git"
)

for repo in "${repos[@]}"; do
    gzh-git clone "$repo"
done
```

### Workflow 3: Programmatic Repository Management

```go
// check-all-repos.go
package main

import (
    "context"
    "fmt"
    "log"
    "sync"

    "github.com/gizzahub/gzh-cli-git/pkg/repository"
)

func main() {
    repos := []string{
        "/path/to/repo1",
        "/path/to/repo2",
        "/path/to/repo3",
    }

    ctx := context.Background()
    client := repository.NewClient()

    var wg sync.WaitGroup
    for _, path := range repos {
        wg.Add(1)
        go func(p string) {
            defer wg.Done()
            checkRepo(ctx, client, p)
        }(path)
    }

    wg.Wait()
}

func checkRepo(ctx context.Context, client repository.Client, path string) {
    repo, err := client.Open(ctx, path)
    if err != nil {
        log.Printf("Error opening %s: %v", path, err)
        return
    }

    status, err := client.GetStatus(ctx, repo)
    if err != nil {
        log.Printf("Error checking status %s: %v", path, err)
        return
    }

    if status.IsClean {
        fmt.Printf("✓ %s is clean\n", path)
    } else {
        fmt.Printf("✗ %s has changes\n", path)
    }
}
```

______________________________________________________________________

## Quick Reference

### Essential Commands

| Command | Description | Example |
|---------|-------------|---------|
| `gzh-git clone <url>` | Clone repository | `gzh-git clone https://github.com/user/repo.git` |
| `gzh-git status [path]` | Check repository status | `gzh-git status` |
| `gzh-git info [path]` | Show repository information | `gzh-git info` |
| `gzh-git --version` | Show version | `gzh-git --version` |
| `gzh-git --help` | Show help | `gzh-git --help` |

### Common Flags

| Flag | Description | Works With |
|------|-------------|------------|
| `-q, --quiet` | Quiet mode (errors only) | All commands |
| `-v, --verbose` | Verbose output | All commands |
| `-b <branch>` | Specific branch | `clone` |
| `--depth <n>` | Shallow clone | `clone` |
| `--single-branch` | Clone only one branch | `clone` |
| `--recursive` | Clone with submodules | `clone` |

______________________________________________________________________

## What's Next?

Now that you know the basics, explore:

1. **Advanced Usage**: Check out [User Guide](../guides/README.md)
1. **Library Integration**: See [Library Guide](../../LIBRARY.md) for detailed API documentation
1. **Troubleshooting**: Visit [Troubleshooting Guide](../../TROUBLESHOOTING.md) for common issues
1. **FAQ**: Read [FAQ](../guides/faq.md) for frequently asked questions

### Planned Features

These features are coming soon (not yet available):

- 🚀 **Commit Automation** (v0.2.0) - Template-based commit messages
- 🌿 **Branch Management** (v0.3.0) - Worktree and parallel development
- 📊 **History Analysis** (v0.4.0) - Statistics and contributor insights
- 🔀 **Advanced Merge** (v0.5.0) - Conflict detection and resolution

See [Roadmap](../../README.md#roadmap) for detailed timeline.

______________________________________________________________________

## Getting Help

**Documentation**:

- [README](../../README.md) - Project overview
- [Installation Guide](../../docs/INSTALL.md) - Detailed installation
- [Command Reference](../../docs/commands/README.md) - All commands

**Community**:

- [GitHub Issues](https://github.com/gizzahub/gzh-cli-git/issues) - Report bugs
- [GitHub Discussions](https://github.com/gizzahub/gzh-cli-git/discussions) - Ask questions

______________________________________________________________________

**Congratulations!** 🎉 You've completed your first steps with gzh-git. Happy coding!
