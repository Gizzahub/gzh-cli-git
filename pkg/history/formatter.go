// Package history provides Git repository history analysis and statistics.
// This package includes commit statistics, contributor analysis, file history tracking,
// and support for multiple output formats (table, JSON, CSV, markdown).
//
// Example usage:
//
//	// Analyze commit statistics
//	analyzer := history.NewAnalyzer()
//	stats, err := analyzer.GetCommitStats(ctx, repo, history.StatsOptions{
//	    Since: time.Now().AddDate(0, -1, 0),
//	})
//
//	// Get top contributors
//	contributors, err := analyzer.GetContributors(ctx, repo, history.ContributorOptions{
//	    TopN: 10,
//	})
package history

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Formatter formats analysis results in various output formats
type Formatter interface {
	FormatCommitStats(stats *CommitStats) ([]byte, error)
	FormatContributors(contributors []*Contributor) ([]byte, error)
	FormatFileHistory(history []*FileCommit) ([]byte, error)
}

type formatter struct {
	format OutputFormat
}

// NewFormatter creates a new formatter for the specified output format
func NewFormatter(format OutputFormat) Formatter {
	return &formatter{
		format: format,
	}
}

// FormatCommitStats formats commit statistics
func (f *formatter) FormatCommitStats(stats *CommitStats) ([]byte, error) {
	if stats == nil {
		return nil, fmt.Errorf("stats cannot be nil")
	}

	switch f.format {
	case FormatTable:
		return f.formatStatsTable(stats), nil
	case FormatJSON:
		return json.MarshalIndent(stats, "", "  ")
	case FormatCSV:
		return f.formatStatsCSV(stats), nil
	case FormatMarkdown:
		return f.formatStatsMarkdown(stats), nil
	default:
		return nil, ErrInvalidFormat
	}
}

// FormatContributors formats contributor list
func (f *formatter) FormatContributors(contributors []*Contributor) ([]byte, error) {
	if contributors == nil {
		return nil, fmt.Errorf("contributors cannot be nil")
	}

	switch f.format {
	case FormatTable:
		return f.formatContributorsTable(contributors), nil
	case FormatJSON:
		return json.MarshalIndent(contributors, "", "  ")
	case FormatCSV:
		return f.formatContributorsCSV(contributors), nil
	case FormatMarkdown:
		return f.formatContributorsMarkdown(contributors), nil
	default:
		return nil, ErrInvalidFormat
	}
}

// FormatFileHistory formats file commit history
func (f *formatter) FormatFileHistory(history []*FileCommit) ([]byte, error) {
	if history == nil {
		return nil, fmt.Errorf("history cannot be nil")
	}

	switch f.format {
	case FormatTable:
		return f.formatFileHistoryTable(history), nil
	case FormatJSON:
		return json.MarshalIndent(history, "", "  ")
	case FormatCSV:
		return f.formatFileHistoryCSV(history), nil
	case FormatMarkdown:
		return f.formatFileHistoryMarkdown(history), nil
	default:
		return nil, ErrInvalidFormat
	}
}

func (f *formatter) formatStatsTable(stats *CommitStats) []byte {
	var b strings.Builder

	b.WriteString("Commit Statistics\n")
	b.WriteString("==================\n\n")
	b.WriteString(fmt.Sprintf("Total Commits:    %d\n", stats.TotalCommits))
	b.WriteString(fmt.Sprintf("Unique Authors:   %d\n", stats.UniqueAuthors))
	b.WriteString(fmt.Sprintf("Total Additions:  %d lines\n", stats.TotalAdditions))
	b.WriteString(fmt.Sprintf("Total Deletions:  %d lines\n", stats.TotalDeletions))
	b.WriteString(fmt.Sprintf("First Commit:     %s\n", formatTime(stats.FirstCommit)))
	b.WriteString(fmt.Sprintf("Last Commit:      %s\n", formatTime(stats.LastCommit)))
	b.WriteString(fmt.Sprintf("Date Range:       %s\n", formatDuration(stats.DateRange)))
	b.WriteString(fmt.Sprintf("Avg Per Day:      %.2f commits\n", stats.AvgPerDay))
	b.WriteString(fmt.Sprintf("Avg Per Week:     %.2f commits\n", stats.AvgPerWeek))
	b.WriteString(fmt.Sprintf("Avg Per Month:    %.2f commits\n", stats.AvgPerMonth))
	b.WriteString(fmt.Sprintf("Peak Day:         %s (%d commits)\n", formatDate(stats.PeakDay), stats.PeakCount))

	return []byte(b.String())
}

