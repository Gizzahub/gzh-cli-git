// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package workspacecli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/reposync"
)

func TestValidateComparisonBranchUsesGitRefRules(t *testing.T) {
	for _, branch := range []string{"--upload-pack=/tmp/evil", "feature..bad"} {
		if err := validateComparisonBranch(branch); err == nil {
			t.Errorf("validateComparisonBranch(%q) = nil, want error", branch)
		}
	}
	for _, branch := range []string{"HEAD", "feature;touch", "team/topic$1"} {
		if err := validateComparisonBranch(branch); err != nil {
			t.Errorf("validateComparisonBranch(%q) = %v, want nil", branch, err)
		}
	}
}

func TestGetFileDiffRejectsInvalidBranchBeforeGit(t *testing.T) {
	_, err := getFileDiff(context.Background(), t.TempDir(), "--upload-pack=/tmp/evil")
	if err == nil || !strings.Contains(err.Error(), "invalid comparison branch") {
		t.Fatalf("getFileDiff invalid branch error = %v", err)
	}
}

func TestBuildSyncSummary(t *testing.T) {
	tests := []struct {
		name       string
		actions    []reposync.Action
		wantClone  int
		wantUpdate int
		wantSkip   int
		wantDelete int
		wantTotal  int
	}{
		{
			name:      "empty actions",
			actions:   []reposync.Action{},
			wantTotal: 0,
		},
		{
			name: "mixed actions",
			actions: []reposync.Action{
				{Type: reposync.ActionClone, Repo: reposync.RepoSpec{Name: "repo1"}},
				{Type: reposync.ActionClone, Repo: reposync.RepoSpec{Name: "repo2"}},
				{Type: reposync.ActionUpdate, Repo: reposync.RepoSpec{Name: "repo3"}},
				{Type: reposync.ActionSkip, Repo: reposync.RepoSpec{Name: "repo4"}},
			},
			wantClone:  2,
			wantUpdate: 1,
			wantSkip:   1,
			wantTotal:  4,
		},
		{
			name: "all updates",
			actions: []reposync.Action{
				{Type: reposync.ActionUpdate, Repo: reposync.RepoSpec{Name: "repo1"}},
				{Type: reposync.ActionUpdate, Repo: reposync.RepoSpec{Name: "repo2"}},
				{Type: reposync.ActionUpdate, Repo: reposync.RepoSpec{Name: "repo3"}},
			},
			wantUpdate: 3,
			wantTotal:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := buildSyncSummary(tt.actions)

			if summary.Clone != tt.wantClone {
				t.Errorf("Clone = %d, want %d", summary.Clone, tt.wantClone)
			}
			if summary.Update != tt.wantUpdate {
				t.Errorf("Update = %d, want %d", summary.Update, tt.wantUpdate)
			}
			if summary.Skip != tt.wantSkip {
				t.Errorf("Skip = %d, want %d", summary.Skip, tt.wantSkip)
			}
			if summary.Delete != tt.wantDelete {
				t.Errorf("Delete = %d, want %d", summary.Delete, tt.wantDelete)
			}
			if summary.Total != tt.wantTotal {
				t.Errorf("Total = %d, want %d", summary.Total, tt.wantTotal)
			}
		})
	}
}

func TestDisplaySyncSummary(t *testing.T) {
	tests := []struct {
		name         string
		summary      syncSummary
		wantContains []string
	}{
		{
			name: "mixed summary",
			summary: syncSummary{
				Clone:  2,
				Update: 3,
				Skip:   1,
				Total:  6,
			},
			wantContains: []string{
				"Sync Preview",
				"Total: 6 repositories",
				"2 will be cloned",
				"3 will be updated",
				"1 will be skipped",
			},
		},
		{
			name: "clone only",
			summary: syncSummary{
				Clone: 5,
				Total: 5,
			},
			wantContains: []string{
				"5 will be cloned",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			displaySyncSummary(&buf, tt.summary)
			output := buf.String()

			for _, want := range tt.wantContains {
				if !bytes.Contains([]byte(output), []byte(want)) {
					t.Errorf("output missing %q, got:\n%s", want, output)
				}
			}
		})
	}
}

