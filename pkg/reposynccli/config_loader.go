// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package reposynccli

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/gizzahub/gzh-cli-gitforge/internal/safefs"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/reposync"
)

// ConfigData contains plan + run inputs loaded from a config file.
type ConfigData struct {
	Plan reposync.PlanRequest
	Run  reposync.RunOptions
}

// SpecLoader loads repository specs from a source (e.g., YAML file).
type SpecLoader interface {
	Load(ctx context.Context, path string) (ConfigData, error)
}

// FileSpecLoader loads configuration from a YAML file on disk.
type FileSpecLoader struct {
	// Optional defaults if the file omits values.
	DefaultStrategy reposync.Strategy
	DefaultParallel int
	DefaultRetries  int
}

type fileConfig struct {
	// Meta information (optional but recommended)
	Version  int               `yaml:"version,omitempty"`  // schema version (currently 1)
	Kind     config.ConfigKind `yaml:"kind,omitempty"`     // "repositories" or "workspace"
	Metadata *config.Metadata  `yaml:"metadata,omitempty"` // optional descriptive info

	// Parent config reference (for inheritance)
	Parent  string `yaml:"parent,omitempty"`  // path to parent config file
	Profile string `yaml:"profile,omitempty"` // profile name to use

	// Sync settings
	Strategy             string      `yaml:"strategy"`
	Parallel             int         `yaml:"parallel"`
	MaxRetries           *int        `yaml:"maxRetries"`
	Resume               bool        `yaml:"resume"`
	DryRun               bool        `yaml:"dryRun"`
	CleanupOrphans       bool        `yaml:"cleanupOrphans"`
	StrictBranchCheckout bool        `yaml:"strictBranchCheckout"` // default: false (lenient)
	Roots                []string    `yaml:"roots"`
	Repositories         []repoEntry `yaml:"repositories"`
}

type repoEntry struct {
	Name                 string            `yaml:"name"`
	Description          string            `yaml:"description"` // optional: human-readable description
	Provider             string            `yaml:"provider"`
	URL                  string            `yaml:"url"`
	AdditionalRemotes    map[string]string `yaml:"additionalRemotes"` // Additional git remotes (name: url)
	Path                 string            `yaml:"path"`
	Branch               config.FlexBranch `yaml:"branch"`               // optional: branch to checkout after clone/update
	StrictBranchCheckout *bool             `yaml:"strictBranchCheckout"` // optional: override global setting (nil = use global)
	Strategy             string            `yaml:"strategy"`
	Enabled              *bool             `yaml:"enabled"`       // optional: if false, exclude from sync (default: true)
	AssumePresent        bool              `yaml:"assumePresent"` // if true, skip clone check (assume already exists)
}

type gzhYamlConfig struct {
	Provider     string        `yaml:"provider"`
	SyncMode     gzhYamlMode   `yaml:"sync_mode"` //nolint:tagliatelle // matches existing gzh.yaml format
	Repositories []gzhYamlRepo `yaml:"repositories"`
}

type gzhYamlMode struct {
	CleanupOrphans bool `yaml:"cleanup_orphans"` //nolint:tagliatelle // matches existing gzh.yaml format
}

type gzhYamlRepo struct {
	Name     string `yaml:"name"`
	CloneURL string `yaml:"clone_url"` //nolint:tagliatelle // matches existing gzh.yaml format
}

// workspacesConfig represents the new hierarchical config format with workspaces.
type workspacesConfig struct {
	Parent     string                     `yaml:"parent"`
	Profile    string                     `yaml:"profile"`
	Parallel   int                        `yaml:"parallel"`
	CloneProto string                     `yaml:"cloneProto"`
	SSHPort    int                        `yaml:"sshPort"`
	Workspaces map[string]*workspaceEntry `yaml:"workspaces"`
	Sync       *syncSettings              `yaml:"sync"`
	Profiles   map[string]*profileEntry   `yaml:"profiles"`
}

type workspaceEntry struct {
	Path       string        `yaml:"path"`
	Type       string        `yaml:"type"` // forge, git, config
	Profile    string        `yaml:"profile"`
	Source     *forgeSource  `yaml:"source"`
	Sync       *syncSettings `yaml:"sync"`
	Parallel   int           `yaml:"parallel"`
	CloneProto string        `yaml:"cloneProto"`
	SSHPort    int           `yaml:"sshPort"`
}

