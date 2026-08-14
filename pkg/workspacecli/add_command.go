// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package workspacecli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// AddOptions holds options for the add command.
type AddOptions struct {
	ConfigFile  string
	Name        string
	URL         string
	Path        string
	FromCurrent bool
}

func (f CommandFactory) newAddCmd() *cobra.Command {
	opts := &AddOptions{}

	cmd := &cobra.Command{
		Use:   "add [url]",
		Short: "Add repository to workspace config",
		Long: cliutil.QuickStartHelp(`  # Add repository by URL
  gz-git workspace add https://github.com/user/repo.git

  # Add with custom name and path
  gz-git workspace add --name myrepo --url git@github.com:user/repo.git --path ./repos/myrepo

  # Add current directory's repo to config
  gz-git workspace add --from-current

  # Add to specific config file
  gz-git workspace add https://github.com/user/repo.git -c myworkspace.yaml`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.URL = args[0]
			}
			return f.runAdd(cmd, opts)
		},
	}

	cmd.Flags().StringVarP(&opts.ConfigFile, "config", "c", "", "Config file (default: "+DefaultConfigFile+")")
	cmd.Flags().StringVar(&opts.Name, "name", "", "Repository name")
	cmd.Flags().StringVar(&opts.URL, "url", "", "Repository URL")
	cmd.Flags().StringVar(&opts.Path, "path", "", "Path for clone")
	cmd.Flags().BoolVar(&opts.FromCurrent, "from-current", false, "Add current directory's repo")

	return cmd
}

func (f CommandFactory) runAdd(cmd *cobra.Command, opts *AddOptions) error {
	// Determine config file
	configPath := opts.ConfigFile
	if configPath == "" {
		detected, err := detectConfigFile(".")
		if err != nil {
			// Create new config if not found
			configPath = DefaultConfigFile
			fmt.Fprintf(cmd.OutOrStdout(), "Creating new config: %s\n", configPath)
		} else {
			configPath = detected
			// DetectConfigFile searches upward, so this may resolve a parent
			// workspace's config; surface it so `add` never silently targets a
			// different workspace than the user expects.
			fmt.Fprintf(cmd.OutOrStdout(), "Using config: %s\n", configPath)
		}
	}

	// Handle --from-current
	if opts.FromCurrent {
		return f.addFromCurrent(cmd, configPath)
	}

	// Validate inputs
	if opts.URL == "" {
		return fmt.Errorf("repository URL required (use --url or pass as argument)")
	}

	// Auto-detect name from URL if not provided
	name := opts.Name
	if name == "" {
		name = extractRepoName(opts.URL)
	}

	// Auto-generate target path if not provided
	targetPath := opts.Path
	if targetPath == "" {
		targetPath = "./" + name
	}

	// Load existing config or create new
	config, err := loadOrCreateConfig(configPath)
	if err != nil {
		return err
	}

	// Validate and add the repository. appendRepoEntry rejects empty/invalid
	// URLs and duplicates before anything is written to disk.
	if err := appendRepoEntry(config, name, opts.URL, targetPath); err != nil {
		return err
	}

	// Write config
	if err := writeConfig(configPath, config); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "✓ Added %s to %s\n", name, configPath)
	return nil
}

func (f CommandFactory) addFromCurrent(cmd *cobra.Command, configPath string) error {
	// Check if current directory is a git repo
	if _, err := os.Stat(".git"); os.IsNotExist(err) {
		return fmt.Errorf("current directory is not a git repository")
	}

	absPath, err := filepath.Abs(".")
	if err != nil {
		return err
	}
	name := filepath.Base(absPath)

	// Resolve the real remote URL instead of scaffolding an empty entry. An
	// empty URL is unusable by sync and used to be written silently; if the repo
	// has no usable remote we fail loudly rather than persist a broken entry.
	ctx := cmd.Context()
	client := repository.NewClient()
	repo, err := client.Open(ctx, absPath)
	if err != nil {
		return fmt.Errorf("open current repository: %w", err)
	}
	info, err := client.GetInfo(ctx, repo)
	if err != nil {
		return fmt.Errorf("read current repository info: %w", err)
	}
	if info.RemoteURL == "" {
		return fmt.Errorf("current repository has no 'origin' remote; add one with 'git remote add origin <url>' or run 'workspace add <url>'")
	}

	config, err := loadOrCreateConfig(configPath)
	if err != nil {
		return err
	}

	// appendRepoEntry validates the resolved URL and rejects duplicates.
	if err := appendRepoEntry(config, name, info.RemoteURL, absPath); err != nil {
		return err
	}

	if err := writeConfig(configPath, config); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "✓ Added %s (%s) to %s\n", name, info.RemoteURL, configPath)
	return nil
}

// appendRepoEntry validates a new repository and appends it to the config's
// repositories list. It rejects empty or malformed URLs via the shared git-input
// sanitizer and refuses duplicates (by URL or by path, tolerating the legacy
// "targetPath" key on existing entries) — so `workspace add` never writes an
// unusable entry to disk. New entries use the canonical "path" key that every
// config loader reads (the old code wrote "targetPath", which loaders ignore).
func appendRepoEntry(config map[string]any, name, url, path string) error {
	if err := gitcmd.SanitizeURL(url); err != nil {
		return fmt.Errorf("invalid repository URL %q: %w", url, err)
	}

	repos, ok := config["repositories"].([]any)
	if !ok {
		repos = []any{}
	}

	for _, r := range repos {
		rm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		if rm["url"] == url {
			return fmt.Errorf("repository with URL %q already exists", url)
		}
		if rm["path"] == path || rm["targetPath"] == path {
			return fmt.Errorf("repository with path %q already exists", path)
		}
	}

	config["repositories"] = append(repos, map[string]any{
		"name": name,
		"url":  url,
		"path": path,
	})
	return nil
}

func loadOrCreateConfig(path string) (map[string]any, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is the explicit workspace config selected by the caller.
	if os.IsNotExist(err) {
		// Create default config structure
		return map[string]any{
			"strategy":     "reset",
			"parallel":     4,
			"maxRetries":   3,
			"cloneProto":   "ssh",
			"sshPort":      0,
			"repositories": []any{},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var config map[string]any
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return config, nil
}

func writeConfig(path string, config map[string]any) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0o600)
}

func extractRepoName(url string) string {
	// Handle various URL formats
	// git@github.com:user/repo.git -> repo
	// https://github.com/user/repo.git -> repo
	// https://github.com/user/repo -> repo

	base := filepath.Base(url)

	// Remove .git suffix
	if len(base) > 4 && base[len(base)-4:] == ".git" {
		base = base[:len(base)-4]
	}

	return base
}