func TestDisplayRepoChange(t *testing.T) {
	tests := []struct {
		name         string
		change       RepoChanges
		wantContains []string
	}{
		{
			name: "clone action",
			change: RepoChanges{
				RepoName: "my-project",
				Action:   reposync.ActionClone,
				URL:      "https://github.com/owner/my-project.git",
			},
			wantContains: []string{
				"+ my-project (clone)",
				"https://github.com/owner/my-project.git",
			},
		},
		{
			name: "update with file changes",
			change: RepoChanges{
				RepoName: "api-server",
				Action:   reposync.ActionUpdate,
				Files: FileChangeSummary{
					Added:    []string{"new_file.go"},
					Modified: []string{"main.go", "config.yaml"},
					Deleted:  []string{"old_file.txt"},
					Total:    4,
				},
			},
			wantContains: []string{
				"↓ api-server (update)",
				"+1 added",
				"~2 modified",
				"-1 deleted",
				"main.go",
				"config.yaml",
			},
		},
		{
			name: "update with conflicts",
			change: RepoChanges{
				RepoName: "web-app",
				Action:   reposync.ActionUpdate,
				Conflicts: []ConflictInfo{
					{
						ConflictType: ConflictTypeDirtyWorktree,
						LocalChanges: true,
						Description:  "Uncommitted local changes detected",
					},
				},
				Warnings: []string{"Local uncommitted changes may be overwritten"},
			},
			wantContains: []string{
				"↓ web-app (update)",
				"⚠️  Uncommitted local changes detected",
			},
		},
		{
			name: "update with diverged branch",
			change: RepoChanges{
				RepoName: "legacy-app",
				Action:   reposync.ActionUpdate,
				Diverged: true,
				Conflicts: []ConflictInfo{
					{
						ConflictType:  ConflictTypeDivergedBranches,
						LocalChanges:  true,
						RemoteChanges: true,
						Description:   "Local branch has diverged from remote",
					},
				},
			},
			wantContains: []string{
				"↓ legacy-app (update)",
				"⚠️  Local branch has diverged from remote",
				"⚠️  Branch has diverged from remote",
			},
		},
		{
			name: "skip action",
			change: RepoChanges{
				RepoName: "stable-lib",
				Action:   reposync.ActionSkip,
			},
			wantContains: []string{
				"⊘ stable-lib (skip - up to date)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			displayRepoChange(&buf, tt.change)
			output := buf.String()

			for _, want := range tt.wantContains {
				if !bytes.Contains([]byte(output), []byte(want)) {
					t.Errorf("output missing %q, got:\n%s", want, output)
				}
			}
		})
	}
}

func TestDisplayDetailedSyncPreview(t *testing.T) {
	changes := []RepoChanges{
		{
			RepoName: "my-project",
			Action:   reposync.ActionClone,
			URL:      "https://github.com/owner/my-project.git",
		},
		{
			RepoName: "api-server",
			Action:   reposync.ActionUpdate,
			Files: FileChangeSummary{
				Modified: []string{"main.go"},
				Total:    1,
			},
			Warnings: []string{"Local uncommitted changes may be overwritten"},
		},
	}

	summary := syncSummary{
		Clone:  1,
		Update: 1,
		Total:  2,
	}

	var buf bytes.Buffer
	displayDetailedSyncPreview(&buf, changes, summary)
	output := buf.String()

	wantContains := []string{
		"Sync Preview (Detailed)",
		"Total: 2 repositories",
		"1 will be cloned",
		"1 will be updated",
		"⚠️  Warnings:",
		"api-server: Local uncommitted changes may be overwritten",
		"Repository Details:",
		"+ my-project (clone)",
		"↓ api-server (update)",
	}

	for _, want := range wantContains {
		if !bytes.Contains([]byte(output), []byte(want)) {
			t.Errorf("output missing %q, got:\n%s", want, output)
		}
	}
}

func TestDisplayFileList(t *testing.T) {
	tests := []struct {
		name         string
		label        string
		files        []string
		maxShow      int
		wantContains []string
		wantNot      []string
	}{
		{
			name:    "short list",
			label:   "modified",
			files:   []string{"file1.go", "file2.go"},
			maxShow: 5,
			wantContains: []string{
				"file1.go",
				"file2.go",
			},
			wantNot: []string{"and", "more"},
		},
		{
			name:    "truncated list",
			label:   "modified",
			files:   []string{"f1.go", "f2.go", "f3.go", "f4.go", "f5.go", "f6.go"},
			maxShow: 3,
			wantContains: []string{
				"f1.go",
				"f2.go",
				"f3.go",
				"and 3 more modified",
			},
			wantNot: []string{"f4.go", "f5.go", "f6.go"},
		},
		{
			name:    "empty list",
			label:   "deleted",
			files:   []string{},
			maxShow: 5,
			wantNot: []string{"deleted"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			displayFileList(&buf, tt.label, tt.files, tt.maxShow)
			output := buf.String()

			for _, want := range tt.wantContains {
				if !bytes.Contains([]byte(output), []byte(want)) {
					t.Errorf("output missing %q, got:\n%s", want, output)
				}
			}

			for _, notWant := range tt.wantNot {
				if bytes.Contains([]byte(output), []byte(notWant)) {
					t.Errorf("output should not contain %q, got:\n%s", notWant, output)
				}
			}
		})
	}
}