type forgeSource struct {
	Provider         string `yaml:"provider"` // gitlab, github, gitea
	Org              string `yaml:"org"`
	BaseURL          string `yaml:"baseURL"` //nolint:tagliatelle // matches existing workspace config format
	Token            string `yaml:"token"`
	IncludeSubgroups bool   `yaml:"includeSubgroups"`
	SubgroupMode     string `yaml:"subgroupMode"`  // flat, nested
	FlatSeparator    string `yaml:"flatSeparator"` // separator for flat mode (default: "-")
}

type syncSettings struct {
	Strategy   string `yaml:"strategy"`
	MaxRetries int    `yaml:"maxRetries"`
}

type profileEntry struct {
	Name             string        `yaml:"name"`
	Provider         string        `yaml:"provider"`
	BaseURL          string        `yaml:"baseURL"` //nolint:tagliatelle // matches existing workspace config format
	Token            string        `yaml:"token"`
	CloneProto       string        `yaml:"cloneProto"`
	SSHPort          int           `yaml:"sshPort"`
	Parallel         int           `yaml:"parallel"`
	IncludeSubgroups bool          `yaml:"includeSubgroups"`
	SubgroupMode     string        `yaml:"subgroupMode"`
	FlatSeparator    string        `yaml:"flatSeparator"` // separator for flat mode (default: "-")
	Sync             *syncSettings `yaml:"sync"`
}

// Load implements SpecLoader.
func (l FileSpecLoader) Load(ctx context.Context, path string) (ConfigData, error) { //nolint:gocognit,gocyclo // multi-format config loading with parent inheritance — complexity is inherent
	if path == "" {
		return ConfigData{}, errors.New("config path is required")
	}

	configPath := cleanPath(path)

	raw, err := os.ReadFile(configPath) // #nosec G304 -- configPath is the explicit workspace config selected by the caller.
	if err != nil {
		return ConfigData{}, fmt.Errorf("read config: %w", err)
	}

	// Determine config kind: explicit kind field > filename > content detection
	kind := detectConfigKind(raw, configPath)

	// Route to appropriate loader based on kind
	if kind == config.KindWorkspace {
		return l.loadWorkspacesConfig(ctx, raw, configPath)
	}

	if isGzhYaml(raw) {
		return l.loadGzhYaml(raw, configPath)
	}

	var cfg fileConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return ConfigData{}, fmt.Errorf("parse config: %w", err)
	}

	// Load and merge parent config if specified
	if cfg.Parent != "" {
		parentPath := resolveParentPath(cfg.Parent, configPath)
		parentCfg, err := l.loadParentConfig(parentPath)
		if err != nil {
			// Log warning but continue - parent might not exist yet
			fmt.Fprintf(os.Stderr, "Warning: failed to load parent config %s: %v\n", parentPath, err)
		} else {
			// Merge: child overrides parent
			cfg = mergeFileConfigs(parentCfg, cfg)
		}
	}

	if len(cfg.Repositories) == 0 {
		return ConfigData{}, errors.New("config has no repositories")
	}

	if cfg.CleanupOrphans && len(cfg.Roots) == 0 {
		return ConfigData{}, errors.New("cleanupOrphans enabled but no roots provided")
	}

	defaultStrategy := l.DefaultStrategy
	if defaultStrategy == "" {
		defaultStrategy = reposync.StrategyReset
	}

	defaultParallel := l.DefaultParallel
	if defaultParallel <= 0 {
		defaultParallel = repository.DefaultLocalParallel
	}

	defaultRetries := l.DefaultRetries
	if defaultRetries <= 0 {
		defaultRetries = 1
	}

	parsedStrategy, err := reposync.ParseStrategy(cfg.Strategy)
	if err != nil {
		return ConfigData{}, err
	}
	if cfg.Strategy == "" {
		parsedStrategy = defaultStrategy
	}

	plan := reposync.PlanRequest{
		Input: reposync.PlanInput{
			Repos: make([]reposync.RepoSpec, 0, len(cfg.Repositories)),
		},
		Options: reposync.PlanOptions{
			DefaultStrategy: parsedStrategy,
			CleanupOrphans:  cfg.CleanupOrphans,
			Roots:           cleanRoots(cfg.Roots),
		},
	}

	seenTargets := make(map[string]struct{}, len(cfg.Repositories))

	for i, repo := range cfg.Repositories {
		// URL is always required
		if repo.URL == "" {
			return ConfigData{}, fmt.Errorf("repository[%d]: missing URL", i)
		}

		// Extract name from URL if not specified
		repoName := repo.Name
		if repoName == "" {
			extracted, err := repository.ExtractRepoNameFromURL(repo.URL)
			if err != nil {
				return ConfigData{}, fmt.Errorf("repository[%d]: cannot extract name from URL %q: %w", i, repo.URL, err)
			}
			repoName = extracted
		}

		// Default path to repo name if not specified
		targetPath := repo.Path
		if targetPath == "" {
			targetPath = repoName
		}

		repoStrategy := parsedStrategy
		if repo.Strategy != "" {
			repoStrategy, err = reposync.ParseStrategy(repo.Strategy)
			if err != nil {
				return ConfigData{}, fmt.Errorf("repository %s: %w", repoName, err)
			}
		}

		targetPath = cleanPath(targetPath)
		if _, exists := seenTargets[targetPath]; exists {
			return ConfigData{}, fmt.Errorf("duplicate path detected: %s", targetPath)
		}
		seenTargets[targetPath] = struct{}{}

		// Determine strictBranchCheckout: use per-repo override if set, otherwise global
		strictBranchCheckout := cfg.StrictBranchCheckout
		if repo.StrictBranchCheckout != nil {
			strictBranchCheckout = *repo.StrictBranchCheckout
		}

		plan.Input.Repos = append(plan.Input.Repos, reposync.RepoSpec{
			Name:                 repoName,
			Description:          repo.Description,
			Provider:             repo.Provider,
			CloneURL:             repo.URL,
			AdditionalRemotes:    repo.AdditionalRemotes,
			TargetPath:           targetPath,
			Branch:               string(repo.Branch),
			StrictBranchCheckout: strictBranchCheckout,
			Strategy:             repoStrategy,
			Enabled:              repo.Enabled,
			AssumePresent:        repo.AssumePresent,
		})
	}

	run := reposync.RunOptions{
		Parallel:   cfg.Parallel,
		MaxRetries: defaultRetries,
		Resume:     cfg.Resume,
		DryRun:     cfg.DryRun,
	}
	if cfg.MaxRetries != nil {
		run.MaxRetries = *cfg.MaxRetries
	}

	if run.Parallel <= 0 {
		run.Parallel = defaultParallel
	}
	if run.MaxRetries < 0 {
		return ConfigData{}, fmt.Errorf("maxRetries must be >= 0 (got %d)", run.MaxRetries)
	}

	return ConfigData{
		Plan: plan,
		Run:  run,
	}, nil
}

