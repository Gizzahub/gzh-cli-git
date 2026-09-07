// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package branch

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// CleanupService analyzes and cleans up branches.
type CleanupService interface {
	// Analyze analyzes branches for cleanup.
	Analyze(ctx context.Context, repo *repository.Repository, opts AnalyzeOptions) (*CleanupReport, error)

	// Execute performs cleanup based on report. The result names which branches
	// were deleted and which were not; a non-nil error means the run never
	// started, not that nothing was deleted.
	Execute(ctx context.Context, repo *repository.Repository, report *CleanupReport, opts ExecuteOptions) (*ExecuteResult, error)
}

// cleanupService implements CleanupService.
type cleanupService struct {
	executor      *gitcmd.Executor
	branchManager BranchManager
}

// NewCleanupService creates a new CleanupService.
func NewCleanupService() CleanupService {
	return &cleanupService{
		executor:      gitcmd.NewExecutor(),
		branchManager: NewManager(),
	}
}

// NewCleanupServiceWithDeps creates a new CleanupService with custom dependencies.
func NewCleanupServiceWithDeps(executor *gitcmd.Executor, branchManager BranchManager) CleanupService {
	return &cleanupService{
		executor:      executor,
		branchManager: branchManager,
	}
}

// run executes a git command and turns a non-zero exit into an error.
//
// Every git read in this file feeds a decision about deleting a branch, and each
// one answers "no evidence" the same way it answers "the read failed" — so a
// failure that stays silent becomes a verdict.
func (c *cleanupService) run(ctx context.Context, dir string, args ...string) (*gitcmd.Result, error) {
	return runGit(ctx, c.executor, dir, args...)
}

// Analyze analyzes branches for cleanup.
func (c *cleanupService) Analyze(ctx context.Context, repo *repository.Repository, opts AnalyzeOptions) (*CleanupReport, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository cannot be nil")
	}

	// Set defaults
	if opts.StaleThreshold == 0 {
		opts.StaleThreshold = 30 * 24 * time.Hour // 30 days
	}

	if opts.BaseBranch == "" {
		// Try to detect base branch
		baseBranch, err := c.detectBaseBranch(ctx, repo)
		if err == nil {
			opts.BaseBranch = baseBranch
		} else {
			opts.BaseBranch = "main" // Default fallback
		}
	}

	// A retirement can be authorized entirely by the cached remote-tracking
	// copy of the canonical branch, so that cache is refreshed before anything
	// is measured against it. Refreshing is what distinguishes a canonical
	// branch that merely advanced (the ancestry still holds) from one that was
	// rewound (it does not) — ls-remote alone cannot tell those apart, because
	// it returns an id without the object needed to test containment. Best
	// effort: offline, the tip check in Execute refuses rather than guesses.
	c.refreshCanonicalTracking(ctx, repo, opts)

	// Get all branches
	branches, err := c.branchManager.List(ctx, repo, ListOptions{
		All: opts.IncludeRemote,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}

	report := &CleanupReport{
		Merged:       make([]*Branch, 0),
		Stale:        make([]*Branch, 0),
		Orphaned:     make([]*Branch, 0),
		Superseded:   make([]*Branch, 0),
		NonCanonical: make([]*Branch, 0),
		Protected:    make([]*Branch, 0),
		Total:        len(branches),
	}

	// Ask git which branches it marks [gone] before the loop: the answer comes
	// from one for-each-ref over all of refs/heads, not from a per-branch query.
	gone := map[string]bool{}

	if opts.IncludeGone {
		gone, err = c.findGoneBranches(ctx, repo)
		if err != nil {
			return nil, fmt.Errorf("failed to find gone branches: %w", err)
		}
	}

	superseded := map[string]bool{}
	if opts.IncludeSuperseded {
		_, names, _, classErr := repository.NewClient().BotRemoteBranches(ctx, repo, opts.BaseBranch)
		if classErr != nil {
			return nil, fmt.Errorf("failed to classify superseded bot remotes: %w", classErr)
		}
		for _, name := range names {
			superseded[name] = true
		}
	}

	for _, branch := range branches {
		c.classifyCleanupBranch(ctx, repo, branch, opts, gone, superseded, report)
	}

	return report, nil
}