func (f *formatter) formatStatsCSV(stats *CommitStats) []byte {
	var b strings.Builder
	w := csv.NewWriter(&b)

	w.Write([]string{"Metric", "Value"})
	w.Write([]string{"Total Commits", fmt.Sprintf("%d", stats.TotalCommits)})
	w.Write([]string{"Unique Authors", fmt.Sprintf("%d", stats.UniqueAuthors)})
	w.Write([]string{"Total Additions", fmt.Sprintf("%d", stats.TotalAdditions)})
	w.Write([]string{"Total Deletions", fmt.Sprintf("%d", stats.TotalDeletions)})
	w.Write([]string{"First Commit", formatTime(stats.FirstCommit)})
	w.Write([]string{"Last Commit", formatTime(stats.LastCommit)})
	w.Write([]string{"Date Range", formatDuration(stats.DateRange)})
	w.Write([]string{"Avg Per Day", fmt.Sprintf("%.2f", stats.AvgPerDay)})
	w.Write([]string{"Avg Per Week", fmt.Sprintf("%.2f", stats.AvgPerWeek)})
	w.Write([]string{"Avg Per Month", fmt.Sprintf("%.2f", stats.AvgPerMonth)})
	w.Write([]string{"Peak Day", formatDate(stats.PeakDay)})
	w.Write([]string{"Peak Count", fmt.Sprintf("%d", stats.PeakCount)})

	w.Flush()
	return []byte(b.String())
}

func (f *formatter) formatStatsMarkdown(stats *CommitStats) []byte {
	var b strings.Builder

	b.WriteString("# Commit Statistics\n\n")
	b.WriteString("| Metric | Value |\n")
	b.WriteString("|--------|-------|\n")
	b.WriteString(fmt.Sprintf("| Total Commits | %d |\n", stats.TotalCommits))
	b.WriteString(fmt.Sprintf("| Unique Authors | %d |\n", stats.UniqueAuthors))
	b.WriteString(fmt.Sprintf("| Total Additions | %d lines |\n", stats.TotalAdditions))
	b.WriteString(fmt.Sprintf("| Total Deletions | %d lines |\n", stats.TotalDeletions))
	b.WriteString(fmt.Sprintf("| First Commit | %s |\n", formatTime(stats.FirstCommit)))
	b.WriteString(fmt.Sprintf("| Last Commit | %s |\n", formatTime(stats.LastCommit)))
	b.WriteString(fmt.Sprintf("| Date Range | %s |\n", formatDuration(stats.DateRange)))
	b.WriteString(fmt.Sprintf("| Avg Per Day | %.2f commits |\n", stats.AvgPerDay))
	b.WriteString(fmt.Sprintf("| Avg Per Week | %.2f commits |\n", stats.AvgPerWeek))
	b.WriteString(fmt.Sprintf("| Avg Per Month | %.2f commits |\n", stats.AvgPerMonth))
	b.WriteString(fmt.Sprintf("| Peak Day | %s (%d commits) |\n", formatDate(stats.PeakDay), stats.PeakCount))

	return []byte(b.String())
}

func (f *formatter) formatContributorsTable(contributors []*Contributor) []byte {
	var b strings.Builder

	b.WriteString("Contributors\n")
	b.WriteString("============\n\n")
	b.WriteString(fmt.Sprintf("%-4s %-30s %-8s %10s %10s %8s\n", "Rank", "Name", "Commits", "Additions", "Deletions", "Files"))
	b.WriteString(strings.Repeat("-", 80) + "\n")

	for _, c := range contributors {
		b.WriteString(fmt.Sprintf("%-4d %-30s %-8d %10d %10d %8d\n",
			c.Rank,
			truncate(c.Name, 30),
			c.TotalCommits,
			c.LinesAdded,
			c.LinesDeleted,
			c.FilesTouched))
	}

	return []byte(b.String())
}

