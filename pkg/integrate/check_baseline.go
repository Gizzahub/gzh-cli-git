// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// BaselineStatus is the non-worsening verdict for a failed make target.
type BaselineStatus int

const (
	// BaselinePass means the branch did not worsen the target tip.
	BaselinePass BaselineStatus = iota
	// BaselineFail means the branch introduced or increased failures.
	BaselineFail
)

// BaselineInput is the already-normalized location lists for one make target.
type BaselineInput struct {
	BranchLocations []string
	BaseLocations   []string
	ChangedPaths    []string
}

// BaselineResult is the count-only non-worsening verdict.
type BaselineResult struct {
	Status BaselineStatus
	Reason string
}

// locationLine matches the "file:line" prefix that go vet, golangci-lint,
// tsc, eslint, ruff, and pytest all emit at the start of a diagnostic line.
var locationLine = regexp.MustCompile(`^[^ \t:]+\.[A-Za-z0-9_]+:\d+`)

// EvaluateBaseline is the non-worsening gate.
//
// It does not compare diagnostic location sets. The measured lint run
// (golangci-lint, same commit, twice) produced 121 locations of which 78
// differed — a set-diff makes the gate unpassable. Only two signals are
// stable: diagnostics on paths this branch changed, and a count increase.
func EvaluateBaseline(in BaselineInput) BaselineResult {
	branch := uniqueSorted(in.BranchLocations)
	base := uniqueSorted(in.BaseLocations)
	changed := make(map[string]struct{}, len(in.ChangedPaths))
	for _, p := range in.ChangedPaths {
		p = strings.TrimSpace(p)
		if p != "" {
			changed[p] = struct{}{}
		}
	}

	var attributed []string
	for _, loc := range branch {
		path, _, ok := splitLoc(loc)
		if !ok {
			continue
		}
		if _, hit := changed[path]; hit {
			attributed = append(attributed, loc)
		}
	}
	if len(attributed) > 0 {
		return BaselineResult{
			Status: BaselineFail,
			Reason: fmt.Sprintf("diagnostics on changed paths: %s", strings.Join(attributed, " ")),
		}
	}
	if len(branch) == 0 {
		return BaselineResult{
			Status: BaselineFail,
			Reason: fmt.Sprintf("no file:line diagnostics to judge non-worsening (base had %d)", len(base)),
		}
	}
	if len(branch) > len(base) {
		return BaselineResult{
			Status: BaselineFail,
			Reason: fmt.Sprintf("diagnostic count increased (%d → %d)", len(base), len(branch)),
		}
	}
	return BaselineResult{
		Status: BaselinePass,
		Reason: fmt.Sprintf("count %d → %d, no diagnostics on changed paths", len(base), len(branch)),
	}
}

// ExtractLocations pulls file:line tokens from tool output and normalizes
// them against the revision's tracked paths (strip leading components until
// a tracked path matches). Unmatched paths are kept, fail-closed.
func ExtractLocations(output string, tracked []string) []string {
	known := make(map[string]struct{}, len(tracked))
	for _, p := range tracked {
		known[p] = struct{}{}
	}
	var out []string
	for _, line := range strings.Split(output, "\n") {
		match := locationLine.FindString(strings.TrimSpace(line))
		if match == "" {
			// also accept a file:line after a leading "./"
			match = locationLine.FindString(strings.TrimPrefix(strings.TrimSpace(line), "./"))
		}
		if match == "" {
			continue
		}
		path, lineNo, ok := splitLoc(match)
		if !ok {
			continue
		}
		path = normalizeTrackedPath(path, known)
		out = append(out, path+":"+lineNo)
	}
	return uniqueSorted(out)
}

// foreignDiagnosticLocations finds diagnostic locations that start outside
// the repository. It must run before normalizeTrackedPath, which intentionally
// strips leading path components to match tracked files.
func foreignDiagnosticLocations(output string) []string {
	var out []string
	for _, line := range strings.Split(output, "\n") {
		match := locationLine.FindString(strings.TrimSpace(line))
		if match == "" {
			match = locationLine.FindString(strings.TrimPrefix(strings.TrimSpace(line), "./"))
		}
		if match == "" {
			continue
		}
		path, _, ok := splitLoc(match)
		if !ok {
			continue
		}
		if strings.HasPrefix(strings.TrimPrefix(filepath.ToSlash(path), "./"), "../") {
			out = append(out, match)
		}
	}
	return uniqueSorted(out)
}

func foreignDiagnosticError(scope, output string) error {
	foreign := foreignDiagnosticLocations(output)
	if len(foreign) == 0 {
		return nil
	}
	return fmt.Errorf("%s diagnostics reference paths outside the repository: %s", scope, strings.Join(foreign, " "))
}

func normalizeTrackedPath(path string, tracked map[string]struct{}) string {
	path = filepath.ToSlash(path)
	for {
		if _, ok := tracked[path]; ok {
			return path
		}
		slash := strings.IndexByte(path, '/')
		if slash < 0 {
			return path
		}
		path = path[slash+1:]
	}
}

func splitLoc(loc string) (path, line string, ok bool) {
	i := strings.LastIndexByte(loc, ':')
	if i <= 0 || i == len(loc)-1 {
		return "", "", false
	}
	return loc[:i], loc[i+1:], true
}

func uniqueSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// setDiffWouldReject is the WRONG gate: it treats a location-set difference
// as a regression. EvaluateBaseline must not use it. The fixture in
// TestBaselineSameCountDifferentSetsPass is chosen so this helper is true
// and the real gate is PASS — if someone later implements set-diff, that
// test goes red.
func setDiffWouldReject(base, branch []string) bool {
	a := uniqueSorted(base)
	b := uniqueSorted(branch)
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return true
		}
	}
	return false
}