func (c *cleanupService) classifyCleanupBranch(
	ctx context.Context,
	repo *repository.Repository,
	branch *Branch,
	opts AnalyzeOptions,
	gone, superseded map[string]bool,
	report *CleanupReport,
) {
	if !normalizeCleanupBranch(branch) || branch.IsHead {
		return
	}
	if branch.IsRemote {
		c.captureRemoteBranchSHA(ctx, repo, branch)
	}
	if opts.IncludeNonCanonical && c.isNonCanonical(ctx, repo, branch, opts) {
		report.NonCanonical = append(report.NonCanonical, branch)
		return
	}
	if c.isProtectedBranch(branch.Name, opts.Exclude) {
		report.Protected = append(report.Protected, branch)
		return
	}
	if opts.BotsOnly && !repository.IsBotBranch(branch.Name) {
		return
	}
	if opts.IncludeMerged {
		if merged, err := c.isBranchMerged(ctx, repo, branch, opts.BaseBranch); err == nil && merged {
			report.Merged = append(report.Merged, branch)
			return
		}
	}
	if opts.IncludeSuperseded && branch.IsRemote && superseded[branch.Name] {
		report.Superseded = append(report.Superseded, branch)
		return
	}
	// Stale stays local-only: an unmerged remote bot branch may be an open PR.
	if opts.IncludeStale && !branch.IsRemote {
		if stale, err := c.isBranchStale(ctx, repo, branch.Name, opts.StaleThreshold); err == nil && stale {
			report.Stale = append(report.Stale, branch)
			return
		}
	}
	if !branch.IsRemote && gone[branch.Name] {
		report.Orphaned = append(report.Orphaned, branch)
	}
}

