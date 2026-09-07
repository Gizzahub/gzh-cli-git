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
	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

const cleanupBranchSchema = "gz-git.cleanup.branch/v1"

// cleanupBranchBulkFlags holds bulk-specific flags.
var cleanupBranchBulkFlags BulkCommandFlags

var (
	cleanupBranchMerged     bool
	cleanupBranchStale      bool
	cleanupBranchGone       bool
	cleanupBranchSuperseded bool
	cleanupBranchStaleDays  int
	cleanupBranchDryRun     bool
	cleanupBranchForce      bool
	cleanupBranchYes        bool
	cleanupBranchRemote     bool
	cleanupBranchProtect    string
	cleanupBranchBaseBranch string
	cleanupBranchBots       bool
	cleanupBranchNonCanon   bool
)

// cleanupBranchCmd represents the cleanup branch command.
var cleanupBranchCmd = &cobra.Command{
	Use:   "branch [directory]",
	Short: "Clean up merged, stale, gone, or superseded branches",
	Long: cliutil.QuickStartHelp(`  # Preview merged branches in current repo
  gz-git cleanup branch --merged

  # Preview leftover Dependabot/Renovate remote branches
  gz-git cleanup branch --bots --merged -r

  # Preview bot remotes whose version already landed on base
  gz-git cleanup branch --bots --superseded -r

  # Machine-readable preview
  gz-git cleanup branch --bots --merged -r --format json

  # Actually delete bot remotes (bulk, non-interactive)
  gz-git cleanup branch --bots --merged -r --force --yes .

  # Preview branches that duplicate the declared canonical branch
  gz-git cleanup branch --non-canonical -r

  # Preview stale branches (no activity for 30 days)
  gz-git cleanup branch --stale

  # Actually delete merged branches
  gz-git cleanup branch --merged --force

  # BULK MODE: Clean up merged branches across all repos
  gz-git cleanup branch --merged --force .

  # Protect additional branches
  gz-git cleanup branch --merged --protect "staging,qa" --force`) + cliutil.ExitCodesBulkHelp(),
	Example: ``,
	RunE:    runCleanupBranch,
}

func init() {
	cleanupCmd.AddCommand(cleanupBranchCmd)

	// Cleanup-specific flags
	cleanupBranchCmd.Flags().BoolVar(&cleanupBranchMerged, "merged", false, "clean up fully merged branches")
	cleanupBranchCmd.Flags().BoolVar(&cleanupBranchStale, "stale", false, "clean up stale branches (no recent activity)")
	cleanupBranchCmd.Flags().BoolVar(&cleanupBranchGone, "gone", false, "clean up gone branches (remote deleted)")
	cleanupBranchCmd.Flags().BoolVar(&cleanupBranchSuperseded, "superseded", false, "clean up unmerged bot remotes whose version target is already on base")
	cleanupBranchCmd.Flags().BoolVar(&cleanupBranchNonCanon, "non-canonical", false, "retire branches that duplicate the canonical branch declared in .gz-git.yaml (requires a declaration)")
	cleanupBranchCmd.Flags().IntVar(&cleanupBranchStaleDays, "stale-days", 30, "days threshold for stale branches")
	cleanupBranchCmd.Flags().BoolVarP(&cleanupBranchDryRun, "dry-run", "n", true, "preview changes without deleting (default: true)")
	cleanupBranchCmd.Flags().BoolVar(&cleanupBranchForce, "force", false, "actually delete branches (disables dry-run)")
	cleanupBranchCmd.Flags().BoolVarP(&cleanupBranchYes, "yes", "y", false, "skip the confirmation prompt for bulk deletion (required in a non-interactive environment)")
	cleanupBranchCmd.Flags().BoolVarP(&cleanupBranchRemote, "remote", "r", false, "also delete remote branches")
	cleanupBranchCmd.Flags().BoolVar(&cleanupBranchBots, "bots", false, "only Dependabot, Renovate, and github-actions branches")
	cleanupBranchCmd.Flags().StringVar(&cleanupBranchProtect, "protect", "", "additional branches to protect (comma-separated)")
	cleanupBranchCmd.Flags().StringVar(&cleanupBranchBaseBranch, "base", "", "base branch for merge detection (default: auto-detect)")

	// Bulk operation flags (skip dry-run to avoid conflict with custom dry-run; skip recursive shorthand to avoid -r clash with --remote)
	addBulkFlagsWithOpts(cleanupBranchCmd, &cleanupBranchBulkFlags, BulkFlagOptions{SkipDryRun: true, SkipWatch: true, SkipFetch: true, SkipRecursive: true})
	cleanupBranchCmd.Flags().BoolVar(&cleanupBranchBulkFlags.IncludeSubmodules, "recursive", false, "recursively include nested repositories and submodules")
}

