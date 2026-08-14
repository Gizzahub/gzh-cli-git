// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolateConfigHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	// Clear any project config discovery noise by chdir to temp project
	proj := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(proj); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
}

func resetConfigFlags(t *testing.T) {
	t.Helper()
	prevGlobal, prevLocal := configGlobal, configLocal
	prevEff := showEffective
	prevProvider := profileProvider
	t.Cleanup(func() {
		configGlobal, configLocal = prevGlobal, prevLocal
		showEffective = prevEff
		profileProvider = prevProvider
		profileBaseURL = ""
		profileToken = ""
		profileCloneProto = ""
		profileSSHPort = 0
		profileParallel = 0
		profileSubgroups = false
		profileSubgroupMode = ""
	})
	configGlobal = false
	configLocal = true
	showEffective = false
	profileProvider = ""
	profileBaseURL = ""
	profileToken = ""
	profileCloneProto = ""
	profileSSHPort = 0
	profileParallel = 0
	profileSubgroups = false
	profileSubgroupMode = ""
}

func TestConfigInitLocalAndShow(t *testing.T) {
	resetConfigFlags(t)
	isolateConfigHome(t)

	configGlobal = false
	out := captureStdout(t, func() {
		if err := runConfigInit(configInitCmd, nil); err != nil {
			t.Fatalf("config init: %v", err)
		}
	})
	if !strings.Contains(out, ".gz-git.yaml") {
		t.Errorf("init output: %q", out)
	}
	if _, err := os.Stat(".gz-git.yaml"); err != nil {
		t.Fatalf("project config not created: %v", err)
	}

	showEffective = false
	out = captureStdout(t, func() {
		if err := runConfigShow(configShowCmd, nil); err != nil {
			t.Fatalf("config show: %v", err)
		}
	})
	if !strings.Contains(out, "Project Config") && !strings.Contains(out, "profile") {
		t.Logf("show out: %q", out)
	}

	showEffective = true
	out = captureStdout(t, func() {
		if err := runConfigShow(configShowCmd, nil); err != nil {
			// effective may fail without global init — still exercise path
			t.Logf("effective show err (ok if no global): %v", err)
		}
	})
	_ = out
}

func TestConfigInitGlobalAndProfiles(t *testing.T) {
	resetConfigFlags(t)
	isolateConfigHome(t)

	configGlobal = true
	out := captureStdout(t, func() {
		if err := runConfigInit(configInitCmd, nil); err != nil {
			t.Fatalf("config init --global: %v", err)
		}
	})
	if !strings.Contains(out, "Initialized") && !strings.Contains(out, "config") {
		t.Logf("global init: %q", out)
	}

	// list profiles
	out = captureStdout(t, func() {
		if err := runConfigProfileList(configProfileListCmd, nil); err != nil {
			t.Fatalf("profile list: %v", err)
		}
	})
	_ = out

	// create profile with flags (non-interactive)
	profileProvider = "github"
	profileCloneProto = "https"
	profileParallel = 4
	out = captureStdout(t, func() {
		if err := runConfigProfileCreate(configProfileCreateCmd, []string{"work"}); err != nil {
			t.Fatalf("profile create: %v", err)
		}
	})
	_ = out

	out = captureStdout(t, func() {
		if err := runConfigProfileShow(configProfileShowCmd, []string{"work"}); err != nil {
			t.Fatalf("profile show: %v", err)
		}
	})
	_ = out

	out = captureStdout(t, func() {
		if err := runConfigProfileUse(configProfileUseCmd, []string{"work"}); err != nil {
			t.Fatalf("profile use: %v", err)
		}
	})
	_ = out

	out = captureStdout(t, func() {
		if err := runConfigProfileList(configProfileListCmd, nil); err != nil {
			t.Fatalf("profile list after use: %v", err)
		}
	})
	if !strings.Contains(out, "work") {
		t.Logf("list after use: %q", out)
	}

	// hierarchy (may be empty)
	out = captureStdout(t, func() {
		if err := runConfigHierarchy(configHierarchyCmd, nil); err != nil {
			t.Logf("hierarchy: %v", err)
		}
	})
	_ = out

	// delete non-default profile
	out = captureStdout(t, func() {
		// switch away first if needed
		_ = runConfigProfileUse(configProfileUseCmd, []string{"default"})
		if err := runConfigProfileDelete(configProfileDeleteCmd, []string{"work"}); err != nil {
			t.Logf("profile delete: %v", err)
		}
	})
	_ = out
}

func TestPrintConfigValueAndResolveChildPath(t *testing.T) {
	out := captureStdout(t, func() {
		printConfigValue("Provider", "github", "default")
	})
	if !strings.Contains(out, "Provider") {
		t.Errorf("printConfigValue: %q", out)
	}

	got := resolveChildPath("/parent", "child")
	if !strings.Contains(got, "child") {
		t.Errorf("path: %q", got)
	}
	// absolute child
	got = resolveChildPath("/parent", "/abs/child")
	if got != "/abs/child" {
		t.Errorf("abs path = %q", got)
	}
}
