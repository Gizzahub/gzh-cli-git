// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)

func TestPrepareLegacyTrees_TargetBeforeSourceAndNoRegistrationRemains(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	if err := os.MkdirAll(filepath.Join(fx.Worktree, "ent"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, fx.Worktree, ".gitignore", "ent/generated/\n")
	writeFile(t, fx.Worktree, "ent/.keep", "")
	runGitInTest(t, fx.Worktree, "add", "ent/.keep", ".gitignore")
	runGitInTest(t, fx.Worktree, "commit", "-m", "ent")
	sha := runGitInTest(t, fx.Worktree, "rev-parse", "HEAD")
	bin := fakeGo(t, "root=$(dirname \"$PWD\"); b=$(basename \"$PWD\"); [ \"$b\" = target ] && [ ! -e \"$root/source\" ]; [ \"$b\" = source ] && [ ! -e \"$root/target\" ]; mkdir -p ent/generated; : > ent/generated/out")
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	g := newGitRepo(gitcmd.NewExecutor(), fx.Worktree)
	p, err := prepareLegacyTrees(context.Background(), g, TargetPlan{BranchSHA: strings.TrimSpace(sha), TargetSHA: strings.TrimSpace(sha)}, &controllerBinding{PrepareProfile: familybookEntPrepareV1})
	if err != nil {
		t.Fatal(err)
	}
	if p.baseline["check"].Target != "check" || p.baseline["lint"].Target != "lint" {
		t.Fatalf("baseline probes not captured: %#v", p.baseline)
	}
	if !p.controllerPrepared || !p.baseline["lint"].ControllerPrepared {
		t.Fatalf("controller prepared evidence was not retained: %#v", p)
	}
	root := p.root
	if err := p.cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("prepare root remains: %v", err)
	}
}

func TestPreparedLegacyWithoutControllerDoesNotAnnotateProbe(t *testing.T) {
	p, err := prepareLegacyTrees(context.Background(), gitRepo{dir: t.TempDir()}, TargetPlan{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	probe := p.annotateProbe(context.Background(), makeProbe{WorkDir: p.source})
	if probe.ControllerPrepared || probe.GoRootSrc != "" {
		t.Fatalf("legacy probe gained controller allowance: %#v", probe)
	}
}

func TestRunPrepareProfileRejectsFailuresAndForbiddenOutput(t *testing.T) {
	for name, body := range map[string]string{
		"failure":         "exit 9",
		"tracked":         "echo x >> tracked",
		"ignored outside": "mkdir -p ignored; : > ignored/x",
	} {
		t.Run(name, func(t *testing.T) {
			fx := testutil.TempWorktreeWithBareOrigin(t)
			if err := os.MkdirAll(filepath.Join(fx.Worktree, "ent"), 0o755); err != nil {
				t.Fatal(err)
			}
			writeFile(t, fx.Worktree, "ent/.keep", "")
			writeFile(t, fx.Worktree, "tracked", "base\n")
			writeFile(t, fx.Worktree, ".gitignore", "ignored/\n")
			runGitInTest(t, fx.Worktree, "add", ".")
			runGitInTest(t, fx.Worktree, "commit", "-m", "fixture")
			t.Setenv("PATH", fakeGo(t, body)+":"+os.Getenv("PATH"))
			if err := runPrepareProfile(context.Background(), newGitRepo(gitcmd.NewExecutor(), fx.Worktree), fx.Worktree, familybookEntPrepareV1); err == nil {
				t.Fatal("preparation unexpectedly passed")
			}
		})
	}
}

func TestRunPrepareProfileRejectsEntSymlink(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(fx.Worktree, "ent")); err != nil {
		t.Fatal(err)
	}
	if err := runPrepareProfile(context.Background(), newGitRepo(gitcmd.NewExecutor(), fx.Worktree), fx.Worktree, familybookEntPrepareV1); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("err=%v", err)
	}
}

func TestPrepareLegacyTreesReportsTargetAndSourcePreparationFailures(t *testing.T) {
	for name, body := range map[string]string{"target": "if [ \"$(basename \"$PWD\")\" = target ]; then exit 7; fi", "source": "if [ \"$(basename \"$PWD\")\" = source ]; then exit 8; fi"} {
		t.Run(name, func(t *testing.T) {
			fx := testutil.TempWorktreeWithBareOrigin(t)
			if err := os.MkdirAll(filepath.Join(fx.Worktree, "ent"), 0o755); err != nil {
				t.Fatal(err)
			}
			writeFile(t, fx.Worktree, "ent/.keep", "")
			runGitInTest(t, fx.Worktree, "add", ".")
			runGitInTest(t, fx.Worktree, "commit", "-m", "ent")
			sha := strings.TrimSpace(runGitInTest(t, fx.Worktree, "rev-parse", "HEAD"))
			t.Setenv("PATH", fakeGo(t, body)+":"+os.Getenv("PATH"))
			_, err := prepareLegacyTrees(context.Background(), newGitRepo(gitcmd.NewExecutor(), fx.Worktree), TargetPlan{BranchSHA: sha, TargetSHA: sha}, &controllerBinding{PrepareProfile: familybookEntPrepareV1})
			if err == nil || !strings.Contains(err.Error(), "prepare "+name) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func fakeGo(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "go")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}
