# Watch Output Improvements - Future Considerations

> **Status**: 💭 Discussion / Future Enhancement
> **Date**: 2025-12-01
> **Context**: After implementing basic watch functionality, considering enhanced output options

## Current Implementation Review

### What We Have Now

**Three Output Formats**:

1. **Default** - Detailed with colors and file lists
1. **Compact** - Single line per event
1. **JSON** - Machine-readable structured data

**Basic Information Displayed**:

- Timestamp (when change occurred)
- Repository name/path
- Change type (modified, staged, untracked, etc.)
- File count
- File list (first 5 files)

### What's Missing / Could Be Better

After implementing the basic functionality, we've identified several areas where output could be more informative or useful:

## Enhancement Ideas

### 1. Contextual Diff Summary

**Problem**: Users see "3 files modified" but don't know the nature of changes

**Proposed Enhancement**: Show change statistics

```
[15:04:05] my-project ● Modified (3 files, +42 -15 lines)
    src/main.go        +30 -5
    pkg/handler.go     +8  -7
    README.md          +4  -3
```

**Implementation**:

- Add `--show-stats` flag
- Run `git diff --stat` for each file
- Cache results to avoid performance hit
- Update Event struct to include stats

**Use Cases**:

- Quick assessment of change magnitude
- Identify major refactors vs. small tweaks
- Code review preparation

**Trade-offs**:

- ➕ More informative
- ➕ Helps prioritize attention
- ➖ Adds ~50ms latency per check
- ➖ More screen space usage

### 2. Grouped/Batched Output

**Problem**: High-frequency changes create output spam

**Proposed Enhancement**: Batch changes within a time window

```
[15:04:05 - 15:04:12] my-project
    ● Modified (3 files)
        src/main.go
        pkg/handler.go
    ● Staged (2 files)
        src/main.go
        pkg/handler.go
    ● Untracked (1 file)
        test/new_test.go
```

**Implementation**:

- Add `--batch-duration` flag (e.g., 5s)
- Accumulate events within window
- Output as single grouped summary
- Deduplicate files across event types

**Use Cases**:

- Saving/auto-format causing multiple rapid changes
- Batch operations (git add multiple files)
- Reducing terminal noise

**Trade-offs**:

- ➕ Cleaner output
- ➕ Better for automated actions
- ➖ Delayed notification
- ➖ May hide temporal sequence

### 3. Smart Filtering

**Problem**: Not all changes are equally important

**Proposed Enhancement**: Intelligent change filtering

```bash
# Only show specific file types
gzh-git watch --files='*.go,*.md'

# Ignore specific patterns
gzh-git watch --ignore='*_test.go,vendor/*'

# Only show "important" changes (configurable)
gzh-git watch --important-only
```

**Important Change Heuristics**:

- Files in critical paths (src/, pkg/)
- Large line changes (>100 lines)
- Multiple files in same module
- Changes to configuration files

**Implementation**:

- Pattern matching on file paths
- Integration with `.gitignore` patterns
- Custom importance rules in config

**Use Cases**:

- Focus on production code (ignore tests)
- Monitor specific subsystem
- Reduce noise from generated files

**Trade-offs**:

- ➕ Focused attention
- ➕ Reduces cognitive load
- ➖ May miss important changes
- ➖ Configuration complexity

### 4. Rich Notifications

**Problem**: Terminal-only output limits awareness

**Proposed Enhancement**: Multi-channel notifications

**Desktop Notifications**:

```bash
gzh-git watch --notify desktop
```

- macOS: Notification Center
- Linux: libnotify/D-Bus
- Windows: Toast notifications

**Webhook Integration**:

```bash
gzh-git watch --webhook https://api.slack.com/hooks/...
```

- POST JSON events to URL
- Slack/Discord/Teams integration
- Custom webhook handlers

**Sound Alerts**:

```bash
gzh-git watch --sound beep     # System beep
gzh-git watch --sound custom.wav
```

**Implementation**:

- Platform-specific notification libraries
- HTTP client for webhooks
- Audio playback libraries

**Use Cases**:

- Background monitoring
- Team awareness (Slack channel)
- Audio cues while coding

**Trade-offs**:

- ➕ Non-intrusive awareness
- ➕ Better for background tasks
- ➖ Platform dependencies
- ➖ Notification fatigue risk

### 5. Interactive Mode

**Problem**: Passive monitoring lacks actionability

**Proposed Enhancement**: Interactive commands on detection

```
[15:04:05] my-project ● Modified (3 files)
    src/main.go
    pkg/handler.go
    README.md

Actions: [d]iff [s]tatus [c]ommit [i]gnore [q]uit >
```

