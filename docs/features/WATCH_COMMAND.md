# Watch Command - Real-time Repository Monitoring

> **Status**: ✅ Implemented
> **Version**: Unreleased (targeting v0.3.0)
> **Package**: `pkg/watch`, `cmd/gzh-git/cmd/watch.go`

## Overview

The `watch` command provides real-time monitoring of Git repositories, detecting changes as they happen. It's useful for:

- **Development workflows**: Monitor working directory while coding
- **CI/CD pipelines**: Trigger actions on repository changes
- **Team awareness**: Track when collaborators make changes
- **Automation**: Build systems that react to Git events

## Features

### Change Detection

Detects the following types of changes:

- **Modified files** - Unstaged changes to tracked files
- **Staged files** - Files added to the staging area
- **Untracked files** - New files not yet tracked by Git
- **Deleted files** - Removed files
- **Branch switches** - When active branch changes
- **Clean state** - When repository becomes clean (optional)

### Multi-Repository Support

Monitor multiple repositories simultaneously:

```bash
gzh-git watch /path/to/repo1 /path/to/repo2 /path/to/repo3
```

Each repository is monitored independently with its own state tracking.

### Output Formats

Three output formats for different use cases:

#### 1. Default (Detailed)

Human-readable format with timestamps, color coding, and file lists:

```
[15:04:05] my-project ● Modified (3 files)
    src/main.go
    pkg/handler.go
    README.md

[15:04:12] my-project ● Staged (2 files)
    src/main.go
    pkg/handler.go
```

#### 2. Compact

Minimal single-line output:

```
[15:04:05] my-project: modified [3]
[15:04:12] my-project: staged [2]
```

#### 3. JSON

Machine-readable format for automation:

```json
{"timestamp":"2025-12-01T15:04:05Z","path":"/path/to/repo","type":"modified","files":["src/main.go","pkg/handler.go","README.md"]}
{"timestamp":"2025-12-01T15:04:12Z","path":"/path/to/repo","type":"staged","files":["src/main.go","pkg/handler.go"]}
```

### Configuration Options

| Option | Default | Description |
|--------|---------|-------------|
| `--interval` | 2s | Polling interval for checking changes |
| `--include-clean` | false | Notify when repository becomes clean |
| `--format` | default | Output format (default, compact, json) |
| `--notify` | false | Play sound on changes (macOS/Linux) |

## Usage Examples

### Basic Monitoring

Watch current directory:

```bash
gzh-git watch
```

Watch specific repository:

```bash
gzh-git watch /path/to/repo
```

### Custom Interval

Check for changes every 5 seconds (less CPU intensive):

```bash
gzh-git watch --interval 5s
```

Check more frequently (1 second):

```bash
gzh-git watch --interval 1s
```

### Output Formats

Compact output for terminals:

```bash
gzh-git watch --format compact
```

JSON for piping to other tools:

```bash
gzh-git watch --format json | jq '.files[]'
```

### Multi-Repository Monitoring

Watch multiple projects:

```bash
gzh-git watch ~/projects/frontend ~/projects/backend ~/projects/api
```

### Include Clean Events

Get notified when repository becomes clean after having changes:

```bash
gzh-git watch --include-clean
```

### Automation Examples

#### Trigger Build on Changes

```bash
gzh-git watch --format json | while read -r event; do
  TYPE=$(echo "$event" | jq -r '.type')
  if [ "$TYPE" = "staged" ]; then
    echo "Running build..."
    make build
  fi
done
```

#### Log Changes to File

```bash
gzh-git watch --format json >> git-changes.log
```

#### Notify Desktop on Changes

```bash
gzh-git watch --format compact | while read -r line; do
  osascript -e "display notification \"$line\" with title \"Git Change Detected\""
done
```

## Architecture

### Event Flow

```
File System Changes
        ↓
    fsnotify
        ↓
  Event Detection
        ↓
   Git Status Check
        ↓
  Change Detection
        ↓
    Debouncing
        ↓
  Event Channel
        ↓
    Formatter
        ↓
     Output
```

### Components

1. **Watcher** (`pkg/watch/watcher.go`)

   - Core monitoring engine
   - File system event handling (fsnotify)
   - Periodic polling (configurable interval)
   - Event debouncing (prevents duplicates)

