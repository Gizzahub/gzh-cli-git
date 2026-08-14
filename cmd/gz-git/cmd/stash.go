package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

var (
	stashMessage        string
	stashIncludeUntrack bool
	stashSaveBulkFlags  BulkCommandFlags
	stashListBulkFlags  BulkCommandFlags
	stashPopBulkFlags   BulkCommandFlags
	stashApplyBulkFlags BulkCommandFlags
)

var stashCmd = &cobra.Command{
	Use:   "stash",
	Short: "Stash management commands",
	Long: cliutil.QuickStartHelp(`  # Stash changes in current repo
  gz-git stash save -m "WIP: feature"

  # Stash all dirty repos (Bulk)
  gz-git stash save . -m "WIP: bulk stash"

  # List stashes
  gz-git stash list

  # Pop latest stash
  gz-git stash pop

  # Apply latest stash without dropping it
  gz-git stash apply`) + cliutil.ExitCodesBulkHelp(),
	Example: ``,
	Args:    cobra.NoArgs,
}

var stashSaveCmd = &cobra.Command{
	Use:   "save [directory]",
	Short: "Save changes to stash",
	Long: cliutil.QuickStartHelp(`  # Stash changes with message
  gz-git stash save -m "WIP: refactoring"

  # Include untracked files
  gz-git stash save -u -m "WIP: new files"

  # BULK: Stash all dirty repos
  gz-git stash save . -m "WIP: before branch switch"`),
	Example: ``,
	RunE:    runStashSave,
}

var stashListCmd = &cobra.Command{
	Use:   "list [directory]",
	Short: "List stash entries",
	Long: cliutil.QuickStartHelp(`  # List stashes in current repo
  gz-git stash list

  # BULK: List stashes across all repos
  gz-git stash list .`),
	Example: ``,
	RunE:    runStashList,
}

var stashPopCmd = &cobra.Command{
	Use:   "pop [directory]",
	Short: "Pop latest stash",
	Long: cliutil.QuickStartHelp(`  # Pop latest stash
  gz-git stash pop

  # BULK: Pop stashes in all repos
  gz-git stash pop .

  # BULK: Dry-run to preview
  gz-git stash pop . -n`),
	Example: ``,
	RunE:    runStashPop,
}

var stashApplyCmd = &cobra.Command{
	Use:   "apply [directory]",
	Short: "Apply latest stash without removing it",
	Long: cliutil.QuickStartHelp(`  # Apply latest stash
  gz-git stash apply

  # BULK: Apply stashes in all repos
  gz-git stash apply .

  # BULK: Dry-run to preview
  gz-git stash apply . -n`),
	Example: ``,
	RunE:    runStashApply,
}

func init() {
	rootCmd.AddCommand(stashCmd)
	stashCmd.AddCommand(stashSaveCmd)
	stashCmd.AddCommand(stashListCmd)
	stashCmd.AddCommand(stashPopCmd)
	stashCmd.AddCommand(stashApplyCmd)

	stashSaveCmd.Flags().StringVarP(&stashMessage, "message", "m", "", "stash message")
	stashSaveCmd.Flags().BoolVarP(&stashIncludeUntrack, "include-untracked", "u", false, "include untracked files")

	addBulkFlags(stashSaveCmd, &stashSaveBulkFlags)
	addBulkFlags(stashListCmd, &stashListBulkFlags)
	addBulkFlags(stashPopCmd, &stashPopBulkFlags)
	addBulkFlags(stashApplyCmd, &stashApplyBulkFlags)
}

func runStashSave(cmd *cobra.Command, args []string) error {
	directory, err := validateBulkDirectory(args)
	if err != nil {
		return err
	}
	if err := validateBulkDepth(cmd, stashSaveBulkFlags.Depth); err != nil {
		return err
	}
	if err := validateBulkFormat(stashSaveBulkFlags.Format); err != nil {
		return err
	}
	return runBulkStashSave(cmdContext(cmd), directory)
}

