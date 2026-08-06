// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// Thresholds for repository health checks.
const (
	// DivergeBehindWarn: warn when local branch is this many commits behind upstream.
	DivergeBehindWarn = 10

	// DivergeAheadWarn: warn when local branch is this many commits ahead of upstream.
	DivergeAheadWarn = 20

	// BranchDistanceWarn: warn when develop is this many commits away from main/master.
	BranchDistanceWarn = 50

	// BranchDistanceError: error when develop is this many commits away from main/master.
	BranchDistanceError = 150

	// FeatureBranchDistanceWarn: warn when a feature branch is this far from its base.
	FeatureBranchDistanceWarn = 30

	// FeatureBranchDistanceError: error when a feature branch exceeds this distance.
	FeatureBranchDistanceError = 100

	// StaleFeatureBranchDays: feature branches older than this are stale.
	StaleFeatureBranchDays = 30
)

// Feature branch prefixes to check for divergence.
var featureBranchPrefixes = []string{"feat/", "feature/", "fix/", "hotfix/", "bugfix/"}

// checkRepositories runs all repository-level checks for repos found in the scan directory.
func checkRepositories(ctx context.Context, opts Options) []CheckResult {
	directory := opts.Directory
	if directory == "" {
		var err error
		directory, err = os.Getwd()
		if err != nil {
			return nil
		}
	}

	repos := scanGitRepos(directory, opts.ScanDepth)
	if len(repos) == 0 {
		return []CheckResult{{
			Name:     "repos",
			Category: CategoryRepo,
			Status:   StatusOK,
			Message:  "no git repositories found in scan directory",
		}}
	}

	executor := gitcmd.NewExecutor(gitcmd.WithTimeout(10 * time.Second))
	var results []CheckResult

	for _, repoPath := range repos {
		name := filepath.Base(repoPath)
		results = append(results, checkSingleRepo(ctx, executor, repoPath, name, opts.Verbose)...)
	}

	return results
}

// scanGitRepos walks the directory tree to find .git directories.
// maxDepth 0 means check only the root directory itself.
// maxDepth 1 means root + immediate subdirectories (default).
func scanGitRepos(root string, maxDepth int) []string {
	if maxDepth < 0 {
		maxDepth = 1
	}

	var repos []string
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil
	}

	walkDir(absRoot, 0, maxDepth, &repos)
	return repos
}

func walkDir(current string, depth, maxDepth int, repos *[]string) {
	gitDir := filepath.Join(current, ".git")
	if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
		*repos = append(*repos, current)
		return // don't recurse into nested .git repos
	}

	if depth >= maxDepth {
		return
	}

	entries, err := os.ReadDir(current)
	if err != nil {
		return
	}

	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		walkDir(filepath.Join(current, e.Name()), depth+1, maxDepth, repos)
	}
}

// checkSingleRepo runs all checks for a single repository.
func checkSingleRepo(ctx context.Context, executor *gitcmd.Executor, repoPath, name string, verbose bool) []CheckResult {
	var results []CheckResult

	// 1. No remote configured
	results = append(results, checkNoRemote(ctx, executor, repoPath, name)...)

	// 2. Detached HEAD
	results = append(results, checkDetachedHead(ctx, executor, repoPath, name)...)

	// 3. Merge/rebase in progress
	results = append(results, checkIncompleteOps(repoPath, name)...)

	// 4. Merge conflicts
	conflictResults := checkConflicts(ctx, executor, repoPath, name)
	results = append(results, conflictResults...)

	// 5. Dirty worktree with behind upstream (sync blocker)
	// Skip if conflicts already reported (conflict files are also dirty)
	if len(conflictResults) == 0 {
		results = append(results, checkDirtyBehind(ctx, executor, repoPath, name)...)
	}

	// 6. Origin divergence (ahead/behind)
	results = append(results, checkUpstreamDivergence(ctx, executor, repoPath, name)...)

	// 7. develop vs main/master distance
	results = append(results, checkDevelopMainDistance(ctx, executor, repoPath, name)...)

	// 8. Feature branch divergence
	if verbose {
		results = append(results, checkFeatureBranchDivergence(ctx, executor, repoPath, name)...)
	}

	// 9. Stash entries stranded on this machine
	results = append(results, checkStrandedStash(ctx, executor, repoPath, name)...)

	return results
}