func runCleanupBranch(cmd *cobra.Command, args []string) error {
	// SIGINT/SIGTERM cancels the context so an in-progress deletion stops
	// gracefully instead of being hard-killed.
	ctx, cancel := withInterruptCancel(context.Background())
	defer cancel()

	// Require at least one cleanup type
	if !cleanupBranchMerged && !cleanupBranchStale && !cleanupBranchGone && !cleanupBranchSuperseded && !cleanupBranchNonCanon {
		return fmt.Errorf("specify at least one cleanup type: --merged, --stale, --gone, --superseded, or --non-canonical")
	}

	if err := validateBulkFormat(cleanupBranchBulkFlags.Format); err != nil {
		return err
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
		if err := resolveBulkDepth(cmd, args[0], &cleanupBranchBulkFlags.Depth); err != nil {
			return err
		}
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
		IncludeMerged:     cleanupBranchMerged,
		IncludeStale:      cleanupBranchStale,
		StaleThreshold:    time.Duration(cleanupBranchStaleDays) * 24 * time.Hour,
		IncludeRemote:     cleanupBranchRemote,
		IncludeGone:       cleanupBranchGone,
		IncludeSuperseded: cleanupBranchSuperseded,
		Exclude:           excludePatterns,
		BaseBranch:        cleanupBranchBaseBranch,
		BotsOnly:          cleanupBranchBots,
	}

	// The canonical declaration is resolved once, here, and only when the
	// classification that needs it was asked for. Resolving it unconditionally
	// would turn a missing or unresolvable .gz-git.yaml into an error for every
	// other cleanup type, none of which depend on it.
	if cleanupBranchNonCanon {
		canonical, taskPatterns, err := resolveCanonicalDeclaration(ctx, repo.Path)
		if err != nil {
			return err
		}
		analyzeOpts.IncludeNonCanonical = true
		analyzeOpts.CanonicalBranch = canonical
		analyzeOpts.TaskPatterns = taskPatterns
		analyzeOpts.CanonicalRemote = resolveGovernedRemote(ctx, repo)
	}

	machine := cliutil.IsMachineFormat(cleanupBranchBulkFlags.Format)
	if shouldShowProgress(cleanupBranchBulkFlags.Format, quiet) {
		fmt.Println("Analyzing branches...")
	}

	start := time.Now()
	report, err := svc.Analyze(ctx, repo, analyzeOpts)
	if err != nil {
		return fmt.Errorf("failed to analyze branches: %w", err)
	}
	currentBranch := ""
	if info, infoErr := repository.NewClient().GetInfo(ctx, repo); infoErr == nil && info != nil {
		currentBranch = info.Branch
	}

	// A dry run has to ask everything the real run asks, short of writing.
	screened, screenErr := screenDryRunCandidates(ctx, svc, repo, report, analyzeOpts, excludePatterns)
	if screenErr != nil {
		return screenErr
	}
	report = screened.report

	if !machine {
		if !quiet {
			printCleanupBranchReport(report, cleanupBranchDryRun)
		}
		printReportRefusals(report)
	}

	entries := cleanupEntriesFromReport(report)
	status := repository.StatusWouldCleanup
	if report.IsEmpty() {
		status = repository.StatusNothingToDo
	}
	// Same rule the bulk engine follows: a preview with nothing left to approve
	// and a refusal behind it is not "nothing to do" — that reads as a clean
	// repository, and this one has a branch the tool declined to touch.
	if len(entries) == 0 && len(screened.refused) > 0 {
		status = repository.StatusError
	}

	if cleanupBranchDryRun {
		return reportDryRunCleanup(repo, currentBranch, entries, status, start, screened, machine)
	}

	if report.IsEmpty() {
		if machine {
			writeCleanupBranchJSON(cleanupBranchJSONFromSingle(repo, currentBranch, nil, repository.StatusNothingToDo, "", start))
			return nil
		}
		if !quiet {
			// The refusals have already gone to stderr above. This line is the
			// stdout report, and it stops short of calling the repository clean
			// when a trunk here was examined and declined.
			if n := len(report.Refused); n > 0 {
				fmt.Printf("\n✓ No branches to clean up (%d trunk(s) examined and declined)\n", n)
			} else {
				fmt.Println("\n✓ No branches to clean up")
			}
		}
		return nil
	}

	// Force deletes unmerged branches too; Confirm is only consulted when
	// Force is false, so it is intentionally omitted here (setting it was a
	// no-op that read as "--force means the user confirmed").
	executeOpts := branch.ExecuteOptions{
		DryRun:          false,
		Force:           true,
		Remote:          cleanupBranchRemote,
		Exclude:         excludePatterns,
		CanonicalBranch: analyzeOpts.CanonicalBranch,
		CanonicalRemote: analyzeOpts.CanonicalRemote,
	}

	result, err := svc.Execute(ctx, repo, report, executeOpts)
	if err != nil {
		return fmt.Errorf("failed to execute cleanup: %w", err)
	}

	deleted := filterCleanupEntries(entries, result.Deleted)
	if machine {
		writeCleanupBranchJSON(cleanupBranchJSONFromSingle(repo, currentBranch, deleted, repository.StatusCleanedUp, "", start))
	} else if !quiet {
		fmt.Printf("\n✓ Deleted %d branch(es)\n", len(result.Deleted))
	}

	if len(result.Failed) > 0 {
		printDeleteFailures(result.Failed)

		return cliutil.NewExitError(cliutil.ExitPartialFailed,
			fmt.Errorf("%d of %d branches failed to delete",
				len(result.Failed), len(result.Deleted)+len(result.Failed)))
	}

	return nil
}

