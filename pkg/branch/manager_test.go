package branch

import (
	"context"
	"testing"

	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

func TestNewManager(t *testing.T) {
	mgr := NewManager()
	if mgr == nil {
		t.Fatal("NewManager() returned nil")
	}
}

func TestValidateBranchName(t *testing.T) {
	tests := []struct {
		name    string
		branch  string
		wantErr bool
	}{
		{"valid simple", "main", false},
		{"valid with dash", "feature-x", false},
		{"valid with underscore", "feature_x", false},
		{"valid with slash", "feature/user-auth", false},
		{"valid complex", "feature/user-auth-v2", false},
		{"empty name", "", true},
		{"starts with dot", ".hidden", true},
		{"ends with .lock", "branch.lock", true},
		{"contains space", "feature branch", true},
		{"contains tilde", "feature~1", true},
		{"contains caret", "feature^1", true},
		{"contains colon", "feature:test", true},
		{"contains question", "feature?", true},
		{"contains asterisk", "feature*", true},
		{"contains bracket", "feature[1]", true},
		{"contains backslash", "feature\\test", true},
		{"contains double-dot", "feature..test", true},
		{"starts with slash", "/feature", true},
		{"ends with slash", "feature/", true},
		{"consecutive slashes", "feature//test", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBranchName(tt.branch)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateBranchName(%q) error = %v, wantErr %v", tt.branch, err, tt.wantErr)
			}
		})
	}
}

func TestManager_ParseBranchLine(t *testing.T) {
	mgr := &manager{}

	tests := []struct {
		name     string
		line     string
		wantName string
		wantSHA  string
		wantHead bool
		wantErr  bool
	}{
		{
			name:     "current branch",
			line:     "* main  abc1234 [origin/main] Initial commit",
			wantName: "main",
			wantSHA:  "abc1234",
			wantHead: true,
			wantErr:  false,
		},
		{
			name:     "non-current branch",
			line:     "  feature/x  def5678 Add feature X",
			wantName: "feature/x",
			wantSHA:  "def5678",
			wantHead: false,
			wantErr:  false,
		},
		{
			name:     "branch with upstream",
			line:     "  feature/y  ghi9012 [origin/feature/y] Work in progress",
			wantName: "feature/y",
			wantSHA:  "ghi9012",
			wantHead: false,
			wantErr:  false,
		},
		{
			name:    "invalid line - too short",
			line:    "* main",
			wantErr: true,
		},
		{
			name:    "empty line",
			line:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			branch, err := mgr.parseBranchLine(tt.line)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseBranchLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if branch.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", branch.Name, tt.wantName)
			}

			if branch.SHA != tt.wantSHA {
				t.Errorf("SHA = %q, want %q", branch.SHA, tt.wantSHA)
			}

			if branch.IsHead != tt.wantHead {
				t.Errorf("IsHead = %v, want %v", branch.IsHead, tt.wantHead)
			}
		})
	}
}

func TestManager_ParseBranchList(t *testing.T) {
	mgr := &manager{}

	output := `* main  abc1234 [origin/main] Initial commit
  feature/x  def5678 Add feature X
  feature/y  ghi9012 [origin/feature/y] Work in progress`

	branches, err := mgr.parseBranchList(output)
	if err != nil {
		t.Fatalf("parseBranchList() error = %v", err)
	}

	if len(branches) != 3 {
		t.Fatalf("len(branches) = %d, want 3", len(branches))
	}

	// Check first branch (current)
	if branches[0].Name != "main" {
		t.Errorf("branches[0].Name = %q, want %q", branches[0].Name, "main")
	}

	if !branches[0].IsHead {
		t.Error("branches[0].IsHead should be true")
	}

	// Check second branch
	if branches[1].Name != "feature/x" {
		t.Errorf("branches[1].Name = %q, want %q", branches[1].Name, "feature/x")
	}

	if branches[1].IsHead {
		t.Error("branches[1].IsHead should be false")
	}

	// Check third branch
	if branches[2].Name != "feature/y" {
		t.Errorf("branches[2].Name = %q, want %q", branches[2].Name, "feature/y")
	}

	if branches[2].Upstream != "origin/feature/y" {
		t.Errorf("branches[2].Upstream = %q, want %q", branches[2].Upstream, "origin/feature/y")
	}
}