// --- Individual Checks ---

func checkNoRemote(ctx context.Context, executor *gitcmd.Executor, repoPath, name string) []CheckResult {
	output, err := executor.RunOutput(ctx, repoPath, "remote")
	if err != nil || strings.TrimSpace(output) == "" {
		return []CheckResult{{
			Name:     fmt.Sprintf("repo:%s:remote", name),
			Category: CategoryRepo,
			Status:   StatusError,
			Message:  fmt.Sprintf("%s: no remote configured", name),
			Detail:   "sync/fetch/pull/push will fail. Add a remote: git remote add origin <url>",
		}}
	}
	return nil
}

func checkDetachedHead(ctx context.Context, executor *gitcmd.Executor, repoPath, name string) []CheckResult {
	output, err := executor.RunOutput(ctx, repoPath, "branch", "--show-current")
	if err != nil {
		return nil
	}
	if strings.TrimSpace(output) == "" {
		return []CheckResult{{
			Name:     fmt.Sprintf("repo:%s:detached", name),
			Category: CategoryRepo,
			Status:   StatusWarning,
			Message:  fmt.Sprintf("%s: HEAD is detached", name),
			Detail:   "sync operations require a checked-out branch",
		}}
	}
	return nil
}

func checkIncompleteOps(repoPath, name string) []CheckResult {
	var results []CheckResult

	if _, err := os.Stat(filepath.Join(repoPath, ".git", "MERGE_HEAD")); err == nil {
		results = append(results, CheckResult{
			Name:     fmt.Sprintf("repo:%s:merge", name),
			Category: CategoryRepo,
			Status:   StatusError,
			Message:  fmt.Sprintf("%s: merge in progress", name),
			Detail:   "resolve conflicts and run 'git merge --continue', or 'git merge --abort'",
		})
	}

	rebaseDirs := []string{
		filepath.Join(repoPath, ".git", "rebase-merge"),
		filepath.Join(repoPath, ".git", "rebase-apply"),
	}
	for _, d := range rebaseDirs {
		if _, err := os.Stat(d); err == nil {
			results = append(results, CheckResult{
				Name:     fmt.Sprintf("repo:%s:rebase", name),
				Category: CategoryRepo,
				Status:   StatusError,
				Message:  fmt.Sprintf("%s: rebase in progress", name),
				Detail:   "run 'git rebase --continue' or 'git rebase --abort'",
			})
			break
		}
	}

	return results
}

func checkConflicts(ctx context.Context, executor *gitcmd.Executor, repoPath, name string) []CheckResult {
	output, err := executor.RunOutput(ctx, repoPath, "status", "--porcelain")
	if err != nil {
		return nil
	}

	conflictCount := 0
	for line := range strings.SplitSeq(output, "\n") {
		if len(line) < 2 {
			continue
		}
		// Unmerged status codes: UU, AA, DD, AU, UA, DU, UD
		x, y := line[0], line[1]
		if x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D') {
			conflictCount++
		}
	}

	if conflictCount > 0 {
		return []CheckResult{{
			Name:     fmt.Sprintf("repo:%s:conflict", name),
			Category: CategoryRepo,
			Status:   StatusError,
			Message:  fmt.Sprintf("%s: %d file(s) with merge conflicts", name, conflictCount),
			Detail:   "resolve conflicts before sync/pull operations",
		}}
	}
	return nil
}

// checkStrandedStash reports stash entries that have outlived the task that
// created them. A fresh stash is not a problem — it is what the command is for —
// so only entries older than repository.StrandedStashAge are reported. Nothing
// here is auto-fixable: a stash is invisible to every other machine, and turning
// one into a commit is a decision rather than a cleanup.
func checkStrandedStash(ctx context.Context, executor *gitcmd.Executor, repoPath, name string) []CheckResult {
	output, err := executor.RunOutput(ctx, repoPath, "stash", "list", "--format=%ct")
	if err != nil {
		return nil
	}

	count, oldest := repository.ParseStashDates(output)
	if count == 0 || !repository.StashIsStranded(oldest) {
		return nil
	}

	subject := fmt.Sprintf("%d stash entries", count)
	if count == 1 {
		subject = "1 stash entry"
	}

	days := int(time.Since(oldest).Hours() / 24)
	return []CheckResult{{
		Name:     fmt.Sprintf("repo:%s:stash", name),
		Category: CategoryRepo,
		Status:   StatusWarning,
		Message:  fmt.Sprintf("%s: %s, oldest %d days old", name, subject, days),
		Detail:   "a stash never leaves this machine. Restore it with 'git stash pop', or commit it to a branch that can be pushed",
	}}
}