// runBulkCleanupBranch performs cleanup across multiple repositories.
func runBulkCleanupBranch(ctx context.Context, directory string, excludePatterns []string) error {
	client := repository.NewClient()

	opts := repository.BulkCleanupOptions{
		Directory:           directory,
		Parallel:            cleanupBranchBulkFlags.Parallel,
		MaxDepth:            cleanupBranchBulkFlags.Depth,
		DryRun:              cleanupBranchDryRun,
		IncludeMerged:       cleanupBranchMerged,
		IncludeStale:        cleanupBranchStale,
		IncludeGone:         cleanupBranchGone,
		IncludeSuperseded:   cleanupBranchSuperseded,
		IncludeNonCanonical: cleanupBranchNonCanon,
		// Bulk mode resolves the declaration per repository, not once for the
		// tree: each repository owns its own .gz-git.yaml, and a tree-wide
		// canonical branch would be exactly the guess this classification exists
		// to avoid. An undeclared repository yields no candidates and no error.
		CanonicalResolver: bulkCanonicalResolver,
		StaleThreshold:    time.Duration(cleanupBranchStaleDays) * 24 * time.Hour,
		BaseBranch:        cleanupBranchBaseBranch,
		DeleteRemote:      cleanupBranchRemote,
		BotsOnly:          cleanupBranchBots,
		ProtectPatterns:   excludePatterns,
		IncludeSubmodules: cleanupBranchBulkFlags.IncludeSubmodules,
		IncludePattern:    cleanupBranchBulkFlags.Include,
		ExcludePattern:    resolveScanExclude(directory, cleanupBranchBulkFlags.Exclude),
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

	if shouldShowProgress(cleanupBranchBulkFlags.Format, quiet) {
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

	format := cleanupBranchBulkFlags.Format
	if format == "json" || format == "llm" {
		writeCleanupBranchJSON(cleanupBranchJSONFromBulk(result, cleanupBranchDryRun))
	} else if !quiet || format == "default" || format == "compact" {
		printBulkCleanupBranchResult(result, cleanupBranchDryRun)
	}

	return errPartialFailure(result.Summary[repository.StatusError], result.TotalProcessed)
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
			branchCount += len(confirmationLines(&repo))
			repoCount++
		}
	}

	machine := cliutil.IsMachineFormat(cleanupBranchBulkFlags.Format)
	if branchCount == 0 {
		// Machine output still needs the empty JSON document from the real
		// run; returning true lets BulkCleanup emit status=nothing-to-do.
		if machine {
			return true, nil
		}
		if !quiet {
			fmt.Println("\n✓ No branches to clean up")
		}
		return false, nil
	}

	if !machine && !quiet {
		fmt.Printf("\nAbout to delete %d branch(es) across %d repositor(ies):\n", branchCount, repoCount)
		for _, repo := range preview.Repositories {
			if repo.Status == repository.StatusWouldCleanup && len(repo.DeletedBranches) > 0 {
				fmt.Printf("  %s: %s\n", repo.RelativePath, strings.Join(confirmationLines(&repo), ", "))
			}
		}
	}

	return confirmDestructiveBulk(cleanupBranchYes)
}

