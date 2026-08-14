// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"context"
	"errors"
	"fmt"
	"io"
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

func (r ReclaimResult) Incomplete() bool {
	return len(r.Failed) > 0
}

type reclaimOpts struct {
	Branch       string
	TargetBranch string
	DefaultName  string
	Integration  string
	Remote       string
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
	var taskTrees []listedWorktree
	var targetWT, mainWT string
	for i, wt := range trees {
		if i == 0 {
			mainWT = wt.Path
		}
		if wt.Branch == opts.Branch {
			taskTrees = append(taskTrees, wt)
		}
		if wt.Branch == opts.TargetBranch && targetWT == "" {
			targetWT = wt.Path
		}
	}
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

	if taskWT != "" {
		if nested := nestedIgnoredRepos(ctx, sg, taskWT); len(nested) > 0 {
			out.Failed = append(out.Failed, "ignored nested git repo in worktree: "+strings.Join(nested, " "))
			return out
		}
		res, err := sg.run(ctx, "worktree", "remove", taskWT)
		if err != nil || (res != nil && res.ExitCode != 0) {
			detail := ""
			if res != nil {
				detail = strings.TrimSpace(res.Stderr)
			}
			if err != nil && detail == "" {
				detail = err.Error()
			}
			out.Failed = append(out.Failed, "worktree remove "+taskWT+": "+detail)
			return out
		}
		out.Done = append(out.Done, "worktree("+taskWT+")")
	}

	res, err := sg.run(ctx, "branch", "-d", opts.Branch)
	if err != nil || (res != nil && res.ExitCode != 0) {
		detail := ""
		if res != nil {
			detail = strings.TrimSpace(res.Stderr)
		}
		if err != nil && detail == "" {
			detail = err.Error()
		}
		out.Failed = append(out.Failed, "branch -d "+opts.Branch+": "+detail)
		return out
	}
	out.Done = append(out.Done, "local-branch")

	if opts.Remote != "" {
		if _, ok, err := sg.revParse(ctx, "refs/remotes/"+opts.Remote+"/"+opts.Branch); err != nil {
			out.Failed = append(out.Failed, err.Error())
			return out
		} else if ok {
			del, err := sg.run(ctx, "push", opts.Remote, "--delete", opts.Branch)
			if err != nil || (del != nil && del.ExitCode != 0) {
				heads, lsErr := sg.output(ctx, "ls-remote", "--heads", opts.Remote, opts.Branch)
				if lsErr != nil || strings.TrimSpace(heads) != "" {
					detail := ""
					if del != nil {
						detail = strings.TrimSpace(del.Stderr)
					}
					out.Failed = append(out.Failed, "push --delete "+opts.Remote+"/"+opts.Branch+": "+detail)
					return out
				}
				out.Done = append(out.Done, "remote-branch(already-deleted)")
			} else {
				out.Done = append(out.Done, "remote-branch")
			}
		}
	}

	if taskWT != "" {
		parent := filepath.Dir(taskWT)
		home, _ := os.UserHomeDir()
		worktreesRoot := filepath.Join(home, "worktrees")
		if parent != worktreesRoot {
			if empty, err := dirEmpty(parent); err == nil && empty {
				if err := os.Remove(parent); err != nil {
					out.Failed = append(out.Failed, "rmdir "+parent+": "+err.Error())
					return out
				}
				out.Done = append(out.Done, "parent-dir("+parent+")")
			}
		}
	}
	return out
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
	f, err := os.Open(path) //nolint:gosec // reclaim inspects the worktree parent
	if err != nil {
		return false, err
	}
	defer f.Close()
	names, err := f.Readdirnames(1)
	if errors.Is(err, io.EOF) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return len(names) == 0, nil
}

func defaultBranchName(defaultRef, remote string) string {
	if remote != "" && strings.HasPrefix(defaultRef, remote+"/") {
		return strings.TrimPrefix(defaultRef, remote+"/")
	}
	if i := strings.LastIndex(defaultRef, "/"); i >= 0 {
		return defaultRef[i+1:]
	}
	return defaultRef
}

func targetBranchName(target, remote string) string {
	if remote != "" && strings.HasPrefix(target, remote+"/") {
		return strings.TrimPrefix(target, remote+"/")
	}
	return target
}
