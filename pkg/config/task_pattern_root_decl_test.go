// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// TestLoadRepoRootTaskPattern_ThisRepoDeclaration guards this repository's own
// repo-root declaration. The post-integrate RECLAIM machinery stays dormant
// until LoadRepoRootTaskPattern finds a repo-root taskPattern, so deleting or
// loosening the declaration silently disables reclaim. The repo root is
// resolved from this test file's location, never from the working directory.
func TestLoadRepoRootTaskPattern_ThisRepoDeclaration(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	decl, err := LoadRepoRootTaskPattern(repoRoot)
	if err != nil {
		t.Fatalf("LoadRepoRootTaskPattern(%s): %v", repoRoot, err)
	}
	if len(decl.Patterns) == 0 {
		t.Fatalf("no taskPattern declaration: facts %v", decl.Facts)
	}

	wantIntegration := []string{"develop"}
	if got := []string(decl.IntegrationBranch); !reflect.DeepEqual(got, wantIntegration) {
		t.Fatalf("integrationBranch = %v, want %v", got, wantIntegration)
	}

	mustMatch := []string{
		"feat/x",
		"test/info-branch-cell-colors",
		"agent/task/hermes-01", // agent/* must cover the multi-segment shape
	}
	for _, name := range mustMatch {
		if !MatchesAnyTaskPattern(name, decl.Patterns) {
			t.Errorf("%s must match taskPattern %v", name, decl.Patterns)
		}
	}

	mustNotMatch := []string{"develop", "master", "main"}
	for _, name := range mustNotMatch {
		if MatchesAnyTaskPattern(name, decl.Patterns) {
			t.Errorf("%s must not match taskPattern %v", name, decl.Patterns)
		}
	}
}
