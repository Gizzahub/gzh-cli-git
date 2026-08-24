// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunMakeTarget_LintUsesIsolatedTemporaryCache(t *testing.T) {
	dir := t.TempDir()
	capture := filepath.Join(dir, "lint-cache")
	writeRepoFile(t, dir, "Makefile", "lint:\n\t@test -n \"$$GOLANGCI_LINT_CACHE\"\n\t@printf '%s' \"$$GOLANGCI_LINT_CACHE\" > \"$$CAPTURE_FILE\"\ncheck:\n\t@test -z \"$$GOLANGCI_LINT_CACHE\"\n")
	t.Setenv("CAPTURE_FILE", capture)
	t.Setenv("GOLANGCI_LINT_CACHE", "")

	first := runMakeTarget(context.Background(), dir, "lint")
	if first.Err != nil {
		t.Fatalf("first lint: %v\n%s", first.Err, first.Output)
	}
	firstCache, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("read first cache: %v", err)
	}
	if _, err := os.Stat(string(firstCache)); !os.IsNotExist(err) {
		t.Fatalf("first cache %q must be removed after lint, stat err = %v", firstCache, err)
	}

	second := runMakeTarget(context.Background(), dir, "lint")
	if second.Err != nil {
		t.Fatalf("second lint: %v\n%s", second.Err, second.Output)
	}
	secondCache, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("read second cache: %v", err)
	}
	if bytes.Equal(firstCache, secondCache) {
		t.Fatalf("lint cache reused %q", firstCache)
	}
	if _, err := os.Stat(string(secondCache)); !os.IsNotExist(err) {
		t.Fatalf("second cache %q must be removed after lint, stat err = %v", secondCache, err)
	}

	check := runMakeTarget(context.Background(), dir, "check")
	if check.Err != nil {
		t.Fatalf("check must not receive lint cache: %v\n%s", check.Err, check.Output)
	}
}

func TestForeignDiagnosticLocations(t *testing.T) {
	got := foreignDiagnosticLocations("../old-worktree/pkg/check.go:12: stale\n./local/file.go:3: allowed\nlocal/other.go:4: allowed\n")
	if len(got) != 1 || got[0] != "../old-worktree/pkg/check.go:12" {
		t.Fatalf("foreign locations = %v", got)
	}

	got = foreignDiagnosticLocations("./../old-worktree/pkg/check.go:12: stale\n")
	if len(got) != 1 || got[0] != "./../old-worktree/pkg/check.go:12" {
		t.Fatalf("dot-prefixed foreign locations = %v", got)
	}
}

func TestJudgeMake_ForeignBranchDiagnosticFailsDespiteSuccessfulExit(t *testing.T) {
	item := judgeMake(context.Background(), gitRepo{}, TargetPlan{}, makeProbe{
		Target:  "lint",
		Defined: true,
		Output:  "../deleted-worktree/pkg/check.go:12: stale\n",
	}, false)
	if item.Status != checkFail || !strings.Contains(item.Detail, "branch diagnostics reference paths outside the repository") {
		t.Fatalf("successful branch probe with foreign diagnostic = %+v", item)
	}
}

func TestForeignDiagnosticError_InvalidatesBranchAndBaseline(t *testing.T) {
	output := "../deleted-worktree/pkg/check.go:12: stale\n"
	for _, scope := range []string{"branch", "baseline"} {
		err := foreignDiagnosticError(scope, output)
		if err == nil || !strings.Contains(err.Error(), scope+" diagnostics reference paths outside the repository") {
			t.Fatalf("%s foreign diagnostics must invalidate the verdict, err = %v", scope, err)
		}
	}
}
