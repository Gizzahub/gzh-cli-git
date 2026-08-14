package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	configpkg "github.com/gizzahub/gzh-cli-gitforge/pkg/config"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

type CloneConfigKind string

const (
	// CloneKindGroups is the named groups format (recommended).
	CloneKindGroups CloneConfigKind = "groups"
	// CloneKindFlat is the flat repositories list format.
	CloneKindFlat CloneConfigKind = "flat"
)

// CloneConfig represents the YAML configuration for bulk clone.
// Supports two formats:
//  1. Flat format: global settings + repositories array (kind: flat)
//  2. Grouped format: global settings + named groups with their own targets (kind: groups)
type CloneConfig struct {
	// Config metadata
	Version int             `yaml:"version,omitempty"` // Config version (1)
	Kind    CloneConfigKind `yaml:"kind,omitempty"`    // Config kind: groups, flat

	// Global settings (can be overridden by CLI flags or group settings)
	Parallel  int    `yaml:"parallel,omitempty"`
	Strategy  string `yaml:"strategy,omitempty"`  // skip, pull, reset, rebase, fetch
	Structure string `yaml:"structure,omitempty"` // flat or user

	// Flat format: single target + repositories list
	Target       string          `yaml:"target,omitempty"`
	Repositories []CloneRepoSpec `yaml:"repositories,omitempty"`

	// Grouped format: named groups (parsed separately due to dynamic keys)
	Groups map[string]*CloneGroup `yaml:"-"`
}

// NormalizeCloneKind normalizes kind value and returns canonical form.
func NormalizeCloneKind(kind string) (CloneConfigKind, string, error) {
	switch CloneConfigKind(kind) {
	case CloneKindGroups:
		return CloneKindGroups, "", nil
	case CloneKindFlat:
		return CloneKindFlat, "", nil
	case "":
		// Empty: will be auto-detected based on content
		return "", "", nil
	default:
		return "", "", fmt.Errorf("invalid kind '%s': must be 'groups' or 'flat'", kind)
	}
}

// ValidCloneStrategies contains valid strategy values for clone config.
var ValidCloneStrategies = []string{"skip", "pull", "reset", "rebase", "fetch"}

// IsValidCloneStrategy checks if the strategy is valid.
func IsValidCloneStrategy(strategy string) bool {
	return slices.Contains(ValidCloneStrategies, strategy)
}

// CloneGroup represents a named group of repositories with its own target.
type CloneGroup struct {
	Target       string               `yaml:"target"`             // Required: target directory for this group
	Branch       configpkg.FlexBranch `yaml:"branch,omitempty"`   // Default branch for all repos in group
	Depth        int                  `yaml:"depth,omitempty"`    // Default depth for all repos in group
	Strategy     string               `yaml:"strategy,omitempty"` // Override global strategy
	Repositories []CloneRepoSpec      `yaml:"repositories"`       // Repository list
	Hooks        *CloneHooks          `yaml:"hooks,omitempty"`    // Group-level hooks (applied to all repos in group)
}

// CloneHooks represents before/after hook commands for clone operations.
// Hooks are executed without shell interpretation for security (no pipes, redirects, etc.).
type CloneHooks struct {
	Before []string `yaml:"before,omitempty"` // Commands to run before clone/update
	After  []string `yaml:"after,omitempty"`  // Commands to run after clone/update
}

// CloneRepoSpec represents a single repository specification in YAML.
type CloneRepoSpec struct {
	URL    string               `yaml:"url"`              // Required
	Name   string               `yaml:"name,omitempty"`   // Optional: custom directory name (extracted from URL if empty)
	Path   string               `yaml:"path,omitempty"`   // Optional: subdirectory within target
	Branch configpkg.FlexBranch `yaml:"branch,omitempty"` // Optional: branch to checkout
	Depth  int                  `yaml:"depth,omitempty"`  // Optional: shallow clone depth
	Hooks  *CloneHooks          `yaml:"hooks,omitempty"`  // Optional: repo-level hooks
}

