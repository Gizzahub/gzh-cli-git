// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package reposynccli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/gitea"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/github"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/gitlab"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/provider"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/reposync"
)

// FromForgeOptions holds options for from-forge sync command.
type FromForgeOptions struct {
	Provider         string
	Organization     string
	TargetPath       string
	Token            string
	BaseURL          string // API endpoint (http/https only)
	CloneProto       string // Clone protocol: ssh, https (default: ssh)
	SSHPort          int    // Custom SSH port (0 = default 22)
	SSHKeyPath       string // SSH private key file path
	SSHKeyContent    string // SSH private key content (use env var)
	Strategy         string
	Parallel         int
	MaxRetries       int
	Resume           bool
	DryRun           bool
	StateFile        string
	IncludeArchived  bool
	IncludeForks     bool
	IncludePrivate   bool
	CleanupOrphans   bool
	IsUser           bool
	IncludeSubgroups bool   // GitLab: include subgroups
	SubgroupMode     string // GitLab: flat | nested

	// Metadata filters
	FilterLanguage string // Comma-separated languages (e.g., "go,rust")
	FilterMinStars int    // Minimum star count
	FilterMaxStars int    // Maximum star count (0 = unlimited)
	FilterLastPush string // Activity filter (e.g., "30d", "6M", "1y")
	FilterInclude  []string
	FilterExclude  []string
}