// confirmationLines renders what a bulk run will actually delete, one entry per
// ref.
//
// DeletedBranches is deduplicated by name on purpose — it is a flat summary —
// but a confirmation prompt is the one place that must not deduplicate: a local
// branch and its remote namesake are two deletions, and --non-canonical makes
// that pair the ordinary case rather than the exception. Counting names there
// would promise two deletions and perform four.
func confirmationLines(repo *repository.RepositoryCleanupResult) []string {
	if len(repo.Branches) == 0 {
		return repo.DeletedBranches
	}
	out := make([]string, 0, len(repo.Branches))
	for _, b := range repo.Branches {
		if b.Location == "remote" {
			out = append(out, b.Name+" (remote)")
			continue
		}
		out = append(out, b.Name)
	}
	return out
}

// printBulkCleanupBranchResult displays bulk cleanup results.
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
	blocked := 0
	errors := 0

	for _, repo := range result.Repositories {
		// Ahead of the status switch, and outside it: a declined trunk is worth
		// the same line whether the repository had other work to do or none at
		// all. NothingToDo is the case that needed it most — it prints nothing,
		// which is how "examined and declined" arrived looking like "clean".
		printRetireRefusals(&repo)

		switch repo.Status {
		case repository.StatusCleanedUp:
			cleanedUp++
			if verbose {
				fmt.Printf("✓ %s: %s\n", repo.RelativePath, repo.Message)
			}
			// Failures print regardless of --verbose: a branch the run could not
			// delete is the one thing here the operator has to act on.
			printCleanupFailures(&repo)
		case repository.StatusWouldCleanup:
			wouldCleanup++
			if !quiet {
				branchList := cleanupBranchListLabel(&repo)
				fmt.Printf("→ %s: %s%s\n", repo.RelativePath, repo.Message, branchList)
			}
			// Same rule as the execute path above: a refusal is what the operator
			// has to act on, and the remedy is in the failure text, not the count.
			// The all-blocked case sets StatusError and prints there; this is the
			// mixed repo, where "1 blocked" would otherwise name nothing.
			printCleanupFailures(&repo)
			blocked += len(repo.FailedBranches)
		case repository.StatusNothingToDo:
			nothingToDo++
		case repository.StatusError:
			errors++
			if !quiet {
				fmt.Printf("✗ %s: %s\n", repo.RelativePath, repo.Message)
			}
			printCleanupFailures(&repo)
		}
	}

	fmt.Println(strings.Repeat("─", 60))
	fmt.Printf("Repositories: %d scanned, %d processed\n", result.TotalScanned, result.TotalProcessed)
	fmt.Printf("Branches: %d analyzed\n", result.TotalBranchesAnalyzed)

	if dryRun {
		fmt.Printf("Would clean up: %d repo(s), Nothing to do: %d, Errors: %d\n", wouldCleanup, nothingToDo, errors)
		if blocked > 0 {
			// Counted only from would-cleanup repos. A fully blocked repo is a
			// StatusError and is already in that total; adding it here too would
			// report the same refusal twice under two names.
			fmt.Printf("Blocked: %d branch(es) — listed above\n", blocked)
		}
		fmt.Printf("\nDry-run mode: use --force to actually delete branches\n")
	} else {
		fmt.Printf(
			"Cleaned up: %d repo(s), Deleted: %d branch(es), Failed: %d branch(es), Errors: %d\n",
			cleanedUp, result.TotalBranchesDeleted, result.TotalBranchesFailed, errors,
		)
	}

	fmt.Printf("Duration: %s\n", result.Duration.Round(time.Millisecond))
}

