package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-git/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-git/pkg/merge"
	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

var (
	detectIncludeBinary bool
	detectBaseCommit    string
)

// detectCmd represents the merge detect command
var detectCmd = &cobra.Command{
	Use:   "detect <source> <target>",
	Short: "Detect potential merge conflicts",
	Long: `Analyze potential conflicts between source and target branches.

This command performs a dry-run merge analysis to identify files that would
conflict during an actual merge, without modifying your working directory.`,
	Example: `  # Detect conflicts between branches
  gzh-git merge detect feature/new-feature main

  # Include binary file conflicts
  gzh-git merge detect feature/new-feature main --include-binary

  # Detect with specific base commit
  gzh-git merge detect feature/new-feature main --base abc123`,
	Args: cobra.ExactArgs(2),
	RunE: runMergeDetect,
}

func init() {
	mergeCmd.AddCommand(detectCmd)

	detectCmd.Flags().BoolVar(&detectIncludeBinary, "include-binary", false, "include binary file conflicts")
	detectCmd.Flags().StringVar(&detectBaseCommit, "base", "", "base commit for three-way merge")
}

func runMergeDetect(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	source := args[0]
	target := args[1]

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

	// Create conflict detector
	detector := merge.NewConflictDetector(gitcmd.NewExecutor())

	// Build options
	opts := merge.DetectOptions{
		Source:        source,
		Target:        target,
		BaseCommit:    detectBaseCommit,
		IncludeBinary: detectIncludeBinary,
	}

	if !quiet {
		fmt.Printf("Analyzing merge: %s → %s\n", source, target)
	}

	// Detect conflicts
	report, err := detector.Detect(ctx, repo, opts)
	if err != nil {
		return fmt.Errorf("failed to detect conflicts: %w", err)
	}

	// Display report
	if !quiet {
		fmt.Println()
		if report.TotalConflicts > 0 {
			fmt.Printf("⚠️  Found %d potential conflicts:\n\n", report.TotalConflicts)
			for _, conflict := range report.Conflicts {
				fmt.Printf("  %s: %s\n", conflict.ConflictType, conflict.FilePath)
				if verbose && conflict.Description != "" {
					fmt.Printf("     Reason: %s\n", conflict.Description)
				}
			}
			fmt.Println()
			fmt.Printf("Difficulty: %s\n", report.Difficulty)
			fmt.Printf("Auto-resolvable: %d/%d\n", report.CanAutoResolve, report.TotalConflicts)
		} else {
			fmt.Println("✅ No conflicts detected - merge should be clean!")
		}

		// Check fast-forward
		canFF, err := detector.CanFastForward(ctx, repo, source, target)
		if err == nil && canFF {
			fmt.Println()
			fmt.Println("💡 Tip: This merge can be fast-forwarded")
		}
	}

	if report.TotalConflicts > 0 {
		return fmt.Errorf("merge would result in conflicts")
	}

	return nil
}