**Interactive Actions**:

- `d` - Show diff of changes
- `s` - Run full status
- `c` - Quick commit (auto-generate message)
- `i` - Ignore this event
- `r` - Revert changes
- `q` - Quit watch

**Implementation**:

- Terminal raw mode for key capture
- Command execution on keypress
- Return to watch after action

**Use Cases**:

- Rapid development workflow
- Quick commit after changes
- Immediate feedback loop

**Trade-offs**:

- ➕ Highly productive workflow
- ➕ Reduces tool switching
- ➖ Not compatible with piping
- ➖ Requires terminal focus

### 6. Historical Timeline

**Problem**: Can't review what happened earlier

**Proposed Enhancement**: Scrollable history view

```bash
gzh-git watch --history 50  # Keep last 50 events
```

**In-Watch Commands**:

- `↑` / `↓` - Scroll through history
- `/` - Search events
- `r` - Replay event details
- `e` - Export history to file

**Visual Timeline**:

```
15:04:05 ● my-project: modified [3]
15:04:12 ● my-project: staged [2]
15:04:20 ● my-project: clean ✓
15:04:45 ● other-proj: modified [1]
────────────────────────────────────
[Watching...] Press 'h' for help
```

**Implementation**:

- Ring buffer for event history
- TUI library (e.g., bubbletea, tview)
- Keyboard event handling

**Use Cases**:

- Review change sequence
- Debug workflow issues
- Generate change logs

**Trade-offs**:

- ➕ Complete context
- ➕ Useful for analysis
- ➖ Memory overhead
- ➖ Complexity increase

### 7. Smart Summarization

**Problem**: Too much detail, not enough insight

**Proposed Enhancement**: AI-powered change summaries

```
[15:04:05] my-project ● Work Session Summary
    📝 Refactoring authentication module (8 files)
    🐛 Fixed bug in user handler (2 files)
    ✨ Added new API endpoint (3 files)

    Confidence: High | Impact: Medium
```

**Analysis Features**:

- Detect related file groups
- Infer intent from file names/paths
- Classify change type (feature, bugfix, refactor)
- Estimate impact/scope

**Implementation**:

- Pattern matching on file paths
- Change size heuristics
- Optional LLM integration for commit message analysis
- Configurable inference rules

**Use Cases**:

- High-level work tracking
- End-of-day summary
- Team status updates

**Trade-offs**:

- ➕ Contextual understanding
- ➕ Reduces cognitive load
- ➖ Inference may be wrong
- ➖ Computational overhead

### 8. Comparative Views

**Problem**: Hard to see patterns across time/repos

**Proposed Enhancement**: Comparative statistics

```bash
gzh-git watch --compare daily
```

**Output**:

```
Repository Activity (Last Hour)
────────────────────────────────
my-project     ████████░░  42 changes
other-project  ██░░░░░░░░  8 changes
third-repo     ░░░░░░░░░░  0 changes

Most Active Files:
  src/main.go      15 changes
  pkg/handler.go   8 changes
  README.md        3 changes
```

**Comparison Modes**:

- Hourly/Daily activity charts
- Repository comparison
- File hotspot detection
- Developer activity (if multi-user)

**Implementation**:

- Event aggregation and analysis
- Simple bar chart rendering
- Periodic statistics updates

**Use Cases**:

- Identify focus areas
- Workload distribution
- Detect unusual activity

**Trade-offs**:

- ➕ Strategic insights
- ➕ Patterns become visible
- ➖ Requires longer monitoring
- ➖ More complex UI

## Recommended Priority

### Phase 1: Quick Wins (Next Release - v0.3.0)

1. **Smart Filtering** (`--files`, `--ignore`)

   - High value, moderate effort
   - Reduces noise immediately
   - Uses existing gitignore patterns

1. **Desktop Notifications** (`--notify desktop`)

   - Platform-specific but valuable
   - Start with macOS, expand later
   - Non-blocking enhancement

### Phase 2: Enhanced Awareness (v0.4.0)

3. **Grouped/Batched Output** (`--batch-duration`)

   - Improves output clarity
   - Moderate implementation effort
   - Good for high-frequency scenarios

1. **Webhook Integration** (`--webhook`)

   - Enables team awareness
   - Simple HTTP POST implementation
   - JSON format already exists

### Phase 3: Advanced Features (v0.5.0+)

5. **Contextual Diff Summary** (`--show-stats`)

   - More informative
   - Performance considerations needed
   - Adds cognitive value

1. **Historical Timeline** (`--history`)

   - Requires TUI framework
   - Significant complexity
   - High learning curve

### Phase 4: Future R&D (v1.0.0+)