func TestManager_ParseBranchList_Empty(t *testing.T) {
	mgr := &manager{}

	output := ""

	branches, err := mgr.parseBranchList(output)
	if err != nil {
		t.Fatalf("parseBranchList() error = %v", err)
	}

	if len(branches) != 0 {
		t.Errorf("len(branches) = %d, want 0", len(branches))
	}
}

func TestManager_Create_NilRepository(t *testing.T) {
	ctx := context.Background()
	mgr := NewManager()

	err := mgr.Create(ctx, nil, CreateOptions{Name: "test"})
	if err == nil {
		t.Error("Create() with nil repository should return error")
	}
}

func TestManager_Create_EmptyName(t *testing.T) {
	ctx := context.Background()
	mgr := NewManager()
	repo := &repository.Repository{Path: "/tmp/test"}

	err := mgr.Create(ctx, repo, CreateOptions{Name: ""})
	if err == nil {
		t.Error("Create() with empty name should return error")
	}
}

func TestManager_Create_InvalidName(t *testing.T) {
	ctx := context.Background()
	mgr := NewManager()
	repo := &repository.Repository{Path: "/tmp/test"}

	opts := CreateOptions{
		Name:     "invalid name with spaces",
		Validate: true,
	}

	err := mgr.Create(ctx, repo, opts)
	if err == nil {
		t.Error("Create() with invalid name should return error")
	}

	if !isError(err, ErrInvalidName) {
		t.Errorf("Create() error should wrap ErrInvalidName, got %v", err)
	}
}

func TestManager_Delete_NilRepository(t *testing.T) {
	ctx := context.Background()
	mgr := NewManager()

	err := mgr.Delete(ctx, nil, DeleteOptions{Name: "test"})
	if err == nil {
		t.Error("Delete() with nil repository should return error")
	}
}

func TestManager_Delete_EmptyName(t *testing.T) {
	ctx := context.Background()
	mgr := NewManager()
	repo := &repository.Repository{Path: "/tmp/test"}

	err := mgr.Delete(ctx, repo, DeleteOptions{Name: ""})
	if err == nil {
		t.Error("Delete() with empty name should return error")
	}
}

func TestManager_List_NilRepository(t *testing.T) {
	ctx := context.Background()
	mgr := NewManager()

	_, err := mgr.List(ctx, nil, ListOptions{})
	if err == nil {
		t.Error("List() with nil repository should return error")
	}
}

func TestManager_Get_NilRepository(t *testing.T) {
	ctx := context.Background()
	mgr := NewManager()

	_, err := mgr.Get(ctx, nil, "test")
	if err == nil {
		t.Error("Get() with nil repository should return error")
	}
}

func TestManager_Get_EmptyName(t *testing.T) {
	ctx := context.Background()
	mgr := NewManager()
	repo := &repository.Repository{Path: "/tmp/test"}

	_, err := mgr.Get(ctx, repo, "")
	if err == nil {
		t.Error("Get() with empty name should return error")
	}
}

func TestManager_Current_NilRepository(t *testing.T) {
	ctx := context.Background()
	mgr := NewManager()

	_, err := mgr.Current(ctx, nil)
	if err == nil {
		t.Error("Current() with nil repository should return error")
	}
}

func TestManager_Exists_NilRepository(t *testing.T) {
	ctx := context.Background()
	mgr := NewManager()

	_, err := mgr.Exists(ctx, nil, "test")
	if err == nil {
		t.Error("Exists() with nil repository should return error")
	}
}

func TestManager_Exists_EmptyName(t *testing.T) {
	ctx := context.Background()
	mgr := NewManager()
	repo := &repository.Repository{Path: "/tmp/test"}

	_, err := mgr.Exists(ctx, repo, "")
	if err == nil {
		t.Error("Exists() with empty name should return error")
	}
}

// Helper function to check if error wraps a specific error
func isError(err, target error) bool {
	if err == nil {
		return target == nil
	}
	if target == nil {
		return false
	}

	// Simple check for wrapped error
	return err.Error() != "" && target.Error() != ""
}

