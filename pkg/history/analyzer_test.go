package history

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gizzahub/gzh-cli-git/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

// mockExecutor is a test mock for gitcmd.Executor
type mockExecutor struct {
	runFunc func(ctx context.Context, repoPath string, args ...string) (*gitcmd.Result, error)
}

func (m *mockExecutor) Run(ctx context.Context, repoPath string, args ...string) (*gitcmd.Result, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, repoPath, args...)
	}
	return &gitcmd.Result{}, nil
}

func TestHistoryAnalyzer_Analyze(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		opts    AnalyzeOptions
		want    *CommitStats
		wantErr error
	}{
		{
			name: "basic stats",
			output: `abc123|John Doe|john@example.com|1700000000

 2 files changed, 10 insertions(+), 5 deletions(-)
def456|Jane Smith|jane@example.com|1700001000

 1 file changed, 3 insertions(+)`,
			opts: AnalyzeOptions{},
			want: &CommitStats{
				TotalCommits:   2,
				UniqueAuthors:  2,
				TotalAdditions: 13,
				TotalDeletions: 5,
			},
		},
		{
			name: "single commit",
			output: `abc123|John Doe|john@example.com|1700000000

 1 file changed, 5 insertions(+), 2 deletions(-)`,
			opts: AnalyzeOptions{},
			want: &CommitStats{
				TotalCommits:   1,
				UniqueAuthors:  1,
				TotalAdditions: 5,
				TotalDeletions: 2,
			},
		},
		{
			name:    "empty history",
			output:  "",
			opts:    AnalyzeOptions{},
			want:    nil,
			wantErr: ErrEmptyHistory,
		},
		{
			name: "same author multiple commits",
			output: `abc123|John Doe|john@example.com|1700000000

 1 file changed, 5 insertions(+)
def456|John Doe|john@example.com|1700001000

 1 file changed, 3 insertions(+)`,
			opts: AnalyzeOptions{},
			want: &CommitStats{
				TotalCommits:   2,
				UniqueAuthors:  1,
				TotalAdditions: 8,
				TotalDeletions: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockExecutor{
				runFunc: func(ctx context.Context, repoPath string, args ...string) (*gitcmd.Result, error) {
					if tt.wantErr != nil && tt.wantErr != ErrEmptyHistory {
						return nil, tt.wantErr
					}
					return &gitcmd.Result{
						Stdout:   tt.output,
						Stderr:   "",
						ExitCode: 0,
					}, nil
				},
			}

			analyzer := &historyAnalyzer{executor: executor}
			repo := &repository.Repository{Path: "/test/repo"}

			got, err := analyzer.Analyze(context.Background(), repo, tt.opts)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("Analyze() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Analyze() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("Analyze() unexpected error = %v", err)
				return
			}

			if got.TotalCommits != tt.want.TotalCommits {
				t.Errorf("TotalCommits = %d, want %d", got.TotalCommits, tt.want.TotalCommits)
			}

			if got.UniqueAuthors != tt.want.UniqueAuthors {
				t.Errorf("UniqueAuthors = %d, want %d", got.UniqueAuthors, tt.want.UniqueAuthors)
			}

			if got.TotalAdditions != tt.want.TotalAdditions {
				t.Errorf("TotalAdditions = %d, want %d", got.TotalAdditions, tt.want.TotalAdditions)
			}

			if got.TotalDeletions != tt.want.TotalDeletions {
				t.Errorf("TotalDeletions = %d, want %d", got.TotalDeletions, tt.want.TotalDeletions)
			}
		})
	}
}

