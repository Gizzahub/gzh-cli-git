package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

var (
	pushFlags       BulkCommandFlags
	pushForce       bool
	pushSetUpstream bool
	pushTags        bool
	pushRefspec     string
	pushRemotes     []string
	pushAllRemotes  bool
	pushIgnoreDirty bool
	pushForceMode   string
	pushForeignWork string
)

// pushCmd represents the push command for multi-repository operations.
var pushCmd = &cobra.Command{
	Use:   "push [directory]",
	Short: "Push commits to multiple repositories in parallel",
	Long: cliutil.QuickStartHelp(`  # Push all repositories in current directory
  gz-git push

  # Push to multiple remotes
  gz-git push --remote origin --remote backup ~/projects

  # Push with custom refspec (local:remote branch mapping)
  gz-git push --refspec develop:master ~/projects

  # Force push (use with caution!)
  gz-git push --force ~/projects

  # Push and set upstream for new branches
  gz-git push --set-upstream ~/projects

  # Push all tags
  gz-git push --tags ~/repos

  # Skip dirty status check (useful for CI/CD)
  gz-git push --ignore-dirty ~/projects`) + cliutil.ExitCodesBulkHelp(),
	Args: cobra.MaximumNArgs(1),
	RunE: runPush,
}

func init() {
	rootCmd.AddCommand(pushCmd)

	// Common bulk operation flags
	addBulkFlags(pushCmd, &pushFlags)

	// Push-specific flags
	pushCmd.Flags().BoolVarP(&pushForce, "force", "f", false, "force push (use with caution!)")
	pushCmd.Flags().BoolVarP(&pushSetUpstream, "set-upstream", "u", false, "set upstream for new branches")
	pushCmd.Flags().BoolVarP(&pushTags, "tags", "t", false, "push all tags to remote")
	pushCmd.Flags().StringVar(&pushRefspec, "refspec", "", "custom refspec (e.g., 'develop:master' to push local develop to remote master)")
	pushCmd.Flags().StringSliceVar(&pushRemotes, "remote", []string{}, "remote(s) to push to (can be specified multiple times)")
	pushCmd.Flags().BoolVar(&pushAllRemotes, "all-remotes", false, "push to all configured remotes")
	pushCmd.Flags().BoolVar(&pushIgnoreDirty, "ignore-dirty", false, "skip dirty status check and warning (useful for CI/CD)")
	addForceModeFlag(pushCmd, &pushForceMode)
	addForeignWorkFlag(pushCmd, &pushForeignWork)
}

