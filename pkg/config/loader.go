// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"reflect"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// ConfigLoader handles configuration loading with 5-layer precedence.
//
// Precedence (highest to lowest):
//  1. Command flags
//  2. Project config (.gz-git.yaml)
//  3. Active profile
//  4. Global config
//  5. Built-in defaults
type ConfigLoader struct {
	manager       *Manager
	globalConfig  *GlobalConfig
	activeProfile *Profile
	projectConfig *ProjectConfig
}

// NewLoader creates a new configuration loader.
func NewLoader() (*ConfigLoader, error) {
	manager, err := NewManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create manager: %w", err)
	}

	return &ConfigLoader{
		manager: manager,
	}, nil
}

// Load loads all configuration layers.
func (l *ConfigLoader) Load() error {
	// Load global config
	globalConfig, err := l.manager.LoadGlobalConfig()
	if err != nil {
		return fmt.Errorf("failed to load global config: %w", err)
	}
	l.globalConfig = globalConfig

	// Load active profile
	activeProfileName, err := l.manager.GetActiveProfile()
	if err != nil {
		return fmt.Errorf("failed to get active profile: %w", err)
	}

	if activeProfileName != "" && l.manager.ProfileExists(activeProfileName) {
		activeProfile, err := l.manager.LoadProfile(activeProfileName)
		if err != nil {
			return fmt.Errorf("failed to load active profile '%s': %w", activeProfileName, err)
		}
		l.activeProfile = activeProfile
	}

	// Load project config (may be nil if not found)
	projectConfig, err := l.manager.LoadProjectConfig()
	if err != nil {
		return fmt.Errorf("failed to load project config: %w", err)
	}
	l.projectConfig = projectConfig

	// If project specifies a different profile, load it
	if projectConfig != nil && projectConfig.Profile != "" && projectConfig.Profile != activeProfileName {
		projectProfile, err := l.manager.LoadProfile(projectConfig.Profile)
		if err != nil {
			return fmt.Errorf("failed to load project profile '%s': %w", projectConfig.Profile, err)
		}
		// Project profile takes precedence over active profile
		l.activeProfile = projectProfile
	}

	return nil
}

// ResolveConfig builds the effective configuration with precedence.
// Flags parameter contains command-line flag values (highest priority).
func (l *ConfigLoader) ResolveConfig(flags map[string]any) (*EffectiveConfig, error) {
	effective := &EffectiveConfig{
		Sources: make(map[string]string),
	}

	// Layer 1: Built-in defaults
	l.applyDefaults(effective)

	// Layer 2: Global config
	if l.globalConfig != nil {
		l.applyGlobalConfig(effective)
	}

	// Layer 3: Active profile
	if l.activeProfile != nil {
		l.applyProfile(effective)
	}

	// Layer 4: Project config
	if l.projectConfig != nil {
		l.applyProjectConfig(effective)
	}

	// Layer 5: OS keychain (above config files; below env/flags)
	l.applyKeychainToken(effective)

	// Layer 6: Environment variables (CI overrides; below flags)
	l.applyEnvToken(effective)

	// Layer 7: Command flags (highest priority)
	l.applyFlags(effective, flags)

	return effective, nil
}

// applyDefaults applies built-in default values.
func (l *ConfigLoader) applyDefaults(cfg *EffectiveConfig) {
	cfg.Parallel = repository.DefaultLocalParallel
	cfg.CloneProto = "ssh"
	cfg.Sync.Strategy = "pull"
	cfg.Fetch.AllRemotes = true
	cfg.Pull.Rebase = false
	cfg.Pull.FFOnly = false
	cfg.Push.SetUpstream = false

	// Mark all as default source
	cfg.Sources["parallel"] = string(SourceDefault)
	cfg.Sources["cloneProto"] = string(SourceDefault)
	cfg.Sources["sync.strategy"] = string(SourceDefault)
	cfg.Sources["fetch.allRemotes"] = string(SourceDefault)
	cfg.Sources["pull.rebase"] = string(SourceDefault)
	cfg.Sources["pull.ffOnly"] = string(SourceDefault)
	cfg.Sources["push.setUpstream"] = string(SourceDefault)
}

// applyGlobalConfig applies global configuration defaults.
func (l *ConfigLoader) applyGlobalConfig(cfg *EffectiveConfig) {
	if l.globalConfig == nil || l.globalConfig.Defaults == nil {
		return
	}

	defaults := l.globalConfig.Defaults

	// Apply global defaults
	if v, ok := defaults["parallel"].(int); ok {
		cfg.Parallel = v
		cfg.Sources["parallel"] = string(SourceGlobal)
	}
	if v, ok := defaults["cloneProto"].(string); ok {
		cfg.CloneProto = v
		cfg.Sources["cloneProto"] = string(SourceGlobal)
	}
}

