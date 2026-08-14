// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package workspacecli

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
)

// ConfigType represents the detected configuration type.
type ConfigType string

const (
	// ConfigTypeWorkspace is workspace/repositories config (.gz-git.yaml).
	ConfigTypeWorkspace ConfigType = "workspace"
	// ConfigTypeClone is clone config with named groups (repos-config.yaml).
	ConfigTypeClone ConfigType = "clone"
	// ConfigTypeUnknown is when the config type cannot be determined.
	ConfigTypeUnknown ConfigType = "unknown"
)

// Clone config kind values.
const (
	CloneKindGroups = "groups"
	CloneKindFlat   = "flat"
	CloneKindGroup  = "group" // deprecated alias for groups
)

// ValidCloneStrategies contains valid strategy values for clone config.
var ValidCloneStrategies = []string{"skip", "pull", "reset", "rebase", "fetch"}

// ValidationResult holds the result of config validation.
type ValidationResult struct {
	ConfigType  ConfigType // Detected config type
	Errors      []string   // Critical errors that must be fixed
	Warnings    []string   // Deprecated or non-standard usage
	Suggestions []string   // Recommendations for improvement
}

// IsValid returns true if there are no errors.
func (r *ValidationResult) IsValid() bool {
	return len(r.Errors) == 0
}

// HasIssues returns true if there are any errors, warnings, or suggestions.
func (r *ValidationResult) HasIssues() bool {
	return len(r.Errors) > 0 || len(r.Warnings) > 0 || len(r.Suggestions) > 0
}

func (f CommandFactory) newValidateCmd() *cobra.Command {
	var configPath string
	var verbose bool

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate workspace config file",
		Long: cliutil.QuickStartHelp(`  # Validate config file
  gz-git workspace validate -c myworkspace.yaml

  # Auto-detect config in current directory
  gz-git workspace validate

  # Verbose output with suggestions
  gz-git workspace validate --verbose`),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			// Auto-detect config file if not specified
			if configPath == "" {
				detected, err := detectConfigFile(".")
				if err != nil {
					return fmt.Errorf("no config file specified and auto-detection failed: %w", err)
				}
				configPath = detected
				fmt.Fprintf(out, "Using config: %s\n\n", configPath)
			}

			// Run validation
			result, err := validateConfigFile(configPath)
			if err != nil {
				return fmt.Errorf("failed to read config: %w", err)
			}

			// Print results
			printValidationResult(out, result, verbose)

			// For workspace config, also try to load with existing loader for additional validation
			// Skip this for clone config as it uses a different loader
			if result.ConfigType == ConfigTypeWorkspace {
				loader := FileSpecLoader{}
				_, loadErr := loader.Load(ctx, configPath)
				if loadErr != nil {
					fmt.Fprintf(out, "\n✗ Load error: %s\n", loadErr)
					return fmt.Errorf("validation failed")
				}
			}

			if !result.IsValid() {
				return fmt.Errorf("validation failed with %d error(s)", len(result.Errors))
			}

			fmt.Fprintf(out, "\n✓ Configuration is valid: %s\n", configPath)
			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to config file (auto-detects "+DefaultConfigFile+")")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show all suggestions and recommendations")

	return cmd
}

// validateConfigFile performs comprehensive validation on a config file.
func validateConfigFile(path string) (*ValidationResult, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- path is the explicit config file selected for validation.
	if err != nil {
		return nil, err
	}

	var rawConfig map[string]any
	if err := yaml.Unmarshal(content, &rawConfig); err != nil {
		return &ValidationResult{
			Errors: []string{fmt.Sprintf("YAML parse error: %s", err)},
		}, nil
	}

	result := &ValidationResult{}

	// Detect config type
	configType := detectConfigType(rawConfig)
	result.ConfigType = configType

	switch configType {
	case ConfigTypeClone:
		validateCloneConfig(rawConfig, result)
	case ConfigTypeWorkspace:
		validateWorkspaceConfig(rawConfig, result)
	case ConfigTypeUnknown:
		// Try to determine what the user intended
		result.Errors = append(result.Errors,
			"cannot determine config type: add 'kind' field to specify")
		result.Suggestions = append(result.Suggestions,
			"For workspace config: add 'kind: workspace' or 'kind: repositories'")
		result.Suggestions = append(result.Suggestions,
			"For clone config: add 'kind: groups' or 'kind: flat'")
	}

	return result, nil
}