// dryRunScreen is what a preview knows after asking every question the real run
// asks: the candidates that survived, and the refusals that did not.
type dryRunScreen struct {
	report         *branch.CleanupReport
	refused        []branch.DeleteFailure
	refusedEntries []repository.CleanupFailureEntry
	// retireRefusals travels alongside, carried from the report rather than
	// produced by the screen: these branches never reached a delete, so the
	// gate has nothing to say about them. It rides here so the dry-run
	// reporters need only one argument to answer both halves.
	retireRefusals []repository.RetireRefusalEntry
}

// screenDryRunCandidates puts the preview through the same screening the real
// run uses. The canonical-tip gate lives inside Execute, and the dry-run path
// used to return straight from Analyze without ever reaching it — so the
// preview offered branches the run then refused, and the operator approved one
// thing and received another. Execute with DryRun is that same screening with
// the deletes turned off: one evaluation reused, not a second copy of the
// decision that can drift away from it.
//
// Analyze already routes protected branches into report.Protected, so for a
// report this command built, Skipped is empty and the only thing this adds is
// the gate. That is the whole reason to come through here.
//
// Outside a dry run it does nothing: Execute is about to run for real and will
// apply the same screening itself.
func screenDryRunCandidates(
	ctx context.Context,
	svc branch.CleanupService,
	repo *repository.Repository,
	report *branch.CleanupReport,
	analyzeOpts branch.AnalyzeOptions,
	excludePatterns []string,
) (dryRunScreen, error) {
	if !cleanupBranchDryRun {
		return dryRunScreen{report: report, retireRefusals: retireRefusalEntriesFromReport(report)}, nil
	}

	screen, err := svc.Execute(ctx, repo, report, branch.ExecuteOptions{
		DryRun:          true,
		Force:           true,
		Remote:          cleanupBranchRemote,
		Exclude:         excludePatterns,
		CanonicalBranch: analyzeOpts.CanonicalBranch,
		CanonicalRemote: analyzeOpts.CanonicalRemote,
	})
	if err != nil {
		return dryRunScreen{}, fmt.Errorf("failed to screen cleanup candidates: %w", err)
	}

	return dryRunScreen{
		report:  reportWithoutBranches(report, screen.Failed),
		refused: screen.Failed,
		// The vocabulary a refusal is described in comes from the same builder
		// that describes the deletable candidates, looked up before the branch
		// is filtered out of the report. Spelling "non-canonical"/"remote" a
		// second time here is how the two lists start disagreeing about what to
		// call the same branch.
		refusedEntries: failureEntriesFrom(screen.Failed, cleanupEntriesFromReport(report)),
		retireRefusals: retireRefusalEntriesFromReport(report),
	}, nil
}

// reportDryRunCleanup emits the preview on whichever channel was asked for. The
// refusals print exactly as the real run prints them: a preview whose only
// difference from the run is that nothing was written is the preview worth
// having, and a machine caller has no scrollback to fall back on, so the remedy
// text travels in the document too.
func reportDryRunCleanup(
	repo *repository.Repository,
	currentBranch string,
	entries []repository.CleanupBranchEntry,
	status string,
	start time.Time,
	screened dryRunScreen,
	machine bool,
) error {
	if machine {
		out := cleanupBranchJSONFromSingle(repo, currentBranch, entries, status, "", start)
		out.Repositories[0].FailedBranches = screened.refusedEntries
		out.Repositories[0].RetireRefusals = screened.retireRefusals
		writeCleanupBranchJSON(out)

		return dryRunRefusalError(entries, screened.refused)
	}

	printDeleteFailures(screened.refused)

	if len(entries) == 0 && len(screened.refused) == 0 {
		if !quiet {
			// printReportRefusals has already named these on stderr; what this
			// line must not do is call the repository clean anyway.
			if n := len(screened.retireRefusals); n > 0 {
				fmt.Printf("\n✓ No branches to clean up (%d trunk(s) examined and declined)\n", n)
			} else {
				fmt.Println("\n✓ No branches to clean up")
			}
		}

		return nil
	}

	if !quiet && len(entries) > 0 {
		fmt.Println("\nDry-run mode: use --force to actually delete branches")
	}

	return dryRunRefusalError(entries, screened.refused)
}

