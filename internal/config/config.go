// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

// Package config provides configuration loading for forge provider credentials and sync settings.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration.
type Config struct {
	GitHub GitHubConfig `yaml:"github"` //nolint:tagliatelle // "github" is the canonical platform name used in config files
	GitLab GitLabConfig `yaml:"gitlab"` //nolint:tagliatelle // "gitlab" is the canonical platform name used in config files
	Gitea  GiteaConfig  `yaml:"gitea"`
	Sync   SyncConfig   `yaml:"sync"`
}

// GitHubConfig holds GitHub-specific configuration.
type GitHubConfig struct {
	Token   string `yaml:"token"`
	BaseURL string `yaml:"base_url"` //nolint:tagliatelle // snake_case is the established config file convention; changing would break existing configs
}

// GitLabConfig holds GitLab-specific configuration.
type GitLabConfig struct {
	Token   string `yaml:"token"`
	BaseURL string `yaml:"base_url"` //nolint:tagliatelle // snake_case is the established config file convention; changing would break existing configs
}

// GiteaConfig holds Gitea-specific configuration.
type GiteaConfig struct {
	Token   string `yaml:"token"`
	BaseURL string `yaml:"base_url"` //nolint:tagliatelle // snake_case is the established config file convention; changing would break existing configs
}

// SyncConfig holds sync operation defaults.
type SyncConfig struct {
	TargetPath      string `yaml:"target_path"` //nolint:tagliatelle // snake_case is the established config file convention
	Parallel        int    `yaml:"parallel"`
	IncludeArchived bool   `yaml:"include_archived"` //nolint:tagliatelle // snake_case is the established config file convention
	IncludeForks    bool   `yaml:"include_forks"`    //nolint:tagliatelle // snake_case is the established config file convention
	IncludePrivate  bool   `yaml:"include_private"`  //nolint:tagliatelle // snake_case is the established config file convention
}

// DefaultConfig returns a config with default values.
func DefaultConfig() *Config {
	return &Config{
		Sync: SyncConfig{
			TargetPath:      ".",
			Parallel:        4,
			IncludeArchived: false,
			IncludeForks:    false,
			IncludePrivate:  true,
		},
	}
}

// Load loads configuration from file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path) // #nosec G304,G703 -- this API intentionally reads the caller-selected config path.
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Override with environment variables
	cfg.applyEnvOverrides()

	return cfg, nil
}

// LoadDefault loads configuration from default locations.
func LoadDefault() (*Config, error) {
	// Try locations in order
	locations := []string{
		"forge.yaml",
		".forge.yaml",
		filepath.Join(os.Getenv("HOME"), ".config", "gzh-forge", "config.yaml"),
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil { // #nosec G304,G703 -- loc is constructed from known config roots and HOME.
			return Load(loc)
		}
	}

	// Return default config if no file found
	cfg := DefaultConfig()
	cfg.applyEnvOverrides()
	return cfg, nil
}

func (c *Config) applyEnvOverrides() {
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		c.GitHub.Token = token
	}
	if token := os.Getenv("GITLAB_TOKEN"); token != "" {
		c.GitLab.Token = token
	}
	if token := os.Getenv("GITEA_TOKEN"); token != "" {
		c.Gitea.Token = token
	}
}
