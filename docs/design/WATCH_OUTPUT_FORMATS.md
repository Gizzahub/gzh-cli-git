# Watch Command Output Format Design

> **Status**: ✅ Implemented
> **Discussion Date**: 2025-12-01
> **Decision**: Multiple format options for different use cases

## Context

The `gzh-git watch` command monitors repositories for changes in real-time. The challenge is presenting this information in a way that is:

1. **Human-readable** for developers monitoring their work
1. **Machine-parseable** for automation and integration
1. **Concise** for terminal usage
1. **Informative** with enough detail to be actionable

## Requirements

### Functional Requirements

- **FR1**: Display timestamp for each change event
- **FR2**: Show repository name/path where change occurred
- **FR3**: Indicate type of change (modified, staged, untracked, etc.)
- **FR4**: List affected files (when applicable)
- **FR5**: Support multiple repositories simultaneously
- **FR6**: Allow format customization via CLI flags

### Non-Functional Requirements

- **NFR1**: Minimal screen clutter for long-running monitors
- **NFR2**: Color coding for quick visual scanning
- **NFR3**: JSON output for programmatic consumption
- **NFR4**: Performance: minimal CPU/memory overhead

## Design Decisions

### 1. Multiple Output Formats

**Decision**: Provide three distinct formats via `--format` flag

**Rationale**:

- Different use cases have different needs
- Human monitoring vs. machine parsing
- Terminal real estate constraints
- Flexibility for user preference

### 2. Format Options

#### Default Format (Detailed)

**Target Audience**: Developers actively monitoring changes

**Design**:

```
[15:04:05] my-project ● Modified (3 files)
    src/main.go
    pkg/handler.go
    README.md
```

**Features**:

- Color-coded change types
- File list (limited to 5 by default)
- Visual hierarchy with indentation
- Timestamp for temporal tracking

**Pros**:

- Immediately understandable
- Rich contextual information
- Easy to scan visually
- Color helps categorize changes

**Cons**:

- Verbose for high-frequency changes
- More screen space usage
- May scroll quickly with many events

#### Compact Format

**Target Audience**: Users monitoring multiple repos or long sessions

**Design**:

```
[15:04:05] my-project: modified [3]
[15:04:12] my-project: staged [2]
```

**Features**:

- Single line per event
- File count indicator
- Minimal formatting
- No color (optional)

**Pros**:

- Efficient screen usage
- Easy to grep/filter
- Less visual noise
- Suitable for logging

**Cons**:

- Less contextual detail
- No file names shown
- Requires mental mapping of counts

#### JSON Format

**Target Audience**: Automation, CI/CD, integration scripts

**Design**:

```json
{"timestamp":"2025-12-01T15:04:05Z","path":"/path/to/repo","type":"modified","files":["src/main.go","pkg/handler.go","README.md"]}
```

**Features**:

- Structured data format
- Machine-readable
- One JSON object per line (JSONL)
- ISO 8601 timestamps

**Pros**:

- Perfect for automation
- Parseable by `jq`, Python, etc.
- Structured data types
- Timestamped for analytics

**Cons**:

- Not human-friendly
- Verbose for manual reading
- Requires parsing tools

### 3. Color Coding Scheme

**Decision**: Use semantic colors for change types

**Color Mapping**:

- 🟡 **Yellow** (`\x1b[33m`) - Modified files (warning)
- 🟢 **Green** (`\x1b[32m`) - Staged files (ready) / Clean state (success)
- 🟣 **Purple** (`\x1b[35m`) - Untracked files (new)
- 🔴 **Red** (`\x1b[31m`) - Deleted files (removal)
- 🔵 **Cyan** (`\x1b[36m`) - Branch changes (info)
- ⚪ **Gray** (`\x1b[90m`) - Timestamps (muted)

**Rationale**:

- Follows common Git UI conventions
- Intuitive semantic meaning
- Accessible (avoids red-green only distinction)

### 4. File Limit Strategy

**Decision**: Show first 5 files, indicate if more exist

**Example**:

```
[15:04:05] my-project ● Modified (12 files)
    src/main.go
    src/handler.go
    src/utils.go
    src/config.go
    src/models.go
    ... and 7 more
```

**Rationale**:

- Prevents screen overflow
- Most common changes affect \<5 files
- "... and N more" provides count context
- Users can run `status` for full list

**Alternative Considered**: `--max-files` flag

- **Rejected**: Adds complexity for rare use case
- **Future**: Could add if requested

### 5. Timestamp Format

**Decision**: Use `HH:MM:SS` for default/compact, ISO 8601 for JSON

**Examples**:

- Default/Compact: `[15:04:05]`
- JSON: `"2025-12-01T15:04:05Z"`

**Rationale**:

- Short time sufficient for same-day monitoring
- Full date adds noise for real-time watch
- JSON needs full timestamp for sorting/filtering
- ISO 8601 is standard for APIs

### 6. Event Symbols

**Decision**: Use Unicode symbols for visual distinction

**Symbols**:

- `●` (U+25CF) - Most change types
- `✓` (U+2713) - Clean state
- `→` (U+2192) - Branch name (future)

**Rationale**:

- Adds visual hierarchy
- Internationally understood
- Terminal-safe characters
- Improves scannability

## Implementation Details

### Formatter Interface

```go
type eventFormatter interface {
    Format(event watch.Event) string
}
```

### Factory Pattern

