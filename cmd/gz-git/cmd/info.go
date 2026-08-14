package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/branch"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

var (
	infoFlags   BulkCommandFlags
	itemLimit   int
	infoFull    bool
	infoAudit   bool
	infoCompact bool
)

// infoCmd represents the info command.
var infoCmd = &cobra.Command{
	Use:   "info [directory]",
	Short: "Display repository information",
	Long: `Display information about Git repositories in the specified directory.

Scans for repositories and shows metadata such as current branch, remote URL,
and status summary.

By default:
  - Scans 1 directory level deep
  - Processes 10 repositories in parallel
  - Shows result in a table format`,
	Args: cobra.MaximumNArgs(1),
	Example: `  # Show info for current directory
  gz-git info

  # Show info for specific repository
  gz-git info /path/to/repo

  # Verbose output with more details
  gz-git info --verbose

  # Shorter lines: hide columns that have nothing to report anywhere
  gz-git info --compact

  # Machine-readable branch audit for an agent to act on
  gz-git info --audit
  gz-git info --audit | jq '.repositories[] | select(.audit_complete | not)'`,
	RunE: runInfo,
}

func init() {
	rootCmd.AddCommand(infoCmd)

	// Common bulk operation flags
	// Add bulk flags
	addBulkFlags(infoCmd, &infoFlags)

	// Add info-specific flags
	infoCmd.Flags().IntVar(&itemLimit, "limit", 10, "max items to show in lists (branches, remotes)")
	infoCmd.Flags().BoolVar(&infoFull, "full", false, "show the per-repository detail block instead of the one-line table")
	infoCmd.Flags().BoolVar(&infoCompact, "compact", false,
		"drop table columns that have nothing to report for any repository")
	infoCmd.Flags().BoolVar(&infoAudit, "audit", false,
		"emit a machine-readable branch audit (JSON) with typed findings and remediations")
}

