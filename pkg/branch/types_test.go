package branch

import "testing"

func TestIsProtected(t *testing.T) {
	tests := []struct {
		name   string
		branch string
		want   bool
	}{
		{"main branch", "main", true},
		{"master branch", "master", true},
		{"develop branch", "develop", true},
		{"development branch", "development", true},
		{"release branch", "release/v1.0", true},
		{"hotfix branch", "hotfix/critical-bug", true},
		{"feature branch", "feature/new-ui", false},
		{"fix branch", "fix/login-bug", false},
		{"random branch", "random-name", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsProtected(tt.branch)
			if got != tt.want {
				t.Errorf("IsProtected(%q) = %v, want %v", tt.branch, got, tt.want)
			}
		})
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name    string
		str     string
		pattern string
		want    bool
	}{
		{"exact match", "main", "main", true},
		{"wildcard match - release", "release/v1.0", "release/*", true},
		{"wildcard match - hotfix", "hotfix/bug-123", "hotfix/*", true},
		{"wildcard no match", "feature/new", "release/*", false},
		{"no wildcard no match", "main", "master", false},
		{"empty string exact match", "", "", true},
		{"star matches all", "test", "*", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchPattern(tt.str, tt.pattern)
			if got != tt.want {
				t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.str, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestInferType(t *testing.T) {
	tests := []struct {
		name   string
		branch string
		want   BranchType
	}{
		{"feature branch", "feature/user-auth", BranchTypeFeature},
		{"fix branch", "fix/login-bug", BranchTypeFix},
		{"hotfix branch", "hotfix/critical", BranchTypeHotfix},
		{"release branch", "release/v1.0.0", BranchTypeRelease},
		{"experiment branch", "experiment/new-ui", BranchTypeExperiment},
		{"main branch", "main", BranchTypeOther},
		{"random branch", "some-branch", BranchTypeOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferType(tt.branch)
			if got != tt.want {
				t.Errorf("InferType(%q) = %v, want %v", tt.branch, got, tt.want)
			}
		})
	}
}

func TestBranch_Struct(t *testing.T) {
	branch := &Branch{
		Name:     "feature/test",
		Ref:      "refs/heads/feature/test",
		SHA:      "abc1234",
		IsHead:   true,
		IsMerged: false,
		Upstream: "origin/feature/test",
	}

	if branch.Name != "feature/test" {
		t.Errorf("Name = %q, want %q", branch.Name, "feature/test")
	}

	if !branch.IsHead {
		t.Error("IsHead should be true")
	}

	if branch.IsMerged {
		t.Error("IsMerged should be false")
	}
}

func TestCreateOptions_Defaults(t *testing.T) {
	opts := CreateOptions{
		Name: "feature/test",
	}

	if opts.Name != "feature/test" {
		t.Errorf("Name = %q, want %q", opts.Name, "feature/test")
	}

	if opts.Checkout {
		t.Error("Checkout should default to false")
	}

	if opts.Track {
		t.Error("Track should default to false")
	}

	if opts.Force {
		t.Error("Force should default to false")
	}
}

func TestDeleteOptions_Defaults(t *testing.T) {
	opts := DeleteOptions{
		Name: "feature/old",
	}

	if opts.Name != "feature/old" {
		t.Errorf("Name = %q, want %q", opts.Name, "feature/old")
	}

	if opts.Remote {
		t.Error("Remote should default to false")
	}

	if opts.Force {
		t.Error("Force should default to false")
	}

	if opts.DryRun {
		t.Error("DryRun should default to false")
	}
}

func TestListOptions_Defaults(t *testing.T) {
	opts := ListOptions{}

	if opts.All {
		t.Error("All should default to false")
	}

	if opts.Merged {
		t.Error("Merged should default to false")
	}

	if opts.Limit != 0 {
		t.Errorf("Limit = %d, want 0", opts.Limit)
	}

	if opts.Sort != "" {
		t.Errorf("Sort = %q, want empty", opts.Sort)
	}
}

func TestSortBy_Constants(t *testing.T) {
	if SortByName != "name" {
		t.Errorf("SortByName = %q, want %q", SortByName, "name")
	}

	if SortByDate != "date" {
		t.Errorf("SortByDate = %q, want %q", SortByDate, "date")
	}

	if SortByAuthor != "author" {
		t.Errorf("SortByAuthor = %q, want %q", SortByAuthor, "author")
	}
}

func TestBranchType_Constants(t *testing.T) {
	types := []struct {
		got  BranchType
		want string
	}{
		{BranchTypeFeature, "feature"},
		{BranchTypeFix, "fix"},
		{BranchTypeHotfix, "hotfix"},
		{BranchTypeRelease, "release"},
		{BranchTypeExperiment, "experiment"},
		{BranchTypeOther, "other"},
	}

	for _, tt := range types {
		if string(tt.got) != tt.want {
			t.Errorf("BranchType = %q, want %q", tt.got, tt.want)
		}
	}
}

func TestIsProtected_EdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		branch string
		want   bool
	}{
		// Protected patterns
		{"main", "main", true},
		{"master", "master", true},
		{"develop", "develop", true},
		{"development", "development", true},
		{"release/v1.0.0", "release/v1.0.0", true},
		{"release/v2.0.0-rc1", "release/v2.0.0-rc1", true},
		{"hotfix/critical-bug", "hotfix/critical-bug", true},
		{"hotfix/urgent", "hotfix/urgent", true},

		// Not protected
		{"feature/new", "feature/new", false},
		{"bugfix/login", "bugfix/login", false},
		{"main-backup", "main-backup", false},
		{"prefix-main", "prefix-main", false},
		{"main/sub", "main/sub", false},
		{"release", "release", false}, // exact match, no slash
		{"hotfix", "hotfix", false},   // exact match, no slash
		{"empty", "", false},
		{"random", "some-random-branch", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsProtected(tt.branch)
			if got != tt.want {
				t.Errorf("IsProtected(%q) = %v, want %v", tt.branch, got, tt.want)
			}
		})
	}
}

