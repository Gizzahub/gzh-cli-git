// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package wizard

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/huh"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/branch"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/scanner"
)

// BranchCleanupOptions holds the results of the branch cleanup wizard.
type BranchCleanupOptions struct {
	Directory      string
	IncludeMerged  bool
	IncludeStale   bool
	IncludeGone    bool
	StaleThreshold time.Duration
	Force          bool
	IncludeRemote  bool
}

// BranchCleanupResult holds the results of cleanup execution.
type BranchCleanupResult struct {
	ReposProcessed  int
	BranchesDeleted int
	BranchesSkipped int
	Errors          []string
}

// BranchCleanupWizard guides users through branch cleanup.
type BranchCleanupWizard struct {
	printer        *Printer
	directory      string
	opts           BranchCleanupOptions
	cleanupService branch.CleanupService
	repoClient     repository.Client
}

// NewBranchCleanupWizard creates a new branch cleanup wizard.
func NewBranchCleanupWizard(directory string) *BranchCleanupWizard {
	if directory == "" {
		directory = "."
	}

	return &BranchCleanupWizard{
		printer:        NewPrinter(),
		directory:      directory,
		cleanupService: branch.NewCleanupService(),
		repoClient:     repository.NewClient(),
		opts: BranchCleanupOptions{
			Directory:      directory,
			IncludeMerged:  true,
			StaleThreshold: 30 * 24 * time.Hour,
		},
	}
}

// Run executes the branch cleanup wizard.
func (w *BranchCleanupWizard) Run(ctx context.Context) (*BranchCleanupResult, error) {
	w.printer.PrintHeader(IconBroom, "Branch Cleanup Wizard")
	w.printer.PrintInfo("This wizard will help you clean up merged, stale, or gone branches.")
	fmt.Println()

	// Step 1: Configure options
	if err := w.runOptionsStep(); err != nil {
		return nil, err
	}

	// Step 2: Scan for repositories
	repos, err := w.scanRepositories(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to scan repositories: %w", err)
	}

	if len(repos) == 0 {
		w.printer.PrintWarning("No Git repositories found in " + w.opts.Directory)
		return &BranchCleanupResult{}, nil
	}

	w.printer.PrintSuccess(fmt.Sprintf("Found %d repositories", len(repos)))
	fmt.Println()

	// Step 3: Process each repository
	result := &BranchCleanupResult{}
	for _, repoPath := range repos {
		deleted, skipped, err := w.processRepository(ctx, repoPath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", filepath.Base(repoPath), err))
			continue
		}
		result.ReposProcessed++
		result.BranchesDeleted += deleted
		result.BranchesSkipped += skipped
	}

	// Print summary
	w.printSummary(result)

	return result, nil
}

func (w *BranchCleanupWizard) runOptionsStep() error {
	directory := w.directory

	var cleanupTypes []string
	staleThresholdDays := "30"

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Directory").
				Description("Directory to scan for Git repositories").
				Placeholder(".").
				Value(&directory),

			huh.NewMultiSelect[string]().
				Title("Cleanup Types").
				Description("Select which branch types to clean up").
				Options(
					huh.NewOption("Merged branches (safe)", "merged").Selected(true),
					huh.NewOption("Stale branches (no recent commits)", "stale"),
					huh.NewOption("Gone branches (remote deleted)", "gone"),
				).
				Value(&cleanupTypes),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Stale Threshold (days)").
				Description("Branches with no commits older than this are stale").
				Placeholder("30").
				Value(&staleThresholdDays),

			huh.NewConfirm().
				Title("Include Remote Branches").
				Description("Also clean up remote tracking branches").
				Affirmative("Yes").
				Negative("No").
				Value(&w.opts.IncludeRemote),

			huh.NewConfirm().
				Title("Force Delete").
				Description("Force delete unmerged branches (dangerous!)").
				Affirmative("Yes (I understand the risk)").
				Negative("No").
				Value(&w.opts.Force),
		),
	).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		return err
	}

	// Parse options
	w.opts.Directory = directory
	w.opts.IncludeMerged = contains(cleanupTypes, "merged")
	w.opts.IncludeStale = contains(cleanupTypes, "stale")
	w.opts.IncludeGone = contains(cleanupTypes, "gone")

	if days := ParseParallel(staleThresholdDays, 30); days > 0 {
		w.opts.StaleThreshold = time.Duration(days) * 24 * time.Hour
	}

	return nil
}