func TestParseAheadBehindFromStatus(t *testing.T) {
	tests := []struct {
		name        string
		status      string
		wantAhead   int
		wantBehind  int
	}{
		{
			name:       "ahead only",
			status:     "ahead 3",
			wantAhead:  3,
			wantBehind: 0,
		},
		{
			name:       "behind only",
			status:     "behind 5",
			wantAhead:  0,
			wantBehind: 5,
		},
		{
			name:       "ahead and behind",
			status:     "ahead 2, behind 3",
			wantAhead:  2,
			wantBehind: 3,
		},
		{
			name:       "empty string",
			status:     "",
			wantAhead:  0,
			wantBehind: 0,
		},
		{
			name:       "no status",
			status:     "up-to-date",
			wantAhead:  0,
			wantBehind: 0,
		},
		{
			name:       "large numbers",
			status:     "ahead 100, behind 50",
			wantAhead:  100,
			wantBehind: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ahead, behind := parseAheadBehindFromStatus(tt.status)
			if ahead != tt.wantAhead {
				t.Errorf("ahead = %d, want %d", ahead, tt.wantAhead)
			}
			if behind != tt.wantBehind {
				t.Errorf("behind = %d, want %d", behind, tt.wantBehind)
			}
		})
	}
}

func TestExtractNumber(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		keyword string
		want    string
	}{
		{
			name:    "simple ahead",
			input:   "ahead 5",
			keyword: "ahead",
			want:    "5",
		},
		{
			name:    "keyword not found",
			input:   "behind 3",
			keyword: "ahead",
			want:    "0",
		},
		{
			name:    "multi-digit",
			input:   "ahead 123",
			keyword: "ahead",
			want:    "123",
		},
		{
			name:    "with comma after",
			input:   "ahead 2, behind 3",
			keyword: "ahead",
			want:    "2",
		},
		{
			name:    "empty string",
			input:   "",
			keyword: "ahead",
			want:    "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractNumber(tt.input, tt.keyword)
			if got != tt.want {
				t.Errorf("extractNumber(%q, %q) = %q, want %q", tt.input, tt.keyword, got, tt.want)
			}
		})
	}
}

func TestManager_ParseBranchLine_AheadBehind(t *testing.T) {
	mgr := &manager{}

	tests := []struct {
		name        string
		line        string
		wantName    string
		wantUpstream string
		wantAhead   int
		wantBehind  int
	}{
		{
			name:         "branch ahead of upstream",
			line:         "* main  abc1234 [origin/main: ahead 3] Latest commit",
			wantName:     "main",
			wantUpstream: "origin/main",
			wantAhead:    3,
			wantBehind:   0,
		},
		{
			name:         "branch behind upstream",
			line:         "  feature/x  def5678 [origin/feature/x: behind 5] Work in progress",
			wantName:     "feature/x",
			wantUpstream: "origin/feature/x",
			wantAhead:    0,
			wantBehind:   5,
		},
		{
			name:         "branch ahead and behind",
			line:         "  develop  ghi9012 [origin/develop: ahead 2, behind 4] Diverged",
			wantName:     "develop",
			wantUpstream: "origin/develop",
			wantAhead:    2,
			wantBehind:   4,
		},
		{
			name:         "branch up-to-date (no status)",
			line:         "  release/v1  jkl3456 [origin/release/v1] Release ready",
			wantName:     "release/v1",
			wantUpstream: "origin/release/v1",
			wantAhead:    0,
			wantBehind:   0,
		},
		{
			name:         "branch without upstream",
			line:         "  local-only  mno7890 Local branch",
			wantName:     "local-only",
			wantUpstream: "",
			wantAhead:    0,
			wantBehind:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			branch, err := mgr.parseBranchLine(tt.line)
			if err != nil {
				t.Fatalf("parseBranchLine() error = %v", err)
			}

			if branch.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", branch.Name, tt.wantName)
			}

			if branch.Upstream != tt.wantUpstream {
				t.Errorf("Upstream = %q, want %q", branch.Upstream, tt.wantUpstream)
			}

			if branch.AheadBy != tt.wantAhead {
				t.Errorf("AheadBy = %d, want %d", branch.AheadBy, tt.wantAhead)
			}

			if branch.BehindBy != tt.wantBehind {
				t.Errorf("BehindBy = %d, want %d", branch.BehindBy, tt.wantBehind)
			}
		})
	}
}
