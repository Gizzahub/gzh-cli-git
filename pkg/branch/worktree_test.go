package branch

import (
	"context"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

func TestNewWorktreeManager(t *testing.T) {
	mgr := NewWorktreeManager()
	if mgr == nil {
		t.Fatal("NewWorktreeManager() returned nil")
	}
}

func TestValidateWorktreePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid absolute path", "/home/user/work", false},
		{"valid relative path", "./work", false},
		{"valid home path", "~/work", false},
		{"valid name", "work", false},
		{"empty path", "", true},
		{"path with null byte", "/path\x00/to/work", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWorktreePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWorktreePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestWorktreeManager_ParseWorktreeList(t *testing.T) {
	mgr := &worktreeManager{}

	output := `worktree /home/user/projects/myapp
HEAD abc1234567890
branch refs/heads/main

worktree /home/user/work/feature-x
HEAD def4567890123
branch refs/heads/feature/x

worktree /home/user/work/detached
HEAD ghi7890123456
detached

`

	worktrees, err := mgr.parseWorktreeList(output)
	if err != nil {
		t.Fatalf("parseWorktreeList() error = %v", err)
	}

	if len(worktrees) != 3 {
		t.Fatalf("len(worktrees) = %d, want 3", len(worktrees))
	}

	// Check first worktree (main)
	if worktrees[0].Path != "/home/user/projects/myapp" {
		t.Errorf("worktrees[0].Path = %q, want %q", worktrees[0].Path, "/home/user/projects/myapp")
	}

	if worktrees[0].Branch != "main" {
		t.Errorf("worktrees[0].Branch = %q, want %q", worktrees[0].Branch, "main")
	}

	if !worktrees[0].IsMain {
		t.Error("worktrees[0].IsMain should be true")
	}

	// Check second worktree
	if worktrees[1].Path != "/home/user/work/feature-x" {
		t.Errorf("worktrees[1].Path = %q, want %q", worktrees[1].Path, "/home/user/work/feature-x")
	}

	if worktrees[1].Branch != "feature/x" {
		t.Errorf("worktrees[1].Branch = %q, want %q", worktrees[1].Branch, "feature/x")
	}

	if worktrees[1].IsMain {
		t.Error("worktrees[1].IsMain should be false")
	}

	// Check third worktree (detached)
	if worktrees[2].Path != "/home/user/work/detached" {
		t.Errorf("worktrees[2].Path = %q, want %q", worktrees[2].Path, "/home/user/work/detached")
	}

	if !worktrees[2].IsDetached {
		t.Error("worktrees[2].IsDetached should be true")
	}
}

func TestWorktreeManager_ParseWorktreeList_Locked(t *testing.T) {
	mgr := &worktreeManager{}

	output := `worktree /home/user/projects/myapp
HEAD abc1234567890
branch refs/heads/main

worktree /home/user/work/locked
HEAD def4567890123
branch refs/heads/feature/locked
locked reason: in use

`

	worktrees, err := mgr.parseWorktreeList(output)
	if err != nil {
		t.Fatalf("parseWorktreeList() error = %v", err)
	}

	if len(worktrees) != 2 {
		t.Fatalf("len(worktrees) = %d, want 2", len(worktrees))
	}

	// Check locked worktree
	if !worktrees[1].IsLocked {
		t.Error("worktrees[1].IsLocked should be true")
	}
}

func TestWorktreeManager_ParseWorktreeList_Prunable(t *testing.T) {
	mgr := &worktreeManager{}

	output := `worktree /home/user/projects/myapp
HEAD abc1234567890
branch refs/heads/main

worktree /home/user/work/orphaned
HEAD def4567890123
branch refs/heads/feature/orphaned
prunable path does not exist

`

	worktrees, err := mgr.parseWorktreeList(output)
	if err != nil {
		t.Fatalf("parseWorktreeList() error = %v", err)
	}

	if len(worktrees) != 2 {
		t.Fatalf("len(worktrees) = %d, want 2", len(worktrees))
	}

	// Check prunable worktree
	if !worktrees[1].IsPrunable {
		t.Error("worktrees[1].IsPrunable should be true")
	}
}

func TestWorktreeManager_ParseWorktreeList_Empty(t *testing.T) {
	mgr := &worktreeManager{}

	output := ""

	worktrees, err := mgr.parseWorktreeList(output)
	if err != nil {
		t.Fatalf("parseWorktreeList() error = %v", err)
	}

	if len(worktrees) != 0 {
		t.Errorf("len(worktrees) = %d, want 0", len(worktrees))
	}
}

func TestWorktreeManager_Add_NilRepository(t *testing.T) {
	ctx := context.Background()
	mgr := NewWorktreeManager()

	_, err := mgr.Add(ctx, nil, AddOptions{Path: "/tmp/test", Branch: "test"})
	if err == nil {
		t.Error("Add() with nil repository should return error")
	}
}

func TestWorktreeManager_Add_EmptyPath(t *testing.T) {
	ctx := context.Background()
	mgr := NewWorktreeManager()
	repo := &repository.Repository{Path: "/tmp/test"}

	_, err := mgr.Add(ctx, repo, AddOptions{Path: "", Branch: "test"})
	if err == nil {
		t.Error("Add() with empty path should return error")
	}
}

func TestWorktreeManager_Add_EmptyBranch(t *testing.T) {
	ctx := context.Background()
	mgr := NewWorktreeManager()
	repo := &repository.Repository{Path: "/tmp/test"}

	_, err := mgr.Add(ctx, repo, AddOptions{Path: "/tmp/work", Branch: ""})
	if err == nil {
		t.Error("Add() with empty branch should return error")
	}
}

func TestWorktreeManager_Add_DetachedNoError(t *testing.T) {
	ctx := context.Background()
	mgr := NewWorktreeManager()
	repo := &repository.Repository{Path: "/tmp/test"}

	// Detached mode should not require branch name
	opts := AddOptions{
		Path:   "/tmp/work",
		Branch: "",
		Detach: true,
	}

	// This will fail because it's not a real repo, but the validation should pass
	_, err := mgr.Add(ctx, repo, opts)
	// Error should not be about missing branch
	if err != nil && strings.Contains(err.Error(), "branch name is required") {
		t.Error("Add() with detach should not require branch name")
	}
}

func TestWorktreeManager_Remove_NilRepository(t *testing.T) {
	ctx := context.Background()
	mgr := NewWorktreeManager()

	err := mgr.Remove(ctx, nil, RemoveOptions{Path: "/tmp/test"})
	if err == nil {
		t.Error("Remove() with nil repository should return error")
	}
}

func TestWorktreeManager_Remove_EmptyPath(t *testing.T) {
	ctx := context.Background()
	mgr := NewWorktreeManager()
	repo := &repository.Repository{Path: "/tmp/test"}

	err := mgr.Remove(ctx, repo, RemoveOptions{Path: ""})
	if err == nil {
		t.Error("Remove() with empty path should return error")
	}
}

func TestWorktreeManager_List_NilRepository(t *testing.T) {
	ctx := context.Background()
	mgr := NewWorktreeManager()

	_, err := mgr.List(ctx, nil)
	if err == nil {
		t.Error("List() with nil repository should return error")
	}
}

func TestWorktreeManager_Prune_NilRepository(t *testing.T) {
	ctx := context.Background()
	mgr := NewWorktreeManager()

	err := mgr.Prune(ctx, nil)
	if err == nil {
		t.Error("Prune() with nil repository should return error")
	}
}

func TestWorktreeManager_Get_NilRepository(t *testing.T) {
	ctx := context.Background()
	mgr := NewWorktreeManager()

	_, err := mgr.Get(ctx, nil, "/tmp/test")
	if err == nil {
		t.Error("Get() with nil repository should return error")
	}
}

func TestWorktreeManager_Get_EmptyPath(t *testing.T) {
	ctx := context.Background()
	mgr := NewWorktreeManager()
	repo := &repository.Repository{Path: "/tmp/test"}

	_, err := mgr.Get(ctx, repo, "")
	if err == nil {
		t.Error("Get() with empty path should return error")
	}
}

func TestWorktreeManager_Exists_NilRepository(t *testing.T) {
	ctx := context.Background()
	mgr := NewWorktreeManager()

	_, err := mgr.Exists(ctx, nil, "/tmp/test")
	if err == nil {
		t.Error("Exists() with nil repository should return error")
	}
}

func TestWorktreeManager_Exists_EmptyPath(t *testing.T) {
	ctx := context.Background()
	mgr := NewWorktreeManager()
	repo := &repository.Repository{Path: "/tmp/test"}

	_, err := mgr.Exists(ctx, repo, "")
	if err == nil {
		t.Error("Exists() with empty path should return error")
	}
}

func TestWorktree_Struct(t *testing.T) {
	wt := &Worktree{
		Path:       "/home/user/work/feature-x",
		Branch:     "feature/x",
		Ref:        "abc1234",
		IsMain:     false,
		IsLocked:   false,
		IsPrunable: false,
		IsDetached: false,
	}

	if wt.Path != "/home/user/work/feature-x" {
		t.Errorf("Path = %q, want %q", wt.Path, "/home/user/work/feature-x")
	}

	if wt.Branch != "feature/x" {
		t.Errorf("Branch = %q, want %q", wt.Branch, "feature/x")
	}

	if wt.IsMain {
		t.Error("IsMain should be false")
	}

	if wt.IsLocked {
		t.Error("IsLocked should be false")
	}

	if wt.IsDetached {
		t.Error("IsDetached should be false")
	}
}

func TestAddOptions_Defaults(t *testing.T) {
	opts := AddOptions{
		Path:   "/tmp/work",
		Branch: "test",
	}

	if opts.Path != "/tmp/work" {
		t.Errorf("Path = %q, want %q", opts.Path, "/tmp/work")
	}

	if opts.Branch != "test" {
		t.Errorf("Branch = %q, want %q", opts.Branch, "test")
	}

	if opts.CreateBranch {
		t.Error("CreateBranch should default to false")
	}

	if opts.Force {
		t.Error("Force should default to false")
	}

	if opts.Detach {
		t.Error("Detach should default to false")
	}
}

func TestRemoveOptions_Defaults(t *testing.T) {
	opts := RemoveOptions{
		Path: "/tmp/work",
	}

	if opts.Path != "/tmp/work" {
		t.Errorf("Path = %q, want %q", opts.Path, "/tmp/work")
	}

	if opts.Force {
		t.Error("Force should default to false")
	}
}

func TestWorktreeManager_ParseWorktreeList_BareRepo(t *testing.T) {
	mgr := &worktreeManager{}

	// Bare repo output
	output := `worktree /home/user/projects/myapp.git
HEAD abc1234567890
bare

`

	worktrees, err := mgr.parseWorktreeList(output)
	if err != nil {
		t.Fatalf("parseWorktreeList() error = %v", err)
	}

	if len(worktrees) != 1 {
		t.Fatalf("len(worktrees) = %d, want 1", len(worktrees))
	}

	if !worktrees[0].IsBare {
		t.Error("worktrees[0].IsBare should be true")
	}
}

func TestWorktreeManager_ParseWorktreeList_LockedWithReason(t *testing.T) {
	mgr := &worktreeManager{}

	output := `worktree /home/user/work/locked
HEAD abc1234567890
branch refs/heads/feature/x
locked reason: being used elsewhere

`

	worktrees, err := mgr.parseWorktreeList(output)
	if err != nil {
		t.Fatalf("parseWorktreeList() error = %v", err)
	}

	if len(worktrees) != 1 {
		t.Fatalf("len(worktrees) = %d, want 1", len(worktrees))
	}

	if !worktrees[0].IsLocked {
		t.Error("worktrees[0].IsLocked should be true")
	}
}

func TestWorktreeManager_ParseWorktreeList_LockedSimple(t *testing.T) {
	mgr := &worktreeManager{}

	output := `worktree /home/user/work/locked
HEAD abc1234567890
branch refs/heads/feature/x
locked

`

	worktrees, err := mgr.parseWorktreeList(output)
	if err != nil {
		t.Fatalf("parseWorktreeList() error = %v", err)
	}

	if len(worktrees) != 1 {
		t.Fatalf("len(worktrees) = %d, want 1", len(worktrees))
	}

	if !worktrees[0].IsLocked {
		t.Error("worktrees[0].IsLocked should be true")
	}
}

func TestWorktreeManager_ParseWorktreeList_PrunableWithReason(t *testing.T) {
	mgr := &worktreeManager{}

	output := `worktree /home/user/work/orphan
HEAD abc1234567890
branch refs/heads/feature/orphan
prunable reason: gitdir file points to non-existent location

`

	worktrees, err := mgr.parseWorktreeList(output)
	if err != nil {
		t.Fatalf("parseWorktreeList() error = %v", err)
	}

	if len(worktrees) != 1 {
		t.Fatalf("len(worktrees) = %d, want 1", len(worktrees))
	}

	if !worktrees[0].IsPrunable {
		t.Error("worktrees[0].IsPrunable should be true")
	}
}

func TestWorktreeManager_ParseWorktreeList_MultipleWorktrees(t *testing.T) {
	mgr := &worktreeManager{}

	output := `worktree /home/user/projects/main
HEAD abc1234567890
branch refs/heads/main

worktree /home/user/work/feature-a
HEAD def4567890123
branch refs/heads/feature/a

worktree /home/user/work/feature-b
HEAD ghi7890123456
branch refs/heads/feature/b
locked

worktree /home/user/work/detached
HEAD jkl0123456789
detached

worktree /home/user/work/orphan
HEAD mno3456789012
branch refs/heads/feature/orphan
prunable

`

	worktrees, err := mgr.parseWorktreeList(output)
	if err != nil {
		t.Fatalf("parseWorktreeList() error = %v", err)
	}

	if len(worktrees) != 5 {
		t.Fatalf("len(worktrees) = %d, want 5", len(worktrees))
	}

	// Check main worktree
	if !worktrees[0].IsMain {
		t.Error("first worktree should be main")
	}

	// Check locked worktree (feature-b)
	if !worktrees[2].IsLocked {
		t.Error("feature-b should be locked")
	}

	// Check detached worktree
	if !worktrees[3].IsDetached {
		t.Error("detached worktree should have IsDetached=true")
	}

	// Check prunable worktree
	if !worktrees[4].IsPrunable {
		t.Error("orphan worktree should be prunable")
	}
}

func TestWorktree_AllFields(t *testing.T) {
	wt := &Worktree{
		Path:       "/home/user/work/test",
		Branch:     "feature/test",
		Ref:        "abc123def456",
		IsMain:     true,
		IsLocked:   true,
		IsPrunable: true,
		IsBare:     true,
		IsDetached: true,
	}

	if wt.Path != "/home/user/work/test" {
		t.Errorf("Path = %q, want %q", wt.Path, "/home/user/work/test")
	}

	if wt.Branch != "feature/test" {
		t.Errorf("Branch = %q, want %q", wt.Branch, "feature/test")
	}

	if wt.Ref != "abc123def456" {
		t.Errorf("Ref = %q, want %q", wt.Ref, "abc123def456")
	}

	if !wt.IsMain {
		t.Error("IsMain should be true")
	}

	if !wt.IsLocked {
		t.Error("IsLocked should be true")
	}

	if !wt.IsPrunable {
		t.Error("IsPrunable should be true")
	}

	if !wt.IsBare {
		t.Error("IsBare should be true")
	}

	if !wt.IsDetached {
		t.Error("IsDetached should be true")
	}
}

func TestAddOptions_AllFields(t *testing.T) {
	opts := AddOptions{
		Path:         "/home/user/work/new",
		Branch:       "feature/new",
		CreateBranch: true,
		Force:        true,
		Detach:       true,
		Checkout:     "abc123",
	}

	if opts.Path != "/home/user/work/new" {
		t.Errorf("Path = %q, want %q", opts.Path, "/home/user/work/new")
	}

	if opts.Branch != "feature/new" {
		t.Errorf("Branch = %q, want %q", opts.Branch, "feature/new")
	}

	if !opts.CreateBranch {
		t.Error("CreateBranch should be true")
	}

	if !opts.Force {
		t.Error("Force should be true")
	}

	if !opts.Detach {
		t.Error("Detach should be true")
	}

	if opts.Checkout != "abc123" {
		t.Errorf("Checkout = %q, want %q", opts.Checkout, "abc123")
	}
}

func TestRemoveOptions_AllFields(t *testing.T) {
	opts := RemoveOptions{
		Path:  "/home/user/work/old",
		Force: true,
	}

	if opts.Path != "/home/user/work/old" {
		t.Errorf("Path = %q, want %q", opts.Path, "/home/user/work/old")
	}

	if !opts.Force {
		t.Error("Force should be true")
	}
}

func TestValidateWorktreePath_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid with spaces", "/home/user/my work", false},
		{"valid unicode", "/home/user/工作", false},
		{"valid with dots", "/home/user/work/../other", false},
		{"null at start", "\x00/path", true},
		{"null at end", "/path\x00", true},
		{"null in middle", "/pa\x00th", true},
		{"whitespace only", "   ", false}, // Not explicitly rejected
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWorktreePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWorktreePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}