func runBulkStashSave(ctx context.Context, directory string) error {
	client := repository.NewClient()

	opts := repository.BulkStashOptions{
		Directory:        directory,
		Parallel:         stashSaveBulkFlags.Parallel,
		MaxDepth:         stashSaveBulkFlags.Depth,
		DryRun:           stashSaveBulkFlags.DryRun,
		Operation:        "save",
		Message:          stashMessage,
		IncludeUntracked: stashIncludeUntrack,
		IncludePattern:   stashSaveBulkFlags.Include,
		ExcludePattern:   stashSaveBulkFlags.Exclude,
		Logger:           repository.NewNoopLogger(),
	}

	if shouldShowProgress(stashSaveBulkFlags.Format, quiet) {
		printScanningMessage(directory, stashSaveBulkFlags.Depth, stashSaveBulkFlags.Parallel, stashSaveBulkFlags.DryRun)
	}

	result, err := client.BulkStash(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk stash save failed: %w", err)
	}

	printBulkStashResult(result, "save", stashSaveBulkFlags.DryRun, stashSaveBulkFlags.Format)
	return errPartialFailure(result.Summary[repository.StatusError], result.TotalProcessed)
}

func runStashList(cmd *cobra.Command, args []string) error {
	directory, err := validateBulkDirectory(args)
	if err != nil {
		return err
	}
	if err := validateBulkDepth(cmd, stashListBulkFlags.Depth); err != nil {
		return err
	}
	if err := validateBulkFormat(stashListBulkFlags.Format); err != nil {
		return err
	}
	return runBulkStashList(cmdContext(cmd), directory)
}

func runBulkStashList(ctx context.Context, directory string) error {
	client := repository.NewClient()

	opts := repository.BulkStashOptions{
		Directory:      directory,
		Parallel:       stashListBulkFlags.Parallel,
		MaxDepth:       stashListBulkFlags.Depth,
		Operation:      "list",
		IncludePattern: stashListBulkFlags.Include,
		ExcludePattern: stashListBulkFlags.Exclude,
		Logger:         repository.NewNoopLogger(),
	}

	if shouldShowProgress(stashListBulkFlags.Format, quiet) {
		printScanningMessage(directory, stashListBulkFlags.Depth, stashListBulkFlags.Parallel, false)
	}

	result, err := client.BulkStash(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk stash list failed: %w", err)
	}

	printBulkStashResult(result, "list", false, stashListBulkFlags.Format)
	return errPartialFailure(result.Summary[repository.StatusError], result.TotalProcessed)
}

func runStashPop(cmd *cobra.Command, args []string) error {
	directory, err := validateBulkDirectory(args)
	if err != nil {
		return err
	}
	if err := validateBulkDepth(cmd, stashPopBulkFlags.Depth); err != nil {
		return err
	}
	if err := validateBulkFormat(stashPopBulkFlags.Format); err != nil {
		return err
	}
	return runBulkStashPop(cmdContext(cmd), directory)
}

func runBulkStashPop(ctx context.Context, directory string) error {
	client := repository.NewClient()

	opts := repository.BulkStashOptions{
		Directory:      directory,
		Parallel:       stashPopBulkFlags.Parallel,
		MaxDepth:       stashPopBulkFlags.Depth,
		DryRun:         stashPopBulkFlags.DryRun,
		Operation:      "pop",
		IncludePattern: stashPopBulkFlags.Include,
		ExcludePattern: stashPopBulkFlags.Exclude,
		Logger:         repository.NewNoopLogger(),
	}

	if shouldShowProgress(stashPopBulkFlags.Format, quiet) {
		printScanningMessage(directory, stashPopBulkFlags.Depth, stashPopBulkFlags.Parallel, stashPopBulkFlags.DryRun)
	}

	result, err := client.BulkStash(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk stash pop failed: %w", err)
	}

	printBulkStashResult(result, "pop", stashPopBulkFlags.DryRun, stashPopBulkFlags.Format)
	return errPartialFailure(result.Summary[repository.StatusError], result.TotalProcessed)
}

