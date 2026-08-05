package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/branch"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// cleanupBranchBulkFlags holds bulk-specific flags
var cleanupBranchBulkFlags BulkCommandFlags

var (
	cleanupBranchMerged     bool
	cleanupBranchStale      bool
	cleanupBranchGone       bool
	cleanupBranchStaleDays  int
	cleanupBranchDryRun     bool
	cleanupBranchForce      bool
	cleanupBranchYes        bool
	cleanupBranchRemote     bool
	cleanupBranchProtect    string
	cleanupBranchBaseBranch string
)

// cleanupBranchCmd represents the cleanup branch command
var cleanupBranchCmd = &cobra.Command{
	Use:   "branch [directory]",
	Short: "Clean up merged, stale, or gone branches",
	Long: cliutil.QuickStartHelp(`  # Preview merged branches in current repo
  gz-git cleanup branch --merged

  # Preview stale branches (no activity for 30 days)
  gz-git cleanup branch --stale

  # Actually delete merged branches
  gz-git cleanup branch --merged --force

  # BULK MODE: Clean up merged branches across all repos
  gz-git cleanup branch --merged --force .

  # Protect additional branches
  gz-git cleanup branch --merged --protect "staging,qa" --force`),
	Example: ``,
	RunE:    runCleanupBranch,
}

func init() {
	cleanupCmd.AddCommand(cleanupBranchCmd)

	// Cleanup-specific flags
	cleanupBranchCmd.Flags().BoolVar(&cleanupBranchMerged, "merged", false, "clean up fully merged branches")
	cleanupBranchCmd.Flags().BoolVar(&cleanupBranchStale, "stale", false, "clean up stale branches (no recent activity)")
	cleanupBranchCmd.Flags().BoolVar(&cleanupBranchGone, "gone", false, "clean up gone branches (remote deleted)")
	cleanupBranchCmd.Flags().IntVar(&cleanupBranchStaleDays, "stale-days", 30, "days threshold for stale branches")
	cleanupBranchCmd.Flags().BoolVarP(&cleanupBranchDryRun, "dry-run", "n", true, "preview changes without deleting (default: true)")
	cleanupBranchCmd.Flags().BoolVar(&cleanupBranchForce, "force", false, "actually delete branches (disables dry-run)")
	cleanupBranchCmd.Flags().BoolVarP(&cleanupBranchYes, "yes", "y", false, "skip the confirmation prompt for bulk deletion (required in a non-interactive environment)")
	cleanupBranchCmd.Flags().BoolVarP(&cleanupBranchRemote, "remote", "r", false, "also delete remote branches")
	cleanupBranchCmd.Flags().StringVar(&cleanupBranchProtect, "protect", "", "additional branches to protect (comma-separated)")
	cleanupBranchCmd.Flags().StringVar(&cleanupBranchBaseBranch, "base", "", "base branch for merge detection (default: auto-detect)")

	// Bulk operation flags (skip dry-run to avoid conflict with custom dry-run; skip recursive shorthand to avoid -r clash with --remote)
	addBulkFlagsWithOpts(cleanupBranchCmd, &cleanupBranchBulkFlags, BulkFlagOptions{SkipDryRun: true, SkipFormat: true, SkipWatch: true, SkipFetch: true, SkipRecursive: true})
	cleanupBranchCmd.Flags().BoolVar(&cleanupBranchBulkFlags.IncludeSubmodules, "recursive", false, "recursively include nested repositories and submodules")
}

func runCleanupBranch(cmd *cobra.Command, args []string) error {
	// SIGINT/SIGTERM cancels the context so an in-progress deletion stops
	// gracefully instead of being hard-killed.
	ctx, cancel := withInterruptCancel(context.Background())
	defer cancel()

	// Require at least one cleanup type
	if !cleanupBranchMerged && !cleanupBranchStale && !cleanupBranchGone {
		return fmt.Errorf("specify at least one cleanup type: --merged, --stale, or --gone")
	}

	// Force flag disables dry-run
	if cleanupBranchForce {
		cleanupBranchDryRun = false
	}

	// Build exclude list
	excludePatterns := []string{}
	if cleanupBranchProtect != "" {
		for p := range strings.SplitSeq(cleanupBranchProtect, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				excludePatterns = append(excludePatterns, p)
			}
		}
	}

	// If directory argument provided, run in bulk mode
	if len(args) > 0 {
		return runBulkCleanupBranch(ctx, args[0], excludePatterns)
	}

	// Single repository mode
	return runSingleRepoCleanupBranch(ctx, excludePatterns)
}