// parseCloneConfig reads and parses YAML or JSON config from file or stdin.
// Detects format automatically: flat (has repositories) or grouped (has named groups).
func parseCloneConfig(configPath string, useStdin bool) (*CloneConfig, error) {
	var data []byte
	var err error

	if useStdin {
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
	} else {
		data, err = os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("read config file %s: %w", configPath, err)
		}
	}

	// Helper to unmarshal based on extension
	unmarshal := func(d []byte, v any) error {
		ext := ""
		if !useStdin {
			ext = strings.ToLower(filepath.Ext(configPath))
		}

		switch ext {
		case ".json":
			return json.Unmarshal(d, v)
		default: // .yaml, .yml, or stdin defaults to yaml
			return yaml.Unmarshal(d, v)
		}
	}

	// First, unmarshal to detect format
	var rawMap map[string]any
	if err := unmarshal(data, &rawMap); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Detect format: if 'repositories' key exists at top level, it's flat format
	_, hasRepositories := rawMap["repositories"]

	config := &CloneConfig{}

	if hasRepositories {
		// Flat format: unmarshal directly
		if err := unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("parse flat config: %w", err)
		}
	} else {
		// Grouped format: parse global settings and groups separately
		// Note: This part still relies on map[string]interface{} and is format-agnostic
		parseGroupedCloneConfig(rawMap, config)
	}

	// Validate
	if err := validateCloneConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

// parseGroupedCloneConfig parses grouped format where keys are group names.
func parseGroupedCloneConfig(rawMap map[string]any, config *CloneConfig) {
	// Known global keys (not groups)
	globalKeys := map[string]bool{
		"parallel": true, "strategy": true, "structure": true,
		"target": true, "update": true,
	}

	// Extract global settings
	if v, ok := rawMap["parallel"].(int); ok {
		config.Parallel = v
	}
	if v, ok := rawMap["strategy"].(string); ok {
		config.Strategy = v
	}
	if v, ok := rawMap["structure"].(string); ok {
		config.Structure = v
	}

	// Parse groups (any key that's not a global key and has 'target' + 'repositories')
	config.Groups = make(map[string]*CloneGroup)

	for key, value := range rawMap {
		if globalKeys[key] {
			continue
		}

		// Check if this looks like a group (map with target and repositories)
		groupMap, ok := value.(map[string]any)
		if !ok {
			continue
		}

		// Must have repositories to be considered a group
		if _, hasRepos := groupMap["repositories"]; !hasRepos {
			continue
		}

		// Parse group
		group := &CloneGroup{}

		if v, ok := groupMap["target"].(string); ok {
			group.Target = v
		}
		group.Branch = extractFlexBranch(groupMap["branch"])
		if v, ok := groupMap["depth"].(int); ok {
			group.Depth = v
		}
		if v, ok := groupMap["strategy"].(string); ok {
			group.Strategy = v
		}

		// Parse group-level hooks
		if hooksRaw, ok := groupMap["hooks"].(map[string]any); ok {
			group.Hooks = parseCloneHooks(hooksRaw)
		}

		// Parse repositories
		if reposRaw, ok := groupMap["repositories"].([]any); ok {
			for _, repoRaw := range reposRaw {
				repoMap, ok := repoRaw.(map[string]any)
				if !ok {
					continue
				}

				spec := CloneRepoSpec{}
				if v, ok := repoMap["url"].(string); ok {
					spec.URL = v
				}
				if v, ok := repoMap["name"].(string); ok {
					spec.Name = v
				}
				if v, ok := repoMap["path"].(string); ok {
					spec.Path = v
				}
				spec.Branch = extractFlexBranch(repoMap["branch"])
				if v, ok := repoMap["depth"].(int); ok {
					spec.Depth = v
				}

				// Parse repo-level hooks
				if hooksRaw, ok := repoMap["hooks"].(map[string]any); ok {
					spec.Hooks = parseCloneHooks(hooksRaw)
				}

				if spec.URL != "" {
					group.Repositories = append(group.Repositories, spec)
				}
			}
		}

		if len(group.Repositories) > 0 {
			config.Groups[key] = group
		}
	}
}

// validateCloneConfig validates the parsed YAML config.
func validateCloneConfig(config *CloneConfig) error {
	isFlat := len(config.Repositories) > 0
	isGrouped := len(config.Groups) > 0

	if !isFlat && !isGrouped {
		return fmt.Errorf("no repositories defined in config (need 'repositories' array or named groups)")
	}

	// Validate global structure
	if config.Structure != "" {
		if config.Structure != "flat" && config.Structure != "user" {
			return fmt.Errorf("invalid structure %q: must be 'flat' or 'user'", config.Structure)
		}
	}

	// Validate global strategy
	if err := validateStrategy(config.Strategy); err != nil {
		return err
	}

	if isFlat {
		// Validate flat format
		return validateFlatRepositories(config.Repositories, "")
	}

	// Validate grouped format
	for groupName, group := range config.Groups {
		if group.Target == "" {
			return fmt.Errorf("group %q: missing target directory", groupName)
		}

		if err := validateStrategy(group.Strategy); err != nil {
			return fmt.Errorf("group %q: %w", groupName, err)
		}

		if err := validateFlatRepositories(group.Repositories, groupName); err != nil {
			return err
		}
	}

	return nil
}

