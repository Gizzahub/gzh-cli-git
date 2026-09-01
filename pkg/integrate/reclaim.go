// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
)

// ReclaimResult records what reclaim did after a successful integrate.
type ReclaimResult struct {
	Skipped string
	Done    []string
	Failed  []string
}

// Incomplete reports a successful integrate whose reclaim did not finish.
func (r ReclaimResult) Incomplete() bool {
	return len(r.Failed) > 0
}

type reclaimOpts struct {
	Branch       string
	TargetBranch string
	DefaultName  string
	Integration  string
	Remote       string
	PushRemote   string
	TaskSHA      string
	Patterns     []string
	Facts        []string
}

func reclaimAfter(ctx context.Context, exec *gitcmd.Executor, g gitRepo, opts reclaimOpts) ReclaimResult {
	var out ReclaimResult
	if opts.Branch == opts.TargetBranch ||
		(opts.DefaultName != "" && opts.Branch == opts.DefaultName) ||
		(opts.Integration != "" && opts.Branch == opts.Integration) {
		out.Skipped = fmt.Sprintf("%s is the integration/default branch", opts.Branch)
		return out
	}

	if len(opts.Patterns) == 0 {
		fact := "no taskPattern declaration"
		if len(opts.Facts) > 0 {
			fact = strings.Join(opts.Facts, "; ")
		}
		out.Skipped = "reclaim nothing: " + fact
		return out
	}
	if !config.MatchesAnyTaskPattern(opts.Branch, opts.Patterns) {
		out.Skipped = fmt.Sprintf("%s does not match taskPattern %s", opts.Branch, strings.Join(opts.Patterns, " "))
		return out
	}

	trees, err := g.listWorktrees(ctx)
	if err != nil {
		out.Failed = append(out.Failed, "list worktrees: "+err.Error())
		return out
	}
	taskTrees, targetWT, mainWT := classifyReclaimTrees(trees, opts.Branch, opts.TargetBranch)
	if len(taskTrees) > 1 {
		paths := make([]string, 0, len(taskTrees))
		for _, wt := range taskTrees {
			paths = append(paths, wt.Path)
		}
		out.Failed = append(out.Failed, fmt.Sprintf("%s is checked out in multiple worktrees: %s", opts.Branch, strings.Join(paths, " ")))
		return out
	}
	var taskWT string
	if len(taskTrees) == 1 {
		taskWT = taskTrees[0].Path
	}
	if taskWT != "" && taskWT == mainWT {
		out.Skipped = fmt.Sprintf("main checkout holds %s (%s)", opts.Branch, taskWT)
		return out
	}

	stand := targetWT
	if stand == "" {
		stand = mainWT
	}
	if stand == "" {
		out.Failed = append(out.Failed, "no live worktree to reclaim from")
		return out
	}
	sg := newGitRepo(exec, stand)
	if !removeTaskWorktree(ctx, sg, taskWT, &out) {
		return out
	}
	if !deleteLocalTaskBranch(ctx, sg, opts.Branch, &out) {
		return out
	}

	if !reclaimRemoteBranch(ctx, sg, opts, &out) {
		return out
	}
	reclaimEmptyParent(taskWT, &out)
	return out
}

func gitCmdDetail(res *gitcmd.Result, err error) string {
	detail := ""
	if res != nil {
		detail = strings.TrimSpace(res.Stderr)
	}
	if err != nil && detail == "" {
		detail = err.Error()
	}
	return detail
}

func removeTaskWorktree(ctx context.Context, sg gitRepo, taskWT string, out *ReclaimResult) bool {
	if taskWT == "" {
		return true
	}
	if nested := nestedIgnoredRepos(ctx, sg, taskWT); len(nested) > 0 {
		out.Failed = append(out.Failed, "ignored nested git repo in worktree: "+strings.Join(nested, " "))
		return false
	}
	res, err := sg.run(ctx, "worktree", "remove", taskWT)
	if err != nil || (res != nil && res.ExitCode != 0) {
		out.Failed = append(out.Failed, "worktree remove "+taskWT+": "+gitCmdDetail(res, err))
		return false
	}
	out.Done = append(out.Done, "worktree("+taskWT+")")
	return true
}

func deleteLocalTaskBranch(ctx context.Context, sg gitRepo, branch string, out *ReclaimResult) bool {
	res, err := sg.run(ctx, "branch", "-d", branch)
	if err != nil || (res != nil && res.ExitCode != 0) {
		out.Failed = append(out.Failed, "branch -d "+branch+": "+gitCmdDetail(res, err))
		return false
	}
	out.Done = append(out.Done, "local-branch")
	return true
}