func runInfo(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Load config with profile support
	var (
		baseCandidates   []string
		autofixOverrides map[string]bool
	)
	effective, _ := LoadEffectiveConfig(cmd, nil)
	if effective != nil {
		autofixOverrides = effective.Audit.Autofix
		if !cmd.Flags().Changed("parallel") && effective.Parallel > 0 {
			infoFlags.Parallel = effective.Parallel
		}
		// The configured integration branches decide what "behind the base"
		// means. When config declares none, ResolveBase falls back to its own
		// heuristic rather than this command inventing an order.
		baseCandidates = effective.Branch.DefaultBranch
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
	if err := validateBulkDepth(cmd, infoFlags.Depth); err != nil {
		return err
	}

	// Create client
	client := repository.NewClient()

	// Create logger for verbose mode
	logger := createBulkLogger(verbose)

	// The progress line goes to stdout, which in audit mode carries the JSON
	// document; a caller piping it into a parser must not receive prose.
	if !infoAudit && shouldShowProgress(infoFlags.Format, quiet) {
		printScanningMessage(directory, infoFlags.Depth, infoFlags.Parallel, false)
	}

	// Setup bulk scan options
	bulkOpts := repository.BulkStatusOptions{
		Directory:         directory,
		Parallel:          infoFlags.Parallel,
		MaxDepth:          infoFlags.Depth,
		Verbose:           verbose,
		IncludeSubmodules: infoFlags.IncludeSubmodules,
		IncludePattern:    infoFlags.Include,
		ExcludePattern:    infoFlags.Exclude,
		Logger:            logger,
	}

	// Execute scan
	result, err := client.BulkStatus(ctx, bulkOpts)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	// Display results. --audit is checked before --format because it defines its
	// own document (schema, findings, remediations); reusing the status JSON
	// would emit the shape a caller did not ask for.
	if !infoAudit && (infoFlags.Format == "json" || infoFlags.Format == "llm") {
		displayInfoResultsStructured(result, infoFlags.Format)
		return nil
	}

	// Base-branch and worktree facts are info-specific and cost extra git
	// invocations, so they are gathered here rather than in BulkStatus, which
	// every other bulk command shares.
	enrichment := enrichInfoResults(
		ctx, client, branch.NewWorktreeManager(),
		result.Repositories, baseCandidates, infoFlags.Parallel, infoAudit,
	)

	if infoAudit {
		return runInfoAudit(cmd.OutOrStdout(), result, enrichment, directory, autofixOverrides, time.Now())
	}

	if infoFull {
		displayInfoResultsDetailed(result, enrichment)
		return nil
	}

	renderInfoTable(cmd.OutOrStdout(), result, enrichment, verbose, infoCompact)

	return nil
}

func displayInfoResultsDetailed(result *repository.BulkStatusResult, enrichment map[string]infoEnrichment) {
	if len(result.Repositories) == 0 {
		fmt.Println("No repositories found.")
		return
	}

	fmt.Println()
	fmt.Printf("found %d repositories (scanned in %s)\n", len(result.Repositories), result.Duration.Round(10*time.Millisecond))

	for _, repo := range result.Repositories {
		fmt.Println()
		displayInfoRepository(repo, enrichment[repo.Path])
	}
	fmt.Println()
}

func displayInfoRepository(repo repository.RepositoryStatusResult, enr infoEnrichment) {
	path := filepath.Base(repo.Path)
	if verbose {
		path = repo.RelativePath
	}
	fmt.Printf("📦 %s\n", path)
	fmt.Println(strings.Repeat("-", 60))

	branchInfo := repo.Branch
	if branchInfo == "" {
		branchInfo = "DETACHED"
	}
	if repo.HeadSHA != "" {
		branchInfo += fmt.Sprintf(" (%s)", repo.HeadSHA)
	}
	fmt.Printf("  Current Branch: %s\n", branchInfo)
	displayInfoUpstream(repo)
	displayInfoBase(enr)
	if enr.LinkedWorktrees > 0 {
		detail := ""
		if len(enr.WorktreeBranches) > 0 {
			detail = ": " + strings.Join(enr.WorktreeBranches, ", ")
		}
		fmt.Printf("  Worktrees:      %d linked%s\n", enr.LinkedWorktrees, detail)
	}
	if enr.Err != nil {
		fmt.Printf("  Note:           partial info (%v)\n", enr.Err)
	}
	displayInfoVersion(repo)
	displayInfoStatus(repo)
	displayInfoLastUpdate(repo)
	displayInfoRemotes(repo.Remotes)
	displayInfoBranches(repo.LocalBranches)
}

func displayInfoUpstream(repo repository.RepositoryStatusResult) {
	switch {
	case repo.Upstream == "":
		fmt.Printf("  Upstream:       (none)\n")
	case repo.CommitsAhead > 0 || repo.CommitsBehind > 0:
		fmt.Printf("  Upstream:       %s (ahead %d, behind %d)\n", repo.Upstream, repo.CommitsAhead, repo.CommitsBehind)
	default:
		fmt.Printf("  Upstream:       %s (in sync)\n", repo.Upstream)
	}
}

func displayInfoBase(enr infoEnrichment) {
	if enr.Base.Name == "" {
		fmt.Printf("  Base:           (none found)\n")
		return
	}
	fmt.Printf("  Base:           %s (ahead %d, behind %d) [%s]\n",
		enr.Base.Name, enr.Base.Ahead, enr.Base.Behind, enr.Base.Source)
}

func displayInfoVersion(repo repository.RepositoryStatusResult) {
	if repo.Describe != "" {
		fmt.Printf("  Version:        %s\n", repo.Describe)
	}
}

func displayInfoStatus(repo repository.RepositoryStatusResult) {
	status := repo.Status
	if repo.Status != "clean" && repo.TrackedChangedFiles > 0 {
		status = fmt.Sprintf("%s (%d uncommitted)", repo.Status, repo.TrackedChangedFiles)
	}
	if repo.StashCount > 0 {
		status += fmt.Sprintf(", %d stash(es)", repo.StashCount)
	}
	fmt.Printf("  Status:         %s\n", status)
}

func displayInfoLastUpdate(repo repository.RepositoryStatusResult) {
	if repo.LastCommitDate == "" {
		return
	}
	msg := repo.LastCommitMsg
	if len(msg) > 50 {
		msg = msg[:47] + "..."
	}
	fmt.Printf("  Last Update:    %s (%s)\n", repo.LastCommitDate, msg)
	if verbose {
		fmt.Printf("  Author:         %s\n", repo.LastCommitAuthor)
	}
}

func displayInfoRemotes(remotes map[string]string) {
	if len(remotes) == 0 {
		fmt.Println("  Remotes:        (none)")
		return
	}
	fmt.Println("  Remotes:")
	keys := make([]string, 0, len(remotes))
	for key := range remotes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for index, key := range keys {
		if index >= itemLimit {
			fmt.Printf("    ... (%d more)\n", len(keys)-index)
			break
		}
		fmt.Printf("    - %-10s %s\n", key, remotes[key])
	}
}

func displayInfoBranches(branches []string) {
	if len(branches) == 0 {
		return
	}
	sort.Strings(branches)
	branchesStr := strings.Join(branches, ", ")
	if len(branches) > itemLimit {
		branchesStr = strings.Join(branches[:itemLimit], ", ") + fmt.Sprintf(", ... (%d more)", len(branches)-itemLimit)
	}
	fmt.Printf("  Branches (%d):   %s\n", len(branches), branchesStr)
}

func displayInfoResultsStructured(result *repository.BulkStatusResult, format string) {
	// Re-use status JSON output format as it contains all info
	displayStatusResultsStructured(result, format)
}
