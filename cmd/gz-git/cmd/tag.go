package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/tag"
)

var (
	tagMessage         string
	tagForce           bool
	tagPushAll         bool
	tagBump            string
	tagCreateBulkFlags BulkCommandFlags
	tagAutoBulkFlags   BulkCommandFlags
	tagListBulkFlags   BulkCommandFlags
	tagPushBulkFlags   BulkCommandFlags
	tagStatusBulkFlags BulkCommandFlags
)

// tagCmd represents the tag command group.
var tagCmd = &cobra.Command{
	Use:   "tag",
	Short: "Tag management commands",
	Long: cliutil.QuickStartHelp(`  # Create a tag
  gz-git tag create v1.0.0 -m "Release 1.0.0"

  # Auto-bump version (patch: v1.0.0 -> v1.0.1)
  gz-git tag auto --bump=patch

  # List tags
  gz-git tag list

  # Push tags
  gz-git tag push

  # BULK: Create same tag across all repos
  gz-git tag create v1.0.0 . -m "Release"

  # BULK: Check tag status
  gz-git tag status .`) + cliutil.ExitCodesBulkHelp(),
	Example: ``,
	Args:    cobra.NoArgs,
}

// tagCreateCmd creates a tag.
var tagCreateCmd = &cobra.Command{
	Use:   "create <name> [directory]",
	Short: "Create a tag",
	Long: cliutil.QuickStartHelp(`  # Create annotated tag
  gz-git tag create v1.0.0 -m "Release 1.0.0"

  # Force overwrite existing tag
  gz-git tag create v1.0.0 -f

  # BULK: Create tag in all repos
  gz-git tag create v1.0.0 . -m "Release"`),
	Example: ``,
	Args:    cobra.MinimumNArgs(1),
	RunE:    runTagCreate,
}

// tagAutoCmd auto-generates next version.
var tagAutoCmd = &cobra.Command{
	Use:   "auto [directory]",
	Short: "Auto-generate next version tag",
	Long: cliutil.QuickStartHelp(`  # Bump patch version (v1.0.0 -> v1.0.1)
  gz-git tag auto --bump=patch

  # Bump minor version (v1.0.0 -> v1.1.0)
  gz-git tag auto --bump=minor

  # Bump major version (v1.0.0 -> v2.0.0)
  gz-git tag auto --bump=major`),
	Example: ``,
	RunE:    runTagAuto,
}

// tagListCmd lists tags.
var tagListCmd = &cobra.Command{
	Use:   "list [directory]",
	Short: "List tags",
	Long: cliutil.QuickStartHelp(`  # List tags
  gz-git tag list

  # BULK: List tags across all repos
  gz-git tag list .`),
	Example: ``,
	RunE:    runTagList,
}

// tagPushCmd pushes tags.
var tagPushCmd = &cobra.Command{
	Use:   "push [name] [directory]",
	Short: "Push tags to remote",
	Long: cliutil.QuickStartHelp(`  # Push a single tag (current repo)
  gz-git tag push v1.2.3

  # Push all tags (current repo)
  gz-git tag push --all

  # BULK: Push all tags from every repo under a directory
  gz-git tag push --all .`),
	Example: ``,
	RunE:    runTagPush,
}

// tagStatusCmd shows tag status.
var tagStatusCmd = &cobra.Command{
	Use:   "status [directory]",
	Short: "Show tag status",
	Long:  `Show tag status across repositories.`,
	RunE:  runTagStatus,
}

