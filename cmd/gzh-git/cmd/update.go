package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

var updateCmd = &cobra.Command{
	Use:   "update <repository-url> [target-path]",
	Short: "Clone repository or update existing one with configurable strategies",
	Long: `Clone a repository if it doesn't exist, or update it using the specified strategy.

This command provides intelligent repository management by automatically detecting
whether a repository exists at the target path and taking the appropriate action.

If target-path is not provided, the repository name will be extracted from the URL
and used as the directory name (similar to 'git clone' behavior).

Available Strategies:
  rebase  - Rebase local changes on top of remote changes (default)
  reset   - Hard reset to match remote state (discards local changes)
  clone   - Remove existing directory and perform fresh clone
  skip    - Leave existing repository unchanged
  pull    - Standard git pull (merge remote changes)
  fetch   - Only fetch remote changes without updating working directory

Examples:
  # Clone into directory named from repository (e.g., 'repo')
  gzh-git update https://github.com/user/repo.git

  # Clone or rebase existing repository with explicit path
  gzh-git update https://github.com/user/repo.git ./my-repo

  # Force fresh clone by removing existing directory
  gzh-git update --strategy clone https://github.com/user/repo.git

  # Update existing repository with hard reset (discard local changes)
  gzh-git update --strategy reset https://github.com/user/repo.git ./repo

  # Skip existing repositories (useful for automation)
  gzh-git update --strategy skip https://github.com/user/repo.git

  # Clone specific branch with shallow history
  gzh-git update --branch develop --depth 1 https://github.com/user/repo.git`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runUpdate,
}

var updateOpts struct {
	strategy string
	branch   string
	depth    int
	force    bool
	verbose  bool
}

func init() {
	updateCmd.Flags().StringVarP(&updateOpts.strategy, "strategy", "s", "rebase",
		"Strategy when repository exists: rebase, reset, clone, skip, pull, fetch")
	updateCmd.Flags().StringVarP(&updateOpts.branch, "branch", "b", "",
		"Specific branch to clone/checkout (default: repository default branch)")
	updateCmd.Flags().IntVarP(&updateOpts.depth, "depth", "d", 0,
		"Create shallow clone with specified depth (0 for full history)")
	updateCmd.Flags().BoolVarP(&updateOpts.force, "force", "f", false,
		"Force operation even if it might be destructive")
	updateCmd.Flags().BoolVarP(&updateOpts.verbose, "verbose", "v", false,
		"Enable verbose logging")

	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	repoURL := args[0]
	var targetPath string

	// If target path is not provided, extract repository name from URL
	if len(args) > 1 {
		targetPath = args[1]
	} else {
		repoName, err := repository.ExtractRepoNameFromURL(repoURL)
		if err != nil {
			return fmt.Errorf("failed to extract repository name from URL: %w", err)
		}
		targetPath = repoName
	}

	// Create logger
	var logger repository.Logger
	if updateOpts.verbose {
		logger = repository.NewWriterLogger(os.Stdout)
	} else {
		logger = repository.NewNoopLogger()
	}

	// Create client
	client := repository.NewClient(repository.WithClientLogger(logger))

	// Prepare options
	opts := repository.CloneOrUpdateOptions{
		URL:         repoURL,
		Destination: targetPath,
		Strategy:    repository.UpdateStrategy(updateOpts.strategy),
		Branch:      updateOpts.branch,
		Depth:       updateOpts.depth,
		Force:       updateOpts.force,
		Logger:      logger,
	}

	if updateOpts.verbose {
		fmt.Printf("Repository URL: %s\n", opts.URL)
		fmt.Printf("Target Path: %s\n", opts.Destination)
		fmt.Printf("Strategy: %s\n", opts.Strategy)
		if opts.Branch != "" {
			fmt.Printf("Branch: %s\n", opts.Branch)
		}
		if opts.Depth > 0 {
			fmt.Printf("Depth: %d\n", opts.Depth)
		}
	}

	// Execute clone-or-update
	result, err := client.CloneOrUpdate(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to clone or update repository: %w", err)
	}

	// Print result based on action
	switch result.Action {
	case "cloned":
		fmt.Printf("✅ %s\n", result.Message)
	case "skipped":
		fmt.Printf("⏭️  %s\n", result.Message)
	case "fetched":
		fmt.Printf("📥 %s\n", result.Message)
	case "pulled":
		fmt.Printf("🔄 %s\n", result.Message)
	case "reset":
		fmt.Printf("🔄 %s\n", result.Message)
	case "rebased":
		fmt.Printf("🔄 %s\n", result.Message)
	default:
		fmt.Printf("✅ %s\n", result.Message)
	}

	return nil
}