func runPush(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Load config with profile support
	effective, _ := LoadEffectiveConfig(cmd, nil)
	if effective != nil {
		// Apply config if flag not explicitly set
		if !cmd.Flags().Changed("parallel") && effective.Parallel > 0 {
			pushFlags.Parallel = effective.Parallel
		}
		if !cmd.Flags().Changed("set-upstream") {
			pushSetUpstream = effective.Push.SetUpstream
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
	if err := validateBulkDepth(cmd, pushFlags.Depth); err != nil {
		return err
	}

	// Validate format
	if err := validateBulkFormat(pushFlags.Format); err != nil {
		return err
	}

	// Validate refspec if provided
	if pushRefspec != "" {
		if _, err := repository.ValidateRefspec(pushRefspec); err != nil {
			return fmt.Errorf("invalid refspec: %w", err)
		}
	}

	guards, err := resolvePushGuards(effective, pushOverrides{
		forceMode:   pushForceMode,
		foreignWork: pushForeignWork,
	})
	if err != nil {
		return err
	}

	// Create client
	client := repository.NewClient()

	// Create logger for verbose mode
	logger := createBulkLogger(verbose)

	// Build options
	opts := repository.BulkPushOptions{
		Directory:         directory,
		Parallel:          pushFlags.Parallel,
		MaxDepth:          pushFlags.Depth,
		DryRun:            pushFlags.DryRun,
		Verbose:           verbose,
		Force:             pushForce,
		SetUpstream:       pushSetUpstream,
		Tags:              pushTags,
		Refspec:           pushRefspec,
		Remotes:           pushRemotes,
		AllRemotes:        pushAllRemotes,
		IgnoreDirty:       pushIgnoreDirty,
		Policy:            guards.policy,
		Identity:          guards.identity,
		IncludeSubmodules: pushFlags.IncludeSubmodules,
		IncludePattern:    pushFlags.Include,
		ExcludePattern:    resolveScanExclude(directory, pushFlags.Exclude),
		Logger:            logger,
		ProgressCallback:  createProgressCallback("Pushing", pushFlags.Format, quiet),
	}

	// Watch mode: continuously push at intervals
	if pushFlags.Watch {
		return runPushWatch(ctx, client, opts)
	}

	// One-time push
	if shouldShowProgress(pushFlags.Format, quiet) {
		printScanningMessage(directory, pushFlags.Depth, pushFlags.Parallel, pushFlags.DryRun)
	}

	result, err := client.BulkPush(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk push failed: %w", err)
	}

	// Display scan completion message
	if shouldShowProgress(pushFlags.Format, quiet) && result.TotalScanned == 0 {
		fmt.Printf("Scan complete: no repositories found\n")
	}

	// Display results (always output for JSON format, otherwise respect quiet flag)
	if pushFlags.Format == "json" || !quiet {
		displayPushResults(result)
	}

	// A policy refusal is a failure, not a skip: the caller asked for a push
	// that did not happen, and a zero exit would hide that from a script.
	failed := result.Summary[repository.StatusError] + result.Summary[repository.StatusBlocked]

	return errPartialFailure(failed, result.TotalProcessed)
}

func runPushWatch(ctx context.Context, client repository.Client, opts repository.BulkPushOptions) error {
	cfg := WatchConfig{
		Interval:      pushFlags.Interval,
		Format:        pushFlags.Format,
		Quiet:         quiet,
		OperationName: "push",
		Directory:     opts.Directory,
		MaxDepth:      opts.MaxDepth,
		Parallel:      opts.Parallel,
	}

	return RunBulkWatch(cfg, func() error {
		return executePush(ctx, client, opts)
	})
}

func executePush(ctx context.Context, client repository.Client, opts repository.BulkPushOptions) error {
	result, err := client.BulkPush(ctx, opts)
	if err != nil {
		return fmt.Errorf("bulk push failed: %w", err)
	}

	// Display results
	if !quiet {
		displayPushResults(result)
	}

	return nil
}

func displayPushResults(result *repository.BulkPushResult) {
	rows := make([]BulkRenderRow, 0, len(result.Repositories))
	for _, repo := range result.Repositories {
		rows = append(rows, BulkRenderRow{
			Path:             repo.GetPath(),
			Branch:           repo.Branch,
			Status:           repo.GetStatus(),
			Message:          repo.GetMessage(),
			Remote:           repo.Remote,
			Err:              repo.GetError(),
			Duration:         repo.Duration,
			CommitsAhead:     repo.CommitsAhead,
			PushedCommits:    repo.PushedCommits,
			UncommittedFiles: repo.TrackedChangedFiles,
			UntrackedFiles:   repo.UntrackedFiles,
		})
	}

	issueStatuses := issueStatusSet(
		"error", "blocked", "no-remote", "no-upstream", "conflict",
		"rebase-in-progress", "merge-in-progress",
	)
	if pushFlags.Format != "compact" {
		issueStatuses["auth-required"] = true
	}

	RenderBulkResults(os.Stdout, BulkRenderConfig{
		Title:         "=== Push Results ===",
		Verb:          "Pushed",
		Format:        pushFlags.Format,
		Verbose:       verbose,
		IssueStatuses: issueStatuses,
		FormatStatus:  formatPushStatus,
		ChangesCount:  func(row BulkRenderRow) int { return row.PushedCommits },
		// A policy refusal is only actionable if the rule is named, so show it
		// without needing --verbose.
		AlwaysShowError: func(row BulkRenderRow) bool {
			return isRefspecError(row.Err) || row.Status == repository.StatusBlocked
		},
		SuccessMessage: "✓ All repositories pushed successfully",
		ShowFooters:    true,
	}, BulkRenderInput{
		TotalScanned:   result.TotalScanned,
		TotalProcessed: result.TotalProcessed,
		Duration:       result.Duration,
		Summary:        result.Summary,
		Rows:           rows,
	})
}

func formatPushStatus(row BulkRenderRow) string {
	switch row.Status {
	case "success", "pushed":
		if row.PushedCommits > 0 {
			return fmt.Sprintf("%d↑ pushed", row.PushedCommits)
		}
		return "up-to-date"
	case "nothing-to-push", "up-to-date":
		return "up-to-date"
	case "would-push":
		if row.CommitsAhead > 0 {
			return fmt.Sprintf("would push %d↑", row.CommitsAhead)
		}
		return "would push"
	case "error":
		return "failed"
	case "blocked":
		return "BLOCKED"
	case "no-remote":
		return "no remote"
	case "no-upstream":
		return "no upstream"
	case "conflict":
		return "CONFLICT"
	case "rebase-in-progress":
		return "REBASE"
	case "merge-in-progress":
		return "MERGE"
	case "skipped":
		return "skipped"
	default:
		return row.Status
	}
}