func (w *BranchCleanupWizard) scanRepositories(ctx context.Context) ([]string, error) {
	w.printer.PrintInfo("Scanning for repositories...")

	gitScanner := &scanner.GitRepoScanner{
		RootPath:         w.opts.Directory,
		MaxDepth:         2,
		RespectGitIgnore: true,
	}

	scannedRepos, err := gitScanner.Scan(ctx)
	if err != nil {
		return nil, err
	}

	repos := make([]string, 0, len(scannedRepos))
	for _, repo := range scannedRepos {
		repos = append(repos, repo.Path)
	}

	return repos, nil
}

func (w *BranchCleanupWizard) processRepository(ctx context.Context, repoPath string) (deleted, skipped int, err error) {
	repoName := filepath.Base(repoPath)
	w.printer.PrintDivider()
	w.printer.PrintSubtitle(fmt.Sprintf("Repository: %s", repoName))
	fmt.Println()

	// Open repository
	repo, err := w.repoClient.Open(ctx, repoPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open repository: %w", err)
	}

	// Analyze branches
	analyzeOpts := branch.AnalyzeOptions{
		IncludeMerged:  w.opts.IncludeMerged,
		IncludeStale:   w.opts.IncludeStale,
		StaleThreshold: w.opts.StaleThreshold,
		IncludeRemote:  w.opts.IncludeRemote,
	}

	report, err := w.cleanupService.Analyze(ctx, repo, analyzeOpts)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to analyze branches: %w", err)
	}

	if report.IsEmpty() {
		w.printer.PrintInfo("No branches to clean up")
		return 0, 0, nil
	}

	// Build branch list for selection
	branches := report.GetAllBranches()
	if len(branches) == 0 {
		w.printer.PrintInfo("No branches eligible for cleanup")
		return 0, 0, nil
	}

	// Create options for multi-select
	options := make([]huh.Option[string], 0, len(branches))
	preSelected := make([]string, 0)

	for _, b := range branches {
		label := formatBranchForSelection(b, report)
		opt := huh.NewOption(label, b.Name)

		// Pre-select merged branches (safest)
		for _, merged := range report.Merged {
			if merged.Name == b.Name {
				opt = opt.Selected(true)
				preSelected = append(preSelected, b.Name)
				break
			}
		}

		options = append(options, opt)
	}

	// Let user select branches to delete
	selectedBranches := preSelected

	selectForm := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title(fmt.Sprintf("Select branches to delete (%d eligible)", len(branches))).
				Description("Use space to toggle, enter to confirm").
				Options(options...).
				Value(&selectedBranches),
		),
	).WithTheme(huh.ThemeCharm())

	if err := selectForm.Run(); err != nil {
		return 0, 0, err
	}

	if len(selectedBranches) == 0 {
		w.printer.PrintInfo("No branches selected, skipping")
		return 0, len(branches), nil
	}

	// Confirm deletion
	var confirmDelete bool
	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Delete %d branches?", len(selectedBranches))).
				Description("This action cannot be undone").
				Affirmative("Yes, delete").
				Negative("No, skip").
				Value(&confirmDelete),
		),
	).WithTheme(huh.ThemeCharm())

	if err := confirmForm.Run(); err != nil {
		return 0, 0, err
	}

	if !confirmDelete {
		w.printer.PrintInfo("Deletion canceled")
		return 0, len(selectedBranches), nil
	}

	// Execute deletion
	deletedCount := 0
	for _, branchName := range selectedBranches {
		// Find the branch in the report
		var branchInfo *branch.Branch
		for _, b := range branches {
			if b.Name == branchName {
				branchInfo = b
				break
			}
		}

		if branchInfo == nil {
			continue
		}

		// Create a filtered report with just this branch
		filteredReport := &branch.CleanupReport{
			Merged:   filterBranches(report.Merged, branchName),
			Stale:    filterBranches(report.Stale, branchName),
			Orphaned: filterBranches(report.Orphaned, branchName),
		}

		executeOpts := branch.ExecuteOptions{
			Force:   w.opts.Force,
			Remote:  w.opts.IncludeRemote && branchInfo.IsRemote,
			Confirm: true, // Skip confirmation (we already confirmed)
		}

		result, err := w.cleanupService.Execute(ctx, repo, filteredReport, executeOpts)
		if err != nil {
			w.printer.PrintError(fmt.Sprintf("Failed to delete %s: %v", branchName, err))
			continue
		}

		// The report is filtered down to this one branch, so its outcome is the
		// whole result. A nil error only says the run started; Failed says whether
		// the branch is gone.
		if len(result.Failed) > 0 {
			w.printer.PrintError(fmt.Sprintf("Failed to delete %s: %v", branchName, result.Failed[0].Err))
			continue
		}

		if len(result.Deleted) == 0 {
			w.printer.PrintError(fmt.Sprintf("Skipped %s: excluded from cleanup", branchName))
			continue
		}

		w.printer.PrintSuccess(fmt.Sprintf("Deleted: %s", branchName))
		deletedCount++
	}

	return deletedCount, len(branches) - deletedCount, nil
}