// detectConfigKind determines the config type using priority:
// 1. Explicit "kind" field in YAML
// 2. Content detection (workspaces/profiles keys → workspace)
// 3. Default to repositories.
func detectConfigKind(raw []byte, _ string) config.ConfigKind {
	var meta struct {
		Kind config.ConfigKind `yaml:"kind"`
	}
	if err := yaml.Unmarshal(raw, &meta); err == nil && meta.Kind.IsValid() {
		return meta.Kind
	}

	// Content-based detection
	if hasWorkspaceKeys(raw) {
		return config.KindWorkspace
	}

	return config.KindRepositories
}

// hasWorkspaceKeys checks if the YAML content has workspace-specific keys.
func hasWorkspaceKeys(raw []byte) bool {
	var root map[string]any
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return false
	}

	// Check for workspaces key (new hierarchical config)
	if _, ok := root["workspaces"]; ok {
		return true
	}
	// Check for profiles key (inline profiles)
	if _, ok := root["profiles"]; ok {
		return true
	}
	return false
}

func isGzhYaml(raw []byte) bool {
	var root map[string]any
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return false
	}

	if _, ok := root["sync_mode"]; ok {
		return true
	}
	if _, ok := root["organization"]; ok {
		return true
	}
	if _, ok := root["generated_at"]; ok {
		return true
	}

	repos, ok := root["repositories"].([]any)
	if !ok {
		return false
	}
	for _, entry := range repos {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := m["clone_url"]; ok {
			return true
		}
	}
	return false
}

