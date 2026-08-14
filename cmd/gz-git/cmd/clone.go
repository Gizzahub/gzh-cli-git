package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

var (
	cloneFlags        BulkCommandFlags
	cloneBranch       string
	cloneDepth        int
	cloneStrategy     string // --update-strategy flag (skip, pull, reset, rebase, fetch)
	cloneStructure    string
	cloneFile         string
	cloneSingleBranch bool
	cloneSubmodules   bool
	cloneURLs         []string // --url flag (repeatable)
	cloneConfig       string   // --config flag (YAML file path)
	cloneConfigStdin  bool     // --config-stdin flag (read YAML from stdin)
	cloneGroup        []string // --group flag (select specific groups)
)

// cloneCmd represents the clone command.
var cloneCmd = &cobra.Command{
	Use:   "clone [directory]",
	Short: "Clone multiple repositories in parallel",
	Long: `Clone one or more repositories from remote URLs in parallel.

` + cliutil.QuickStartHelp(`  # 1. Clone multiple URLs to current directory
  gz-git clone --url https://github.com/user/repo1.git --url https://github.com/user/repo2.git

  # 2. Clone from a file containing URLs
  gz-git clone --file repos.txt

  # 3. Clone with user directory structure (user/repo)
  gz-git clone --structure user --url ...

See 'gcl' alias for single repository cloning.`) + cliutil.ExitCodesBulkHelp(),
	Example: "",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runClone,
}

func init() {
	rootCmd.AddCommand(cloneCmd)

	// Bulk operation flags — clone only consumes parallel/format/dry-run.
	// Suppress non-functional bulk flags; --submodules/--single-branch below
	// cover clone's own recurse/branch scoping.
	addBulkFlagsWithOpts(cloneCmd, &cloneFlags, BulkFlagOptions{
		SkipScanDepth: true,
		SkipRecursive: true,
		SkipInclude:   true,
		SkipExclude:   true,
		SkipWatch:     true,
		SkipFetch:     true,
	})

	// Clone-specific flags
	cloneCmd.Flags().StringArrayVar(&cloneURLs, "url", nil, "repository URL to clone (can be repeated)")
	cloneCmd.Flags().StringVarP(&cloneBranch, "branch", "b", "", "checkout specific branch")
	cloneCmd.Flags().IntVar(&cloneDepth, "depth", 0, "create a shallow clone with truncated history")
	cloneCmd.Flags().StringVarP(&cloneStrategy, "update-strategy", "s", "", "existing repo handling: skip (default), pull, reset, rebase, fetch")
	cloneCmd.Flags().StringVar(&cloneStructure, "structure", "flat", "directory structure: flat or user")
	cloneCmd.Flags().StringVar(&cloneFile, "file", "", "file containing repository URLs (one per line)")
	cloneCmd.Flags().BoolVar(&cloneSingleBranch, "single-branch", false, "clone only one branch")
	cloneCmd.Flags().BoolVar(&cloneSubmodules, "submodules", false, "initialize submodules in the clone")
	cloneCmd.Flags().StringVarP(&cloneConfig, "config", "c", "", "YAML config file for clone specifications")
	cloneCmd.Flags().BoolVar(&cloneConfigStdin, "config-stdin", false, "read YAML config from stdin")
	cloneCmd.Flags().StringArrayVarP(&cloneGroup, "group", "g", nil, "clone only specified groups (can be repeated)")
}