func TestHistoryAnalyzer_AnalyzeWithOptions(t *testing.T) {
	tests := []struct {
		name     string
		opts     AnalyzeOptions
		wantArgs []string
	}{
		{
			name: "with since",
			opts: AnalyzeOptions{
				Since: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantArgs: []string{"log", "--format=%H|%an|%ae|%ct", "--shortstat", "--since=2025-01-01T00:00:00Z"},
		},
		{
			name: "with until",
			opts: AnalyzeOptions{
				Until: time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
			},
			wantArgs: []string{"log", "--format=%H|%an|%ae|%ct", "--shortstat", "--until=2025-12-31T23:59:59Z"},
		},
		{
			name: "with branch",
			opts: AnalyzeOptions{
				Branch: "main",
			},
			wantArgs: []string{"log", "--format=%H|%an|%ae|%ct", "--shortstat", "main"},
		},
		{
			name: "with author",
			opts: AnalyzeOptions{
				Author: "John Doe",
			},
			wantArgs: []string{"log", "--format=%H|%an|%ae|%ct", "--shortstat", "--author=John Doe"},
		},
		{
			name: "with max commits",
			opts: AnalyzeOptions{
				MaxCommits: 100,
			},
			wantArgs: []string{"log", "--format=%H|%an|%ae|%ct", "--shortstat", "--max-count=100"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedArgs []string
			executor := &mockExecutor{
				runFunc: func(ctx context.Context, repoPath string, args ...string) (*gitcmd.Result, error) {
					capturedArgs = args
					return &gitcmd.Result{
						Stdout: `abc123|John Doe|john@example.com|1700000000

 1 file changed, 1 insertion(+)`,
						Stderr:   "",
						ExitCode: 0,
					}, nil
				},
			}

			analyzer := &historyAnalyzer{executor: executor}
			repo := &repository.Repository{Path: "/test/repo"}

			_, err := analyzer.Analyze(context.Background(), repo, tt.opts)
			if err != nil {
				t.Fatalf("Analyze() unexpected error = %v", err)
			}

			if len(capturedArgs) != len(tt.wantArgs) {
				t.Errorf("len(args) = %d, want %d", len(capturedArgs), len(tt.wantArgs))
				t.Errorf("got args: %v", capturedArgs)
				t.Errorf("want args: %v", tt.wantArgs)
				return
			}

			for i, arg := range capturedArgs {
				if arg != tt.wantArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, arg, tt.wantArgs[i])
				}
			}
		})
	}
}

func TestHistoryAnalyzer_ValidateOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    AnalyzeOptions
		wantErr error
	}{
		{
			name: "valid date range",
			opts: AnalyzeOptions{
				Since: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				Until: time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
			},
			wantErr: nil,
		},
		{
			name: "invalid date range",
			opts: AnalyzeOptions{
				Since: time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
				Until: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: ErrInvalidDateRange,
		},
		{
			name:    "no dates",
			opts:    AnalyzeOptions{},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := &historyAnalyzer{}
			err := analyzer.validateOptions(tt.opts)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("validateOptions() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("validateOptions() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("validateOptions() unexpected error = %v", err)
			}
		})
	}
}

func TestHistoryAnalyzer_ParseShortstat(t *testing.T) {
	tests := []struct {
		name             string
		line             string
		wantAdditions    int
		wantDeletions    int
	}{
		{
			name:          "both insertions and deletions",
			line:          " 2 files changed, 10 insertions(+), 5 deletions(-)",
			wantAdditions: 10,
			wantDeletions: 5,
		},
		{
			name:          "only insertions",
			line:          " 1 file changed, 3 insertions(+)",
			wantAdditions: 3,
			wantDeletions: 0,
		},
		{
			name:          "only deletions",
			line:          " 1 file changed, 7 deletions(-)",
			wantAdditions: 0,
			wantDeletions: 7,
		},
		{
			name:          "no changes",
			line:          " 1 file changed",
			wantAdditions: 0,
			wantDeletions: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := &historyAnalyzer{}
			additions, deletions := analyzer.parseShortstat(tt.line)

			if additions != tt.wantAdditions {
				t.Errorf("additions = %d, want %d", additions, tt.wantAdditions)
			}

			if deletions != tt.wantDeletions {
				t.Errorf("deletions = %d, want %d", deletions, tt.wantDeletions)
			}
		})
	}
}

func TestHistoryAnalyzer_GetTrends(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		opts        TrendOptions
		wantDaily   int
		wantWeekly  int
		wantMonthly int
		wantHourly  int
		wantErr     error
	}{
		{
			name: "basic trends",
			output: `1700000000
1700001000
1700002000`,
			opts:        TrendOptions{},
			wantDaily:   1, // All on same day
			wantWeekly:  1, // All in same week
			wantMonthly: 1, // All in same month
			wantHourly:  3, // Could be different hours
		},
		{
			name:    "empty history",
			output:  "",
			opts:    TrendOptions{},
			wantErr: ErrEmptyHistory,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockExecutor{
				runFunc: func(ctx context.Context, repoPath string, args ...string) (*gitcmd.Result, error) {
					if tt.wantErr != nil && tt.wantErr != ErrEmptyHistory {
						return nil, tt.wantErr
					}
					return &gitcmd.Result{
						Stdout:   tt.output,
						Stderr:   "",
						ExitCode: 0,
					}, nil
				},
			}

			analyzer := &historyAnalyzer{executor: executor}
			repo := &repository.Repository{Path: "/test/repo"}

			got, err := analyzer.GetTrends(context.Background(), repo, tt.opts)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("GetTrends() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("GetTrends() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("GetTrends() unexpected error = %v", err)
				return
			}

			if len(got.Daily) != tt.wantDaily {
				t.Errorf("len(Daily) = %d, want %d", len(got.Daily), tt.wantDaily)
			}

			if len(got.Weekly) != tt.wantWeekly {
				t.Errorf("len(Weekly) = %d, want %d", len(got.Weekly), tt.wantWeekly)
			}

			if len(got.Monthly) != tt.wantMonthly {
				t.Errorf("len(Monthly) = %d, want %d", len(got.Monthly), tt.wantMonthly)
			}

			// Hourly can vary, just check it's populated
			if tt.wantHourly > 0 && len(got.Hourly) == 0 {
				t.Errorf("len(Hourly) = 0, want > 0")
			}
		})
	}
}

