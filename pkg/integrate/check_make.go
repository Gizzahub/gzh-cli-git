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
	"regexp"
	"strings"
	"time"
)

type makeProbe struct {
	Target string
	// WorkDir records where Make was measured. GoRootSrc and
	// ControllerPrepared are added only for controller-prepared probes; they
	// retain the measurement-specific evidence because the target worktree is
	// gone before the source probe runs.
	WorkDir                  string
	GoRootSrc                string
	ApprovedForeignLocations []string
	ControllerPrepared       bool
	Defined                  bool
	Skipped                  bool
	MissingCD                string
	Output                   string
	Err                      error
	Code                     int
	Unavailable              string
	// ToolCrash holds the line proving the tool died instead of reporting
	// findings. It is separate from Err because a crash and a rule violation
	// are the same non-zero exit; only the output distinguishes them.
	ToolCrash string
}

const (
	golangciLockAttempts  = 3
	golangciLockRetryWait = 250 * time.Millisecond
)

// allowedMakeTargets is the closed set runMakeTarget may launch. check.go
// probes exactly this set; the guard keeps that invariant enforced if a
// future caller grows the list.
var allowedMakeTargets = map[string]struct{}{
	"check": {},
	"lint":  {},
}

func runMakeTarget(ctx context.Context, dir, target string) makeProbe {
	if _, ok := allowedMakeTargets[target]; !ok {
		return makeProbe{Target: target, Err: fmt.Errorf("undeclared make target %q", target)}
	}
	for attempt := 1; ; attempt++ {
		probe := runMakeTargetOnce(ctx, dir, target)
		if err := ctx.Err(); err != nil {
			probe.Err = err
			return probe
		}
		// A crashed tool is not a held lock. Retrying it three times only
		// delays the report and hides the stack that identifies the cause.
		if target != "lint" || probe.Err == nil || probe.ToolCrash != "" || !golangciLintLocked(probe.Output) {
			return probe
		}
		if attempt == golangciLockAttempts {
			probe.Unavailable = fmt.Sprintf("golangci-lint lock persisted after %d attempts", golangciLockAttempts)
			return probe
		}
		if err := waitForRetry(ctx, golangciLockRetryWait); err != nil {
			probe.Err = err
			return probe
		}
	}
}

func runMakeTargetOnce(ctx context.Context, dir, target string) makeProbe {
	var lintCache string
	if target == "lint" {
		var err error
		lintCache, err = os.MkdirTemp("", "gz-git-integrate-golangci-lint-")
		if err != nil {
			return makeProbe{Target: target, WorkDir: dir, Err: fmt.Errorf("create golangci-lint cache: %w", err)}
		}
		defer func() { _ = os.RemoveAll(lintCache) }()
	}
	cmd := exec.CommandContext(ctx, "make", target) // #nosec G204 -- validated against allowedMakeTargets above
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MAKELEVEL=0", "MAKEFLAGS=", "LC_ALL=C")
	if lintCache != "" {
		cmd.Env = withoutEnv(cmd.Env, "GOLANGCI_LINT_CACHE")
		cmd.Env = append(cmd.Env, "GOLANGCI_LINT_CACHE="+lintCache)
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()
	probe := makeProbe{Target: target, WorkDir: dir, Output: out, Err: err}
	if err == nil {
		probe.Defined = true
		if hasSkippedCheck(out) {
			probe.Skipped = true
		}
		return probe
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		probe.Code = exitErr.ExitCode()
	}
	if isUndefinedMakeTarget(out, target) {
		return probe
	}
	probe.Defined = true
	if missing := missingCD(out); missing != "" {
		probe.MissingCD = missing
	}
	probe.ToolCrash = toolCrashSignature(out)
	return probe
}

// toolCrashSignature returns the line proving the tool terminated abnormally
// rather than reporting findings, or "" when the output shows an ordinary
// failure.
//
// This distinction is the whole point. A crashing linter writes its panic and
// stack to the same stream as its diagnostics, and every stack frame carries a
// `file.go:line` token. The foreign-diagnostic parser looks for exactly those
// tokens and nothing else, so it reads a crash as a wall of findings from paths
// outside the repository and reports that instead. The operator is then told the
// build tree is dirty when the truth is that the tool died, and the actual stderr
// — the only thing that identifies the cause — never reaches them.
//
// Matching is anchored at the start of the trimmed line so that a diagnostic
// merely quoting the word "panic" in its message is not mistaken for one.
func toolCrashSignature(out string) string {
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(stripANSI(line))
		switch {
		case strings.HasPrefix(trimmed, "panic: "),
			strings.HasPrefix(trimmed, "fatal error: "),
			strings.HasPrefix(trimmed, "runtime error: "),
			strings.HasPrefix(trimmed, "SIGSEGV:"),
			strings.HasPrefix(trimmed, "SIGABRT:"):
			return trimmed
		case strings.HasPrefix(trimmed, "goroutine ") && strings.HasSuffix(trimmed, "[running]:"):
			return trimmed
		}
	}
	return ""
}

