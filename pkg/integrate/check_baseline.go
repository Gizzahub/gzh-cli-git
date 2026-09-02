// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	pathpkg "path"
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

// labelPrefix matches an uppercase diagnostic tag ("BROKEN", "DANGLING_ROW")
// that some project check scripts emit before the file:line token, instead
// of starting the line with the path itself. Observed in infra-ops's
// scripts/{link,task-index}-check.sh: "BROKEN  tasks/batch.md:9: ...".
var labelPrefix = regexp.MustCompile(`^[A-Z][A-Z0-9_]*[ \t]+`)

// ruffArrowPrefix matches Ruff's full output location marker. Ruff prints the
// diagnostic code on one line and the location on the next as
// "--> path.py:line:column" instead of starting that line with the path.
var ruffArrowPrefix = regexp.MustCompile(`^-->[ \t]+`)

// ariadneLocation matches the location line rolldown/oxc's ariadne-style
// diagnostic renderer prints as a frame opener. Measured against a real
// captured failure (pkg/integrate/testdata/rolldown-oxc-unloadable-dependency*.txt,
// vite 8 / rolldown 1.2.5): after ANSI is stripped, the raw docker-buildkit
// capture reads "#19 12.71     ╭─[ src/routes/BuildHistory.svelte:20:34 ]"
// while a plain capture reads "    ╭─[ src/routes/ProjectDashboard.svelte:12:30 ]".
// Unlike every other locationLine form this one does not start the line —
// there is a "╭─[" opener, and in the docker capture a buildkit
// step/timestamp prefix before that — so it is found by searching for the
// opener anywhere in the line instead of anchoring at ^. The space after
// '[' and before ']' is optional in both observed forms. It must NOT match
// the fixture's body line ("12 │ import ... .svelte';", no "╭─[" marker) or
// the node stack-trace lines further down ("at ... (file:///...:48:18)",
// same reason). The column (":34") is dropped so the result matches every
// other locationLine consumer's "file:line" shape.
var ariadneLocation = regexp.MustCompile(`╭─\[\s*([^\s\]]+\.[A-Za-z0-9_]+:\d+)(?::\d+)?\s*\]`)

// diagnosticCandidates returns the strings to try locationLine against for
// one output line: as-is, with a leading label tag stripped, with Ruff's
// location arrow stripped, and each recognized form with a leading "./"
// stripped. Order matters only for foreign-path detection, which needs the
// untouched path form before normalization.
func diagnosticCandidates(line string) []string {
	trimmed := strings.TrimSpace(line)
	noLabel := labelPrefix.ReplaceAllString(trimmed, "")
	noRuffArrow := ruffArrowPrefix.ReplaceAllString(trimmed, "")
	return []string{
		trimmed,
		noLabel,
		noRuffArrow,
		strings.TrimPrefix(trimmed, "./"),
		strings.TrimPrefix(noLabel, "./"),
		strings.TrimPrefix(noRuffArrow, "./"),
	}
}

func findLocationMatch(line string) string {
	// stripANSI (check_make.go) is applied first: the ariadne opener and the
	// rest of the diagnostic frame are individually SGR-colored in the raw
	// docker capture ("\x1b[38;5;246m╭\x1b[0m\x1b[38;5;246m─\x1b[0m..."), which
	// would otherwise split "╭─[" across escape sequences and defeat every
	// match below, not just ariadneLocation.
	clean := stripANSI(line)
	if m := ariadneLocation.FindStringSubmatch(clean); m != nil {
		return m[1]
	}
	for _, candidate := range diagnosticCandidates(clean) {
		if match := locationLine.FindString(candidate); match != "" {
			return match
		}
	}
	return ""
}

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
	return extractLocationsForProbe(makeProbe{Output: output}, tracked)
}

// extractLocationsForProbe preserves legacy extraction unless this is a
// controller-prepared probe with a verified standard-library allowance. Such
// external locations are toolchain diagnostics, not repository baseline
// locations, and therefore must not affect count comparisons.
func extractLocationsForProbe(probe makeProbe, tracked []string) []string {
	probe.Output = suppressGoTestVerboseNoise(probe.Output)
	known := make(map[string]struct{}, len(tracked))
	for _, p := range tracked {
		known[p] = struct{}{}
	}
	var out []string
	for _, line := range strings.Split(probe.Output, "\n") {
		match := findLocationMatch(line)
		if match == "" {
			continue
		}
		path, lineNo, ok := splitLoc(match)
		if !ok {
			continue
		}
		if probe.approvedForeignLocation(match) {
			continue
		}
		path = normalizeTrackedPath(path, known)
		out = append(out, path+":"+lineNo)
	}
	return uniqueSorted(out)
}