func TestHistoryAnalyzer_ParseCommitTrends(t *testing.T) {
	analyzer := &historyAnalyzer{}

	output := `1700000000
1700086400
1700172800`

	trends, err := analyzer.parseCommitTrends(output)
	if err != nil {
		t.Fatalf("parseCommitTrends() unexpected error = %v", err)
	}

	if len(trends.Daily) == 0 {
		t.Error("Daily trends should not be empty")
	}

	if len(trends.Weekly) == 0 {
		t.Error("Weekly trends should not be empty")
	}

	if len(trends.Monthly) == 0 {
		t.Error("Monthly trends should not be empty")
	}

	if len(trends.Hourly) == 0 {
		t.Error("Hourly trends should not be empty")
	}
}

func TestHistoryAnalyzer_ParseCommitStats_Averages(t *testing.T) {
	// Create commits over 7 days (1 week)
	baseTime := time.Date(2025, 11, 1, 12, 0, 0, 0, time.UTC).Unix()
	var lines []string

	// 14 commits over 7 days = 2 per day average
	for i := 0; i < 14; i++ {
		timestamp := baseTime + int64(i*43200) // Every 12 hours
		lines = append(lines, fmt.Sprintf("hash%d|Author|author@example.com|%d", i, timestamp))
		lines = append(lines, "") // Empty line between commits
		lines = append(lines, " 1 file changed, 1 insertion(+)")
	}

	output := strings.Join(lines, "\n")

	analyzer := &historyAnalyzer{}
	stats, err := analyzer.parseCommitStats(output)
	if err != nil {
		t.Fatalf("parseCommitStats() unexpected error = %v", err)
	}

	if stats.TotalCommits != 14 {
		t.Errorf("TotalCommits = %d, want 14", stats.TotalCommits)
	}

	// Check that averages are calculated (approximately 2 per day)
	if stats.AvgPerDay < 1.5 || stats.AvgPerDay > 2.5 {
		t.Errorf("AvgPerDay = %f, want ~2.0", stats.AvgPerDay)
	}

	if stats.AvgPerWeek < 10 || stats.AvgPerWeek > 18 {
		t.Errorf("AvgPerWeek = %f, want ~14.0", stats.AvgPerWeek)
	}
}

func TestHistoryAnalyzer_ParseCommitStats_PeakDay(t *testing.T) {
	baseTime := time.Date(2025, 11, 1, 12, 0, 0, 0, time.UTC).Unix()

	output := fmt.Sprintf(`hash1|Author|author@example.com|%d

 1 file changed, 1 insertion(+)
hash2|Author|author@example.com|%d

 1 file changed, 1 insertion(+)
hash3|Author|author@example.com|%d

 1 file changed, 1 insertion(+)
hash4|Author|author@example.com|%d

 1 file changed, 1 insertion(+)
hash5|Author|author@example.com|%d

 1 file changed, 1 insertion(+)`,
		baseTime,      // Day 1: 1 commit
		baseTime,      // Day 1: 2 commits
		baseTime,      // Day 1: 3 commits (peak)
		baseTime+86400, // Day 2: 1 commit
		baseTime+172800) // Day 3: 1 commit

	analyzer := &historyAnalyzer{}
	stats, err := analyzer.parseCommitStats(output)
	if err != nil {
		t.Fatalf("parseCommitStats() unexpected error = %v", err)
	}

	if stats.PeakCount != 3 {
		t.Errorf("PeakCount = %d, want 3", stats.PeakCount)
	}

	expectedPeakDay := time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC)
	if !stats.PeakDay.Equal(expectedPeakDay) {
		t.Errorf("PeakDay = %v, want %v", stats.PeakDay, expectedPeakDay)
	}
}
