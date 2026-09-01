// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

// Package config provides configuration management for gz-git, including
// profile-based settings, global defaults, and project-specific overrides.
//
// Configuration follows a 5-layer precedence system (highest to lowest):
//  1. Command flags (e.g., --provider gitlab)
//  2. Project config (.gz-git.yaml in current dir or parent)
//  3. Active profile (~/.config/gz-git/profiles/{active}.yaml)
//  4. Global config (~/.config/gz-git/config.yaml)
//  5. Built-in defaults
package config

import (
	"fmt"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/branch"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/identity"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// ================================================================================
// Config File Meta Information
// ================================================================================

// ConfigKind represents the type of configuration file.
type ConfigKind string

// Hooks represents before/after hook commands for sync operations.
// Hooks are executed without shell interpretation for security (no pipes, redirects, etc.).
type Hooks struct {
	Before []string `yaml:"before,omitempty"` // Commands to run before sync operation
	After  []string `yaml:"after,omitempty"`  // Commands to run after sync operation
}

const (
	// KindRepositories is for simple flat repository lists (repositories array).
	KindRepositories ConfigKind = "repositories"

	// KindWorkspace is for hierarchical workspace configurations (workspaces map).
	KindWorkspace ConfigKind = "workspace"
)

// IsValid returns true if this is a valid config kind.
func (k ConfigKind) IsValid() bool {
	return k == KindRepositories || k == KindWorkspace
}

// ConfigMeta holds common metadata for all config file types.
// This should be at the top of every config file.
//
// Example:
//
//	version: 1
//	kind: repositories
//	metadata:
//	  name: "my-devbox"
//	  team: "platform"
type ConfigMeta struct {
	// Version is the schema version (currently 1)
	Version int `yaml:"version,omitempty"`

	// Kind specifies the config type: "repositories" or "workspace"
	// If omitted, inferred from content:
	//   - Has "workspaces" or "profiles" key → workspace
	//   - Otherwise → repositories (default)
	Kind ConfigKind `yaml:"kind,omitempty"`

	// Metadata holds optional descriptive information
	Metadata *Metadata `yaml:"metadata,omitempty"`
}

// ================================================================================
// Profiles
// ================================================================================

// Profile represents a named configuration profile.
// A profile contains default values for command flags, eliminating
// the need to repeatedly specify the same options.
//
// Example profile file (~/.config/gz-git/profiles/work.yaml):
//
//	name: work
//	provider: gitlab
//	baseURL: https://gitlab.company.com
//	token: ${WORK_GITLAB_TOKEN}
//	cloneProto: ssh
//	sshPort: 2224
//	parallel: 10
//	sync:
//	  strategy: reset
//	  maxRetries: 3
type Profile struct {
	// Name is the profile identifier (e.g., "work", "personal")
	Name string `yaml:"name"`

	// Forge provider settings
	Provider string `yaml:"provider,omitempty"` // github, gitlab, gitea
	BaseURL  string `yaml:"baseURL,omitempty"`  // API endpoint
	Token    string `yaml:"token,omitempty"`    // API token (use ${ENV_VAR})

	// Clone settings
	CloneProto    string `yaml:"cloneProto,omitempty"`    // ssh, https
	SSHPort       int    `yaml:"sshPort,omitempty"`       // Custom SSH port
	SSHKeyPath    string `yaml:"sshKeyPath,omitempty"`    // SSH private key file path (priority)
	SSHKeyContent string `yaml:"sshKeyContent,omitempty"` // SSH private key content (use ${ENV_VAR})

	// Bulk operation settings
	Parallel         int    `yaml:"parallel,omitempty"`         // Parallel job count
	IncludeSubgroups bool   `yaml:"includeSubgroups,omitempty"` // GitLab subgroups
	SubgroupMode     string `yaml:"subgroupMode,omitempty"`     // flat, nested

	// Identity names the machine and agent recorded on automated commits.
	Identity *identity.Identity `yaml:"identity,omitempty"`

	// Command-specific overrides
	Sync   *SyncConfig   `yaml:"sync,omitempty"`
	Branch *BranchConfig `yaml:"branch,omitempty"`
	Fetch  *FetchConfig  `yaml:"fetch,omitempty"`
	Pull   *PullConfig   `yaml:"pull,omitempty"`
	Push   *PushConfig   `yaml:"push,omitempty"`
	Audit  *AuditConfig  `yaml:"audit,omitempty"`
}

// SyncConfig holds sync command defaults.
type SyncConfig struct {
	Strategy       string `yaml:"strategy,omitempty"`       // pull, reset, rebase, skip, clone
	MaxRetries     int    `yaml:"maxRetries,omitempty"`     // Retry count
	Timeout        string `yaml:"timeout,omitempty"`        // Operation timeout
	Recursive      bool   `yaml:"recursive,omitempty"`      // Auto-sync child workspace repos
	CleanupOrphans bool   `yaml:"cleanupOrphans,omitempty"` // Delete local repos not in forge
}

// SelfSyncConfig controls sync behavior for the config directory itself.
// This allows the devbox/orchestrator directory to be synced along with workspaces.
type SelfSyncConfig struct {
	// Enabled controls whether the config directory itself should be synced.
	// Default: false (config directory is not synced)
	Enabled bool `yaml:"enabled,omitempty"`

	// Strategy specifies how to sync the config directory.
	// Values: "fetch" (default, safe), "pull" (with dirty check), "skip"
	// Note: "reset" is not allowed for self-sync to prevent data loss.
	Strategy string `yaml:"strategy,omitempty"`
}

// DefaultsConfig groups all default settings for clarity.
// These settings apply globally unless overridden at workspace level.
type DefaultsConfig struct {
	// Clone settings
	Clone *CloneDefaults `yaml:"clone,omitempty"`

	// Sync settings
	Sync *SyncDefaults `yaml:"sync,omitempty"`

	// Scan settings
	Scan *ScanDefaults `yaml:"scan,omitempty"`

	// Output settings
	Output *OutputDefaults `yaml:"output,omitempty"`

	// Filter settings
	Filter *FilterDefaults `yaml:"filter,omitempty"`
}

// CloneDefaults holds clone-related default settings.
type CloneDefaults struct {
	Proto         string `yaml:"proto,omitempty"`         // ssh, https
	SSHPort       int    `yaml:"sshPort,omitempty"`       // Custom SSH port
	SSHKeyPath    string `yaml:"sshKeyPath,omitempty"`    // SSH private key file path
	SSHKeyContent string `yaml:"sshKeyContent,omitempty"` // SSH private key content (use ${ENV_VAR})
}

// SyncDefaults holds sync-related default settings.
type SyncDefaults struct {
	Strategy   string `yaml:"strategy,omitempty"`   // reset, pull, rebase, fetch, skip, clone
	Parallel   int    `yaml:"parallel,omitempty"`   // Parallel workers
	MaxRetries int    `yaml:"maxRetries,omitempty"` // Retry count
	Timeout    string `yaml:"timeout,omitempty"`    // Operation timeout
}

// ScanDefaults holds scan-related default settings.
type ScanDefaults struct {
	// Depth is the default scan depth for bulk operations. Zero means omitted,
	// so a child value of zero inherits its parent and a top-level zero keeps the
	// CLI default. Positive values opt into a different depth.
	Depth int `yaml:"depth,omitempty"`

	// Exclude holds regex patterns removed from the *local directory scan*
	// that every bulk command runs. It exists because the only way to keep a
	// repository out of `push`/`commit` used to be remembering `--exclude` on
	// every invocation, and forgetting it failed silently: the repository just
	// appeared as another success row.
	//
	// This is deliberately not `defaults.filter.exclude`. That key filters the
	// repository list a forge API returns to `workspace sync` and has never
	// affected the local scan, so reusing it would have made one name mean two
	// scopes. See GetScanExcludePatterns.
	Exclude []string `yaml:"exclude,omitempty"`
}

// OutputDefaults holds output-related default settings.
type OutputDefaults struct {
	Compact bool   `yaml:"compact,omitempty"` // Omit redundant fields in generated configs
	Format  string `yaml:"format,omitempty"`  // Output format (default, compact, json, llm)
}

// FilterDefaults holds filter pattern settings.
type FilterDefaults struct {
	Include []string `yaml:"include,omitempty"` // Include repos matching these patterns
	Exclude []string `yaml:"exclude,omitempty"` // Exclude repos matching these patterns
}

// BranchConfig holds branch command defaults.
// Supports both string shorthand and struct format in YAML:
//   - branch: develop              → BranchConfig{DefaultBranch: ["develop"]}
//   - branch: develop,master       → BranchConfig{DefaultBranch: ["develop", "master"]}
//   - branch:
//     defaultBranch: develop     → standard struct format
type BranchConfig struct {
	DefaultBranch     BranchList `yaml:"defaultBranch,omitempty"`     // main, develop, master (string or list)
	ProtectedBranches []string   `yaml:"protectedBranches,omitempty"` // Branches to protect

	// IntegrationBranch is the ordered list of integration-branch names.
	// Consumers must read it from the repo-root file (LoadRepoRootTaskPattern),
	// not from the 5-layer merger.
	IntegrationBranch BranchList `yaml:"integrationBranch,omitempty"`

	// TaskPattern is the reclaim allow-list (first-* namespace prefix).
	// Load it only via LoadRepoRootTaskPattern — never findConfigUpward.
	TaskPattern BranchList `yaml:"taskPattern,omitempty"`

	// Readiness is a target-owned integration gate. Like TaskPattern, it is
	// preserved here for project-config round trips but must never be inherited
	// or merged from a parent, profile, workspace, or global configuration.
	Readiness *Readiness `yaml:"readiness,omitempty" json:"readiness,omitempty"`

	// Naming templates the branch names that `gz-git branch name` builds, so a
	// task branch is spelled the same way on every machine and by every agent.
	Naming *branch.Naming `yaml:"naming,omitempty"`
}

// UnmarshalYAML implements yaml.Unmarshaler to support string shorthand.
// When YAML contains `branch: develop` (a plain string), it is converted to
// BranchConfig{DefaultBranch: ["develop"]}. This follows the same pattern
// as BranchList.UnmarshalYAML.
func (b *BranchConfig) UnmarshalYAML(unmarshal func(any) error) error {
	// Try string first (shorthand: branch: develop)
	var str string
	if err := unmarshal(&str); err == nil {
		var bl BranchList
		// Reuse BranchList parsing logic for comma-separated support
		if err := bl.UnmarshalYAML(func(v any) error {
			sp, ok := v.(*string)
			if ok {
				*sp = str
				return nil
			}
			return fmt.Errorf("expected *string")
		}); err == nil && len(bl) > 0 {
			b.DefaultBranch = bl
			return nil
		}
		// If BranchList parsing failed but string was valid, handle directly
		if str != "" {
			parts := strings.Split(str, ",")
			result := make([]string, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					result = append(result, p)
				}
			}
			if len(result) > 0 {
				b.DefaultBranch = result
				return nil
			}
		}
		return nil
	}

	// Fallback to struct unmarshal (standard format)
	type branchConfigAlias BranchConfig
	var alias branchConfigAlias
	if err := unmarshal(&alias); err != nil {
		return err
	}
	*b = BranchConfig(alias)
	return nil
}