func checkDirtyBehind(ctx context.Context, executor *gitcmd.Executor, repoPath, name string) []CheckResult {
	// Check dirty
	dirty, err := executor.RunOutput(ctx, repoPath, "status", "--porcelain")
	if err != nil || strings.TrimSpace(dirty) == "" {
		return nil // clean or error
	}

	// Check behind
	output, err := executor.RunOutput(ctx, repoPath, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		return nil // no upstream
	}

	_, behind := parseAheadBehind(output)
	if behind > 0 {
		return []CheckResult{{
			Name:     fmt.Sprintf("repo:%s:dirty-behind", name),
			Category: CategoryRepo,
			Status:   StatusError,
			Message:  fmt.Sprintf("%s: dirty worktree + %d commits behind upstream", name, behind),
			Detail:   "sync will fail. commit/stash changes first, then pull",
		}}
	}
	return nil
}

func checkUpstreamDivergence(ctx context.Context, executor *gitcmd.Executor, repoPath, name string) []CheckResult {
	output, err := executor.RunOutput(ctx, repoPath, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		return nil // no upstream configured
	}

	ahead, behind := parseAheadBehind(output)

	if ahead > 0 && behind > 0 {
		status := StatusWarning
		if behind > DivergeBehindWarn {
			status = StatusError
		}
		return []CheckResult{{
			Name:     fmt.Sprintf("repo:%s:diverged", name),
			Category: CategoryRepo,
			Status:   status,
			Message:  fmt.Sprintf("%s: diverged from upstream (%d ahead, %d behind)", name, ahead, behind),
			Detail:   "branches have diverged. merge or rebase to reconcile",
		}}
	}

	if behind > DivergeBehindWarn {
		return []CheckResult{{
			Name:     fmt.Sprintf("repo:%s:behind", name),
			Category: CategoryRepo,
			Status:   StatusWarning,
			Message:  fmt.Sprintf("%s: %d commits behind upstream", name, behind),
			Detail:   "run 'gz-git pull' or 'gz-git sync' to update",
		}}
	}

	if ahead > DivergeAheadWarn {
		return []CheckResult{{
			Name:     fmt.Sprintf("repo:%s:ahead", name),
			Category: CategoryRepo,
			Status:   StatusWarning,
			Message:  fmt.Sprintf("%s: %d commits ahead of upstream (unpushed)", name, ahead),
			Detail:   "run 'gz-git push' to publish changes",
		}}
	}

	return nil
}

func checkDevelopMainDistance(ctx context.Context, executor *gitcmd.Executor, repoPath, name string) []CheckResult {
	// Find develop branch (local or remote tracking)
	developBranch := ""
	for _, candidate := range []string{"develop", "dev"} {
		ok, err := executor.RunQuiet(ctx, repoPath, "rev-parse", "--verify", candidate)
		if err == nil && ok {
			developBranch = candidate
			break
		}
	}
	if developBranch == "" {
		return nil
	}

	// Find main branch
	mainBranch := findMainBranch(ctx, executor, repoPath)
	if mainBranch == "" {
		return nil
	}

	distance := branchDistance(ctx, executor, repoPath, developBranch, mainBranch)
	if distance < 0 {
		return nil
	}

	if distance >= BranchDistanceError {
		return []CheckResult{{
			Name:     fmt.Sprintf("repo:%s:develop-main", name),
			Category: CategoryRepo,
			Status:   StatusError,
			Message:  fmt.Sprintf("%s: %s is %d commits from %s", name, developBranch, distance, mainBranch),
			Detail:   fmt.Sprintf("consider merging %s into %s (threshold: %d)", developBranch, mainBranch, BranchDistanceError),
		}}
	}

	if distance >= BranchDistanceWarn {
		return []CheckResult{{
			Name:     fmt.Sprintf("repo:%s:develop-main", name),
			Category: CategoryRepo,
			Status:   StatusWarning,
			Message:  fmt.Sprintf("%s: %s is %d commits from %s", name, developBranch, distance, mainBranch),
			Detail:   fmt.Sprintf("branches are drifting apart (warn: %d, error: %d)", BranchDistanceWarn, BranchDistanceError),
		}}
	}

	return nil
}

