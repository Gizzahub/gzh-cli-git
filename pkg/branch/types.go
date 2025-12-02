package branch

import "time"

// Branch represents a Git branch with metadata.
type Branch struct {
	Name       string     // Branch name
	Ref        string     // Full ref (refs/heads/...)
	SHA        string     // Commit SHA
	IsHead     bool       // Currently checked out
	IsMerged   bool       // Fully merged into base branch
	IsRemote   bool       // Remote branch
	Upstream   string     // Upstream branch (if set)
	AheadBy    int        // Commits ahead of upstream
	BehindBy   int        // Commits behind upstream
	LastCommit *Commit    // Last commit on this branch
	CreatedAt  *time.Time // Creation time (if available)
	UpdatedAt  *time.Time // Last update time
}

// Commit represents a Git commit with metadata.
type Commit struct {
	SHA      string
	Author   string
	Email    string
	Date     time.Time
	Message  string
	ShortMsg string // First line of message
}

// CreateOptions configures branch creation.
type CreateOptions struct {
	Name     string // Branch name (required)
	StartRef string // Starting ref (default: HEAD)
	Checkout bool   // Checkout after creation
	Track    bool   // Set upstream tracking
	Force    bool   // Overwrite existing branch
	Validate bool   // Validate naming conventions (default: true)
}

// DeleteOptions configures branch deletion.
type DeleteOptions struct {
	Name    string // Branch name (required)
	Remote  bool   // Delete remote branch
	Force   bool   // Force delete (even if unmerged)
	DryRun  bool   // Preview deletion
	Confirm bool   // Skip confirmation prompt
}

// ListOptions configures branch listing.
type ListOptions struct {
	All      bool   // Include remote branches
	Merged   bool   // Only merged branches
	Unmerged bool   // Only unmerged branches
	Pattern  string // Name pattern filter
	Sort     SortBy // Sort order
	Limit    int    // Max results (0 = unlimited)
	Remote   string // Specific remote (empty = all)
}

// SortBy defines branch sorting order.
type SortBy string

const (
	SortByName     SortBy = "name"     // Alphabetical by name
	SortByDate     SortBy = "date"     // Most recent first
	SortByAuthor   SortBy = "author"   // Alphabetical by author
	SortByUpstream SortBy = "upstream" // Group by upstream
)

// BranchType represents branch purpose/category.
type BranchType string

const (
	BranchTypeFeature    BranchType = "feature"    // feature/*
	BranchTypeFix        BranchType = "fix"        // fix/*
	BranchTypeHotfix     BranchType = "hotfix"     // hotfix/*
	BranchTypeRelease    BranchType = "release"    // release/*
	BranchTypeExperiment BranchType = "experiment" // experiment/*
	BranchTypeOther      BranchType = "other"      // Unclassified
)

// ProtectedBranches are branches that require --force to delete.
var ProtectedBranches = []string{
	"main",
	"master",
	"develop",
	"development",
	"release/*",
	"hotfix/*",
}

// IsProtected checks if a branch name matches protected patterns.
func IsProtected(name string) bool {
	for _, pattern := range ProtectedBranches {
		if matchPattern(name, pattern) {
			return true
		}
	}
	return false
}

// matchPattern checks if name matches pattern (supports * wildcard).
func matchPattern(name, pattern string) bool {
	// Simple wildcard matching
	if pattern == name {
		return true
	}

	// Handle trailing wildcard (e.g., "release/*")
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(name) >= len(prefix) && name[:len(prefix)] == prefix
	}

	return false
}

// InferType infers branch type from name.
func InferType(name string) BranchType {
	switch {
	case matchPattern(name, "feature/*"):
		return BranchTypeFeature
	case matchPattern(name, "fix/*"):
		return BranchTypeFix
	case matchPattern(name, "hotfix/*"):
		return BranchTypeHotfix
	case matchPattern(name, "release/*"):
		return BranchTypeRelease
	case matchPattern(name, "experiment/*"):
		return BranchTypeExperiment
	default:
		return BranchTypeOther
	}
}

// Worktree represents a Git worktree.
type Worktree struct {
	Path       string // Worktree path
	Branch     string // Branch name
	Ref        string // Full ref (HEAD or commit SHA)
	IsMain     bool   // Is the main worktree
	IsLocked   bool   // Is locked
	IsPrunable bool   // Can be pruned
	IsBare     bool   // Is bare repository
	IsDetached bool   // Is detached HEAD
}

// AddOptions configures worktree addition.
type AddOptions struct {
	Path         string // Worktree path (required)
	Branch       string // Branch name (required)
	CreateBranch bool   // Create new branch
	Force        bool   // Overwrite existing
	Detach       bool   // Detached HEAD
	Checkout     string // Specific commit to checkout
}

// RemoveOptions configures worktree removal.
type RemoveOptions struct {
	Path  string // Worktree path (required)
	Force bool   // Force removal (even with uncommitted changes)
}

// AnalyzeOptions configures branch cleanup analysis.
type AnalyzeOptions struct {
	IncludeMerged  bool          // Include fully merged branches
	IncludeStale   bool          // Include stale branches (no activity)
	StaleThreshold time.Duration // Threshold for stale (default: 30 days)
	IncludeRemote  bool          // Include remote branches
	Exclude        []string      // Patterns to exclude
	BaseBranch     string        // Base branch for merge detection (default: main/master)
}

// ExecuteOptions configures branch cleanup execution.
type ExecuteOptions struct {
	DryRun  bool     // Preview only, don't delete
	Force   bool     // Force delete unmerged branches
	Remote  bool     // Also delete remote branches
	Confirm bool     // Skip confirmation prompts
	Exclude []string // Additional patterns to exclude
}

// CleanupReport summarizes branches eligible for cleanup.
type CleanupReport struct {
	Merged    []*Branch // Fully merged branches
	Stale     []*Branch // Stale branches
	Orphaned  []*Branch // Orphaned tracking branches
	Protected []*Branch // Protected (won't delete)
	Total     int       // Total branches analyzed
}

// CleanupStrategy defines cleanup approach.
type CleanupStrategy string

const (
	StrategyMerged   CleanupStrategy = "merged"   // Only merged branches
	StrategyStale    CleanupStrategy = "stale"    // Only stale branches
	StrategyOrphaned CleanupStrategy = "orphaned" // Only orphaned branches
	StrategyAll      CleanupStrategy = "all"      // All eligible branches
)
