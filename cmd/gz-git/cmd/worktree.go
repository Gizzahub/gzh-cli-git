package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/branch"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

var (
	worktreeAddCreate bool
	worktreeAddForce  bool
	worktreeRmForce   bool
	worktreeRmDryRun  bool
	worktreeListFlags BulkCommandFlags
)

// worktreeCmd is the parent command group for git worktree operations.
var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Manage git worktrees",
	Long: cliutil.QuickStartHelp(`  # List worktrees in current repo
  gz-git worktree list

  # BULK: List worktrees for all repos in a directory
  gz-git worktree list .

  # Add a worktree for an existing branch
  gz-git worktree add feature/my-work

  # Add a worktree and create the branch
  gz-git worktree add -b feature/new-work

  # Remove a worktree
  gz-git worktree remove ../my-repo-feature`),
	Example: ``,
	Args:    cobra.NoArgs,
}

// worktreeListCmd lists git worktrees for one or many repositories.
var worktreeListCmd = &cobra.Command{
	Use:   "list [directory]",
	Short: "List git worktrees",
	Long: cliutil.QuickStartHelp(`  # List worktrees in current repo
  gz-git worktree list

  # BULK: List worktrees for all repos under a directory
  gz-git worktree list .

  # JSON output
  gz-git worktree list --format json`),
	Example: ``,
	RunE:    runWorktreeList,
}

// worktreeAddCmd creates a new git worktree from an existing or new branch.
var worktreeAddCmd = &cobra.Command{
	Use:   "add <branch> [path]",
	Short: "Add a new git worktree",
	Long: cliutil.QuickStartHelp(`  # Add worktree for an existing branch (path defaults to ../<repo>-<branch>)
  gz-git worktree add feature/my-work

  # Add worktree at an explicit path
  gz-git worktree add feature/my-work /tmp/my-work

  # Create branch and add worktree
  gz-git worktree add -b feature/new-work

  # Overwrite existing path
  gz-git worktree add --force feature/my-work`),
	Example: ``,
	Args:    cobra.RangeArgs(1, 2),
	RunE:    runWorktreeAdd,
}

// worktreeRemoveCmd removes a git worktree from the current repository.
var worktreeRemoveCmd = &cobra.Command{
	Use:   "remove <path>",
	Short: "Remove a git worktree",
	Long: cliutil.QuickStartHelp(`  # Remove a worktree
  gz-git worktree remove ../my-repo-feature

  # Preview removal without touching anything
  gz-git worktree remove ../my-repo-feature --dry-run

  # Force remove (even with uncommitted changes or if locked)
  gz-git worktree remove ../my-repo-feature --force`),
	Example: ``,
	Args:    cobra.ExactArgs(1),
	RunE:    runWorktreeRemove,
}

func init() {
	rootCmd.AddCommand(worktreeCmd)
	worktreeCmd.AddCommand(worktreeListCmd)
	worktreeCmd.AddCommand(worktreeAddCmd)
	worktreeCmd.AddCommand(worktreeRemoveCmd)

	// list: bulk-compatible flags (no dry-run, no watch, no fetch for a read-only list)
	addBulkFlagsWithOpts(worktreeListCmd, &worktreeListFlags, BulkFlagOptions{
		SkipDryRun: true,
		SkipWatch:  true,
		SkipFetch:  true,
	})

	// add flags
	worktreeAddCmd.Flags().BoolVarP(&worktreeAddCreate, "create", "b", false, "create the branch if it does not already exist")
	worktreeAddCmd.Flags().BoolVar(&worktreeAddForce, "force", false, "overwrite existing worktree path")

	// remove flags
	worktreeRemoveCmd.Flags().BoolVar(&worktreeRmForce, "force", false, "remove even if worktree has uncommitted changes or is locked")
	worktreeRemoveCmd.Flags().BoolVarP(&worktreeRmDryRun, "dry-run", "n", false, "preview the removal without deleting the worktree")
}

// ─── worktree list ────────────────────────────────────────────────────────────

