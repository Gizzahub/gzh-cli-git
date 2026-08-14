package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

var (
	branchListAll   bool
	branchListFlags BulkCommandFlags
)

// branchListCmd lists branches in repositories.
var branchListCmd = &cobra.Command{
	Use:   "list [directory]",
	Short: "List branches in repositories",
	Long: cliutil.QuickStartHelp(`  # List branches in current directory
  gz-git branch list

  # List all branches (including remote)
  gz-git branch list -a

  # List branches in specific directory
  gz-git branch list ~/projects

  # List with filters
  gz-git branch list --include "gzh-cli.*" .`),
	RunE: runBranchList,
}

func init() {
	branchCmd.AddCommand(branchListCmd)

	// Branch-specific flags
	branchListCmd.Flags().BoolVarP(&branchListAll, "all", "a", false, "show remote branches too")

	// Bulk flags
	addBranchListBulkFlags(branchListCmd)
}

func addBranchListBulkFlags(cmd *cobra.Command) {
	addBulkFlagsWithOpts(cmd, &branchListFlags, BulkFlagOptions{SkipDryRun: true, SkipFormat: true, SkipWatch: true, SkipFetch: true})
}

func runBranchList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	directory, err := validateBulkDirectory(args)
	if err != nil {
		return err
	}

	return runBulkBranchList(ctx, directory)
}

func runBulkBranchList(ctx context.Context, directory string) error {
	client := repository.NewClient()

	opts := repository.BulkBranchListOptions{
		Directory:      directory,
		Parallel:       branchListFlags.Parallel,
		MaxDepth:       branchListFlags.Depth,
		All:            branchListAll,
		IncludePattern: branchListFlags.Include,
		ExcludePattern: branchListFlags.Exclude,
		Logger:         repository.NewNoopLogger(),
	}

	if !quiet {
		fmt.Printf("Scanning for repositories in %s...\n", directory)
	}

	result, err := client.BulkBranchList(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk branch list failed: %w", err)
	}

	printBulkBranchListResult(result, branchListAll)
	return nil
}

func printBulkBranchListResult(result *repository.BulkBranchListResult, showRemote bool) {
	fmt.Println()
	fmt.Println("=== Branch List Results ===")
	fmt.Printf("Scanned: %d repositories | Duration: %.1fs\n",
		result.TotalScanned, result.Duration.Seconds())
	fmt.Println()

	// Print each repository
	for _, repo := range result.Repositories {
		if repo.Status == repository.StatusError {
			fmt.Printf("%s ✗ %s\n", repo.RelativePath, repo.Error)
			continue
		}

		if len(repo.Branches) == 0 {
			continue
		}

		// Repository header with current branch
		fmt.Printf("%s (%s)\n", repo.RelativePath, repo.CurrentBranch)

		// Find max length for alignment
		maxLen := 0
		for _, b := range repo.Branches {
			if len(b.Name) > maxLen {
				maxLen = len(b.Name)
			}
		}
		if maxLen > 30 {
			maxLen = 30
		}

		// Print branches
		for _, b := range repo.Branches {
			printBulkBranchLine(b, maxLen)
		}
		fmt.Println()
	}

	// Summary
	if showRemote && result.TotalRemoteCount > 0 {
		fmt.Printf("Summary: %d branches (%d local, %d remote) across %d repositories\n",
			result.TotalBranchCount, result.TotalLocalCount, result.TotalRemoteCount, result.TotalProcessed)
	} else {
		fmt.Printf("Summary: %d branches across %d repositories\n",
			result.TotalLocalCount, result.TotalProcessed)
	}
}

func printBulkBranchLine(b repository.BranchInfo, maxLen int) {
	// Current branch marker
	marker := " "
	if b.IsHead {
		marker = "*"
	}

	// Format name
	name := b.Name
	if len(name) > maxLen {
		name = name[:maxLen-3] + "..."
	}

	// Short SHA
	sha := ""
	if len(b.SHA) >= 7 {
		sha = b.SHA[:7]
	}

	// Upstream info (compact for bulk mode)
	upstreamInfo := ""
	if b.Upstream != "" {
		upstreamInfo = fmt.Sprintf("[%s", b.Upstream)
		if b.AheadBy > 0 || b.BehindBy > 0 {
			parts := []string{}
			if b.AheadBy > 0 {
				parts = append(parts, fmt.Sprintf("%d ahead", b.AheadBy))
			}
			if b.BehindBy > 0 {
				parts = append(parts, fmt.Sprintf("%d behind", b.BehindBy))
			}
			upstreamInfo += ": " + strings.Join(parts, ", ")
		}
		upstreamInfo += "]"
	}

	fmt.Printf("%s %-*s %s  %s\n", marker, maxLen, name, sha, upstreamInfo)
}