func runStashApply(cmd *cobra.Command, args []string) error {
	directory, err := validateBulkDirectory(args)
	if err != nil {
		return err
	}
	if err := validateBulkDepth(cmd, stashApplyBulkFlags.Depth); err != nil {
		return err
	}
	if err := validateBulkFormat(stashApplyBulkFlags.Format); err != nil {
		return err
	}
	return runBulkStashApply(cmdContext(cmd), directory)
}

func runBulkStashApply(ctx context.Context, directory string) error {
	client := repository.NewClient()

	opts := repository.BulkStashOptions{
		Directory:      directory,
		Parallel:       stashApplyBulkFlags.Parallel,
		MaxDepth:       stashApplyBulkFlags.Depth,
		DryRun:         stashApplyBulkFlags.DryRun,
		Operation:      "apply",
		IncludePattern: stashApplyBulkFlags.Include,
		ExcludePattern: stashApplyBulkFlags.Exclude,
		Logger:         repository.NewNoopLogger(),
	}

	if shouldShowProgress(stashApplyBulkFlags.Format, quiet) {
		printScanningMessage(directory, stashApplyBulkFlags.Depth, stashApplyBulkFlags.Parallel, stashApplyBulkFlags.DryRun)
	}

	result, err := client.BulkStash(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk stash apply failed: %w", err)
	}

	printBulkStashResult(result, "apply", stashApplyBulkFlags.DryRun, stashApplyBulkFlags.Format)
	return errPartialFailure(result.Summary[repository.StatusError], result.TotalProcessed)
}

func printBulkStashResult(result *repository.BulkStashResult, operation string, dryRun bool, format string) {
	if format == "json" || format == "llm" {
		displayStashResultsStructured(result, operation, format)
		return
	}

	modeStr := ""
	if dryRun {
		modeStr = "[DRY-RUN] "
	}

	fmt.Printf("\n%sBulk Stash %s Report\n", modeStr, titleCase(operation))
	fmt.Println(strings.Repeat("─", 50))

	for _, repo := range result.Repositories {
		switch repo.Status {
		case repository.StatusStashed, repository.StatusWouldStash:
			icon := "✓"
			if dryRun {
				icon = "→"
			}
			fmt.Printf("%s %s: %s\n", icon, repo.RelativePath, repo.Message)

		case repository.StatusPopped, repository.StatusWouldPop:
			icon := "✓"
			if dryRun {
				icon = "→"
			}
			fmt.Printf("%s %s: %s\n", icon, repo.RelativePath, repo.Message)

		case repository.StatusHasStash:
			fmt.Printf("📦 %s: %s\n", repo.RelativePath, repo.Message)

		case repository.StatusError:
			fmt.Printf("✗ %s: %s\n", repo.RelativePath, repo.Message)
		}
	}

	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("Repositories: %d scanned, %d processed\n", result.TotalScanned, result.TotalProcessed)
	fmt.Printf("Total stashes: %d\n", result.TotalStashCount)
	fmt.Printf("Duration: %s\n", result.Duration.Round(time.Millisecond))
}

type StashJSONOutput struct {
	Operation      string                      `json:"operation"`
	TotalScanned   int                         `json:"total_scanned"`
	TotalProcessed int                         `json:"total_processed"`
	TotalStashes   int                         `json:"total_stashes"`
	DurationMs     int64                       `json:"duration_ms"`
	Repositories   []StashRepositoryJSONOutput `json:"repositories"`
}

type StashRepositoryJSONOutput struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func displayStashResultsStructured(result *repository.BulkStashResult, operation, format string) {
	output := StashJSONOutput{
		Operation:      operation,
		TotalScanned:   result.TotalScanned,
		TotalProcessed: result.TotalProcessed,
		TotalStashes:   result.TotalStashCount,
		DurationMs:     result.Duration.Milliseconds(),
		Repositories:   make([]StashRepositoryJSONOutput, 0, len(result.Repositories)),
	}

	for _, repo := range result.Repositories {
		output.Repositories = append(output.Repositories, StashRepositoryJSONOutput{
			Path:    repo.RelativePath,
			Status:  repo.Status,
			Message: repo.Message,
		})
	}

	writeBulkOutput(format, output)
}