func classifyReclaimTrees(trees []listedWorktree, branch, target string) (taskTrees []listedWorktree, targetWT, mainWT string) {
	for i, wt := range trees {
		if i == 0 {
			mainWT = wt.Path
		}
		if wt.Branch == branch {
			taskTrees = append(taskTrees, wt)
		}
		if wt.Branch == target && targetWT == "" {
			targetWT = wt.Path
		}
	}
	return taskTrees, targetWT, mainWT
}

func reclaimRemoteBranch(ctx context.Context, sg gitRepo, opts reclaimOpts, out *ReclaimResult) bool {
	if opts.Remote == "" {
		return true
	}
	if _, ok, err := sg.revParse(ctx, "refs/remotes/"+opts.Remote+"/"+opts.Branch); err != nil {
		out.Failed = append(out.Failed, err.Error())
		return false
	} else if !ok {
		return true
	}
	if opts.TaskSHA == "" {
		out.Failed = append(out.Failed, "remote delete needs the integrated commit for a lease")
		return false
	}
	// Lease against the SHA we integrated, not the current tracking ref.
	// A fetch between check and reclaim can see a newer tip; deleting that
	// tip would drop work that never landed on the target.
	ref := "refs/heads/" + opts.Branch
	lease := "--force-with-lease=" + ref + ":" + opts.TaskSHA
	pushRemote := opts.PushRemote
	if pushRemote == "" {
		pushRemote = opts.Remote
	}
	del, err := sg.run(ctx, "push", lease, pushRemote, ":"+ref)
	if err == nil && (del == nil || del.ExitCode == 0) {
		out.Done = append(out.Done, "remote-branch")
		return true
	}
	heads, lsErr := sg.output(ctx, "ls-remote", "--heads", pushRemote, opts.Branch)
	if lsErr != nil || strings.TrimSpace(heads) != "" {
		detail := ""
		if del != nil {
			detail = strings.TrimSpace(del.Stderr)
		}
		out.Failed = append(out.Failed, "leased remote delete "+opts.Remote+"/"+opts.Branch+": "+detail)
		return false
	}
	out.Done = append(out.Done, "remote-branch(already-deleted)")
	return true
}

func reclaimEmptyParent(taskWT string, out *ReclaimResult) {
	if taskWT == "" {
		return
	}
	parent := filepath.Dir(taskWT)
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	if parent == filepath.Join(home, "worktrees") {
		return
	}
	empty, err := dirEmpty(parent)
	if err != nil || !empty {
		return
	}
	if err := os.Remove(parent); err != nil {
		out.Failed = append(out.Failed, "rmdir "+parent+": "+err.Error())
		return
	}
	out.Done = append(out.Done, "parent-dir("+parent+")")
}

type listedWorktree struct {
	Path   string
	Branch string
}

func (g gitRepo) listWorktrees(ctx context.Context) ([]listedWorktree, error) {
	out, err := g.output(ctx, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var trees []listedWorktree
	var cur listedWorktree
	flush := func() {
		if cur.Path != "" {
			trees = append(trees, cur)
		}
		cur = listedWorktree{}
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch refs/heads/"):
			cur.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		}
	}
	flush()
	return trees, nil
}

func nestedIgnoredRepos(ctx context.Context, g gitRepo, worktree string) []string {
	res, err := g.run(ctx, "-C", worktree, "status", "--porcelain", "--ignored", "-z")
	if err != nil || res == nil || res.ExitCode != 0 {
		return nil
	}
	var nested []string
	for _, rec := range strings.Split(res.Stdout, "\x00") {
		if !strings.HasPrefix(rec, "!! ") {
			continue
		}
		entry := strings.TrimPrefix(rec, "!! ")
		if !strings.HasSuffix(entry, "/") {
			continue
		}
		if _, err := os.Stat(filepath.Join(worktree, strings.TrimSuffix(entry, "/"), ".git")); err == nil {
			nested = append(nested, entry)
		}
	}
	return nested
}

func dirEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func defaultBranchName(defaultRef, remote string) string {
	remotes := []string(nil)
	if remote != "" {
		remotes = []string{remote}
	}
	if name := NormalizeName(defaultRef, remotes); name != "" {
		return name
	}
	return defaultRef
}

func targetBranchName(target, remote string) string {
	if remote != "" && strings.HasPrefix(target, remote+"/") {
		return strings.TrimPrefix(target, remote+"/")
	}
	return target
}
