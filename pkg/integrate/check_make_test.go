// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestRunMakeTarget_LintRetriesGlobalLock(t *testing.T) {
	dir := t.TempDir()
	count := filepath.Join(dir, "attempts")
	writeRepoFile(t, dir, "Makefile", "lint:\n\t@count=0; if test -f \"$$COUNT_FILE\"; then count=$$(cat \"$$COUNT_FILE\"); fi; count=$$((count + 1)); printf '%s' \"$$count\" > \"$$COUNT_FILE\"; if test \"$$count\" -lt 3; then printf '%s\\n' 'Error: parallel golangci-lint is running'; false; fi\n")
	t.Setenv("COUNT_FILE", count)

	probe := runMakeTarget(context.Background(), dir, "lint")
	if probe.Err != nil {
		t.Fatalf("lint must succeed after transient global lock: %v\n%s", probe.Err, probe.Output)
	}
	got, err := os.ReadFile(count)
	if err != nil {
		t.Fatalf("read attempts: %v", err)
	}
	if string(got) != "3" {
		t.Fatalf("attempts = %q, want 3", got)
	}
}

func TestRunMakeTarget_LintLockFailsAfterRetries(t *testing.T) {
	dir := t.TempDir()
	count := filepath.Join(dir, "attempts")
	writeRepoFile(t, dir, "Makefile", "lint:\n\t@printf x >> \"$$COUNT_FILE\"; printf '%s\\n' 'Error: parallel golangci-lint is running'; false\n")
	t.Setenv("COUNT_FILE", count)

	probe := runMakeTarget(context.Background(), dir, "lint")
	if probe.Err == nil {
		t.Fatal("locked lint must fail")
	}
	got, err := os.ReadFile(count)
	if err != nil {
		t.Fatalf("read attempts: %v", err)
	}
	if string(got) != "xxx" {
		t.Fatalf("attempts = %q, want three runs", got)
	}
}

func TestRunMakeTarget_LintLockWithDiagnosticDoesNotRetryOrSkip(t *testing.T) {
	dir := t.TempDir()
	count := filepath.Join(dir, "attempts")
	writeRepoFile(t, dir, "Makefile", "lint:\n\t@printf x >> \"$$COUNT_FILE\"; printf '%s\\n' 'Error: parallel golangci-lint is running' 'main.go:12:3: actual diagnostic'; false\n")
	t.Setenv("COUNT_FILE", count)

	probe := runMakeTarget(context.Background(), dir, "lint")
	if probe.Err == nil {
		t.Fatalf("mixed lock and diagnostic must remain a failure: %+v", probe)
	}
	got, err := os.ReadFile(count)
	if err != nil {
		t.Fatalf("read attempts: %v", err)
	}
	if string(got) != "x" {
		t.Fatalf("mixed output retried %q, want one run", got)
	}
}

func TestGolangciLintLocked_RejectsOtherFailureOutput(t *testing.T) {
	if golangciLintLocked("Error: parallel golangci-lint is running\nother tool failed\n") {
		t.Fatal("lock text mixed with another failure must not be retried or skipped")
	}
}

func TestGolangciLintLocked_ObservedMakeOutputWithANSI(t *testing.T) {
	output := "\x1b[36mRunning golangci-lint...\x1b[0m\n" +
		"GOWORK=off golangci-lint run ./...\n" +
		"golangci-lint run ./...\n" +
		"Error: parallel golangci-lint is running\n" +
		"make[1]: *** [Makefile:165: lint] Error 3\n"
	if !golangciLintLocked(output) {
		t.Fatalf("observed pure lock output was not recognized:\n%s", output)
	}
}

func TestGolangciLintLocked_MakeWrapperTargets(t *testing.T) {
	for _, line := range []string{
		"make[1]: *** [Makefile:165: lint] Error 3",
		"make[2]: *** [.make/quality.mk:165: lint-check] Error 3",
		"make: *** [lint] Error 3",
		"make: *** [lint-check] Error 3",
		"-e Running golangci-lint...",
	} {
		output := "Running golangci-lint...\nGOWORK=off golangci-lint run ./...\nError: parallel golangci-lint is running\n" + line + "\n"
		if !golangciLintLocked(output) {
			t.Fatalf("known lock wrapper was rejected: %q", line)
		}
	}
	for _, line := range []string{
		"make[1]: *** [Makefile:165: lint-extra] Error 3",
		"make[1]: *** [Makefile:165: check] Error 3",
		"make[1]: *** [Makefile:165: lint] Error nope",
		"make[1]: *** [Makefile:165: lint] other failure",
	} {
		output := "Running golangci-lint...\nError: parallel golangci-lint is running\n" + line + "\n"
		if golangciLintLocked(output) {
			t.Fatalf("non-lint wrapper was accepted: %q", line)
		}
	}
}

func TestWaitForRetry_ContextCancellationWins(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitForRetry(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForRetry error = %v, want context cancellation", err)
	}
}

func TestRunMakeTarget_LintCancellationDuringFinalLockAttemptIsNotUnavailable(t *testing.T) {
	dir := t.TempDir()
	count := filepath.Join(dir, "attempts")
	started := filepath.Join(dir, "third-attempt-started")
	writeRepoFile(t, dir, "Makefile", "lint:\n\t@count=0; if test -f \"$$COUNT_FILE\"; then count=$$(cat \"$$COUNT_FILE\"); fi; count=$$((count + 1)); printf '%s' \"$$count\" > \"$$COUNT_FILE\"; if test \"$$count\" -eq 3; then : > \"$$THIRD_ATTEMPT_FILE\"; sleep 1; fi; printf '%s\\n' 'Error: parallel golangci-lint is running'; false\n")
	t.Setenv("COUNT_FILE", count)
	t.Setenv("THIRD_ATTEMPT_FILE", started)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(started); err == nil {
				cancel()
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	probe := runMakeTarget(ctx, dir, "lint")
	if !errors.Is(probe.Err, context.Canceled) {
		t.Fatalf("canceled final lock attempt error = %v, want context.Canceled", probe.Err)
	}
	got, err := os.ReadFile(count)
	if err != nil {
		t.Fatalf("read attempts: %v", err)
	}
	if string(got) != "3" {
		t.Fatalf("attempts = %q, want cancellation during third run", got)
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
