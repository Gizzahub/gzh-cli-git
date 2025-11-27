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