// newFromForgeCmd creates a command for syncing from git forges.
func (f CommandFactory) newFromForgeCmd() *cobra.Command {
	opts := &FromForgeOptions{
		Strategy:     "reset",
		Parallel:     repository.DefaultForgeParallel,
		MaxRetries:   3,
		CloneProto:   "ssh",  // Default to SSH
		SubgroupMode: "flat", // Default to flat
	}

	cmd := &cobra.Command{
		Use:     "from",
		Aliases: []string{"from-forge"},
		Short:   "Sync repositories from a Git forge (GitHub, GitLab, Gitea)",
		Long: cliutil.QuickStartHelp(`  # Sync from GitHub organization (default: SSH clone)
  gz-git forge from --provider github --org myorg --path ./repos --token $GITHUB_TOKEN

  # Sync from GitLab group with HTTPS clone
  gz-git forge from --provider gitlab --org mygroup --path ./repos \
    --token $GITLAB_TOKEN --clone-proto https

  # Sync from self-hosted GitLab with custom SSH port
  gz-git forge from --provider gitlab --org mygroup --path ./repos \
    --base-url https://gitlab.company.com --token $GITLAB_TOKEN \
    --clone-proto ssh --ssh-port 2224

  # Sync GitLab with subgroups (flat mode)
  gz-git forge from --provider gitlab --org parent-group --path ./repos \
    --include-subgroups --subgroup-mode flat

  # Sync from Gitea
  gz-git forge from --provider gitea --org myorg --path ./repos \
    --base-url https://gitea.company.com --token $GITEA_TOKEN

  # Selective filtering: only Go repos with 100+ stars, active in last 30 days
  gz-git forge from --provider github --org kubernetes --path ./k8s \
    --language go --min-stars 100 --last-push-within 30d

  # Multiple languages with star count range
  gz-git forge from --provider github --org rust-lang --path ./rust-repos \
    --language "rust,go" --min-stars 50 --max-stars 1000

  # Activity filter with custom duration (7d, 30d, 6M, 1y)
  gz-git forge from --provider github --org myorg --path ./repos \
    --last-push-within 6M

  # Sync only selected repositories by name or full path
  gz-git forge from --provider github --org myorg --path ./repos \
    --include "api|web" --exclude "archive"

Filter Flags:
  --include          Include repos matching regex (name or full path; repeatable)
  --exclude          Exclude repos matching regex (name or full path; repeatable)
  --language          Comma-separated languages (e.g., go,rust,python)
  --min-stars         Minimum star count (0 = no minimum)
  --max-stars         Maximum star count (0 = unlimited)
  --last-push-within  Activity cutoff: 7d, 30d, 6M, 1y (d=days, w=weeks, M=months, y=years)

Note: GitLab and Gitea do not provide language info via API; --language may not work as expected.`),
		RunE: func(cmd *cobra.Command, args []string) error {
			return f.runFromForge(cmd, opts)
		},
	}

	// Provider and path (required)
	cmd.Flags().StringVar(&opts.Provider, "provider", "", "Git forge provider: github, gitlab, gitea [required]")
	cmd.Flags().StringVar(&opts.Organization, "org", "", "Organization/group name to sync [required]")
	cmd.Flags().StringVar(&opts.TargetPath, "path", "", "Directory for cloned repositories [required]")
	cmd.Flags().BoolVar(&opts.IsUser, "user", false, "Treat --org as a user instead of organization")

	// Authentication
	cmd.Flags().StringVar(&opts.Token, "token", "", "API token for authentication")
	cmd.Flags().StringVar(&opts.BaseURL, "base-url", "", "Base URL for self-hosted instances (API endpoint)")

	// Clone options
	cmd.Flags().StringVar(&opts.CloneProto, "clone-proto", opts.CloneProto, "Clone protocol: ssh, https")
	cmd.Flags().IntVar(&opts.SSHPort, "ssh-port", 0, "Custom SSH port (0 = default 22)")
	cmd.Flags().StringVar(&opts.SSHKeyPath, "ssh-key", "", "SSH private key file path")
	cmd.Flags().StringVar(&opts.SSHKeyContent, "ssh-key-content", "", "SSH private key content (use env var for security)")

	// Sync options
	cmd.Flags().StringVar(&opts.Strategy, "sync-strategy", opts.Strategy, "Sync strategy (reset, pull, rebase, fetch)")
	cmd.Flags().IntVar(&opts.Parallel, "parallel", opts.Parallel, "Number of parallel workers")
	cmd.Flags().IntVar(&opts.MaxRetries, "max-retries", opts.MaxRetries, "Max retry attempts")
	cmd.Flags().BoolVar(&opts.Resume, "resume", false, "Resume from previous state")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Show what would be done without executing")
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "State file for resume support")

	// Filtering options
	cmd.Flags().BoolVar(&opts.IncludeArchived, "include-archived", false, "Include archived repositories")
	cmd.Flags().BoolVar(&opts.IncludeForks, "include-forks", false, "Include forked repositories")
	cmd.Flags().BoolVar(&opts.IncludePrivate, "include-private", true, "Include private repositories")
	cmd.Flags().BoolVar(&opts.CleanupOrphans, "cleanup-orphans", false, "Delete directories not in organization")

	// GitLab specific
	cmd.Flags().BoolVar(&opts.IncludeSubgroups, "include-subgroups", false, "Include subgroups (GitLab only)")
	cmd.Flags().StringVar(&opts.SubgroupMode, "subgroup-mode", opts.SubgroupMode, "Subgroup mode: flat (dash-separated) or nested (directories)")

	// Metadata filters
	cmd.Flags().StringVar(&opts.FilterLanguage, "language", "", "Filter by language (comma-separated, e.g., go,rust)")
	cmd.Flags().IntVar(&opts.FilterMinStars, "min-stars", 0, "Minimum star count")
	cmd.Flags().IntVar(&opts.FilterMaxStars, "max-stars", 0, "Maximum star count (0 = unlimited)")
	cmd.Flags().StringVar(&opts.FilterLastPush, "last-push-within", "", "Filter by recent activity (e.g., 7d, 30d, 6M, 1y)")
	cmd.Flags().StringSliceVar(&opts.FilterInclude, "include", nil, "Include repos matching regex (name or full path; can be repeated)")
	cmd.Flags().StringSliceVar(&opts.FilterExclude, "exclude", nil, "Exclude repos matching regex (name or full path; can be repeated)")

	// Required flags
	_ = cmd.MarkFlagRequired("provider")
	_ = cmd.MarkFlagRequired("org")
	_ = cmd.MarkFlagRequired("path")

	return cmd
}

