// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

var (
	// envVarPattern matches ${VAR_NAME} syntax.
	envVarPattern = regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)\}`)

	// validProfileName matches valid profile names (alphanumeric, dash, underscore).
	validProfileName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

	// validProviders lists supported forge providers.
	validProviders = map[string]bool{
		"github": true,
		"gitlab": true,
		"gitea":  true,
	}

	// validCloneProtos lists supported clone protocols.
	validCloneProtos = map[string]bool{
		"ssh":   true,
		"https": true,
	}

	// validSubgroupModes lists supported subgroup modes.
	validSubgroupModes = map[string]bool{
		"flat":   true,
		"nested": true,
	}

	// validSyncStrategies lists supported sync strategies.
	validSyncStrategies = map[string]bool{
		"pull":   true,
		"reset":  true,
		"skip":   true,
		"rebase": true,
		"clone":  true,
		"fetch":  true,
	}
)

// Validator handles configuration validation and environment variable expansion.
type Validator struct {
	// ExpandEnvVars enables environment variable expansion
	ExpandEnvVars bool
}

// NewValidator creates a new Validator with default settings.
func NewValidator() *Validator {
	return &Validator{
		ExpandEnvVars: true,
	}
}

// ValidateProfile validates a profile configuration.
func (v *Validator) ValidateProfile(p *Profile) error {
	if p == nil {
		return fmt.Errorf("profile is nil")
	}

	// Validate profile name
	if p.Name == "" {
		return fmt.Errorf("profile name is required")
	}
	if !validProfileName.MatchString(p.Name) {
		return fmt.Errorf("invalid profile name '%s': must contain only alphanumeric, dash, or underscore", p.Name)
	}

	// Validate provider if set
	if p.Provider != "" && !validProviders[p.Provider] {
		return fmt.Errorf("invalid provider '%s': must be github, gitlab, or gitea", p.Provider)
	}

	// Validate base URL if set (github.com / GitHub Enterprise / GitLab / Gitea)
	if p.BaseURL != "" && !IsValidBaseURL(p.BaseURL) {
		return fmt.Errorf("invalid baseURL '%s': must be an http(s) URL", p.BaseURL)
	}

	// Validate clone protocol if set
	if p.CloneProto != "" && !validCloneProtos[p.CloneProto] {
		return fmt.Errorf("invalid clone protocol '%s': must be ssh or https", p.CloneProto)
	}

	// Validate SSH port if set
	if p.SSHPort < 0 || p.SSHPort > 65535 {
		return fmt.Errorf("invalid SSH port %d: must be between 0 and 65535", p.SSHPort)
	}

	// Validate parallel count if set
	if p.Parallel < 0 {
		return fmt.Errorf("invalid parallel count %d: must be >= 0", p.Parallel)
	}

	// Validate subgroup mode if set
	if p.SubgroupMode != "" && !validSubgroupModes[p.SubgroupMode] {
		return fmt.Errorf("invalid subgroup mode '%s': must be flat or nested", p.SubgroupMode)
	}

	// Validate sync config if set
	if p.Sync != nil {
		if err := v.ValidateSyncConfig(p.Sync); err != nil {
			return fmt.Errorf("invalid sync config: %w", err)
		}
	}

	return nil
}

// ValidateSyncConfig validates sync configuration.
func (v *Validator) ValidateSyncConfig(s *SyncConfig) error {
	if s == nil {
		return nil
	}

	// Validate strategy if set
	if s.Strategy != "" && !validSyncStrategies[s.Strategy] {
		return fmt.Errorf("invalid sync strategy '%s': must be pull, reset, rebase, skip, clone, or fetch", s.Strategy)
	}

	// Validate max retries if set
	if s.MaxRetries < 0 {
		return fmt.Errorf("invalid max retries %d: must be >= 0", s.MaxRetries)
	}

	return nil
}

// ValidateGlobalConfig validates global configuration.
func (v *Validator) ValidateGlobalConfig(g *GlobalConfig) error {
	if g == nil {
		return fmt.Errorf("global config is nil")
	}

	// Validate active profile name if set
	if g.ActiveProfile != "" && !validProfileName.MatchString(g.ActiveProfile) {
		return fmt.Errorf("invalid active profile name '%s': must contain only alphanumeric, dash, or underscore", g.ActiveProfile)
	}

	return nil
}

// ValidateProjectConfig validates project configuration.
func (v *Validator) ValidateProjectConfig(p *ProjectConfig) error {
	if p == nil {
		return fmt.Errorf("project config is nil")
	}

	// Validate profile name if set
	if p.Profile != "" && !validProfileName.MatchString(p.Profile) {
		return fmt.Errorf("invalid profile name '%s': must contain only alphanumeric, dash, or underscore", p.Profile)
	}

	// Validate sync config if set
	if p.Sync != nil {
		if err := v.ValidateSyncConfig(p.Sync); err != nil {
			return fmt.Errorf("invalid sync config: %w", err)
		}
	}

	return nil
}

// ExpandEnvVarsInProfile expands environment variables in a profile.
// Variables use ${VAR_NAME} syntax.
func (v *Validator) ExpandEnvVarsInProfile(p *Profile) error {
	if !v.ExpandEnvVars || p == nil {
		return nil
	}

	var err error

	// Expand token
	p.Token, err = v.expandString(p.Token)
	if err != nil {
		return fmt.Errorf("failed to expand token: %w", err)
	}

	// Expand base URL
	p.BaseURL, err = v.expandString(p.BaseURL)
	if err != nil {
		return fmt.Errorf("failed to expand baseURL: %w", err)
	}

	return nil
}

// ExpandEnvVarsInGlobalConfig expands environment variables in global config.
func (v *Validator) ExpandEnvVarsInGlobalConfig(g *GlobalConfig) error {
	if !v.ExpandEnvVars || g == nil {
		return nil
	}

	// Expand environment tokens
	for envName, env := range g.Environments {
		var err error
		env.GitHubToken, err = v.expandString(env.GitHubToken)
		if err != nil {
			return fmt.Errorf("failed to expand %s.githubToken: %w", envName, err)
		}

		env.GitLabToken, err = v.expandString(env.GitLabToken)
		if err != nil {
			return fmt.Errorf("failed to expand %s.gitlabToken: %w", envName, err)
		}

		env.GiteaToken, err = v.expandString(env.GiteaToken)
		if err != nil {
			return fmt.Errorf("failed to expand %s.giteaToken: %w", envName, err)
		}

		g.Environments[envName] = env
	}

	return nil
}

// ExpandEnvVarsInConfig expands environment variables in a recursive config.
func (v *Validator) ExpandEnvVarsInConfig(c *Config) error {
	if !v.ExpandEnvVars || c == nil {
		return nil
	}

	var err error

	// Expand provider token/url if set at root level
	c.Token, err = v.expandString(c.Token)
	if err != nil {
		return fmt.Errorf("failed to expand token: %w", err)
	}

	c.BaseURL, err = v.expandString(c.BaseURL)
	if err != nil {
		return fmt.Errorf("failed to expand baseURL: %w", err)
	}

	// Expand profiles
	for name, profile := range c.Profiles {
		if err := v.ExpandEnvVarsInProfile(profile); err != nil {
			return fmt.Errorf("failed to expand profile[%s]: %w", name, err)
		}
	}

	// Expand workspaces
	for name, ws := range c.Workspaces {
		if err := v.ExpandEnvVarsInWorkspace(ws); err != nil {
			return fmt.Errorf("failed to expand workspace[%s]: %w", name, err)
		}
	}

	return nil
}

// ExpandEnvVarsInWorkspace expands environment variables in a workspace.
func (v *Validator) ExpandEnvVarsInWorkspace(ws *Workspace) error {
	if !v.ExpandEnvVars || ws == nil {
		return nil
	}

	var err error

	// Expand source
	if ws.Source != nil {
		ws.Source.Token, err = v.expandString(ws.Source.Token)
		if err != nil {
			return fmt.Errorf("failed to expand source token: %w", err)
		}

		ws.Source.BaseURL, err = v.expandString(ws.Source.BaseURL)
		if err != nil {
			return fmt.Errorf("failed to expand source baseURL: %w", err)
		}
	}

	// Recurse
	for name, nestedWs := range ws.Workspaces {
		if err := v.ExpandEnvVarsInWorkspace(nestedWs); err != nil {
			return fmt.Errorf("failed to expand nested workspace[%s]: %w", name, err)
		}
	}

	return nil
}

// expandString expands environment variables in a string.
// Returns the expanded string or an error if expansion fails.
//
//nolint:unparam // error return is always nil now but kept for future expansion (e.g. strict mode for missing vars)
func (v *Validator) expandString(s string) (string, error) {
	if s == "" {
		return s, nil
	}

	// Find all ${VAR} occurrences
	result := envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		// Extract variable name (remove ${ and })
		varName := match[2 : len(match)-1]

		// Get value from environment
		// Empty env vars are silently accepted — forge commands validate token
		// availability separately when the token is actually needed.
		value := os.Getenv(varName)

		return value
	})

	return result, nil
}

// IsValidProfileName checks if a profile name is valid.
func IsValidProfileName(name string) bool {
	return validProfileName.MatchString(name)
}

// IsValidProvider checks if a provider name is valid.
func IsValidProvider(provider string) bool {
	return validProviders[provider]
}

// IsValidCloneProto checks if a clone protocol is valid.
func IsValidCloneProto(proto string) bool {
	return validCloneProtos[proto]
}

// IsValidBaseURL reports whether s is empty (optional) or a parseable http(s) URL with a host.
// Used to validate forge base URLs (GitHub Enterprise / GitLab / Gitea).
// Prefix-only values such as "https://" are rejected (fail-closed).
func IsValidBaseURL(s string) bool {
	if s = strings.TrimSpace(s); s == "" {
		return true
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return u.Host != ""
}

// IsValidSyncStrategy checks if a sync strategy is valid.
func IsValidSyncStrategy(strategy string) bool {
	return validSyncStrategies[strategy]
}

// SanitizeToken removes credentials from URLs for safe logging.
func SanitizeToken(s string) string {
	// Replace common token patterns in URLs
	s = regexp.MustCompile(`://[^:]+:[^@]+@`).ReplaceAllString(s, "://***:***@")
	s = regexp.MustCompile(`\?.*token=[^&]+`).ReplaceAllString(s, "?token=***")
	s = regexp.MustCompile(`&token=[^&]+`).ReplaceAllString(s, "&token=***")

	return s
}

