// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// MaxConfigDepth limits the recursion depth for config loading.
	// This prevents stack overflow from deeply nested or malformed configs.
	MaxConfigDepth = 10
)

// LoadConfigRecursive loads a config file and recursively loads all workspaces.
// This function works at ANY level (workstation, workspace, project, etc.)
//
// Parameters:
//   - path: The directory containing the config file
//   - configFile: The config file name (e.g., ".gz-git.yaml", ".gz-git-config.yaml")
//
// Returns:
//   - *Config: The loaded config with all workspaces recursively loaded
//   - error: Any error encountered during loading
//
// Example:
//
//	// Load workstation config
//	home, _ := os.UserHomeDir()
//	config, err := LoadConfigRecursive(home, ".gz-git-config.yaml")
//
//	// Load workspace config
//	config, err := LoadConfigRecursive("/home/user/mydevbox", ".gz-git.yaml")
func LoadConfigRecursive(path, configFile string) (*Config, error) {
	return loadConfigRecursiveWithVisited(path, configFile, make(map[string]bool), true, 0)
}

// loadConfigRecursiveWithVisited loads config with circular reference detection.
//
//nolint:gocognit,gocyclo // complex by design: handles recursive config loading with cycle detection, path resolution, and workspace traversal
func loadConfigRecursiveWithVisited(path, configFile string, visited map[string]bool, loadWorkspaces bool, depth int) (*Config, error) {
	// Check depth limit
	if depth > MaxConfigDepth {
		return nil, fmt.Errorf("config recursion depth exceeded (max %d): possible circular or excessively nested config", MaxConfigDepth)
	}

	// 1. Load this level's config file
	configPath := filepath.Join(path, configFile)
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve config path %s: %w", configPath, err)
	}

	// Check for circular reference
	if visited[absConfigPath] {
		return nil, fmt.Errorf("circular config reference detected: %s", absConfigPath)
	}
	visited[absConfigPath] = true

	data, err := os.ReadFile(absConfigPath) // #nosec G304 -- absConfigPath is resolved from the configured workspace root.
	if err != nil {
		return nil, fmt.Errorf("failed to read config %s: %w", absConfigPath, err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config %s: %w", absConfigPath, err)
	}

	// Store the config path for relative path resolution
	config.ConfigPath = absConfigPath

	// Expand environment variables
	validator := NewValidator()
	if err := validator.ExpandEnvVarsInConfig(&config); err != nil {
		return nil, fmt.Errorf("failed to expand env vars in %s: %w", absConfigPath, err)
	}

	// 2. Load parent config if specified
	if config.Parent != "" {
		parentPath, err := resolvePath(path, config.Parent)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve parent path '%s' in %s: %w", config.Parent, absConfigPath, err)
		}

		// Determine parent config file and directory
		parentDir, parentFile := filepath.Split(parentPath)
		if parentFile == "" {
			// Parent is a directory, use default config file
			parentDir = parentPath
			parentFile = ".gz-git.yaml"
		}

		// Load parent config recursively (with same visited map for cycle detection)
		// IMPORTANT: Pass loadWorkspaces=false to prevent infinite recursion
		// We only want the parent's settings, not to traverse its workspaces again
		parentConfig, err := loadConfigRecursiveWithVisited(parentDir, parentFile, visited, false, depth+1)
		if err != nil {
			return nil, fmt.Errorf("failed to load parent config '%s': %w", config.Parent, err)
		}

		// Merge parent config (child overrides parent)
		mergeParentConfig(&config, parentConfig)
		config.ParentConfig = parentConfig
	}

	// 3. Recursively load workspaces
	// Skip if disabled (e.g., when we are being loaded as a parent)
	if !loadWorkspaces {
		return &config, nil
	}

	for name, ws := range config.Workspaces {
		if ws == nil {
			continue
		}

		// Default path to workspace name when omitted (compact config convention)
		effectivePath := ws.Path
		if effectivePath == "" {
			effectivePath = name
		}

		// Resolve workspace path (handle ~, relative paths)
		wsPath, err := resolvePath(path, effectivePath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve workspace '%s' path '%s' in %s: %w", name, effectivePath, configPath, err)
		}

		// CRITICAL: Update the workspace path with the resolved absolute path
		// This ensures that downstream code (e.g., sync_command.go) uses the expanded path
		ws.Path = wsPath

		// Determine effective type
		effectiveType := ws.Type.Resolve(ws.Source != nil)

		switch effectiveType {
		case WorkspaceTypeConfig:
			// Workspace has a nested config file - recurse!
			nestedConfigFile := ".gz-git.yaml"
			nestedConfig, err := LoadConfigRecursive(wsPath, nestedConfigFile)
			if err != nil {
				// Config file not found is OK (use workspace settings only)
				// But other errors (permission denied, disk I/O) should be reported
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return nil, fmt.Errorf("failed to load workspace '%s' config: %w", name, err)
			}

			// Merge workspace overrides into loaded config
			mergeWorkspaceOverrides(nestedConfig, ws)

		case WorkspaceTypeGit:
			// Plain git repo - validate that it exists and is a git repo
			if !isGitRepo(wsPath) {
				// Git repo doesn't exist yet - this is OK for sync targets
				// It will be cloned when sync is run
				continue
			}

		case WorkspaceTypeForge:
			// Forge workspace - target directory may not exist yet
			// It will be created when forge from is run
			// No validation needed here
		}

		// Recursively process nested workspaces
		for nestedName, nestedWs := range ws.Workspaces {
			if nestedWs == nil {
				continue
			}

			nestedPath, err := resolvePath(wsPath, nestedWs.Path)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve nested workspace '%s/%s' path '%s': %w",
					name, nestedName, nestedWs.Path, err)
			}

			nestedType := nestedWs.Type.Resolve(nestedWs.Source != nil)
			if nestedType == WorkspaceTypeConfig {
				nestedConfigFile := ".gz-git.yaml"
				_, err := LoadConfigRecursive(nestedPath, nestedConfigFile)
				if err != nil {
					// Config not found is OK, but report other errors
					if errors.Is(err, os.ErrNotExist) {
						continue
					}
					return nil, fmt.Errorf("failed to load nested workspace '%s/%s' config: %w",
						name, nestedName, err)
				}
			}
		}
	}

	return &config, nil
}