func (f CommandFactory) runFromForge(cmd *cobra.Command, opts *FromForgeOptions) error {
	ctx := cmd.Context()

	// Validate clone protocol
	if opts.CloneProto != "ssh" && opts.CloneProto != "https" {
		return fmt.Errorf("invalid --clone-proto: %s (must be ssh or https)", opts.CloneProto)
	}

	// Validate subgroup mode
	if opts.SubgroupMode != "flat" && opts.SubgroupMode != "nested" {
		return fmt.Errorf("invalid --subgroup-mode: %s (must be flat or nested)", opts.SubgroupMode)
	}

	// Build metadata filter
	metadataFilter, err := BuildFilterFromOptions(
		opts.FilterLanguage,
		opts.FilterMinStars,
		opts.FilterMaxStars,
		opts.FilterLastPush,
	)
	if err != nil {
		return err
	}

	if _, err := reposync.NewRepositoryPatternFilter(opts.FilterInclude, opts.FilterExclude); err != nil {
		return err
	}

	// Create provider
	forgeProvider, err := createFromForgeProvider(opts)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	// Parse strategy
	strategy, err := reposync.ParseStrategy(opts.Strategy)
	if err != nil {
		return fmt.Errorf("invalid strategy: %w", err)
	}

	// Print warnings for provider-specific filter limitations
	PrintWarningsForProvider(opts.Provider, metadataFilter, cmd.OutOrStdout())

	// Create ForgePlanner with authentication config
	plannerConfig := reposync.ForgePlannerConfig{
		TargetPath:       opts.TargetPath,
		Organization:     opts.Organization,
		IsUser:           opts.IsUser,
		IncludeArchived:  opts.IncludeArchived,
		IncludeForks:     opts.IncludeForks,
		IncludePrivate:   opts.IncludePrivate,
		CloneProto:       opts.CloneProto,
		SSHPort:          opts.SSHPort,
		IncludeSubgroups: opts.IncludeSubgroups,
		SubgroupMode:     opts.SubgroupMode,
		Auth: reposync.AuthConfig{
			Token:         opts.Token,
			Provider:      opts.Provider,
			SSHKeyPath:    opts.SSHKeyPath,
			SSHKeyContent: opts.SSHKeyContent,
			SSHPort:       opts.SSHPort,
		},
		// Metadata filters
		FilterLanguages:       metadataFilter.Languages,
		FilterMinStars:        metadataFilter.MinStars,
		FilterMaxStars:        metadataFilter.MaxStars,
		FilterLastPushAfter:   metadataFilter.LastPushAfter,
		FilterIncludePatterns: opts.FilterInclude,
		FilterExcludePatterns: opts.FilterExclude,
	}

	planner := reposync.NewForgePlanner(forgeProvider, plannerConfig)

	// Create orchestrator with ForgePlanner
	executor := reposync.GitExecutor{}
	orchestrator := reposync.NewOrchestrator(planner, executor, nil)

	// Build plan request
	planReq := reposync.PlanRequest{
		Options: reposync.PlanOptions{
			DefaultStrategy: strategy,
			CleanupOrphans:  opts.CleanupOrphans,
		},
	}

	if opts.CleanupOrphans {
		planReq.Options.Roots = []string{opts.TargetPath}
	}

	// Build run options
	runOpts := reposync.RunOptions{
		Parallel:   opts.Parallel,
		MaxRetries: opts.MaxRetries,
		Resume:     opts.Resume,
		DryRun:     opts.DryRun,
	}

	if opts.Resume && opts.StateFile == "" {
		return fmt.Errorf("resume requested but no --state-file provided")
	}

	// Progress and state
	progress := ConsoleProgressSink{Out: cmd.OutOrStdout()}

	var state reposync.StateStore
	if opts.StateFile != "" {
		state = reposync.NewFileStateStore(opts.StateFile)
	}

	// Run
	result, err := orchestrator.Run(ctx, reposync.RunRequest{
		PlanRequest: planReq,
		RunOptions:  runOpts,
		Progress:    progress,
		State:       state,
	})
	if err != nil {
		return fmt.Errorf("forge sync failed: %w", err)
	}

	// Print summary
	fmt.Fprintf(cmd.OutOrStdout(), "\nSync completed: %d succeeded, %d failed, %d skipped\n",
		len(result.Succeeded), len(result.Failed), len(result.Skipped))

	if failed := len(result.Failed); failed > 0 {
		total := failed + len(result.Succeeded) + len(result.Skipped)
		return fmt.Errorf("%d of %d repositories failed to sync", failed, total)
	}
	return nil
}