// NormalizeProvider converts provider name to lowercase.
func NormalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

// ================================================================================
// Recursive Configuration Validation with Workspaces
// ================================================================================

// ValidateConfig validates a recursive hierarchical config.
//
//nolint:gocognit,gocyclo // complex by design: validates all optional fields of a hierarchical config with many sub-sections
func (v *Validator) ValidateConfig(c *Config) error {
	if c == nil {
		return nil // nil config is valid (optional)
	}

	// Validate parent path if specified
	if c.Parent != "" {
		if err := v.ValidateParentPath(c.Parent); err != nil {
			return fmt.Errorf("invalid parent config: %w", err)
		}
	}

	// Validate provider if specified
	if c.Provider != "" && !IsValidProvider(c.Provider) {
		return fmt.Errorf("invalid provider '%s': must be github, gitlab, or gitea", c.Provider)
	}

	// Validate defaults if specified
	if c.Defaults != nil {
		if c.Defaults.Clone != nil {
			if c.Defaults.Clone.Proto != "" && !IsValidCloneProto(c.Defaults.Clone.Proto) {
				return fmt.Errorf("invalid clone protocol '%s': must be ssh or https", c.Defaults.Clone.Proto)
			}
			if c.Defaults.Clone.SSHPort != 0 && (c.Defaults.Clone.SSHPort < 1 || c.Defaults.Clone.SSHPort > 65535) {
				return fmt.Errorf("invalid SSH port %d: must be between 1 and 65535", c.Defaults.Clone.SSHPort)
			}
		}
		if c.Defaults.Sync != nil {
			if c.Defaults.Sync.Parallel < 0 {
				return fmt.Errorf("invalid parallel count %d: must be non-negative", c.Defaults.Sync.Parallel)
			}
		}
	}

	// Validate subgroup mode if specified
	if c.SubgroupMode != "" && c.SubgroupMode != "flat" && c.SubgroupMode != "nested" {
		return fmt.Errorf("invalid subgroup mode '%s': must be flat or nested", c.SubgroupMode)
	}

	// Validate command-specific configs
	if c.Sync != nil {
		if err := v.ValidateSyncConfig(c.Sync); err != nil {
			return fmt.Errorf("sync config validation failed: %w", err)
		}
	}

	// Validate root-level hooks
	if c.Hooks != nil {
		if err := v.ValidateHooks(c.Hooks); err != nil {
			return fmt.Errorf("root hooks validation failed: %w", err)
		}
	}

	// Validate workspaces
	for name, ws := range c.Workspaces {
		if err := v.ValidateWorkspace(ws, name); err != nil {
			return fmt.Errorf("workspace[%s] validation failed: %w", name, err)
		}
	}

	// Validate discovery config
	if c.Discovery != nil {
		if err := v.ValidateDiscoveryConfig(c.Discovery); err != nil {
			return fmt.Errorf("discovery config validation failed: %w", err)
		}
	}

	// Validate global child config mode
	if c.ChildConfigMode != "" {
		if err := v.ValidateChildConfigMode(c.ChildConfigMode); err != nil {
			return fmt.Errorf("config-level childConfigMode validation failed: %w", err)
		}
	}

	return nil
}

