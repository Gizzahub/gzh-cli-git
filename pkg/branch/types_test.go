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
