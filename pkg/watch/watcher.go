package watch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

// watcher implements the Watcher interface using fsnotify.
type watcher struct {
	client   repository.Client
	fswatch  *fsnotify.Watcher
	options  WatchOptions
	events   chan Event
	errors   chan error
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mu       sync.RWMutex
	watching map[string]*repoState
	logger   Logger
}

// repoState tracks the state of a watched repository.
type repoState struct {
	path         string
	lastStatus   *repository.Status
	lastEventAt  time.Time
	currentBranch string
}

// NewWatcher creates a new repository watcher with the given options.
//
// Example:
//
//	watcher, err := watch.NewWatcher(repoClient, watch.WatchOptions{
//	    Interval:         5 * time.Second,
//	    IncludeClean:     true,
//	    DebounceDuration: 500 * time.Millisecond,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer watcher.Stop()
func NewWatcher(client repository.Client, options WatchOptions) (Watcher, error) {
	// Set defaults
	if options.Interval == 0 {
		options.Interval = 2 * time.Second
	}
	if options.DebounceDuration == 0 {
		options.DebounceDuration = 500 * time.Millisecond
	}
	if options.Logger == nil {
		options.Logger = &noopLogger{}
	}

	// Create fsnotify watcher
	fswatch, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	w := &watcher{
		client:   client,
		fswatch:  fswatch,
		options:  options,
		events:   make(chan Event, 100),
		errors:   make(chan error, 50), // Increased buffer to prevent blocking
		watching: make(map[string]*repoState),
		logger:   options.Logger,
	}

	return w, nil
}

// Start begins monitoring the specified repository paths.
func (w *watcher) Start(ctx context.Context, paths []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Create cancellable context
	ctx, w.cancel = context.WithCancel(ctx)

	// Initialize watch state for each repository
	for _, path := range paths {
		// Open repository
		repo, err := w.client.Open(ctx, path)
		if err != nil {
			return fmt.Errorf("failed to open repository %s: %w", path, err)
		}

		// Get initial status
		status, err := w.client.GetStatus(ctx, repo)
		if err != nil {
			return fmt.Errorf("failed to get status for %s: %w", path, err)
		}

		// Get initial branch
		info, err := w.client.GetInfo(ctx, repo)
		if err != nil {
			w.logger.Warn("failed to get info for %s: %v", path, err)
		}

		branch := ""
		if info != nil {
			branch = info.Branch
		}

		// Initialize state
		w.watching[path] = &repoState{
			path:          path,
			lastStatus:    status,
			lastEventAt:   time.Time{},
			currentBranch: branch,
		}

		// Add path to fsnotify
		if err := w.fswatch.Add(path); err != nil {
			return fmt.Errorf("failed to watch path %s: %w", path, err)
		}

		w.logger.Info("Started watching repository: %s", path)
	}

	// Start event loop
	w.wg.Add(1)
	go w.eventLoop(ctx)

	return nil
}

// Events returns the channel for receiving watch events.
func (w *watcher) Events() <-chan Event {
	return w.events
}

// Errors returns the channel for receiving errors.
func (w *watcher) Errors() <-chan error {
	return w.errors
}

// Stop stops the watcher and closes all channels.
func (w *watcher) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancel != nil {
		w.cancel()
	}

	// Close fsnotify
	if w.fswatch != nil {
		if err := w.fswatch.Close(); err != nil {
			return fmt.Errorf("failed to close file watcher: %w", err)
		}
	}

	// Wait for event loop to finish
	w.wg.Wait()

	// Close channels
	close(w.events)
	close(w.errors)

	w.logger.Info("Stopped watching all repositories")

	return nil
}

// eventLoop is the main event processing loop.
func (w *watcher) eventLoop(ctx context.Context) {
	defer w.wg.Done()

	ticker := time.NewTicker(w.options.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			// Poll all watched repositories
			w.pollRepositories(ctx)

		case fsEvent, ok := <-w.fswatch.Events:
			if !ok {
				return
			}
			w.logger.Debug("File system event: %s %s", fsEvent.Op, fsEvent.Name)
			// Trigger immediate check for affected repository
			w.checkRepository(ctx, w.findRepoForPath(fsEvent.Name))

		case err, ok := <-w.fswatch.Errors:
			if !ok {
				return
			}
			select {
			case w.errors <- err:
			case <-ctx.Done():
				return
			}
		}
	}
}

// pollRepositories checks all watched repositories for changes.
func (w *watcher) pollRepositories(ctx context.Context) {
	w.mu.RLock()
	paths := make([]string, 0, len(w.watching))
	for path := range w.watching {
		paths = append(paths, path)
	}
	w.mu.RUnlock()

	for _, path := range paths {
		w.checkRepository(ctx, path)
	}
}

