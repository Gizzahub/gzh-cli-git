package history

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

// ContributorAnalyzer analyzes contributor activity
type ContributorAnalyzer interface {
	Analyze(ctx context.Context, repo *repository.Repository, opts ContributorOptions) ([]*Contributor, error)
	GetTopContributors(ctx context.Context, repo *repository.Repository, limit int) ([]*Contributor, error)
}

type contributorAnalyzer struct {
	executor GitExecutor
}

// NewContributorAnalyzer creates a new contributor analyzer
func NewContributorAnalyzer(executor GitExecutor) ContributorAnalyzer {
	return &contributorAnalyzer{
		executor: executor,
	}
}

// Analyze analyzes contributor activity and returns detailed statistics
func (c *contributorAnalyzer) Analyze(ctx context.Context, repo *repository.Repository, opts ContributorOptions) ([]*Contributor, error) {
	// Get basic contributor stats from shortlog
	result, err := c.executor.Run(ctx, repo.Path, "shortlog", "-s", "-n", "-e", "--all")
	if err != nil {
		return nil, fmt.Errorf("failed to get shortlog: %w", err)
	}

	contributors := c.parseShortlog(result.Stdout)

	// Filter by minimum commits if specified
	if opts.MinCommits > 0 {
		filtered := make([]*Contributor, 0)
		for _, contributor := range contributors {
			if contributor.TotalCommits >= opts.MinCommits {
				filtered = append(filtered, contributor)
			}
		}
		contributors = filtered
	}

	// Enrich with detailed stats for each contributor
	for _, contributor := range contributors {
		if err := c.enrichContributor(ctx, repo, contributor, opts); err != nil {
			// Log error but continue with other contributors
			continue
		}
	}

	// Sort contributors
	c.sortContributors(contributors, opts.SortBy)

	// Assign ranks
	for i, contributor := range contributors {
		contributor.Rank = i + 1
	}

	return contributors, nil
}

// GetTopContributors returns the top N contributors by commit count
func (c *contributorAnalyzer) GetTopContributors(ctx context.Context, repo *repository.Repository, limit int) ([]*Contributor, error) {
	opts := ContributorOptions{
		SortBy: SortByCommits,
	}

	contributors, err := c.Analyze(ctx, repo, opts)
	if err != nil {
		return nil, err
	}

	if limit > 0 && len(contributors) > limit {
		contributors = contributors[:limit]
	}

	return contributors, nil
}

func (c *contributorAnalyzer) parseShortlog(output string) []*Contributor {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	contributors := make([]*Contributor, 0)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Format: "   123  John Doe <john@example.com>"
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		// Parse commit count
		count, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		// Parse name and email
		// Everything after the count is "Name <email>"
		nameEmail := strings.Join(parts[1:], " ")
		name, email := c.parseNameEmail(nameEmail)

		contributor := &Contributor{
			Name:         name,
			Email:        email,
			TotalCommits: count,
		}

		contributors = append(contributors, contributor)
	}

	return contributors
}

func (c *contributorAnalyzer) parseNameEmail(nameEmail string) (string, string) {
	// Format: "Name <email>"
	parts := strings.Split(nameEmail, "<")
	if len(parts) != 2 {
		return nameEmail, ""
	}

	name := strings.TrimSpace(parts[0])
	email := strings.TrimSpace(strings.TrimSuffix(parts[1], ">"))

	return name, email
}

func (c *contributorAnalyzer) enrichContributor(ctx context.Context, repo *repository.Repository, contributor *Contributor, opts ContributorOptions) error {
	// Build git log command for this contributor
	args := []string{"log", "--author=" + contributor.Email, "--format=%ct", "--numstat"}

	if !opts.Since.IsZero() {
		args = append(args, fmt.Sprintf("--since=%s", opts.Since.Format(time.RFC3339)))
	}

	if !opts.Until.IsZero() {
		args = append(args, fmt.Sprintf("--until=%s", opts.Until.Format(time.RFC3339)))
	}

	result, err := c.executor.Run(ctx, repo.Path, args...)
	if err != nil {
		return fmt.Errorf("failed to get contributor stats: %w", err)
	}

	// Parse detailed stats
	c.parseContributorStats(contributor, result.Stdout)

	return nil
}

func (c *contributorAnalyzer) parseContributorStats(contributor *Contributor, output string) {
	lines := strings.Split(strings.TrimSpace(output), "\n")

	var timestamps []time.Time
	uniqueDays := make(map[string]bool)
	uniqueFiles := make(map[string]bool)

	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			i++
			continue
		}

		// Parse timestamp line
		timestamp, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			i++
			continue
		}

		commitTime := time.Unix(timestamp, 0)
		timestamps = append(timestamps, commitTime)

		// Track unique days
		dateKey := commitTime.Format("2006-01-02")
		uniqueDays[dateKey] = true

		// Move to numstat lines
		i++
		for i < len(lines) {
			statLine := strings.TrimSpace(lines[i])
			if statLine == "" {
				i++
				break
			}

			// Parse numstat line: "additions deletions filename"
			fields := strings.Fields(statLine)
			if len(fields) >= 3 {
				// Track additions/deletions
				additions, _ := strconv.Atoi(fields[0])
				deletions, _ := strconv.Atoi(fields[1])
				filename := fields[2]

				contributor.LinesAdded += additions
				contributor.LinesDeleted += deletions
				uniqueFiles[filename] = true
			}

			i++
		}
	}

	// Calculate derived stats
	contributor.FilesTouched = len(uniqueFiles)
	contributor.ActiveDays = len(uniqueDays)

	if len(timestamps) > 0 {
		// Find first and last commits
		var firstCommit, lastCommit time.Time
		for _, ts := range timestamps {
			if firstCommit.IsZero() || ts.Before(firstCommit) {
				firstCommit = ts
			}
			if lastCommit.IsZero() || ts.After(lastCommit) {
				lastCommit = ts
			}
		}

		contributor.FirstCommit = firstCommit
		contributor.LastCommit = lastCommit

		// Calculate commits per week
		dateRange := lastCommit.Sub(firstCommit)
		weeks := dateRange.Hours() / 24 / 7
		if weeks < 1 {
			weeks = 1
		}
		contributor.CommitsPerWeek = float64(contributor.TotalCommits) / weeks
	}
}

func (c *contributorAnalyzer) sortContributors(contributors []*Contributor, sortBy ContributorSortBy) {
	switch sortBy {
	case SortByCommits:
		sort.Slice(contributors, func(i, j int) bool {
			return contributors[i].TotalCommits > contributors[j].TotalCommits
		})
	case SortByLinesAdded:
		sort.Slice(contributors, func(i, j int) bool {
			return contributors[i].LinesAdded > contributors[j].LinesAdded
		})
	case SortByLinesDeleted:
		sort.Slice(contributors, func(i, j int) bool {
			return contributors[i].LinesDeleted > contributors[j].LinesDeleted
		})
	case SortByRecent:
		sort.Slice(contributors, func(i, j int) bool {
			return contributors[i].LastCommit.After(contributors[j].LastCommit)
		})
	default:
		// Default to sort by commits
		sort.Slice(contributors, func(i, j int) bool {
			return contributors[i].TotalCommits > contributors[j].TotalCommits
		})
	}
}
