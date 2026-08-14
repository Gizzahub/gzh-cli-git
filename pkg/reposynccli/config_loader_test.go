// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package reposynccli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileSpecLoader_Load_NameExtractedFromURL(t *testing.T) {
	tests := []struct {
		name         string
		yaml         string
		wantRepos    int
		wantNames    []string
		wantPaths    []string
		wantErr      bool
		wantErrMatch string
	}{
		{
			name: "name omitted - extract from HTTPS URL",
			yaml: `
repositories:
  - url: https://github.com/test/my-repo.git
`,
			wantRepos: 1,
			wantNames: []string{"my-repo"},
			wantPaths: []string{"my-repo"},
			wantErr:   false,
		},
		{
			name: "name omitted - extract from SSH URL",
			yaml: `
repositories:
  - url: git@github.com:test/another-repo.git
`,
			wantRepos: 1,
			wantNames: []string{"another-repo"},
			wantPaths: []string{"another-repo"},
			wantErr:   false,
		},
		{
			name: "name omitted with custom path",
			yaml: `
repositories:
  - url: https://github.com/test/my-repo.git
    path: ./custom/location
`,
			wantRepos: 1,
			wantNames: []string{"my-repo"},
			wantPaths: []string{"custom/location"},
			wantErr:   false,
		},
		{
			name: "name provided - use explicit name",
			yaml: `
repositories:
  - name: custom-name
    url: https://github.com/test/original-name.git
`,
			wantRepos: 1,
			wantNames: []string{"custom-name"},
			wantPaths: []string{"custom-name"},
			wantErr:   false,
		},
		{
			name: "mixed - some with name, some without",
			yaml: `
repositories:
  - url: https://github.com/test/auto-extracted.git
  - name: explicit-name
    url: https://github.com/test/other-repo.git
`,
			wantRepos: 2,
			wantNames: []string{"auto-extracted", "explicit-name"},
			wantPaths: []string{"auto-extracted", "explicit-name"},
			wantErr:   false,
		},
		{
			name: "missing URL - error",
			yaml: `
repositories:
  - name: no-url-repo
`,
			wantErr:      true,
			wantErrMatch: "missing URL",
		},
		{
			name: "SSH URL with port",
			yaml: `
repositories:
  - url: ssh://git@gitlab.example.com:2224/group/project-name.git
`,
			wantRepos: 1,
			wantNames: []string{"project-name"},
			wantPaths: []string{"project-name"},
			wantErr:   false,
		},
		{
			name: "multiple repos without names",
			yaml: `
strategy: pull
parallel: 4
repositories:
  - url: https://github.com/discourse/discourse_docker.git
  - url: https://github.com/discourse/discourse.git
    path: discourse_app
`,
			wantRepos: 2,
			wantNames: []string{"discourse_docker", "discourse"},
			wantPaths: []string{"discourse_docker", "discourse_app"},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "test-config.yaml")

			if err := os.WriteFile(configPath, []byte(tt.yaml), 0o644); err != nil {
				t.Fatalf("failed to write test config: %v", err)
			}

			loader := FileSpecLoader{}
			result, err := loader.Load(context.Background(), configPath)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.wantErrMatch != "" && !strings.Contains(err.Error(), tt.wantErrMatch) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrMatch)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result.Plan.Input.Repos) != tt.wantRepos {
				t.Errorf("got %d repos, want %d", len(result.Plan.Input.Repos), tt.wantRepos)
				return
			}

			for i, wantName := range tt.wantNames {
				if result.Plan.Input.Repos[i].Name != wantName {
					t.Errorf("repo[%d].Name = %q, want %q",
						i, result.Plan.Input.Repos[i].Name, wantName)
				}
			}

			for i, wantPath := range tt.wantPaths {
				if result.Plan.Input.Repos[i].TargetPath != wantPath {
					t.Errorf("repo[%d].TargetPath = %q, want %q",
						i, result.Plan.Input.Repos[i].TargetPath, wantPath)
				}
			}
		})
	}
}

func TestScanGitReposRejectsOutsideGitSymlink(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), ".git")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}

	repo := filepath.Join(workspace, "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, ".git")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	repos, err := scanGitRepos(workspace)
	if err != nil {
		t.Fatalf("scanGitRepos: %v", err)
	}
	if len(repos) != 0 {
		t.Fatalf("scanGitRepos found %d repository through outside .git symlink, want 0: %+v", len(repos), repos)
	}
}

func TestFileSpecLoader_Load_BasicConfig(t *testing.T) {
	yaml := `
strategy: reset
parallel: 4
maxRetries: 3
repositories:
  - name: test-repo
    url: https://github.com/test/repo.git
    path: ./repos/test-repo
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	if err := os.WriteFile(configPath, []byte(yaml), 0o644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	loader := FileSpecLoader{}
	result, err := loader.Load(context.Background(), configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Plan.Input.Repos) != 1 {
		t.Errorf("got %d repos, want 1", len(result.Plan.Input.Repos))
	}

	if result.Run.Parallel != 4 {
		t.Errorf("parallel = %d, want 4", result.Run.Parallel)
	}
}

func TestFileSpecLoader_Load_EmptyConfig(t *testing.T) {
	yaml := `
repositories: []
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	if err := os.WriteFile(configPath, []byte(yaml), 0o644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	loader := FileSpecLoader{}
	_, err := loader.Load(context.Background(), configPath)
	if err == nil {
		t.Error("expected error for empty repositories")
	}
}

func TestFileSpecLoader_Load_FileNotFound(t *testing.T) {
	loader := FileSpecLoader{}
	_, err := loader.Load(context.Background(), "/nonexistent/path/config.yaml")

	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestFileSpecLoader_Load_InvalidYAML(t *testing.T) {
	yaml := `strategy: [invalid`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	if err := os.WriteFile(configPath, []byte(yaml), 0o644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	loader := FileSpecLoader{}
	_, err := loader.Load(context.Background(), configPath)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}