func runClone(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	effective, _ := LoadEffectiveConfig(cmd, nil)
	if effective != nil {
		if !cmd.Flags().Changed("parallel") && effective.Parallel > 0 {
			cloneFlags.Parallel = effective.Parallel
		}
		if verbose {
			PrintConfigSources(cmd, effective)
		}
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		<-sigChan
		if !quiet {
			fmt.Println("\nInterrupted, canceling...")
		}
		cancel()
	}()

	directory := "."
	if len(args) > 0 {
		directory = args[0]
		if directory != "." {
			if _, err := os.Stat(directory); os.IsNotExist(err) {
				return fmt.Errorf("directory does not exist: %s", directory)
			}
		}
	}

	if cloneConfig != "" || cloneConfigStdin {
		return runCloneFromConfig(ctx, directory)
	}

	// Collect URLs from --url flag and --file
	urls, err := collectCloneURLs(cloneURLs, cloneFile)
	if err != nil {
		return err
	}

	if len(urls) == 0 {
		return fmt.Errorf("no repository URLs provided. Use --url, --file, or --config")
	}

	// Validate structure
	structure := repository.DirectoryStructure(cloneStructure)
	if structure != repository.StructureFlat && structure != repository.StructureUser {
		return fmt.Errorf("invalid structure %q: must be 'flat' or 'user'", cloneStructure)
	}

	// Validate format
	if err := validateBulkFormat(cloneFlags.Format); err != nil {
		return err
	}

	// Create client
	client := repository.NewClient()

	// Create logger for verbose mode
	logger := createBulkLogger(verbose)

	// Resolve strategy from CLI flags
	strategy := resolveCloneStrategy(cloneStrategy, "")

	// Build options
	opts := repository.BulkCloneOptions{
		URLs:         urls,
		Directory:    directory,
		Structure:    structure,
		Strategy:     strategy,
		Branch:       cloneBranch,
		Depth:        cloneDepth,
		SingleBranch: cloneSingleBranch,
		Recursive:    cloneSubmodules,
		Parallel:     cloneFlags.Parallel,
		DryRun:       cloneFlags.DryRun,
		Verbose:      verbose,
		Logger:       logger,
		ProgressCallback: func(current, total int, url string) {
			if shouldShowProgress(cloneFlags.Format, quiet) {
				repoName, _ := repository.ExtractRepoNameFromURL(url)
				if repoName == "" {
					repoName = "repository"
				}
				fmt.Printf("[%d/%d] Cloning %s...\n", current, total, repoName)
			}
		},
	}

	// Show scanning message
	if shouldShowProgress(cloneFlags.Format, quiet) {
		suffix := ""
		if cloneFlags.DryRun {
			suffix = " [DRY-RUN]"
		}
		strategyStr := ""
		if strategy != repository.StrategySkip {
			strategyStr = fmt.Sprintf(", strategy: %s", strategy)
		}
		dirStr := ""
		if directory != "." {
			dirStr = fmt.Sprintf(" to %s", directory)
		}
		fmt.Printf("Cloning %d repositories%s (parallel: %d, structure: %s%s)%s\n",
			len(urls), dirStr, cloneFlags.Parallel, cloneStructure, strategyStr, suffix)
	}

	// Execute bulk clone
	result, err := client.BulkClone(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk clone failed: %w", err)
	}

	// Display results
	displayCloneResults(result)

	return errPartialFailure(result.TotalFailed, result.TotalRequested)
}

// collectCloneURLs collects URLs from --url flags and --file.
func collectCloneURLs(urlFlags []string, filePath string) ([]string, error) {
	urls := make([]string, 0)

	// Add URLs from --url flags
	for _, url := range urlFlags {
		url = strings.TrimSpace(url)
		if url != "" && !strings.HasPrefix(url, "#") {
			urls = append(urls, url)
		}
	}

	// Add URLs from file
	if filePath != "" {
		file, err := os.Open(filePath) // #nosec G304 -- --file is an explicit user-selected clone list path.
		if err != nil {
			return nil, fmt.Errorf("cannot open file %s: %w", filePath, err)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			// Skip empty lines and comments
			if line != "" && !strings.HasPrefix(line, "#") {
				urls = append(urls, line)
			}
		}

		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("error reading file %s: %w", filePath, err)
		}
	}

	return urls, nil
}

func displayCloneResults(result *repository.BulkCloneResult) {
	// JSON or LLM output mode
	if cloneFlags.Format == "json" || cloneFlags.Format == "llm" {
		displayCloneResultsStructured(result, cloneFlags.Format)
		return
	}

	fmt.Println()
	fmt.Println("=== Bulk Clone Results ===")
	fmt.Printf("Total requested: %d repositories\n", result.TotalRequested)
	fmt.Printf("Total cloned:    %d repositories\n", result.TotalCloned)
	fmt.Printf("Total updated:   %d repositories\n", result.TotalUpdated)
	fmt.Printf("Total skipped:   %d repositories\n", result.TotalSkipped)
	fmt.Printf("Total failed:    %d repositories\n", result.TotalFailed)
	fmt.Printf("Duration:        %s\n", result.Duration.Round(100_000_000))
	fmt.Println()

	// Display summary
	if len(result.Summary) > 0 {
		fmt.Println("Summary by status:")
		for status, count := range result.Summary {
			icon := getCloneStatusIcon(status)
			fmt.Printf("  %s %-15s %d\n", icon, status+":", count)
		}
		fmt.Println()
	}

	// Display individual results if not compact
	if cloneFlags.Format != "compact" && len(result.Repositories) > 0 {
		fmt.Println("Repository details:")
		for _, repo := range result.Repositories {
			displayCloneRepositoryResult(repo)
		}
	}
}