func init() {
	rootCmd.AddCommand(tagCmd)
	tagCmd.AddCommand(tagCreateCmd)
	tagCmd.AddCommand(tagAutoCmd)
	tagCmd.AddCommand(tagListCmd)
	tagCmd.AddCommand(tagPushCmd)
	tagCmd.AddCommand(tagStatusCmd)

	// Create flags
	tagCreateCmd.Flags().StringVarP(&tagMessage, "message", "m", "", "tag message (creates annotated tag)")
	tagCreateCmd.Flags().BoolVarP(&tagForce, "force", "f", false, "force overwrite existing tag")

	// Auto flags
	tagAutoCmd.Flags().StringVar(&tagBump, "bump", "patch", "version bump type: major, minor, patch")
	tagAutoCmd.Flags().StringVarP(&tagMessage, "message", "m", "", "tag message")

	// Push flags
	tagPushCmd.Flags().BoolVar(&tagPushAll, "all", false, "push all tags (required for bulk/directory mode)")

	// Bulk flags for subcommands (using shared addBulkFlags)
	addBulkFlags(tagCreateCmd, &tagCreateBulkFlags)
	addBulkFlags(tagAutoCmd, &tagAutoBulkFlags)
	addBulkFlags(tagListCmd, &tagListBulkFlags)
	addBulkFlags(tagPushCmd, &tagPushBulkFlags)
	addBulkFlags(tagStatusCmd, &tagStatusBulkFlags)
}

func runTagCreate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	tagName := args[0]

	// Bulk mode
	if len(args) > 1 {
		if err := resolveBulkDepth(cmd, args[1], &tagCreateBulkFlags.Depth); err != nil {
			return err
		}
		return runBulkTagCreate(ctx, args[1], tagName)
	}

	// Single repo mode
	return runSingleTagCreate(ctx, tagName)
}

func runSingleTagCreate(ctx context.Context, tagName string) error {
	repo, err := openCurrentRepo(ctx)
	if err != nil {
		return err
	}

	mgr := tag.NewManager()
	opts := tag.CreateOptions{
		Name:    tagName,
		Message: tagMessage,
		Force:   tagForce,
	}

	if err := mgr.Create(ctx, repo, opts); err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}

	if !quiet {
		fmt.Printf("✓ Created tag %s\n", tagName)
	}

	return nil
}

func runBulkTagCreate(ctx context.Context, directory, tagName string) error {
	client := repository.NewClient()

	opts := repository.BulkTagOptions{
		Directory:      directory,
		Parallel:       tagCreateBulkFlags.Parallel,
		MaxDepth:       tagCreateBulkFlags.Depth,
		DryRun:         tagCreateBulkFlags.DryRun,
		Operation:      "create",
		TagName:        tagName,
		Message:        tagMessage,
		Force:          tagForce,
		IncludePattern: tagCreateBulkFlags.Include,
		ExcludePattern: resolveScanExclude(directory, tagCreateBulkFlags.Exclude),
		Logger:         repository.NewNoopLogger(),
	}

	if shouldShowProgress(tagCreateBulkFlags.Format, quiet) {
		printScanningMessage(directory, tagCreateBulkFlags.Depth, tagCreateBulkFlags.Parallel, tagCreateBulkFlags.DryRun)
	}

	result, err := client.BulkTag(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk tag create failed: %w", err)
	}

	printBulkTagResult(result, "create", tagCreateBulkFlags.DryRun, tagCreateBulkFlags.Format)
	return errPartialFailure(result.Summary[repository.StatusError], result.TotalProcessed)
}

func runTagAuto(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	repo, err := openCurrentRepo(ctx)
	if err != nil {
		return err
	}

	mgr := tag.NewManager()

	// Get next version
	nextVersion, err := mgr.NextVersion(ctx, repo, tagBump)
	if err != nil {
		return fmt.Errorf("failed to determine next version: %w", err)
	}

	if tagAutoBulkFlags.DryRun {
		fmt.Printf("Would create tag: %s\n", nextVersion)
		return nil
	}

	// Create tag
	opts := tag.CreateOptions{
		Name:    nextVersion,
		Message: tagMessage,
	}
	if opts.Message == "" {
		opts.Message = fmt.Sprintf("Release %s", nextVersion)
	}

	if err := mgr.Create(ctx, repo, opts); err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}

	if !quiet {
		fmt.Printf("✓ Created tag %s\n", nextVersion)
	}

	return nil
}

func runTagList(cmd *cobra.Command, args []string) error {
	directory, err := validateBulkDirectory(args)
	if err != nil {
		return err
	}
	if err := resolveBulkDepth(cmd, directory, &tagListBulkFlags.Depth); err != nil {
		return err
	}
	if err := validateBulkFormat(tagListBulkFlags.Format); err != nil {
		return err
	}
	return runBulkTagList(cmdContext(cmd), directory)
}