func (f *formatter) formatContributorsCSV(contributors []*Contributor) []byte {
	var b strings.Builder
	w := csv.NewWriter(&b)

	w.Write([]string{"Rank", "Name", "Email", "Commits", "Additions", "Deletions", "Files", "Active Days", "Commits/Week", "First Commit", "Last Commit"})

	for _, c := range contributors {
		w.Write([]string{
			fmt.Sprintf("%d", c.Rank),
			c.Name,
			c.Email,
			fmt.Sprintf("%d", c.TotalCommits),
			fmt.Sprintf("%d", c.LinesAdded),
			fmt.Sprintf("%d", c.LinesDeleted),
			fmt.Sprintf("%d", c.FilesTouched),
			fmt.Sprintf("%d", c.ActiveDays),
			fmt.Sprintf("%.2f", c.CommitsPerWeek),
			formatTime(c.FirstCommit),
			formatTime(c.LastCommit),
		})
	}

	w.Flush()
	return []byte(b.String())
}

func (f *formatter) formatContributorsMarkdown(contributors []*Contributor) []byte {
	var b strings.Builder

	b.WriteString("# Contributors\n\n")
	b.WriteString("| Rank | Name | Commits | Additions | Deletions | Files |\n")
	b.WriteString("|------|------|---------|-----------|-----------|-------|\n")

	for _, c := range contributors {
		b.WriteString(fmt.Sprintf("| %d | %s | %d | %d | %d | %d |\n",
			c.Rank,
			c.Name,
			c.TotalCommits,
			c.LinesAdded,
			c.LinesDeleted,
			c.FilesTouched))
	}

	return []byte(b.String())
}

func (f *formatter) formatFileHistoryTable(history []*FileCommit) []byte {
	var b strings.Builder

	b.WriteString("File History\n")
	b.WriteString("============\n\n")
	b.WriteString(fmt.Sprintf("%-10s %-20s %-30s %6s %6s\n", "Hash", "Date", "Author", "Added", "Deleted"))
	b.WriteString(strings.Repeat("-", 80) + "\n")

	for _, commit := range history {
		hash := truncate(commit.Hash, 10)
		author := truncate(commit.Author, 30)
		date := formatDate(commit.Date)

		b.WriteString(fmt.Sprintf("%-10s %-20s %-30s %6d %6d\n",
			hash,
			date,
			author,
			commit.LinesAdded,
			commit.LinesDeleted))
	}

	return []byte(b.String())
}

func (f *formatter) formatFileHistoryCSV(history []*FileCommit) []byte {
	var b strings.Builder
	w := csv.NewWriter(&b)

	w.Write([]string{"Hash", "Author", "Email", "Date", "Message", "Additions", "Deletions", "Binary", "Renamed", "Old Path"})

	for _, commit := range history {
		w.Write([]string{
			commit.Hash,
			commit.Author,
			commit.AuthorEmail,
			formatTime(commit.Date),
			commit.Message,
			fmt.Sprintf("%d", commit.LinesAdded),
			fmt.Sprintf("%d", commit.LinesDeleted),
			fmt.Sprintf("%t", commit.IsBinary),
			fmt.Sprintf("%t", commit.WasRenamed),
			commit.OldPath,
		})
	}

	w.Flush()
	return []byte(b.String())
}

func (f *formatter) formatFileHistoryMarkdown(history []*FileCommit) []byte {
	var b strings.Builder

	b.WriteString("# File History\n\n")
	b.WriteString("| Hash | Date | Author | Message | +/- |\n")
	b.WriteString("|------|------|--------|---------|-----|\n")

	for _, commit := range history {
		hash := truncate(commit.Hash, 8)
		author := truncate(commit.Author, 20)
		message := truncate(commit.Message, 40)
		changes := fmt.Sprintf("+%d/-%d", commit.LinesAdded, commit.LinesDeleted)

		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			hash,
			formatDate(commit.Date),
			author,
			message,
			changes))
	}

	return []byte(b.String())
}

// Helper functions

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "N/A"
	}
	return t.Format("2006-01-02 15:04:05")
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return "N/A"
	}
	return t.Format("2006-01-02")
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days == 0 {
		return "< 1 day"
	}
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