func (l FileSpecLoader) loadGzhYaml(raw []byte, path string) (ConfigData, error) {
	var cfg gzhYamlConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return ConfigData{}, fmt.Errorf("parse gzh.yaml: %w", err)
	}
	if len(cfg.Repositories) == 0 {
		return ConfigData{}, errors.New("gzh.yaml has no repositories")
	}

	defaultStrategy := l.DefaultStrategy
	if defaultStrategy == "" {
		defaultStrategy = reposync.StrategyReset
	}

	defaultParallel := l.DefaultParallel
	if defaultParallel <= 0 {
		defaultParallel = repository.DefaultLocalParallel
	}

	defaultRetries := l.DefaultRetries
	if defaultRetries <= 0 {
		defaultRetries = 1
	}

	root := cleanPath(filepath.Dir(path))

	plan := reposync.PlanRequest{
		Input: reposync.PlanInput{
			Repos: make([]reposync.RepoSpec, 0, len(cfg.Repositories)),
		},
		Options: reposync.PlanOptions{
			DefaultStrategy: defaultStrategy,
			CleanupOrphans:  cfg.SyncMode.CleanupOrphans,
			Roots:           []string{root},
		},
	}

	seenTargets := make(map[string]struct{}, len(cfg.Repositories))
	for _, repo := range cfg.Repositories {
		if repo.Name == "" || repo.CloneURL == "" {
			return ConfigData{}, errors.New("gzh.yaml repository entry is missing required fields (name/clone_url)")
		}

		targetPath := cleanPath(filepath.Join(root, repo.Name))
		if _, exists := seenTargets[targetPath]; exists {
			return ConfigData{}, fmt.Errorf("duplicate path detected: %s", targetPath)
		}
		seenTargets[targetPath] = struct{}{}

		plan.Input.Repos = append(plan.Input.Repos, reposync.RepoSpec{
			Name:       repo.Name,
			Provider:   cfg.Provider,
			CloneURL:   repo.CloneURL,
			TargetPath: targetPath,
			Strategy:   defaultStrategy,
		})
	}

	run := reposync.RunOptions{
		Parallel:   defaultParallel,
		MaxRetries: defaultRetries,
	}

	return ConfigData{
		Plan: plan,
		Run:  run,
	}, nil
}

