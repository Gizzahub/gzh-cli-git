package history

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gizzahub/gzh-cli-git/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

func TestContributorAnalyzer_Analyze(t *testing.T) {
	tests := []struct {
		name             string
		shortlogOutput   string
		logOutputs       map[string]string // email -> log output
		opts             ContributorOptions
		wantCount        int
		wantFirstName    string
		wantFirstCommits int
	}{
		{
			name: "basic contributors",
			shortlogOutput: `   10  John Doe <john@example.com>
     5  Jane Smith <jane@example.com>`,
			logOutputs: map[string]string{
				"john@example.com": `1700000000
10	5	file1.go

1700001000
3	2	file2.go
`,
				"jane@example.com": `1700002000
5	1	file3.go
`,
			},
			opts:             ContributorOptions{},
			wantCount:        2,
			wantFirstName:    "John Doe",
			wantFirstCommits: 10,
		},
		{
			name: "with min commits filter",
			shortlogOutput: `   10  John Doe <john@example.com>
     5  Jane Smith <jane@example.com>
     2  Bob Jones <bob@example.com>`,
			logOutputs: map[string]string{
				"john@example.com": `1700000000
10	5	file1.go
`,
				"jane@example.com": `1700001000
5	1	file2.go
`,
				"bob@example.com": `1700002000
2	1	file3.go
`,
			},
			opts:             ContributorOptions{MinCommits: 5},
			wantCount:        2,
			wantFirstName:    "John Doe",
			wantFirstCommits: 10,
		},
		{
			name:           "empty shortlog",
			shortlogOutput: "",
			logOutputs:     map[string]string{},
			opts:           ContributorOptions{},
			wantCount:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockExecutor{
				runFunc: func(ctx context.Context, repoPath string, args ...string) (*gitcmd.Result, error) {
					// Handle shortlog command
					if len(args) > 0 && args[0] == "shortlog" {
						return &gitcmd.Result{
							Stdout:   tt.shortlogOutput,
							Stderr:   "",
							ExitCode: 0,
						}, nil
					}

					// Handle log command for specific author
					if len(args) > 0 && args[0] == "log" {
						for i, arg := range args {
							if strings.HasPrefix(arg, "--author=") {
								email := strings.TrimPrefix(arg, "--author=")
								if output, ok := tt.logOutputs[email]; ok {
									return &gitcmd.Result{
										Stdout:   output,
										Stderr:   "",
										ExitCode: 0,
									}, nil
								}
							}
							// Also check next arg in case it's --author EMAIL format
							if arg == "--author" && i+1 < len(args) {
								email := args[i+1]
								if output, ok := tt.logOutputs[email]; ok {
									return &gitcmd.Result{
										Stdout:   output,
										Stderr:   "",
										ExitCode: 0,
									}, nil
								}
							}
						}
					}

					return &gitcmd.Result{Stdout: "", Stderr: "", ExitCode: 0}, nil
				},
			}

			analyzer := &contributorAnalyzer{executor: executor}
			repo := &repository.Repository{Path: "/test/repo"}

			got, err := analyzer.Analyze(context.Background(), repo, tt.opts)
			if err != nil {
				t.Fatalf("Analyze() unexpected error = %v", err)
			}

			if len(got) != tt.wantCount {
				t.Errorf("len(contributors) = %d, want %d", len(got), tt.wantCount)
			}

			if tt.wantCount > 0 {
				if got[0].Name != tt.wantFirstName {
					t.Errorf("contributors[0].Name = %q, want %q", got[0].Name, tt.wantFirstName)
				}

				if got[0].TotalCommits != tt.wantFirstCommits {
					t.Errorf("contributors[0].TotalCommits = %d, want %d", got[0].TotalCommits, tt.wantFirstCommits)
				}

				if got[0].Rank != 1 {
					t.Errorf("contributors[0].Rank = %d, want 1", got[0].Rank)
				}
			}
		})
	}
}