func displayCloneRepositoryResult(repo repository.RepositoryCloneResult) {
	icon := getCloneStatusIcon(repo.Status)

	// Build output line
	pathPart := repo.RelativePath
	if repo.Branch != "" {
		pathPart += fmt.Sprintf(" (%s)", repo.Branch)
	}

	statusStr := repo.Status
	if repo.Error != nil && verbose {
		statusStr = fmt.Sprintf("%s: %v", repo.Status, repo.Error)
	}

	durationStr := ""
	if repo.Duration > 0 {
		durationStr = fmt.Sprintf(" %s", repo.Duration.Round(10_000_000))
	}

	fmt.Printf("  %s %-50s %-15s%s\n", icon, pathPart, statusStr, durationStr)
}

func getCloneStatusIcon(status string) string {
	switch status {
	case "cloned":
		return "✓"
	case "updated", "pulled", "rebased":
		return "↓"
	case "skipped":
		return "⊘"
	case "would-clone", "would-update":
		return "→"
	case "dirty":
		return "⚠"
	case "error":
		return "✗"
	default:
		return "•"
	}
}

// CloneJSONOutput represents the JSON output structure for clone command.
type CloneJSONOutput struct {
	TotalRequested int                       `json:"total_requested"`
	TotalCloned    int                       `json:"total_cloned"`
	TotalUpdated   int                       `json:"total_updated"`
	TotalSkipped   int                       `json:"total_skipped"`
	TotalFailed    int                       `json:"total_failed"`
	DurationMs     int64                     `json:"duration_ms"`
	Summary        map[string]int            `json:"summary"`
	Repositories   []CloneRepositoryJSONItem `json:"repositories"`
}