// loadWorkspacesConfig handles the new hierarchical config format with workspaces.
// Supports three workspace types:
//   - type: forge  → fetch repos from forge API (GitHub/GitLab/Gitea)
//   - type: config → recursively load nested .gz-git.yaml
//   - type: git    → scan directory for git repositories (default)
func (l FileSpecLoader) loadWorkspacesConfig(ctx context.Context, raw []byte, configPath string) (ConfigData, error) { //nolint:gocognit,gocyclo // complex multi-workspace loading logic — extracting helpers would lose clarity
	var cfg workspacesConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return ConfigData{}, fmt.Errorf("parse workspaces config: %w", err)
	}

	if len(cfg.Workspaces) == 0 {
		return ConfigData{}, errors.New("config has no workspaces defined")
	}

	// Load and merge parent config profiles (if parent is specified)
	if err := mergeProfilesIntoConfig(&cfg, configPath); err != nil {
		// Log warning but continue - parent profiles are optional
		fmt.Fprintf(os.Stderr, "Warning: failed to merge parent profiles: %v\n", err)
	}

	// Build profile lookup map (inline profiles + inherited from parent)
	profiles := cfg.Profiles

	// Resolve active profile settings
	var activeProfile *profileEntry
	if cfg.Profile != "" && profiles != nil {
		activeProfile = profiles[cfg.Profile]
	}

	// Default settings
	defaultStrategy := l.DefaultStrategy
	if defaultStrategy == "" {
		defaultStrategy = reposync.StrategyPull
	}
	if cfg.Sync != nil && cfg.Sync.Strategy != "" {
		if parsed, err := reposync.ParseStrategy(cfg.Sync.Strategy); err == nil {
			defaultStrategy = parsed
		}
	}

	defaultParallel := l.DefaultParallel
	if defaultParallel <= 0 {
		defaultParallel = repository.DefaultLocalParallel
	}
	if cfg.Parallel > 0 {
		defaultParallel = cfg.Parallel
	}

	defaultRetries := l.DefaultRetries
	if defaultRetries <= 0 {
		defaultRetries = 1
	}
	if cfg.Sync != nil && cfg.Sync.MaxRetries > 0 {
		defaultRetries = cfg.Sync.MaxRetries
	}

	defaultCloneProto := cfg.CloneProto
	if defaultCloneProto == "" {
		defaultCloneProto = "ssh"
	}

	defaultSSHPort := cfg.SSHPort

	plan := reposync.PlanRequest{
		Input: reposync.PlanInput{
			Repos: make([]reposync.RepoSpec, 0),
		},
		Options: reposync.PlanOptions{
			DefaultStrategy: defaultStrategy,
		},
	}

	var roots []string
	seenTargets := make(map[string]struct{})
	configDir := filepath.Dir(configPath)

	// Process each workspace
	for name, ws := range cfg.Workspaces {
		if ws == nil {
			continue
		}

		// Resolve workspace path (relative to config file location)
		wsPath := ws.Path
		if wsPath == "" {
			wsPath = "."
		}
		if wsPath == "." {
			wsPath = configDir
		} else if !filepath.IsAbs(wsPath) && !strings.HasPrefix(wsPath, "~") {
			wsPath = filepath.Join(configDir, wsPath)
		}
		wsPath = cleanPath(wsPath)
		roots = append(roots, wsPath)

		// Determine workspace strategy
		wsStrategy := defaultStrategy
		if ws.Sync != nil && ws.Sync.Strategy != "" {
			if parsed, err := reposync.ParseStrategy(ws.Sync.Strategy); err == nil {
				wsStrategy = parsed
			}
		}

		// Resolve workspace profile (inherit from parent or use own)
		wsProfile := activeProfile
		if ws.Profile != "" && profiles != nil {
			if p, ok := profiles[ws.Profile]; ok {
				wsProfile = p
			}
		}

		// Determine workspace type
		wsType := ws.Type
		if wsType == "" {
			if ws.Source != nil {
				wsType = "forge"
			} else {
				wsType = "git"
			}
		}

		var repos []reposync.RepoSpec
		var err error

		switch wsType {
		case "forge":
			repos, err = l.loadForgeWorkspace(ctx, ws, wsProfile, wsPath, defaultCloneProto, defaultSSHPort)
			if err != nil {
				// Log warning but continue with other workspaces
				fmt.Fprintf(os.Stderr, "Warning: workspace '%s' forge fetch failed: %v\n", name, err)
				continue
			}

		case "config":
			repos, err = l.loadConfigWorkspace(ctx, wsPath)
			if err != nil {
				// Config workspace might not exist yet
				continue
			}

		default: // "git" or unspecified
			repos, err = scanGitRepos(wsPath)
			if err != nil {
				// Workspace might not exist yet
				continue
			}
		}

		for _, repo := range repos {
			if _, exists := seenTargets[repo.TargetPath]; exists {
				continue
			}
			seenTargets[repo.TargetPath] = struct{}{}

			if repo.Strategy == "" {
				repo.Strategy = wsStrategy
			}
			plan.Input.Repos = append(plan.Input.Repos, repo)
		}
	}

	if len(plan.Input.Repos) == 0 {
		return ConfigData{}, errors.New("no git repositories found in workspaces")
	}

	plan.Options.Roots = roots

	run := reposync.RunOptions{
		Parallel:   defaultParallel,
		MaxRetries: defaultRetries,
	}

	return ConfigData{
		Plan: plan,
		Run:  run,
	}, nil
}