// Execute performs cleanup based on report.
//
// A failure on one branch does not stop the others — that policy is deliberate
// and unchanged. What it returns is the account of which ones succeeded, because
// without it the caller has no way to tell a run that deleted everything from one
// that deleted nothing.
//
// Analyze already routes protected branches into report.Protected, but Execute
// must not rely on that: CleanupReport is a public type and callers may assemble
// one by hand. Built-in IsProtected (main/master/…) and opts.Exclude are always
// applied here, including when Exclude is empty and when DryRun is true.
func (c *cleanupService) Execute(ctx context.Context, repo *repository.Repository, report *CleanupReport, opts ExecuteOptions) (*ExecuteResult, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository cannot be nil")
	}

	if report == nil {
		return nil, fmt.Errorf("cleanup report cannot be nil")
	}

	// Collect all branches to delete
	toDelete := make([]*Branch, 0)
	toDelete = append(toDelete, report.Merged...)
	toDelete = append(toDelete, report.Stale...)
	toDelete = append(toDelete, report.Orphaned...)
	toDelete = append(toDelete, report.Superseded...)

	result := &ExecuteResult{}

	// Always screen protected branches (built-in + Exclude patterns). Do not gate
	// this on len(opts.Exclude): isProtectedBranch still enforces IsProtected when
	// Exclude is empty.
	filtered := make([]*Branch, 0, len(toDelete))
	for _, branch := range toDelete {
		if c.isProtectedBranch(branch.Name, opts.Exclude) {
			result.Skipped = append(result.Skipped, branch.Name)
			continue
		}
		filtered = append(filtered, branch)
	}
	toDelete = filtered

	// report.NonCanonical is the one bucket allowed past built-in name
	// protection, so it is screened on its own terms rather than added to
	// toDelete above. Analyze already applied these conditions; they are applied
	// again because CleanupReport is public and a hand-assembled report must not
	// be able to turn this into "delete master". Only --protect patterns and a
	// re-verified ancestry check stand between a candidate and deletion, and both
	// are checked here, in this process, against this repository.
	for _, branch := range report.NonCanonical {
		if !c.authorizeRetire(ctx, repo, branch, opts) {
			result.Skipped = append(result.Skipped, branch.Name)
			continue
		}
		// The ancestry above may have been measured against a remote-tracking
		// ref, which is a cache from the last fetch. Where it was, that cache is
		// the whole of the evidence, and --force-with-lease does not check it:
		// the lease covers the branch being deleted, never the branch the
		// ancestry was measured against. A canonical branch rewound since the
		// fetch makes the deletion authorized by something no longer true, and
		// the commits it drops exist nowhere else.
		//
		// A remote candidate is always measured against the cache. A local one
		// is too whenever this clone has no local copy of the canonical branch —
		// the ordinary shape of a fresh clone, which is exactly the repository
		// this command exists to clean.
		if c.retirementRestsOnCache(ctx, repo, branch, opts) {
			if err := c.requireCurrentCanonicalTip(ctx, repo, branch, opts); err != nil {
				result.Failed = append(result.Failed, DeleteFailure{Branch: branch.Name, Err: err})
				continue
			}
		}
		toDelete = append(toDelete, branch)
	}

	// Dry run: do not delete. Skipped already lists protected branches; non-protected
	// candidates remain only in the input report (nothing was removed).
	if opts.DryRun {
		return result, nil
	}

	// Remote-only names are deleted with a leased push of :refs/heads/<name>.
	// BranchManager.Delete Exists only on refs/heads/, so a remotes/origin/…
	// candidate would fail with not found. Only names Analyze already classified
	// as remote-merged are deleted here — pairing a local delete with a
	// same-named unmerged remote would drop an open PR.
	for _, branch := range toDelete {
		if branch.IsRemote {
			if err := c.deleteRemoteBranch(ctx, repo, branch); err != nil {
				result.Failed = append(result.Failed, DeleteFailure{Branch: branch.Name, Err: err})
				continue
			}
			result.Deleted = append(result.Deleted, branch.Name)
			continue
		}

		deleteOpts := DeleteOptions{
			Name:    branch.Name,
			Force:   opts.Force,
			Remote:  false,
			Confirm: opts.Confirm,
		}

		// Carry the failure rather than dropping it. Continuing past a branch that
		// could not be deleted is the right policy; doing so silently meant the
		// caller counted the report and announced that many deletions.
		if err := c.branchManager.Delete(ctx, repo, deleteOpts); err != nil {
			result.Failed = append(result.Failed, DeleteFailure{Branch: branch.Name, Err: err})
			continue
		}

		result.Deleted = append(result.Deleted, branch.Name)
	}

	return result, nil
}

// detectBaseBranch detects the main/master branch.
func (c *cleanupService) detectBaseBranch(ctx context.Context, repo *repository.Repository) (string, error) {
	// Try common base branches in order
	candidates := []string{"main", "master", "develop", "development"}

	for _, branch := range candidates {
		exists, err := c.branchManager.Exists(ctx, repo, branch)
		if err == nil && exists {
			return branch, nil
		}
	}

	return "", fmt.Errorf("could not detect base branch")
}

// isBranchMerged checks if a branch is fully merged into base.
// Local names use `git branch --merged`. Remote-only names are invisible
// to that listing, so they are checked with merge-base --is-ancestor
// against the remote-tracking ref.
func (c *cleanupService) isBranchMerged(ctx context.Context, repo *repository.Repository, branch *Branch, base string) (bool, error) {
	if branch.IsRemote {
		tip := branch.Ref
		if tip == "" {
			tip = "refs/remotes/origin/" + branch.Name
		}
		result, err := c.executor.Run(ctx, repo.Path, "merge-base", "--is-ancestor", tip, base)
		if err != nil {
			return false, err
		}
		return result.ExitCode == 0, nil
	}

	result, err := c.run(ctx, repo.Path, "branch", "--merged", base)
	if err != nil {
		return false, err
	}

	lines := strings.SplitSeq(strings.TrimSpace(result.Stdout), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "* ")
		if line == branch.Name {
			return true, nil
		}
	}

	return false, nil
}