1. **Repository State Tracking**

   - Maintains last known status for each repository
   - Compares current vs. previous state
   - Detects specific change types

1. **Event Types** (`pkg/watch/interfaces.go`)

   - Type-safe event definitions
   - Structured event data
   - Timestamp tracking

1. **Formatters** (`cmd/gzh-git/cmd/watch.go`)

   - Default: Human-readable with colors
   - Compact: Single-line output
   - JSON: Machine-readable

### Debouncing

The watcher implements debouncing to prevent duplicate events:

- **Default debounce duration**: 500ms
- **Purpose**: Prevents multiple events for rapid successive changes
- **Behavior**: Only one event per repository per debounce period

## Library Usage

Use the watch package programmatically:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/gizzahub/gzh-cli-git/pkg/repository"
    "github.com/gizzahub/gzh-cli-git/pkg/watch"
)

func main() {
    ctx := context.Background()

    // Create repository client
    repoClient := repository.NewClient()

    // Create watcher
    watcher, err := watch.NewWatcher(repoClient, watch.WatchOptions{
        Interval:         2 * time.Second,
        IncludeClean:     true,
        DebounceDuration: 500 * time.Millisecond,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer watcher.Stop()

    // Start watching
    paths := []string{"/path/to/repo"}
    if err := watcher.Start(ctx, paths); err != nil {
        log.Fatal(err)
    }

    // Process events
    for {
        select {
        case event := <-watcher.Events():
            fmt.Printf("Change detected: %s (%d files)\n",
                event.Type, len(event.Files))

        case err := <-watcher.Errors():
            log.Printf("Error: %v\n", err)
        }
    }
}
```

## Performance Characteristics

### CPU Usage

- **Idle (no changes)**: ~0.1% CPU
- **Active monitoring**: ~0.5-1% CPU per repository
- **Recommendation**: Use longer intervals for multiple repositories

### Memory Usage

- **Base**: ~5MB per repository
- **Scales linearly**: Approximately 5MB per additional repository
- **Event buffer**: 100 events (configurable)

### File System Events

Uses `fsnotify` for efficient file system monitoring:

- **Native OS support**: inotify (Linux), FSEvents (macOS), ReadDirectoryChangesW (Windows)
- **Low overhead**: Events delivered by OS kernel
- **Fallback**: Periodic polling if fsnotify unavailable

## Limitations

### Current Limitations

1. **Submodule tracking**: Does not track changes in submodules
1. **Large repositories**: Performance may degrade with >10,000 files
1. **Network filesystems**: May not work reliably on NFS/SMB mounts
1. **Sound notifications**: macOS only (placeholder for other platforms)

### Future Enhancements

- [ ] Submodule change detection
- [ ] File pattern filtering (e.g., only watch `*.go` files)
- [ ] Change history/replay
- [ ] Webhook notifications
- [ ] Cross-platform sound notifications
- [ ] Performance optimizations for large repos

## Troubleshooting

### High CPU Usage

**Problem**: Watch command uses excessive CPU

**Solutions**:

- Increase polling interval: `--interval 5s` or higher
- Reduce number of monitored repositories
- Check for very large repositories (>10k files)

### Missing Events

**Problem**: Some changes not detected

**Solutions**:

- Decrease polling interval: `--interval 1s`
- Check file system event support (fsnotify)
- Verify repository is not on network filesystem

### Duplicate Events

**Problem**: Same change reported multiple times

**Solutions**:

- Increase debounce duration (requires code change)
- Use `--format compact` for less verbose output

## Testing

Tests located in `pkg/watch/watcher_test.go`:

```bash
# Run watch package tests
go test ./pkg/watch/... -v

# Run with coverage
go test ./pkg/watch/... -cover

# Benchmark
go test ./pkg/watch/... -bench=.
```

## Related Documentation

- [Repository Package](../pkg/repository/README.md)
- [Status Command](./STATUS_COMMAND.md)
- [CLI Architecture](../ARCHITECTURE.md)

## Changelog

### Unreleased

- ✅ Initial implementation
- ✅ Multiple output formats
- ✅ Multi-repository support
- ✅ File system integration (fsnotify)
- ✅ Comprehensive tests

______________________________________________________________________

**Last Updated**: 2025-12-01
**Status**: Ready for release in v0.3.0