func TestContributorAnalyzer_GetTopContributors(t *testing.T) {
	shortlogOutput := `   10  John Doe <john@example.com>
     8  Jane Smith <jane@example.com>
     5  Bob Jones <bob@example.com>
     3  Alice Brown <alice@example.com>`

	logOutputs := map[string]string{
		"john@example.com":  "1700000000\n10\t5\tfile1.go\n",
		"jane@example.com":  "1700001000\n8\t3\tfile2.go\n",
		"bob@example.com":   "1700002000\n5\t2\tfile3.go\n",
		"alice@example.com": "1700003000\n3\t1\tfile4.go\n",
	}

	tests := []struct {
		name      string
		limit     int
		wantCount int
	}{
		{"top 2", 2, 2},
		{"top 3", 3, 3},
		{"top 10 (more than available)", 10, 4},
		{"no limit", 0, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockExecutor{
				runFunc: func(ctx context.Context, repoPath string, args ...string) (*gitcmd.Result, error) {
					if len(args) > 0 && args[0] == "shortlog" {
						return &gitcmd.Result{Stdout: shortlogOutput, Stderr: "", ExitCode: 0}, nil
					}

					if len(args) > 0 && args[0] == "log" {
						for i, arg := range args {
							if strings.HasPrefix(arg, "--author=") {
								email := strings.TrimPrefix(arg, "--author=")
								if output, ok := logOutputs[email]; ok {
									return &gitcmd.Result{Stdout: output, Stderr: "", ExitCode: 0}, nil
								}
							}
							if arg == "--author" && i+1 < len(args) {
								if output, ok := logOutputs[args[i+1]]; ok {
									return &gitcmd.Result{Stdout: output, Stderr: "", ExitCode: 0}, nil
								}
							}
						}
					}

					return &gitcmd.Result{Stdout: "", Stderr: "", ExitCode: 0}, nil
				},
			}

			analyzer := &contributorAnalyzer{executor: executor}
			repo := &repository.Repository{Path: "/test/repo"}

			got, err := analyzer.GetTopContributors(context.Background(), repo, tt.limit)
			if err != nil {
				t.Fatalf("GetTopContributors() unexpected error = %v", err)
			}

			if len(got) != tt.wantCount {
				t.Errorf("len(contributors) = %d, want %d", len(got), tt.wantCount)
			}

			// Verify sorted by commits (descending)
			for i := 0; i < len(got)-1; i++ {
				if got[i].TotalCommits < got[i+1].TotalCommits {
					t.Errorf("contributors not sorted by commits: %d < %d", got[i].TotalCommits, got[i+1].TotalCommits)
				}
			}
		})
	}
}

func TestContributorAnalyzer_ParseShortlog(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantCount int
		wantFirst *Contributor
	}{
		{
			name: "basic shortlog",
			output: `   10  John Doe <john@example.com>
     5  Jane Smith <jane@example.com>`,
			wantCount: 2,
			wantFirst: &Contributor{
				Name:         "John Doe",
				Email:        "john@example.com",
				TotalCommits: 10,
			},
		},
		{
			name: "shortlog with extra spaces",
			output: `     123  John Doe <john@example.com>
       5  Jane Smith <jane@example.com>`,
			wantCount: 2,
			wantFirst: &Contributor{
				Name:         "John Doe",
				Email:        "john@example.com",
				TotalCommits: 123,
			},
		},
		{
			name:      "empty shortlog",
			output:    "",
			wantCount: 0,
		},
		{
			name:      "whitespace only",
			output:    "   \n  \n",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := &contributorAnalyzer{}
			got := analyzer.parseShortlog(tt.output)

			if len(got) != tt.wantCount {
				t.Errorf("len(contributors) = %d, want %d", len(got), tt.wantCount)
			}

			if tt.wantFirst != nil && len(got) > 0 {
				if got[0].Name != tt.wantFirst.Name {
					t.Errorf("contributors[0].Name = %q, want %q", got[0].Name, tt.wantFirst.Name)
				}

				if got[0].Email != tt.wantFirst.Email {
					t.Errorf("contributors[0].Email = %q, want %q", got[0].Email, tt.wantFirst.Email)
				}

				if got[0].TotalCommits != tt.wantFirst.TotalCommits {
					t.Errorf("contributors[0].TotalCommits = %d, want %d", got[0].TotalCommits, tt.wantFirst.TotalCommits)
				}
			}
		})
	}
}

func TestContributorAnalyzer_ParseNameEmail(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantName  string
		wantEmail string
	}{
		{
			name:      "standard format",
			input:     "John Doe <john@example.com>",
			wantName:  "John Doe",
			wantEmail: "john@example.com",
		},
		{
			name:      "with extra spaces",
			input:     "  John Doe  <  john@example.com  >",
			wantName:  "John Doe",
			wantEmail: "john@example.com",
		},
		{
			name:      "no email",
			input:     "John Doe",
			wantName:  "John Doe",
			wantEmail: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := &contributorAnalyzer{}
			name, email := analyzer.parseNameEmail(tt.input)

			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}

			if email != tt.wantEmail {
				t.Errorf("email = %q, want %q", email, tt.wantEmail)
			}
		})
	}
}

