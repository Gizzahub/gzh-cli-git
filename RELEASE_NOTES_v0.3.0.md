# Release Notes - v0.3.0

**Release Date:** 2025-12-02

## 🎯 Highlights

v0.3.0 introduces powerful bulk repository management features with the new `pull` command, continuous monitoring capabilities, and improved CLI ergonomics.

## ✨ New Features

### 1. Bulk Pull Command - Parallel Repository Updates

The new `gz-git pull` command enables parallel pulling from multiple repositories with sophisticated merge strategies:

**Core Capabilities:**

- Pull updates from multiple repositories in parallel
- Three merge strategies: `merge` (default), `rebase`, `ff-only`
- Automatic stash/pop for repositories with local changes
- Smart detection of remote and upstream configuration
- Context-aware operations with ahead/behind tracking

**CLI Features:**

```bash
# Basic usage - pull all repos in current directory
gz-git pull -d 1

# Pull with rebase strategy
gz-git pull --strategy rebase -d 2 ~/projects

# Auto-stash local changes before pull
gz-git pull --stash -d 2 ~/projects

# Watch mode - continuous pulling (default: 1m)
gz-git pull -d 2 --watch ~/projects

# Dry-run to preview
gz-git pull --dry-run -d 2 ~/work
```

**Why This Matters:**

- Fills the gap between `fetch` (download only) and `update` (single repo)
- Respects Git semantics: pull = fetch + merge/rebase
- Handles complex scenarios: dirty repos, merge conflicts, missing upstreams
- Status icons show operation results at a glance

### 2. Watch Mode for Fetch - Continuous Remote Monitoring

The `fetch` command now supports continuous monitoring:

```bash
# Continuously fetch every 5 minutes (default)
gz-git fetch -d 2 --watch ~/projects

# Custom interval
gz-git fetch --watch --interval 1m ~/work
```

**Features:**

- Performs initial fetch immediately
- Graceful shutdown with Ctrl+C
- Continues watching even if individual fetches fail
- Appropriate default interval (5 minutes) for remote operations

### 3. Enhanced Nested Repository Scanning

Improved repository discovery with intelligent submodule handling:

- `--include-submodules` flag for opt-in submodule scanning
- Default behavior: scans independent nested repos, excludes git submodules
- Smart detection using `.git` file vs directory differentiation
- Handles deeply nested repository hierarchies

## 🔄 Breaking Changes

### 1. CLI Flag Simplification

**Changed:** `--max-depth` → `-d, --depth`

```bash
# Old (v0.2.0)
gz-git fetch --max-depth 2

# New (v0.3.0)
gz-git fetch -d 2
```

**Why:**

- Shorter and more intuitive for frequently used command
- Consistent with Unix conventions (`du -d`, `fd -d`)
- Clean breaking change with no deprecated flag support

**Impact:** Affects `fetch` and `pull` commands

### 2. Added Ergonomic Shorthand Flags

Following GNU and Git conventions, we've added shorthand flags for frequently used options:

**New Shorthand Flags:**

| Flag | Short | Convention | Commands |
|------|-------|-----------|----------|
| `--parallel` | `-j` | make -j, xargs -P | fetch, pull |
| `--dry-run` | `-n` | make -n, apt -s | fetch, pull |
| `--tags` | `-t` | git fetch -t | fetch, pull |
| `--prune` | `-p` | git prune | pull |
| `--recursive` | `-r` | cp -r, grep -r | fetch, pull |

**Examples:**

```bash
# Before (verbose)
gz-git pull --parallel 10 --dry-run --tags --prune --recursive -d 2 ~/work

# After (concise)
gz-git pull -j 10 -n -t -p -r -d 2 ~/work
```

### 3. Renamed Flag for Consistency

**Changed:** `--include-submodules` → `--recursive` (with `-r` shorthand)

**Why:**

- More intuitive and follows GNU conventions (cp -r, grep -r)
- Shorter and more memorable
- Consistent with industry-standard naming

**Impact:** Affects both `fetch` and `pull` commands

## 🐛 Bug Fixes

- **Fixed:** `isSubmodule()` false positive detection

  - Previous: Incorrectly identified independent nested repos as submodules
  - Fixed: Only checks `.git` file type, not parent `.gitmodules`

- **Fixed:** `walkDirectoryWithConfig()` early return bug

  - Previous: Stopped scanning when finding nested repos
  - Fixed: Continues scanning to find deeply nested structures

## 📊 Performance & Quality

- All tests passing (10/10 bulk operation tests)
- Comprehensive coverage for nested repository scenarios
- Production-ready error handling and recovery
- Optimized parallel processing with configurable concurrency

## 🔧 Technical Details

### Architecture

**New Types:**

- `BulkPullOptions` - Configuration for bulk pull operations
- `BulkPullResult` - Detailed results with status summaries
- `RepositoryPullResult` - Individual repo results with commits ahead/behind

**New Methods:**

- `Client.BulkPull()` - Main bulk pull implementation
- `processPullRepositories()` - Parallel processing with errgroup
- `processPullRepository()` - Individual repo pull logic

**Watch Implementation:**

- Signal handling for graceful shutdown (SIGINT, SIGTERM)
- Ticker-based scheduling with configurable intervals
- Error resilience - continues watching on individual failures

## 📚 Documentation

- Updated README with pull command examples
- Comprehensive CHANGELOG with all changes
- Inline code documentation for all new APIs

## 🚀 Upgrade Guide

### From v0.2.0 to v0.3.0

1. **Update flag usage:**

   ```bash
   # Replace --max-depth with -d or --depth
   find . -name "*.sh" -exec sed -i 's/--max-depth/-d/g' {} \;
   ```

1. **Try new pull command:**

   ```bash
   # Instead of manual cd + git pull loops
   gz-git pull -d 2 ~/projects
   ```

1. **Enable watch mode for continuous monitoring:**

   ```bash
   # Keep repos up to date automatically (fetch: 5m, pull: 1m)
   gz-git pull -d 2 --watch ~/work
   gz-git fetch -d 2 --watch ~/work
   ```

## 🎁 What's Next

Future enhancements under consideration:

- Commit automation with templates
- Advanced merge conflict resolution
- Git history analysis tools
- Custom hooks and plugins

## 📦 Installation

```bash
# Using Go
go install github.com/gizzahub/gzh-cli-git/cmd/gzh-git@v0.3.0

# From source
git clone https://github.com/gizzahub/gzh-cli-git
cd gzh-cli-git
git checkout v0.3.0
make install
```

## 🔗 Links

- **Repository:** https://github.com/gizzahub/gzh-cli-git
- **Documentation:** https://pkg.go.dev/github.com/gizzahub/gzh-cli-git
- **Issues:** https://github.com/gizzahub/gzh-cli-git/issues
- **Changelog:** [CHANGELOG.md](CHANGELOG.md)

## 🙏 Acknowledgments

Built with:

- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [Go](https://golang.org/) - Programming language

Follows:

- [Conventional Commits](https://www.conventionalcommits.org/)
- [Semantic Versioning](https://semver.org/)
- [Keep a Changelog](https://keepachangelog.com/)

______________________________________________________________________

**Full Changelog:** https://github.com/gizzahub/gzh-cli-git/compare/v0.2.0...v0.3.0