// applyProfile applies active profile configuration.
func (l *ConfigLoader) applyProfile(cfg *EffectiveConfig) {
	if l.activeProfile == nil {
		return
	}

	prof := l.activeProfile
	source := fmt.Sprintf("%s:%s", SourceProfile, prof.Name)

	// Provider settings
	if prof.Provider != "" {
		cfg.Provider = prof.Provider
		cfg.Sources["provider"] = source
	}
	if prof.BaseURL != "" {
		cfg.BaseURL = prof.BaseURL
		cfg.Sources["baseURL"] = source
	}
	if prof.Token != "" {
		cfg.Token = prof.Token
		cfg.Sources["token"] = source
	}

	// Clone settings
	if prof.CloneProto != "" {
		cfg.CloneProto = prof.CloneProto
		cfg.Sources["cloneProto"] = source
	}
	if prof.SSHPort > 0 {
		cfg.SSHPort = prof.SSHPort
		cfg.Sources["sshPort"] = source
	}

	// Bulk settings
	if prof.Parallel > 0 {
		cfg.Parallel = prof.Parallel
		cfg.Sources["parallel"] = source
	}
	if prof.IncludeSubgroups {
		cfg.IncludeSubgroups = prof.IncludeSubgroups
		cfg.Sources["includeSubgroups"] = source
	}
	if prof.SubgroupMode != "" {
		cfg.SubgroupMode = prof.SubgroupMode
		cfg.Sources["subgroupMode"] = source
	}

	// Command-specific configs
	if prof.Sync != nil {
		l.applySyncConfig(&cfg.Sync, prof.Sync)
	}
	if prof.Branch != nil {
		l.applyBranchConfig(&cfg.Branch, prof.Branch)
	}
	if prof.Fetch != nil {
		l.applyFetchConfig(&cfg.Fetch, prof.Fetch)
	}
	if prof.Pull != nil {
		l.applyPullConfig(&cfg.Pull, prof.Pull)
	}
	if prof.Push != nil {
		l.applyPushConfig(&cfg.Push, prof.Push)
	}
}

// applyProjectConfig applies project configuration.
func (l *ConfigLoader) applyProjectConfig(cfg *EffectiveConfig) {
	if l.projectConfig == nil {
		return
	}

	proj := l.projectConfig

	// Command-specific configs
	if proj.Sync != nil {
		l.applySyncConfig(&cfg.Sync, proj.Sync)
	}
	if proj.Branch != nil {
		l.applyBranchConfig(&cfg.Branch, proj.Branch)
	}
	if proj.Fetch != nil {
		l.applyFetchConfig(&cfg.Fetch, proj.Fetch)
	}
	if proj.Pull != nil {
		l.applyPullConfig(&cfg.Pull, proj.Pull)
	}
	if proj.Push != nil {
		l.applyPushConfig(&cfg.Push, proj.Push)
	}
}

// applyKeychainToken loads a forge token from the OS keychain when available.
// Failures (missing backend, not found) are silent so headless/CI keeps working.
func (l *ConfigLoader) applyKeychainToken(cfg *EffectiveConfig) {
	if cfg.Provider == "" {
		return
	}
	tok, err := DefaultTokenStore.Get(cfg.Provider)
	if err != nil || tok == "" {
		return
	}
	cfg.Token = tok
	cfg.Sources["token"] = string(SourceKeychain)
}

// applyEnvToken applies provider/env token overrides (CI-friendly).
func (l *ConfigLoader) applyEnvToken(cfg *EffectiveConfig) {
	tok, source := ResolveTokenFromEnv(cfg.Provider)
	if tok == "" {
		return
	}
	cfg.Token = tok
	cfg.Sources["token"] = source
}