// printDeleteFailures lists refusals on stderr. The dry run and the real run
// call this same function, so a change to how a refusal reads cannot land in
// one and miss the other.
func printDeleteFailures(failed []branch.DeleteFailure) {
	if len(failed) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "\n✗ Failed to delete %d branch(es):\n", len(failed))
	for _, f := range failed {
		fmt.Fprintf(os.Stderr, "  %-30s %v\n", f.Branch, f.Err)
	}
}

// failureEntriesFrom converts screening refusals into the machine-readable
// shape. A scripted caller has no scrollback to squint at, so the remedy text
// has to travel in the document rather than only on stderr.
func failureEntriesFrom(
	failed []branch.DeleteFailure, described []repository.CleanupBranchEntry,
) []repository.CleanupFailureEntry {
	if len(failed) == 0 {
		return nil
	}
	byName := make(map[string]repository.CleanupBranchEntry, len(described))
	for _, e := range described {
		byName[e.Name] = e
	}
	out := make([]repository.CleanupFailureEntry, 0, len(failed))
	for _, f := range failed {
		entry := repository.CleanupFailureEntry{Name: f.Branch, Error: f.Err.Error()}
		if d, ok := byName[f.Branch]; ok {
			entry.Reason = d.Reason
			entry.Location = d.Location
		}
		out = append(out, entry)
	}
	return out
}

// dryRunRefusalError decides what a blocked preview exits with, on the rule the
// bulk engine already follows: a partial plan is still a plan the operator will
// go on to execute, so it exits zero; a preview with nothing left to approve
// and a refusal behind it is not a plan at all, and reporting success for it is
// the report the operator would act on wrongly.
func dryRunRefusalError(entries []repository.CleanupBranchEntry, refused []branch.DeleteFailure) error {
	if len(refused) == 0 || len(entries) > 0 {
		return nil
	}
	return cliutil.NewExitError(cliutil.ExitPartialFailed,
		fmt.Errorf("%d branch(es) blocked, none deletable", len(refused)))
}

// reportWithoutBranches drops the named branches from every candidate bucket.
// The gate only ever refuses non-canonical candidates today; filtering all of
// them costs nothing and means a gate added to another bucket later cannot
// leave a refused branch sitting in the preview.
func reportWithoutBranches(report *branch.CleanupReport, failed []branch.DeleteFailure) *branch.CleanupReport {
	if report == nil || len(failed) == 0 {
		return report
	}
	drop := make(map[string]struct{}, len(failed))
	for _, f := range failed {
		drop[f.Branch] = struct{}{}
	}
	keep := func(in []*branch.Branch) []*branch.Branch {
		out := make([]*branch.Branch, 0, len(in))
		for _, b := range in {
			if _, blocked := drop[b.Name]; blocked {
				continue
			}
			out = append(out, b)
		}
		return out
	}
	filtered := *report
	filtered.Merged = keep(report.Merged)
	filtered.Stale = keep(report.Stale)
	filtered.Orphaned = keep(report.Orphaned)
	filtered.Superseded = keep(report.Superseded)
	filtered.NonCanonical = keep(report.NonCanonical)
	return &filtered
}

// printCleanupFailures lists the branches a repository could not delete.
//
// stderr, and not gated on quiet, matching printDeleteFailures on the single
// engine. A refusal is a diagnostic, not progress: --quiet conventionally
// silences the latter, and putting the refusal on stderr leaves the operator
// the conventional opt-out (2>/dev/null) instead of an all-or-nothing flag.
//
// This used to honor quiet, on the reasoning that the machine channel would
// carry the detail. That channel does carry it now, but it only ever helped
// callers who chose --format json, and --quiet on its own is the common
// scripted invocation — so the refusal reached nothing at all.
func printCleanupFailures(repo *repository.RepositoryCleanupResult) {
	for _, f := range repo.FailedBranches {
		fmt.Fprintf(os.Stderr, "  ✗ %s: %s (%s) — %s\n", repo.RelativePath, f.Name, f.Location, f.Error)
	}
}