// resolvePath resolves a path relative to a parent directory.
// Handles:
//   - Home-relative paths: ~/foo/bar → /home/user/foo/bar
//   - Absolute paths: /foo/bar → /foo/bar
//   - Relative paths: ./foo → /parent/path/foo
//   - Relative paths: foo → /parent/path/foo
func resolvePath(parentPath, childPath string) (string, error) {
	// Handle home-relative paths
	if strings.HasPrefix(childPath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		return filepath.Join(home, childPath[2:]), nil
	}

	// Handle absolute paths
	if filepath.IsAbs(childPath) {
		return childPath, nil
	}

	// Handle relative paths
	return filepath.Join(parentPath, childPath), nil
}

// mergeWorkspaceOverrides applies workspace overrides to loaded Config.
// Workspace overrides take precedence over the nested config file settings.
//
//nolint:gocognit // complex by design: merges many optional sub-config fields across multiple config types
func mergeWorkspaceOverrides(config *Config, ws *Workspace) {
	if ws.Profile != "" {
		config.Profile = ws.Profile
	}
	// Merge clone/sync settings into defaults
	if ws.Parallel > 0 || ws.CloneProto != "" || ws.SSHPort > 0 {
		if config.Defaults == nil {
			config.Defaults = &DefaultsConfig{}
		}
		if ws.CloneProto != "" || ws.SSHPort > 0 {
			if config.Defaults.Clone == nil {
				config.Defaults.Clone = &CloneDefaults{}
			}
			if ws.CloneProto != "" {
				config.Defaults.Clone.Proto = ws.CloneProto
			}
			if ws.SSHPort > 0 {
				config.Defaults.Clone.SSHPort = ws.SSHPort
			}
		}
		if ws.Parallel > 0 {
			if config.Defaults.Sync == nil {
				config.Defaults.Sync = &SyncDefaults{}
			}
			config.Defaults.Sync.Parallel = ws.Parallel
		}
	}
	if ws.Sync != nil {
		if config.Sync == nil {
			config.Sync = &SyncConfig{}
		}
		mergeSyncConfig(config.Sync, ws.Sync)
	}
	if ws.Branch != nil {
		if config.Branch == nil {
			config.Branch = &BranchConfig{}
		}
		mergeBranchConfig(config.Branch, ws.Branch)
	}
	if ws.Fetch != nil {
		if config.Fetch == nil {
			config.Fetch = &FetchConfig{}
		}
		mergeFetchConfig(config.Fetch, ws.Fetch)
	}
	if ws.Pull != nil {
		if config.Pull == nil {
			config.Pull = &PullConfig{}
		}
		mergePullConfig(config.Pull, ws.Pull)
	}
	if ws.Push != nil {
		if config.Push == nil {
			config.Push = &PushConfig{}
		}
		mergePushConfig(config.Push, ws.Push)
	}
}