// detectConfigType determines whether the config is workspace or clone type.
func detectConfigType(config map[string]any) ConfigType {
	kind, hasKind := config["kind"].(string)

	// Check explicit kind
	if hasKind {
		switch kind {
		case "workspace", "workspaces", "repositories", "repository":
			return ConfigTypeWorkspace
		case CloneKindGroups, CloneKindFlat, CloneKindGroup:
			return ConfigTypeClone
		}
	}

	// Heuristic detection
	// Clone config: has named groups (keys with target/repositories structure)
	// Workspace config: has kind, workspaces, or repositories at top level

	if _, hasWorkspaces := config["workspaces"]; hasWorkspaces {
		return ConfigTypeWorkspace
	}

	// Check if there are named groups (clone config pattern)
	for key, value := range config {
		// Skip known global keys
		if isGlobalConfigKey(key) {
			continue
		}
		// Check if value looks like a group (has target and/or repositories)
		if group, ok := value.(map[string]any); ok {
			if _, hasTarget := group["target"]; hasTarget {
				return ConfigTypeClone
			}
			if _, hasRepos := group["repositories"]; hasRepos {
				return ConfigTypeClone
			}
		}
	}

	// If has top-level repositories array, it's workspace config
	if _, hasRepos := config["repositories"]; hasRepos {
		return ConfigTypeWorkspace
	}

	return ConfigTypeUnknown
}

// isGlobalConfigKey returns true if the key is a known global config key.
func isGlobalConfigKey(key string) bool {
	globalKeys := []string{
		"version", "kind", "metadata", "strategy", "parallel",
		"cloneProto", "sshPort", "structure", "target", "repositories",
		"workspaces", "profiles", "update", "sync",
	}
	return slices.Contains(globalKeys, key)
}

// validateWorkspaceConfig validates workspace/repositories config.
func validateWorkspaceConfig(config map[string]any, result *ValidationResult) {
	// Validate kind
	validateKind(config, result)

	// Validate version
	validateVersion(config, result)

	// Validate strategy
	validateStrategy(config, result)

	// Validate structure based on kind
	validateStructure(config, result)

	// Validate branch configuration
	validateBranchConfig(config, result)

	// Validate repositories/workspaces entries
	validateEntries(config, result)
}

// validateCloneConfig validates clone config with named groups.
func validateCloneConfig(config map[string]any, result *ValidationResult) {
	// Validate kind
	validateCloneKind(config, result)

	// Validate version
	validateVersion(config, result)

	// Validate strategy (clone uses different valid values)
	validateCloneStrategy(config, result)

	// Validate groups
	validateCloneGroups(config, result)
}

// validateCloneKind checks the kind field for clone config.
func validateCloneKind(config map[string]any, result *ValidationResult) {
	kind, hasKind := config["kind"]
	if !hasKind {
		result.Suggestions = append(result.Suggestions,
			"Add 'kind: groups' for named groups config or 'kind: flat' for simple list")
		return
	}

	kindStr, ok := kind.(string)
	if !ok {
		result.Errors = append(result.Errors, "'kind' must be a string")
		return
	}

	switch kindStr {
	case CloneKindGroups, CloneKindFlat:
		// Valid kinds - OK
	case CloneKindGroup:
		result.Warnings = append(result.Warnings,
			"'kind: group' is deprecated, use 'kind: groups' instead")
	default:
		result.Errors = append(result.Errors,
			fmt.Sprintf("invalid kind '%s': must be 'groups' or 'flat'", kindStr))
	}
}

// validateCloneStrategy checks the strategy field for clone config.
func validateCloneStrategy(config map[string]any, result *ValidationResult) {
	strategy, ok := config["strategy"]
	if !ok {
		result.Suggestions = append(result.Suggestions,
			"Add 'strategy: pull' (or skip, reset, rebase, fetch) to specify update behavior")
		return
	}

	strategyStr, ok := strategy.(string)
	if !ok {
		result.Errors = append(result.Errors, "'strategy' must be a string")
		return
	}

	valid := slices.Contains(ValidCloneStrategies, strategyStr)
	if !valid {
		result.Errors = append(result.Errors,
			fmt.Sprintf("invalid strategy '%s': must be one of %v", strategyStr, ValidCloneStrategies))
	}
}