// captureRemoteBranchSHA stores the full object name of the remote-tracking
// ref. git branch -vv may leave an abbreviated SHA, which is not a reliable
// lease expect value.
func (c *cleanupService) captureRemoteBranchSHA(ctx context.Context, repo *repository.Repository, branch *Branch) {
	tip := branch.Ref
	if tip == "" {
		remote, name := remoteAndBranch(branch)
		if name == "" {
			branch.SHA = ""
			return
		}
		if remote == "" {
			remote = "origin"
		}
		tip = "refs/remotes/" + remote + "/" + name
	}
	result, err := c.executor.Run(ctx, repo.Path, "rev-parse", "--verify", tip)
	if err != nil || result == nil || result.ExitCode != 0 {
		branch.SHA = ""
		return
	}
	branch.SHA = strings.TrimSpace(result.Stdout)
}

// deleteRemoteBranch leases the classified SHA and pushes :refs/heads/<name>.
// An empty SHA refuses the delete rather than falling back to unleased
// --delete. Name must already be stripped of the origin/ prefix.
func (c *cleanupService) deleteRemoteBranch(ctx context.Context, repo *repository.Repository, branch *Branch) error {
	remote, name := remoteAndBranch(branch)
	if name == "" {
		return fmt.Errorf("empty remote branch name")
	}
	if remote == "" {
		remote = "origin"
	}
	if branch.SHA == "" {
		return fmt.Errorf("remote delete needs the classified commit for a lease")
	}
	ref := "refs/heads/" + name
	lease := "--force-with-lease=" + ref + ":" + branch.SHA
	result, err := c.executor.RunWithEnv(ctx, repo.Path, repository.NonInteractiveEnv(), "push", lease, remote, ":"+ref)
	if err == nil && result != nil && result.ExitCode == 0 {
		return nil
	}
	heads, lsErr := c.executor.RunWithEnv(ctx, repo.Path, repository.NonInteractiveEnv(), "ls-remote", "--heads", remote, name)
	if lsErr != nil || heads == nil || heads.ExitCode != 0 || strings.TrimSpace(heads.Stdout) != "" {
		if err != nil {
			return err
		}
		detail := ""
		if result != nil {
			detail = strings.TrimSpace(result.Stderr)
		}
		if repository.IsRemoteHeadRefusal(detail) {
			return fmt.Errorf(
				"leased remote delete %s/%s: refused because it is still %s's default branch"+
					" — repoint the default branch first, then re-run",
				remote, name, remote,
			)
		}
		return fmt.Errorf("leased remote delete %s/%s: %s", remote, name, detail)
	}
	return nil
}

// normalizeCleanupBranch strips a remotes/<remote>/ prefix from Name so
// delete argv is the branch name. Returns false for origin/HEAD and other
// unusable refs.
func normalizeCleanupBranch(branch *Branch) bool {
	if branch == nil || branch.Name == "" {
		return false
	}
	if !branch.IsRemote {
		return branch.Name != "HEAD"
	}
	remote, name := splitRemoteTrackingName(branch.Name)
	if name == "" || name == "HEAD" {
		return false
	}
	if branch.Ref == "" {
		if remote == "" {
			remote = "origin"
		}
		branch.Ref = "refs/remotes/" + remote + "/" + name
	}
	branch.Name = name
	return true
}

func splitRemoteTrackingName(name string) (remote, branch string) {
	name = strings.TrimPrefix(name, "remotes/")
	name = strings.TrimPrefix(name, "refs/remotes/")
	i := strings.IndexByte(name, '/')
	if i <= 0 {
		return "", name
	}
	return name[:i], name[i+1:]
}

func remoteAndBranch(branch *Branch) (remote, name string) {
	if branch == nil {
		return "", ""
	}
	if strings.HasPrefix(branch.Ref, "refs/remotes/") {
		return splitRemoteTrackingName(strings.TrimPrefix(branch.Ref, "refs/remotes/"))
	}
	// Name is already stripped of origin/ by normalizeCleanupBranch.
	if branch.IsRemote {
		return "origin", branch.Name
	}
	return "", branch.Name
}