func TestContributorAnalyzer_ParseContributorStats(t *testing.T) {
	tests := []struct {
		name             string
		output           string
		wantLinesAdded   int
		wantLinesDeleted int
		wantFilesTouched int
		wantActiveDays   int
	}{
		{
			name: "basic stats",
			output: `1700000000
10	5	file1.go
3	2	file2.go

1700086400
5	1	file3.go
`,
			wantLinesAdded:   18,
			wantLinesDeleted: 8,
			wantFilesTouched: 3,
			wantActiveDays:   2, // 2 different days
		},
		{
			name: "same file multiple times",
			output: `1700000000
10	5	file1.go

1700001000
5	2	file1.go
`,
			wantLinesAdded:   15,
			wantLinesDeleted: 7,
			wantFilesTouched: 1, // Same file
			wantActiveDays:   1, // Same day
		},
		{
			name:             "empty output",
			output:           "",
			wantLinesAdded:   0,
			wantLinesDeleted: 0,
			wantFilesTouched: 0,
			wantActiveDays:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := &contributorAnalyzer{}
			contributor := &Contributor{}

			analyzer.parseContributorStats(contributor, tt.output)

			if contributor.LinesAdded != tt.wantLinesAdded {
				t.Errorf("LinesAdded = %d, want %d", contributor.LinesAdded, tt.wantLinesAdded)
			}

			if contributor.LinesDeleted != tt.wantLinesDeleted {
				t.Errorf("LinesDeleted = %d, want %d", contributor.LinesDeleted, tt.wantLinesDeleted)
			}

			if contributor.FilesTouched != tt.wantFilesTouched {
				t.Errorf("FilesTouched = %d, want %d", contributor.FilesTouched, tt.wantFilesTouched)
			}

			if contributor.ActiveDays != tt.wantActiveDays {
				t.Errorf("ActiveDays = %d, want %d", contributor.ActiveDays, tt.wantActiveDays)
			}
		})
	}
}

func TestContributorAnalyzer_SortContributors(t *testing.T) {
	baseTime := time.Date(2025, 11, 1, 12, 0, 0, 0, time.UTC)

	contributors := []*Contributor{
		{Name: "A", TotalCommits: 10, LinesAdded: 100, LinesDeleted: 50, LastCommit: baseTime},
		{Name: "B", TotalCommits: 20, LinesAdded: 200, LinesDeleted: 30, LastCommit: baseTime.Add(24 * time.Hour)},
		{Name: "C", TotalCommits: 5, LinesAdded: 50, LinesDeleted: 100, LastCommit: baseTime.Add(48 * time.Hour)},
	}

	tests := []struct {
		name      string
		sortBy    ContributorSortBy
		wantFirst string
	}{
		{"by commits", SortByCommits, "B"},
		{"by lines added", SortByLinesAdded, "B"},
		{"by lines deleted", SortByLinesDeleted, "C"},
		{"by recent", SortByRecent, "C"},
		{"default (commits)", "", "B"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying the original
			testContributors := make([]*Contributor, len(contributors))
			copy(testContributors, contributors)

			analyzer := &contributorAnalyzer{}
			analyzer.sortContributors(testContributors, tt.sortBy)

			if testContributors[0].Name != tt.wantFirst {
				t.Errorf("first contributor = %q, want %q", testContributors[0].Name, tt.wantFirst)
			}
		})
	}
}

func TestContributorAnalyzer_ParseContributorStats_CommitsPerWeek(t *testing.T) {
	// Create commits over 2 weeks (14 days)
	baseTime := time.Date(2025, 11, 1, 12, 0, 0, 0, time.UTC).Unix()

	// 14 commits over 14 days = 1 per day = 7 per week
	var lines []string
	for i := 0; i < 14; i++ {
		timestamp := baseTime + int64(i*86400) // One per day
		lines = append(lines, fmt.Sprintf("%d", timestamp))
		lines = append(lines, "1\t0\tfile.go")
		lines = append(lines, "") // Empty line
	}

	output := strings.Join(lines, "\n")

	analyzer := &contributorAnalyzer{}
	contributor := &Contributor{TotalCommits: 14}

	analyzer.parseContributorStats(contributor, output)

	// 14 commits over ~2 weeks = ~7 commits/week (13 days = 1.857 weeks)
	expectedCommitsPerWeek := 7.0
	tolerance := 1.0 // Allow for date range calculation variations

	if contributor.CommitsPerWeek < expectedCommitsPerWeek-tolerance ||
		contributor.CommitsPerWeek > expectedCommitsPerWeek+tolerance {
		t.Errorf("CommitsPerWeek = %f, want ~%f", contributor.CommitsPerWeek, expectedCommitsPerWeek)
	}

	if contributor.FirstCommit.IsZero() {
		t.Error("FirstCommit should not be zero")
	}

	if contributor.LastCommit.IsZero() {
		t.Error("LastCommit should not be zero")
	}

	if !contributor.LastCommit.After(contributor.FirstCommit) {
		t.Error("LastCommit should be after FirstCommit")
	}
}

func TestEscapeRegexChars(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no special chars",
			input: "user@example.com",
			want:  "user@example.com",
		},
		{
			name:  "bot email with brackets",
			input: "49699333+dependabot[bot]@users.noreply.github.com",
			want:  "49699333+dependabot\\[bot\\]@users.noreply.github.com",
		},
		{
			name:  "multiple brackets",
			input: "test[foo][bar]@example.com",
			want:  "test\\[foo\\]\\[bar\\]@example.com",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeRegexChars(tt.input)
			if got != tt.want {
				t.Errorf("escapeRegexChars(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
