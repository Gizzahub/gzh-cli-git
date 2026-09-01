// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type preparedLegacy struct {
	source, root string
	baseline     map[string]makeProbe
	g            gitRepo
}

func (p preparedLegacy) cleanup(ctx context.Context) error {
	if p.root == "" {
		return nil
	}
	return removePreparedWorktree(ctx, p.g, p.source, p.root)
}

// target is prepared and measured before it is removed; source is never alive
// at the same time, so repository code cannot use the baseline worktree.
func prepareLegacyTrees(ctx context.Context, g gitRepo, plan TargetPlan, c *controllerBinding) (preparedLegacy, error) {
	if c == nil || c.PrepareProfile == "" {
		return preparedLegacy{source: g.dir}, nil
	}
	root, err := os.MkdirTemp("", "gz-git-integrate-prepare-")
	if err != nil {
		return preparedLegacy{}, err
	}
	target := filepath.Join(root, "target")
	if err := g.worktreeAddDetach(ctx, target, plan.TargetSHA); err != nil {
		_ = os.RemoveAll(root)
		return preparedLegacy{}, fmt.Errorf("prepare target worktree: %w", err)
	}
	if err := runPrepareProfile(ctx, g, target, c.PrepareProfile); err != nil {
		cleanupErr := removePreparedWorktree(ctx, g, target, root)
		return preparedLegacy{}, errors.Join(fmt.Errorf("prepare target: %w", err), cleanupErr)
	}
	baseline := map[string]makeProbe{"check": runMakeTarget(ctx, target, "check"), "lint": runMakeTarget(ctx, target, "lint")}
	if err := removePreparedWorktree(ctx, g, target, ""); err != nil {
		return preparedLegacy{}, fmt.Errorf("cleanup prepared target: %w", err)
	}
	source := filepath.Join(root, "source")
	if err := g.worktreeAddDetach(ctx, source, plan.BranchSHA); err != nil {
		_ = os.RemoveAll(root)
		return preparedLegacy{}, fmt.Errorf("prepare source worktree: %w", err)
	}
	if err := runPrepareProfile(ctx, g, source, c.PrepareProfile); err != nil {
		cleanupErr := removePreparedWorktree(ctx, g, source, root)
		return preparedLegacy{}, errors.Join(fmt.Errorf("prepare source: %w", err), cleanupErr)
	}
	return preparedLegacy{source: source, root: root, baseline: baseline, g: g}, nil
}

func removePreparedWorktree(parent context.Context, g gitRepo, wt, root string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 30*time.Second)
	defer cancel()
	if err := g.worktreeRemoveForce(ctx, wt); err != nil {
		return err
	}
	trees, err := g.listWorktrees(ctx)
	if err != nil {
		return err
	}
	for _, tree := range trees {
		if tree.Path == wt {
			return fmt.Errorf("worktree remains registered: %s", wt)
		}
		if root != "" {
			rel, relErr := filepath.Rel(root, tree.Path)
			if relErr == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))) {
				return fmt.Errorf("worktree remains registered below prepare root: %s", tree.Path)
			}
		}
	}
	if root != "" {
		return os.RemoveAll(root)
	}
	return nil
}

// This is process isolation, not a security sandbox. As with legacy Make,
// the selected task revision is trusted to execute repository-owned code.
func runPrepareProfile(parent context.Context, g gitRepo, dir, profile string) error {
	if profile != familybookEntPrepareV1 {
		return fmt.Errorf("unsupported preparation profile %q", profile)
	}
	if err := rejectEntSymlinkChain(dir); err != nil {
		return err
	}
	before, err := g.refNames(parent)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	isolated, err := os.MkdirTemp("", "gz-git-integrate-go-env-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(isolated)
	home := filepath.Join(isolated, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "go", "generate", "./ent") // #nosec G204 -- fixed closed profile.
	cmd.Dir = dir
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(isolated, "xdg-config"), "XDG_CACHE_HOME=" + filepath.Join(isolated, "xdg-cache"), "GOCACHE=" + filepath.Join(isolated, "go-cache"), "GOMODCACHE=" + filepath.Join(isolated, "go-mod"), "GOWORK=off", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C"}
	var out bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &out, n: 128 << 10}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go generate ./ent failed: %w: %s", err, strings.TrimSpace(out.String()))
	}
	if ctx.Err() != nil {
		return fmt.Errorf("preparation timed out")
	}
	after, err := g.refNames(parent)
	if err != nil {
		return err
	}
	if strings.Join(before, "\x00") != strings.Join(after, "\x00") {
		return fmt.Errorf("preparation changed git refs")
	}
	return validatePreparedStatus(ctx, dir)
}

func rejectEntSymlinkChain(dir string) error {
	for _, part := range []string{"ent", "ent/generated"} {
		info, err := os.Lstat(filepath.Join(dir, part))
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("preparation path is symlink: %s", part)
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func validatePreparedStatus(ctx context.Context, dir string) error {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignored=matching")
	cmd.Dir = dir
	raw, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("inspect prepared tree: %w", err)
	}
	for _, record := range bytes.Split(raw, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		if len(record) < 4 {
			return fmt.Errorf("invalid preparation status")
		}
		state, path := string(record[:2]), string(record[3:])
		// Familybook Ent output is intentionally ignored. Untracked output is
		// rejected too: the profile must not silently introduce new files.
		if state != "!!" || !strings.HasPrefix(path, "ent/generated/") {
			return fmt.Errorf("preparation changed forbidden path: %s", path)
		}
	}
	return nil
}

type limitedWriter struct {
	w *bytes.Buffer
	n int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.n <= 0 {
		return len(p), nil
	}
	if len(p) > w.n {
		p = p[:w.n]
	}
	w.n -= len(p)
	return w.w.Write(p)
}
