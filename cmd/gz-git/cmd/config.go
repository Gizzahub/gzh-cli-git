// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/wizard"
)

var (
	configGlobal  bool // For init --global, set --global
	configLocal   bool // For init --local, show --local
	showEffective bool // For show --effective
)

// configCmd represents the config command.
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration profiles and settings",
	Long: cliutil.QuickStartHelp(`  # Initialize project config (.gz-git.yaml)
  gz-git config init

  # Create a profile
  gz-git config profile create work

  # Set active profile
  gz-git config profile use work

  # Show effective config
  gz-git config show`),
	Example: ``,
}

// configInitCmd initializes configuration.
var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize project or global configuration",
	Long: cliutil.QuickStartHelp(`  # Initialize project config (default)
  gz-git config init

  # Initialize project config (explicit)
  gz-git config init --local

  # Initialize global config
  gz-git config init --global`),
	Example: ``,
	RunE:    runConfigInit,
}

// configShowCmd shows effective configuration.
var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show project or effective configuration",
	Long: cliutil.QuickStartHelp(`  # Show project config (.gz-git.yaml) - Default
  gz-git config show

  # Show project config (explicit)
  gz-git config show --local

  # Show effective config with source attribution
  gz-git config show --effective`),
	Example: ``,
	RunE:    runConfigShow,
}

// Profile subcommands.
var configProfileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage configuration profiles",
	Long: cliutil.QuickStartHelp(`  # Create a profile
  gz-git config profile create work

  # List all profiles
  gz-git config profile list

  # Switch to a profile
  gz-git config profile use work

  # Show profile details
  gz-git config profile show work`),
	Example: ``,
}

var configProfileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all profiles",
	Long:  `List all available configuration profiles.`,
	RunE:  runConfigProfileList,
}

var configProfileShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show profile details",
	Long:  `Show the contents of a specific profile.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigProfileShow,
}

var configProfileCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new profile",
	Long: cliutil.QuickStartHelp(`  # Interactive creation (recommended for first-time)
  gz-git config profile create work

  # Create work profile with all settings
  gz-git config profile create work \
    --provider gitlab \
    --base-url https://gitlab.company.com \
    --token ${WORK_GITLAB_TOKEN} \
    --clone-proto ssh \
    --ssh-port 2224

  # After creation, activate the profile
  gz-git config profile use work`),
	Example: ``,
	RunE:    runConfigProfileCreate,
}

var configProfileUseCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Set active profile",
	Long:  `Set the active configuration profile.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigProfileUse,
}

var configProfileDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a profile",
	Long:  `Delete a configuration profile. The default profile cannot be deleted.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigProfileDelete,
}

var configHierarchyCmd = &cobra.Command{
	Use:   "hierarchy",
	Short: "Show config hierarchy tree",
	Long: cliutil.QuickStartHelp(`  # Show full hierarchy from current directory
  gz-git config hierarchy

  # Show hierarchy with validation (check for errors)
  gz-git config hierarchy --validate

  # Show compact format (less verbose)
  gz-git config hierarchy --compact`),
	Example: ``,
	RunE:    runConfigHierarchy,
}

// Config command flags.
var (
	validateFlag bool
	compactFlag  bool
)

// Profile creation flags.
var (
	profileProvider     string
	profileBaseURL      string
	profileToken        string
	profileCloneProto   string
	profileSSHPort      int
	profileParallel     int
	profileSubgroups    bool
	profileSubgroupMode string
)

func init() {
	rootCmd.AddCommand(configCmd)

	// Subcommands
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configProfileCmd)
	configCmd.AddCommand(configHierarchyCmd)

	// Profile subcommands
	configProfileCmd.AddCommand(configProfileListCmd)
	configProfileCmd.AddCommand(configProfileShowCmd)
	configProfileCmd.AddCommand(configProfileCreateCmd)
	configProfileCmd.AddCommand(configProfileUseCmd)
	configProfileCmd.AddCommand(configProfileDeleteCmd)

	// Flags
	configInitCmd.Flags().BoolVar(&configGlobal, "global", false, "Initialize global configuration (~/.config/gz-git)")
	configInitCmd.Flags().BoolVar(&configLocal, "local", false, "Initialize project config (default)")

	configShowCmd.Flags().BoolVar(&showEffective, "effective", false, "Show effective configuration (merged)")
	configShowCmd.Flags().BoolVar(&configLocal, "local", false, "Show project config (default)")

	configHierarchyCmd.Flags().BoolVar(&validateFlag, "validate", false, "Validate all config files in hierarchy")
	configHierarchyCmd.Flags().BoolVar(&compactFlag, "compact", false, "Show compact output")

	// Profile creation flags
	configProfileCreateCmd.Flags().StringVar(&profileProvider, "provider", "", "Forge provider (github, gitlab, gitea)")
	configProfileCreateCmd.Flags().StringVar(&profileBaseURL, "base-url", "", "API base URL")
	configProfileCreateCmd.Flags().StringVar(&profileToken, "token", "", "API token (use ${ENV_VAR} for env vars)")
	configProfileCreateCmd.Flags().StringVar(&profileCloneProto, "clone-proto", "", "Clone protocol (ssh, https)")
	configProfileCreateCmd.Flags().IntVar(&profileSSHPort, "ssh-port", 0, "SSH port")
	configProfileCreateCmd.Flags().IntVar(&profileParallel, "parallel", 0, "Parallel job count")
	configProfileCreateCmd.Flags().BoolVar(&profileSubgroups, "include-subgroups", false, "Include subgroups (GitLab)")
	configProfileCreateCmd.Flags().StringVar(&profileSubgroupMode, "subgroup-mode", "", "Subgroup mode (flat, nested)")
}

// runConfigInit initializes configuration.
func runConfigInit(cmd *cobra.Command, args []string) error {
	mgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to create manager: %w", err)
	}

	if configGlobal {
		// Initialize global config
		if err := mgr.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize config: %w", err)
		}

		paths, _ := config.NewPaths()
		fmt.Printf("Initialized configuration in %s\n", paths.ConfigDir)
		fmt.Println("Created default profile")
		return nil
	}

	// Initialize project config (Default)
	projectConfig := &config.ProjectConfig{
		Profile: config.DefaultProfileName,
	}

	if err := mgr.SaveProjectConfig(projectConfig); err != nil {
		return fmt.Errorf("failed to create project config: %w", err)
	}

	cwd, _ := os.Getwd()
	fmt.Printf("Created .gz-git.yaml in %s\n", cwd)
	return nil
}

// runConfigShow shows effective configuration.
func runConfigShow(cmd *cobra.Command, args []string) error {
	mgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to create manager: %w", err)
	}

	if !showEffective {
		// Show project config (Default)
		projectConfig, err := mgr.LoadProjectConfig()
		if err != nil {
			return fmt.Errorf("failed to load project config: %w", err)
		}

		if projectConfig == nil {
			fmt.Println("No project config found (.gz-git.yaml)")
			return nil
		}

		data, err := yaml.Marshal(projectConfig)
		if err != nil {
			return fmt.Errorf("failed to marshal config: %w", err)
		}

		fmt.Printf("Project Config (.gz-git.yaml):\n%s", string(data))
		return nil
	}

	// Load and show effective config.
	// LoadEffectiveConfig applies the global --profile override, so the output
	// matches what other commands will actually use.
	effective, err := LoadEffectiveConfig(cmd, nil)
	if err != nil {
		return fmt.Errorf("failed to resolve config: %w", err)
	}

	// Print effective config with sources
	fmt.Println("Effective Configuration:")
	fmt.Println()

	printConfigValue("Provider", effective.Provider, effective.GetSource("provider"))
	printConfigValue("Base URL", effective.BaseURL, effective.GetSource("baseURL"))
	printConfigValue("Token", sanitizeToken(effective.Token), effective.GetSource("token"))
	printConfigValue("Clone Protocol", effective.CloneProto, effective.GetSource("cloneProto"))
	if effective.SSHPort > 0 {
		printConfigValue("SSH Port", fmt.Sprintf("%d", effective.SSHPort), effective.GetSource("sshPort"))
	}
	printConfigValue("Parallel", fmt.Sprintf("%d", effective.Parallel), effective.GetSource("parallel"))
	printConfigValue("Include Subgroups", fmt.Sprintf("%t", effective.IncludeSubgroups), effective.GetSource("includeSubgroups"))
	if effective.SubgroupMode != "" {
		printConfigValue("Subgroup Mode", effective.SubgroupMode, effective.GetSource("subgroupMode"))
	}

	return nil
}

// runConfigProfileList lists all profiles.
func runConfigProfileList(cmd *cobra.Command, args []string) error {
	mgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to create manager: %w", err)
	}

	profiles, err := mgr.ListProfiles()
	if err != nil {
		return fmt.Errorf("failed to list profiles: %w", err)
	}

	if len(profiles) == 0 {
		fmt.Println("No profiles found. Run 'gz-git config init' to create default profile.")
		return nil
	}

	// Get active profile
	activeProfile, _ := mgr.GetActiveProfile()

	// Sort profiles
	sort.Strings(profiles)

	fmt.Println("Available profiles:")
	for _, name := range profiles {
		marker := " "
		if name == activeProfile {
			marker = "*"
		}
		fmt.Printf("  %s %s", marker, name)
		if name == activeProfile {
			fmt.Print(" (active)")
		}
		fmt.Println()
	}

	return nil
}

// runConfigProfileShow shows profile details.
func runConfigProfileShow(cmd *cobra.Command, args []string) error {
	profileName := args[0]

	mgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to create manager: %w", err)
	}

	profile, err := mgr.LoadProfile(profileName)
	if err != nil {
		return fmt.Errorf("failed to load profile: %w", err)
	}

	data, err := yaml.Marshal(profile)
	if err != nil {
		return fmt.Errorf("failed to marshal profile: %w", err)
	}

	fmt.Printf("Profile: %s\n\n%s", profileName, string(data))
	return nil
}

// runConfigProfileCreate creates a new profile.
func runConfigProfileCreate(cmd *cobra.Command, args []string) error {
	profileName := args[0]
	ctx := cmd.Context()

	mgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to create manager: %w", err)
	}

	// Check if profile already exists
	if mgr.ProfileExists(profileName) {
		return fmt.Errorf("profile '%s' already exists", profileName)
	}

	var profile *config.Profile

	// If flags provided, use them
	if profileProvider != "" || profileBaseURL != "" || profileToken != "" {
		profile = &config.Profile{
			Name:             profileName,
			Provider:         profileProvider,
			BaseURL:          profileBaseURL,
			Token:            profileToken,
			CloneProto:       profileCloneProto,
			SSHPort:          profileSSHPort,
			Parallel:         profileParallel,
			IncludeSubgroups: profileSubgroups,
			SubgroupMode:     profileSubgroupMode,
		}
	} else {
		// Interactive mode with wizard
		w := wizard.NewProfileCreateWizard(profileName)
		profile, err = w.Run(ctx)
		if err != nil {
			return fmt.Errorf("failed to create profile: %w", err)
		}
	}

	// Create profile
	if err := mgr.CreateProfile(profile); err != nil {
		return fmt.Errorf("failed to create profile: %w", err)
	}

	fmt.Printf("Created profile '%s'\n", profileName)
	fmt.Printf("Set as active with: gz-git config profile use %s\n", profileName)

	return nil
}

// runConfigProfileUse sets active profile.
func runConfigProfileUse(cmd *cobra.Command, args []string) error {
	profileName := args[0]

	mgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to create manager: %w", err)
	}

	if err := mgr.SetActiveProfile(profileName); err != nil {
		return fmt.Errorf("failed to set active profile: %w", err)
	}

	fmt.Printf("Switched to profile '%s'\n", profileName)
	return nil
}

// runConfigProfileDelete deletes a profile.
func runConfigProfileDelete(cmd *cobra.Command, args []string) error {
	profileName := args[0]

	mgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to create manager: %w", err)
	}

	if err := mgr.DeleteProfile(profileName); err != nil {
		return fmt.Errorf("failed to delete profile: %w", err)
	}

	fmt.Printf("Deleted profile '%s'\n", profileName)
	return nil
}

// runConfigHierarchy shows config hierarchy tree.
func runConfigHierarchy(cmd *cobra.Command, args []string) error {
	// Load config from home directory or current directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Try to find config starting from current directory
	configDir, err := config.FindConfigRecursive(cwd, ".gz-git.yaml")
	if err != nil {
		// No project .gz-git.yaml between cwd and $HOME. Fall back to the home
		// directory so the global/profile layers still render — but say so,
		// rather than silently switching the search root.
		home, _ := os.UserHomeDir()
		configDir = home
		fmt.Printf("No project config found between %s and $HOME; showing home config hierarchy (%s)\n\n", cwd, configDir)
	}

	cfg, err := config.LoadConfigRecursive(configDir, ".gz-git.yaml")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg == nil {
		fmt.Println("No config found (.gz-git.yaml)")
		fmt.Println("Create one with: gz-git config init --local")
		return nil
	}

	fmt.Println("Configuration Hierarchy:")
	fmt.Println()

	printConfigTree(cfg, configDir, ".gz-git.yaml", 0, validateFlag, compactFlag)

	return nil
}

// printConfigTree recursively prints config hierarchy.
func printConfigTree(cfg *config.Config, path, configFile string, depth int, validate, compact bool) {
	indent := strings.Repeat("  ", depth)

	// Print this level
	fmt.Printf("%s%s/%s\n", indent, path, configFile)

	// Print config summary if not compact
	if !compact {
		if cfg.Profile != "" {
			fmt.Printf("%s  profile: %s\n", indent, cfg.Profile)
		}
		if cfg.GetParallel() > 0 {
			fmt.Printf("%s  parallel: %d\n", indent, cfg.GetParallel())
		}
	}

	// Validate if requested
	if validate {
		v := config.NewValidator()
		if err := v.ValidateConfig(cfg); err != nil {
			fmt.Printf("%s  ⚠ validation error: %v\n", indent, err)
		} else if !compact {
			fmt.Printf("%s  ✓ valid\n", indent)
		}
	}

	// Print workspaces
	for name, ws := range cfg.Workspaces {
		wsIndent := strings.Repeat("  ", depth+1)
		wsPath := resolveChildPath(path, ws.Path)

		effectiveType := ws.Type.Resolve(ws.Source != nil)

		switch effectiveType {
		case config.WorkspaceTypeConfig:
			// Recursive: load workspace config
			wsConfigFile := ".gz-git.yaml"

			wsCfg, err := config.LoadConfigRecursive(wsPath, wsConfigFile)
			if err != nil {
				fmt.Printf("%s%s (type=config) ⚠ failed to load: %v\n", wsIndent, name, err)
			} else {
				printConfigTree(wsCfg, wsPath, wsConfigFile, depth+1, validate, compact)
			}

		case config.WorkspaceTypeForge:
			// Forge workspace
			fmt.Printf("%s%s (type=forge)\n", wsIndent, name)
			if !compact {
				fmt.Printf("%s  path: %s\n", wsIndent, ws.Path)
				if ws.Source != nil {
					fmt.Printf("%s  source: %s/%s\n", wsIndent, ws.Source.Provider, ws.Source.Org)
				}
				if ws.Profile != "" {
					fmt.Printf("%s  profile: %s\n", wsIndent, ws.Profile)
				}
			}

		case config.WorkspaceTypeGit:
			// Git workspace
			fmt.Printf("%s%s (type=git)\n", wsIndent, name)
			if !compact {
				fmt.Printf("%s  path: %s\n", wsIndent, ws.Path)
				if ws.Profile != "" {
					fmt.Printf("%s  profile: %s\n", wsIndent, ws.Profile)
				}
			}
		}
	}
}

// resolveChildPath resolves child path relative to parent.
func resolveChildPath(parentPath, childPath string) string {
	if strings.HasPrefix(childPath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(parentPath, childPath)
		}
		return filepath.Join(home, childPath[2:])
	}
	if filepath.IsAbs(childPath) {
		return childPath
	}
	return filepath.Join(parentPath, childPath)
}

// printConfigValue prints a config value with its source.
func printConfigValue(key, value, source string) {
	if value == "" {
		return
	}
	fmt.Printf("  %-20s %s", key+":", value)
	if source != "" {
		fmt.Printf(" (from %s)", source)
	}
	fmt.Println()
}

// sanitizeToken removes sensitive parts of tokens for display.
func sanitizeToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "..." + token[len(token)-4:]
}