func runSingleRepoCleanupBranch(ctx context.Context, excludePatterns []string) error {
	repo, err := openCurrentRepo(ctx)
	if err != nil {
		return err
	}

	svc := branch.NewCleanupService()

	// Analyze branches
	analyzeOpts := branch.AnalyzeOptions{
		IncludeMerged:  cleanupBranchMerged,
		IncludeStale:   cleanupBranchStale,
		StaleThreshold: time.Duration(cleanupBranchStaleDays) * 24 * time.Hour,
		IncludeRemote:  cleanupBranchRemote,
		IncludeGone:    cleanupBranchGone,
		Exclude:        excludePatterns,
		BaseBranch:     cleanupBranchBaseBranch,
	}

	if !quiet {
		fmt.Println("Analyzing branches...")
	}

	report, err := svc.Analyze(ctx, repo, analyzeOpts)
	if err != nil {
		return fmt.Errorf("failed to analyze branches: %w", err)
	}

	// Display report
	if !quiet {
		printCleanupBranchReport(report, cleanupBranchDryRun)
	}

	// If no branches to clean up, exit
	if report.IsEmpty() {
		if !quiet {
			fmt.Println("\n✓ No branches to clean up")
		}
		return nil
	}

	// Execute cleanup if not dry-run
	if !cleanupBranchDryRun {
		// Force deletes unmerged branches too; Confirm is only consulted when
		// Force is false, so it is intentionally omitted here (setting it was a
		// no-op that read as "--force means the user confirmed").
		executeOpts := branch.ExecuteOptions{
			DryRun:  false,
			Force:   true,
			Remote:  cleanupBranchRemote,
			Exclude: excludePatterns,
		}

		result, err := svc.Execute(ctx, repo, report, executeOpts)
		if err != nil {
			return fmt.Errorf("failed to execute cleanup: %w", err)
		}

		// Report what was deleted, not what was a candidate. The old count came
		// from the report, so it was announced whether or not a single deletion
		// went through — and Execute discarded the failures on its way out.
		if !quiet {
			fmt.Printf("\n✓ Deleted %d branch(es)\n", len(result.Deleted))
		}

		if len(result.Failed) > 0 {
			fmt.Fprintf(os.Stderr, "\n✗ Failed to delete %d branch(es):\n", len(result.Failed))
			for _, f := range result.Failed {
				fmt.Fprintf(os.Stderr, "  %-30s %v\n", f.Branch, f.Err)
			}

			// Exit 2, the code this CLI reserves for "some of the work failed",
			// so a script can tell a partial cleanup from a tool error (1).
			return cliutil.NewExitError(cliutil.ExitPartialFailed,
				fmt.Errorf("%d of %d branches failed to delete",
					len(result.Failed), len(result.Deleted)+len(result.Failed)))
		}
	} else {
		if !quiet {
			fmt.Println("\nDry-run mode: use --force to actually delete branches")
		}
	}

	return nil
}

// runBulkCleanupBranch performs cleanup across multiple repositories
func runBulkCleanupBranch(ctx context.Context, directory string, excludePatterns []string) error {
	client := repository.NewClient()

	opts := repository.BulkCleanupOptions{
		Directory:         directory,
		Parallel:          cleanupBranchBulkFlags.Parallel,
		MaxDepth:          cleanupBranchBulkFlags.Depth,
		DryRun:            cleanupBranchDryRun,
		IncludeMerged:     cleanupBranchMerged,
		IncludeStale:      cleanupBranchStale,
		IncludeGone:       cleanupBranchGone,
		StaleThreshold:    time.Duration(cleanupBranchStaleDays) * 24 * time.Hour,
		BaseBranch:        cleanupBranchBaseBranch,
		DeleteRemote:      cleanupBranchRemote,
		ProtectPatterns:   excludePatterns,
		IncludeSubmodules: cleanupBranchBulkFlags.IncludeSubmodules,
		IncludePattern:    cleanupBranchBulkFlags.Include,
		ExcludePattern:    cleanupBranchBulkFlags.Exclude,
		Logger:            repository.NewNoopLogger(),
	}

	// Destructive execute path: preview what would be deleted, then require
	// confirmation (bulk × destructive) before actually deleting.
	if !cleanupBranchDryRun {
		proceed, err := confirmBulkCleanupBranch(ctx, client, opts)
		if err != nil {
			return err
		}
		if !proceed {
			if !quiet {
				fmt.Println("Aborted. No branches were deleted.")
			}
			return nil
		}
	}

	if !quiet {
		modeStr := "[DRY-RUN]"
		if !cleanupBranchDryRun {
			modeStr = "[EXECUTE]"
		}
		fmt.Printf("%s Scanning for repositories in %s...\n", modeStr, directory)
	}

	result, err := client.BulkCleanup(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk cleanup failed: %w", err)
	}

	// Print results
	printBulkCleanupBranchResult(result, cleanupBranchDryRun)

	return nil
}