// validateStrategy validates a strategy value.
func validateStrategy(strategy string) error {
	if strategy == "" {
		return nil
	}
	validStrategies := map[string]bool{
		"skip": true, "pull": true, "reset": true, "rebase": true, "fetch": true,
	}
	if !validStrategies[strategy] {
		return fmt.Errorf("invalid strategy %q: must be one of 'skip', 'pull', 'reset', 'rebase', 'fetch'", strategy)
	}
	return nil
}

// validateFlatRepositories validates a list of repository specs.
func validateFlatRepositories(repos []CloneRepoSpec, groupName string) error {
	prefix := ""
	if groupName != "" {
		prefix = fmt.Sprintf("group %q: ", groupName)
	}

	if len(repos) == 0 {
		return fmt.Errorf("%sno repositories defined", prefix)
	}

	namesSeen := make(map[string]bool)
	for i, repo := range repos {
		if repo.URL == "" {
			return fmt.Errorf("%srepository[%d]: missing URL", prefix, i)
		}

		// Extract name from URL if not specified
		name := repo.Name
		if name == "" {
			extracted, err := repository.ExtractRepoNameFromURL(repo.URL)
			if err != nil {
				return fmt.Errorf("%srepository[%d]: cannot extract name from URL %q: %w", prefix, i, repo.URL, err)
			}
			name = extracted
		}

		// Include path in uniqueness check
		fullPath := name
		if repo.Path != "" {
			fullPath = filepath.Join(repo.Path, name)
		}

		if namesSeen[fullPath] {
			return fmt.Errorf("%srepository[%d]: duplicate path %q", prefix, i, fullPath)
		}
		namesSeen[fullPath] = true
	}

	return nil
}

// extractFlexBranch extracts a FlexBranch from a raw interface{} value.
// Handles string values directly and map values by extracting the defaultBranch key.
func extractFlexBranch(v any) configpkg.FlexBranch {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return configpkg.FlexBranch(val)
	case map[string]any:
		db, ok := val["defaultBranch"]
		if !ok {
			return ""
		}
		switch dbVal := db.(type) {
		case string:
			return configpkg.FlexBranch(dbVal)
		case []any:
			parts := make([]string, 0, len(dbVal))
			for _, item := range dbVal {
				if s, ok := item.(string); ok && s != "" {
					parts = append(parts, s)
				}
			}
			return configpkg.FlexBranch(strings.Join(parts, ","))
		}
	}
	return ""
}

// buildCloneOptionsFromConfig builds BulkCloneOptions from YAML config.
// CLI flags take precedence over YAML config.
func buildCloneOptionsFromConfig(
	config *CloneConfig,
	directory string,
	flags BulkCommandFlags,
	branch string,
	depth int,
	strategy string,
	structure string,
	logger repository.Logger,
) repository.BulkCloneOptions {
	// Use CLI argument if provided, otherwise YAML config, otherwise default
	targetDir := directory
	if targetDir == "." && config.Target != "" {
		targetDir = config.Target
	}

	// Use CLI flags if set, otherwise YAML config, otherwise defaults
	parallel := flags.Parallel
	if parallel == 0 && config.Parallel > 0 {
		parallel = config.Parallel
	}
	if parallel == 0 {
		parallel = repository.DefaultLocalParallel
	}

	structureVal := structure
	if structureVal == "flat" && config.Structure != "" {
		structureVal = config.Structure
	}

	// Resolve strategy: CLI flag > YAML config
	strategyVal := resolveCloneStrategy(strategy, config.Strategy)

	// Build BulkCloneOptions with custom per-repo settings
	opts := repository.BulkCloneOptions{
		Directory: targetDir,
		Structure: repository.DirectoryStructure(structureVal),
		Strategy:  strategyVal,
		Branch:    branch,
		Depth:     depth,
		Parallel:  parallel,
		DryRun:    flags.DryRun,
		Verbose:   verbose,
		Logger:    logger,
		ProgressCallback: func(current, total int, url string) {
			if shouldShowProgress(flags.Format, quiet) {
				repoName, _ := repository.ExtractRepoNameFromURL(url)
				if repoName == "" {
					repoName = "repository"
				}
				fmt.Printf("[%d/%d] Cloning %s...\n", current, total, repoName)
			}
		},
	}

	return opts
}