// applyFlags applies command-line flags (highest priority).
func (l *ConfigLoader) applyFlags(cfg *EffectiveConfig, flags map[string]any) {
	if flags == nil {
		return
	}

	source := string(SourceFlag)

	// Provider settings
	if v, ok := flags["provider"].(string); ok && v != "" {
		cfg.Provider = v
		cfg.Sources["provider"] = source
	}
	if v, ok := flags["base-url"].(string); ok && v != "" {
		cfg.BaseURL = v
		cfg.Sources["baseURL"] = source
	}
	if v, ok := flags["token"].(string); ok && v != "" {
		cfg.Token = v
		cfg.Sources["token"] = source
	}

	// Clone settings
	if v, ok := flags["clone-proto"].(string); ok && v != "" {
		cfg.CloneProto = v
		cfg.Sources["cloneProto"] = source
	}
	if v, ok := flags["ssh-port"].(int); ok && v > 0 {
		cfg.SSHPort = v
		cfg.Sources["sshPort"] = source
	}

	// Bulk settings
	if v, ok := flags["parallel"].(int); ok && v > 0 {
		cfg.Parallel = v
		cfg.Sources["parallel"] = source
	}
	if v, ok := flags["include-subgroups"].(bool); ok {
		cfg.IncludeSubgroups = v
		cfg.Sources["includeSubgroups"] = source
	}
	if v, ok := flags["subgroup-mode"].(string); ok && v != "" {
		cfg.SubgroupMode = v
		cfg.Sources["subgroupMode"] = source
	}
}

// applySyncConfig merges sync configuration.
func (l *ConfigLoader) applySyncConfig(dst, src *SyncConfig) {
	if src == nil {
		return
	}

	if src.Strategy != "" {
		dst.Strategy = src.Strategy
	}
	if src.MaxRetries > 0 {
		dst.MaxRetries = src.MaxRetries
	}
	if src.Timeout != "" {
		dst.Timeout = src.Timeout
	}
}

// applyBranchConfig merges branch configuration.
func (l *ConfigLoader) applyBranchConfig(dst, src *BranchConfig) {
	if src == nil {
		return
	}

	if len(src.DefaultBranch) > 0 {
		dst.DefaultBranch = src.DefaultBranch
	}
	if len(src.ProtectedBranches) > 0 {
		dst.ProtectedBranches = src.ProtectedBranches
	}
}

// applyFetchConfig merges fetch configuration.
func (l *ConfigLoader) applyFetchConfig(dst, src *FetchConfig) {
	if src == nil {
		return
	}

	if src.AllRemotes {
		dst.AllRemotes = src.AllRemotes
	}
	if src.Prune {
		dst.Prune = src.Prune
	}
}

// applyPullConfig merges pull configuration.
func (l *ConfigLoader) applyPullConfig(dst, src *PullConfig) {
	if src == nil {
		return
	}

	if src.Rebase {
		dst.Rebase = src.Rebase
	}
	if src.FFOnly {
		dst.FFOnly = src.FFOnly
	}
}

// applyPushConfig merges push configuration.
func (l *ConfigLoader) applyPushConfig(dst, src *PushConfig) {
	if src == nil {
		return
	}

	if src.SetUpstream {
		dst.SetUpstream = src.SetUpstream
	}
	// A policy replaces rather than merges: a project that narrows the protected
	// list should not silently inherit the wider one from a profile.
	if src.Policy != nil {
		dst.Policy = src.Policy
	}
}

// GetString retrieves a string value by key from effective config.
func (cfg *EffectiveConfig) GetString(key string) (string, bool) {
	val := reflect.ValueOf(cfg).Elem()
	field := val.FieldByName(key)

	if !field.IsValid() || field.Kind() != reflect.String {
		return "", false
	}

	str := field.String()
	return str, str != ""
}

// GetInt retrieves an integer value by key from effective config.
func (cfg *EffectiveConfig) GetInt(key string) (int, bool) {
	val := reflect.ValueOf(cfg).Elem()
	field := val.FieldByName(key)

	if !field.IsValid() || field.Kind() != reflect.Int {
		return 0, false
	}

	i := int(field.Int())
	return i, i > 0
}

// GetBool retrieves a boolean value by key from effective config.
func (cfg *EffectiveConfig) GetBool(key string) (value, ok bool) {
	val := reflect.ValueOf(cfg).Elem()
	field := val.FieldByName(key)

	if !field.IsValid() || field.Kind() != reflect.Bool {
		return false, false
	}

	return field.Bool(), true
}

// GetSource returns the source for a given config key.
func (cfg *EffectiveConfig) GetSource(key string) string {
	if source, ok := cfg.Sources[key]; ok {
		return source
	}
	return string(SourceDefault)
}

// SetActiveProfileInternal sets the active profile without persisting to disk.
// This is used for temporary profile override (e.g., --profile flag).
func (l *ConfigLoader) SetActiveProfileInternal(profile *Profile) {
	l.activeProfile = profile
}