func runBulkTagList(ctx context.Context, directory string) error {
	client := repository.NewClient()

	opts := repository.BulkTagOptions{
		Directory:      directory,
		Parallel:       tagListBulkFlags.Parallel,
		MaxDepth:       tagListBulkFlags.Depth,
		Operation:      "list",
		IncludePattern: tagListBulkFlags.Include,
		ExcludePattern: resolveScanExclude(directory, tagListBulkFlags.Exclude),
		Logger:         repository.NewNoopLogger(),
	}

	if shouldShowProgress(tagListBulkFlags.Format, quiet) {
		printScanningMessage(directory, tagListBulkFlags.Depth, tagListBulkFlags.Parallel, false)
	}

	result, err := client.BulkTag(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk tag list failed: %w", err)
	}

	printBulkTagResult(result, "list", false, tagListBulkFlags.Format)
	return errPartialFailure(result.Summary[repository.StatusError], result.TotalProcessed)
}

func runTagPush(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	if tagPushAll {
		// All-tags mode. An optional directory arg selects bulk mode.
		if len(args) > 0 {
			if err := resolveBulkDepth(cmd, args[0], &tagPushBulkFlags.Depth); err != nil {
				return err
			}
			return runBulkTagPush(ctx, args[0])
		}
		return runSingleTagPush(ctx, "")
	}

	// Without --all, a tag name is required (single-repo, single-tag push).
	if len(args) == 0 {
		return fmt.Errorf("either --all or a tag name is required (e.g. 'gz-git tag push v1.2.3' or 'gz-git tag push --all')")
	}
	return runSingleTagPush(ctx, args[0])
}

func runSingleTagPush(ctx context.Context, name string) error {
	repo, err := openCurrentRepo(ctx)
	if err != nil {
		return err
	}

	mgr := tag.NewManager()
	opts := tag.PushOptions{
		All:  tagPushAll,
		Name: name,
	}

	if err := mgr.Push(ctx, repo, opts); err != nil {
		return fmt.Errorf("failed to push tags: %w", err)
	}

	if !quiet {
		if name != "" {
			fmt.Printf("✓ Pushed tag %s\n", name)
		} else {
			fmt.Println("✓ Tags pushed")
		}
	}

	return nil
}

func runBulkTagPush(ctx context.Context, directory string) error {
	client := repository.NewClient()

	opts := repository.BulkTagOptions{
		Directory:      directory,
		Parallel:       tagPushBulkFlags.Parallel,
		MaxDepth:       tagPushBulkFlags.Depth,
		DryRun:         tagPushBulkFlags.DryRun,
		Operation:      "push",
		PushAll:        tagPushAll,
		IncludePattern: tagPushBulkFlags.Include,
		ExcludePattern: resolveScanExclude(directory, tagPushBulkFlags.Exclude),
		Logger:         repository.NewNoopLogger(),
	}

	if shouldShowProgress(tagPushBulkFlags.Format, quiet) {
		printScanningMessage(directory, tagPushBulkFlags.Depth, tagPushBulkFlags.Parallel, tagPushBulkFlags.DryRun)
	}

	result, err := client.BulkTag(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk tag push failed: %w", err)
	}

	printBulkTagResult(result, "push", tagPushBulkFlags.DryRun, tagPushBulkFlags.Format)
	return errPartialFailure(result.Summary[repository.StatusError], result.TotalProcessed)
}