func (w *BranchCleanupWizard) printSummary(result *BranchCleanupResult) {
	fmt.Println()
	w.printer.PrintDivider()
	w.printer.PrintSubtitle("Cleanup Summary")
	fmt.Println()

	w.printer.PrintKeyValue("Repositories processed", fmt.Sprintf("%d", result.ReposProcessed))
	w.printer.PrintKeyValue("Branches deleted", fmt.Sprintf("%d", result.BranchesDeleted))
	w.printer.PrintKeyValue("Branches skipped", fmt.Sprintf("%d", result.BranchesSkipped))

	if len(result.Errors) > 0 {
		fmt.Println()
		w.printer.PrintWarning(fmt.Sprintf("%d errors occurred:", len(result.Errors)))
		for _, e := range result.Errors {
			fmt.Printf("  - %s\n", e)
		}
	}
}

// formatBranchForSelection formats a branch for display in selection.
func formatBranchForSelection(b *branch.Branch, report *branch.CleanupReport) string {
	var tags []string

	// Add type tag
	for _, merged := range report.Merged {
		if merged.Name == b.Name {
			tags = append(tags, "[merged]")
			break
		}
	}
	for _, stale := range report.Stale {
		if stale.Name == b.Name {
			tags = append(tags, "[stale]")
			break
		}
	}
	for _, orphaned := range report.Orphaned {
		if orphaned.Name == b.Name {
			tags = append(tags, "[gone]")
			break
		}
	}

	// Add ahead/behind info if available
	if b.AheadBy > 0 || b.BehindBy > 0 {
		tags = append(tags, fmt.Sprintf("↑%d ↓%d", b.AheadBy, b.BehindBy))
	}

	if len(tags) > 0 {
		return fmt.Sprintf("%s %s", b.Name, strings.Join(tags, " "))
	}
	return b.Name
}

// filterBranches returns branches matching the given name.
func filterBranches(branches []*branch.Branch, name string) []*branch.Branch {
	for _, b := range branches {
		if b.Name == name {
			return []*branch.Branch{b}
		}
	}
	return nil
}

// contains checks if a slice contains a string.
func contains(slice []string, item string) bool {
	return slices.Contains(slice, item)
}