// FlexBranch is a string type that accepts both string and map YAML formats.
// Use this for per-repo or per-group branch fields where only the branch name(s) matter.
//
// Accepted formats:
//
//	branch: develop              → "develop"
//	branch: develop,master       → "develop,master"
//	branch:
//	  defaultBranch: develop     → "develop"
//	branch:
//	  defaultBranch: [dev, main] → "dev,main"
type FlexBranch string

// UnmarshalYAML implements yaml.Unmarshaler to support both string and map formats.
func (f *FlexBranch) UnmarshalYAML(unmarshal func(any) error) error {
	// Try string first
	var str string
	if err := unmarshal(&str); err == nil {
		*f = FlexBranch(str)
		return nil
	}

	// Try map with defaultBranch key
	var m map[string]any
	if err := unmarshal(&m); err != nil {
		return fmt.Errorf("branch: expected string or map with defaultBranch key")
	}

	val, ok := m["defaultBranch"]
	if !ok {
		*f = ""
		return nil
	}

	switch v := val.(type) {
	case string:
		*f = FlexBranch(v)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				parts = append(parts, s)
			}
		}
		*f = FlexBranch(strings.Join(parts, ","))
	default:
		return fmt.Errorf("branch.defaultBranch: expected string or list, got %T", val)
	}
	return nil
}