func runTagStatus(cmd *cobra.Command, args []string) error {
	directory, err := validateBulkDirectory(args)
	if err != nil {
		return err
	}
	if err := resolveBulkDepth(cmd, directory, &tagStatusBulkFlags.Depth); err != nil {
		return err
	}
	if err := validateBulkFormat(tagStatusBulkFlags.Format); err != nil {
		return err
	}

	client := repository.NewClient()

	opts := repository.BulkTagOptions{
		Directory:      directory,
		Parallel:       tagStatusBulkFlags.Parallel,
		MaxDepth:       tagStatusBulkFlags.Depth,
		Operation:      "status",
		IncludePattern: tagStatusBulkFlags.Include,
		ExcludePattern: resolveScanExclude(directory, tagStatusBulkFlags.Exclude),
		Logger:         repository.NewNoopLogger(),
	}

	if shouldShowProgress(tagStatusBulkFlags.Format, quiet) {
		printScanningMessage(directory, tagStatusBulkFlags.Depth, tagStatusBulkFlags.Parallel, false)
	}

	result, err := client.BulkTag(cmdContext(cmd), opts)
	if err != nil {
		return fmt.Errorf("tag status failed: %w", err)
	}

	printBulkTagResult(result, "status", false, tagStatusBulkFlags.Format)
	return errPartialFailure(result.Summary[repository.StatusError], result.TotalProcessed)
}

func printBulkTagResult(result *repository.BulkTagResult, operation string, dryRun bool, format string) {
	// JSON or LLM output mode
	if format == "json" || format == "llm" {
		displayTagResultsStructured(result, operation, format)
		return
	}

	modeStr := ""
	if dryRun {
		modeStr = "[DRY-RUN] "
	}

	fmt.Printf("\n%sBulk Tag %s Report\n", modeStr, titleCase(operation))
	fmt.Println(strings.Repeat("─", 50))

	// Show repos with tags
	for _, repo := range result.Repositories {
		switch repo.Status {
		case repository.StatusTagCreated, repository.StatusWouldCreateTag:
			icon := "✓"
			if dryRun {
				icon = "→"
			}
			fmt.Printf("%s %s: %s\n", icon, repo.RelativePath, repo.Message)

		case repository.StatusTagPushed, repository.StatusWouldPushTag:
			icon := "✓"
			if dryRun {
				icon = "→"
			}
			fmt.Printf("%s %s: %s\n", icon, repo.RelativePath, repo.Message)

		case repository.StatusHasTags:
			fmt.Printf("🏷  %s: %s\n", repo.RelativePath, repo.Message)

		case repository.StatusTagExists:
			fmt.Printf("= %s: %s\n", repo.RelativePath, repo.Message)

		case repository.StatusNoTags:
			if verbose {
				fmt.Printf("  %s: %s\n", repo.RelativePath, repo.Message)
			}

		case repository.StatusError:
			fmt.Printf("✗ %s: %s\n", repo.RelativePath, repo.Message)
		}
	}

	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("Repositories: %d scanned, %d processed\n", result.TotalScanned, result.TotalProcessed)
	fmt.Printf("Total tags: %d\n", result.TotalTagCount)
	fmt.Printf("Duration: %s\n", result.Duration.Round(time.Millisecond))
}

// TagJSONOutput represents the JSON output structure for tag command.
type TagJSONOutput struct {
	Operation      string                    `json:"operation"`
	TotalScanned   int                       `json:"total_scanned"`
	TotalProcessed int                       `json:"total_processed"`
	TotalTags      int                       `json:"total_tags"`
	DurationMs     int64                     `json:"duration_ms"`
	Repositories   []TagRepositoryJSONOutput `json:"repositories"`
}

// TagRepositoryJSONOutput represents a single repository in JSON output.
type TagRepositoryJSONOutput struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func displayTagResultsStructured(result *repository.BulkTagResult, operation, format string) {
	output := TagJSONOutput{
		Operation:      operation,
		TotalScanned:   result.TotalScanned,
		TotalProcessed: result.TotalProcessed,
		TotalTags:      result.TotalTagCount,
		DurationMs:     result.Duration.Milliseconds(),
		Repositories:   make([]TagRepositoryJSONOutput, 0, len(result.Repositories)),
	}

	for _, repo := range result.Repositories {
		output.Repositories = append(output.Repositories, TagRepositoryJSONOutput{
			Path:    repo.RelativePath,
			Status:  repo.Status,
			Message: repo.Message,
		})
	}

	writeBulkOutput(format, output)
}