// checkRepository checks a single repository for changes.
func (w *watcher) checkRepository(ctx context.Context, path string) {
	if path == "" {
		return
	}

	w.mu.RLock()
	state := w.watching[path]
	w.mu.RUnlock()

	if state == nil {
		return
	}

	// Check debounce
	if time.Since(state.lastEventAt) < w.options.DebounceDuration {
		return
	}

	// Open repository
	repo, err := w.client.Open(ctx, path)
	if err != nil {
		select {
		case w.errors <- fmt.Errorf("failed to open repository %s: %w", path, err):
		case <-ctx.Done():
		default:
			// Error channel full, log and continue
			w.logger.Warn("Error channel full, dropping error: failed to open repository %s: %v", path, err)
		}
		return
	}

	// Get current status
	status, err := w.client.GetStatus(ctx, repo)
	if err != nil {
		select {
		case w.errors <- fmt.Errorf("failed to get status for %s: %w", path, err):
		case <-ctx.Done():
		default:
			// Error channel full, log and continue
			w.logger.Warn("Error channel full, dropping error: failed to get status for %s: %v", path, err)
		}
		return
	}

	// Get current branch
	info, err := w.client.GetInfo(ctx, repo)
	currentBranch := state.currentBranch
	if err == nil && info != nil {
		currentBranch = info.Branch
	}

	// Detect changes
	events := w.detectChanges(state, status, currentBranch)

	// Send events
	for _, event := range events {
		select {
		case w.events <- event:
			state.lastEventAt = time.Now()
		case <-ctx.Done():
			return
		}
	}

	// Update state
	w.mu.Lock()
	state.lastStatus = status
	state.currentBranch = currentBranch
	w.mu.Unlock()
}

// detectChanges compares old and new status to detect change events.
func (w *watcher) detectChanges(state *repoState, newStatus *repository.Status, newBranch string) []Event {
	var events []Event
	now := time.Now()

	oldStatus := state.lastStatus

	// Branch change
	if newBranch != state.currentBranch && newBranch != "" {
		events = append(events, Event{
			Path:      state.path,
			Type:      EventTypeBranch,
			Timestamp: now,
			Status:    newStatus,
		})
	}

	// Check if became clean
	if !oldStatus.IsClean && newStatus.IsClean {
		if w.options.IncludeClean {
			events = append(events, Event{
				Path:      state.path,
				Type:      EventTypeClean,
				Timestamp: now,
				Status:    newStatus,
			})
		}
		return events // Clean state, no other changes to report
	}

	// Staged files changed
	if !equalStringSlices(oldStatus.StagedFiles, newStatus.StagedFiles) {
		events = append(events, Event{
			Path:      state.path,
			Type:      EventTypeStaged,
			Timestamp: now,
			Status:    newStatus,
			Files:     newStatus.StagedFiles,
		})
	}

	// Modified files changed
	if !equalStringSlices(oldStatus.ModifiedFiles, newStatus.ModifiedFiles) {
		events = append(events, Event{
			Path:      state.path,
			Type:      EventTypeModified,
			Timestamp: now,
			Status:    newStatus,
			Files:     newStatus.ModifiedFiles,
		})
	}

	// Untracked files changed
	if !equalStringSlices(oldStatus.UntrackedFiles, newStatus.UntrackedFiles) {
		events = append(events, Event{
			Path:      state.path,
			Type:      EventTypeUntracked,
			Timestamp: now,
			Status:    newStatus,
			Files:     newStatus.UntrackedFiles,
		})
	}

	// Deleted files changed
	if !equalStringSlices(oldStatus.DeletedFiles, newStatus.DeletedFiles) {
		events = append(events, Event{
			Path:      state.path,
			Type:      EventTypeDeleted,
			Timestamp: now,
			Status:    newStatus,
			Files:     newStatus.DeletedFiles,
		})
	}

	return events
}

// findRepoForPath finds which repository a file path belongs to.
func (w *watcher) findRepoForPath(filePath string) string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	// Simple prefix match for now
	// TODO: More sophisticated path matching
	for repoPath := range w.watching {
		if len(filePath) >= len(repoPath) && filePath[:len(repoPath)] == repoPath {
			return repoPath
		}
	}

	return ""
}

// equalStringSlices compares two string slices for equality.
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// noopLogger is a logger that does nothing.
type noopLogger struct{}

func (l *noopLogger) Debug(format string, args ...interface{}) {}
func (l *noopLogger) Info(format string, args ...interface{})  {}
func (l *noopLogger) Warn(format string, args ...interface{})  {}
func (l *noopLogger) Error(format string, args ...interface{}) {}