7. **Interactive Mode**

   - Experimental workflow
   - Needs user testing
   - May not suit all use cases

1. **Smart Summarization**

   - Research phase
   - Optional AI integration
   - Accuracy concerns

## User Research Questions

Before implementing, we should validate:

1. **Which format do users prefer most?**

   - Run survey after v0.2.0 release
   - Track flag usage analytics
   - Gather feedback on GitHub

1. **What's the common polling interval?**

   - Default 2s may be too frequent/slow
   - Auto-adjust based on activity?

1. **Do users want more or less information?**

   - Current file list limit (5 files)
   - Statistics overload vs. minimalism

1. **Is real-time always necessary?**

   - Maybe batch mode should be default?
   - Periodic summaries vs. instant

## Configuration File Design

**Proposed**: `.gzh-git/watch.yaml`

```yaml
watch:
  # Output
  format: default
  show_stats: true
  max_files: 10
  batch_duration: 5s

  # Filtering
  include_patterns:
    - "*.go"
    - "*.md"
  exclude_patterns:
    - "*_test.go"
    - "vendor/*"
    - "*.generated.go"

  # Notifications
  desktop_notify: true
  sound: beep
  webhooks:
    - url: https://hooks.slack.com/...
      events: [staged, clean]

  # Behavior
  interval: 2s
  include_clean: false
  debounce: 500ms

  # Per-repository overrides
  repositories:
    ~/critical-project:
      interval: 1s
      desktop_notify: true
    ~/experiments:
      interval: 10s
      include_clean: true
```

**Benefits**:

- Persistent preferences
- Per-repo customization
- Shareable team configs

## Technical Considerations

### Performance Budget

| Feature | Latency | Memory | CPU |
|---------|---------|--------|-----|
| Basic watch | \<10ms | 5MB/repo | 0.1% |
| With stats | +50ms | +2MB | +0.5% |
| With history | N/A | +10MB | 0% |
| Desktop notify | +5ms | +1MB | 0% |
| Webhooks | +100ms | +1MB | 0% |

**Goal**: Keep total overhead under 100ms per check, \<50MB memory

### Compatibility

- **Terminal**: Ensure works on various terminal emulators
- **SSH**: Remote sessions should work
- **Pipes**: JSON format must remain pipeable
- **CI**: Quiet/JSON modes for automation

### Error Handling

- **Network failures**: Webhook retries with backoff
- **Permission issues**: Graceful degradation
- **Large repos**: Performance warnings
- **Invalid config**: Clear error messages

## User Stories

### Story 1: Solo Developer

**Persona**: Sarah, full-stack developer

**Need**: Monitor changes while switching between editor and terminal

**Solution**:

```bash
gzh-git watch --notify desktop --files='*.go,*.ts'
```

**Experience**: Gets desktop notification when code files change, can stay focused in IDE

### Story 2: Team Lead

**Persona**: Alex, tech lead monitoring team activity

**Need**: See high-level team progress

**Solution**:

```bash
gzh-git watch ~/team-repos/* --batch-duration 60s --webhook https://slack.com/...
```

**Experience**: Hourly summary posted to Slack channel, can spot blockers

### Story 3: DevOps Engineer

**Persona**: Jordan, maintaining CI/CD pipelines

**Need**: Trigger builds on specific changes

**Solution**:

```bash
gzh-git watch --format json --files='src/**' | \
  jq -r 'select(.type=="staged")' | \
  xargs -I {} ./trigger-build.sh
```

**Experience**: Automated build triggers, no manual intervention

### Story 4: Open Source Maintainer

**Persona**: Chris, maintaining popular OSS project

**Need**: Track activity across multiple contributors

**Solution**:

```bash
gzh-git watch --compare hourly --history 100
```

**Experience**: See contribution patterns, identify hot files

## Conclusion

### Immediate Actions (v0.2.0 → v0.3.0)

1. ✅ Implement smart filtering (`--files`, `--ignore`)
1. ✅ Add desktop notifications (macOS first)
1. ✅ Document current output formats
1. 📊 Gather user feedback on existing formats

### Research Questions

1. **Survey users**: Which enhancements are most valuable?
1. **Prototype**: Interactive mode and timeline view
1. **Benchmark**: Performance impact of statistics
1. **Design**: Configuration file format

### Decision Deferred

- Interactive mode: Wait for user demand
- AI summarization: Research phase only
- Historical timeline: v1.0.0 consideration

The current implementation provides a solid foundation. Future enhancements should be driven by actual user needs rather than speculative features.

______________________________________________________________________

**Status**: 💭 Discussion Document
**Next Review**: After v0.3.0 beta user feedback
**Owner**: To be assigned based on priority