// String returns the FlexBranch value as a plain string.
func (f FlexBranch) String() string {
	return string(f)
}

// MarshalYAML implements yaml.Marshaler to output as a plain string.
func (f FlexBranch) MarshalYAML() (any, error) {
	if f == "" {
		return nil, nil //nolint:nilnil // returning nil to yaml marshaler means "omit this field"
	}
	return string(f), nil
}

// BranchList supports both string and list formats for branch specification.
// Examples:
//   - defaultBranch: develop           # single branch
//   - defaultBranch: develop,master    # comma-separated string
//   - defaultBranch: [develop, master] # YAML list
type BranchList []string

// UnmarshalYAML implements yaml.Unmarshaler to support both string and list formats.
func (b *BranchList) UnmarshalYAML(unmarshal func(any) error) error {
	// Try string first
	var str string
	if err := unmarshal(&str); err == nil {
		if str == "" {
			*b = nil
			return nil
		}
		// Split by comma and trim spaces
		parts := strings.Split(str, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				result = append(result, p)
			}
		}
		*b = result
		return nil
	}

	// Try list
	var list []string
	if err := unmarshal(&list); err != nil {
		return err
	}
	*b = list
	return nil
}

// MarshalYAML implements yaml.Marshaler to output as comma-separated string.
func (b BranchList) MarshalYAML() (any, error) {
	if len(b) == 0 {
		return nil, nil //nolint:nilnil // returning nil to yaml marshaler means "omit this field"
	}
	if len(b) == 1 {
		return b[0], nil
	}
	return strings.Join(b, ","), nil
}

// String returns the branch list as comma-separated string.
func (b BranchList) String() string {
	return strings.Join(b, ",")
}

// First returns the first branch in the list, or empty string if empty.
func (b BranchList) First() string {
	if len(b) == 0 {
		return ""
	}
	return b[0]
}

// FetchConfig holds fetch command defaults.
type FetchConfig struct {
	AllRemotes bool `yaml:"allRemotes,omitempty"` // Fetch all remotes
	Prune      bool `yaml:"prune,omitempty"`      // Prune deleted branches
}