func runWorktreeList(cmd *cobra.Command, args []string) error {
	directory, err := validateBulkDirectory(args)
	if err != nil {
		return err
	}
	if err := validateBulkDepth(cmd, worktreeListFlags.Depth); err != nil {
		return err
	}
	return runBulkWorktreeList(cmdContext(cmd), directory)
}

func runBulkWorktreeList(ctx context.Context, directory string) error {
	client := repository.NewClient()

	if shouldShowProgress(worktreeListFlags.Format, quiet) {
		printScanningMessage(directory, worktreeListFlags.Depth, worktreeListFlags.Parallel, false)
	}

	scanResult, err := client.ScanRepositories(ctx, repository.ScanOptions{
		Directory:      directory,
		MaxDepth:       worktreeListFlags.Depth,
		IncludePattern: worktreeListFlags.Include,
		ExcludePattern: resolveScanExclude(directory, worktreeListFlags.Exclude),
	})
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	if len(scanResult.Paths) == 0 {
		if !quiet {
			fmt.Printf("No repositories found in %s\n", directory)
		}
		return nil
	}

	mgr := branch.NewWorktreeManager()
	var results []repoWorktreeResult

	for _, repoPath := range scanResult.Paths {
		repo, err := client.Open(ctx, repoPath)
		if err != nil {
			if !quiet {
				fmt.Fprintf(os.Stderr, "✗ %s: %v\n", repoPath, err)
			}
			continue
		}

		worktrees, err := mgr.List(ctx, repo)
		if err != nil {
			if !quiet {
				fmt.Fprintf(os.Stderr, "✗ %s: %v\n", repoPath, err)
			}
			continue
		}

		results = append(results, repoWorktreeResult{Path: repoPath, Worktrees: worktrees})
	}

	if worktreeListFlags.Format == "json" {
		return printWorktreesJSON(results)
	}

	for _, r := range results {
		printWorktreesTable(r.Path, r.Worktrees)
	}

	if !quiet {
		fmt.Printf("\nRepositories: %d scanned, %d processed\n", scanResult.TotalScanned, len(results))
	}

	return nil
}

// ─── worktree add ─────────────────────────────────────────────────────────────

func runWorktreeAdd(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	branchName := args[0]
	if err := gitcmd.SanitizeBranchName(branchName); err != nil {
		return fmt.Errorf("invalid branch name: %w", err)
	}

	repo, err := openCurrentRepo(ctx)
	if err != nil {
		return err
	}
	absPath := repo.Path

	worktreePath := ""
	if len(args) > 1 {
		worktreePath = args[1]
	} else {
		worktreePath = worktreeDefaultPath(absPath, branchName)
	}

	mgr := branch.NewWorktreeManager()
	opts := branch.AddOptions{
		Path:         worktreePath,
		Branch:       branchName,
		CreateBranch: worktreeAddCreate,
		Force:        worktreeAddForce,
	}

	wt, err := mgr.Add(ctx, repo, opts)
	if err != nil {
		return fmt.Errorf("failed to add worktree: %w", err)
	}

	if !quiet {
		fmt.Printf("✓ Added worktree %s on branch %s\n", wt.Path, wt.Branch)
	}

	return nil
}

// ─── worktree remove ──────────────────────────────────────────────────────────

func runWorktreeRemove(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	worktreePath := args[0]

	repo, err := openCurrentRepo(ctx)
	if err != nil {
		return err
	}

	mgr := branch.NewWorktreeManager()

	// Dry-run: confirm the path is a worktree of this repo and report what would
	// happen, without deleting anything.
	if worktreeRmDryRun {
		worktrees, err := mgr.List(ctx, repo)
		if err != nil {
			return fmt.Errorf("failed to list worktrees: %w", err)
		}

		target := worktreePath
		if abs, absErr := filepath.Abs(worktreePath); absErr == nil {
			target = filepath.Clean(abs)
		}

		matched := ""
		for _, wt := range worktrees {
			if filepath.Clean(wt.Path) == target || wt.Path == worktreePath {
				matched = wt.Path
				break
			}
		}
		if matched == "" {
			return fmt.Errorf("not a worktree of this repository: %s", worktreePath)
		}

		if !quiet {
			fmt.Printf("[DRY-RUN] Would remove worktree %s\n", matched)
		}
		return nil
	}

	opts := branch.RemoveOptions{
		Path:  worktreePath,
		Force: worktreeRmForce,
	}

	if err := mgr.Remove(ctx, repo, opts); err != nil {
		return fmt.Errorf("failed to remove worktree: %w", err)
	}

	if !quiet {
		fmt.Printf("✓ Removed worktree %s\n", worktreePath)
	}

	return nil
}