// printRetireRefusals reports trunks the non-canonical gate examined and turned
// down.
//
// stderr, and not gated on --quiet, for the reason TASK-181 settled: --quiet
// silences progress on stdout, not diagnostics on stderr, and a refusal is a
// diagnostic. The operator who wants it gone has 2>/dev/null, which is
// per-stream rather than all-or-nothing. The counts stay on stdout — those are
// report.
//
// The marker is ⊘ rather than ✗: nothing failed here. The gate did its job and
// the answer was no.
// cleanupBranchListLabel renders the preview's branch list, carrying the same
// basis label the single-repo report puts on its candidate lines.
//
// It reads repo.Branches rather than repo.DeletedBranches because only the
// former knows what each candidate was measured against; DeletedBranches is a
// deduplicated list of names and has nowhere to put it. The dedup is kept —
// local and remote copies share a name — but keyed on name and location, since
// those are two different refs whose bases can differ, and collapsing them was
// how a remote retirement came to be described by a local measurement.
//
// It falls back to DeletedBranches when Branches is empty, which is the shape a
// caller assembling a result by hand produces.
func cleanupBranchListLabel(repo *repository.RepositoryCleanupResult) string {
	if len(repo.Branches) == 0 {
		if len(repo.DeletedBranches) == 0 {
			return ""
		}
		return fmt.Sprintf(" [%s]", strings.Join(repo.DeletedBranches, ", "))
	}

	seen := make(map[string]bool, len(repo.Branches))
	labels := make([]string, 0, len(repo.Branches))
	for _, e := range repo.Branches {
		key := e.Name + "\x00" + e.Location
		if seen[key] {
			continue
		}
		seen[key] = true
		labels = append(labels, e.Name+entryBasisLabel(e))
	}

	return fmt.Sprintf(" [%s]", strings.Join(labels, ", "))
}

// entryBasisLabel is the bulk engine's spelling of retireBasisLabel. The two
// produce the same text from the same facts; keeping them phrased identically
// is what criterion 2 of the card asks for, and a diverging phrase here would
// mean the preview an operator reads depends on which command they reached it
// through.
func entryBasisLabel(e repository.CleanupBranchEntry) string {
	if e.TargetRef == "" {
		return ""
	}
	if e.TargetSHA == "" {
		return fmt.Sprintf(" → contained in %s", e.TargetRef)
	}
	return fmt.Sprintf(" → contained in %s @ %s", e.TargetRef, shortObjectID(e.TargetSHA))
}

// retireBasisLabel renders the fact that authorized retiring one candidate:
// the ref its ancestry was measured against, and where that ref pointed.
//
// This is report, not diagnostic, so it stays on stdout with the candidate line
// and is silenced by --quiet along with it — the opposite side of the split
// that sends refusals to stderr. The two are different questions and only look
// like one because they concern the same branch: a refusal changes what the
// operator does next, while this changes whether they can justify what they
// were already going to do.
//
// The target ref is spelled in full on purpose. Since the local→remote fallback
// landed, `master` may have been measured against refs/heads/develop or against
// refs/remotes/origin/develop, and abbreviating them both to "develop" would
// hide exactly the distinction the label exists to show. The short id says when
// that ref was last seen, which is the question a cached target raises.
//
// An unresolvable basis renders as nothing rather than as a placeholder. A line
// with no label reads as "no extra information"; a line reading "→ unknown"
// reads as a defect in the branch.
func retireBasisLabel(report *branch.CleanupReport, b *branch.Branch) string {
	if report == nil || b == nil {
		return ""
	}
	ref := b.Ref
	if ref == "" {
		ref = b.Name
	}
	for _, basis := range report.Bases {
		if basis.Ref != ref || basis.TargetRef == "" {
			continue
		}
		if basis.TargetSHA == "" {
			return fmt.Sprintf(" → contained in %s", basis.TargetRef)
		}
		return fmt.Sprintf(" → contained in %s @ %s", basis.TargetRef, shortObjectID(basis.TargetSHA))
	}
	return ""
}

// shortObjectID abbreviates an object id for an operator-facing line.
func shortObjectID(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// printReportRefusals is the single-repo half of printRetireRefusals, and is
// kept out of printCleanupBranchReport on purpose: that function is the stdout
// report and its caller gates it on --quiet. A refusal is a diagnostic, so it
// must survive --quiet, which means it cannot live inside the thing --quiet
// silences. Machine mode is the one exemption — there the JSON document carries
// it, and a second copy on stderr would be noise beside a parsed stream.
func printReportRefusals(report *branch.CleanupReport) {
	if report == nil || len(report.Refused) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "\n⊘ Trunks examined and declined (%d):\n", len(report.Refused))
	for _, r := range report.Refused {
		location := ""
		if r.IsRemote {
			location = " (remote)"
		}
		fmt.Fprintf(os.Stderr, "   ⊘ %s%s — %s\n", r.Branch, location, r.Reason)
	}
}