// PullConfig holds pull command defaults.
type PullConfig struct {
	Rebase bool `yaml:"rebase,omitempty"` // Use rebase instead of merge
	FFOnly bool `yaml:"ffOnly,omitempty"` // Fast-forward only
}

// PushConfig holds push command defaults.
//
// Example:
//
//	push:
//	  setUpstream: true
//	  policy:
//	    protected: [main, master]
//	    forceMode: lease-only
//	    foreignWork: block
type PushConfig struct {
	SetUpstream bool `yaml:"setUpstream,omitempty"` // Auto set upstream

	// Policy restricts which branches push may write and how. Unset means no
	// branch is protected and only the built-in lease-only force rule applies.
	Policy *repository.PushPolicy `yaml:"policy,omitempty"`
}

// AuditConfig holds `info --audit` defaults.
//
// Example:
//
//	audit:
//	  autofix:
//	    BRANCH_BEHIND_BASE: false        # this project rebases by hand
//	    MERGED_BRANCH_NOT_RECLAIMED: true
type AuditConfig struct {
	// Autofix overrides repository.DefaultAutofixPolicy per finding code. Keys
	// are finding codes (BRANCH_BEHIND_BASE, …); a code that is absent keeps its
	// built-in default, so a project states only where it disagrees.
	//
	// There are no tier names to choose from. A name like "aggressive" would
	// have to be decoded into behavior before anyone could trust it, and it
	// would force every code added later into a bucket someone else picked. A
	// per-code boolean is the same information without the indirection.
	//
	// Enabling a code here still cannot make an irreversible remediation
	// automatic: reversibility is checked after policy, so configuration widens
	// what is permitted but never what is safe.
	Autofix map[string]bool `yaml:"autofix,omitempty"`
}

// GlobalConfig represents ~/.config/gz-git/config.yaml
//
// Example global config file:
//
//	activeProfile: work
//	defaults:
//	  parallel: 5
//	  cloneProto: ssh
//	  format: default
//	environments:
//	  work:
//	    gitlabToken: ${WORK_GITLAB_TOKEN}
//	  personal:
//	    githubToken: ${PERSONAL_GITHUB_TOKEN}
type GlobalConfig struct {
	// ActiveProfile is the default profile to use
	ActiveProfile string `yaml:"activeProfile,omitempty"`

	// Defaults apply to all profiles unless overridden
	Defaults map[string]any `yaml:"defaults,omitempty"`

	// Identity names this machine and, if one is driving, the agent on it.
	// It lives here rather than in a project's .gz-git.yaml because that file
	// is committed, and a shared device name names nothing.
	Identity *identity.Identity `yaml:"identity,omitempty"`

	// Environments define named token sets
	Environments map[string]Environment `yaml:"environments,omitempty"`
}

// Environment represents a named set of API tokens.
type Environment struct {
	GitHubToken string `yaml:"githubToken,omitempty"`
	GitLabToken string `yaml:"gitlabToken,omitempty"`
	GiteaToken  string `yaml:"giteaToken,omitempty"`
}

// ProjectConfig represents .gz-git.yaml in a project directory.
// This file is auto-detected by walking up the directory tree.
//
// Example project config file:
//
//	profile: work
//	sync:
//	  strategy: pull
//	  parallel: 3
//	branch:
//	  defaultBranch: main
//	  protectedBranches: [main, develop, release/*]
//	metadata:
//	  team: backend
//	  repository: https://gitlab.company.com/backend/myproject
type ProjectConfig struct {
	// Profile specifies which profile to use for this project
	Profile string `yaml:"profile,omitempty"`

	// Command-specific overrides
	Sync   *SyncConfig   `yaml:"sync,omitempty"`
	Branch *BranchConfig `yaml:"branch,omitempty"`
	Fetch  *FetchConfig  `yaml:"fetch,omitempty"`
	Pull   *PullConfig   `yaml:"pull,omitempty"`
	Push   *PushConfig   `yaml:"push,omitempty"`
	Audit  *AuditConfig  `yaml:"audit,omitempty"`

	// Metadata is optional project information
	Metadata *ProjectMetadata `yaml:"metadata,omitempty"`
}

// ProjectMetadata holds optional project information.
type ProjectMetadata struct {
	Team       string `yaml:"team,omitempty"`
	Repository string `yaml:"repository,omitempty"`
	Owner      string `yaml:"owner,omitempty"`
}

// ================================================================================
// Recursive Hierarchical Configuration with Workspaces
// ================================================================================