// validateCloneGroups validates named groups in clone config.
func validateCloneGroups(config map[string]any, result *ValidationResult) {
	groupCount := 0

	for key, value := range config {
		// Skip global keys
		if isGlobalConfigKey(key) {
			continue
		}

		group, ok := value.(map[string]any)
		if !ok {
			continue
		}

		groupCount++

		// Validate group structure
		validateCloneGroup(key, group, result)
	}

	if groupCount == 0 {
		// Check if it's flat format
		if _, hasRepos := config["repositories"]; hasRepos {
			// Flat format - validate repositories
			if repos, ok := config["repositories"].([]any); ok {
				for i, repo := range repos {
					validateCloneRepoEntry(i, repo, result)
				}
			}
		} else {
			result.Suggestions = append(result.Suggestions,
				"No groups or repositories defined in config")
		}
	}
}

// validateCloneGroup validates a single named group.
func validateCloneGroup(name string, group map[string]any, result *ValidationResult) {
	// Check for target
	if _, hasTarget := group["target"]; !hasTarget {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("group '%s': missing 'target' field (will use current directory)", name))
	}

	// Check for repositories
	repos, hasRepos := group["repositories"]
	if !hasRepos {
		result.Errors = append(result.Errors,
			fmt.Sprintf("group '%s': missing 'repositories' field", name))
		return
	}

	repoList, ok := repos.([]any)
	if !ok {
		result.Errors = append(result.Errors,
			fmt.Sprintf("group '%s': 'repositories' must be an array", name))
		return
	}

	if len(repoList) == 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("group '%s': 'repositories' is empty", name))
	}

	// Validate each repository entry
	for i, repo := range repoList {
		validateCloneRepoEntryInGroup(name, i, repo, result)
	}

	// Check strategy override
	if strategy, ok := group["strategy"].(string); ok {
		valid := slices.Contains(ValidCloneStrategies, strategy)
		if !valid {
			result.Errors = append(result.Errors,
				fmt.Sprintf("group '%s': invalid strategy '%s'", name, strategy))
		}
	}
}

// validateCloneRepoEntry validates a repository entry in flat format.
func validateCloneRepoEntry(index int, entry any, result *ValidationResult) {
	// String format (just URL)
	if url, ok := entry.(string); ok {
		if url == "" {
			result.Errors = append(result.Errors,
				fmt.Sprintf("repositories[%d]: empty URL", index))
		}
		return
	}

	// Map format
	repoMap, ok := entry.(map[string]any)
	if !ok {
		result.Errors = append(result.Errors,
			fmt.Sprintf("repositories[%d]: invalid format, must be string or map", index))
		return
	}

	// Check for URL
	if url, hasURL := repoMap["url"]; !hasURL || url == "" {
		result.Errors = append(result.Errors,
			fmt.Sprintf("repositories[%d]: missing or empty 'url' field", index))
	}
}

// validateCloneRepoEntryInGroup validates a repository entry within a group.
func validateCloneRepoEntryInGroup(groupName string, index int, entry any, result *ValidationResult) {
	// String format (just URL)
	if url, ok := entry.(string); ok {
		if url == "" {
			result.Errors = append(result.Errors,
				fmt.Sprintf("group '%s' repositories[%d]: empty URL", groupName, index))
		}
		return
	}

	// Map format
	repoMap, ok := entry.(map[string]any)
	if !ok {
		result.Errors = append(result.Errors,
			fmt.Sprintf("group '%s' repositories[%d]: invalid format, must be string or map", groupName, index))
		return
	}

	// Check for URL
	if url, hasURL := repoMap["url"]; !hasURL || url == "" {
		result.Errors = append(result.Errors,
			fmt.Sprintf("group '%s' repositories[%d]: missing or empty 'url' field", groupName, index))
	}
}