// toolCrashDetail renders a crash so the cause is reachable from the check
// table itself. A bare "the tool crashed" would be honest but still leave the
// operator without the stderr, which is the failure this replaces.
func toolCrashDetail(probe makeProbe) string {
	detail := fmt.Sprintf("tool failure — make %s terminated abnormally instead of reporting findings: %s", probe.Target, probe.ToolCrash)
	if probe.Code != 0 {
		detail += fmt.Sprintf(" (exit %d)", probe.Code)
	}
	if tail := crashOutputTail(probe.Output, 12); tail != "" {
		detail += "; output tail:\n" + tail
	}
	return detail
}

// crashOutputTail keeps the last non-empty lines of the run. The cause of a
// panic is usually at the top of the stack, but the lines that identify which
// input triggered it are typically the last ones the tool managed to write.
func crashOutputTail(out string, limit int) string {
	var kept []string
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimRight(stripANSI(line), " \t\r")
		if strings.TrimSpace(trimmed) == "" {
			continue
		}
		kept = append(kept, trimmed)
	}
	if len(kept) > limit {
		kept = kept[len(kept)-limit:]
	}
	return strings.Join(kept, "\n")
}

func golangciLintLocked(out string) bool {
	locked := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(stripANSI(line))
		if line == "" {
			continue
		}
		if findLocationMatch(line) != "" {
			return false
		}
		switch {
		case line == "Error: parallel golangci-lint is running":
			locked = true
		case isGolangciLockBoilerplate(line):
			// Command echo and make's exit wrapper carry no independent result.
		default:
			// A pure lock failure must not contain another tool failure.
			return false
		}
	}
	return locked
}

var (
	ansiEscape             = regexp.MustCompile(`\x1b\[[\x30-\x3f]*[\x20-\x2f]*[\x40-\x7e]`)
	recursiveMakeLintError = regexp.MustCompile(`^make(?:\[\d+\])?: \*\*\* \[(?:.+: )?(?:lint|lint-check)\] Error \d+$`)
)

func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

func isGolangciLockBoilerplate(line string) bool {
	return line == "Running golangci-lint..." ||
		line == "-e Running golangci-lint..." ||
		strings.HasPrefix(line, "golangci-lint run ") ||
		strings.HasPrefix(line, "GOWORK=off golangci-lint run ") ||
		recursiveMakeLintError.MatchString(line)
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func withoutEnv(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, item := range env {
		if !strings.HasPrefix(item, prefix) {
			out = append(out, item)
		}
	}
	return out
}

func isUndefinedMakeTarget(out, target string) bool {
	if strings.Contains(out, "No targets specified and no makefile found") {
		return true
	}
	// Only the top-level make line. Recursive make prints "make[1]:".
	needle := "make: *** No rule to make target `" + target + "'"
	alt := "make: *** No rule to make target '" + target + "'"
	return strings.Contains(out, needle) || strings.Contains(out, alt) ||
		strings.Contains(out, "No rule to make target `"+target+"'") ||
		strings.Contains(out, "No rule to make target '"+target+"'")
}

func hasSkippedCheck(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "SKIPPED CHECK:") {
			return true
		}
	}
	return false
}

func missingCD(out string) string {
	for _, line := range strings.Split(out, "\n") {
		const prefix = "cd: "
		i := strings.Index(line, prefix)
		if i < 0 {
			continue
		}
		rest := line[i+len(prefix):]

		// dash (the /bin/sh used by Ubuntu) reports a failed directory
		// change as, for example, "/bin/sh: 1: cd: can't cd to api".
		// The path is the entire non-empty tail: unlike the BSD/GNU form
		// below, dash supplies no suffix delimiter.
		const dashPrefix = "can't cd to "
		if strings.HasPrefix(rest, dashPrefix) {
			if !shellCDPrefix(line[:i]) {
				continue
			}
			if path := strings.TrimSpace(strings.TrimPrefix(rest, dashPrefix)); path != "" {
				return path
			}
			continue
		}

		// Preserve the BSD/GNU shell form, including a shell-specific prefix
		// before "cd:". A non-empty path is required so arbitrary output
		// containing the suffix cannot be mistaken for a missing component.
		const posixSuffix = ": No such file or directory"
		if path, found := strings.CutSuffix(rest, posixSuffix); found {
			if path = strings.TrimSpace(path); path != "" {
				return path
			}
		}
	}
	return ""
}