// Config represents a hierarchical configuration that can be nested recursively.
// This is the unified config type used at ALL levels: workstation, workspace,
// project, submodule, etc.
//
// Example usage:
//
//	# ~/.gz-git.yaml (workstation level)
//	profile: polypia
//
//	defaults:
//	  clone:
//	    proto: ssh
//	  sync:
//	    strategy: reset
//	    parallel: 10
//
//	# Inline profiles (no external file needed!)
//	profiles:
//	  polypia:
//	    provider: gitlab
//	    baseURL: https://gitlab.polypia.net
//	    token: ${GITLAB_POLYPIA_TOKEN}
//	  github-personal:
//	    provider: github
//	    token: ${GITHUB_TOKEN}
//
//	workspaces:
//	  devbox:
//	    path: ~/mydevbox
//	    source:
//	      provider: gitlab
//	      org: devbox
//	      includeSubgroups: true
//	      subgroupMode: flat
//	    sync:
//	      strategy: pull
type Config struct {
	// === Parent config reference ===

	// Parent specifies an explicit path to a parent config file.
	// When set, the parent config is loaded and merged (child overrides parent).
	// Supports: absolute paths, home-relative (~), relative paths.
	Parent string `yaml:"parent,omitempty"`

	// === This level's settings ===

	// Profile specifies which profile to use at this level
	Profile string `yaml:"profile,omitempty"`

	// === Inline Profiles ===

	// Profiles defines named profiles inline (no external file needed)
	Profiles map[string]*Profile `yaml:"profiles,omitempty"`

	// === Defaults (grouped settings) ===

	// Defaults groups all default settings
	Defaults *DefaultsConfig `yaml:"defaults,omitempty"`

	// === Forge provider settings ===

	Provider         string `yaml:"provider,omitempty"`         // github, gitlab, gitea
	BaseURL          string `yaml:"baseURL,omitempty"`          // API endpoint
	Token            string `yaml:"token,omitempty"`            // API token (use ${ENV_VAR})
	IncludeSubgroups bool   `yaml:"includeSubgroups,omitempty"` // GitLab subgroups
	SubgroupMode     string `yaml:"subgroupMode,omitempty"`     // flat, nested

	// Default workspace settings
	DefaultWorkspaceType WorkspaceType `yaml:"defaultWorkspaceType,omitempty"` // forge/git/config

	// Self-sync configuration (sync config directory itself)
	SelfSync *SelfSyncConfig `yaml:"selfSync,omitempty"`

	// Command-specific overrides
	Sync   *SyncConfig   `yaml:"sync,omitempty"`
	Branch *BranchConfig `yaml:"branch,omitempty"`
	Fetch  *FetchConfig  `yaml:"fetch,omitempty"`
	Pull   *PullConfig   `yaml:"pull,omitempty"`
	Push   *PushConfig   `yaml:"push,omitempty"`

	// Hooks defines global before/after commands for all workspace syncs
	Hooks *Hooks `yaml:"hooks,omitempty"`

	// Workspaces is a map of named workspace configurations
	Workspaces map[string]*Workspace `yaml:"workspaces,omitempty"`

	// Metadata is optional information about this level
	Metadata *Metadata `yaml:"metadata,omitempty"`

	// Discovery controls how workspaces are discovered
	Discovery *DiscoveryConfig `yaml:"discovery,omitempty"`

	// ChildConfigMode sets the default child config mode for all workspaces.
	// Values: "repositories" (default), "workspaces", "none"
	ChildConfigMode ChildConfigMode `yaml:"childConfigMode,omitempty"`

	// === Internal fields (not serialized) ===

	ParentConfig *Config `yaml:"-"` // Resolved parent config
	ConfigPath   string  `yaml:"-"` // Absolute path to this config file
}