// CloneRepositoryJSONItem represents a single repository in JSON output.
type CloneRepositoryJSONItem struct {
	URL        string `json:"url"`
	Path       string `json:"path"`
	Status     string `json:"status"`
	Branch     string `json:"branch,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Error      string `json:"error,omitempty"`
}

func displayCloneResultsStructured(result *repository.BulkCloneResult, format string) {
	output := CloneJSONOutput{
		TotalRequested: result.TotalRequested,
		TotalCloned:    result.TotalCloned,
		TotalUpdated:   result.TotalUpdated,
		TotalSkipped:   result.TotalSkipped,
		TotalFailed:    result.TotalFailed,
		DurationMs:     result.Duration.Milliseconds(),
		Summary:        result.Summary,
		Repositories:   make([]CloneRepositoryJSONItem, 0, len(result.Repositories)),
	}

	for _, repo := range result.Repositories {
		item := CloneRepositoryJSONItem{
			URL:        repo.URL,
			Path:       repo.RelativePath,
			Status:     repo.Status,
			Branch:     repo.Branch,
			DurationMs: repo.Duration.Milliseconds(),
		}
		if repo.Error != nil {
			item.Error = repo.Error.Error()
		}
		output.Repositories = append(output.Repositories, item)
	}

	writeBulkOutput(format, output)
}

// ============================================================================
// YAML Config Support
// ============================================================================
// Clone Config Types
// ============================================================================

// cloneSingleRepository clones a single repository with custom settings.
func cloneSingleRepository(
	ctx context.Context,
	client repository.Client,
	spec CloneRepoSpec,
	repoName string,
	baseOpts repository.BulkCloneOptions,
	groupHooks *CloneHooks,
) repository.RepositoryCloneResult {
	startTime := time.Now()
	destination, relativePath := cloneDestination(baseOpts.Directory, spec, repoName)
	branch, depth := cloneBranchAndDepth(baseOpts, spec)
	exists, isGitRepo := cloneTargetState(destination)
	shouldUpdate := baseOpts.Strategy != "" && baseOpts.Strategy != repository.StrategySkip

	if baseOpts.DryRun {
		return cloneResult(startTime, spec.URL, destination, relativePath, branch,
			cloneDryRunStatus(exists, isGitRepo, shouldUpdate), nil)
	}
	if exists && isGitRepo && !shouldUpdate {
		return cloneResult(startTime, spec.URL, destination, relativePath, branch, "skipped", nil)
	}

	mergedHooks := mergeHooks(groupHooks, spec.Hooks)
	if err := runCloneBeforeHooks(ctx, mergedHooks, baseOpts); err != nil {
		return cloneResult(startTime, spec.URL, destination, relativePath, branch, "error", err)
	}

	status, err := cloneOrUpdate(ctx, client, spec.URL, destination, branch, depth, baseOpts, exists, isGitRepo, shouldUpdate)
	result := cloneResult(startTime, spec.URL, destination, relativePath, branch, status, err)
	if result.Status != "error" {
		if err := runCloneAfterHooks(ctx, mergedHooks, destination, baseOpts.Logger); err != nil {
			result.Status = "error"
			result.Error = err
		}
	}
	return result
}

func cloneDestination(baseDir string, spec CloneRepoSpec, repoName string) (destination, relativePath string) {
	if spec.Path != "" {
		return filepath.Join(baseDir, spec.Path), spec.Path
	}
	return filepath.Join(baseDir, repoName), repoName
}

func cloneBranchAndDepth(opts repository.BulkCloneOptions, spec CloneRepoSpec) (branch string, depth int) {
	branch = string(spec.Branch)
	if branch == "" {
		branch = opts.Branch
	}
	depth = spec.Depth
	if depth == 0 {
		depth = opts.Depth
	}
	return branch, depth
}

func cloneTargetState(destination string) (exists, isGitRepo bool) {
	fi, err := os.Stat(destination)
	if err != nil {
		return false, false
	}
	exists = true
	if fi.IsDir() {
		_, err = os.Stat(filepath.Join(destination, ".git"))
		isGitRepo = err == nil
	}
	return exists, isGitRepo
}

func cloneDryRunStatus(exists, isGitRepo, shouldUpdate bool) string {
	if exists && isGitRepo {
		if shouldUpdate {
			return "would-update"
		}
		return "skipped"
	}
	return "would-clone"
}

func cloneResult(start time.Time, url, destination, relativePath, branch, status string, err error) repository.RepositoryCloneResult {
	return repository.RepositoryCloneResult{
		URL:          url,
		Path:         destination,
		RelativePath: relativePath,
		Branch:       branch,
		Status:       status,
		Error:        err,
		Duration:     time.Since(start),
	}
}

func runCloneBeforeHooks(ctx context.Context, hooks *CloneHooks, opts repository.BulkCloneOptions) error {
	if hooks == nil || len(hooks.Before) == 0 {
		return nil
	}
	parentDir := opts.Directory
	if parentDir == "" {
		parentDir = "."
	}
	if err := os.MkdirAll(parentDir, 0o755); err != nil { // #nosec G301 -- cloned repositories must retain normal shared directory readability.
		return fmt.Errorf("create parent directory for before hooks: %w", err)
	}
	if err := executeHooks(ctx, hooks.Before, parentDir, opts.Logger); err != nil {
		return fmt.Errorf("before hook: %w", err)
	}
	return nil
}

func cloneOrUpdate(ctx context.Context, client repository.Client, url, destination, branch string, depth int, opts repository.BulkCloneOptions, exists, isGitRepo, shouldUpdate bool) (string, error) {
	if exists && isGitRepo && shouldUpdate {
		updateResult, err := client.CloneOrUpdate(ctx, repository.CloneOrUpdateOptions{
			URL:         url,
			Destination: destination,
			Strategy:    opts.Strategy,
			Branch:      branch,
			Depth:       depth,
			Logger:      opts.Logger,
		})
		if err != nil {
			return "error", fmt.Errorf("update failed (strategy: %s): %w", opts.Strategy, err)
		}
		return updateResult.Action, nil
	}

	_, err := client.Clone(ctx, repository.CloneOptions{
		URL:          url,
		Destination:  destination,
		Branch:       branch,
		Depth:        depth,
		SingleBranch: cloneSingleBranch,
		Recursive:    cloneSubmodules,
	})
	if err != nil {
		return "error", err
	}
	return "cloned", nil
}

func runCloneAfterHooks(ctx context.Context, hooks *CloneHooks, destination string, logger repository.Logger) error {
	if hooks == nil || len(hooks.After) == 0 {
		return nil
	}
	if err := executeHooks(ctx, hooks.After, destination, logger); err != nil {
		return fmt.Errorf("after hook: %w", err)
	}
	return nil
}