// shellCDPrefix accepts a bare shell diagnostic or the conventional
// "<shell>: [line:]" prefix. Requiring a shell name for dash's un-delimited
// form avoids treating arbitrary tool output containing "can't cd to" as a
// missing Make component.
func shellCDPrefix(prefix string) bool {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return true
	}
	parts := strings.Split(prefix, ":")
	if len(parts) < 2 {
		return false
	}
	shell := strings.TrimSpace(parts[0])
	return strings.HasSuffix(shell, "sh")
}

// judgeMakeLegacy preserves the repository-owned baseline comparison used when
// no controller has captured a prepared target probe.
func judgeMakeLegacy(ctx context.Context, g gitRepo, plan TargetPlan, probe makeProbe, allowSkipped bool) CheckItem {
	name := "make " + probe.Target
	if !probe.Defined {
		return CheckItem{Name: name, Status: checkSkip, Detail: "undefined"}
	}
	if probe.Unavailable != "" {
		return CheckItem{Name: name, Status: checkFail, Detail: "measurement unavailable: " + probe.Unavailable}
	}
	// The crash verdict must precede the foreign-diagnostic classifier: a panic
	// stack is made entirely of path:line tokens, so whichever runs first owns
	// the report.
	if probe.ToolCrash != "" {
		return CheckItem{Name: name, Status: checkFail, Detail: toolCrashDetail(probe)}
	}
	if probe.Target == "lint" {
		if err := foreignDiagnosticErrorForProbe("branch", probe); err != nil {
			return CheckItem{
				Name:   name,
				Status: checkFail,
				Detail: err.Error(),
			}
		}
	}
	if probe.MissingCD != "" {
		return CheckItem{
			Name:   name,
			Status: checkFail,
			Detail: fmt.Sprintf("not run — %q missing in this tree", probe.MissingCD),
		}
	}
	if probe.Err == nil {
		if probe.Skipped && !allowSkipped {
			return CheckItem{Name: name, Status: checkFail, Detail: "SKIPPED CHECK (not a pass); pass --allow-skipped-checks to downgrade"}
		}
		if probe.Skipped {
			return CheckItem{Name: name, Status: checkWarn, Detail: "SKIPPED CHECK downgraded by --allow-skipped-checks"}
		}
		return CheckItem{Name: name, Status: checkPass, Detail: "ok"}
	}

	verdict, err := baselineAgainstTarget(ctx, g, plan, probe)
	if err != nil {
		return CheckItem{Name: name, Status: checkFail, Detail: err.Error()}
	}
	if verdict.Status == BaselineFail {
		return CheckItem{Name: name, Status: checkFail, Detail: verdict.Reason}
	}
	return CheckItem{Name: name, Status: checkWarn, Detail: "baseline failure, non-worsening: " + verdict.Reason}
}