// Workspace represents a named workspace in the hierarchy.
// Each workspace can sync from a forge source or manage existing git repos.
//
// Example:
//
//	devbox:
//	  path: ./devbox
//	  configLink: ./gz-git/mydevbox.yaml
//	  hooks:
//	    before:
//	      - mkdir -p logs
//	    after:
//	      - make setup
//	  source:
//	    provider: gitlab
//	    org: devbox
//	    includeSubgroups: true
//	    subgroupMode: flat
//	  sync:
//	    strategy: pull
//	  workspaces:
//	    subproject:
//	      path: ./subproject
//	      type: git
type Workspace struct {
	// Path is the target directory for this workspace
	// Supports: absolute (/foo/bar), relative (./foo), home-relative (~/foo)
	Path string `yaml:"path"`

	// ConfigLink specifies a config file to symlink into the workspace as .gz-git.yaml
	// Supports: absolute paths, home-relative (~/), relative (./), relative to parent config dir
	// The symlink is created at {workspace.Path}/.gz-git.yaml → {configLink}
	ConfigLink string `yaml:"configLink,omitempty"`

	// Hooks defines before/after commands for this workspace sync
	// Before hooks run before clone/update, After hooks run after successful sync
	Hooks *Hooks `yaml:"hooks,omitempty"`

	// Type specifies what kind of workspace this is
	// Values: "forge" (sync from forge), "git" (single repo), "config" (has nested config)
	// Default: "forge" if Source is set, "git" otherwise
	Type WorkspaceType `yaml:"type,omitempty"`

	// Profile overrides the parent profile for this workspace
	Profile string `yaml:"profile,omitempty"`

	// Access controls whether this machine may publish changes to the
	// workspace. Read-only workspaces are still cloned and pulled, but bulk
	// push and handoff never write to their remotes.
	Access WorkspaceAccess `yaml:"access,omitempty"`

	// === Git Repository Settings (for type=git) ===

	// URL is the git clone URL (required for type=git sync)
	// Supports: HTTPS, SSH, git:// protocols
	URL string `yaml:"url,omitempty"`

	// AdditionalRemotes defines extra git remotes to configure after clone
	// Map of remote name to URL (e.g., {"upstream": "https://github.com/original/repo.git"})
	AdditionalRemotes map[string]string `yaml:"additionalRemotes,omitempty"`

	// === Forge Source (for type=forge) ===

	// Source defines the forge to sync from
	Source *ForgeSource `yaml:"source,omitempty"`

	// === Sync settings ===

	Sync     *SyncConfig `yaml:"sync,omitempty"`
	Parallel int         `yaml:"parallel,omitempty"`

	// === Other settings ===

	CloneProto    string        `yaml:"cloneProto,omitempty"`
	SSHPort       int           `yaml:"sshPort,omitempty"`
	SSHKeyPath    string        `yaml:"sshKeyPath,omitempty"`    // SSH private key file path
	SSHKeyContent string        `yaml:"sshKeyContent,omitempty"` // SSH private key content
	Branch        *BranchConfig `yaml:"branch,omitempty"`
	// Integration is an opt-in controller-owned integration policy. It is read
	// only when a caller explicitly selects this workspace config; it is never
	// inherited by a repository project config.
	Integration *IntegrationControl `yaml:"integration,omitempty"`
	Fetch       *FetchConfig        `yaml:"fetch,omitempty"`
	Pull        *PullConfig         `yaml:"pull,omitempty"`
	Push        *PushConfig         `yaml:"push,omitempty"`

	// Filter patterns (override parent patterns)
	IncludePatterns []string `yaml:"includePatterns,omitempty"` // Include repos matching these patterns
	ExcludePatterns []string `yaml:"excludePatterns,omitempty"` // Exclude repos matching these patterns

	// === Nested workspaces (recursive!) ===

	// Workspaces allows nested workspace definitions
	Workspaces map[string]*Workspace `yaml:"workspaces,omitempty"`

	// === Metadata ===

	Metadata *Metadata `yaml:"metadata,omitempty"`

	// === Child config generation ===

	// ChildConfigMode controls how child config files are generated during sync.
	// Values: "repositories" (default), "workspaces", "none"
	ChildConfigMode ChildConfigMode `yaml:"childConfigMode,omitempty"`
}

// IntegrationControl is deliberately small: it selects a closed preparation
// profile, while branch.integrationBranch and branch.taskPattern on the same
// workspace remain the target and reclaim declarations.
type IntegrationControl struct {
	PrepareProfile string `yaml:"prepareProfile,omitempty"`
}

// WorkspaceAccess describes the remote-write contract for a workspace.
type WorkspaceAccess string

const (
	// WorkspaceAccessReadWrite is the default: normal pull and push behavior.
	WorkspaceAccessReadWrite WorkspaceAccess = "read-write"
	// WorkspaceAccessReadOnly keeps the checkout current while forbidding push.
	WorkspaceAccessReadOnly WorkspaceAccess = "read-only"
)

// IsValid reports whether the configured access mode is supported. Empty is
// the backward-compatible read-write default.
func (a WorkspaceAccess) IsValid() bool {
	return a == "" || a == WorkspaceAccessReadWrite || a == WorkspaceAccessReadOnly
}

// IsReadOnly reports whether remote writes must be suppressed.
func (a WorkspaceAccess) IsReadOnly() bool {
	return a == WorkspaceAccessReadOnly
}

// WorkspaceType represents the type of workspace.
type WorkspaceType string

const (
	// WorkspaceTypeForge indicates the workspace syncs from a forge (GitLab/GitHub/Gitea)
	// This is the default when Source is defined.
	WorkspaceTypeForge WorkspaceType = "forge"

	// WorkspaceTypeGit indicates the workspace is a single git repository
	// No forge sync, just manages an existing repo.
	WorkspaceTypeGit WorkspaceType = "git"

	// WorkspaceTypeConfig indicates the workspace has a nested config file
	// Loads .gz-git.yaml from the workspace path.
	WorkspaceTypeConfig WorkspaceType = "config"
)

// ChildConfigMode controls how child config files are generated during workspace sync.
type ChildConfigMode string

const (
	// ChildConfigModeRepositories generates a flat array format (default).
	// Example: repositories: [{name: repo1, url: ...}, ...].
	ChildConfigModeRepositories ChildConfigMode = "repositories"

	// ChildConfigModeWorkspaces generates a map structure format.
	// Example: workspaces: {repo1: {path: repo1, type: git}, ...}.
	ChildConfigModeWorkspaces ChildConfigMode = "workspaces"

	// ChildConfigModeNone creates directory only, no config file.
	// Useful when child config is manually maintained or not needed.
	ChildConfigModeNone ChildConfigMode = "none"
)

