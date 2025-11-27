package history

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gizzahub/gzh-cli-git/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

// GitExecutor defines the interface for executing git commands
type GitExecutor interface {
	Run(ctx context.Context, repoPath string, args ...string) (*gitcmd.Result, error)
}

// HistoryAnalyzer analyzes commit history
type HistoryAnalyzer interface {
	Analyze(ctx context.Context, repo *repository.Repository, opts AnalyzeOptions) (*CommitStats, error)
	GetTrends(ctx context.Context, repo *repository.Repository, opts TrendOptions) (*CommitTrends, error)
}

type historyAnalyzer struct {
	executor GitExecutor
}

// NewHistoryAnalyzer creates a new history analyzer
func NewHistoryAnalyzer(executor *gitcmd.Executor) HistoryAnalyzer {
	return &historyAnalyzer{
		executor: executor,
	}
}

// Analyze analyzes commit history and returns statistics
func (h *historyAnalyzer) Analyze(ctx context.Context, repo *repository.Repository, opts AnalyzeOptions) (*CommitStats, error) {
	// Validate options
	if err := h.validateOptions(opts); err != nil {
		return nil, err
	}

	// Build git log command
	args := []string{"log", "--format=%H|%an|%ae|%ct", "--shortstat"}

	if !opts.Since.IsZero() {
		args = append(args, fmt.Sprintf("--since=%s", opts.Since.Format(time.RFC3339)))
	}

	if !opts.Until.IsZero() {
		args = append(args, fmt.Sprintf("--until=%s", opts.Until.Format(time.RFC3339)))
	}

	if opts.Branch != "" {
		args = append(args, opts.Branch)
	}

	if opts.Author != "" {
		args = append(args, fmt.Sprintf("--author=%s", opts.Author))
	}

	if opts.MaxCommits > 0 {
		args = append(args, fmt.Sprintf("--max-count=%d", opts.MaxCommits))
	}

	// Execute git log
	result, err := h.executor.Run(ctx, repo.Path, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit log: %w", err)
	}

	// Parse output
	stats, err := h.parseCommitStats(result.Stdout)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

// GetTrends analyzes commit trends over time
func (h *historyAnalyzer) GetTrends(ctx context.Context, repo *repository.Repository, opts TrendOptions) (*CommitTrends, error) {
	// Build git log command
	args := []string{"log", "--format=%ct"}

	if !opts.Since.IsZero() {
		args = append(args, fmt.Sprintf("--since=%s", opts.Since.Format(time.RFC3339)))
	}

	if !opts.Until.IsZero() {
		args = append(args, fmt.Sprintf("--until=%s", opts.Until.Format(time.RFC3339)))
	}

	if opts.Branch != "" {
		args = append(args, opts.Branch)
	}

	// Execute git log
	result, err := h.executor.Run(ctx, repo.Path, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit log: %w", err)
	}

	// Parse output
	trends, err := h.parseCommitTrends(result.Stdout)
	if err != nil {
		return nil, err
	}

	return trends, nil
}

func (h *historyAnalyzer) validateOptions(opts AnalyzeOptions) error {
	if !opts.Since.IsZero() && !opts.Until.IsZero() && opts.Since.After(opts.Until) {
		return ErrInvalidDateRange
	}
	return nil
}

func (h *historyAnalyzer) parseCommitStats(output string) (*CommitStats, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return nil, ErrEmptyHistory
	}

	stats := &CommitStats{}
	authors := make(map[string]bool)
	dailyCounts := make(map[string]int)
	var timestamps []time.Time

	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			i++
			continue
		}

		// Parse commit line: hash|author|email|timestamp
		parts := strings.Split(line, "|")
		if len(parts) != 4 {
			i++
			continue
		}

		authorEmail := parts[2]
		timestampStr := parts[3]

		// Parse timestamp
		timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
		if err != nil {
			i++
			continue
		}

		commitTime := time.Unix(timestamp, 0)
		timestamps = append(timestamps, commitTime)

		// Track unique authors
		authors[authorEmail] = true

		// Track daily counts
		dateKey := commitTime.Format("2006-01-02")
		dailyCounts[dateKey]++

		stats.TotalCommits++

		// Move to next line, skip empty lines, and look for shortstat
		i++
		for i < len(lines) {
			statLine := strings.TrimSpace(lines[i])
			if statLine == "" {
				i++
				continue
			}
			// Parse shortstat line if present
			if strings.Contains(statLine, "changed") {
				additions, deletions := h.parseShortstat(statLine)
				stats.TotalAdditions += additions
				stats.TotalDeletions += deletions
				i++
			}
			break
		}
	}

	if stats.TotalCommits == 0 {
		return nil, ErrEmptyHistory
	}

	// Calculate derived statistics
	stats.UniqueAuthors = len(authors)

	if len(timestamps) > 0 {
		// Find first (oldest) and last (newest) commits
		// Git log shows newest first, so timestamps[0] is most recent
		var firstCommit, lastCommit time.Time
		for _, ts := range timestamps {
			if firstCommit.IsZero() || ts.Before(firstCommit) {
				firstCommit = ts
			}
			if lastCommit.IsZero() || ts.After(lastCommit) {
				lastCommit = ts
			}
		}

		stats.FirstCommit = firstCommit
		stats.LastCommit = lastCommit
		stats.DateRange = stats.LastCommit.Sub(stats.FirstCommit)

		// Calculate averages
		days := stats.DateRange.Hours() / 24
		if days < 1 {
			days = 1 // Minimum 1 day to avoid division by zero
		}
		stats.AvgPerDay = float64(stats.TotalCommits) / days
		stats.AvgPerWeek = stats.AvgPerDay * 7
		stats.AvgPerMonth = stats.AvgPerDay * 30

		// Find peak day
		maxCount := 0
		for date, count := range dailyCounts {
			if count > maxCount {
				maxCount = count
				peakTime, _ := time.Parse("2006-01-02", date)
				stats.PeakDay = peakTime
				stats.PeakCount = count
			}
		}
	}

	return stats, nil
}

func (h *historyAnalyzer) parseShortstat(line string) (additions, deletions int) {
	// Example: " 2 files changed, 10 insertions(+), 5 deletions(-)"
	parts := strings.Split(line, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)

		if strings.Contains(part, "insertion") {
			fields := strings.Fields(part)
			if len(fields) >= 1 {
				additions, _ = strconv.Atoi(fields[0])
			}
		}

		if strings.Contains(part, "deletion") {
			fields := strings.Fields(part)
			if len(fields) >= 1 {
				deletions, _ = strconv.Atoi(fields[0])
			}
		}
	}

	return
}

func (h *historyAnalyzer) parseCommitTrends(output string) (*CommitTrends, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return nil, ErrEmptyHistory
	}

	trends := &CommitTrends{
		Daily:   make(map[string]int),
		Weekly:  make(map[string]int),
		Monthly: make(map[string]int),
		Hourly:  make(map[int]int),
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		timestamp, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			continue
		}

		commitTime := time.Unix(timestamp, 0)

		// Daily trend (YYYY-MM-DD)
		dailyKey := commitTime.Format("2006-01-02")
		trends.Daily[dailyKey]++

		// Weekly trend (YYYY-WW)
		year, week := commitTime.ISOWeek()
		weeklyKey := fmt.Sprintf("%04d-W%02d", year, week)
		trends.Weekly[weeklyKey]++

		// Monthly trend (YYYY-MM)
		monthlyKey := commitTime.Format("2006-01")
		trends.Monthly[monthlyKey]++

		// Hourly trend (0-23)
		hour := commitTime.Hour()
		trends.Hourly[hour]++
	}

	return trends, nil
}
