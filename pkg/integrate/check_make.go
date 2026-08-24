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
)

type makeProbe struct {
	Target    string
	Defined   bool
	Skipped   bool
	MissingCD string
	Output    string
	Err       error
	Code      int
}

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
	var lintCache string
	if target == "lint" {
		var err error
		lintCache, err = os.MkdirTemp("", "gz-git-integrate-golangci-lint-")
		if err != nil {
			return makeProbe{Target: target, Err: fmt.Errorf("create golangci-lint cache: %w", err)}
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
	probe := makeProbe{Target: target, Output: out, Err: err}
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
	return probe
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
		const mid = ": No such file or directory"
		if !strings.Contains(line, "cd: ") || !strings.Contains(line, mid) {
			continue
		}
		i := strings.Index(line, "cd: ")
		rest := line[i+4:]
		j := strings.Index(rest, mid)
		if j > 0 {
			return rest[:j]
		}
	}
	return ""
}

func judgeMake(ctx context.Context, g gitRepo, plan TargetPlan, probe makeProbe, allowSkipped bool) CheckItem {
	name := "make " + probe.Target
	if !probe.Defined {
		return CheckItem{Name: name, Status: checkSkip, Detail: "undefined"}
	}
	if probe.Target == "lint" {
		if err := foreignDiagnosticError("branch", probe.Output); err != nil {
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
			Status: checkWarn,
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
	if probe.Target == "lint" {
		if err := foreignDiagnosticError("baseline", baseProbe.Output); err != nil {
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
		BranchLocations: ExtractLocations(probe.Output, branchTracked),
		BaseLocations:   ExtractLocations(baseProbe.Output, baseTracked),
		ChangedPaths:    changed,
	}), nil
}