// IsValid returns true if this is a valid child config mode.
func (m ChildConfigMode) IsValid() bool {
	return m == "" || m == ChildConfigModeRepositories || m == ChildConfigModeWorkspaces || m == ChildConfigModeNone
}

// Default returns the default mode if empty.
func (m ChildConfigMode) Default() ChildConfigMode {
	if m == "" {
		return ChildConfigModeRepositories
	}
	return m
}

// IsValid returns true if this is a valid workspace type.
func (t WorkspaceType) IsValid() bool {
	return t == WorkspaceTypeForge || t == WorkspaceTypeGit || t == WorkspaceTypeConfig || t == ""
}

// Resolve returns the effective type based on context.
// If type is empty, it's inferred from Source presence.
func (t WorkspaceType) Resolve(hasSource bool) WorkspaceType {
	if t != "" {
		return t
	}
	if hasSource {
		return WorkspaceTypeForge
	}
	return WorkspaceTypeGit
}

// ForgeSource defines a forge (GitLab/GitHub/Gitea) to sync repositories from.
//
// Example:
//
//	source:
//	  provider: gitlab
//	  org: devbox
//	  baseURL: https://gitlab.polypia.net
//	  includeSubgroups: true
//	  subgroupMode: flat
type ForgeSource struct {
	// Provider is the forge type: gitlab, github, gitea
	Provider string `yaml:"provider"`

	// Org is the organization/group to sync from
	Org string `yaml:"org"`

	// BaseURL is the API endpoint (optional, uses default for provider)
	BaseURL string `yaml:"baseURL,omitempty"`

	// Token overrides the profile token (use ${ENV_VAR} for security)
	Token string `yaml:"token,omitempty"`

	// IncludeSubgroups includes subgroups (GitLab only)
	IncludeSubgroups bool `yaml:"includeSubgroups,omitempty"`

	// SubgroupMode controls directory structure: "flat" or "nested"
	SubgroupMode string `yaml:"subgroupMode,omitempty"`
}

// Metadata holds optional information about a config level.
type Metadata struct {
	Name       string `yaml:"name,omitempty"`       // workstation, mydevbox, project-name
	Type       string `yaml:"type,omitempty"`       // development, production, personal
	Owner      string `yaml:"owner,omitempty"`      // archmagece, team-name
	Team       string `yaml:"team,omitempty"`       // backend, frontend
	Repository string `yaml:"repository,omitempty"` // https://...
}

// DiscoveryConfig controls how children are discovered.
type DiscoveryConfig struct {
	// Mode controls the discovery behavior
	// Values: "explicit" (use children only), "auto" (scan directories),
	//         "hybrid" (use children if defined, otherwise scan)
	// Default: "hybrid"
	Mode DiscoveryMode `yaml:"mode,omitempty"`
}

// DiscoveryMode represents the children discovery mode.
type DiscoveryMode string

const (
	// ExplicitMode only uses children explicitly defined in the config.
	ExplicitMode DiscoveryMode = "explicit"

	// AutoMode scans directories to find children automatically
	// Ignores explicit children definition.
	AutoMode DiscoveryMode = "auto"

	// HybridMode uses children if defined, otherwise scans directories
	// This is the default mode.
	HybridMode DiscoveryMode = "hybrid"
)

// IsValid returns true if this is a valid discovery mode.
func (m DiscoveryMode) IsValid() bool {
	return m == ExplicitMode || m == AutoMode || m == HybridMode
}

// Default returns the default discovery mode.
func (m DiscoveryMode) Default() DiscoveryMode {
	if m == "" {
		return HybridMode
	}
	return m
}

// EffectiveConfig represents the final resolved configuration
// after applying all precedence layers.
type EffectiveConfig struct {
	// Forge provider settings
	Provider string
	BaseURL  string
	Token    string

	// Clone settings
	CloneProto    string
	SSHPort       int
	SSHKeyPath    string
	SSHKeyContent string

	// Bulk operation settings
	Parallel         int
	IncludeSubgroups bool
	SubgroupMode     string

	// Identity, already resolved against the environment and hostname, so a
	// caller can use it without knowing where it came from.
	Identity identity.Identity

	// Command-specific settings
	Sync   SyncConfig
	Branch BranchConfig
	Fetch  FetchConfig
	Pull   PullConfig
	Push   PushConfig
	Audit  AuditConfig

	// Metadata for debugging
	Sources map[string]string // key -> source (e.g., "provider" -> "profile:work")
}

// ConfigSource represents where a config value came from.
type ConfigSource string