func TestMatchPattern_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		str     string
		pattern string
		want    bool
	}{
		// Exact matches
		{"exact empty", "", "", true},
		{"exact single char", "a", "a", true},
		{"exact long", "feature/user-authentication", "feature/user-authentication", true},

		// Wildcard matches
		{"wildcard prefix", "release/v1.0", "release/*", true},
		{"wildcard empty suffix", "release/", "release/*", true},
		{"wildcard only star", "anything", "*", true},
		{"wildcard empty star", "", "*", true},

		// Non-matches
		{"no match different", "main", "master", false},
		{"no match shorter", "release", "release/*", false},
		{"no match wrong prefix", "feature/test", "release/*", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchPattern(tt.str, tt.pattern)
			if got != tt.want {
				t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.str, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestInferType_AllTypes(t *testing.T) {
	tests := []struct {
		name   string
		branch string
		want   BranchType
	}{
		// Feature branches
		{"feature/simple", "feature/simple", BranchTypeFeature},
		{"feature/nested/path", "feature/nested/path", BranchTypeFeature},
		{"feature/123-ticket", "feature/123-ticket", BranchTypeFeature},

		// Fix branches
		{"fix/bug", "fix/bug", BranchTypeFix},
		{"fix/issue-123", "fix/issue-123", BranchTypeFix},

		// Hotfix branches
		{"hotfix/critical", "hotfix/critical", BranchTypeHotfix},
		{"hotfix/security-patch", "hotfix/security-patch", BranchTypeHotfix},

		// Release branches
		{"release/1.0", "release/1.0", BranchTypeRelease},
		{"release/v2.0.0-beta", "release/v2.0.0-beta", BranchTypeRelease},

		// Experiment branches
		{"experiment/new-ui", "experiment/new-ui", BranchTypeExperiment},
		{"experiment/ai-integration", "experiment/ai-integration", BranchTypeExperiment},

		// Other/unclassified
		{"main", "main", BranchTypeOther},
		{"master", "master", BranchTypeOther},
		{"develop", "develop", BranchTypeOther},
		{"bugfix/login", "bugfix/login", BranchTypeOther},
		{"wip/draft", "wip/draft", BranchTypeOther},
		{"empty", "", BranchTypeOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferType(tt.branch)
			if got != tt.want {
				t.Errorf("InferType(%q) = %v, want %v", tt.branch, got, tt.want)
			}
		})
	}
}

func TestSortBy_AllConstants(t *testing.T) {
	tests := []struct {
		constant SortBy
		value    string
	}{
		{SortByName, "name"},
		{SortByDate, "date"},
		{SortByAuthor, "author"},
		{SortByUpstream, "upstream"},
	}

	for _, tt := range tests {
		if string(tt.constant) != tt.value {
			t.Errorf("SortBy constant = %q, want %q", tt.constant, tt.value)
		}
	}
}

func TestBranch_AllFields(t *testing.T) {
	branch := &Branch{
		Name:       "feature/all-fields",
		Ref:        "refs/heads/feature/all-fields",
		SHA:        "abc123def456789",
		IsHead:     true,
		IsMerged:   true,
		IsRemote:   true,
		Upstream:   "origin/feature/all-fields",
		AheadBy:    5,
		BehindBy:   3,
		LastCommit: nil,
		CreatedAt:  nil,
		UpdatedAt:  nil,
	}

	if branch.Name != "feature/all-fields" {
		t.Errorf("Name = %q, want %q", branch.Name, "feature/all-fields")
	}

	if branch.Ref != "refs/heads/feature/all-fields" {
		t.Errorf("Ref = %q, want expected ref", branch.Ref)
	}

	if branch.SHA != "abc123def456789" {
		t.Errorf("SHA = %q, want expected SHA", branch.SHA)
	}

	if !branch.IsHead {
		t.Error("IsHead should be true")
	}

	if !branch.IsMerged {
		t.Error("IsMerged should be true")
	}

	if !branch.IsRemote {
		t.Error("IsRemote should be true")
	}

	if branch.Upstream != "origin/feature/all-fields" {
		t.Errorf("Upstream = %q, want expected upstream", branch.Upstream)
	}

	if branch.AheadBy != 5 {
		t.Errorf("AheadBy = %d, want 5", branch.AheadBy)
	}

	if branch.BehindBy != 3 {
		t.Errorf("BehindBy = %d, want 3", branch.BehindBy)
	}
}

func TestCommit_Struct(t *testing.T) {
	commit := &Commit{
		SHA:      "abc123def",
		Author:   "John Doe",
		Email:    "john@example.com",
		Message:  "feat: add new feature\n\nThis adds a new feature.",
		ShortMsg: "feat: add new feature",
	}

	if commit.SHA != "abc123def" {
		t.Errorf("SHA = %q, want %q", commit.SHA, "abc123def")
	}

	if commit.Author != "John Doe" {
		t.Errorf("Author = %q, want %q", commit.Author, "John Doe")
	}

	if commit.Email != "john@example.com" {
		t.Errorf("Email = %q, want %q", commit.Email, "john@example.com")
	}

	if commit.ShortMsg != "feat: add new feature" {
		t.Errorf("ShortMsg = %q, want expected message", commit.ShortMsg)
	}
}

func TestProtectedBranches_List(t *testing.T) {
	// Verify the protected branches list contains expected patterns
	expectedPatterns := map[string]bool{
		"main":        true,
		"master":      true,
		"develop":     true,
		"development": true,
		"release/*":   true,
		"hotfix/*":    true,
	}

	for _, pattern := range ProtectedBranches {
		if !expectedPatterns[pattern] {
			t.Errorf("Unexpected pattern in ProtectedBranches: %q", pattern)
		}
		delete(expectedPatterns, pattern)
	}

	// Check all expected patterns were found
	for pattern := range expectedPatterns {
		t.Errorf("Missing pattern in ProtectedBranches: %q", pattern)
	}
}