// mergeSyncConfig merges override into target (override takes precedence).
func mergeSyncConfig(target, override *SyncConfig) {
	if override.Strategy != "" {
		target.Strategy = override.Strategy
	}
	if override.MaxRetries > 0 {
		target.MaxRetries = override.MaxRetries
	}
	if override.Timeout != "" {
		target.Timeout = override.Timeout
	}
}

// mergeBranchConfig merges override into target (override takes precedence).
func mergeBranchConfig(target, override *BranchConfig) {
	if len(override.DefaultBranch) > 0 {
		target.DefaultBranch = override.DefaultBranch
	}
	if len(override.ProtectedBranches) > 0 {
		target.ProtectedBranches = override.ProtectedBranches
	}
}

// mergeFetchConfig merges override into target (override takes precedence).
func mergeFetchConfig(target, override *FetchConfig) {
	if override.AllRemotes {
		target.AllRemotes = override.AllRemotes
	}
	if override.Prune {
		target.Prune = override.Prune
	}
}

// mergePullConfig merges override into target (override takes precedence).
func mergePullConfig(target, override *PullConfig) {
	if override.Rebase {
		target.Rebase = override.Rebase
	}
	if override.FFOnly {
		target.FFOnly = override.FFOnly
	}
}

// mergePushConfig merges override into target (override takes precedence).
func mergePushConfig(target, override *PushConfig) {
	if override.SetUpstream {
		target.SetUpstream = override.SetUpstream
	}
	if override.Policy != nil {
		target.Policy = override.Policy
	}
}

// mergeParentConfig merges parent config into child config.
// Child values override parent values (child takes precedence).
// This is the inverse of mergeWorkspaceOverrides - parent provides defaults.
//
//nolint:gocyclo // complex by design: merges many optional config fields across provider, defaults, and command-specific configs
func mergeParentConfig(child, parent *Config) {
	if parent == nil {
		return
	}

	// Merge forge provider settings (parent provides defaults)
	if child.Provider == "" && parent.Provider != "" {
		child.Provider = parent.Provider
	}
	if child.BaseURL == "" && parent.BaseURL != "" {
		child.BaseURL = parent.BaseURL
	}
	if child.Token == "" && parent.Token != "" {
		child.Token = parent.Token
	}
	if !child.IncludeSubgroups && parent.IncludeSubgroups {
		child.IncludeSubgroups = parent.IncludeSubgroups
	}
	if child.SubgroupMode == "" && parent.SubgroupMode != "" {
		child.SubgroupMode = parent.SubgroupMode
	}

	// Merge defaults (parent provides defaults)
	mergeParentDefaults(child, parent)

	// Merge command-specific configs (child overrides, but parent provides defaults)
	if child.Sync == nil && parent.Sync != nil {
		child.Sync = &SyncConfig{}
		*child.Sync = *parent.Sync
	} else if child.Sync != nil && parent.Sync != nil {
		mergeParentSyncConfig(child.Sync, parent.Sync)
	}

	if child.Branch == nil && parent.Branch != nil {
		child.Branch = &BranchConfig{}
		*child.Branch = *parent.Branch
	} else if child.Branch != nil && parent.Branch != nil {
		mergeParentBranchConfig(child.Branch, parent.Branch)
	}

	if child.Fetch == nil && parent.Fetch != nil {
		child.Fetch = &FetchConfig{}
		*child.Fetch = *parent.Fetch
	}

	if child.Pull == nil && parent.Pull != nil {
		child.Pull = &PullConfig{}
		*child.Pull = *parent.Pull
	}

	if child.Push == nil && parent.Push != nil {
		child.Push = &PushConfig{}
		*child.Push = *parent.Push
	}

	// Note: Profiles are NOT merged automatically.
	// Profile lookup traverses the parent chain via GetProfileFromChain().
	// Workspaces are also NOT merged - each level has its own workspaces.
}

