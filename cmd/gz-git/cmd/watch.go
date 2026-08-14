package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/watch"
)

var (
	watchPaths        []string
	watchInterval     time.Duration
	watchIncludeClean bool
	watchOutputFormat string
	watchNotifySound  bool
)

// watchCmd represents the watch command.
var watchCmd = &cobra.Command{
	Use:   "watch [paths...]",
	Short: "Monitor repositories for changes",
	Long: cliutil.QuickStartHelp(`  # Watch current directory
  gz-git watch

  # Watch multiple repositories
  gz-git watch /path/to/repo1 /path/to/repo2

  # Watch with custom interval
  gz-git watch --interval 5s

  # Notify when repository becomes clean
  gz-git watch --include-clean

  # Compact output format
  gz-git watch --format compact`),
	Example: ``,
	RunE:    runWatch,
}

func init() {
	rootCmd.AddCommand(watchCmd)

	// Flags
	watchCmd.Flags().DurationVar(&watchInterval, "interval", 2*time.Second, "polling interval for checking changes")
	watchCmd.Flags().BoolVar(&watchIncludeClean, "include-clean", false, "notify when repository becomes clean")
	watchCmd.Flags().StringVar(&watchOutputFormat, "format", "default", "output format: default, compact, json, llm")
	watchCmd.Flags().BoolVar(&watchNotifySound, "notify", false, "play sound on changes (not yet implemented; flag reserved)")
	// Hidden until the sound backend is implemented; passing it emits a
	// "not implemented" warning at startup instead of failing.
	_ = watchCmd.Flags().MarkHidden("notify")
}

func runWatch(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Validate format (reuse centralized validator from bulk_common.go)
	if err := validateBulkFormat(watchOutputFormat); err != nil {
		return err
	}

	// Warn once if --notify was requested: the feature is not implemented yet
	// (real sound support is tracked separately). Goes to stderr so it never
	// pollutes a --format json/llm stdout stream.
	if watchNotifySound {
		fmt.Fprintln(os.Stderr, "warning: --notify is not yet implemented; no sound will play")
	}

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

	// Print header (suppressed for machine formats so stdout stays parseable)
	if shouldShowProgress(watchOutputFormat, quiet) {
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
			if shouldShowProgress(watchOutputFormat, quiet) {
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
	case "llm":
		return &llmFormatter{}
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
	sb.WriteString(fmt.Sprintf("%s[%s]%s ", cliutil.ColorGray, timestamp, cliutil.ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s%s ", cliutil.ColorCyan, filepath.Base(event.Path), cliutil.ColorReset))

	// Event type with color
	switch event.Type {
	case watch.EventTypeModified:
		sb.WriteString(cliutil.ColorYellow + "● Modified" + cliutil.ColorReset)
	case watch.EventTypeStaged:
		sb.WriteString(cliutil.ColorGreen + "● Staged" + cliutil.ColorReset)
	case watch.EventTypeUntracked:
		sb.WriteString(cliutil.ColorMagenta + "● Untracked" + cliutil.ColorReset)
	case watch.EventTypeDeleted:
		sb.WriteString(cliutil.ColorRed + "● Deleted" + cliutil.ColorReset)
	case watch.EventTypeBranch:
		sb.WriteString(cliutil.ColorCyan + "● Branch Changed" + cliutil.ColorReset)
	case watch.EventTypeClean:
		sb.WriteString(cliutil.ColorGreen + "✓ Clean" + cliutil.ColorReset)
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
				sb.WriteString(fmt.Sprintf("    %s... and %d more%s\n", cliutil.ColorGray, remaining, cliutil.ColorReset))
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

// watchEventJSON is the wire format for a watch event.
type watchEventJSON struct {
	Timestamp string   `json:"timestamp"`
	Path      string   `json:"path"`
	Type      string   `json:"type"`
	Files     []string `json:"files"`
}

func (f *jsonFormatter) Format(event watch.Event) string {
	files := event.Files
	if files == nil {
		files = []string{}
	}

	data, err := json.Marshal(watchEventJSON{
		Timestamp: event.Timestamp.Format(time.RFC3339),
		Path:      event.Path,
		Type:      string(event.Type),
		Files:     files,
	})
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`+"\n", err.Error())
	}
	return string(data) + "\n"
}

// llmFormatter provides LLM-friendly structured output.
type llmFormatter struct{}

func (f *llmFormatter) Format(event watch.Event) string {
	var sb strings.Builder

	timestamp := event.Timestamp.Format("2006-01-02 15:04:05")
	repoName := filepath.Base(event.Path)

	sb.WriteString(fmt.Sprintf("## Repository Change Event\n"))
	sb.WriteString(fmt.Sprintf("- Time: %s\n", timestamp))
	sb.WriteString(fmt.Sprintf("- Repository: %s\n", repoName))
	sb.WriteString(fmt.Sprintf("- Path: %s\n", event.Path))
	sb.WriteString(fmt.Sprintf("- Event Type: %s\n", event.Type))

	if len(event.Files) > 0 {
		sb.WriteString(fmt.Sprintf("- Files Changed: %d\n", len(event.Files)))
		sb.WriteString("- File List:\n")
		for _, file := range event.Files {
			sb.WriteString(fmt.Sprintf("  - %s\n", file))
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

// watchLogger implements watch.Logger interface.
type watchLogger struct {
	verbose bool
}

func newWatchLogger(verbose bool) *watchLogger {
	return &watchLogger{verbose: verbose}
}

func (l *watchLogger) Debug(format string, args ...any) {
	if l.verbose {
		fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
	}
}

func (l *watchLogger) Info(format string, args ...any) {
	if l.verbose {
		fmt.Fprintf(os.Stderr, "[INFO] "+format+"\n", args...)
	}
}

func (l *watchLogger) Warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[WARN] "+format+"\n", args...)
}

func (l *watchLogger) Error(format string, args ...any) {
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
//
//	macOS: exec.Command("afplay", "/System/Library/Sounds/Ping.aiff").Run()
//	Linux: exec.Command("paplay", "/usr/share/sounds/freedesktop/stereo/message.oga").Run()
//
// Reference: docs/design/WATCH_OUTPUT_IMPROVEMENTS.md (Phase 1)
// Priority: Medium (P2) - Planned for v0.3.0.
func playNotificationSound() {
	// Placeholder - feature not yet implemented
	_ = playSystemSound()
}

func playSystemSound() error {
	// Platform-specific implementation pending
	// See TODO above for planned approach
	return nil
}