// validateKind checks the kind field.
func validateKind(config map[string]any, result *ValidationResult) {
	kind, ok := config["kind"]
	if !ok {
		result.Errors = append(result.Errors,
			"missing 'kind' field: must be 'workspace' or 'repositories'")
		result.Suggestions = append(result.Suggestions,
			"Add 'kind: workspace' for hierarchical config or 'kind: repositories' for flat list")
		return
	}

	kindStr, ok := kind.(string)
	if !ok {
		result.Errors = append(result.Errors, "'kind' must be a string")
		return
	}

	switch kindStr {
	case "workspace", "repositories":
		// Valid kinds - OK
	case "workspaces":
		// Deprecated alias - warn but accept
		result.Warnings = append(result.Warnings,
			"'kind: workspaces' is deprecated, use 'kind: workspace' instead")
	case "repository":
		// Deprecated alias - warn but accept
		result.Warnings = append(result.Warnings,
			"'kind: repository' is deprecated, use 'kind: repositories' instead")
	default:
		result.Errors = append(result.Errors,
			fmt.Sprintf("invalid kind '%s': must be 'workspace' or 'repositories'", kindStr))
	}
}

// validateVersion checks the version field.
func validateVersion(config map[string]any, result *ValidationResult) {
	version, ok := config["version"]
	if !ok {
		result.Suggestions = append(result.Suggestions,
			"Add 'version: 1' for future compatibility")
		return
	}

	switch v := version.(type) {
	case int:
		if v != 1 {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("unknown version %d, only version 1 is supported", v))
		}
	case float64:
		if v != 1.0 {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("unknown version %.0f, only version 1 is supported", v))
		}
	default:
		result.Warnings = append(result.Warnings, "'version' should be an integer")
	}
}

// validateBranchConfig checks the branch configuration.
// Branch should be explicitly specified to avoid ambiguity.
func validateBranchConfig(config map[string]any, result *ValidationResult) {
	// Check for branch at config level (defaults.branch or branch)
	hasBranch := false

	// Check defaults.branch
	if defaults, ok := config["defaults"].(map[string]any); ok {
		if _, hasBranchInDefaults := defaults["branch"]; hasBranchInDefaults {
			hasBranch = true
		}
	}

	// Check top-level branch
	if _, hasTopBranch := config["branch"]; hasTopBranch {
		hasBranch = true
	}

	if !hasBranch {
		result.Warnings = append(result.Warnings,
			"no 'branch' configuration found - recommend specifying 'branch.defaultBranch' to avoid ambiguity")
		result.Suggestions = append(result.Suggestions,
			"Add 'branch: { defaultBranch: develop,master }' in defaults or at top level")
	}
}

// validateStrategy checks the strategy field.
func validateStrategy(config map[string]any, result *ValidationResult) {
	strategy, ok := config["strategy"]
	if !ok {
		result.Suggestions = append(result.Suggestions,
			"Add 'strategy: pull' (or reset, rebase, fetch, skip) to specify sync behavior")
		return
	}

	strategyStr, ok := strategy.(string)
	if !ok {
		result.Errors = append(result.Errors, "'strategy' must be a string")
		return
	}

	if !isValidStrategy(strategyStr) {
		result.Errors = append(result.Errors,
			fmt.Sprintf("invalid strategy '%s': must be one of %v", strategyStr, ValidStrategies))
	}
}

// validateStructure checks that the config has the right structure for its kind.
func validateStructure(config map[string]any, result *ValidationResult) {
	kind, ok := config["kind"].(string)
	if !ok {
		kind = ""
	}

	hasRepositories := config["repositories"] != nil
	hasWorkspaces := config["workspaces"] != nil

	switch kind {
	case "workspace", "workspaces":
		if hasRepositories && !hasWorkspaces {
			result.Warnings = append(result.Warnings,
				"kind is 'workspace' but config has 'repositories' instead of 'workspaces'")
			result.Suggestions = append(result.Suggestions,
				"Change 'repositories:' to 'workspaces:' or set 'kind: repositories'")
		}
	case "repositories", "repository":
		if hasWorkspaces && !hasRepositories {
			result.Warnings = append(result.Warnings,
				"kind is 'repositories' but config has 'workspaces' instead of 'repositories'")
			result.Suggestions = append(result.Suggestions,
				"Change 'workspaces:' to 'repositories:' or set 'kind: workspace'")
		}
	}

	if !hasRepositories && !hasWorkspaces {
		result.Suggestions = append(result.Suggestions,
			"Config has no 'repositories' or 'workspaces' defined")
	}
}

