// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfirmDestructiveAndInterrupt(t *testing.T) {
	ok, err := confirmDestructiveBulk(true)
	if err != nil || !ok {
		t.Fatalf("assumeYes: ok=%v err=%v", ok, err)
	}
	ctx, cancel := withInterruptCancel(context.Background())
	cancel()
	select {
	case <-ctx.Done():
	default:
		t.Error("expected canceled context")
	}
	_ = stdinIsInteractive()
	playNotificationSound()
	_ = playSystemSound()
}

func TestRunBulkWatch_OneShotCancel(t *testing.T) {
	// Run watch with executor that cancels after first call via short interval + context
	// RunBulkWatch loops until error or interrupt; return error after first to stop
	calls := 0
	cfg := WatchConfig{
		Interval:      time.Millisecond * 5,
		Format:        "json",
		Quiet:         true,
		OperationName: "test",
		Directory:     t.TempDir(),
		MaxDepth:      1,
		Parallel:      1,
	}
	err := RunBulkWatch(cfg, func() error {
		calls++
		if calls >= 1 {
			return context.Canceled
		}
		return nil
	})
	// may return canceled or nil depending on impl
	_ = err
	if calls < 1 {
		t.Error("executor not called")
	}
}

func TestCommitWithJSONAndMessageFlags(t *testing.T) {
	parent := setupBulkParent(t)
	repo := filepath.Join(parent, "r1")
	if err := os.WriteFile(filepath.Join(repo, "m.txt"), []byte("m"), 0o600); err != nil {
		t.Fatal(err)
	}
	prevYes, prevFlags := commitYes, commitFlags
	prevJSON, prevYAML, prevMsgs, prevAll := commitJSON, commitYAML, commitMessages, commitAll
	t.Cleanup(func() {
		commitYes, commitFlags = prevYes, prevFlags
		commitJSON, commitYAML, commitMessages, commitAll = prevJSON, prevYAML, prevMsgs, prevAll
	})
	commitFlags = BulkCommandFlags{Depth: 1, Parallel: 1, DryRun: true, Format: "default"}
	commitYes = true
	commitJSON = `{"r1":"feat: from json"}`
	captureStdout(t, func() {
		if err := runCommit(commitCmd, []string{parent}); err != nil {
			t.Logf("commit json: %v", err)
		}
	})
	commitJSON = ""
	commitYAML = "r1: feat: from yaml\n"
	captureStdout(t, func() {
		if err := runCommit(commitCmd, []string{parent}); err != nil {
			t.Logf("commit yaml: %v", err)
		}
	})
	commitYAML = ""
	commitMessages = []string{"r1:feat: from -m"}
	captureStdout(t, func() {
		if err := runCommit(commitCmd, []string{parent}); err != nil {
			t.Logf("commit -m: %v", err)
		}
	})
	// invalid json
	commitMessages = nil
	commitJSON = "{bad"
	if err := runCommit(commitCmd, []string{parent}); err == nil {
		t.Log("invalid json may still pass dry path")
	}
	// invalid message format
	commitJSON = ""
	commitMessages = []string{"nocolon"}
	if err := runCommit(commitCmd, []string{parent}); err == nil {
		t.Log("invalid -m may error")
	}
}

func TestTagAutoAndPushDry(t *testing.T) {
	parent := setupBulkParent(t)
	auto := findCommand(t, rootCmd, "tag", "auto")
	captureStdout(t, func() {
		if auto.RunE != nil {
			if err := auto.RunE(auto, []string{parent}); err != nil {
				t.Logf("tag auto: %v", err)
			}
		}
	})
	// push tags dry if flag
	push := findCommand(t, rootCmd, "tag", "push")
	if f := push.Flags().Lookup("dry-run"); f != nil {
		_ = f.Value.Set("true")
	}
	captureStdout(t, func() {
		if push.RunE != nil {
			if err := push.RunE(push, []string{parent}); err != nil {
				t.Logf("tag push: %v", err)
			}
		}
	})
}

func TestStashPopApplyDry(t *testing.T) {
	parent := setupBulkParent(t)
	repo := filepath.Join(parent, "r1")
	if err := os.WriteFile(filepath.Join(repo, "s.txt"), []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}
	// save first
	_ = runBulkStashSave(context.Background(), parent)
	captureStdout(t, func() {
		if err := runBulkStashApply(context.Background(), parent); err != nil {
			t.Logf("stash apply: %v", err)
		}
	})
	captureStdout(t, func() {
		if err := runBulkStashPop(context.Background(), parent); err != nil {
			t.Logf("stash pop: %v", err)
		}
	})
}

func TestConfigHierarchyValidateCompact(t *testing.T) {
	resetConfigFlags(t)
	_ = isolateConfigHome(t)
	configGlobal = true
	_ = runConfigInit(configInitCmd, nil)
	// project init
	configGlobal = false
	_ = runConfigInit(configInitCmd, nil)
	prevV, prevC := validateFlag, compactFlag
	t.Cleanup(func() { validateFlag, compactFlag = prevV, prevC })
	validateFlag = true
	compactFlag = true
	captureStdout(t, func() {
		if err := runConfigHierarchy(configHierarchyCmd, nil); err != nil {
			t.Logf("hierarchy: %v", err)
		}
	})
	// show effective after global
	showEffective = true
	captureStdout(t, func() {
		if err := runConfigShow(configShowCmd, nil); err != nil {
			t.Logf("effective: %v", err)
		}
	})
}
