// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
)

type gitRepo struct {
	exec *gitcmd.Executor
	dir  string
}

func newGitRepo(exec *gitcmd.Executor, dir string) gitRepo {
	return gitRepo{exec: exec, dir: dir}
}

func (g gitRepo) run(ctx context.Context, args ...string) (*gitcmd.Result, error) {
	res, err := g.exec.Run(ctx, g.dir, args...)
	if err != nil {
		return res, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return res, nil
}

func (g gitRepo) output(ctx context.Context, args ...string) (string, error) {
	out, err := g.exec.RunOutput(ctx, g.dir, args...)
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(out), nil
}

func (g gitRepo) isRepo(ctx context.Context) bool {
	res, err := g.run(ctx, "rev-parse", "--git-dir")
	return err == nil && res != nil && res.ExitCode == 0
}

func (g gitRepo) toplevel(ctx context.Context) (string, error) {
	return g.output(ctx, "rev-parse", "--show-toplevel")
}

func (g gitRepo) remotes(ctx context.Context) ([]string, error) {
	res, err := g.run(ctx, "remote")
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return nil, nil
	}
	return splitNonEmpty(res.Stdout), nil
}

func (g gitRepo) refNames(ctx context.Context) ([]string, error) {
	res, err := g.run(ctx, "for-each-ref", "--format=%(refname)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("git for-each-ref failed: %s", strings.TrimSpace(res.Stderr))
	}
	return splitNonEmpty(res.Stdout), nil
}

func (g gitRepo) shortRefs(ctx context.Context, prefixes ...string) ([]string, error) {
	args := make([]string, 0, 2+len(prefixes))
	args = append(args, "for-each-ref", "--format=%(refname:short)")
	args = append(args, prefixes...)
	res, err := g.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("git for-each-ref failed: %s", strings.TrimSpace(res.Stderr))
	}
	return splitNonEmpty(res.Stdout), nil
}

func (g gitRepo) revParse(ctx context.Context, rev string) (sha string, ok bool, err error) {
	res, err := g.run(ctx, "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	if err != nil {
		return "", false, err
	}
	if res.ExitCode != 0 {
		return "", false, nil
	}
	sha = strings.TrimSpace(res.Stdout)
	return sha, sha != "", nil
}

func (g gitRepo) symbolicRef(ctx context.Context, name string) (ref string, ok bool, err error) {
	res, err := g.run(ctx, "symbolic-ref", "--quiet", "--short", name)
	if err != nil {
		return "", false, err
	}
	if res.ExitCode != 0 {
		return "", false, nil
	}
	out := strings.TrimSpace(res.Stdout)
	return out, out != "", nil
}

func (g gitRepo) fetchPrune(ctx context.Context, remote string) error {
	res, err := g.run(ctx, "fetch", remote, "--prune", "--quiet")
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("git fetch %s --prune failed: %s", remote, strings.TrimSpace(res.Stderr))
	}
	return nil
}

func (g gitRepo) aheadBehind(ctx context.Context, left, right string) (ahead, behind int, err error) {
	out, err := g.output(ctx, "rev-list", "--left-right", "--count", left+"..."+right)
	if err != nil {
		return 0, 0, err
	}
	return parseAheadBehind(out)
}

func (g gitRepo) mergeBase(ctx context.Context, a, b string) (string, error) {
	return g.output(ctx, "merge-base", a, b)
}

func (g gitRepo) revCount(ctx context.Context, revRange string) (int, error) {
	out, err := g.output(ctx, "rev-list", "--count", revRange)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, fmt.Errorf("parse rev-list count %q: %w", out, err)
	}
	return n, nil
}

func (g gitRepo) mergeTreeClean(ctx context.Context, a, b string) (bool, error) {
	res, err := g.run(ctx, "merge-tree", "--write-tree", a, b)
	if err != nil {
		return false, err
	}
	return res.ExitCode == 0, nil
}

func (g gitRepo) commitTime(ctx context.Context, rev string) (int64, error) {
	out, err := g.output(ctx, "log", "-1", "--format=%ct", rev)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse commit time %q: %w", out, err)
	}
	return n, nil
}

func (g gitRepo) currentBranch(ctx context.Context) (string, error) {
	return g.output(ctx, "rev-parse", "--abbrev-ref", "HEAD")
}

func (g gitRepo) headSHA(ctx context.Context) (string, error) {
	return g.output(ctx, "rev-parse", "HEAD")
}

func (g gitRepo) porcelain(ctx context.Context) (string, error) {
	res, err := g.run(ctx, "status", "--porcelain")
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("git status --porcelain failed: %s", strings.TrimSpace(res.Stderr))
	}
	return res.Stdout, nil
}

func (g gitRepo) upstreamSHA(ctx context.Context, branch string) (sha string, ok bool, err error) {
	return g.revParse(ctx, branch+"@{upstream}")
}

func (g gitRepo) upstreamName(ctx context.Context, branch string) (name string, ok bool, err error) {
	res, err := g.run(ctx, "rev-parse", "--abbrev-ref", "--symbolic-full-name", branch+"@{upstream}")
	if err != nil {
		return "", false, err
	}
	if res.ExitCode != 0 {
		return "", false, nil
	}
	name = strings.TrimSpace(res.Stdout)
	return name, name != "", nil
}

func (g gitRepo) branchRemote(ctx context.Context, branch string) (string, error) {
	res, err := g.run(ctx, "config", "--get", "branch."+branch+".remote")
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", nil
	}
	return strings.TrimSpace(res.Stdout), nil
}

func (g gitRepo) isAncestor(ctx context.Context, ancestor, tip string) (bool, error) {
	res, err := g.run(ctx, "merge-base", "--is-ancestor", ancestor, tip)
	if err != nil {
		return false, err
	}
	return res.ExitCode == 0, nil
}

func (g gitRepo) lsTreeNames(ctx context.Context, sha string) ([]string, error) {
	out, err := g.output(ctx, "ls-tree", "-r", "--name-only", sha)
	if err != nil {
		return nil, err
	}
	return splitNonEmpty(out), nil
}

func (g gitRepo) diffNames(ctx context.Context, a, b string) ([]string, error) {
	out, err := g.output(ctx, "diff", "--name-only", a, b)
	if err != nil {
		return nil, err
	}
	return splitNonEmpty(out), nil
}

func (g gitRepo) worktreeAddDetach(ctx context.Context, path, sha string) error {
	res, err := g.run(ctx, "worktree", "add", "--detach", path, sha)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("git worktree add failed: %s", strings.TrimSpace(res.Stderr))
	}
	return nil
}

func (g gitRepo) worktreeRemoveForce(ctx context.Context, path string) error {
	res, err := g.run(ctx, "worktree", "remove", "--force", path)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("git worktree remove failed: %s", strings.TrimSpace(res.Stderr))
	}
	return nil
}

func parseAheadBehind(raw string) (ahead, behind int, err error) {
	raw = strings.TrimSpace(raw)
	parts := strings.Fields(raw)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected rev-list count %q", raw)
	}
	ahead, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("parse ahead %q: %w", parts[0], err)
	}
	behind, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("parse behind %q: %w", parts[1], err)
	}
	return ahead, behind, nil
}

func splitNonEmpty(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// preferredRemote picks the remote queue/check/run should talk to.
// origin wins when it exists; otherwise a single remote; otherwise the first.
func preferredRemote(remotes []string) string {
	if isRemoteName("origin", remotes) {
		return "origin"
	}
	if len(remotes) == 1 {
		return remotes[0]
	}
	if len(remotes) > 0 {
		return remotes[0]
	}
	return ""
}