// loadForgeWorkspace fetches repos from a forge (GitHub/GitLab/Gitea).
func (l FileSpecLoader) loadForgeWorkspace( //nolint:gocyclo // complex provider resolution logic — adding helpers would obscure the fallback chain
	ctx context.Context,
	ws *workspaceEntry,
	profile *profileEntry,
	targetPath string,
	defaultCloneProto string,
	defaultSSHPort int,
) ([]reposync.RepoSpec, error) {
	if ws.Source == nil {
		return nil, errors.New("forge workspace requires source configuration")
	}

	src := ws.Source

	// Resolve provider settings (workspace source > profile > defaults)
	provider := src.Provider
	if provider == "" && profile != nil {
		provider = profile.Provider
	}
	if provider == "" {
		return nil, errors.New("forge workspace requires provider")
	}

	org := src.Org
	if org == "" {
		return nil, errors.New("forge workspace requires org")
	}

	baseURL := src.BaseURL
	if baseURL == "" && profile != nil {
		baseURL = profile.BaseURL
	}

	token := expandEnvVar(src.Token)
	if token == "" && profile != nil {
		token = expandEnvVar(profile.Token)
	}

	cloneProto := ws.CloneProto
	if cloneProto == "" && profile != nil {
		cloneProto = profile.CloneProto
	}
	if cloneProto == "" {
		cloneProto = defaultCloneProto
	}

	sshPort := ws.SSHPort
	if sshPort == 0 && profile != nil {
		sshPort = profile.SSHPort
	}
	if sshPort == 0 {
		sshPort = defaultSSHPort
	}

	includeSubgroups := src.IncludeSubgroups
	if !includeSubgroups && profile != nil {
		includeSubgroups = profile.IncludeSubgroups
	}

	subgroupMode := src.SubgroupMode
	if subgroupMode == "" && profile != nil {
		subgroupMode = profile.SubgroupMode
	}
	if subgroupMode == "" {
		subgroupMode = "flat"
	}

	flatSeparator := src.FlatSeparator
	if flatSeparator == "" && profile != nil {
		flatSeparator = profile.FlatSeparator
	}
	// Default separator is "-" (handled in buildTargetPath)

	// Create ForgePlanner and fetch repos
	forgeProvider, err := CreateForgeProviderRaw(provider, token, baseURL, sshPort)
	if err != nil {
		return nil, fmt.Errorf("create provider: %w", err)
	}

	plannerConfig := reposync.ForgePlannerConfig{
		TargetPath:       targetPath,
		Organization:     org,
		CloneProto:       cloneProto,
		SSHPort:          sshPort,
		IncludeSubgroups: includeSubgroups,
		SubgroupMode:     subgroupMode,
		FlatSeparator:    flatSeparator,
		IncludePrivate:   true,
	}

	planner := reposync.NewForgePlanner(forgeProvider, plannerConfig)

	// Generate plan to get repo list
	planResult, err := planner.Plan(ctx, reposync.PlanRequest{})
	if err != nil {
		return nil, fmt.Errorf("fetch repos: %w", err)
	}

	// Extract repos from plan actions
	repos := make([]reposync.RepoSpec, 0, len(planResult.Actions))
	for _, action := range planResult.Actions {
		repos = append(repos, action.Repo)
	}

	return repos, nil
}

// loadConfigWorkspace recursively loads a nested config file.
func (l FileSpecLoader) loadConfigWorkspace(ctx context.Context, wsPath string) ([]reposync.RepoSpec, error) {
	// Try .gz-git.yaml first, then .gz-git.yml
	candidates := []string{".gz-git.yaml", ".gz-git.yml"}

	for _, name := range candidates {
		configPath := filepath.Join(wsPath, name)
		if _, err := os.Stat(configPath); err == nil {
			// Recursively load the nested config
			data, err := l.Load(ctx, configPath)
			if err != nil {
				return nil, err
			}
			return data.Plan.Input.Repos, nil
		}
	}

	// No config file found, fall back to scanning for git repos
	return scanGitRepos(wsPath)
}

// expandEnvVar expands ${VAR} syntax in a string.
func expandEnvVar(s string) string {
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
		varName := s[2 : len(s)-1]
		return os.Getenv(varName)
	}
	return os.ExpandEnv(s)
}

// scanGitRepos scans a directory for git repositories (depth 1).
func scanGitRepos(dir string) ([]reposync.RepoSpec, error) {
	root, err := safefs.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()

	entries, err := root.ReadDir(".")
	if err != nil {
		return nil, err
	}

	var repos []reposync.RepoSpec
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Skip hidden directories
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		repoRel := entry.Name()
		repoPath := filepath.Join(dir, repoRel)
		gitDir := filepath.Join(repoRel, ".git")

		// Check if it's a git repository
		if info, err := root.Stat(gitDir); err == nil && info.IsDir() {
			// Get remote URL if available
			remoteURL := getGitRemoteURLAt(root, repoRel)

			repos = append(repos, reposync.RepoSpec{
				Name:       entry.Name(),
				TargetPath: repoPath,
				CloneURL:   remoteURL,
			})
		}
	}

	return repos, nil
}