// ValidateWorkspace validates a workspace entry.
func (v *Validator) ValidateWorkspace(ws *Workspace, name string) error {
	if ws == nil {
		return fmt.Errorf("workspace is nil")
	}

	if !ws.Access.IsValid() {
		return fmt.Errorf("invalid workspace access %q: must be 'read-write' or 'read-only'", ws.Access)
	}

	// Validate path
	if ws.Path == "" {
		return fmt.Errorf("workspace path is empty")
	}

	// Validate type
	if !ws.Type.IsValid() {
		return fmt.Errorf("invalid workspace type '%s': must be 'forge', 'git', or 'config'", ws.Type)
	}

	// Determine effective type
	effectiveType := ws.Type.Resolve(ws.Source != nil)

	// Validate source is required for forge type
	if effectiveType == WorkspaceTypeForge && ws.Source == nil {
		return fmt.Errorf("source is required for forge workspace")
	}

	// Validate source if provided
	if ws.Source != nil {
		if err := v.ValidateForgeSource(ws.Source); err != nil {
			return fmt.Errorf("forge source validation failed: %w", err)
		}
	}

	// Validate sync config if provided
	if ws.Sync != nil {
		if err := v.ValidateSyncConfig(ws.Sync); err != nil {
			return fmt.Errorf("sync config validation failed: %w", err)
		}
	}

	// Validate parallel count if specified
	if ws.Parallel < 0 {
		return fmt.Errorf("invalid parallel count %d: must be non-negative", ws.Parallel)
	}

	// Validate clone protocol if specified
	if ws.CloneProto != "" && !IsValidCloneProto(ws.CloneProto) {
		return fmt.Errorf("invalid clone protocol '%s': must be ssh or https", ws.CloneProto)
	}

	// Validate SSH port if specified
	if ws.SSHPort != 0 && (ws.SSHPort < 1 || ws.SSHPort > 65535) {
		return fmt.Errorf("invalid SSH port %d: must be between 1 and 65535", ws.SSHPort)
	}

	// Validate child config mode if specified
	if ws.ChildConfigMode != "" && !ws.ChildConfigMode.IsValid() {
		return fmt.Errorf("invalid child config mode '%s': must be 'repositories', 'workspaces', or 'none'", ws.ChildConfigMode)
	}

	// Validate hooks if specified
	if ws.Hooks != nil {
		if err := v.ValidateHooks(ws.Hooks); err != nil {
			return fmt.Errorf("hooks validation failed: %w", err)
		}
	}

	// Validate configLink if specified
	if ws.ConfigLink != "" {
		if err := v.ValidateConfigLink(ws.ConfigLink); err != nil {
			return fmt.Errorf("configLink validation failed: %w", err)
		}
	}

	// Validate nested workspaces recursively
	for nestedName, nestedWs := range ws.Workspaces {
		if err := v.ValidateWorkspace(nestedWs, nestedName); err != nil {
			return fmt.Errorf("nested workspace[%s] validation failed: %w", nestedName, err)
		}
	}

	return nil
}

