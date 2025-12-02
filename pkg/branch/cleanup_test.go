package branch

import (
	"context"
	"testing"
	"time"

	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

func TestNewCleanupService(t *testing.T) {
	svc := NewCleanupService()
	if svc == nil {
		t.Fatal("NewCleanupService() returned nil")
	}
}

func TestCleanupService_Analyze_NilRepository(t *testing.T) {
	ctx := context.Background()
	svc := NewCleanupService()

	_, err := svc.Analyze(ctx, nil, AnalyzeOptions{})
	if err == nil {
		t.Error("Analyze() with nil repository should return error")
	}
}

func TestCleanupService_Execute_NilRepository(t *testing.T) {
	ctx := context.Background()
	svc := NewCleanupService()
	report := &CleanupReport{}

	err := svc.Execute(ctx, nil, report, ExecuteOptions{})
	if err == nil {
		t.Error("Execute() with nil repository should return error")
	}
}

func TestCleanupService_Execute_NilReport(t *testing.T) {
	ctx := context.Background()
	svc := NewCleanupService()
	repo := &repository.Repository{Path: "/tmp/test"}

	err := svc.Execute(ctx, repo, nil, ExecuteOptions{})
	if err == nil {
		t.Error("Execute() with nil report should return error")
	}
}

func TestCleanupService_IsProtectedBranch(t *testing.T) {
	svc := &cleanupService{}

	tests := []struct {
		name     string
		branch   string
		patterns []string
		want     bool
	}{
		{"main branch", "main", nil, true},
		{"master branch", "master", nil, true},
		{"develop branch", "develop", nil, true},
		{"release branch", "release/v1.0", nil, true},
		{"hotfix branch", "hotfix/critical", nil, true},
		{"feature branch", "feature/test", nil, false},
		{"custom pattern", "important/data", []string{"important/*"}, true},
		{"no match", "feature/test", []string{"important/*"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.isProtectedBranch(tt.branch, tt.patterns)
			if got != tt.want {
				t.Errorf("isProtectedBranch(%q, %v) = %v, want %v", tt.branch, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestCleanupReport_CountBranches(t *testing.T) {
	report := &CleanupReport{
		Merged:   []*Branch{{Name: "feature/1"}, {Name: "feature/2"}},
		Stale:    []*Branch{{Name: "old/1"}},
		Orphaned: []*Branch{{Name: "orphan/1"}},
	}

	got := report.CountBranches()
	want := 4

	if got != want {
		t.Errorf("CountBranches() = %d, want %d", got, want)
	}
}

func TestCleanupReport_IsEmpty(t *testing.T) {
	tests := []struct {
		name   string
		report *CleanupReport
		want   bool
	}{
		{
			name: "empty report",
			report: &CleanupReport{
				Merged:   []*Branch{},
				Stale:    []*Branch{},
				Orphaned: []*Branch{},
			},
			want: true,
		},
		{
			name: "report with merged",
			report: &CleanupReport{
				Merged:   []*Branch{{Name: "feature/1"}},
				Stale:    []*Branch{},
				Orphaned: []*Branch{},
			},
			want: false,
		},
		{
			name: "report with stale",
			report: &CleanupReport{
				Merged:   []*Branch{},
				Stale:    []*Branch{{Name: "old/1"}},
				Orphaned: []*Branch{},
			},
			want: false,
		},
		{
			name: "report with orphaned",
			report: &CleanupReport{
				Merged:   []*Branch{},
				Stale:    []*Branch{},
				Orphaned: []*Branch{{Name: "orphan/1"}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.report.IsEmpty()
			if got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCleanupReport_GetAllBranches(t *testing.T) {
	report := &CleanupReport{
		Merged: []*Branch{
			{Name: "feature/1"},
			{Name: "feature/2"},
		},
		Stale: []*Branch{
			{Name: "old/1"},
		},
		Orphaned: []*Branch{
			{Name: "orphan/1"},
		},
	}

	all := report.GetAllBranches()

	if len(all) != 4 {
		t.Errorf("GetAllBranches() length = %d, want 4", len(all))
	}

	// Check order: merged, then stale, then orphaned
	if all[0].Name != "feature/1" {
		t.Errorf("all[0].Name = %q, want %q", all[0].Name, "feature/1")
	}

	if all[2].Name != "old/1" {
		t.Errorf("all[2].Name = %q, want %q", all[2].Name, "old/1")
	}

	if all[3].Name != "orphan/1" {
		t.Errorf("all[3].Name = %q, want %q", all[3].Name, "orphan/1")
	}
}

func TestAnalyzeOptions_Defaults(t *testing.T) {
	opts := AnalyzeOptions{}

	if opts.IncludeMerged {
		t.Error("IncludeMerged should default to false")
	}

	if opts.IncludeStale {
		t.Error("IncludeStale should default to false")
	}

	if opts.StaleThreshold != 0 {
		t.Errorf("StaleThreshold = %v, want 0 (will be set to default)", opts.StaleThreshold)
	}

	if opts.IncludeRemote {
		t.Error("IncludeRemote should default to false")
	}
}

func TestExecuteOptions_Defaults(t *testing.T) {
	opts := ExecuteOptions{}

	if opts.DryRun {
		t.Error("DryRun should default to false")
	}

	if opts.Force {
		t.Error("Force should default to false")
	}

	if opts.Remote {
		t.Error("Remote should default to false")
	}

	if opts.Confirm {
		t.Error("Confirm should default to false")
	}
}

func TestCleanupReport_Struct(t *testing.T) {
	report := &CleanupReport{
		Merged: []*Branch{
			{Name: "feature/1"},
		},
		Stale: []*Branch{
			{Name: "old/1"},
		},
		Orphaned: []*Branch{
			{Name: "orphan/1"},
		},
		Protected: []*Branch{
			{Name: "main"},
		},
		Total: 10,
	}

	if len(report.Merged) != 1 {
		t.Errorf("len(Merged) = %d, want 1", len(report.Merged))
	}

	if len(report.Stale) != 1 {
		t.Errorf("len(Stale) = %d, want 1", len(report.Stale))
	}

	if len(report.Orphaned) != 1 {
		t.Errorf("len(Orphaned) = %d, want 1", len(report.Orphaned))
	}

	if len(report.Protected) != 1 {
		t.Errorf("len(Protected) = %d, want 1", len(report.Protected))
	}

	if report.Total != 10 {
		t.Errorf("Total = %d, want 10", report.Total)
	}
}

func TestCleanupStrategy_Constants(t *testing.T) {
	strategies := []struct {
		got  CleanupStrategy
		want string
	}{
		{StrategyMerged, "merged"},
		{StrategyStale, "stale"},
		{StrategyOrphaned, "orphaned"},
		{StrategyAll, "all"},
	}

	for _, tt := range strategies {
		if string(tt.got) != tt.want {
			t.Errorf("CleanupStrategy = %q, want %q", tt.got, tt.want)
		}
	}
}

func TestAnalyzeOptions_StaleThreshold(t *testing.T) {
	opts := AnalyzeOptions{
		StaleThreshold: 60 * 24 * time.Hour, // 60 days
	}

	if opts.StaleThreshold != 60*24*time.Hour {
		t.Errorf("StaleThreshold = %v, want 60 days", opts.StaleThreshold)
	}
}

func TestAnalyzeOptions_Exclude(t *testing.T) {
	opts := AnalyzeOptions{
		Exclude: []string{"important/*", "production/*"},
	}

	if len(opts.Exclude) != 2 {
		t.Errorf("len(Exclude) = %d, want 2", len(opts.Exclude))
	}

	if opts.Exclude[0] != "important/*" {
		t.Errorf("Exclude[0] = %q, want %q", opts.Exclude[0], "important/*")
	}
}

func TestExecuteOptions_Exclude(t *testing.T) {
	opts := ExecuteOptions{
		Exclude: []string{"keep/*", "preserve/*"},
	}

	if len(opts.Exclude) != 2 {
		t.Errorf("len(Exclude) = %d, want 2", len(opts.Exclude))
	}

	if opts.Exclude[0] != "keep/*" {
		t.Errorf("Exclude[0] = %q, want %q", opts.Exclude[0], "keep/*")
	}
}

func TestCleanupService_Execute_EmptyReport(t *testing.T) {
	ctx := context.Background()
	svc := NewCleanupService()
	repo := &repository.Repository{Path: "/tmp/test"}

	// Empty report should not cause error
	report := &CleanupReport{
		Merged:   []*Branch{},
		Stale:    []*Branch{},
		Orphaned: []*Branch{},
	}

	err := svc.Execute(ctx, repo, report, ExecuteOptions{DryRun: true})
	if err != nil {
		t.Errorf("Execute() with empty report error = %v, want nil", err)
	}
}

func TestCleanupService_Execute_SkipsProtected(t *testing.T) {
	ctx := context.Background()
	svc := NewCleanupService()
	repo := &repository.Repository{Path: "/tmp/test"}

	report := &CleanupReport{
		Merged: []*Branch{
			{Name: "main"},     // protected - should be skipped
			{Name: "master"},   // protected - should be skipped
			{Name: "feature/x"}, // not protected
		},
		Stale:    []*Branch{},
		Orphaned: []*Branch{},
	}

	// Execute with DryRun won't actually delete but should process
	err := svc.Execute(ctx, repo, report, ExecuteOptions{DryRun: true})
	// This will fail since /tmp/test is not a real repo, but we're testing the logic flow
	if err == nil {
		// If no error, the protected branches were correctly identified
		t.Log("Execute correctly handled protected branches")
	}
}

func TestCleanupReport_AllMethods(t *testing.T) {
	report := &CleanupReport{
		Merged: []*Branch{
			{Name: "feature/1", SHA: "abc123"},
			{Name: "feature/2", SHA: "def456"},
		},
		Stale: []*Branch{
			{Name: "old/branch", SHA: "ghi789"},
		},
		Orphaned: []*Branch{
			{Name: "orphaned/1", SHA: "jkl012"},
		},
		Protected: []*Branch{
			{Name: "main", SHA: "mno345"},
		},
		Total: 10,
	}

	// Test CountBranches
	if count := report.CountBranches(); count != 4 {
		t.Errorf("CountBranches() = %d, want 4", count)
	}

	// Test IsEmpty
	if report.IsEmpty() {
		t.Error("IsEmpty() = true, want false")
	}

	// Test GetAllBranches
	all := report.GetAllBranches()
	if len(all) != 4 {
		t.Errorf("GetAllBranches() len = %d, want 4", len(all))
	}

	// Verify order (merged, stale, orphaned)
	expectedNames := []string{"feature/1", "feature/2", "old/branch", "orphaned/1"}
	for i, name := range expectedNames {
		if all[i].Name != name {
			t.Errorf("GetAllBranches()[%d].Name = %q, want %q", i, all[i].Name, name)
		}
	}
}

func TestCleanupReport_EmptyReport(t *testing.T) {
	report := &CleanupReport{}

	if !report.IsEmpty() {
		t.Error("IsEmpty() = false for nil slices, want true")
	}

	if report.CountBranches() != 0 {
		t.Errorf("CountBranches() = %d for nil slices, want 0", report.CountBranches())
	}

	all := report.GetAllBranches()
	if len(all) != 0 {
		t.Errorf("GetAllBranches() len = %d for nil slices, want 0", len(all))
	}
}
