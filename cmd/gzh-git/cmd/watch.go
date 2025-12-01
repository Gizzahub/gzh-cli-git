package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-git/pkg/repository"
	"github.com/gizzahub/gzh-cli-git/pkg/watch"
)

var (
	watchPaths          []string
	watchInterval       time.Duration
	watchIncludeClean   bool
	watchOutputFormat   string
	watchNotifySound    bool
)

// watchCmd represents the watch command
var watchCmd = &cobra.Command{
	Use:   "watch [paths...]",
	Short: "Monitor repositories for changes",
	Long: `Monitor one or more Git repositories for changes in real-time.

The watch command continuously monitors specified repositories and reports
changes as they happen, including:
  - Modified files (unstaged changes)
  - Staged files (ready to commit)
  - Untracked files (new files)
  - Deleted files
  - Branch switches
  - Repository becoming clean

By default, it monitors the current directory. You can specify multiple
repository paths to monitor them simultaneously.`,
	Example: `  # Watch current directory
  gzh-git watch

  # Watch specific repository
  gzh-git watch /path/to/repo

  # Watch multiple repositories
  gzh-git watch /path/to/repo1 /path/to/repo2

  # Watch with custom interval (default: 2s)
  gzh-git watch --interval 5s

  # Include notifications when repository becomes clean
  gzh-git watch --include-clean

  # Compact output format
  gzh-git watch --format compact

  # JSON output format (machine-readable)
  gzh-git watch --format json`,
	RunE: runWatch,
}

func init() {
	rootCmd.AddCommand(watchCmd)

	// Flags
	watchCmd.Flags().DurationVar(&watchInterval, "interval", 2*time.Second, "polling interval for checking changes")
	watchCmd.Flags().BoolVar(&watchIncludeClean, "include-clean", false, "notify when repository becomes clean")
	watchCmd.Flags().StringVar(&watchOutputFormat, "format", "default", "output format (default, compact, json)")
	watchCmd.Flags().BoolVar(&watchNotifySound, "notify", false, "play sound on changes (macOS/Linux)")
}

func runWatch(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Determine paths to watch
	paths := watchPaths
	if len(args) > 0 {
		paths = args
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}

	// Resolve absolute paths
	absPaths := make([]string, 0, len(paths))
	for _, path := range paths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("failed to resolve path %s: %w", path, err)
		}
		absPaths = append(absPaths, absPath)
	}

	// Create repository client
	repoClient := repository.NewClient()

	// Validate all paths are repositories
	for _, path := range absPaths {
		if !repoClient.IsRepository(ctx, path) {
			return fmt.Errorf("not a git repository: %s", path)
		}
	}

	// Create watcher
	watcher, err := watch.NewWatcher(repoClient, watch.WatchOptions{
		Interval:         watchInterval,
		IncludeClean:     watchIncludeClean,
		DebounceDuration: 500 * time.Millisecond,
		Logger:           newWatchLogger(verbose),
	})
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}
	defer watcher.Stop()

	// Start watching
	if err := watcher.Start(ctx, absPaths); err != nil {
		return fmt.Errorf("failed to start watching: %w", err)
	}

	// Print header
	if !quiet {
		fmt.Printf("Watching %d repositor%s for changes (interval: %s)\n",
			len(absPaths),
			pluralize(len(absPaths), "y", "ies"),
			watchInterval)
		for _, path := range absPaths {
			fmt.Printf("  - %s\n", path)
		}
		fmt.Println("\nPress Ctrl+C to stop...")
		fmt.Println()
	}

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create event formatter
	formatter := newEventFormatter(watchOutputFormat)

	// Event loop
	for {
		select {
		case <-sigChan:
			if !quiet {
				fmt.Println("\nStopping watch...")
			}
			return nil

		case event, ok := <-watcher.Events():
			if !ok {
				return nil
			}

			// Format and print event
			output := formatter.Format(event)
			fmt.Print(output)

			// Play notification sound if requested
			if watchNotifySound {
				playNotificationSound()
			}

		case err, ok := <-watcher.Errors():
			if !ok {
				return nil
			}
			if !quiet {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
		}
	}
}

// eventFormatter formats watch events for display.
type eventFormatter interface {
	Format(event watch.Event) string
}

// newEventFormatter creates a formatter for the specified format.
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

// defaultFormatter provides detailed, human-readable output.
type defaultFormatter struct{}