// ValidateForgeSource validates a forge source configuration.
func (v *Validator) ValidateForgeSource(s *ForgeSource) error {
	if s == nil {
		return nil
	}

	// Validate provider
	if s.Provider == "" {
		return fmt.Errorf("forge source provider is required")
	}
	if !IsValidProvider(s.Provider) {
		return fmt.Errorf("invalid provider '%s': must be github, gitlab, or gitea", s.Provider)
	}

	// Validate org
	if s.Org == "" {
		return fmt.Errorf("forge source org is required")
	}

	// Validate base URL if set
	if s.BaseURL != "" && !IsValidBaseURL(s.BaseURL) {
		return fmt.Errorf("invalid baseURL '%s': must be an http(s) URL", s.BaseURL)
	}

	// Validate subgroup mode if specified
	if s.SubgroupMode != "" && s.SubgroupMode != "flat" && s.SubgroupMode != "nested" {
		return fmt.Errorf("invalid subgroup mode '%s': must be flat or nested", s.SubgroupMode)
	}

	// Warn if includeSubgroups is used with non-gitlab provider
	if s.IncludeSubgroups && s.Provider != "gitlab" {
		// This is a warning, not an error - subgroups only apply to GitLab
		fmt.Fprintf(os.Stderr, "Warning: includeSubgroups is only supported for GitLab provider\n")
	}

	return nil
}