// getGitRemoteURL gets the origin remote URL from a git repository.
func getGitRemoteURL(repoPath string) string {
	root, err := safefs.OpenRoot(repoPath)
	if err != nil {
		return ""
	}
	defer func() { _ = root.Close() }()

	return getGitRemoteURLAt(root, "")
}

func getGitRemoteURLAt(root *safefs.Root, repoRel string) string {
	configPath := filepath.Join(repoRel, ".git", "config")
	data, err := root.ReadFile(configPath)
	if err != nil {
		return ""
	}

	// Simple parsing to find [remote "origin"] url
	lines := strings.Split(string(data), "\n")
	inOrigin := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == `[remote "origin"]` {
			inOrigin = true
			continue
		}
		if inOrigin {
			if strings.HasPrefix(line, "[") {
				break
			}
			if after, ok := strings.CutPrefix(line, "url = "); ok {
				return after
			}
		}
	}
	return ""
}

// loadParentProfiles recursively loads parent configs and merges profiles.
// Returns merged profiles map where child profiles override parent profiles.
// Uses visited set to prevent circular references.
func loadParentProfiles(configPath string, visited map[string]bool) (map[string]*profileEntry, error) {
	absPath, err := filepath.Abs(cleanPath(configPath))
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	// Check for circular reference
	if visited[absPath] {
		return nil, fmt.Errorf("circular parent reference detected: %s", absPath)
	}
	visited[absPath] = true

	raw, err := os.ReadFile(absPath) // #nosec G304 -- absPath is resolved from the configured parent workspace.
	if err != nil {
		return nil, fmt.Errorf("read parent config %s: %w", absPath, err)
	}

	var cfg workspacesConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse parent config %s: %w", absPath, err)
	}

	// Start with parent's profiles (recursively load if parent has its own parent)
	profiles := make(map[string]*profileEntry)
	if cfg.Parent != "" {
		parentPath := cfg.Parent
		// Resolve relative path based on current config's directory
		if !filepath.IsAbs(parentPath) && !strings.HasPrefix(parentPath, "~") {
			parentPath = filepath.Join(filepath.Dir(absPath), parentPath)
		}
		parentProfiles, err := loadParentProfiles(parentPath, visited)
		if err != nil {
			// Log warning but continue - parent might not exist
			fmt.Fprintf(os.Stderr, "Warning: failed to load parent config: %v\n", err)
		} else {
			// Copy parent profiles
			maps.Copy(profiles, parentProfiles)
		}
	}

	// Merge current config's profiles (overrides parent)
	maps.Copy(profiles, cfg.Profiles)

	return profiles, nil
}

// mergeProfilesIntoConfig loads parent profiles and merges them into the current config.
// Child profiles override parent profiles.
func mergeProfilesIntoConfig(cfg *workspacesConfig, configPath string) error {
	if cfg.Parent == "" {
		return nil // No parent to merge
	}

	parentPath := cfg.Parent
	configDir := filepath.Dir(configPath)

	// Resolve relative path
	if !filepath.IsAbs(parentPath) && !strings.HasPrefix(parentPath, "~") {
		parentPath = filepath.Join(configDir, parentPath)
	}

	visited := make(map[string]bool)
	// Mark current config as visited
	if absPath, err := filepath.Abs(configPath); err == nil {
		visited[absPath] = true
	}

	parentProfiles, err := loadParentProfiles(parentPath, visited)
	if err != nil {
		return fmt.Errorf("load parent profiles: %w", err)
	}

	// Merge: parent profiles first, then child profiles override
	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]*profileEntry)
	}

	for k, v := range parentProfiles {
		if _, exists := cfg.Profiles[k]; !exists {
			cfg.Profiles[k] = v
		}
	}

	return nil
}

func cleanPath(path string) string {
	if path == "" {
		return path
	}
	expanded := os.ExpandEnv(path)
	if expanded == "~" || strings.HasPrefix(expanded, "~/") || strings.HasPrefix(expanded, "~\\") {
		if home, err := os.UserHomeDir(); err == nil {
			rest := strings.TrimPrefix(expanded[1:], "/")
			rest = strings.TrimPrefix(rest, "\\")
			expanded = filepath.Join(home, rest)
		}
	}
	return filepath.Clean(expanded)
}