// NewForgeProviderWithAuth is the single owner of forge-provider construction.
// It maps a provider name to the concrete github/gitlab/gitea provider and
// returns it as provider.ProviderWithAuth — the richest common interface, so
// callers that need token validation or rate-limit access (e.g. doctor) get
// them, while sync callers narrow it to reposync.ForgeProvider via
// CreateForgeProviderRaw. Keeping this switch in exactly one place is the point:
// per-provider wiring — notably the GitLab SSH port — can no longer drift
// between the from_forge, wizard, config-loader and doctor paths.
func NewForgeProviderWithAuth(providerName, token, baseURL string, sshPort int) (provider.ProviderWithAuth, error) {
	switch providerName {
	case "github":
		p, err := github.NewProvider(token, baseURL)
		if err != nil {
			return nil, err
		}
		return p, nil

	case "gitlab":
		p, err := gitlab.NewProviderWithOptions(gitlab.ProviderOptions{
			Token:   token,
			BaseURL: baseURL,
			SSHPort: sshPort, // only GitLab honors a custom SSH port
		})
		if err != nil {
			return nil, err
		}
		return p, nil

	case "gitea":
		p, err := gitea.NewProvider(token, baseURL)
		if err != nil {
			return nil, err
		}
		return p, nil

	default:
		return nil, fmt.Errorf("unsupported provider: %s (supported: github, gitlab, gitea)", providerName)
	}
}

// createFromForgeProvider adapts FromForgeOptions to the shared constructor.
func createFromForgeProvider(opts *FromForgeOptions) (reposync.ForgeProvider, error) {
	return CreateForgeProviderRaw(opts.Provider, opts.Token, opts.BaseURL, opts.SSHPort)
}

// forgeProviderAdapter adapts gitforge providers to the narrow ForgeProvider
// interface used by the reposync core (Name + repo listing only).
type forgeProviderAdapter struct {
	provider.Provider
}

// CreateForgeProviderRaw creates a provider from raw strings and narrows it to
// reposync.ForgeProvider. It is the entry point for sync callers (and other
// packages like workspacecli); doctor uses NewForgeProviderWithAuth directly
// when it needs the auth-capable interface.
func CreateForgeProviderRaw(providerName, token, baseURL string, sshPort int) (reposync.ForgeProvider, error) {
	p, err := NewForgeProviderWithAuth(providerName, token, baseURL, sshPort)
	if err != nil {
		return nil, err
	}
	return forgeProviderAdapter{p}, nil
}

// CreateProviderFromSource creates a forge provider from config types with profile fallback.
// This handles the common pattern of extracting provider settings from ForgeSource,
// falling back to Workspace settings, then to profile settings from the config chain.
func CreateProviderFromSource(src *config.ForgeSource, ws *config.Workspace, cfg *config.Config) (reposync.ForgeProvider, error) { //nolint:gocognit // multi-layer profile fallback chain — extracting helpers would obscure the precedence logic
	// Extract values from source
	token := src.Token
	baseURL := src.BaseURL
	sshPort := ws.SSHPort
	providerName := src.Provider

	// Fallback to workspace's profile values if not set in source
	if ws.Profile != "" && cfg != nil {
		profile := config.GetProfileFromChain(cfg, ws.Profile)
		if profile != nil {
			if token == "" {
				token = profile.Token
			}
			if baseURL == "" {
				baseURL = profile.BaseURL
			}
			if sshPort == 0 {
				sshPort = profile.SSHPort
			}
			if providerName == "" {
				providerName = profile.Provider
			}
		}
	}

	// Fallback to root config's active profile and direct settings
	if cfg != nil {
		if cfg.Profile != "" {
			profile := config.GetProfileFromChain(cfg, cfg.Profile)
			if profile != nil {
				if token == "" {
					token = profile.Token
				}
				if baseURL == "" {
					baseURL = profile.BaseURL
				}
				if sshPort == 0 {
					sshPort = profile.SSHPort
				}
				if providerName == "" {
					providerName = profile.Provider
				}
			}
		}
		if token == "" {
			token = cfg.Token
		}
		if baseURL == "" {
			baseURL = cfg.BaseURL
		}
		if providerName == "" {
			providerName = cfg.Provider
		}
	}

	return CreateForgeProviderRaw(providerName, token, baseURL, sshPort)
}