// mergeParentSyncConfig fills empty child fields from parent.
func mergeParentSyncConfig(child, parent *SyncConfig) {
	if child.Strategy == "" && parent.Strategy != "" {
		child.Strategy = parent.Strategy
	}
	if child.MaxRetries == 0 && parent.MaxRetries != 0 {
		child.MaxRetries = parent.MaxRetries
	}
	if child.Timeout == "" && parent.Timeout != "" {
		child.Timeout = parent.Timeout
	}
}

// mergeParentBranchConfig fills empty child fields from parent.
func mergeParentBranchConfig(child, parent *BranchConfig) {
	if len(child.DefaultBranch) == 0 && len(parent.DefaultBranch) > 0 {
		child.DefaultBranch = parent.DefaultBranch
	}
	if len(child.ProtectedBranches) == 0 && len(parent.ProtectedBranches) > 0 {
		child.ProtectedBranches = parent.ProtectedBranches
	}
}

// mergeParentDefaults merges parent defaults into child defaults.
// Child values take precedence over parent values.
//
//nolint:gocognit,gocyclo // complex by design: merges many optional nested default config sections
func mergeParentDefaults(child, parent *Config) {
	if parent.Defaults == nil {
		return
	}

	if child.Defaults == nil {
		child.Defaults = &DefaultsConfig{}
	}

	// Merge Clone defaults
	if parent.Defaults.Clone != nil {
		if child.Defaults.Clone == nil {
			child.Defaults.Clone = &CloneDefaults{}
		}
		if child.Defaults.Clone.Proto == "" {
			child.Defaults.Clone.Proto = parent.Defaults.Clone.Proto
		}
		if child.Defaults.Clone.SSHPort == 0 {
			child.Defaults.Clone.SSHPort = parent.Defaults.Clone.SSHPort
		}
		if child.Defaults.Clone.SSHKeyPath == "" {
			child.Defaults.Clone.SSHKeyPath = parent.Defaults.Clone.SSHKeyPath
		}
		if child.Defaults.Clone.SSHKeyContent == "" {
			child.Defaults.Clone.SSHKeyContent = parent.Defaults.Clone.SSHKeyContent
		}
	}

	// Merge Sync defaults
	if parent.Defaults.Sync != nil {
		if child.Defaults.Sync == nil {
			child.Defaults.Sync = &SyncDefaults{}
		}
		if child.Defaults.Sync.Strategy == "" {
			child.Defaults.Sync.Strategy = parent.Defaults.Sync.Strategy
		}
		if child.Defaults.Sync.Parallel == 0 {
			child.Defaults.Sync.Parallel = parent.Defaults.Sync.Parallel
		}
		if child.Defaults.Sync.MaxRetries == 0 {
			child.Defaults.Sync.MaxRetries = parent.Defaults.Sync.MaxRetries
		}
		if child.Defaults.Sync.Timeout == "" {
			child.Defaults.Sync.Timeout = parent.Defaults.Sync.Timeout
		}
	}

	// Merge Scan defaults
	if parent.Defaults.Scan != nil {
		if child.Defaults.Scan == nil {
			child.Defaults.Scan = &ScanDefaults{}
		}
		if child.Defaults.Scan.Depth == 0 {
			child.Defaults.Scan.Depth = parent.Defaults.Scan.Depth
		}
	}

	// Merge Output defaults
	if parent.Defaults.Output != nil {
		if child.Defaults.Output == nil {
			child.Defaults.Output = &OutputDefaults{}
		}
		if !child.Defaults.Output.Compact {
			child.Defaults.Output.Compact = parent.Defaults.Output.Compact
		}
		if child.Defaults.Output.Format == "" {
			child.Defaults.Output.Format = parent.Defaults.Output.Format
		}
	}

	// Merge Filter defaults
	if parent.Defaults.Filter != nil {
		if child.Defaults.Filter == nil {
			child.Defaults.Filter = &FilterDefaults{}
		}
		if len(child.Defaults.Filter.Include) == 0 {
			child.Defaults.Filter.Include = parent.Defaults.Filter.Include
		}
		if len(child.Defaults.Filter.Exclude) == 0 {
			child.Defaults.Filter.Exclude = parent.Defaults.Filter.Exclude
		}
	}
}