// validateEntries checks individual repository/workspace entries.
func validateEntries(config map[string]any, result *ValidationResult) {
	// Check repositories array
	if repos, ok := config["repositories"].([]any); ok {
		for i, repo := range repos {
			validateRepoEntry(i, repo, result)
		}
	}

	// Check workspaces map
	if workspaces, ok := config["workspaces"].(map[string]any); ok {
		for name, ws := range workspaces {
			validateWorkspaceEntry(name, ws, result)
		}
	}
}

// validateRepoEntry validates a single repository entry.
func validateRepoEntry(index int, entry any, result *ValidationResult) {
	// String format (just URL)
	if _, ok := entry.(string); ok {
		return // Valid simple format
	}

	// Map format
	repoMap, ok := entry.(map[string]any)
	if !ok {
		result.Errors = append(result.Errors,
			fmt.Sprintf("repositories[%d]: invalid format, must be string or map", index))
		return
	}

	// Check for URL
	if _, hasURL := repoMap["url"]; !hasURL {
		// Check if it's a scanned entry (may have path but no url)
		if _, hasPath := repoMap["path"]; !hasPath {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("repositories[%d]: missing 'url' field", index))
		}
	}
}

// validateWorkspaceEntry validates a single workspace entry.
func validateWorkspaceEntry(name string, entry any, result *ValidationResult) {
	wsMap, ok := entry.(map[string]any)
	if !ok {
		result.Errors = append(result.Errors,
			fmt.Sprintf("workspaces.%s: invalid format, must be a map", name))
		return
	}

	// Check for path or url
	_, hasPath := wsMap["path"]
	_, hasURL := wsMap["url"]
	if !hasPath && !hasURL {
		result.Suggestions = append(result.Suggestions,
			fmt.Sprintf("workspaces.%s: consider adding 'path' or 'url' field", name))
	}

	// Check type field
	if wsType, ok := wsMap["type"].(string); ok {
		validTypes := []string{"git", "config", "forge"}
		isValid := slices.Contains(validTypes, wsType)
		if !isValid {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("workspaces.%s: unknown type '%s', expected git, config, or forge", name, wsType))
		}
	}
}

// printValidationResult prints the validation result to the writer.
func printValidationResult(out io.Writer, result *ValidationResult, verbose bool) {
	// Print detected config type
	if result.ConfigType != "" && result.ConfigType != ConfigTypeUnknown {
		fmt.Fprintf(out, "Config type: %s\n\n", result.ConfigType)
	}

	// Print errors
	if len(result.Errors) > 0 {
		fmt.Fprintln(out, "Errors:")
		for _, err := range result.Errors {
			fmt.Fprintf(out, "  ✗ %s\n", err)
		}
		fmt.Fprintln(out)
	}

	// Print warnings
	if len(result.Warnings) > 0 {
		fmt.Fprintln(out, "Warnings:")
		for _, warn := range result.Warnings {
			fmt.Fprintf(out, "  ⚠ %s\n", warn)
		}
		fmt.Fprintln(out)
	}

	// Print suggestions (only in verbose mode or if there are issues)
	if len(result.Suggestions) > 0 && (verbose || len(result.Errors) > 0) {
		fmt.Fprintln(out, "Suggestions:")
		for _, sug := range result.Suggestions {
			fmt.Fprintf(out, "  → %s\n", sug)
		}
		fmt.Fprintln(out)
	}

	// Summary
	var parts []string
	if len(result.Errors) > 0 {
		parts = append(parts, fmt.Sprintf("%d error(s)", len(result.Errors)))
	}
	if len(result.Warnings) > 0 {
		parts = append(parts, fmt.Sprintf("%d warning(s)", len(result.Warnings)))
	}
	if verbose && len(result.Suggestions) > 0 {
		parts = append(parts, fmt.Sprintf("%d suggestion(s)", len(result.Suggestions)))
	}
	if len(parts) == 0 {
		fmt.Fprintln(out, "No issues found.")
	} else {
		fmt.Fprintf(out, "Found: %s\n", strings.Join(parts, ", "))
	}
}
