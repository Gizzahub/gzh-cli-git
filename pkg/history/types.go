package history

import "time"

// CommitStats represents commit statistics for a repository
type CommitStats struct {
	TotalCommits   int
	FirstCommit    time.Time
	LastCommit     time.Time
	DateRange      time.Duration
	AvgPerDay      float64
	AvgPerWeek     float64
	AvgPerMonth    float64
	PeakDay        time.Time
	PeakCount      int
	UniqueAuthors  int
	TotalFiles     int
	TotalAdditions int
	TotalDeletions int
}

// CommitTrends represents commit trend data over time
type CommitTrends struct {
	Daily   map[string]int // date (YYYY-MM-DD) -> count
	Weekly  map[string]int // week (YYYY-WW) -> count
	Monthly map[string]int // month (YYYY-MM) -> count
	Hourly  map[int]int    // hour (0-23) -> count
}

// AnalyzeOptions configures history analysis
type AnalyzeOptions struct {
	Since      time.Time
	Until      time.Time
	Branch     string
	Author     string
	MaxCommits int
}

// TrendOptions configures trend analysis
type TrendOptions struct {
	Since  time.Time
	Until  time.Time
	Branch string
}

// Contributor represents a repository contributor
type Contributor struct {
	Name           string
	Email          string
	TotalCommits   int
	FirstCommit    time.Time
	LastCommit     time.Time
	LinesAdded     int
	LinesDeleted   int
	FilesTouched   int
	ActiveDays     int
	CommitsPerWeek float64
	Rank           int
}

// ContributorOptions configures contributor analysis
type ContributorOptions struct {
	Since      time.Time
	Until      time.Time
	MinCommits int
	SortBy     ContributorSortBy
}

// ContributorSortBy defines sorting criteria for contributors
type ContributorSortBy string

const (
	SortByCommits      ContributorSortBy = "commits"
	SortByLinesAdded   ContributorSortBy = "additions"
	SortByLinesDeleted ContributorSortBy = "deletions"
	SortByRecent       ContributorSortBy = "recent"
)

// FileCommit represents a commit affecting a specific file
type FileCommit struct {
	Hash         string
	Author       string
	AuthorEmail  string
	Date         time.Time
	Message      string
	LinesAdded   int
	LinesDeleted int
	IsBinary     bool
	WasRenamed   bool
	OldPath      string
}

// BlameInfo represents file blame information
type BlameInfo struct {
	FilePath string
	Lines    []*BlameLine
}

// BlameLine represents blame information for a single line
type BlameLine struct {
	LineNumber  int
	Content     string
	Hash        string
	Author      string
	AuthorEmail string
	Date        time.Time
}

// HistoryOptions configures file history retrieval
type HistoryOptions struct {
	MaxCount int
	Since    time.Time
	Until    time.Time
	Follow   bool // Follow renames
	Author   string
}

// OutputFormat defines the output format for analysis results
type OutputFormat string

const (
	FormatTable    OutputFormat = "table"
	FormatJSON     OutputFormat = "json"
	FormatCSV      OutputFormat = "csv"
	FormatMarkdown OutputFormat = "markdown"
)
