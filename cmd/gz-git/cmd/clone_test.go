package cmd

import (
	"context"
	"os"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		// HTTPS URLs
		{
			name: "HTTPS URL with .git suffix",
			url:  "https://github.com/user/repo.git",
			want: "repo",
		},
		{
			name: "HTTPS URL without .git suffix",
			url:  "https://github.com/user/repo",
			want: "repo",
		},
		{
			name: "HTTPS URL with organization",
			url:  "https://github.com/organization/project-name.git",
			want: "project-name",
		},

		// SSH URLs
		{
			name: "SSH URL with .git suffix",
			url:  "git@github.com:user/repo.git",
			want: "repo",
		},
		{
			name: "SSH URL without .git suffix",
			url:  "git@github.com:user/repo",
			want: "repo",
		},
		{
			name: "SSH URL with organization",
			url:  "git@github.com:organization/my-project.git",
			want: "my-project",
		},
		{
			name: "SSH URL with GitLab",
			url:  "git@gitlab.com:group/subgroup/repo.git",
			want: "repo",
		},

		// Git protocol URLs
		{
			name: "Git protocol URL",
			url:  "git://github.com/user/repo.git",
			want: "repo",
		},

		// File paths
		{
			name: "Unix file path",
			url:  "/home/user/projects/my-repo",
			want: "my-repo",
		},
		{
			name: "Unix file path with .git suffix",
			url:  "/path/to/repo.git",
			want: "repo",
		},
		// Note: Windows paths are not valid Git URLs
		// The utility function ExtractRepoNameFromURL is designed for Git URLs only

		// Edge cases
		{
			name: "Empty URL returns default",
			url:  "",
			want: "repository",
		},
		{
			name: "Only .git suffix",
			url:  ".git",
			want: "repository",
		},
		{
			name: "Simple name",
			url:  "repo",
			want: "repo",
		},
		{
			name: "URL with trailing slash should still work",
			url:  "https://github.com/user/repo.git",
			want: "repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := repository.ExtractRepoNameFromURL(tt.url)
			if got == "" {
				got = "repository"
			}
			if got != tt.want {
				t.Errorf("ExtractRepoNameFromURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestExtractRepoName_SSHVariants(t *testing.T) {
	// Specific test cases for SSH URL parsing
	tests := []struct {
		url  string
		want string
	}{
		// Standard SSH format
		{"git@github.com:user/repo.git", "repo"},
		{"git@github.com:user/repo", "repo"},

		// Custom SSH hosts
		{"git@myserver.com:projects/repo.git", "repo"},
		{"git@192.168.1.1:user/repo.git", "repo"},

		// Nested paths
		{"git@gitlab.com:group/subgroup/repo.git", "repo"},
		{"git@gitlab.com:a/b/c/deep-repo.git", "deep-repo"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got, _ := repository.ExtractRepoNameFromURL(tt.url)
			if got == "" {
				got = "repository"
			}
			if got != tt.want {
				t.Errorf("ExtractRepoNameFromURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// ============================================================================
// YAML Config Tests
// ============================================================================

func TestParseCloneConfig_ValidYAML(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		wantErr  bool
		validate func(*testing.T, *CloneConfig)
	}{
		{
			name: "minimal config",
			yaml: `
repositories:
  - url: https://github.com/user/repo.git
`,
			wantErr: false,
			validate: func(t *testing.T, cfg *CloneConfig) {
				t.Helper()
				if len(cfg.Repositories) != 1 {
					t.Errorf("expected 1 repository, got %d", len(cfg.Repositories))
				}
				if cfg.Repositories[0].URL != "https://github.com/user/repo.git" {
					t.Errorf("unexpected URL: %s", cfg.Repositories[0].URL)
				}
			},
		},
		{
			name: "full config with custom names",
			yaml: `
target: ~/projects
parallel: 5
structure: flat
strategy: pull
repositories:
  - url: https://github.com/user/repo1.git
    name: custom-name-1
    branch: develop
    depth: 1
  - url: https://github.com/user/repo2.git
    name: custom-name-2
`,
			wantErr: false,
			validate: func(t *testing.T, cfg *CloneConfig) {
				t.Helper()
				if cfg.Target != "~/projects" {
					t.Errorf("expected target ~/projects, got %s", cfg.Target)
				}
				if cfg.Parallel != 5 {
					t.Errorf("expected parallel 5, got %d", cfg.Parallel)
				}
				if cfg.Structure != "flat" {
					t.Errorf("expected structure flat, got %s", cfg.Structure)
				}
				if cfg.Strategy != "pull" {
					t.Errorf("expected strategy pull, got %s", cfg.Strategy)
				}
				if len(cfg.Repositories) != 2 {
					t.Fatalf("expected 2 repositories, got %d", len(cfg.Repositories))
				}
				if cfg.Repositories[0].Name != "custom-name-1" {
					t.Errorf("expected name custom-name-1, got %s", cfg.Repositories[0].Name)
				}
				if cfg.Repositories[0].Branch != "develop" {
					t.Errorf("expected branch develop, got %s", cfg.Repositories[0].Branch)
				}
				if cfg.Repositories[0].Depth != 1 {
					t.Errorf("expected depth 1, got %d", cfg.Repositories[0].Depth)
				}
			},
		},
		{
			name: "user structure",
			yaml: `
structure: user
repositories:
  - url: https://github.com/user/repo.git
`,
			wantErr: false,
			validate: func(t *testing.T, cfg *CloneConfig) {
				t.Helper()
				if cfg.Structure != "user" {
					t.Errorf("expected structure user, got %s", cfg.Structure)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file with YAML content
			tmpfile, err := os.CreateTemp("", "clone-config-*.yaml")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpfile.Name())

			if _, err := tmpfile.WriteString(tt.yaml); err != nil {
				t.Fatal(err)
			}
			tmpfile.Close()

			// Parse config
			cfg, err := parseCloneConfig(tmpfile.Name(), false)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCloneConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, cfg)
			}
		})
	}
}

func TestParseCloneConfig_InvalidYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "invalid YAML syntax",
			yaml: `
repositories:
  - url: https://github.com/user/repo.git
    invalid syntax here
`,
			wantErr: "parse config",
		},
		{
			name: "no repositories",
			yaml: `
target: ~/projects
`,
			wantErr: "no repositories defined",
		},
		{
			name: "missing URL",
			yaml: `
repositories:
  - name: repo-name
`,
			wantErr: "missing URL",
		},
		{
			name: "invalid structure",
			yaml: `
structure: invalid-value
repositories:
  - url: https://github.com/user/repo.git
`,
			wantErr: "invalid structure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpfile, err := os.CreateTemp("", "clone-config-*.yaml")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpfile.Name())

			if _, err := tmpfile.WriteString(tt.yaml); err != nil {
				t.Fatal(err)
			}
			tmpfile.Close()

			_, err = parseCloneConfig(tmpfile.Name(), false)
			if err == nil {
				t.Errorf("parseCloneConfig() expected error containing %q, got nil", tt.wantErr)
				return
			}

			if !containsString(err.Error(), tt.wantErr) {
				t.Errorf("parseCloneConfig() error = %q, want error containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateCloneConfig_DuplicateNames(t *testing.T) {
	config := &CloneConfig{
		Repositories: []CloneRepoSpec{
			{
				URL:  "https://github.com/user/repo1.git",
				Name: "my-repo",
			},
			{
				URL:  "https://github.com/user/repo2.git",
				Name: "my-repo", // Duplicate!
			},
		},
	}

	err := validateCloneConfig(config)
	if err == nil {
		t.Error("validateCloneConfig() expected error for duplicate names, got nil")
	}

	if !containsString(err.Error(), "duplicate path") {
		t.Errorf("validateCloneConfig() error = %q, want error containing 'duplicate path'", err.Error())
	}
}

func TestValidateCloneConfig_DuplicateExtractedNames(t *testing.T) {
	// Two different URLs that extract to the same repo name
	config := &CloneConfig{
		Repositories: []CloneRepoSpec{
			{
				URL: "https://github.com/user1/repo.git",
				// Name not specified, will extract "repo"
			},
			{
				URL: "https://github.com/user2/repo.git",
				// Name not specified, will also extract "repo"
			},
		},
	}

	err := validateCloneConfig(config)
	if err == nil {
		t.Error("validateCloneConfig() expected error for duplicate extracted names, got nil")
	}

	if !containsString(err.Error(), "duplicate path") {
		t.Errorf("validateCloneConfig() error = %q, want error containing 'duplicate path'", err.Error())
	}
}

func TestParseCloneConfig_Stdin(t *testing.T) {
	yaml := `
repositories:
  - url: https://github.com/user/repo.git
    name: test-repo
`

	// Save current stdin
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	// Create pipe for stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r

	// Write YAML to stdin
	go func() {
		defer w.Close()
		w.WriteString(yaml)
	}()

	// Parse from stdin
	cfg, err := parseCloneConfig("", true)
	if err != nil {
		t.Errorf("parseCloneConfig() from stdin error = %v", err)
		return
	}

	if len(cfg.Repositories) != 1 {
		t.Errorf("expected 1 repository, got %d", len(cfg.Repositories))
	}
	if cfg.Repositories[0].Name != "test-repo" {
		t.Errorf("expected name test-repo, got %s", cfg.Repositories[0].Name)
	}
}

// Helper function for string contains check.
func containsString(s, substr string) bool {
	return s != "" && substr != "" && (s == substr || len(s) >= len(substr) && s[:len(substr)] == substr || stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ============================================================================
// Strategy Tests
// ============================================================================

func TestResolveCloneStrategy(t *testing.T) {
	tests := []struct {
		name         string
		cliStrategy  string
		yamlStrategy string
		want         repository.UpdateStrategy
	}{
		{
			name:         "defaults to skip",
			cliStrategy:  "",
			yamlStrategy: "",
			want:         repository.StrategySkip,
		},
		{
			name:         "CLI strategy takes precedence",
			cliStrategy:  "reset",
			yamlStrategy: "pull",
			want:         repository.StrategyReset,
		},
		{
			name:         "YAML strategy when no CLI",
			cliStrategy:  "",
			yamlStrategy: "rebase",
			want:         repository.StrategyRebase,
		},
		{
			name:         "CLI skip overrides YAML",
			cliStrategy:  "skip",
			yamlStrategy: "pull",
			want:         repository.StrategySkip,
		},
		{
			name:         "YAML fetch",
			cliStrategy:  "",
			yamlStrategy: "fetch",
			want:         repository.StrategyFetch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveCloneStrategy(tt.cliStrategy, tt.yamlStrategy)
			if got != tt.want {
				t.Errorf("resolveCloneStrategy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateCloneConfig_Strategy(t *testing.T) {
	tests := []struct {
		name     string
		strategy string
		wantErr  bool
	}{
		{"valid skip", "skip", false},
		{"valid pull", "pull", false},
		{"valid reset", "reset", false},
		{"valid rebase", "rebase", false},
		{"valid fetch", "fetch", false},
		{"empty is valid", "", false},
		{"invalid strategy", "invalid", true},
		{"invalid merge", "merge", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &CloneConfig{
				Strategy: tt.strategy,
				Repositories: []CloneRepoSpec{
					{URL: "https://github.com/user/repo.git"},
				},
			}
			err := validateCloneConfig(config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCloneConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseCloneConfig_WithStrategy(t *testing.T) {
	yaml := `
strategy: reset
repositories:
  - url: https://github.com/user/repo.git
`
	tmpfile, err := os.CreateTemp("", "clone-config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.WriteString(yaml); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	cfg, err := parseCloneConfig(tmpfile.Name(), false)
	if err != nil {
		t.Errorf("parseCloneConfig() error = %v", err)
		return
	}

	if cfg.Strategy != "reset" {
		t.Errorf("expected strategy reset, got %s", cfg.Strategy)
	}
}

// ============================================================================
// Grouped Format Tests
// ============================================================================

func TestParseCloneConfig_GroupedFormat(t *testing.T) {
	yaml := `
parallel: 8
strategy: pull

root:
  target: "."
  repositories:
    - url: https://github.com/discourse/discourse_docker.git
    - url: https://github.com/discourse/discourse.git
      name: discourse_app
      branch: stable

plugins:
  target: all-the-plugins
  branch: develop
  repositories:
    - url: https://github.com/discourse/docker_manager.git
    - url: https://github.com/discourse/discourse-akismet.git
`

	tmpfile, err := os.CreateTemp("", "clone-grouped-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.WriteString(yaml); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	cfg, err := parseCloneConfig(tmpfile.Name(), false)
	if err != nil {
		t.Fatalf("parseCloneConfig() error = %v", err)
	}

	// Should detect as grouped format
	if len(cfg.Repositories) > 0 {
		t.Error("expected no flat repositories in grouped format")
	}

	if len(cfg.Groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(cfg.Groups))
	}

	// Check global settings
	if cfg.Parallel != 8 {
		t.Errorf("expected parallel=8, got %d", cfg.Parallel)
	}
	if cfg.Strategy != "pull" {
		t.Errorf("expected strategy=pull, got %s", cfg.Strategy)
	}

	// Check root group
	root, ok := cfg.Groups["root"]
	if !ok {
		t.Fatal("missing 'root' group")
	}
	if root.Target != "." {
		t.Errorf("root.Target = %q, want '.'", root.Target)
	}
	if len(root.Repositories) != 2 {
		t.Errorf("root.Repositories count = %d, want 2", len(root.Repositories))
	}
	if root.Repositories[1].Name != "discourse_app" {
		t.Errorf("expected second repo name 'discourse_app', got %q", root.Repositories[1].Name)
	}
	if root.Repositories[1].Branch != "stable" {
		t.Errorf("expected second repo branch 'stable', got %q", root.Repositories[1].Branch)
	}

	// Check plugins group
	plugins, ok := cfg.Groups["plugins"]
	if !ok {
		t.Fatal("missing 'plugins' group")
	}
	if plugins.Target != "all-the-plugins" {
		t.Errorf("plugins.Target = %q, want 'all-the-plugins'", plugins.Target)
	}
	if plugins.Branch != "develop" {
		t.Errorf("plugins.Branch = %q, want 'develop'", plugins.Branch)
	}
	if len(plugins.Repositories) != 2 {
		t.Errorf("plugins.Repositories count = %d, want 2", len(plugins.Repositories))
	}
}

func TestValidateCloneConfig_GroupedFormat_MissingTarget(t *testing.T) {
	cfg := &CloneConfig{
		Groups: map[string]*CloneGroup{
			"test": {
				// Missing Target
				Repositories: []CloneRepoSpec{
					{URL: "https://github.com/user/repo.git"},
				},
			},
		},
	}

	err := validateCloneConfig(cfg)
	if err == nil {
		t.Error("expected error for missing target")
	}
	if !containsString(err.Error(), "missing target") {
		t.Errorf("error = %q, want containing 'missing target'", err.Error())
	}
}

func TestValidateCloneConfig_GroupedFormat_EmptyRepositories(t *testing.T) {
	cfg := &CloneConfig{
		Groups: map[string]*CloneGroup{
			"test": {
				Target:       "test-dir",
				Repositories: []CloneRepoSpec{},
			},
		},
	}

	err := validateCloneConfig(cfg)
	if err == nil {
		t.Error("expected error for empty repositories")
	}
	if !containsString(err.Error(), "no repositories") {
		t.Errorf("error = %q, want containing 'no repositories'", err.Error())
	}
}

func TestParseCloneConfig_DetectsFlatFormat(t *testing.T) {
	yaml := `
target: /tmp/repos
parallel: 4
repositories:
  - url: https://github.com/user/repo1.git
  - url: https://github.com/user/repo2.git
`

	tmpfile, err := os.CreateTemp("", "clone-flat-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.WriteString(yaml); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	cfg, err := parseCloneConfig(tmpfile.Name(), false)
	if err != nil {
		t.Fatalf("parseCloneConfig() error = %v", err)
	}

	// Should detect as flat format
	if len(cfg.Repositories) != 2 {
		t.Errorf("expected 2 flat repositories, got %d", len(cfg.Repositories))
	}
	if len(cfg.Groups) != 0 {
		t.Errorf("expected 0 groups in flat format, got %d", len(cfg.Groups))
	}
	if cfg.Target != "/tmp/repos" {
		t.Errorf("expected target '/tmp/repos', got %q", cfg.Target)
	}
}

// ============================================================================
// Clone Hooks Tests
// ============================================================================

func TestParseCloneHooks(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]any
		want *CloneHooks
	}{
		{
			name: "before and after hooks",
			raw: map[string]any{
				"before": []any{"echo before"},
				"after":  []any{"echo after", "make setup"},
			},
			want: &CloneHooks{
				Before: []string{"echo before"},
				After:  []string{"echo after", "make setup"},
			},
		},
		{
			name: "only after hooks",
			raw: map[string]any{
				"after": []any{"./reset-all-repos"},
			},
			want: &CloneHooks{
				After: []string{"./reset-all-repos"},
			},
		},
		{
			name: "only before hooks",
			raw: map[string]any{
				"before": []any{"mkdir -p backup"},
			},
			want: &CloneHooks{
				Before: []string{"mkdir -p backup"},
			},
		},
		{
			name: "empty hooks",
			raw:  map[string]any{},
			want: nil,
		},
		{
			name: "empty arrays",
			raw: map[string]any{
				"before": []any{},
				"after":  []any{},
			},
			want: nil,
		},
		{
			name: "nil raw map",
			raw:  nil,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.raw == nil {
				// Handle nil case
				return
			}
			got := parseCloneHooks(tt.raw)
			if tt.want == nil {
				if got != nil {
					t.Errorf("parseCloneHooks() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Errorf("parseCloneHooks() = nil, want %v", tt.want)
				return
			}
			if len(got.Before) != len(tt.want.Before) {
				t.Errorf("Before hooks: got %d, want %d", len(got.Before), len(tt.want.Before))
			}
			if len(got.After) != len(tt.want.After) {
				t.Errorf("After hooks: got %d, want %d", len(got.After), len(tt.want.After))
			}
		})
	}
}

func TestMergeHooks(t *testing.T) {
	tests := []struct {
		name       string
		groupHooks *CloneHooks
		repoHooks  *CloneHooks
		wantBefore int
		wantAfter  int
	}{
		{
			name:       "both nil",
			groupHooks: nil,
			repoHooks:  nil,
			wantBefore: 0,
			wantAfter:  0,
		},
		{
			name: "only group hooks",
			groupHooks: &CloneHooks{
				Before: []string{"echo group-before"},
				After:  []string{"echo group-after"},
			},
			repoHooks:  nil,
			wantBefore: 1,
			wantAfter:  1,
		},
		{
			name:       "only repo hooks",
			groupHooks: nil,
			repoHooks: &CloneHooks{
				Before: []string{"echo repo-before"},
				After:  []string{"echo repo-after"},
			},
			wantBefore: 1,
			wantAfter:  1,
		},
		{
			name: "both group and repo hooks",
			groupHooks: &CloneHooks{
				Before: []string{"echo group-before"},
				After:  []string{"echo group-after"},
			},
			repoHooks: &CloneHooks{
				Before: []string{"echo repo-before"},
				After:  []string{"echo repo-after"},
			},
			wantBefore: 2,
			wantAfter:  2,
		},
		{
			name: "multiple hooks merged",
			groupHooks: &CloneHooks{
				Before: []string{"cmd1", "cmd2"},
				After:  []string{"cmd3"},
			},
			repoHooks: &CloneHooks{
				After: []string{"cmd4", "cmd5"},
			},
			wantBefore: 2,
			wantAfter:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeHooks(tt.groupHooks, tt.repoHooks)
			if tt.wantBefore == 0 && tt.wantAfter == 0 {
				if got != nil {
					t.Errorf("mergeHooks() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("mergeHooks() = nil, want non-nil")
			}
			if len(got.Before) != tt.wantBefore {
				t.Errorf("Before count = %d, want %d", len(got.Before), tt.wantBefore)
			}
			if len(got.After) != tt.wantAfter {
				t.Errorf("After count = %d, want %d", len(got.After), tt.wantAfter)
			}
		})
	}
}

func TestParseCloneConfig_WithHooks(t *testing.T) {
	yaml := `
core:
  target: "."
  hooks:
    after:
      - echo "Group clone complete"
  repositories:
    - url: https://github.com/discourse/all-the-plugins.git
      hooks:
        after:
          - ./reset-all-repos

plugins:
  target: ./plugins
  repositories:
    - url: https://github.com/user/plugin1.git
    - url: https://github.com/user/plugin2.git
      hooks:
        before:
          - echo "Preparing plugin2"
        after:
          - make setup
`

	tmpfile, err := os.CreateTemp("", "clone-hooks-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.WriteString(yaml); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	cfg, err := parseCloneConfig(tmpfile.Name(), false)
	if err != nil {
		t.Fatalf("parseCloneConfig() error = %v", err)
	}

	// Check core group hooks
	core, ok := cfg.Groups["core"]
	if !ok {
		t.Fatal("missing 'core' group")
	}
	if core.Hooks == nil {
		t.Fatal("core.Hooks is nil")
	}
	if len(core.Hooks.After) != 1 {
		t.Errorf("core.Hooks.After count = %d, want 1", len(core.Hooks.After))
	}
	if core.Hooks.After[0] != `echo "Group clone complete"` {
		t.Errorf("core.Hooks.After[0] = %q, want %q", core.Hooks.After[0], `echo "Group clone complete"`)
	}

	// Check repo-level hooks
	if len(core.Repositories) < 1 {
		t.Fatal("core.Repositories is empty")
	}
	if core.Repositories[0].Hooks == nil {
		t.Fatal("core.Repositories[0].Hooks is nil")
	}
	if len(core.Repositories[0].Hooks.After) != 1 {
		t.Errorf("core.Repositories[0].Hooks.After count = %d, want 1", len(core.Repositories[0].Hooks.After))
	}
	if core.Repositories[0].Hooks.After[0] != "./reset-all-repos" {
		t.Errorf("repo hook = %q, want './reset-all-repos'", core.Repositories[0].Hooks.After[0])
	}

	// Check plugins group - repo with both before and after hooks
	plugins, ok := cfg.Groups["plugins"]
	if !ok {
		t.Fatal("missing 'plugins' group")
	}
	if len(plugins.Repositories) < 2 {
		t.Fatal("plugins.Repositories should have 2 repos")
	}
	// First plugin has no hooks
	if plugins.Repositories[0].Hooks != nil {
		t.Error("plugin1 should have no hooks")
	}
	// Second plugin has hooks
	if plugins.Repositories[1].Hooks == nil {
		t.Fatal("plugin2.Hooks is nil")
	}
	if len(plugins.Repositories[1].Hooks.Before) != 1 {
		t.Errorf("plugin2.Hooks.Before count = %d, want 1", len(plugins.Repositories[1].Hooks.Before))
	}
	if len(plugins.Repositories[1].Hooks.After) != 1 {
		t.Errorf("plugin2.Hooks.After count = %d, want 1", len(plugins.Repositories[1].Hooks.After))
	}
}

func TestExecuteHooks_Success(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	ctx := context.Background()

	hooks := []string{
		"echo hello",
		"ls -la",
	}

	err := executeHooks(ctx, hooks, tmpDir, nil)
	if err != nil {
		t.Errorf("executeHooks() error = %v", err)
	}
}

func TestExecuteHooks_Failure(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	hooks := []string{
		"nonexistent-command-that-should-fail",
	}

	err := executeHooks(ctx, hooks, tmpDir, nil)
	if err == nil {
		t.Error("executeHooks() expected error for nonexistent command")
	}
}

func TestExecuteHooks_InvalidWorkDir(t *testing.T) {
	ctx := context.Background()
	hooks := []string{"echo test"}

	err := executeHooks(ctx, hooks, "/nonexistent/path/that/does/not/exist", nil)
	if err == nil {
		t.Error("executeHooks() expected error for invalid working directory")
	}
}

func TestExecuteHooks_EmptyHooks(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	// Empty hooks should succeed
	err := executeHooks(ctx, []string{}, tmpDir, nil)
	if err != nil {
		t.Errorf("executeHooks() with empty hooks error = %v", err)
	}

	// Nil hooks should also succeed
	err = executeHooks(ctx, nil, tmpDir, nil)
	if err != nil {
		t.Errorf("executeHooks() with nil hooks error = %v", err)
	}
}
