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
	if err := resolveBulkDepth(cmd, directory, &infoFlags.Depth); err != nil {
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
		ExcludePattern:    resolveScanExclude(directory, infoFlags.Exclude),
		Logger:            logger,
	}

	// Execute scan
	result, err := client.BulkStatus(ctx, bulkOpts)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
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

	if infoFlags.Format == "json" || infoFlags.Format == "llm" {
		displayInfoResultsStructured(result, enrichment, infoFlags.Format)
		return nil
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

	// The owner most of the workspace shares, so each detail block can flag
	// the repository that points somewhere else — the stray-fork signal the
	// table's REMOTE column no longer carries now that it is presence-only.
	majorityOwner, _ := majorityRemoteOwner(result.Repositories)

	for _, repo := range result.Repositories {
		fmt.Println()
		displayInfoRepository(repo, enrichment[repo.Path], majorityOwner)
	}
	fmt.Println()
}

func displayInfoRepository(repo repository.RepositoryStatusResult, enr infoEnrichment, majorityOwner string) {
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
	if note := ownerNote(repo.RemoteURL, majorityOwner); note != "" {
		fmt.Print(note)
	}
	displayInfoRemotes(repo.Remotes)
	displayInfoBranches(repo.LocalBranches)
	displayInfoRemoteOnlyBranches(remoteOnlyTrackingBranches(&repo, enr))
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

func displayInfoRemoteOnlyBranches(branches []string) {
	if len(branches) == 0 {
		return
	}
	shown := branches
	if len(branches) > itemLimit {
		shown = append([]string(nil), branches[:itemLimit]...)
		shown = append(shown, fmt.Sprintf("... (%d more)", len(branches)-itemLimit))
	}
	fmt.Printf("  Remote-only (%d): %s\n", len(branches), strings.Join(shown, ", "))
}

// InfoJSONOutput is the structured contract for `gz-git info`. Unlike status,
// it includes remote_only_branches: complete remote-tracking refs with no
// corresponding local, current, base, or upstream branch.
type InfoJSONOutput struct {
	TotalScanned   int                        `json:"total_scanned"`
	TotalProcessed int                        `json:"total_processed"`
	DurationMs     int64                      `json:"duration_ms"`
	Summary        map[string]int             `json:"summary"`
	Repositories   []InfoRepositoryJSONOutput `json:"repositories"`
}

type InfoRepositoryJSONOutput struct {
	Path               string   `json:"path"`
	Branch             string   `json:"branch,omitempty"`
	Status             string   `json:"status"`
	UncommittedFiles   int      `json:"uncommitted_files,omitempty"`
	UntrackedFiles     int      `json:"untracked_files,omitempty"`
	CommitsAhead       int      `json:"commits_ahead,omitempty"`
	CommitsBehind      int      `json:"commits_behind,omitempty"`
	ConflictFiles      []string `json:"conflict_files,omitempty"`
	RemoteOnlyBranches []string `json:"remote_only_branches"`
	DurationMs         int64    `json:"duration_ms,omitempty"`
	Error              string   `json:"error,omitempty"`
}

func displayInfoResultsStructured(result *repository.BulkStatusResult, enrichment map[string]infoEnrichment, format string) {
	output := InfoJSONOutput{
		TotalScanned:   result.TotalScanned,
		TotalProcessed: result.TotalProcessed,
		DurationMs:     result.Duration.Milliseconds(),
		Summary:        result.Summary,
		Repositories:   make([]InfoRepositoryJSONOutput, 0, len(result.Repositories)),
	}
	for i := range result.Repositories {
		repo := &result.Repositories[i]
		repoOutput := InfoRepositoryJSONOutput{
			Path:               repo.RelativePath,
			Branch:             repo.Branch,
			Status:             repo.Status,
			UncommittedFiles:   repo.TrackedChangedFiles,
			UntrackedFiles:     repo.UntrackedFiles,
			CommitsAhead:       repo.CommitsAhead,
			CommitsBehind:      repo.CommitsBehind,
			ConflictFiles:      repo.ConflictFiles,
			RemoteOnlyBranches: remoteOnlyTrackingBranches(repo, enrichment[repo.Path]),
			DurationMs:         repo.Duration.Milliseconds(),
		}
		if repo.Error != nil {
			repoOutput.Error = repo.Error.Error()
		}
		output.Repositories = append(output.Repositories, repoOutput)
	}
	writeBulkOutput(format, output)
}

// The functions below read the remote owner, which only --full reports now:
// the table's REMOTE column is presence-only, so "which owner" moved here.

// ownerNote flags a repository whose remote points at a different owner than
// the workspace majority. Repositories on the majority print nothing — the
// URL list that follows already carries the full detail.
func ownerNote(remoteURL, majorityOwner string) string {
	owner := remoteOwner(remoteURL)
	if owner == "" || sameOwner(owner, majorityOwner) {
		return ""
	}
	return fmt.Sprintf("  Owner:          %s (differs from workspace majority %s)\n", owner, majorityOwner)
}

// majorityRemoteOwner returns the most common remote owner across the scan and
// how many repositories use it. Owners are grouped case-insensitively because
// forge hosts treat "Gizzahub" and "gizzahub" as the same account, so counting
// them separately would invent a discrepancy that does not exist. The returned
// spelling is the one that occurs most often, ties broken alphabetically for a
// stable result across runs.
func majorityRemoteOwner(repos []repository.RepositoryStatusResult) (owner string, count int) {
	counts := make(map[string]int)
	spellings := make(map[string]map[string]int)

	for i := range repos {
		o := remoteOwner(repos[i].RemoteURL)
		if o == "" {
			continue
		}
		key := strings.ToLower(o)
		counts[key]++
		if spellings[key] == nil {
			spellings[key] = make(map[string]int)
		}
		spellings[key][o]++
	}

	bestKey := ""
	for key, n := range counts {
		if n > counts[bestKey] || (n == counts[bestKey] && key < bestKey) {
			bestKey = key
		}
	}
	if bestKey == "" {
		return "", 0
	}

	best := ""
	for spelling, n := range spellings[bestKey] {
		if n > spellings[bestKey][best] || (n == spellings[bestKey][best] && spelling < best) {
			best = spelling
		}
	}
	return best, counts[bestKey]
}

// sameOwner compares owners the way forge hosts do.
func sameOwner(a, b string) bool {
	return a != "" && strings.EqualFold(a, b)
}

// remoteOwner reduces a remote URL to "host/owner", covering both the scp-like
// form (git@host:owner/repo.git) and the URL form (https://host/owner/repo).
// It returns "" when the URL is empty or does not carry an owner segment.
func remoteOwner(remoteURL string) string {
	url := strings.TrimSuffix(strings.TrimSpace(remoteURL), ".git")
	if url == "" {
		return ""
	}

	// scp-like: strip the user@ prefix and turn the single ":" into "/".
	if !strings.Contains(url, "://") {
		if at := strings.Index(url, "@"); at >= 0 {
			url = url[at+1:]
		}
		url = strings.Replace(url, ":", "/", 1)
	} else {
		url = url[strings.Index(url, "://")+3:]
		if at := strings.Index(url, "@"); at >= 0 {
			url = url[at+1:]
		}
	}

	parts := strings.Split(strings.Trim(url, "/"), "/")
	if len(parts) < 3 {
		return ""
	}
	return parts[0] + "/" + parts[1]
}