func (f *defaultFormatter) Format(event watch.Event) string {
	var sb strings.Builder

	// Timestamp and repository
	timestamp := event.Timestamp.Format("15:04:05")
	sb.WriteString(fmt.Sprintf("\x1b[90m[%s]\x1b[0m ", timestamp))
	sb.WriteString(fmt.Sprintf("\x1b[36m%s\x1b[0m ", filepath.Base(event.Path)))

	// Event type with color
	switch event.Type {
	case watch.EventTypeModified:
		sb.WriteString("\x1b[33m● Modified\x1b[0m")
	case watch.EventTypeStaged:
		sb.WriteString("\x1b[32m● Staged\x1b[0m")
	case watch.EventTypeUntracked:
		sb.WriteString("\x1b[35m● Untracked\x1b[0m")
	case watch.EventTypeDeleted:
		sb.WriteString("\x1b[31m● Deleted\x1b[0m")
	case watch.EventTypeBranch:
		sb.WriteString("\x1b[36m● Branch Changed\x1b[0m")
	case watch.EventTypeClean:
		sb.WriteString("\x1b[32m✓ Clean\x1b[0m")
	default:
		sb.WriteString(fmt.Sprintf("● %s", event.Type))
	}

	// File count
	if len(event.Files) > 0 {
		sb.WriteString(fmt.Sprintf(" (%d file%s)", len(event.Files), pluralize(len(event.Files), "", "s")))
	}

	sb.WriteString("\n")

	// Show files (limit to first 5)
	if len(event.Files) > 0 {
		maxFiles := 5
		for i, file := range event.Files {
			if i >= maxFiles {
				remaining := len(event.Files) - maxFiles
				sb.WriteString(fmt.Sprintf("    \x1b[90m... and %d more\x1b[0m\n", remaining))
				break
			}
			sb.WriteString(fmt.Sprintf("    %s\n", file))
		}
	}

	return sb.String()
}

// compactFormatter provides minimal, single-line output.
type compactFormatter struct{}

func (f *compactFormatter) Format(event watch.Event) string {
	timestamp := event.Timestamp.Format("15:04:05")
	repoName := filepath.Base(event.Path)

	fileInfo := ""
	if len(event.Files) > 0 {
		fileInfo = fmt.Sprintf(" [%d]", len(event.Files))
	}

	return fmt.Sprintf("[%s] %s: %s%s\n", timestamp, repoName, event.Type, fileInfo)
}

// jsonFormatter provides JSON output for machine parsing.
type jsonFormatter struct{}

func (f *jsonFormatter) Format(event watch.Event) string {
	// Simplified JSON for now
	files := "[]"
	if len(event.Files) > 0 {
		fileList := make([]string, len(event.Files))
		for i, file := range event.Files {
			fileList[i] = fmt.Sprintf("%q", file)
		}
		files = "[" + strings.Join(fileList, ",") + "]"
	}

	return fmt.Sprintf(`{"timestamp":"%s","path":"%s","type":"%s","files":%s}`+"\n",
		event.Timestamp.Format(time.RFC3339),
		event.Path,
		event.Type,
		files)
}

// watchLogger implements watch.Logger interface.
type watchLogger struct {
	verbose bool
}

func newWatchLogger(verbose bool) *watchLogger {
	return &watchLogger{verbose: verbose}
}

func (l *watchLogger) Debug(format string, args ...interface{}) {
	if l.verbose {
		fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
	}
}

func (l *watchLogger) Info(format string, args ...interface{}) {
	if l.verbose {
		fmt.Fprintf(os.Stderr, "[INFO] "+format+"\n", args...)
	}
}

func (l *watchLogger) Warn(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[WARN] "+format+"\n", args...)
}

func (l *watchLogger) Error(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[ERROR] "+format+"\n", args...)
}

// pluralize returns singular or plural suffix based on count.
func pluralize(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

// playNotificationSound plays a system notification sound.
// TODO(feature): Implement platform-specific sound notifications
//
// Planned implementation:
// - macOS: Use 'afplay /System/Library/Sounds/Ping.aiff'
// - Linux: Use 'paplay' or 'aplay' with system sounds
// - Windows: Use Windows Beep API via syscall
//
// Example:
//   macOS: exec.Command("afplay", "/System/Library/Sounds/Ping.aiff").Run()
//   Linux: exec.Command("paplay", "/usr/share/sounds/freedesktop/stereo/message.oga").Run()
//
// Reference: docs/design/WATCH_OUTPUT_IMPROVEMENTS.md (Phase 1)
// Priority: Medium (P2) - Planned for v0.3.0
func playNotificationSound() {
	// Placeholder - feature not yet implemented
	_ = playSystemSound()
}

func playSystemSound() error {
	// Platform-specific implementation pending
	// See TODO above for planned approach
	return nil
}