// ─── output helpers ───────────────────────────────────────────────────────────

// repoWorktreeResult groups worktrees for a single repository.
type repoWorktreeResult struct {
	Path      string
	Worktrees []*branch.Worktree
}

// printWorktreesTable prints worktrees in a human-readable table for one repo.
func printWorktreesTable(repoPath string, worktrees []*branch.Worktree) {
	if quiet {
		return
	}

	fmt.Printf("\n%s (%d worktree(s)):\n", repoPath, len(worktrees))
	fmt.Println(strings.Repeat("─", 60))

	for _, wt := range worktrees {
		var tags []string
		if wt.IsMain {
			tags = append(tags, "main")
		}
		if wt.IsLocked {
			tags = append(tags, "locked")
		}
		if wt.IsPrunable {
			tags = append(tags, "prunable")
		}
		if wt.IsDetached {
			tags = append(tags, "detached")
		}

		branchStr := wt.Branch
		if branchStr == "" {
			branchStr = wt.Ref
		}

		tagStr := ""
		if len(tags) > 0 {
			tagStr = " [" + strings.Join(tags, ", ") + "]"
		}

		fmt.Printf("  %-45s %s%s\n", wt.Path, branchStr, tagStr)
	}
}

// worktreeListJSONOutput is the JSON output structure for worktree list.
type worktreeListJSONOutput struct {
	Repositories []worktreeRepoJSON `json:"repositories"`
}

// worktreeRepoJSON represents a single repository in JSON output.
type worktreeRepoJSON struct {
	Path      string         `json:"path"`
	Worktrees []worktreeJSON `json:"worktrees"`
}

// worktreeJSON represents a single worktree entry in JSON output.
type worktreeJSON struct {
	Path       string `json:"path"`
	Branch     string `json:"branch,omitempty"`
	Ref        string `json:"ref,omitempty"`
	IsMain     bool   `json:"is_main"`
	IsLocked   bool   `json:"is_locked,omitempty"`
	IsPrunable bool   `json:"is_prunable,omitempty"`
	IsDetached bool   `json:"is_detached,omitempty"`
}

func printWorktreesJSON(results []repoWorktreeResult) error {
	output := worktreeListJSONOutput{
		Repositories: make([]worktreeRepoJSON, 0, len(results)),
	}

	for _, r := range results {
		repo := worktreeRepoJSON{
			Path:      r.Path,
			Worktrees: make([]worktreeJSON, 0, len(r.Worktrees)),
		}
		for _, wt := range r.Worktrees {
			repo.Worktrees = append(repo.Worktrees, worktreeJSON{
				Path:       wt.Path,
				Branch:     wt.Branch,
				Ref:        wt.Ref,
				IsMain:     wt.IsMain,
				IsLocked:   wt.IsLocked,
				IsPrunable: wt.IsPrunable,
				IsDetached: wt.IsDetached,
			})
		}
		output.Repositories = append(output.Repositories, repo)
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	fmt.Println(string(data))
	return nil
}

// worktreeDefaultPath computes the default path for a new worktree.
// It creates a sibling directory: ../<repo-name>-<branch-safe>.
// Slashes in the branch name are replaced with dashes to form a valid directory name.
func worktreeDefaultPath(repoPath, branchName string) string {
	repoName := filepath.Base(repoPath)
	safeBranch := strings.ReplaceAll(branchName, "/", "-")
	return filepath.Join(filepath.Dir(repoPath), repoName+"-"+safeBranch)
}