func printRetireRefusals(repo *repository.RepositoryCleanupResult) {
	for _, r := range repo.RetireRefusals {
		fmt.Fprintf(os.Stderr, "  ⊘ %s: %s (%s) — %s\n", repo.RelativePath, r.Name, r.Location, r.Reason)
	}
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

	if len(report.NonCanonical) > 0 {
		fmt.Printf("\n🏷  Non-canonical branches (%d):\n", len(report.NonCanonical))
		for _, b := range report.NonCanonical {
			// Local and remote copies of the same branch both land here and print
			// the same bare name, so the marker is what tells the reader which ref
			// a line is about before they pass --force.
			location := ""
			if b.IsRemote {
				location = " (remote)"
			}
			fmt.Printf("   • %s%s%s\n", b.Name, location, retireBasisLabel(report, b))
		}
	}

	if len(report.Superseded) > 0 {
		fmt.Printf("\n📦 Superseded bot branches (%d):\n", len(report.Superseded))
		for _, b := range report.Superseded {
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

// resolveCanonicalDeclaration reads the repository's own .gz-git.yaml for the
// canonical branch and the task-branch allow-list.
//
// A repository that declares no canonical branch gets an error rather than a
// fallback. Every other cleanup type can lean on a name heuristic when the
// declaration is silent, because the worst case is that it skips a branch. This
// one authorizes deleting a branch git protects by name, so the only acceptable
// basis is an explicit declaration — guessing here would delete master because
// master merely looks retired.
func resolveCanonicalDeclaration(ctx context.Context, repoPath string) (canonical string, taskPatterns []string, err error) {
	canonical, err = config.ResolveDeclaredIntegrationBranch(ctx, repoPath)
	if err != nil {
		return "", nil, fmt.Errorf("--non-canonical: %w", err)
	}
	if canonical == "" {
		return "", nil, fmt.Errorf(
			"--non-canonical requires branch.integrationBranch in %s/.gz-git.yaml; "+
				"without it there is no declared canonical branch to retire the others against", repoPath,
		)
	}

	decl, err := config.LoadRepoRootTaskPattern(repoPath)
	if err != nil {
		return "", nil, fmt.Errorf("--non-canonical: load task-branch declaration: %w", err)
	}

	return canonical, decl.Patterns, nil
}

// resolveGovernedRemote names the one remote a .gz-git.yaml speaks for.
//
// It mirrors how the bulk path picks its remote — repository info, falling back
// to origin — so the two cleanup paths cannot disagree about which remote a
// declaration governs. That divergence is exactly what let the single-repo path
// aim a delete at a fork's `upstream`.
func resolveGovernedRemote(ctx context.Context, repo *repository.Repository) string {
	info, err := repository.NewClient().GetInfo(ctx, repo)
	if err == nil && info != nil && strings.TrimSpace(info.Remote) != "" {
		return info.Remote
	}
	return repository.DefaultRemoteName
}

// bulkCanonicalResolver adapts the per-repository declaration lookup to the
// signature pkg/repository accepts.
//
// It reports "no declaration" as ("", nil, nil) rather than an error. In bulk
// mode a repository that never declared a canonical branch is an ordinary
// member of the tree, not a fault: it simply contributes no candidates, and the
// scan continues. The single-repository path makes the opposite call, because
// there the user named that one repository and a silent no-op would read as
// "nothing to clean up".
func bulkCanonicalResolver(ctx context.Context, repoPath string) (canonical string, taskPatterns []string, err error) {
	canonical, err = config.ResolveDeclaredIntegrationBranch(ctx, repoPath)
	if err != nil || canonical == "" {
		return "", nil, nil //nolint:nilerr // an undeclared or unresolvable repo contributes nothing; it does not fail the tree
	}
	decl, err := config.LoadRepoRootTaskPattern(repoPath)
	if err != nil {
		return "", nil, nil //nolint:nilerr // same policy: skip this repository rather than fail the run
	}
	return canonical, decl.Patterns, nil
}
