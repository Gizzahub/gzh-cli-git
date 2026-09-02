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
	if probe.Unavailable == "" {
		t.Fatal("exhausted pure lint lock must report measurement unavailable")
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
	if probe.Unavailable != "" {
		t.Fatalf("canceled final lock attempt unavailable = %q, want empty", probe.Unavailable)
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

func TestLexicallyForeignDiagnosticPath_PreservesLegacyAndSlashForms(t *testing.T) {
	for _, path := range []string{"../old-worktree/pkg/x.go", "./../old-worktree/pkg/x.go", "subdir/../../outside.go", "/absolute/x.go"} {
		if !isLexicallyForeignDiagnosticPath(path) {
			t.Fatalf("%q must be lexically foreign", path)
		}
	}
	if isLexicallyForeignDiagnosticPath("subdir/../file.go") {
		t.Fatal("legacy internal subdir/../file.go must not become foreign")
	}
}

func TestForeignDiagnosticLocationsForProbe_AllowsOnlyCapturedGoRootSrc(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	goRoot := filepath.Join(root, "go-real")
	src := filepath.Join(goRoot, "src")
	allowed := filepath.Join(src, "runtime", "runtime.go")
	other := filepath.Join(root, "other", "outside.go")
	prefixCollision := filepath.Join(root, "go-real", "src-sibling", "bad.go")
	if err := os.MkdirAll(filepath.Dir(allowed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(other), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(prefixCollision), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{allowed, other, prefixCollision} {
		if err := os.WriteFile(path, []byte("fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	relAllowed, err := filepath.Rel(work, allowed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "other"), filepath.Join(src, "escape")); err != nil {
		t.Fatal(err)
	}
	canonicalSrc, err := filepath.EvalSymlinks(src)
	if err != nil {
		t.Fatal(err)
	}
	probe := makeProbe{WorkDir: work, GoRootSrc: canonicalSrc, ControllerPrepared: true, Output: strings.Join([]string{
		relAllowed + ":1: relative standard library",
		allowed + ":2: absolute standard library",
		other + ":3: other external file",
		prefixCollision + ":4: prefix collision",
		filepath.Join(src, "escape", "outside.go") + ":5: symlink escape",
	}, "\n")}
	probe = freezeGoRootDiagnosticApprovals(probe)
	got := foreignDiagnosticLocationsForProbe(probe)
	want := []string{
		prefixCollision + ":4",
		filepath.Join(src, "escape", "outside.go") + ":5",
		other + ":3",
	}
	if !bytes.Equal([]byte(strings.Join(got, "\n")), []byte(strings.Join(want, "\n"))) {
		t.Fatalf("foreign locations = %v, want %v", got, want)
	}

	probe.GoRootSrc = ""
	probe.ApprovedForeignLocations = nil
	if got := foreignDiagnosticLocationsForProbe(probe); len(got) != 5 {
		t.Fatalf("no captured allowance must reject every external diagnostic, got %v", got)
	}
	probe.GoRootSrc = canonicalSrc
	probe.ControllerPrepared = false
	if got := foreignDiagnosticLocationsForProbe(probe); len(got) != 5 {
		t.Fatalf("legacy probe must not consume a controller allowance, got %v", got)
	}
}

func TestAnnotateControllerPreparedProbe_DiscoveryFailureLeavesNoAllowance(t *testing.T) {
	probe := annotateControllerPreparedProbe(context.Background(), makeProbe{
		WorkDir: filepath.Join(t.TempDir(), "does-not-exist"),
		Output:  "/outside/file.go:1: diagnostic",
	})
	if probe.GoRootSrc != "" {
		t.Fatalf("failed discovery allowance = %q, want empty", probe.GoRootSrc)
	}
	if got := foreignDiagnosticLocationsForProbe(probe); len(got) != 1 {
		t.Fatalf("failed discovery must reject external diagnostic, got %v", got)
	}
}

func TestFreezeGoRootDiagnosticApprovals_ResolvesSymlinksBeforeDotDotAndSurvivesTargetRemoval(t *testing.T) {
	root := t.TempDir()
	outerWork := filepath.Join(root, "outer", "work")
	innerWork := filepath.Join(root, "inner", "work")
	src := filepath.Join(root, "inner", "goroot", "src")
	stdlib := filepath.Join(src, "runtime", "runtime.go")
	if err := os.MkdirAll(outerWork, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(innerWork, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(stdlib), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stdlib, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(innerWork, filepath.Join(outerWork, "step")); err != nil {
		t.Fatal(err)
	}
	canonicalSrc, err := filepath.EvalSymlinks(src)
	if err != nil {
		t.Fatal(err)
	}
	// Lexically cleaning this path would use outer/work/goroot. Component-order
	// resolution first follows step to inner/work, then applies "..".
	probe := freezeGoRootDiagnosticApprovals(makeProbe{
		WorkDir:            outerWork,
		GoRootSrc:          canonicalSrc,
		ControllerPrepared: true,
		Output:             "step/../goroot/src/runtime/runtime.go:1: toolchain",
	})
	if len(probe.ApprovedForeignLocations) != 1 {
		t.Fatalf("symlink then dot-dot diagnostic was not approved: %#v", probe)
	}

	workLink := filepath.Join(root, "work-link")
	if err := os.Symlink(innerWork, workLink); err != nil {
		t.Fatal(err)
	}
	symlinkedWorkProbe := freezeGoRootDiagnosticApprovals(makeProbe{
		WorkDir:            workLink,
		GoRootSrc:          canonicalSrc,
		ControllerPrepared: true,
		Output:             "../goroot/src/runtime/runtime.go:2: toolchain",
	})
	if len(symlinkedWorkProbe.ApprovedForeignLocations) != 1 {
		t.Fatalf("symlinked worktree diagnostic was not approved: %#v", symlinkedWorkProbe)
	}
	if err := os.Symlink(src, filepath.Join(outerWork, "stdlib-link")); err != nil {
		t.Fatal(err)
	}
	relativeSymlinkProbe := freezeGoRootDiagnosticApprovals(makeProbe{
		WorkDir:            outerWork,
		GoRootSrc:          canonicalSrc,
		ControllerPrepared: true,
		Output:             "stdlib-link/runtime/runtime.go:3: toolchain",
	})
	if len(relativeSymlinkProbe.ApprovedForeignLocations) != 1 {
		t.Fatalf("relative symlink into GOROOT was not approved: %#v", relativeSymlinkProbe)
	}
	if isLexicallyForeignDiagnosticPath("stdlib-link/runtime/runtime.go") {
		t.Fatal("relative symlink candidate must not alter legacy foreign classification")
	}
	if got := extractLocationsForProbe(relativeSymlinkProbe, nil); len(got) != 0 {
		t.Fatalf("relative symlink GOROOT location must be excluded from counts, got %v", got)
	}

	if err := os.RemoveAll(filepath.Join(root, "inner")); err != nil {
		t.Fatal(err)
	}
	if got := foreignDiagnosticLocationsForProbe(probe); len(got) != 0 {
		t.Fatalf("frozen target approval must survive target deletion, got %v", got)
	}
	if got := extractLocationsForProbe(probe, nil); len(got) != 0 {
		t.Fatalf("frozen target approval must stay out of baseline counts, got %v", got)
	}
}

func TestAnnotateControllerPreparedProbe_CapturesCanonicalGoRootSrc(t *testing.T) {
	probe := annotateControllerPreparedProbe(context.Background(), makeProbe{WorkDir: t.TempDir()})
	if !probe.ControllerPrepared {
		t.Fatal("controller preparation marker was not retained")
	}
	if probe.GoRootSrc == "" {
		t.Fatal("go env GOROOT did not produce an allowance")
	}
	if got, err := filepath.EvalSymlinks(probe.GoRootSrc); err != nil || got != probe.GoRootSrc {
		t.Fatalf("GoRootSrc must itself be canonical: got %q canonical %q err=%v", probe.GoRootSrc, got, err)
	}
	if info, err := os.Stat(probe.GoRootSrc); err != nil || !info.IsDir() {
		t.Fatalf("GoRootSrc must be a directory: info=%v err=%v", info, err)
	}
}

func TestExtractLocationsForProbe_ExcludesAllowedGoRootLocationsFromCounts(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	src := filepath.Join(root, "goroot", "src")
	stdlib := filepath.Join(src, "runtime", "runtime.go")
	if err := os.MkdirAll(filepath.Dir(stdlib), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stdlib, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	canonicalSrc, err := filepath.EvalSymlinks(src)
	if err != nil {
		t.Fatal(err)
	}
	probe := makeProbe{
		WorkDir:            work,
		GoRootSrc:          canonicalSrc,
		ControllerPrepared: true,
		Output:             stdlib + ":1: toolchain\nrepo.go:2: repository",
	}
	probe = freezeGoRootDiagnosticApprovals(probe)
	// The GOROOT line is dropped and the repository line is kept -- the subject
	// of this test. The kept line is unattributed because "repo.go" carries no
	// directory; knowing make ran in probe.WorkDir does not restore one, since
	// the printing tool stripped it whatever the working directory was.
	if got, want := extractLocationsForProbe(probe, []string{"repo.go"}), []string{unattributedPrefix + "repo.go:2"}; !bytes.Equal([]byte(strings.Join(got, "\n")), []byte(strings.Join(want, "\n"))) {
		t.Fatalf("controller baseline locations = %v, want %v", got, want)
	}
	probe.ControllerPrepared = false
	if got := extractLocationsForProbe(probe, []string{"repo.go"}); len(got) != 2 {
		t.Fatalf("legacy extraction must preserve GOROOT location, got %v", got)
	}
}

func TestForeignDiagnosticLocationsForProbe_TargetAndSourceKeepDistinctAllowances(t *testing.T) {
	root := t.TempDir()
	makeProbeFor := func(name string) (makeProbe, string) {
		work := filepath.Join(root, name, "work")
		src := filepath.Join(root, name, "goroot", "src")
		file := filepath.Join(src, "runtime", "runtime.go")
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(work, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
		canonicalSrc, err := filepath.EvalSymlinks(src)
		if err != nil {
			t.Fatal(err)
		}
		probe := freezeGoRootDiagnosticApprovals(makeProbe{WorkDir: work, GoRootSrc: canonicalSrc, ControllerPrepared: true, Output: file + ":1: diagnostic"})
		return probe, file
	}
	target, targetFile := makeProbeFor("target")
	source, sourceFile := makeProbeFor("source")
	if got := foreignDiagnosticLocationsForProbe(target); len(got) != 0 {
		t.Fatalf("target's own allowance rejected %s: %v", targetFile, got)
	}
	if got := foreignDiagnosticLocationsForProbe(source); len(got) != 0 {
		t.Fatalf("source's own allowance rejected %s: %v", sourceFile, got)
	}
	target.Output = sourceFile + ":1: wrong probe root"
	if got := foreignDiagnosticLocationsForProbe(target); len(got) != 1 {
		t.Fatalf("target must not inherit source allowance, got %v", got)
	}
}

func TestJudgeMake_ForeignBranchDiagnosticFailsDespiteSuccessfulExit(t *testing.T) {
	item := judgeMakeLegacy(context.Background(), gitRepo{}, TargetPlan{}, makeProbe{
		Target:  "lint",
		Defined: true,
		Output:  "../deleted-worktree/pkg/check.go:12: stale\n",
	}, false)
	if item.Status != checkFail || !strings.Contains(item.Detail, "branch diagnostics reference paths outside the repository") {
		t.Fatalf("successful branch probe with foreign diagnostic = %+v", item)
	}
}

func TestJudgeMake_CheckAllowsForeignDiagnosticOutput(t *testing.T) {
	item := judgeMakeLegacy(context.Background(), gitRepo{}, TargetPlan{}, makeProbe{
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
		item := judgeMakeLegacy(context.Background(), gitRepo{}, TargetPlan{}, makeProbe{
			Target:    "lint",
			Defined:   true,
			MissingCD: "missing-component",
		}, allowSkipped)
		if item.Status != checkFail || !strings.Contains(item.Detail, "not run") {
			t.Fatalf("allowSkipped=%v missing cd = %+v", allowSkipped, item)
		}
	}
}

func TestJudgeMake_UnavailableFailsBeforeBaselineEvenWhenSkippedChecksAreAllowed(t *testing.T) {
	for _, allowSkipped := range []bool{false, true} {
		item := judgeMakeLegacy(context.Background(), gitRepo{}, TargetPlan{}, makeProbe{
			Target: "lint", Defined: true, Unavailable: "golangci-lint lock persisted after 3 attempts",
		}, allowSkipped)
		if item.Status != checkFail || !strings.Contains(item.Detail, "measurement unavailable") {
			t.Fatalf("allowSkipped=%v unavailable = %+v", allowSkipped, item)
		}
	}
}

func TestJudgeMakeAgainstProbeRejectsBaselineMissingCD(t *testing.T) {
	item := judgeMakeAgainstProbe(context.Background(), gitRepo{}, TargetPlan{}, makeProbe{
		Target: "lint", Defined: true, Err: errors.New("branch lint failed"), Code: 1,
	}, false, makeProbe{
		Target: "lint", Defined: true, Err: errors.New("baseline lint failed"), MissingCD: "ent/generated",
	})
	if item.Status != checkFail || !strings.Contains(item.Detail, "baseline make lint did not run") {
		t.Fatalf("baseline missing cd = %+v", item)
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

// panicLintOutput reproduces the shape that made this class of failure
// unreadable: make runs the linter, the linter panics, and every stack frame
// carries a file.go:line token from outside the repository. Before crash
// detection the gate reported these frames as foreign diagnostics, so the
// operator was told the tree was dirty and never saw the panic.
const panicLintOutput = "Running golangci-lint...\n" +
	"panic: runtime error: invalid memory address or nil pointer dereference\n" +
	"[signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x104a1b2c8]\n" +
	"\n" +
	"goroutine 1 [running]:\n" +
	"github.com/golangci/golangci-lint/pkg/lint.(*Runner).Run(0x14000123400)\n" +
	"\t/Users/build/go/pkg/mod/github.com/golangci/golangci-lint/pkg/lint/runner.go:112 +0x88\n" +
	"main.main()\n" +
	"\t/Users/build/go/pkg/mod/github.com/golangci/golangci-lint/cmd/golangci-lint/main.go:24 +0x1c\n" +
	"make: *** [lint] Error 2\n"

func TestToolCrashSignature_DistinguishesCrashFromFindings(t *testing.T) {
	if got := toolCrashSignature(panicLintOutput); !strings.HasPrefix(got, "panic: runtime error:") {
		t.Fatalf("panic output signature = %q", got)
	}
	// A tool that merely names the word in a finding has not crashed. Anchoring
	// at the start of the line is what keeps this apart.
	ordinary := "pkg/x/y.go:12:3: do not panic: use an error instead (revive)\nmake: *** [lint] Error 1\n"
	if got := toolCrashSignature(ordinary); got != "" {
		t.Fatalf("ordinary finding must not read as a crash, got %q", got)
	}
	if got := toolCrashSignature(""); got != "" {
		t.Fatalf("empty output = %q", got)
	}
}

func TestToolCrashSignature_SurvivesANSIColouring(t *testing.T) {
	colored := "\x1b[36mRunning golangci-lint...\x1b[0m\n\x1b[31mfatal error: concurrent map writes\x1b[0m\n"
	if got := toolCrashSignature(colored); got != "fatal error: concurrent map writes" {
		t.Fatalf("ANSI-wrapped crash signature = %q", got)
	}
}

// TestJudgeMake_CrashIsReportedAsToolFailureNotForeignDiagnostics is the
// regression this change exists for. Both judge paths must reach the crash
// verdict before the foreign-diagnostic classifier consumes the stack frames.
func TestJudgeMake_CrashIsReportedAsToolFailureNotForeignDiagnostics(t *testing.T) {
	probe := makeProbe{
		Target:    "lint",
		Defined:   true,
		Output:    panicLintOutput,
		Err:       errors.New("exit status 2"),
		Code:      2,
		ToolCrash: toolCrashSignature(panicLintOutput),
	}

	legacy := judgeMakeLegacy(context.Background(), gitRepo{}, TargetPlan{}, probe, false)
	against := judgeMakeAgainstProbe(context.Background(), gitRepo{}, TargetPlan{}, probe, false,
		makeProbe{Target: "lint", Defined: true})

	for name, item := range map[string]CheckItem{"legacy": legacy, "against-probe": against} {
		if item.Status != checkFail {
			t.Fatalf("%s: crash must fail the check, got %+v", name, item)
		}
		if strings.Contains(item.Detail, "diagnostics reference paths outside the repository") {
			t.Fatalf("%s: crash was still reported as foreign diagnostics: %s", name, item.Detail)
		}
		if !strings.Contains(item.Detail, "terminated abnormally") {
			t.Fatalf("%s: detail does not name the tool failure: %s", name, item.Detail)
		}
		// The point of the fix is that the real stderr becomes reachable.
		if !strings.Contains(item.Detail, "invalid memory address") {
			t.Fatalf("%s: detail does not carry the actual output: %s", name, item.Detail)
		}
	}
}

func TestRunMakeTarget_CrashIsNotRetriedAsALock(t *testing.T) {
	// golangciLintLocked already returns false once any location token appears,
	// so a panic could never be classified as a lock; pin that the crash flag
	// short-circuits the retry loop independently of that coincidence.
	if golangciLintLocked(panicLintOutput) {
		t.Fatal("panic output must never be classified as a held lock")
	}
	if toolCrashSignature(panicLintOutput) == "" {
		t.Fatal("crash flag must be set for the retry loop to short-circuit")
	}
}

func TestCrashOutputTail_KeepsLastNonEmptyLines(t *testing.T) {
	tail := crashOutputTail(panicLintOutput, 3)
	lines := strings.Split(tail, "\n")
	if len(lines) != 3 {
		t.Fatalf("tail line count = %d (%q)", len(lines), tail)
	}
	if !strings.Contains(tail, "make: *** [lint] Error 2") {
		t.Fatalf("tail must end at the last written line: %q", tail)
	}
	if strings.Contains(tail, "\n\n") {
		t.Fatalf("blank lines must be dropped: %q", tail)
	}
}

// TestMakeTargetMatchedByPatternRuleIsUndeclared pins the gitforge/gzh-cli
// Makefile shape: a catch-all pattern rule guarding "make run <arg>", with no
// check target declared. Before the fix, make exited 0 having done nothing and
// the probe was marked Defined, so the integration gate reported
// "PASS make check" for a gate that did not exist.
func TestMakeTargetMatchedByPatternRuleIsUndeclared(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "catchall-ran")
	writeRepoFile(t, dir, "Makefile",
		"lint:\n\t@:\n\n"+
			"# Prevent make from interpreting arguments as targets\n"+
			"%:\n\t@touch "+marker+"\n")

	check := runMakeTarget(context.Background(), dir, "check")
	if check.Defined {
		t.Fatalf("check is satisfied only by the catch-all pattern rule; Defined must be false (err=%v)\n%s", check.Err, check.Output)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("undeclared target must not be run at all; catch-all recipe executed, stat err = %v", err)
	}

	// A declared target in the same Makefile stays declared: the detector must
	// key on the pattern-rule stem, not on the presence of a catch-all.
	lint := runMakeTarget(context.Background(), dir, "lint")
	if !lint.Defined {
		t.Fatalf("declared lint target must stay Defined (err=%v)\n%s", lint.Err, lint.Output)
	}

	// Without the catch-all, the pre-existing output-string detection must keep
	// working — the new probe must not mask the "No rule to make target" path.
	plain := t.TempDir()
	writeRepoFile(t, plain, "Makefile", "lint:\n\t@:\n")
	if bare := runMakeTarget(context.Background(), plain, "check"); bare.Defined {
		t.Fatalf("missing check target must not be Defined\n%s", bare.Output)
	}
}
