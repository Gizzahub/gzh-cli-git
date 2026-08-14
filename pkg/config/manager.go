// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// unmarshalFile reads a file and unmarshals it based on its extension.
func unmarshalFile(path string, v any) error {
	data, err := os.ReadFile(path) // #nosec G304 -- path is the manager's explicit config path.
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		if err := json.Unmarshal(data, v); err != nil {
			return fmt.Errorf("failed to parse json file: %w", err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, v); err != nil {
			return fmt.Errorf("failed to parse yaml file: %w", err)
		}
	default:
		return fmt.Errorf("unsupported config file extension: %s", ext)
	}

	return nil
}

// marshalFile marshals data and writes it to a file based on its extension.
func marshalFile(path string, v any, perm os.FileMode) error {
	var data []byte
	var err error

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		data, err = json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal to json: %w", err)
		}
	case ".yaml", ".yml":
		data, err = yaml.Marshal(v)
		if err != nil {
			return fmt.Errorf("failed to marshal to yaml: %w", err)
		}
	default:
		return fmt.Errorf("unsupported config file extension: %s", ext)
	}

	if err := os.WriteFile(path, data, perm); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// Manager handles CRUD operations for profiles and configurations.
type Manager struct {
	paths     *Paths
	validator *Validator
}

// NewManager creates a new configuration manager.
func NewManager() (*Manager, error) {
	paths, err := NewPaths()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize paths: %w", err)
	}

	return &Manager{
		paths:     paths,
		validator: NewValidator(),
	}, nil
}

// Initialize creates the config directory structure with default profile.
func (m *Manager) Initialize() error {
	// Create directories
	if err := m.paths.EnsureDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Create default profile if it doesn't exist
	if !m.paths.ProfileExists(DefaultProfileName) {
		defaultProfile := &Profile{
			Name:       DefaultProfileName,
			Parallel:   10,
			CloneProto: "ssh",
		}

		if err := m.SaveProfile(defaultProfile); err != nil {
			return fmt.Errorf("failed to create default profile: %w", err)
		}
	}

	// Create global config if it doesn't exist
	if _, err := os.Stat(m.paths.GlobalConfigFile); os.IsNotExist(err) {
		globalConfig := &GlobalConfig{
			ActiveProfile: DefaultProfileName,
			Defaults: map[string]any{
				"parallel":   10,
				"cloneProto": "ssh",
			},
		}

		if err := m.SaveGlobalConfig(globalConfig); err != nil {
			return fmt.Errorf("failed to create global config: %w", err)
		}
	}

	return nil
}

// CreateProfile creates a new profile.
func (m *Manager) CreateProfile(profile *Profile) error {
	if profile == nil {
		return fmt.Errorf("profile is nil")
	}

	// Validate profile
	if err := m.validator.ValidateProfile(profile); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Check if profile already exists
	if m.paths.ProfileExists(profile.Name) {
		return fmt.Errorf("profile '%s' already exists", profile.Name)
	}

	// Save profile
	return m.SaveProfile(profile)
}

// SaveProfile saves a profile to disk.
func (m *Manager) SaveProfile(profile *Profile) error {
	if profile == nil {
		return fmt.Errorf("profile is nil")
	}

	// Validate profile
	if err := m.validator.ValidateProfile(profile); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Ensure profiles directory exists
	if err := os.MkdirAll(m.paths.ProfilesDir, 0o700); err != nil {
		return fmt.Errorf("failed to create profiles directory: %w", err)
	}

	// Get path for existing profile, or construct a new one.
	profilePath := m.paths.ProfilePath(profile.Name)
	if profilePath == "" {
		// This is a new profile, create it with the default .yaml extension.
		profilePath = filepath.Join(m.paths.ProfilesDir, profile.Name+".yaml")
	}

	// Write to file with restricted permissions (user read/write only)
	if err := marshalFile(profilePath, profile, 0o600); err != nil {
		return fmt.Errorf("failed to write profile file: %w", err)
	}

	return nil
}

// LoadProfile loads a profile from disk.
func (m *Manager) LoadProfile(name string) (*Profile, error) {
	if name == "" {
		return nil, fmt.Errorf("profile name is required")
	}

	profilePath := m.paths.ProfilePath(name)

	// Check if profile exists
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("profile '%s' not found", name)
	}

	// Unmarshal file based on extension
	var profile Profile
	if err := unmarshalFile(profilePath, &profile); err != nil {
		return nil, fmt.Errorf("failed to parse profile file: %w", err)
	}

	// Expand environment variables
	if err := m.validator.ExpandEnvVarsInProfile(&profile); err != nil {
		return nil, fmt.Errorf("failed to expand environment variables: %w", err)
	}

	// Validate profile
	if err := m.validator.ValidateProfile(&profile); err != nil {
		return nil, fmt.Errorf("profile validation failed: %w", err)
	}

	return &profile, nil
}

// DeleteProfile deletes a profile.
func (m *Manager) DeleteProfile(name string) error {
	if name == "" {
		return fmt.Errorf("profile name is required")
	}

	// Prevent deletion of default profile
	if name == DefaultProfileName {
		return fmt.Errorf("cannot delete default profile")
	}

	profilePath := m.paths.ProfilePath(name)

	// Check if profile exists
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		return fmt.Errorf("profile '%s' not found", name)
	}

	// Delete file
	if err := os.Remove(profilePath); err != nil {
		return fmt.Errorf("failed to delete profile file: %w", err)
	}

	// If this was the active profile, reset to default
	activeProfile, err := m.paths.GetActiveProfile()
	if err != nil {
		// Failure to read the active profile is non-fatal here; treat as if no active profile is set.
		activeProfile = ""
	}
	if activeProfile == name {
		if err := m.paths.SetActiveProfile(DefaultProfileName); err != nil {
			// Log warning but don't fail
			fmt.Fprintf(os.Stderr, "Warning: failed to reset active profile to default: %v\n", err)
		}
	}

	return nil
}

