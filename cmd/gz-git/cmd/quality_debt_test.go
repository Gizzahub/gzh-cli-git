package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

func TestCommitMessageGeneratorUsesPathPrecedence(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "nested", "repo")
	generator := customCommitMessageGenerator(root, map[string]string{
		"nested/repo": "relative",
		"repo":        "base",
		repoPath:      "full",
	})

	got, err := generator(context.Background(), repoPath, nil)
	if err != nil {
		t.Fatalf("generator returned error: %v", err)
	}
	if got != "relative" {
		t.Fatalf("message = %q, want relative-path message", got)
	}

	got, err = customCommitMessageGenerator(root, map[string]string{"repo": "base"})(context.Background(), repoPath, nil)
	if err != nil || got != "base" {
		t.Fatalf("base-name fallback = %q, err=%v", got, err)
	}
}

func TestEditMessagesInEditorEmptyFileCancels(t *testing.T) {
	editor := filepath.Join(t.TempDir(), "empty-editor")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\n: > \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	previous := os.Getenv("EDITOR")
	if err := os.Setenv("EDITOR", editor); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("EDITOR", previous) })

	got, err := editMessagesInEditor(&repository.BulkCommitResult{
		Repositories: []repository.RepositoryCommitResult{{
			RelativePath:     "repo",
			Status:           "dirty",
			SuggestedMessage: "suggested",
		}},
	})
	if err != nil {
		t.Fatalf("editMessagesInEditor returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("empty editor result = %#v, want nil cancellation", got)
	}
}

func TestCloneSingleRepositoryResultShapes(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(existing, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	client := repository.NewClient()

	tests := []struct {
		name     string
		path     string
		strategy repository.UpdateStrategy
		dryRun   bool
		want     string
		wantErr  string
	}{
		{name: "dry run new", path: "new", dryRun: true, want: "would-clone"},
		{name: "dry run existing", path: "repo", dryRun: true, want: "skipped"},
		{name: "dry run update", path: "repo", strategy: repository.StrategyFetch, dryRun: true, want: "would-update"},
		{name: "existing default", path: "repo", want: "skipped"},
		{name: "update validation error", path: "repo", strategy: repository.UpdateStrategy("invalid"), want: "error", wantErr: "update failed (strategy: invalid)"},
		{name: "before hook error", path: "new-before", want: "error", wantErr: "before hook:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := CloneRepoSpec{URL: "file:///source", Hooks: nil}
			if strings.Contains(tt.name, "before hook") {
				spec.Hooks = &CloneHooks{Before: []string{"command-that-does-not-exist"}}
			}
			result := cloneSingleRepository(context.Background(), client, spec, filepath.Base(tt.path), repository.BulkCloneOptions{
				Directory: root,
				Strategy:  tt.strategy,
				DryRun:    tt.dryRun,
			}, nil)
			if result.Status != tt.want {
				t.Fatalf("status = %q, want %q (error=%v)", result.Status, tt.want, result.Error)
			}
			if tt.wantErr == "" && result.Error != nil {
				t.Fatalf("unexpected error: %v", result.Error)
			}
			if tt.wantErr != "" && (result.Error == nil || !strings.Contains(result.Error.Error(), tt.wantErr)) {
				t.Fatalf("error = %v, want substring %q", result.Error, tt.wantErr)
			}
		})
	}
}

func TestCloneSingleRepositoryAfterHookErrorShape(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "init")
	runGit(t, source, "config", "user.email", "test@example.com")
	runGit(t, source, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(source, "README"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "add", "README")
	runGit(t, source, "commit", "-m", "init")

	result := cloneSingleRepository(context.Background(), repository.NewClient(), CloneRepoSpec{
		URL:   source,
		Hooks: &CloneHooks{After: []string{"command-that-does-not-exist"}},
	}, "clone", repository.BulkCloneOptions{Directory: root}, nil)
	if result.Status != "error" {
		t.Fatalf("status = %q, want error", result.Status)
	}
	if result.Error == nil || !strings.Contains(result.Error.Error(), "after hook:") {
		t.Fatalf("error = %v, want after-hook error", result.Error)
	}
	if _, err := os.Stat(filepath.Join(root, "clone", ".git")); err != nil {
		t.Fatalf("clone should complete before after hook: %v", err)
	}
}

func TestDisplayInfoDetailedSortsAndLimits(t *testing.T) {
	previousLimit, previousVerbose := itemLimit, verbose
	t.Cleanup(func() { itemLimit, verbose = previousLimit, previousVerbose })
	itemLimit = 2
	verbose = false
	result := &repository.BulkStatusResult{Repositories: []repository.RepositoryStatusResult{
		{
			Path:          "/tmp/z-repo",
			RelativePath:  "z-repo",
			Status:        "clean",
			Remotes:       map[string]string{"z": "z-url", "a": "a-url", "m": "m-url"},
			LocalBranches: []string{"z", "a", "m"},
		},
		{Path: "/tmp/a-repo", RelativePath: "a-repo", Status: "dirty"},
	}}
	out := captureStdout(t, func() { displayInfoResultsDetailed(result, nil) })

	if strings.Index(out, "📦 z-repo") > strings.Index(out, "📦 a-repo") {
		t.Fatal("repository output order changed")
	}
	for _, expected := range []string{"    - a          a-url", "    - m          m-url", "    ... (1 more)", "Branches (3):   a, m, ... (1 more)"} {
		if !strings.Contains(out, expected) {
			t.Errorf("output missing %q:\n%s", expected, out)
		}
	}
	if strings.Contains(out, "    - z          z-url") || strings.Contains(out, "Branches (3):   a, m, z") {
		t.Error("item limit was not applied")
	}
}