// isBranchStale checks if a branch has no recent activity.
func (c *cleanupService) isBranchStale(ctx context.Context, repo *repository.Repository, branch string, threshold time.Duration) (bool, error) {
	// Get last commit date
	result, err := c.run(ctx, repo.Path, "log", "-1", "--format=%ct", branch)
	if err != nil {
		return false, err
	}

	// Parse timestamp
	var timestamp int64
	if _, err := fmt.Sscanf(strings.TrimSpace(result.Stdout), "%d", &timestamp); err != nil {
		return false, err
	}

	// Check if older than threshold
	lastCommit := time.Unix(timestamp, 0)
	age := time.Since(lastCommit)

	return age > threshold, nil
}

// findGoneBranches returns the set of local branches whose upstream is gone.
//
// git answers this directly through %(upstream:track), which reads "[gone]" when
// a branch's configured upstream no longer resolves. The marker only appears once
// the stale remote-tracking ref has been pruned, so the prune runs first; it is
// best-effort because a repository with no reachable remote still has a truthful
// answer for every branch whose upstream was already pruned.
//
// This mirrors the bulk path (pkg/repository, getGoneBranches), which is the
// implementation `gz-git cleanup branch <dir> --gone` has always used.
func (c *cleanupService) findGoneBranches(ctx context.Context, repo *repository.Repository) (map[string]bool, error) {
	_, _ = c.executor.RunWithEnv(ctx, repo.Path, repository.NonInteractiveEnv(), "fetch", "--prune") //nolint:errcheck // best-effort; the for-each-ref below is the read that decides

	result, err := c.run(ctx, repo.Path,
		"for-each-ref", "--format=%(refname:short) %(upstream:track)", "refs/heads/")
	if err != nil {
		return nil, err
	}

	gone := make(map[string]bool)

	for line := range strings.SplitSeq(strings.TrimSpace(result.Stdout), "\n") {
		if !strings.Contains(line, "[gone]") {
			continue
		}

		if name, _, ok := strings.Cut(line, " "); ok && name != "" {
			gone[name] = true
		}
	}

	return gone, nil
}

// isAncestorOf reports whether ref is fully contained in target.
//
// It fails closed: a git error, a missing ref, or an unreadable repository all
// yield false, because the caller uses this answer to authorize a deletion that
// bypasses built-in protection.
func (c *cleanupService) isAncestorOf(ctx context.Context, repo *repository.Repository, ref, target string) bool {
	if ref == "" || target == "" {
		return false
	}
	result, err := c.run(ctx, repo.Path, "merge-base", "--is-ancestor", ref, target)
	if err != nil || result == nil {
		return false
	}
	return result.ExitCode == 0
}

// isProtectedBranch checks if a branch is protected.
func (c *cleanupService) isProtectedBranch(branch string, additionalPatterns []string) bool {
	// Check built-in protected branches
	if IsProtected(branch) {
		return true
	}

	// Check additional patterns
	for _, pattern := range additionalPatterns {
		if matchPattern(branch, pattern) {
			return true
		}
	}

	return false
}

// CountBranches returns the total number of branches in the report.
func (r *CleanupReport) CountBranches() int {
	return len(r.Merged) + len(r.Stale) + len(r.Orphaned) + len(r.Superseded) + len(r.NonCanonical)
}

// IsEmpty checks if the report has no branches to clean up.
func (r *CleanupReport) IsEmpty() bool {
	return r.CountBranches() == 0
}

// GetAllBranches returns all branches eligible for cleanup.
func (r *CleanupReport) GetAllBranches() []*Branch {
	all := make([]*Branch, 0, len(r.Merged)+len(r.Stale)+len(r.Orphaned)+len(r.Superseded)+len(r.NonCanonical))
	all = append(all, r.Merged...)
	all = append(all, r.Stale...)
	all = append(all, r.Orphaned...)
	all = append(all, r.Superseded...)
	all = append(all, r.NonCanonical...)
	return all
}