// foreignDiagnosticLocations finds absolute and parent-traversing diagnostic
// locations before normalizeTrackedPath can erase their provenance.
func foreignDiagnosticLocations(output string) []string {
	return foreignDiagnosticLocationsForProbe(makeProbe{Output: output})
}

func foreignDiagnosticError(scope, output string) error {
	return foreignDiagnosticErrorForProbe(scope, makeProbe{Output: output})
}

// annotateControllerPreparedProbe records the canonical standard-library
// source root observed by this exact prepared worktree. It deliberately does
// not surface discovery failures: without the evidence, external diagnostics
// remain rejected by foreignDiagnosticErrorForProbe.
func annotateControllerPreparedProbe(ctx context.Context, probe makeProbe) makeProbe {
	probe.ControllerPrepared = true
	if probe.WorkDir == "" {
		return probe
	}
	cmd := exec.CommandContext(ctx, "go", "env", "GOROOT") // #nosec G204 -- fixed Go subcommand.
	cmd.Dir = probe.WorkDir
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		return probe
	}
	root := strings.TrimSpace(string(out))
	if root == "" || !filepath.IsAbs(root) {
		return probe
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return probe
	}
	canonicalSrc, err := filepath.EvalSymlinks(filepath.Join(canonicalRoot, "src"))
	if err != nil {
		return probe
	}
	info, err := os.Stat(canonicalSrc)
	if err != nil || !info.IsDir() {
		return probe
	}
	probe.GoRootSrc = canonicalSrc
	return freezeGoRootDiagnosticApprovals(probe)
}

func foreignDiagnosticErrorForProbe(scope string, probe makeProbe) error {
	foreign := foreignDiagnosticLocationsForProbe(probe)
	if len(foreign) == 0 {
		return nil
	}
	return fmt.Errorf("%s diagnostics reference paths outside the repository: %s", scope, strings.Join(foreign, " "))
}

// foreignDiagnosticLocationsForProbe allows an external path only when a
// controller-prepared probe captured a GOROOT/src allowance for the same
// worktree. It does not make GOROOT configurable and retains fail-closed
// behavior for every unresolved or escaping path.
func foreignDiagnosticLocationsForProbe(probe makeProbe) []string {
	probe.Output = suppressGoTestVerboseNoise(probe.Output)
	var out []string
	for _, line := range strings.Split(probe.Output, "\n") {
		match := findLocationMatch(line)
		if match == "" {
			continue
		}
		path, _, ok := splitLoc(match)
		if !ok || !isLexicallyForeignDiagnosticPath(path) {
			continue
		}
		if probe.approvedForeignLocation(match) {
			continue
		}
		out = append(out, match)
	}
	return uniqueSorted(out)
}

// isLexicallyForeignDiagnosticPath is the legacy, filesystem-independent
// boundary. Keep it separate from controller approval: a relative path can
// legitimately traverse a symlink into GOROOT/src without being a legacy
// foreign path.
func isLexicallyForeignDiagnosticPath(diagnosticPath string) bool {
	path := filepath.ToSlash(diagnosticPath)
	if filepath.IsAbs(strings.ReplaceAll(path, "/", string(filepath.Separator))) {
		return true
	}
	cleaned := pathpkg.Clean(path)
	return cleaned == ".." || strings.HasPrefix(cleaned, "../")
}

func (probe makeProbe) approvedForeignLocation(location string) bool {
	if !probe.ControllerPrepared {
		return false
	}
	for _, approved := range probe.ApprovedForeignLocations {
		if location == approved {
			return true
		}
	}
	return false
}

