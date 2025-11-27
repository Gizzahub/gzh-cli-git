package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-git/pkg/branch"
	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

var (
	listAll      bool
	listRemote   bool
	listMerged   bool
	listNoMerged bool
)

// listCmd represents the branch list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List branches",
	Long: `List all local and remote branches.

By default, shows only local branches. Use flags to show remote branches,
merged branches, or unmerged branches.`,
	Example: `  # List local branches
  gzh-git branch list

  # List all branches (local and remote)
  gzh-git branch list --all

  # List only remote branches
  gzh-git branch list --remote

  # List only merged branches
  gzh-git branch list --merged

  # List only unmerged branches
  gzh-git branch list --no-merged`,
	RunE: runBranchList,
}

func init() {
	branchCmd.AddCommand(listCmd)

	listCmd.Flags().BoolVarP(&listAll, "all", "a", false, "list both local and remote branches")
	listCmd.Flags().BoolVarP(&listRemote, "remote", "r", false, "list only remote branches")
	listCmd.Flags().BoolVar(&listMerged, "merged", false, "list only merged branches")
	listCmd.Flags().BoolVar(&listNoMerged, "no-merged", false, "list only unmerged branches")
}

func runBranchList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get repository path
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Create client
	client := repository.NewClient()

	// Check if it's a repository
	if !client.IsRepository(ctx, absPath) {
		return fmt.Errorf("not a git repository: %s", absPath)
	}

	// Open repository
	repo, err := client.Open(ctx, absPath)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Create branch manager
	mgr := branch.NewManager()

	// List branches based on flags
	opts := branch.ListOptions{
		All: listAll,
	}

	branches, err := mgr.List(ctx, repo, opts)
	if err != nil {
		return fmt.Errorf("failed to list branches: %w", err)
	}

	// Filter branches based on flags
	var filtered []*branch.Branch
	for _, b := range branches {
		// Skip remote branches if not requested
		if !listAll && !listRemote && b.IsRemote {
			continue
		}

		// Skip local branches if only remote requested
		if listRemote && !b.IsRemote {
			continue
		}

		// Filter by merged status if requested
		if listMerged && !b.IsMerged {
			continue
		}
		if listNoMerged && b.IsMerged {
			continue
		}

		filtered = append(filtered, b)
	}

	// Display branches
	if len(filtered) == 0 {
		if !quiet {
			fmt.Println("No branches found")
		}
		return nil
	}

	if !quiet {
		fmt.Printf("\n📋 Branches (%d):\n\n", len(filtered))
	}

	for _, b := range filtered {
		// Show current branch indicator
		indicator := "  "
		if b.IsHead {
			indicator = "* "
		}

		// Format branch name
		name := b.Name
		if b.IsRemote {
			name = fmt.Sprintf("remotes/%s", name)
		}

		// Show branch info
		fmt.Printf("%s%s\n", indicator, name)

		// Show additional info in verbose mode
		if verbose {
			if b.Upstream != "" {
				fmt.Printf("    Tracks: %s\n", b.Upstream)
			}
			if b.IsMerged {
				fmt.Println("    Status: Merged")
			}
			if b.LastCommit != nil && b.LastCommit.SHA != "" {
				fmt.Printf("    Last commit: %s\n", b.LastCommit.SHA[:8])
			}
		}
	}

	fmt.Println()
	return nil
}