// ConfigSource values identify which layer of the 5-layer precedence provided a value.
const (
	SourceFlag     ConfigSource = "flag"
	SourceEnv      ConfigSource = "env"
	SourceKeychain ConfigSource = "keychain"
	SourceProject  ConfigSource = "project"
	SourceProfile  ConfigSource = "profile"
	SourceGlobal   ConfigSource = "global"
	SourceDefault  ConfigSource = "default"
)

// String returns a human-readable source description.
func (s ConfigSource) String() string {
	return string(s)
}

// ================================================================================
// Config Helper Methods
// ================================================================================

// GetCloneProto returns clone protocol from defaults.clone.proto.
func (c *Config) GetCloneProto() string {
	if c.Defaults != nil && c.Defaults.Clone != nil {
		return c.Defaults.Clone.Proto
	}
	return ""
}

// GetSSHPort returns SSH port from defaults.clone.sshPort.
func (c *Config) GetSSHPort() int {
	if c.Defaults != nil && c.Defaults.Clone != nil {
		return c.Defaults.Clone.SSHPort
	}
	return 0
}

// GetSSHKeyPath returns SSH key path from defaults.clone.sshKeyPath.
func (c *Config) GetSSHKeyPath() string {
	if c.Defaults != nil && c.Defaults.Clone != nil {
		return c.Defaults.Clone.SSHKeyPath
	}
	return ""
}

// GetSSHKeyContent returns SSH key content from defaults.clone.sshKeyContent.
func (c *Config) GetSSHKeyContent() string {
	if c.Defaults != nil && c.Defaults.Clone != nil {
		return c.Defaults.Clone.SSHKeyContent
	}
	return ""
}

// GetSyncStrategy returns sync strategy (sync.strategy overrides defaults.sync.strategy).
func (c *Config) GetSyncStrategy() string {
	if c.Sync != nil && c.Sync.Strategy != "" {
		return c.Sync.Strategy
	}
	if c.Defaults != nil && c.Defaults.Sync != nil {
		return c.Defaults.Sync.Strategy
	}
	return ""
}

// GetParallel returns parallel worker count from defaults.sync.parallel.
func (c *Config) GetParallel() int {
	if c.Defaults != nil && c.Defaults.Sync != nil {
		return c.Defaults.Sync.Parallel
	}
	return 0
}

// GetScanDepth returns scan depth from defaults.scan.depth.
func (c *Config) GetScanDepth() int {
	if c.Defaults != nil && c.Defaults.Scan != nil {
		return c.Defaults.Scan.Depth
	}
	return 0
}

// GetCompactOutput returns compact output setting from defaults.output.compact.
func (c *Config) GetCompactOutput() bool {
	if c.Defaults != nil && c.Defaults.Output != nil {
		return c.Defaults.Output.Compact
	}
	return false
}

// GetFormat returns output format from defaults.output.format.
func (c *Config) GetFormat() string {
	if c.Defaults != nil && c.Defaults.Output != nil {
		return c.Defaults.Output.Format
	}
	return ""
}

// GetScanExcludePatterns returns regex patterns from defaults.scan.exclude.
//
// These apply to the local directory scan shared by every bulk command, unlike
// GetExcludePatterns, which applies only to the forge API listing in
// `workspace sync`.
func (c *Config) GetScanExcludePatterns() []string {
	if c.Defaults != nil && c.Defaults.Scan != nil {
		return c.Defaults.Scan.Exclude
	}
	return nil
}

// GetIncludePatterns returns include patterns from defaults.filter.include.
//
// Scope: the forge API repository listing in `workspace sync` only. It does not
// filter the local directory scan that bulk commands run — see
// GetScanExcludePatterns for that.
func (c *Config) GetIncludePatterns() []string {
	if c.Defaults != nil && c.Defaults.Filter != nil {
		return c.Defaults.Filter.Include
	}
	return nil
}

// GetExcludePatterns returns exclude patterns from defaults.filter.exclude.
//
// Scope: the forge API repository listing in `workspace sync` only. Setting it
// does not keep a repository out of `push`, `commit`, or any other bulk
// command — use defaults.scan.exclude for that.
func (c *Config) GetExcludePatterns() []string {
	if c.Defaults != nil && c.Defaults.Filter != nil {
		return c.Defaults.Filter.Exclude
	}
	return nil
}

// GetMaxRetries returns max retries (sync.maxRetries overrides defaults.sync.maxRetries).
func (c *Config) GetMaxRetries() int {
	if c.Sync != nil && c.Sync.MaxRetries != 0 {
		return c.Sync.MaxRetries
	}
	if c.Defaults != nil && c.Defaults.Sync != nil {
		return c.Defaults.Sync.MaxRetries
	}
	return 0
}

// GetTimeout returns timeout (sync.timeout overrides defaults.sync.timeout).
func (c *Config) GetTimeout() string {
	if c.Sync != nil && c.Sync.Timeout != "" {
		return c.Sync.Timeout
	}
	if c.Defaults != nil && c.Defaults.Sync != nil {
		return c.Defaults.Sync.Timeout
	}
	return ""
}