func cleanRoots(roots []string) []string {
	out := make([]string, 0, len(roots))
	for _, root := range roots {
		if root == "" {
			continue
		}
		out = append(out, cleanPath(root))
	}
	return out
}

// detectConfigFile searches upward from the given directory to $HOME for a
// .gz-git config file (priority: .gz-git.yaml > .gz-git.yml > .gz-git.json).
// This is a thin wrapper around config.DetectConfigFile.
func detectConfigFile(dir string) (string, error) {
	return config.DetectConfigFile(dir)
}

// resolveParentPath resolves a parent config path relative to the current config.
// Supports: absolute paths, ~/relative paths, and relative paths.
func resolveParentPath(parentRef, configPath string) string {
	if parentRef == "" {
		return ""
	}

	// Expand environment variables
	parentRef = os.ExpandEnv(parentRef)

	// Handle ~ (home directory)
	if parentRef == "~" || strings.HasPrefix(parentRef, "~/") || strings.HasPrefix(parentRef, "~\\") {
		if home, err := os.UserHomeDir(); err == nil {
			rest := strings.TrimPrefix(parentRef[1:], "/")
			rest = strings.TrimPrefix(rest, "\\")
			parentRef = filepath.Join(home, rest)
		}
	}

	// If absolute, use as-is
	if filepath.IsAbs(parentRef) {
		return filepath.Clean(parentRef)
	}

	// Relative path: resolve relative to config file's directory
	configDir := filepath.Dir(configPath)
	return filepath.Clean(filepath.Join(configDir, parentRef))
}

// loadParentConfig loads a parent config file and returns its parsed fileConfig.
func (l FileSpecLoader) loadParentConfig(parentPath string) (fileConfig, error) {
	raw, err := os.ReadFile(parentPath) // #nosec G304 -- parentPath is a validated parent config path.
	if err != nil {
		return fileConfig{}, fmt.Errorf("read parent config %s: %w", parentPath, err)
	}

	var cfg fileConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return fileConfig{}, fmt.Errorf("parse parent config %s: %w", parentPath, err)
	}

	// Recursively load parent's parent if specified
	if cfg.Parent != "" {
		grandparentPath := resolveParentPath(cfg.Parent, parentPath)
		grandparentCfg, err := l.loadParentConfig(grandparentPath)
		if err != nil {
			// Log warning but continue
			fmt.Fprintf(os.Stderr, "Warning: failed to load grandparent config %s: %v\n", grandparentPath, err)
		} else {
			// Merge grandparent into parent (parent overrides grandparent)
			cfg = mergeFileConfigs(grandparentCfg, cfg)
		}
	}

	return cfg, nil
}

// mergeFileConfigs merges parent config into child config.
// Child values override parent values (child takes precedence).
func mergeFileConfigs(parent, child fileConfig) fileConfig {
	result := parent // Start with parent as base

	// Child meta fields override if set
	if child.Version != 0 {
		result.Version = child.Version
	}
	if child.Kind != "" {
		result.Kind = child.Kind
	}
	if child.Metadata != nil {
		result.Metadata = child.Metadata
	}

	// Parent reference is NOT inherited (child's parent is its own declaration)
	result.Parent = child.Parent
	if child.Profile != "" {
		result.Profile = child.Profile
	}

	// Sync settings: child overrides if set
	if child.Strategy != "" {
		result.Strategy = child.Strategy
	}
	if child.Parallel != 0 {
		result.Parallel = child.Parallel
	}
	if child.MaxRetries != nil {
		result.MaxRetries = child.MaxRetries
	}
	if child.Resume {
		result.Resume = child.Resume
	}
	if child.DryRun {
		result.DryRun = child.DryRun
	}
	if child.CleanupOrphans {
		result.CleanupOrphans = child.CleanupOrphans
	}
	if child.StrictBranchCheckout {
		result.StrictBranchCheckout = child.StrictBranchCheckout
	}

	// Roots: child replaces if set (not merged)
	if len(child.Roots) > 0 {
		result.Roots = child.Roots
	}

	// Repositories: child replaces entirely (not merged)
	// This is intentional: child config defines its own repo list
	if len(child.Repositories) > 0 {
		result.Repositories = child.Repositories
	}

	return result
}