// ValidateDiscoveryConfig validates discovery configuration.
func (v *Validator) ValidateDiscoveryConfig(d *DiscoveryConfig) error {
	if d == nil {
		return nil
	}

	// Validate mode
	if d.Mode != "" && !d.Mode.IsValid() {
		return fmt.Errorf("invalid discovery mode '%s': must be 'explicit', 'auto', or 'hybrid'", d.Mode)
	}

	return nil
}

// ValidateParentPath validates a parent config path.
// Checks:
//   - Path is not empty (already handled by caller)
//   - Path doesn't contain dangerous patterns
//   - Path format is valid (allows ~/, ./, absolute, relative)
func (v *Validator) ValidateParentPath(path string) error {
	if path == "" {
		return nil // Empty is valid (no parent)
	}

	// Validate path characters (basic security check)
	// Allow: alphanumeric, /, -, _, ., ~
	for _, c := range path {
		if !isValidPathChar(c) {
			return fmt.Errorf("parent path contains invalid character: %c", c)
		}
	}

	// Check path doesn't start with dangerous prefixes (except home-relative)
	dangerousPrefixes := []string{"/etc/", "/usr/", "/bin/", "/root/"}
	for _, prefix := range dangerousPrefixes {
		if strings.HasPrefix(path, prefix) {
			return fmt.Errorf("parent path cannot reference system directories: %s", prefix)
		}
	}

	return nil
}

// isValidPathChar checks if a character is valid in a config path.
func isValidPathChar(c rune) bool {
	// Allow alphanumeric
	if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
		return true
	}
	// Allow path separators and common path characters
	switch c {
	case '/', '-', '_', '.', '~':
		return true
	}
	return false
}

// ================================================================================
// Child Config Format Detection
// ================================================================================

// ChildConfigFormat represents the format/origin of a child config file.
type ChildConfigFormat string

