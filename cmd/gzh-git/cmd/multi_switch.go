package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

var (
	multiSwitchFlags  BulkCommandFlags
	multiSwitchCreate bool
	multiSwitchForce  bool
)

// multiSwitchCmd represents the multi switch command
var multiSwitchCmd = &cobra.Command{
	Use:   "switch <branch> [directory]",
	Short: "Switch branches across multiple repositories",
	Long: `Scan for Git repositories and switch their branches in parallel.

This command recursively scans the specified directory (or current directory)
for Git repositories and switches them to the specified branch.

By default:
  - Scans 1 directory level deep
  - Processes 5 repositories in parallel
  - Skips repositories with uncommitted changes
  - Skips repositories where the branch doesn't exist

The command will skip repositories that:
  - Already on the target branch
  - Have uncommitted changes (unless --force)
  - Have rebase or merge in progress
  - Don't have the target branch (unless --create)`,
	Example: `  # Switch all repos to develop branch
  gz-git multi switch develop

  # Preview what would happen (dry-run)
  gz-git multi switch main --dry-run

  # Create branch if it doesn't exist
  gz-git multi switch feature/new --create

  # Switch with custom directory depth
  gz-git multi switch develop -d 2

  # Process more repos in parallel
  gz-git multi switch main -j 10

  # Only include specific repos
  gz-git multi switch develop --include "gzh-cli-.*"

  # Exclude certain repos
  gz-git multi switch develop --exclude ".*-mcp-.*"

  # Force switch (discards uncommitted changes - DANGEROUS!)
  gz-git multi switch main --force`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runMultiSwitch,
}

func init() {
	multiCmd.AddCommand(multiSwitchCmd)

	// Common bulk operation flags (except watch/interval which don't apply)
	multiSwitchCmd.Flags().IntVarP(&multiSwitchFlags.Depth, "depth", "d", repository.DefaultBulkMaxDepth, "directory depth to scan")
	multiSwitchCmd.Flags().IntVarP(&multiSwitchFlags.Parallel, "parallel", "j", repository.DefaultBulkParallel, "number of parallel operations")
	multiSwitchCmd.Flags().BoolVarP(&multiSwitchFlags.DryRun, "dry-run", "n", false, "show what would be done without doing it")
	multiSwitchCmd.Flags().BoolVarP(&multiSwitchFlags.IncludeSubmodules, "recursive", "r", false, "recursively include nested repositories and submodules")
	multiSwitchCmd.Flags().StringVar(&multiSwitchFlags.Include, "include", "", "regex pattern to include repositories")
	multiSwitchCmd.Flags().StringVar(&multiSwitchFlags.Exclude, "exclude", "", "regex pattern to exclude repositories")
	multiSwitchCmd.Flags().StringVar(&multiSwitchFlags.Format, "format", "default", "output format: default, compact")

	// Switch-specific flags
	multiSwitchCmd.Flags().BoolVarP(&multiSwitchCreate, "create", "c", false, "create branch if it doesn't exist")
	multiSwitchCmd.Flags().BoolVarP(&multiSwitchForce, "force", "f", false, "force switch even with uncommitted changes (DANGEROUS!)")
}