// resolveCloneStrategy resolves the effective strategy with precedence:
// CLI --update-strategy > YAML strategy > default (skip).
func resolveCloneStrategy(cliStrategy, yamlStrategy string) repository.UpdateStrategy {
	// CLI --update-strategy takes highest precedence
	if cliStrategy != "" {
		return repository.UpdateStrategy(cliStrategy)
	}
	// YAML strategy
	if yamlStrategy != "" {
		return repository.UpdateStrategy(yamlStrategy)
	}
	// Default: skip
	return repository.StrategySkip
}

// runCloneFromConfig executes clone operation based on YAML config.
// Supports both flat format (repositories array) and grouped format (named groups).
func runCloneFromConfig(ctx context.Context, directory string) error {
	// Parse YAML config
	config, err := parseCloneConfig(cloneConfig, cloneConfigStdin)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// Validate format
	if err := validateBulkFormat(cloneFlags.Format); err != nil {
		return err
	}

	// Determine if flat or grouped format
	isGrouped := len(config.Groups) > 0

	if isGrouped {
		return runCloneFromGroupedConfig(ctx, config, directory)
	}

	return runCloneFromFlatConfig(ctx, config, directory)
}

// runCloneFromFlatConfig handles flat format (repositories array).
func runCloneFromFlatConfig(ctx context.Context, config *CloneConfig, directory string) error {
	client := repository.NewClient()
	logger := createBulkLogger(verbose)

	baseOpts := buildCloneOptionsFromConfig(
		config,
		directory,
		cloneFlags,
		cloneBranch,
		cloneDepth,
		cloneStrategy,
		cloneStructure,
		logger,
	)

	totalRepos := len(config.Repositories)

	if shouldShowProgress(cloneFlags.Format, quiet) {
		suffix := ""
		if cloneFlags.DryRun {
			suffix = " [DRY-RUN]"
		}
		strategyStr := ""
		if baseOpts.Strategy != repository.StrategySkip {
			strategyStr = fmt.Sprintf(", strategy: %s", baseOpts.Strategy)
		}
		dirStr := ""
		if baseOpts.Directory != "." {
			dirStr = fmt.Sprintf(" to %s", baseOpts.Directory)
		}
		fmt.Printf("Cloning %d repositories%s (parallel: %d, structure: %s%s)%s\n",
			totalRepos, dirStr, baseOpts.Parallel, baseOpts.Structure, strategyStr, suffix)
	}

	results := cloneRepositoriesParallel(ctx, client, config.Repositories, baseOpts, "", nil)
	displayCloneResults(results)

	return errPartialFailure(results.TotalFailed, results.TotalRequested)
}

// runCloneFromGroupedConfig handles grouped format (named groups with targets).
func runCloneFromGroupedConfig(ctx context.Context, config *CloneConfig, directory string) error {
	client := repository.NewClient()
	logger := createBulkLogger(verbose)

	// Filter groups if --group flag is specified
	groupsToClone := config.Groups
	if len(cloneGroup) > 0 {
		groupsToClone = make(map[string]*CloneGroup)
		for _, name := range cloneGroup {
			if group, ok := config.Groups[name]; ok {
				groupsToClone[name] = group
			} else {
				return fmt.Errorf("group %q not found in config", name)
			}
		}
	}

	// Count total repositories
	totalRepos := 0
	for _, group := range groupsToClone {
		totalRepos += len(group.Repositories)
	}

	if shouldShowProgress(cloneFlags.Format, quiet) {
		suffix := ""
		if cloneFlags.DryRun {
			suffix = " [DRY-RUN]"
		}
		fmt.Printf("Cloning %d repositories from %d groups%s\n",
			totalRepos, len(groupsToClone), suffix)
	}

	// Clone each group
	allResults := &repository.BulkCloneResult{
		TotalRequested: totalRepos,
		Summary:        make(map[string]int),
		Repositories:   make([]repository.RepositoryCloneResult, 0, totalRepos),
	}

	startTime := time.Now()

	for groupName, group := range groupsToClone {
		// Resolve target directory (relative to CLI directory argument)
		targetDir := group.Target
		if directory != "." && !filepath.IsAbs(targetDir) {
			targetDir = filepath.Join(directory, targetDir)
		}

		// Build options for this group
		groupOpts := buildGroupCloneOptions(config, group, targetDir, logger)

		if shouldShowProgress(cloneFlags.Format, quiet) {
			fmt.Printf("\n[%s] → %s (%d repos)\n", groupName, targetDir, len(group.Repositories))
		}

		// Clone repositories in this group
		groupResults := cloneRepositoriesParallel(ctx, client, group.Repositories, groupOpts, groupName, group.Hooks)

		// Merge results
		for _, r := range groupResults.Repositories {
			allResults.Repositories = append(allResults.Repositories, r)
			allResults.Summary[r.Status]++

			switch r.Status {
			case "cloned":
				allResults.TotalCloned++
			case "updated", "pulled", "rebased", "fetched", "reset":
				allResults.TotalUpdated++
			case "skipped":
				allResults.TotalSkipped++
			case "error":
				allResults.TotalFailed++
			}
		}
	}

	allResults.Duration = time.Since(startTime)
	displayCloneResults(allResults)

	return errPartialFailure(allResults.TotalFailed, allResults.TotalRequested)
}

