package branch

import (
	"context"
	"testing"

	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

func TestNewParallelWorkflow(t *testing.T) {
	pw := NewParallelWorkflow()
	if pw == nil {
		t.Fatal("NewParallelWorkflow() returned nil")
	}
}

func TestParallelWorkflow_GetActiveContexts_NilRepository(t *testing.T) {
	ctx := context.Background()
	pw := NewParallelWorkflow()

	_, err := pw.GetActiveContexts(ctx, nil)
	if err == nil {
		t.Error("GetActiveContexts() with nil repository should return error")
	}
}

func TestParallelWorkflow_SwitchContext_NilRepository(t *testing.T) {
	ctx := context.Background()
	pw := NewParallelWorkflow()

	_, err := pw.SwitchContext(ctx, nil, "/tmp/work")
	if err == nil {
		t.Error("SwitchContext() with nil repository should return error")
	}
}

func TestParallelWorkflow_SwitchContext_EmptyPath(t *testing.T) {
	ctx := context.Background()
	pw := NewParallelWorkflow()
	repo := &repository.Repository{Path: "/tmp/test"}

	_, err := pw.SwitchContext(ctx, repo, "")
	if err == nil {
		t.Error("SwitchContext() with empty path should return error")
	}
}

func TestParallelWorkflow_DetectConflicts_NilRepository(t *testing.T) {
	ctx := context.Background()
	pw := NewParallelWorkflow()

	_, err := pw.DetectConflicts(ctx, nil)
	if err == nil {
		t.Error("DetectConflicts() with nil repository should return error")
	}
}

func TestParallelWorkflow_GetStatus_NilRepository(t *testing.T) {
	ctx := context.Background()
	pw := NewParallelWorkflow()

	_, err := pw.GetStatus(ctx, nil)
	if err == nil {
		t.Error("GetStatus() with nil repository should return error")
	}
}

func TestParallelWorkflow_DetermineConflictSeverity(t *testing.T) {
	pw := &parallelWorkflow{}

	tests := []struct {
		name      string
		file      string
		worktrees []string
		want      ConflictSeverity
	}{
		{
			name:      "high severity - multiple worktrees",
			file:      "src/main.go",
			worktrees: []string{"/work/a", "/work/b"},
			want:      SeverityHigh,
		},
		{
			name:      "low severity - single worktree",
			file:      "src/main.go",
			worktrees: []string{"/work/a"},
			want:      SeverityLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pw.determineConflictSeverity(tt.file, tt.worktrees)
			if got != tt.want {
				t.Errorf("determineConflictSeverity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorkContext_Struct(t *testing.T) {
	ctx := &WorkContext{
		Path:          "/work/feature-a",
		Branch:        "feature/a",
		IsMain:        false,
		HasChanges:    true,
		ModifiedFiles: []string{"file1.go", "file2.go"},
	}

	if ctx.Path != "/work/feature-a" {
		t.Errorf("Path = %q, want %q", ctx.Path, "/work/feature-a")
	}

	if ctx.Branch != "feature/a" {
		t.Errorf("Branch = %q, want %q", ctx.Branch, "feature/a")
	}

	if !ctx.HasChanges {
		t.Error("HasChanges should be true")
	}

	if len(ctx.ModifiedFiles) != 2 {
		t.Errorf("len(ModifiedFiles) = %d, want 2", len(ctx.ModifiedFiles))
	}
}

func TestSwitchInfo_Struct(t *testing.T) {
	info := &SwitchInfo{
		FromPath:   "/work/current",
		ToPath:     "/work/target",
		ToBranch:   "feature/x",
		Command:    "cd /work/target",
		HasChanges: true,
	}

	if info.FromPath != "/work/current" {
		t.Errorf("FromPath = %q, want %q", info.FromPath, "/work/current")
	}

	if info.ToPath != "/work/target" {
		t.Errorf("ToPath = %q, want %q", info.ToPath, "/work/target")
	}

	if info.ToBranch != "feature/x" {
		t.Errorf("ToBranch = %q, want %q", info.ToBranch, "feature/x")
	}

	if info.Command != "cd /work/target" {
		t.Errorf("Command = %q, want %q", info.Command, "cd /work/target")
	}

	if !info.HasChanges {
		t.Error("HasChanges should be true")
	}
}

func TestConflict_Struct(t *testing.T) {
	conflict := &Conflict{
		File:      "src/main.go",
		Worktrees: []string{"/work/a", "/work/b"},
		Severity:  SeverityHigh,
	}

	if conflict.File != "src/main.go" {
		t.Errorf("File = %q, want %q", conflict.File, "src/main.go")
	}

	if len(conflict.Worktrees) != 2 {
		t.Errorf("len(Worktrees) = %d, want 2", len(conflict.Worktrees))
	}

	if conflict.Severity != SeverityHigh {
		t.Errorf("Severity = %q, want %q", conflict.Severity, SeverityHigh)
	}
}

func TestConflictSeverity_Constants(t *testing.T) {
	severities := []struct {
		got  ConflictSeverity
		want string
	}{
		{SeverityLow, "low"},
		{SeverityMedium, "medium"},
		{SeverityHigh, "high"},
	}

	for _, tt := range severities {
		if string(tt.got) != tt.want {
			t.Errorf("ConflictSeverity = %q, want %q", tt.got, tt.want)
		}
	}
}

func TestParallelStatus_HasConflicts(t *testing.T) {
	tests := []struct {
		name   string
		status *ParallelStatus
		want   bool
	}{
		{
			name: "no conflicts",
			status: &ParallelStatus{
				Conflicts: 0,
			},
			want: false,
		},
		{
			name: "has conflicts",
			status: &ParallelStatus{
				Conflicts: 2,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.HasConflicts()
			if got != tt.want {
				t.Errorf("HasConflicts() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParallelStatus_IsActive(t *testing.T) {
	tests := []struct {
		name   string
		status *ParallelStatus
		want   bool
	}{
		{
			name: "no active worktrees",
			status: &ParallelStatus{
				ActiveWorktrees: 0,
			},
			want: false,
		},
		{
			name: "has active worktrees",
			status: &ParallelStatus{
				ActiveWorktrees: 2,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.IsActive()
			if got != tt.want {
				t.Errorf("IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParallelStatus_GetMainContext(t *testing.T) {
	mainCtx := &WorkContext{
		Path:   "/project",
		Branch: "main",
		IsMain: true,
	}

	otherCtx := &WorkContext{
		Path:   "/work/feature-a",
		Branch: "feature/a",
		IsMain: false,
	}

	status := &ParallelStatus{
		Contexts: []*WorkContext{mainCtx, otherCtx},
	}

	got := status.GetMainContext()
	if got == nil {
		t.Fatal("GetMainContext() returned nil")
	}

	if got.Path != "/project" {
		t.Errorf("GetMainContext().Path = %q, want %q", got.Path, "/project")
	}

	if !got.IsMain {
		t.Error("GetMainContext().IsMain should be true")
	}
}

func TestParallelStatus_GetMainContext_None(t *testing.T) {
	status := &ParallelStatus{
		Contexts: []*WorkContext{
			{Path: "/work/a", IsMain: false},
			{Path: "/work/b", IsMain: false},
		},
	}

	got := status.GetMainContext()
	if got != nil {
		t.Error("GetMainContext() should return nil when no main context")
	}
}

func TestParallelStatus_GetActiveContexts(t *testing.T) {
	status := &ParallelStatus{
		Contexts: []*WorkContext{
			{Path: "/work/a", HasChanges: true},
			{Path: "/work/b", HasChanges: false},
			{Path: "/work/c", HasChanges: true},
		},
	}

	active := status.GetActiveContexts()

	if len(active) != 2 {
		t.Errorf("len(GetActiveContexts()) = %d, want 2", len(active))
	}

	if active[0].Path != "/work/a" {
		t.Errorf("active[0].Path = %q, want %q", active[0].Path, "/work/a")
	}

	if active[1].Path != "/work/c" {
		t.Errorf("active[1].Path = %q, want %q", active[1].Path, "/work/c")
	}
}

func TestParallelStatus_GetActiveContexts_Empty(t *testing.T) {
	status := &ParallelStatus{
		Contexts: []*WorkContext{
			{Path: "/work/a", HasChanges: false},
			{Path: "/work/b", HasChanges: false},
		},
	}

	active := status.GetActiveContexts()

	if len(active) != 0 {
		t.Errorf("len(GetActiveContexts()) = %d, want 0", len(active))
	}
}

func TestParallelStatus_Struct(t *testing.T) {
	status := &ParallelStatus{
		TotalWorktrees:  3,
		ActiveWorktrees: 2,
		Conflicts:       1,
		Contexts: []*WorkContext{
			{Path: "/work/a"},
			{Path: "/work/b"},
			{Path: "/work/c"},
		},
	}

	if status.TotalWorktrees != 3 {
		t.Errorf("TotalWorktrees = %d, want 3", status.TotalWorktrees)
	}

	if status.ActiveWorktrees != 2 {
		t.Errorf("ActiveWorktrees = %d, want 2", status.ActiveWorktrees)
	}

	if status.Conflicts != 1 {
		t.Errorf("Conflicts = %d, want 1", status.Conflicts)
	}

	if len(status.Contexts) != 3 {
		t.Errorf("len(Contexts) = %d, want 3", len(status.Contexts))
	}
}

func TestParallelWorkflow_DetermineConflictSeverity_EdgeCases(t *testing.T) {
	pw := &parallelWorkflow{}

	tests := []struct {
		name      string
		file      string
		worktrees []string
		want      ConflictSeverity
	}{
		{
			name:      "empty worktrees",
			file:      "src/main.go",
			worktrees: []string{},
			want:      SeverityLow,
		},
		{
			name:      "three worktrees - still high",
			file:      "src/main.go",
			worktrees: []string{"/work/a", "/work/b", "/work/c"},
			want:      SeverityHigh,
		},
		{
			name:      "exactly two worktrees",
			file:      "config.yaml",
			worktrees: []string{"/work/x", "/work/y"},
			want:      SeverityHigh,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pw.determineConflictSeverity(tt.file, tt.worktrees)
			if got != tt.want {
				t.Errorf("determineConflictSeverity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorkContext_AllFields(t *testing.T) {
	ctx := &WorkContext{
		Path:          "/work/feature-branch",
		Branch:        "feature/user-auth",
		IsMain:        false,
		HasChanges:    true,
		ModifiedFiles: []string{"auth.go", "user.go", "test.go"},
	}

	// Test all fields
	if ctx.Path != "/work/feature-branch" {
		t.Errorf("Path = %q, want %q", ctx.Path, "/work/feature-branch")
	}

	if ctx.Branch != "feature/user-auth" {
		t.Errorf("Branch = %q, want %q", ctx.Branch, "feature/user-auth")
	}

	if ctx.IsMain {
		t.Error("IsMain should be false")
	}

	if !ctx.HasChanges {
		t.Error("HasChanges should be true")
	}

	if len(ctx.ModifiedFiles) != 3 {
		t.Errorf("len(ModifiedFiles) = %d, want 3", len(ctx.ModifiedFiles))
	}
}

func TestWorkContext_MainContext(t *testing.T) {
	ctx := &WorkContext{
		Path:          "/project",
		Branch:        "main",
		IsMain:        true,
		HasChanges:    false,
		ModifiedFiles: []string{},
	}

	if !ctx.IsMain {
		t.Error("IsMain should be true for main context")
	}

	if ctx.HasChanges {
		t.Error("HasChanges should be false")
	}

	if len(ctx.ModifiedFiles) != 0 {
		t.Errorf("len(ModifiedFiles) = %d, want 0", len(ctx.ModifiedFiles))
	}
}

func TestSwitchInfo_AllFields(t *testing.T) {
	info := &SwitchInfo{
		FromPath:   "/work/current",
		ToPath:     "/work/target",
		ToBranch:   "feature/new",
		Command:    "cd /work/target && git checkout feature/new",
		HasChanges: false,
	}

	if info.FromPath != "/work/current" {
		t.Errorf("FromPath = %q, want %q", info.FromPath, "/work/current")
	}

	if info.ToPath != "/work/target" {
		t.Errorf("ToPath = %q, want %q", info.ToPath, "/work/target")
	}

	if info.ToBranch != "feature/new" {
		t.Errorf("ToBranch = %q, want %q", info.ToBranch, "feature/new")
	}

	if info.Command == "" {
		t.Error("Command should not be empty")
	}

	if info.HasChanges {
		t.Error("HasChanges should be false")
	}
}

func TestConflict_AllFields(t *testing.T) {
	conflict := &Conflict{
		File:      "src/database/connection.go",
		Worktrees: []string{"/work/feature-a", "/work/feature-b", "/work/feature-c"},
		Severity:  SeverityHigh,
	}

	if conflict.File != "src/database/connection.go" {
		t.Errorf("File = %q, want specific path", conflict.File)
	}

	if len(conflict.Worktrees) != 3 {
		t.Errorf("len(Worktrees) = %d, want 3", len(conflict.Worktrees))
	}

	if conflict.Severity != SeverityHigh {
		t.Errorf("Severity = %v, want %v", conflict.Severity, SeverityHigh)
	}
}

func TestParallelStatus_EmptyContexts(t *testing.T) {
	status := &ParallelStatus{
		TotalWorktrees:  0,
		ActiveWorktrees: 0,
		Conflicts:       0,
		Contexts:        []*WorkContext{},
	}

	if status.HasConflicts() {
		t.Error("HasConflicts() should be false for zero conflicts")
	}

	if status.IsActive() {
		t.Error("IsActive() should be false for zero active worktrees")
	}

	if main := status.GetMainContext(); main != nil {
		t.Error("GetMainContext() should return nil for empty contexts")
	}

	active := status.GetActiveContexts()
	if len(active) != 0 {
		t.Errorf("len(GetActiveContexts()) = %d, want 0", len(active))
	}
}

func TestParallelStatus_NilContexts(t *testing.T) {
	status := &ParallelStatus{
		Contexts: nil,
	}

	// Should not panic
	main := status.GetMainContext()
	if main != nil {
		t.Error("GetMainContext() should return nil for nil contexts")
	}

	active := status.GetActiveContexts()
	if len(active) != 0 {
		t.Errorf("len(GetActiveContexts()) = %d, want 0", len(active))
	}
}
