package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

var (
	infoFlags BulkCommandFlags
	itemLimit int
)

// infoCmd represents the info command
var infoCmd = &cobra.Command{
	Use:   "info [directory]",
	Short: "Display repository information",
	Long: `Display information about Git repositories in the specified directory.

Scans for repositories and shows metadata such as current branch, remote URL,
and status summary.

By default:
  - Scans 1 directory level deep
  - Processes 10 repositories in parallel
  - Shows result in a table format`,
	Args: cobra.MaximumNArgs(1),
	Example: `  # Show info for current directory
  gz-git info

  # Show info for specific repository
  gz-git info /path/to/repo

  # Verbose output with more details
  gz-git info --verbose`,
	RunE: runInfo,
}

func init() {
	rootCmd.AddCommand(infoCmd)

	// Common bulk operation flags
	// Add bulk flags
	addBulkFlags(infoCmd, &infoFlags)

	// Add info-specific flags
	infoCmd.Flags().IntVar(&itemLimit, "limit", 10, "max items to show in lists (branches, remotes)")
}

func runInfo(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Load config with profile support
	effective, _ := LoadEffectiveConfig(cmd, nil)
	if effective != nil {
		if !cmd.Flags().Changed("parallel") && effective.Parallel > 0 {
			infoFlags.Parallel = effective.Parallel
		}
		if verbose {
			PrintConfigSources(cmd, effective)
		}
	}

	// Validate and parse directory
	directory, err := validateBulkDirectory(args)
	if err != nil {
		return err
	}

	// Validate depth
	if err := validateBulkDepth(cmd, infoFlags.Depth); err != nil {
		return err
	}

	// Create client
	client := repository.NewClient()

	// Create logger for verbose mode
	logger := createBulkLogger(verbose)

	if shouldShowProgress(infoFlags.Format, quiet) {
		printScanningMessage(directory, infoFlags.Depth, infoFlags.Parallel, false)
	}

	// Setup bulk scan options
	bulkOpts := repository.BulkStatusOptions{
		Directory:         directory,
		Parallel:          infoFlags.Parallel,
		MaxDepth:          infoFlags.Depth,
		Verbose:           verbose,
		IncludeSubmodules: infoFlags.IncludeSubmodules,
		IncludePattern:    infoFlags.Include,
		ExcludePattern:    infoFlags.Exclude,
		Logger:            logger,
	}

	// Execute scan
	result, err := client.BulkStatus(ctx, bulkOpts)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	// Display results
	if infoFlags.Format == "json" || infoFlags.Format == "llm" {
		displayInfoResultsStructured(result, infoFlags.Format)
		return nil
	}

	displayInfoResultsDetailed(result)

	return nil
}

func displayInfoResultsDetailed(result *repository.BulkStatusResult) {
	if len(result.Repositories) == 0 {
		fmt.Println("No repositories found.")
		return
	}

	fmt.Println()
	fmt.Printf("found %d repositories (scanned in %s)\n", len(result.Repositories), result.Duration.Round(10*time.Millisecond))

	for _, repo := range result.Repositories {
		fmt.Println()
		// Header with nice formatting
		// 📦 repo-name (relative/path)
		path := filepath.Base(repo.Path)
		if verbose {
			path = repo.RelativePath
		}
		fmt.Printf("📦 %s\n", path)
		fmt.Println(strings.Repeat("-", 60))

		// 1. Current Branch & Hash
		branchInfo := repo.Branch
		if branchInfo == "" {
			branchInfo = "DETACHED"
		}
		if repo.HeadSHA != "" {
			branchInfo += fmt.Sprintf(" (%s)", repo.HeadSHA)
		}
		fmt.Printf("  Current Branch: %s\n", branchInfo)

		// 2. Version
		if repo.Describe != "" {
			fmt.Printf("  Version:        %s\n", repo.Describe)
		}

		// 3. Status
		status := repo.Status
		if repo.Status != "clean" && repo.TrackedChangedFiles > 0 {
			status = fmt.Sprintf("%s (%d uncommitted)", repo.Status, repo.TrackedChangedFiles)
		}
		if repo.StashCount > 0 {
			status += fmt.Sprintf(", %d stash(es)", repo.StashCount)
		}
		fmt.Printf("  Status:         %s\n", status)

		// 4. Update Info
		if repo.LastCommitDate != "" {
			msg := repo.LastCommitMsg
			if len(msg) > 50 {
				msg = msg[:47] + "..."
			}
			fmt.Printf("  Last Update:    %s (%s)\n", repo.LastCommitDate, msg)
			if verbose {
				fmt.Printf("  Author:         %s\n", repo.LastCommitAuthor)
			}
		}

		// 5. Remotes (Full List with Limit)
		if len(repo.Remotes) > 0 {
			fmt.Println("  Remotes:")
			// Sort keys for consistent output
			var keys []string
			for k := range repo.Remotes {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			displayCount := 0
			for _, k := range keys {
				if displayCount >= itemLimit {
					remaining := len(keys) - displayCount
					fmt.Printf("    ... (%d more)\n", remaining)
					break
				}
				fmt.Printf("    - %-10s %s\n", k, repo.Remotes[k])
				displayCount++
			}
		} else {
			fmt.Println("  Remotes:        (none)")
		}

		// 6. Local Branches (Full List with Limit)
		if len(repo.LocalBranches) > 0 {
			// Sort branches
			sort.Strings(repo.LocalBranches)

			branchesStr := ""
			if len(repo.LocalBranches) <= itemLimit {
				branchesStr = strings.Join(repo.LocalBranches, ", ")
			} else {
				visible := repo.LocalBranches[:itemLimit]
				branchesStr = strings.Join(visible, ", ") + fmt.Sprintf(", ... (%d more)", len(repo.LocalBranches)-itemLimit)
			}

			fmt.Printf("  Branches (%d):   %s\n", len(repo.LocalBranches), branchesStr)
		}
	}
	fmt.Println()
}

func displayInfoResultsStructured(result *repository.BulkStatusResult, format string) {
	// Re-use status JSON output format as it contains all info
	displayStatusResultsStructured(result, format)
}