// ListProfiles returns all available profile names.
func (m *Manager) ListProfiles() ([]string, error) {
	return m.paths.ListProfiles()
}

// ProfileExists checks if a profile exists.
func (m *Manager) ProfileExists(name string) bool {
	return m.paths.ProfileExists(name)
}

// GetActiveProfile returns the active profile name.
func (m *Manager) GetActiveProfile() (string, error) {
	profile, err := m.paths.GetActiveProfile()
	if err != nil {
		return "", err
	}

	// Default to "default" if not set
	if profile == "" {
		profile = DefaultProfileName
	}

	return profile, nil
}

// SetActiveProfile sets the active profile.
func (m *Manager) SetActiveProfile(name string) error {
	if name == "" {
		return fmt.Errorf("profile name is required")
	}

	// Check if profile exists
	if !m.paths.ProfileExists(name) {
		return fmt.Errorf("profile '%s' not found", name)
	}

	return m.paths.SetActiveProfile(name)
}

// SaveGlobalConfig saves the global configuration.
func (m *Manager) SaveGlobalConfig(config *GlobalConfig) error {
	if config == nil {
		return fmt.Errorf("global config is nil")
	}

	// Validate global config
	if err := m.validator.ValidateGlobalConfig(config); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Ensure config directory exists
	if err := os.MkdirAll(m.paths.ConfigDir, 0o700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Get path for existing global config, or construct a new one.
	configPath := m.paths.GlobalConfigFile
	if configPath == "" {
		// This is a new global config, create it with the default .yaml extension.
		configPath = filepath.Join(m.paths.ConfigDir, GlobalConfigFileName+".yaml")
	}

	// Write to file
	if err := marshalFile(configPath, config, 0o600); err != nil {
		return fmt.Errorf("failed to write global config file: %w", err)
	}

	return nil
}

// LoadGlobalConfig loads the global configuration.
func (m *Manager) LoadGlobalConfig() (*GlobalConfig, error) {
	// Check if config file exists
	if _, err := os.Stat(m.paths.GlobalConfigFile); os.IsNotExist(err) {
		// Return default config if file doesn't exist
		return &GlobalConfig{
			ActiveProfile: DefaultProfileName,
			Defaults: map[string]any{
				"parallel":   10,
				"cloneProto": "ssh",
			},
		}, nil
	}

	// Unmarshal YAML or JSON
	var config GlobalConfig
	if err := unmarshalFile(m.paths.GlobalConfigFile, &config); err != nil {
		return nil, fmt.Errorf("failed to parse global config file: %w", err)
	}

	// Expand environment variables
	if err := m.validator.ExpandEnvVarsInGlobalConfig(&config); err != nil {
		return nil, fmt.Errorf("failed to expand environment variables: %w", err)
	}

	// Validate config
	if err := m.validator.ValidateGlobalConfig(&config); err != nil {
		return nil, fmt.Errorf("global config validation failed: %w", err)
	}

	return &config, nil
}

// LoadProjectConfig loads project-specific configuration.
func (m *Manager) LoadProjectConfig() (*ProjectConfig, error) {
	// Find project config file
	configPath, err := FindProjectConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to find project config: %w", err)
	}

	// No project config found (not an error)
	if configPath == "" {
		return nil, nil //nolint:nilnil // nil config is the documented "not found" sentinel for this function
	}

	// Unmarshal YAML or JSON
	var config ProjectConfig
	if err := unmarshalFile(configPath, &config); err != nil {
		return nil, fmt.Errorf("failed to parse project config file: %w", err)
	}

	// Validate config
	if err := m.validator.ValidateProjectConfig(&config); err != nil {
		return nil, fmt.Errorf("project config validation failed: %w", err)
	}

	return &config, nil
}

// ================================================================================
// Recursive Hierarchical Configuration (NEW!)
// ================================================================================

// LoadConfigRecursiveFromPath loads a recursive config from the specified path.
// This is a wrapper around LoadConfigRecursive with manager validation.
func (m *Manager) LoadConfigRecursiveFromPath(path, configFile string) (*Config, error) {
	// Use the standalone LoadConfigRecursive function
	config, err := LoadConfigRecursive(path, configFile)
	if err != nil {
		return nil, err
	}

	// Validate the loaded config
	if err := m.validator.ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}

// SaveConfig saves a recursive config to the specified path.
func (m *Manager) SaveConfig(path, configFile string, config *Config) error {
	if config == nil {
		return fmt.Errorf("config is nil")
	}

	// Validate config
	if err := m.validator.ValidateConfig(config); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Write to file
	configPath := fmt.Sprintf("%s/%s", path, configFile)
	if err := marshalFile(configPath, config, 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// FindNearestConfig finds the nearest config file by walking up from the current directory.
func (m *Manager) FindNearestConfig(configFile string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	return FindConfigRecursive(cwd, configFile)
}

// SaveProjectConfig saves project configuration to current directory.
func (m *Manager) SaveProjectConfig(config *ProjectConfig) error {
	if config == nil {
		return fmt.Errorf("project config is nil")
	}

	// Validate config
	if err := m.validator.ValidateProjectConfig(config); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Get current directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Write to current directory, ensuring it has a .yaml extension.
	// The constant from paths.go is extension-agnostic.
	configPath := filepath.Join(cwd, ProjectConfigFileName+".yaml")
	if err := marshalFile(configPath, config, 0o644); err != nil {
		return fmt.Errorf("failed to write project config file: %w", err)
	}

	return nil
}