// buildGroupCloneOptions builds clone options for a specific group.
func buildGroupCloneOptions(config *CloneConfig, group *CloneGroup, targetDir string, logger repository.Logger) repository.BulkCloneOptions {
	// Resolve parallel: CLI > global config > default
	parallel := cloneFlags.Parallel
	if parallel == 0 && config.Parallel > 0 {
		parallel = config.Parallel
	}
	if parallel == 0 {
		parallel = repository.DefaultLocalParallel
	}

	// Resolve strategy: CLI > group > global > default
	strategy := resolveCloneStrategy(cloneStrategy, group.Strategy)
	if strategy == repository.StrategySkip && config.Strategy != "" {
		strategy = repository.UpdateStrategy(config.Strategy)
	}

	// Resolve structure
	structureVal := cloneStructure
	if structureVal == "flat" && config.Structure != "" {
		structureVal = config.Structure
	}

	// Resolve branch: CLI > group > empty
	branch := cloneBranch
	if branch == "" && group.Branch != "" {
		branch = string(group.Branch)
	}

	// Resolve depth: CLI > group > 0
	depth := cloneDepth
	if depth == 0 && group.Depth > 0 {
		depth = group.Depth
	}

	return repository.BulkCloneOptions{
		Directory: targetDir,
		Structure: repository.DirectoryStructure(structureVal),
		Strategy:  strategy,
		Branch:    branch,
		Depth:     depth,
		Parallel:  parallel,
		DryRun:    cloneFlags.DryRun,
		Verbose:   verbose,
		Logger:    logger,
	}
}

// cloneRepositoriesParallel clones repositories in parallel and returns results.
func cloneRepositoriesParallel(
	ctx context.Context,
	client repository.Client,
	repos []CloneRepoSpec,
	opts repository.BulkCloneOptions,
	groupName string,
	groupHooks *CloneHooks,
) *repository.BulkCloneResult {
	totalRepos := len(repos)

	results := &repository.BulkCloneResult{
		TotalRequested: totalRepos,
		Summary:        make(map[string]int),
		Repositories:   make([]repository.RepositoryCloneResult, 0, totalRepos),
	}

	startTime := time.Now()

	type workItem struct {
		index    int
		repoSpec CloneRepoSpec
		repoName string
	}

	workChan := make(chan workItem, totalRepos)
	resultsChan := make(chan repository.RepositoryCloneResult, totalRepos)

	// Start worker pool
	var wg sync.WaitGroup
	for w := 0; w < opts.Parallel; w++ {
		wg.Go(func() {
			for work := range workChan {
				// Show progress
				if shouldShowProgress(cloneFlags.Format, quiet) {
					prefix := ""
					if groupName != "" {
						prefix = fmt.Sprintf("[%s] ", groupName)
					}
					fmt.Printf("%s[%d/%d] Cloning %s...\n", prefix, work.index+1, totalRepos, work.repoName)
				}

				result := cloneSingleRepository(ctx, client, work.repoSpec, work.repoName, opts, groupHooks)
				resultsChan <- result
			}
		})
	}

	// Send work items
	go func() {
		for i, repoSpec := range repos {
			repoName := repoSpec.Name
			if repoName == "" {
				extracted, _ := repository.ExtractRepoNameFromURL(repoSpec.URL)
				repoName = extracted
			}

			workChan <- workItem{
				index:    i,
				repoSpec: repoSpec,
				repoName: repoName,
			}
		}
		close(workChan)
	}()

	// Wait for workers to finish
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results
	for result := range resultsChan {
		results.Repositories = append(results.Repositories, result)

		switch result.Status {
		case "cloned":
			results.TotalCloned++
		case "updated", "pulled", "rebased", "fetched", "reset":
			results.TotalUpdated++
		case "skipped":
			results.TotalSkipped++
		case "error":
			results.TotalFailed++
		}

		if results.Summary == nil {
			results.Summary = make(map[string]int)
		}
		results.Summary[result.Status]++
	}

	results.Duration = time.Since(startTime)
	return results
}