// confirmBulkCleanupBranch runs a dry-run preview, prints the branches that
// would be deleted, and asks the user to confirm before the real deletion.
// It returns (proceed, err): proceed is false when nothing would be deleted or
// the user declines; a non-interactive run without --yes returns an error.
func confirmBulkCleanupBranch(ctx context.Context, client repository.Client, opts repository.BulkCleanupOptions) (bool, error) {
	previewOpts := opts
	previewOpts.DryRun = true

	preview, err := client.BulkCleanup(ctx, previewOpts)
	if err != nil {
		return false, fmt.Errorf("bulk cleanup preview failed: %w", err)
	}

	branchCount := 0
	repoCount := 0
	for _, repo := range preview.Repositories {
		if repo.Status == repository.StatusWouldCleanup && len(repo.DeletedBranches) > 0 {
			branchCount += len(repo.DeletedBranches)
			repoCount++
		}
	}

	if branchCount == 0 {
		if !quiet {
			fmt.Println("\n✓ No branches to clean up")
		}
		return false, nil
	}

	if !quiet {
		fmt.Printf("\nAbout to delete %d branch(es) across %d repositor(ies):\n", branchCount, repoCount)
		for _, repo := range preview.Repositories {
			if repo.Status == repository.StatusWouldCleanup && len(repo.DeletedBranches) > 0 {
				fmt.Printf("  %s: %s\n", repo.RelativePath, strings.Join(repo.DeletedBranches, ", "))
			}
		}
	}

	return confirmDestructiveBulk(cleanupBranchYes)
}

// printBulkCleanupBranchResult displays bulk cleanup results
func printBulkCleanupBranchResult(result *repository.BulkCleanupResult, dryRun bool) {
	modeStr := "[DRY-RUN]"
	if !dryRun {
		modeStr = "[EXECUTE]"
	}

	fmt.Printf("\n%s Bulk Branch Cleanup Report\n", modeStr)
	fmt.Println(strings.Repeat("─", 60))

	// Group by status
	cleanedUp := 0
	wouldCleanup := 0
	nothingToDo := 0
	errors := 0

	for _, repo := range result.Repositories {
		switch repo.Status {
		case repository.StatusCleanedUp:
			cleanedUp++
			if verbose {
				fmt.Printf("✓ %s: %s\n", repo.RelativePath, repo.Message)
			}
		case repository.StatusWouldCleanup:
			wouldCleanup++
			if !quiet {
				branchList := ""
				if len(repo.DeletedBranches) > 0 {
					branchList = fmt.Sprintf(" [%s]", strings.Join(repo.DeletedBranches, ", "))
				}
				fmt.Printf("→ %s: %s%s\n", repo.RelativePath, repo.Message, branchList)
			}
		case repository.StatusNothingToDo:
			nothingToDo++
		case repository.StatusError:
			errors++
			if !quiet {
				fmt.Printf("✗ %s: %s\n", repo.RelativePath, repo.Message)
			}
		}
	}

	fmt.Println(strings.Repeat("─", 60))
	fmt.Printf("Repositories: %d scanned, %d processed\n", result.TotalScanned, result.TotalProcessed)
	fmt.Printf("Branches: %d analyzed\n", result.TotalBranchesAnalyzed)

	if dryRun {
		fmt.Printf("Would clean up: %d repo(s), Nothing to do: %d, Errors: %d\n", wouldCleanup, nothingToDo, errors)
		fmt.Printf("\nDry-run mode: use --force to actually delete branches\n")
	} else {
		fmt.Printf("Cleaned up: %d repo(s), Deleted: %d branch(es), Errors: %d\n", cleanedUp, result.TotalBranchesDeleted, errors)
	}

	fmt.Printf("Duration: %s\n", result.Duration.Round(time.Millisecond))
}

// printCleanupBranchReport displays the cleanup analysis report.
func printCleanupBranchReport(report *branch.CleanupReport, dryRun bool) {
	modeStr := "[DRY-RUN]"
	if !dryRun {
		modeStr = "[EXECUTE]"
	}

	fmt.Printf("\n%s Branch Cleanup Report\n", modeStr)
	fmt.Println(strings.Repeat("─", 50))

	// Merged branches
	if len(report.Merged) > 0 {
		fmt.Printf("\n📦 Merged branches (%d):\n", len(report.Merged))
		for _, b := range report.Merged {
			fmt.Printf("   • %s\n", b.Name)
		}
	}

	// Stale branches
	if len(report.Stale) > 0 {
		fmt.Printf("\n⏰ Stale branches (%d):\n", len(report.Stale))
		for _, b := range report.Stale {
			ageStr := ""
			if b.UpdatedAt != nil {
				age := time.Since(*b.UpdatedAt)
				ageStr = fmt.Sprintf(" (%.0f days old)", age.Hours()/24)
			}
			fmt.Printf("   • %s%s\n", b.Name, ageStr)
		}
	}

	// Orphaned branches
	if len(report.Orphaned) > 0 {
		fmt.Printf("\n👻 Gone branches (%d):\n", len(report.Orphaned))
		for _, b := range report.Orphaned {
			fmt.Printf("   • %s\n", b.Name)
		}
	}

	// Protected branches (info only)
	if len(report.Protected) > 0 {
		fmt.Printf("\n🔒 Protected branches (%d) - will not be deleted:\n", len(report.Protected))
		for _, b := range report.Protected {
			fmt.Printf("   • %s\n", b.Name)
		}
	}

	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("Total: %d branch(es) to clean up (analyzed %d)\n", report.CountBranches(), report.Total)
}