// judgeMakeAgainstProbe consumes a target measurement captured before source
// code exists. Parser and baseline rules remain exactly the legacy rules.
func judgeMakeAgainstProbe(ctx context.Context, g gitRepo, plan TargetPlan, probe makeProbe, allowSkipped bool, base makeProbe) CheckItem {
	if base.Target == "" {
		return judgeMakeLegacy(ctx, g, plan, probe, allowSkipped)
	}
	name := "make " + probe.Target
	if !probe.Defined {
		return CheckItem{Name: name, Status: checkSkip, Detail: "undefined"}
	}
	if probe.Unavailable != "" {
		return CheckItem{Name: name, Status: checkFail, Detail: "measurement unavailable: " + probe.Unavailable}
	}
	if probe.ToolCrash != "" {
		return CheckItem{Name: name, Status: checkFail, Detail: toolCrashDetail(probe)}
	}
	if probe.Target == "lint" {
		if err := foreignDiagnosticErrorForProbe("branch", probe); err != nil {
			return CheckItem{Name: name, Status: checkFail, Detail: err.Error()}
		}
	}
	if probe.MissingCD != "" {
		return CheckItem{Name: name, Status: checkFail, Detail: fmt.Sprintf("not run — %q missing in this tree", probe.MissingCD)}
	}
	if probe.Err == nil {
		if probe.Skipped && !allowSkipped {
			return CheckItem{Name: name, Status: checkFail, Detail: "SKIPPED CHECK (not a pass); pass --allow-skipped-checks to downgrade"}
		}
		if probe.Skipped {
			return CheckItem{Name: name, Status: checkWarn, Detail: "SKIPPED CHECK downgraded by --allow-skipped-checks"}
		}
		return CheckItem{Name: name, Status: checkPass, Detail: "ok"}
	}
	if base.Unavailable != "" {
		return CheckItem{Name: name, Status: checkFail, Detail: "measurement unavailable: baseline make " + probe.Target + ": " + base.Unavailable}
	}
	if base.MissingCD != "" {
		return CheckItem{Name: name, Status: checkFail, Detail: fmt.Sprintf("baseline make %s did not run — %q missing in target tree", probe.Target, base.MissingCD)}
	}
	if probe.Target == "lint" {
		if err := foreignDiagnosticErrorForProbe("baseline", base); err != nil {
			return CheckItem{Name: name, Status: checkFail, Detail: err.Error()}
		}
	}
	if base.Err == nil {
		return CheckItem{Name: name, Status: checkFail, Detail: fmt.Sprintf("failed here (rc=%d) but target tip passes", probe.Code)}
	}
	branchTracked, err := g.lsTreeNames(ctx, plan.BranchSHA)
	if err != nil {
		return CheckItem{Name: name, Status: checkFail, Detail: err.Error()}
	}
	baseTracked, err := g.lsTreeNames(ctx, plan.TargetSHA)
	if err != nil {
		return CheckItem{Name: name, Status: checkFail, Detail: err.Error()}
	}
	changed, err := g.diffNames(ctx, plan.TargetSHA, plan.BranchSHA)
	if err != nil {
		return CheckItem{Name: name, Status: checkFail, Detail: err.Error()}
	}
	verdict := EvaluateBaseline(BaselineInput{BranchLocations: extractLocationsForProbe(probe, branchTracked), BaseLocations: extractLocationsForProbe(base, baseTracked), ChangedPaths: changed})
	if verdict.Status == BaselineFail {
		return CheckItem{Name: name, Status: checkFail, Detail: verdict.Reason}
	}
	return CheckItem{Name: name, Status: checkWarn, Detail: "baseline failure, non-worsening: " + verdict.Reason}
}

func baselineAgainstTarget(ctx context.Context, g gitRepo, plan TargetPlan, probe makeProbe) (BaselineResult, error) {
	root, err := os.MkdirTemp("", "gz-git-integrate-base-")
	if err != nil {
		return BaselineResult{}, fmt.Errorf("temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	wt := filepath.Join(root, "wt")
	if err := g.worktreeAddDetach(ctx, wt, plan.TargetSHA); err != nil {
		return BaselineResult{}, fmt.Errorf("cannot build baseline worktree: %w", err)
	}
	defer func() { _ = g.worktreeRemoveForce(ctx, wt) }()

	baseProbe := runMakeTarget(ctx, wt, probe.Target)
	if baseProbe.Unavailable != "" {
		return BaselineResult{}, fmt.Errorf("measurement unavailable: baseline make %s: %s", probe.Target, baseProbe.Unavailable)
	}
	if baseProbe.MissingCD != "" {
		return BaselineResult{}, fmt.Errorf(
			"baseline make %s did not run — %q missing in target tree",
			probe.Target,
			baseProbe.MissingCD,
		)
	}
	if probe.Target == "lint" {
		if err := foreignDiagnosticErrorForProbe("baseline", baseProbe); err != nil {
			return BaselineResult{}, err
		}
	}
	if baseProbe.Err == nil {
		return BaselineResult{Status: BaselineFail, Reason: fmt.Sprintf("failed here (rc=%d) but target tip passes", probe.Code)}, nil
	}

	branchTracked, err := g.lsTreeNames(ctx, plan.BranchSHA)
	if err != nil {
		return BaselineResult{}, err
	}
	baseTracked, err := g.lsTreeNames(ctx, plan.TargetSHA)
	if err != nil {
		return BaselineResult{}, err
	}
	changed, err := g.diffNames(ctx, plan.TargetSHA, plan.BranchSHA)
	if err != nil {
		return BaselineResult{}, err
	}
	return EvaluateBaseline(BaselineInput{
		BranchLocations: extractLocationsForProbe(probe, branchTracked),
		BaseLocations:   extractLocationsForProbe(baseProbe, baseTracked),
		ChangedPaths:    changed,
	}), nil
}