func checkFeatureBranchDivergence(ctx context.Context, executor *gitcmd.Executor, repoPath, name string) []CheckResult {
	output, err := executor.RunOutput(ctx, repoPath, "branch", "--format=%(refname:short)")
	if err != nil {
		return nil
	}

	// Find base branch for distance measurement
	baseBranch := findBaseBranch(ctx, executor, repoPath)
	if baseBranch == "" {
		return nil
	}

	var results []CheckResult
	for line := range strings.SplitSeq(output, "\n") {
		branchName := strings.TrimSpace(line)
		if branchName == "" {
			continue
		}

		isFeature := false
		for _, prefix := range featureBranchPrefixes {
			if strings.HasPrefix(branchName, prefix) {
				isFeature = true
				break
			}
		}
		if !isFeature {
			continue
		}

		distance := branchDistance(ctx, executor, repoPath, branchName, baseBranch)
		if distance < 0 {
			continue
		}

		if distance >= FeatureBranchDistanceError {
			results = append(results, CheckResult{
				Name:     fmt.Sprintf("repo:%s:branch:%s", name, branchName),
				Category: CategoryRepo,
				Status:   StatusError,
				Message:  fmt.Sprintf("%s: branch '%s' is %d commits from %s", name, branchName, distance, baseBranch),
				Detail:   "branch may be unmergeable. rebase or consider abandoning",
			})
		} else if distance >= FeatureBranchDistanceWarn {
			results = append(results, CheckResult{
				Name:     fmt.Sprintf("repo:%s:branch:%s", name, branchName),
				Category: CategoryRepo,
				Status:   StatusWarning,
				Message:  fmt.Sprintf("%s: branch '%s' is %d commits from %s", name, branchName, distance, baseBranch),
				Detail:   fmt.Sprintf("consider rebasing onto %s soon (warn: %d, error: %d)", baseBranch, FeatureBranchDistanceWarn, FeatureBranchDistanceError),
			})
		}
	}

	return results
}

// --- Helpers ---

func parseAheadBehind(output string) (ahead, behind int) {
	parts := strings.Fields(strings.TrimSpace(output))
	if len(parts) != 2 {
		return 0, 0
	}
	ahead, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0
	}
	behind, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0
	}
	return ahead, behind
}

// branchDistance returns the total symmetric commit distance between two refs.
// Returns -1 if comparison is not possible.
func branchDistance(ctx context.Context, executor *gitcmd.Executor, repoPath, branch1, branch2 string) int {
	output, err := executor.RunOutput(ctx, repoPath, "rev-list", "--left-right", "--count", branch1+"..."+branch2)
	if err != nil {
		return -1
	}
	ahead, behind := parseAheadBehind(output)
	return ahead + behind
}

// findMainBranch returns "main" or "master", whichever exists locally.
func findMainBranch(ctx context.Context, executor *gitcmd.Executor, repoPath string) string {
	for _, candidate := range []string{"main", "master"} {
		ok, err := executor.RunQuiet(ctx, repoPath, "rev-parse", "--verify", candidate)
		if err == nil && ok {
			return candidate
		}
	}
	return ""
}

// findBaseBranch returns the best base branch for feature comparison (develop > main > master).
func findBaseBranch(ctx context.Context, executor *gitcmd.Executor, repoPath string) string {
	for _, candidate := range []string{"develop", "dev", "main", "master"} {
		ok, err := executor.RunQuiet(ctx, repoPath, "rev-parse", "--verify", candidate)
		if err == nil && ok {
			return candidate
		}
	}
	return ""
}