func runMultiSwitch(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get branch name (required)
	branch := args[0]

	// Get directory (optional, defaults to current)
	directory := "."
	if len(args) > 1 {
		directory = args[1]
	}

	// Validate directory exists
	if _, err := os.Stat(directory); err != nil {
		return fmt.Errorf("directory does not exist: %s", directory)
	}

	// Validate depth
	if err := validateBulkDepth(cmd, multiSwitchFlags.Depth); err != nil {
		return err
	}

	// Create client
	client := repository.NewClient()

	// Create logger for verbose mode
	logger := createBulkLogger(verbose)

	// Build options
	opts := repository.BulkSwitchOptions{
		Directory:         directory,
		Branch:            branch,
		Parallel:          multiSwitchFlags.Parallel,
		MaxDepth:          multiSwitchFlags.Depth,
		DryRun:            multiSwitchFlags.DryRun,
		Verbose:           verbose,
		Create:            multiSwitchCreate,
		Force:             multiSwitchForce,
		IncludeSubmodules: multiSwitchFlags.IncludeSubmodules,
		IncludePattern:    multiSwitchFlags.Include,
		ExcludePattern:    multiSwitchFlags.Exclude,
		Logger:            logger,
		ProgressCallback:  createProgressCallback("Switching", multiSwitchFlags.Format, quiet),
	}

	// Print header
	if !quiet {
		if multiSwitchFlags.DryRun {
			fmt.Printf("Scanning for repositories in %s (depth: %d) [DRY-RUN]...\n", directory, multiSwitchFlags.Depth)
		} else {
			fmt.Printf("Scanning for repositories in %s (depth: %d)...\n", directory, multiSwitchFlags.Depth)
		}
	}

	// Execute bulk switch
	result, err := client.BulkSwitch(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk switch failed: %w", err)
	}

	// Display results
	if !quiet {
		displaySwitchResults(result)
	}

	// Return error if there were any failures
	if result.Summary[repository.StatusError] > 0 {
		return fmt.Errorf("%d repositories failed to switch", result.Summary[repository.StatusError])
	}

	return nil
}

// displaySwitchResults displays the results of a bulk switch operation
func displaySwitchResults(result *repository.BulkSwitchResult) {
	fmt.Println()
	fmt.Printf("Target branch: %s\n", result.TargetBranch)
	fmt.Printf("Scanned: %d repositories\n", result.TotalScanned)
	fmt.Printf("Processed: %d repositories\n", result.TotalProcessed)
	fmt.Println()

	// Display each repository result
	for _, repo := range result.Repositories {
		displaySwitchRepoResult(repo)
	}

	// Display summary
	fmt.Println()
	displaySwitchSummary(result)
	fmt.Printf("Duration: %s\n", result.Duration.Round(time.Millisecond))
}

// displaySwitchRepoResult displays a single repository switch result
func displaySwitchRepoResult(repo repository.RepositorySwitchResult) {
	var icon string
	switch repo.Status {
	case repository.StatusSwitched:
		icon = "+"
	case repository.StatusBranchCreated:
		icon = "+"
	case repository.StatusAlreadyOnBranch:
		icon = "="
	case repository.StatusWouldSwitch:
		icon = "~"
	case repository.StatusDirty:
		icon = "!"
	case repository.StatusBranchNotFound:
		icon = "?"
	case repository.StatusRebaseInProgress, repository.StatusMergeInProgress:
		icon = "!"
	default:
		icon = "x"
	}

	fmt.Printf("[%s] %-40s %s\n", icon, repo.RelativePath, repo.Message)
}

// displaySwitchSummary displays the summary of bulk switch results
func displaySwitchSummary(result *repository.BulkSwitchResult) {
	fmt.Print("Summary: ")

	parts := []string{}

	if count := result.Summary[repository.StatusSwitched]; count > 0 {
		parts = append(parts, fmt.Sprintf("%d switched", count))
	}
	if count := result.Summary[repository.StatusBranchCreated]; count > 0 {
		parts = append(parts, fmt.Sprintf("%d created", count))
	}
	if count := result.Summary[repository.StatusAlreadyOnBranch]; count > 0 {
		parts = append(parts, fmt.Sprintf("%d already", count))
	}
	if count := result.Summary[repository.StatusWouldSwitch]; count > 0 {
		parts = append(parts, fmt.Sprintf("%d would-switch", count))
	}
	if count := result.Summary[repository.StatusDirty]; count > 0 {
		parts = append(parts, fmt.Sprintf("%d dirty", count))
	}
	if count := result.Summary[repository.StatusBranchNotFound]; count > 0 {
		parts = append(parts, fmt.Sprintf("%d not-found", count))
	}
	if count := result.Summary[repository.StatusError]; count > 0 {
		parts = append(parts, fmt.Sprintf("%d errors", count))
	}

	if len(parts) == 0 {
		fmt.Println("no changes")
	} else {
		for i, part := range parts {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Print(part)
		}
		fmt.Println()
	}
}