const (
	// ChildConfigFormatAutoGenerated indicates the config was auto-generated by gz-git.
	// It can be safely overwritten during sync.
	ChildConfigFormatAutoGenerated ChildConfigFormat = "auto-generated"

	// ChildConfigFormatUserMaintained indicates the config is user-maintained.
	// It should NOT be overwritten without explicit --force flag.
	ChildConfigFormatUserMaintained ChildConfigFormat = "user-maintained"

	// ChildConfigFormatNotFound indicates no config file exists at the path.
	ChildConfigFormatNotFound ChildConfigFormat = "not-found"
)

// AutoGeneratedMarker is the comment marker that identifies auto-generated configs.
const AutoGeneratedMarker = "# AUTO-GENERATED"

// DetectChildConfigFormat checks if a config file is auto-generated or user-maintained.
//
// Detection logic:
//   - File not exists → ChildConfigFormatNotFound
//   - File contains "# AUTO-GENERATED" marker → ChildConfigFormatAutoGenerated
//   - Otherwise → ChildConfigFormatUserMaintained
func DetectChildConfigFormat(path string) (ChildConfigFormat, error) {
	// Check if file exists
	content, err := os.ReadFile(path) // #nosec G304 -- path is the explicit config file selected for validation.
	if err != nil {
		if os.IsNotExist(err) {
			return ChildConfigFormatNotFound, nil
		}
		return "", fmt.Errorf("read config file: %w", err)
	}

	// Check for AUTO-GENERATED marker in first 500 bytes (header area)
	checkLen := min(len(content), 500)
	header := string(content[:checkLen])

	if strings.Contains(header, AutoGeneratedMarker) {
		return ChildConfigFormatAutoGenerated, nil
	}

	return ChildConfigFormatUserMaintained, nil
}

// ValidateChildConfigMode validates a child config mode value.
func (v *Validator) ValidateChildConfigMode(mode ChildConfigMode) error {
	if !mode.IsValid() {
		return fmt.Errorf("invalid child config mode '%s': must be 'repositories', 'workspaces', or 'none'", mode)
	}
	return nil
}

// ================================================================================
// Hooks and ConfigLink Validation
// ================================================================================

// ValidateHooks validates hook commands for security.
// Checks that commands don't contain shell special characters.
func (v *Validator) ValidateHooks(hooks *Hooks) error {
	if hooks == nil {
		return nil
	}

	// Check before hooks
	for i, cmd := range hooks.Before {
		if err := validateHookCommand(cmd); err != nil {
			return fmt.Errorf("before[%d]: %w", i, err)
		}
	}

	// Check after hooks
	for i, cmd := range hooks.After {
		if err := validateHookCommand(cmd); err != nil {
			return fmt.Errorf("after[%d]: %w", i, err)
		}
	}

	return nil
}

// validateHookCommand checks if a single hook command is safe.
func validateHookCommand(cmd string) error {
	if cmd == "" {
		return fmt.Errorf("empty command")
	}

	// Check for shell special characters that could indicate shell injection
	dangerousChars := []string{"|", ">", "<", ">>", "<<", "$(", "`", "&&", "||", ";"}
	for _, char := range dangerousChars {
		if strings.Contains(cmd, char) {
			return fmt.Errorf("command %q contains shell special character %q - use a script file instead", cmd, char)
		}
	}

	return nil
}

// ValidateConfigLink validates a configLink path.
func (v *Validator) ValidateConfigLink(path string) error {
	if path == "" {
		return nil // Empty is valid (no link)
	}

	// Validate path characters (same as parent path validation)
	for _, c := range path {
		if !isValidPathChar(c) {
			return fmt.Errorf("configLink contains invalid character: %c", c)
		}
	}

	// Check for dangerous prefixes
	dangerousPrefixes := []string{"/etc/", "/usr/", "/bin/", "/root/"}
	for _, prefix := range dangerousPrefixes {
		if strings.HasPrefix(path, prefix) {
			return fmt.Errorf("configLink cannot reference system directories: %s", prefix)
		}
	}

	return nil
}