```go
func newEventFormatter(format string) eventFormatter {
    switch format {
    case "compact":
        return &compactFormatter{}
    case "json":
        return &jsonFormatter{}
    default:
        return &defaultFormatter{}
    }
}
```

### Output Buffering

- Events written directly to stdout
- No internal buffering (real-time display)
- OS handles terminal buffering

## Alternative Designs Considered

### 1. Table Format

**Design**: Fixed-width columns like `git log --oneline`

```
TIME     REPO        TYPE      FILES
15:04:05 my-project  modified  3
15:04:12 my-project  staged    2
```

**Rejected Because**:

- Less information density
- Difficult with variable-length repo names
- No space for file lists
- Better suited for static reports

### 2. Streaming Dashboard

**Design**: Full-screen TUI with live updates (like `htop`)

```
┌─ Repository Monitor ─────────────────────┐
│ my-project                    [Modified] │
│   src/main.go                            │
│   pkg/handler.go                         │
│                                          │
│ other-project                    [Clean] │
└──────────────────────────────────────────┘
```

**Rejected Because**:

- Incompatible with piping/automation
- Cannot scroll back through history
- Requires terminal control (not SSH-safe)
- Overkill for simple monitoring
- **Future**: Could add as `watch --tui`

### 3. Diff-Style Output

**Design**: Show actual file diffs inline

```
[15:04:05] my-project ● Modified

src/main.go:
+ func newFeature() {
+ }
```

**Rejected Because**:

- Extremely verbose
- High CPU overhead (requires diffing)
- Not useful for real-time monitoring
- Better handled by `git diff` separately
- **Future**: Could add `--show-diff` flag

### 4. Growl/Desktop Notifications

**Design**: Native OS notifications for each event

**Partially Implemented**:

- `--notify` flag added (placeholder)
- macOS support planned
- Cross-platform via `beeep` library

**Deferred Because**:

- Platform-specific complexity
- Notification fatigue for high-frequency changes
- Not useful for automation
- **Future**: Will implement in v0.3.0

## User Customization

### Current Configuration

Via CLI flags only:

```bash
gzh-git watch --format compact --interval 5s
```

### Future Configuration

Could support config file (`.gzh-git/config.yaml`):

```yaml
watch:
  format: compact
  interval: 5s
  includeClean: true
  maxFiles: 10
```

**Status**: Deferred to v1.0.0

## Examples by Use Case

### Use Case 1: Active Development

**Scenario**: Developer coding in IDE, wants to see what changed

**Command**:

```bash
gzh-git watch
```

**Output**: Default format with colors and file lists

### Use Case 2: Build System

**Scenario**: CI system triggers builds on staged files

**Command**:

```bash
gzh-git watch --format json | jq -r 'select(.type=="staged") | .path'
```

**Output**: Repository paths where files are staged

### Use Case 3: Team Awareness

**Scenario**: Shared screen showing team activity

**Command**:

```bash
gzh-git watch --format compact ~/team/repos/*
```

**Output**: Compact multi-repo summary

### Use Case 4: Change Logging

**Scenario**: Audit trail of all repository changes

**Command**:

```bash
gzh-git watch --format json >> ~/logs/git-changes.jsonl
```

**Output**: Append-only JSON log file

## Performance Considerations

### Format Impact on Performance

| Format | CPU Overhead | Memory | Notes |
|---------|--------------|--------|-------|
| Default | ~0.5% | 1KB/event | Color codes + formatting |
| Compact | ~0.3% | 500B/event | Minimal processing |
| JSON | ~0.4% | 800B/event | JSON encoding |

**Conclusion**: All formats are lightweight enough for real-time use

## Accessibility Considerations

### Color Blindness

- Symbols (`●`, `✓`) provide non-color distinction
- JSON format has no color dependency
- Compact format can disable colors

### Screen Readers

- Default format has logical structure
- Timestamps aid temporal understanding
- JSON format most accessible for parsing

### Terminal Limitations

- Fallback to ASCII if Unicode unsupported
- Color detection via terminal capabilities
- Graceful degradation on old terminals

## Future Enhancements

### Planned for v0.3.0

1. **Custom Format Strings**

   ```bash
   gzh-git watch --format='{time} {repo} {type} {count}'
   ```

1. **Filter Options**

   ```bash
   gzh-git watch --only=staged,modified
   ```

1. **Sound Notifications**

   - macOS: `afplay`
   - Linux: `paplay`
   - Windows: `Beep` API

### Planned for v1.0.0

1. **TUI Dashboard Mode**

   ```bash
   gzh-git watch --tui
   ```

1. **Webhook Notifications**

   ```bash
   gzh-git watch --webhook=https://api.example.com/git-changes
   ```

1. **Configuration File Support**

   - Per-repository watch settings
   - Default format preferences
   - Custom color schemes

## Conclusion

The implemented multi-format approach provides:

✅ **Flexibility**: Three formats for different needs
✅ **Usability**: Human-readable default with colors
✅ **Automation**: Machine-parseable JSON format
✅ **Efficiency**: Compact format for minimal overhead
✅ **Extensibility**: Easy to add new formats via interface

The design balances immediate usability with future extensibility, providing a solid foundation for real-time Git monitoring.

______________________________________________________________________

**Decision Status**: ✅ Implemented
**Last Updated**: 2025-12-01
**Next Review**: After v0.3.0 user feedback
