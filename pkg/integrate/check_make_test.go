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
	inheritedCache := filepath.Join(dir, "inherited-lint-cache")
	t.Setenv("GOLANGCI_LINT_CACHE", inheritedCache)

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
	if string(firstCache) == inheritedCache {
		t.Fatalf("lint reused inherited cache %q", inheritedCache)
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

	t.Setenv("GOLANGCI_LINT_CACHE", "")
	check := runMakeTarget(context.Background(), dir, "check")
	if check.Err != nil {
		t.Fatalf("check must not receive lint cache: %v\n%s", check.Err, check.Output)
	}
}

func TestRunMakeTarget_LintCleansCacheAfterFailure(t *testing.T) {
	dir := t.TempDir()
	capture := filepath.Join(dir, "lint-cache")
	writeRepoFile(t, dir, "Makefile", "lint:\n\t@printf '%s' \"$$GOLANGCI_LINT_CACHE\" > \"$$CAPTURE_FILE\"\n\t@false\n")
	t.Setenv("CAPTURE_FILE", capture)

	probe := runMakeTarget(context.Background(), dir, "lint")
	if probe.Err == nil {
		t.Fatal("failing lint must return an error")
	}
	cache, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("read lint cache: %v", err)
	}
	if _, err := os.Stat(string(cache)); !os.IsNotExist(err) {
		t.Fatalf("failed lint cache %q must be removed, stat err = %v", cache, err)
	}
}

func TestForeignDiagnosticLocations(t *testing.T) {
	got := foreignDiagnosticLocations("../old-worktree/pkg/check.go:12: stale\n./local/file.go:3: allowed\nlocal/other.go:4: allowed\nsubdir/../../outside.go:8: stale\n././../other-outside.go:9: stale\n/absolute/path.go:10: stale\n")
	want := []string{"../old-worktree/pkg/check.go:12", "././../other-outside.go:9", "/absolute/path.go:10", "subdir/../../outside.go:8"}
	if !bytes.Equal([]byte(strings.Join(got, "\n")), []byte(strings.Join(want, "\n"))) {
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

func TestJudgeMake_CheckAllowsForeignDiagnosticOutput(t *testing.T) {
	item := judgeMake(context.Background(), gitRepo{}, TargetPlan{}, makeProbe{
		Target:  "check",
		Defined: true,
		Output:  "../other-worktree/pkg/check.go:12: diagnostic\n",
	}, false)
	if item.Status != checkPass {
		t.Fatalf("check output must preserve existing behavior, got %+v", item)
	}
}

func TestJudgeMake_MissingCDFailsEvenWhenSkippedChecksAreAllowed(t *testing.T) {
	for _, allowSkipped := range []bool{false, true} {
		item := judgeMake(context.Background(), gitRepo{}, TargetPlan{}, makeProbe{
			Target:    "lint",
			Defined:   true,
			MissingCD: "missing-component",
		}, allowSkipped)
		if item.Status != checkFail || !strings.Contains(item.Detail, "not run") {
			t.Fatalf("allowSkipped=%v missing cd = %+v", allowSkipped, item)
		}
	}
}

func TestMissingCD(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want string
	}{
		{
			name: "bsd GNU form with shell prefix",
			out:  "/bin/sh: cd: missing-component: No such file or directory\n",
			want: "missing-component",
		},
		{
			name: "dash form with numbered shell prefix",
			out:  "/bin/sh: 1: cd: can't cd to missing-component\n",
			want: "missing-component",
		},
		{
			name: "dash form keeps spaces in the component path",
			out:  "dash: 1: cd: can't cd to missing component/path\n",
			want: "missing component/path",
		},
		{
			name: "dash form trims presentation spacing around path",
			out:  "/bin/sh: 1: cd: can't cd to   missing-component   \n",
			want: "missing-component",
		},
		{
			name: "similar wording is not a shell cd error",
			out:  "tool: cd: cannot cd to missing-component\n",
		},
		{
			name: "non shell prefix is not a dash cd error",
			out:  "tool: 1: cd: can't cd to missing-component\n",
		},
		{
			name: "empty dash path is rejected",
			out:  "/bin/sh: 1: cd: can't cd to \n",
		},
		{
			name: "empty POSIX path is rejected",
			out:  "cd: : No such file or directory\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := missingCD(tt.out); got != tt.want {
				t.Fatalf("missingCD(%q) = %q, want %q", tt.out, got, tt.want)
			}
		})
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