// isGitRepo checks if a directory is a git repository.
func isGitRepo(path string) bool {
	gitDir := filepath.Join(path, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// LoadWorkspaces loads workspaces based on discovery mode.
// This function is called after LoadConfigRecursive to optionally auto-discover workspaces.
//
// Parameters:
//   - path: The directory to search for workspaces
//   - config: The config to append discovered workspaces to
//   - mode: The discovery mode (explicit, auto, hybrid)
//
// Returns:
//   - error: Any error encountered during discovery
//
// Example:
//
//	config, _ := LoadConfigRecursive("/home/user/mydevbox", ".gz-git.yaml")
//	err := LoadWorkspaces("/home/user/mydevbox", config, HybridMode)
func LoadWorkspaces(path string, config *Config, mode DiscoveryMode) error {
	// Use the default mode if not specified
	mode = mode.Default()

	switch mode {
	case ExplicitMode:
		// Already loaded by LoadConfigRecursive - nothing to do
		return nil

	case AutoMode:
		// Scan directory and replace config.Workspaces with discovered repos
		config.Workspaces = nil // Clear explicit workspaces
		return autoDiscoverWorkspaces(path, config)

	case HybridMode:
		// Use explicit workspaces if defined, otherwise auto-discover
		if len(config.Workspaces) > 0 {
			return nil // Use explicit
		}
		return autoDiscoverWorkspaces(path, config)
	}

	return nil
}

// autoDiscoverWorkspaces scans directory and appends discovered repos to config.Workspaces.
func autoDiscoverWorkspaces(path string, config *Config) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", path, err)
	}

	if config.Workspaces == nil {
		config.Workspaces = make(map[string]*Workspace)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip hidden directories
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		childPath := filepath.Join(path, entry.Name())

		// Check if it has a config file
		if hasFile(childPath, ".gz-git.yaml") {
			config.Workspaces[entry.Name()] = &Workspace{
				Path: childPath,
				Type: WorkspaceTypeConfig,
			}
			continue
		}

		// Check if it's a git repo
		if isGitRepo(childPath) {
			config.Workspaces[entry.Name()] = &Workspace{
				Path: childPath,
				Type: WorkspaceTypeGit,
			}
		}
	}

	return nil
}