// freezeGoRootDiagnosticApprovals resolves foreign diagnostic paths while the
// prepared worktree still exists. The resulting exact location tokens can be
// safely consumed after the target worktree has been removed.
func freezeGoRootDiagnosticApprovals(probe makeProbe) makeProbe {
	if !probe.ControllerPrepared || probe.WorkDir == "" || probe.GoRootSrc == "" {
		return probe
	}
	var approved []string
	for _, line := range strings.Split(probe.Output, "\n") {
		location := findLocationMatch(line)
		if location == "" {
			continue
		}
		path, _, ok := splitLoc(location)
		if !ok {
			continue
		}
		canonicalPath, err := evalDiagnosticPathInOrder(probe.WorkDir, path)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(probe.GoRootSrc, canonicalPath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == "." {
			continue
		}
		approved = append(approved, location)
	}
	probe.ApprovedForeignLocations = uniqueSorted(approved)
	return probe
}

// evalDiagnosticPathInOrder processes path components in order, resolving
// each symlink before applying a following ".." component. filepath.Join or
// Clean would erase that ordering and can change the meaning of a diagnostic
// path that traverses a symlinked worktree.
func evalDiagnosticPathInOrder(workDir, diagnosticPath string) (string, error) {
	separatorPath := strings.ReplaceAll(filepath.ToSlash(diagnosticPath), "/", string(filepath.Separator))
	raw := separatorPath
	if !filepath.IsAbs(raw) {
		raw = workDir + string(filepath.Separator) + raw
	}
	if !filepath.IsAbs(raw) {
		return "", fmt.Errorf("diagnostic path is not absolute")
	}
	volume := filepath.VolumeName(raw)
	current := volume + string(filepath.Separator)
	remainder := strings.TrimPrefix(raw, volume)
	remainder = strings.TrimLeft(remainder, `/\`)
	for _, component := range strings.Split(filepath.ToSlash(remainder), "/") {
		switch component {
		case "", ".":
			continue
		case "..":
			current = filepath.Dir(current)
		default:
			resolved, err := filepath.EvalSymlinks(filepath.Join(current, component))
			if err != nil {
				return "", err
			}
			current = resolved
		}
	}
	return current, nil
}

// goTestRunMarker matches a go test -v test-entry marker ("=== RUN",
// "=== PAUSE", "=== CONT", "=== NAME"). It always starts at column 0 with no
// leading indentation, unlike its subtest's "--- STATUS:" trailer.
var goTestRunMarker = regexp.MustCompile(`^=== (RUN|PAUSE|CONT|NAME)\s`)

// goTestStatusTrailer matches a go test -v per-test trailer line, e.g.
// "--- FAIL: TestFoo (0.00s)" or, for a nested subtest, the indented
// "    --- SKIP: TestFoo/sub (0.00s)". Capture group 1 is FAIL/SKIP/PASS.
var goTestStatusTrailer = regexp.MustCompile(`^\s*--- (FAIL|SKIP|PASS): `)

// suppressGoTestVerboseNoise blanks the indented "file.go:NN:" lines that
// `go test -v` prints for a subtest's t.Skip/t.Log/t.Errorf output before
// that subtest's own trailer line — but only when the trailer is
// "--- SKIP:" or "--- PASS:". Without this, strings.TrimSpace (in
// diagnosticCandidates) erases the only signal — indentation depth — that
// distinguishes a nested t.Skip/t.Log line from a t.Errorf line, and a run
// full of skips gets counted as a run full of failures (and vice versa).
//
// gz-git invokes a repository's own `make check` / `make lint`; it does not
// invoke `go test` itself, so it cannot add `-json` to a command it doesn't
// own. This is therefore a detection pass over captured `go test -v` human
// output, keyed on the "=== RUN"/"--- FAIL|SKIP|PASS:" markers, which are
// stable across Go versions and don't require gz-git to control the command.
//
// go test prints a subtest's buffered log output immediately BEFORE that
// subtest's own trailer, not after it:
//
//	=== RUN   TestFoo/sub
//	    foo_test.go:20: assertion failed
//	--- FAIL: TestFoo/sub (0.00s)
//
// so candidate lines are held in a pending buffer since the last boundary
// (an "=== RUN"-family marker or the previous trailer) and resolved against
// the next trailer's status when it arrives — blanked for SKIP/PASS, kept
// for FAIL. A trailer with an empty pending buffer (e.g. a parent test's
// trailer, printed after its subtests already consumed their own lines)
// is a no-op.
//
// This does not attempt to de-interleave t.Parallel() output: several
// tests' lines can interleave under one shared marker/trailer sequence,
// and in that case the pending buffer may span more than one test. It also
// does not track lines outside any "=== RUN"-opened span (e.g. earlier
// tool output in the same `make check` stream) — those are left untouched
// and counted normally, which is the conservative default.
func suppressGoTestVerboseNoise(output string) string {
	lines := strings.Split(output, "\n")
	var pending []int
	inRun := false
	for i, line := range lines {
		clean := stripANSI(line)
		if goTestRunMarker.MatchString(clean) {
			inRun = true
			pending = nil
			continue
		}
		if m := goTestStatusTrailer.FindStringSubmatch(clean); m != nil {
			if m[1] != "FAIL" {
				for _, idx := range pending {
					lines[idx] = ""
				}
			}
			pending = nil
			inRun = true
			continue
		}
		if inRun && findLocationMatch(line) != "" {
			pending = append(pending, i)
		}
	}
	return strings.Join(lines, "\n")
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