// hasFile checks if a file exists in a directory.
func hasFile(dir, fileName string) bool {
	filePath := filepath.Join(dir, fileName)
	info, err := os.Stat(filePath)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// FindConfigRecursive walks up from startPath to $HOME to find the nearest
// directory containing configFile. It delegates to the shared findConfigUpward
// core, so the $HOME ceiling is identical to FindProjectConfig/DetectConfigFile;
// the configFile parameter lets callers probe a non-default filename.
//
// Parameters:
//   - startPath: The directory to start searching from
//   - configFile: The config file name to search for (e.g., ".gz-git.yaml")
//
// Returns:
//   - string: The directory containing the config file
//   - error: Error if config file not found
//
// Example:
//
//	// Find nearest .gz-git.yaml
//	configDir, err := FindConfigRecursive("/home/user/mydevbox/project", ".gz-git.yaml")
//	// Returns: "/home/user/mydevbox" if .gz-git.yaml exists there
func FindConfigRecursive(startPath, configFile string) (string, error) {
	absStart, err := filepath.Abs(startPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	dir, _, err := findConfigUpward(absStart, []string{configFile})
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", fmt.Errorf("config file %s not found in %s or any parent directory", configFile, startPath)
	}
	return dir, nil
}

// GetWorkspaceByName returns a workspace by name from the config.
func GetWorkspaceByName(config *Config, name string) *Workspace {
	if config == nil || config.Workspaces == nil {
		return nil
	}
	return config.Workspaces[name]
}

// GetAllWorkspaces returns all workspaces as a slice with their names.
func GetAllWorkspaces(config *Config) []struct {
	Name      string
	Workspace *Workspace
} {
	if config == nil || config.Workspaces == nil {
		return nil
	}

	result := make([]struct {
		Name      string
		Workspace *Workspace
	}, 0, len(config.Workspaces))

	for name, ws := range config.Workspaces {
		result = append(result, struct {
			Name      string
			Workspace *Workspace
		}{Name: name, Workspace: ws})
	}

	return result
}

// GetForgeWorkspaces returns only workspaces that sync from a forge.
func GetForgeWorkspaces(config *Config) map[string]*Workspace {
	result := make(map[string]*Workspace)

	if config == nil || config.Workspaces == nil {
		return result
	}

	for name, ws := range config.Workspaces {
		if ws.Source != nil || ws.Type == WorkspaceTypeForge {
			result[name] = ws
		}
	}

	return result
}

// GetGitWorkspaces returns only workspaces that are single git repositories (type=git with URL).
// These are workspaces with explicit URL that should be cloned/synced directly.
func GetGitWorkspaces(config *Config) map[string]*Workspace {
	result := make(map[string]*Workspace)

	if config == nil || config.Workspaces == nil {
		return result
	}

	for name, ws := range config.Workspaces {
		// Must be type=git (explicit or inferred) with a URL
		effectiveType := ws.Type.Resolve(ws.Source != nil)
		if effectiveType == WorkspaceTypeGit && ws.URL != "" {
			result[name] = ws
		}
	}

	return result
}

// GetConfigWorkspaces returns workspaces with type=config that have sync.recursive enabled.
// These are workspaces whose child config should be loaded and synced by the parent.
func GetConfigWorkspaces(config *Config) map[string]*Workspace {
	result := make(map[string]*Workspace)

	if config == nil || config.Workspaces == nil {
		return result
	}

	for name, ws := range config.Workspaces {
		// Skip forge and git workspaces (handled by their own planners)
		if ws.Source != nil || ws.Type == WorkspaceTypeForge {
			continue
		}
		effectiveType := ws.Type.Resolve(ws.Source != nil)
		if effectiveType == WorkspaceTypeGit && ws.URL != "" {
			continue
		}

		// Only include if sync.recursive is true
		if ws.Sync != nil && ws.Sync.Recursive {
			result[name] = ws
		}
	}

	return result
}

// GetProfileByName returns a profile by name from the config.
// Lookup order: inline (config.Profiles) → external (~/.config/gz-git/profiles/)
// Returns nil if profile not found in either location.
func GetProfileByName(config *Config, name string) *Profile {
	if config == nil || name == "" {
		return nil
	}

	// 1. Check inline profiles first (higher priority)
	if config.Profiles != nil {
		if profile, ok := config.Profiles[name]; ok {
			return profile
		}
	}

	// 2. External profiles would be loaded by manager.go
	// This function only handles inline profiles
	// For external profile loading, use Manager.GetProfile()
	return nil
}

// GetAllProfiles returns all inline profiles from the config.
func GetAllProfiles(config *Config) map[string]*Profile {
	if config == nil || config.Profiles == nil {
		return make(map[string]*Profile)
	}
	return config.Profiles
}

// HasInlineProfile checks if a profile exists in the inline profiles.
func HasInlineProfile(config *Config, name string) bool {
	if config == nil || config.Profiles == nil {
		return false
	}
	_, ok := config.Profiles[name]
	return ok
}

// GetProfileFromChain looks up a profile by traversing the parent config chain.
// Lookup order:
//  1. Current config's inline profiles (config.Profiles)
//  2. Parent config's inline profiles (config.ParentConfig.Profiles)
//  3. Grandparent config's inline profiles (and so on...)
//
// Returns nil if profile not found in any level.
// For external profiles (~/.config/gz-git/profiles/), use Manager.GetProfile().
func GetProfileFromChain(config *Config, name string) *Profile {
	if config == nil || name == "" {
		return nil
	}

	// 1. Check current config's inline profiles
	if config.Profiles != nil {
		if profile, ok := config.Profiles[name]; ok {
			return profile
		}
	}

	// 2. Traverse parent chain
	if config.ParentConfig != nil {
		return GetProfileFromChain(config.ParentConfig, name)
	}

	// 3. Not found in chain - caller should check external profiles
	return nil
}

// GetProfileSource returns the source location of a profile.
// Useful for debugging config precedence.
// Returns empty string if profile not found.
func GetProfileSource(config *Config, name string) string {
	if config == nil || name == "" {
		return ""
	}

	// Check current config's inline profiles
	if config.Profiles != nil {
		if _, ok := config.Profiles[name]; ok {
			if config.ConfigPath != "" {
				return config.ConfigPath
			}
			return "inline"
		}
	}

	// Traverse parent chain
	if config.ParentConfig != nil {
		return GetProfileSource(config.ParentConfig, name)
	}

	return ""
}

// GetParentChain returns all configs in the parent chain (including current).
// Useful for debugging and displaying config precedence.
func GetParentChain(config *Config) []*Config {
	if config == nil {
		return nil
	}

	chain := []*Config{config}
	current := config.ParentConfig
	for current != nil {
		chain = append(chain, current)
		current = current.ParentConfig
	}
	return chain
}
