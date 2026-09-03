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
	// BaselineUnmeasurable means the comparison could not be made at all:
	// the target tip's own run failed without emitting a single file:line
	// diagnostic, so there is no baseline to compare against. It is not a
	// verdict about the branch and must never be reported as one.
	BaselineUnmeasurable
)

// BaselineInput is the already-normalized location lists for one make target,
// plus the evidence needed to read an empty BaseLocations correctly. The two
// evidence fields are documented with their types in check_baseline_state.go.
type BaselineInput struct {
	BranchLocations []string
	BaseLocations   []string
	ChangedPaths    []string
	// BranchPrepared and BasePrepared record the tree each side ran in.
	BranchPrepared PrepareState
	BasePrepared   PrepareState
	// BaseMeasurement says whether the target-tip run reached the stage where
	// it could have reported a diagnostic at all.
	BaseMeasurement BaseMeasurement
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

	// Rule (a) asks whether the branch caused a diagnostic, so a diagnostic the
	// target tip already reported at the same file:line is not evidence against
	// it. Without this subtraction, touching any file that already carried a
	// warning hard-failed the branch even when base and branch were identical,
	// which made pre-existing-warning files unmodifiable.
	//
	// Identity is exact file:line. A diagnostic that merely shifted down
	// because lines were inserted above it still reads as new; normalizing that
	// is a separate problem and is deliberately not attempted here.
	//
	// When the base could not be measured at all, base is empty and this
	// subtracts nothing, so rule (a) keeps firing on any diagnostic on a changed
	// path — the stance the comment below relies on.
	preexisting := make(map[string]struct{}, len(base))
	for _, loc := range base {
		preexisting[loc] = struct{}{}
	}

	var attributed []string
	for _, loc := range branch {
		path, _, ok := splitLoc(loc)
		if !ok {
			continue
		}
		if _, hit := changed[path]; !hit {
			continue
		}
		if _, old := preexisting[loc]; old {
			continue
		}
		attributed = append(attributed, loc)
	}
	if len(attributed) > 0 {
		return BaselineResult{
			Status: BaselineFail,
			Reason: fmt.Sprintf("diagnostics on changed paths: %s", strings.Join(attributed, " ")),
		}
	}
	// A failed target tip that emitted no file:line diagnostic MAY not have
	// been measured: zero can mean "the tool never got far enough to report"
	// rather than "the baseline is clean". A checker that dies early on its
	// own — mix without deps/, a test runner without node_modules, a venv-less
	// pytest — exits non-zero with no parseable location, and comparing
	// against it is what made every commit unlandable in such a repository: a
	// branch measured at 0 failed rule (b), and a branch that did run failed
	// rule (c) as "count increased (0 -> N)". Neither is a statement about the
	// branch.
	//
	// But zero is not always that. A run that went to completion and failed in
	// a shape with no file:line in it — gofmt -l, gci diff, a shell check
	// script — measured zero, and against that baseline a branch emitting N is
	// a real worsening. Reading it as unmeasurable hands that worsening to
	// --allow-skipped-checks, whose contract is that the check was skipped;
	// this one was not. The two are told apart by BaseMeasurement, which the
	// call sites fill from probe evidence, not by the location count, which
	// cannot see the difference. Only positively evidenced measurement falls
	// through to the count rules below; a caller that says nothing keeps the
	// reading it had before the field existed, because taking the operator's
	// escape hatch away on no evidence is how a repository stops being able
	// to land anything at all.
	//
	// Rule (a) stays above this either way — a diagnostic on a path this
	// branch changed is evidence of harm whether or not the baseline could be
	// measured.
	if len(base) == 0 && in.BaseMeasurement != BaseMeasured {
		return BaselineResult{
			Status: BaselineUnmeasurable,
			Reason: unmeasurableReason(len(branch), in.BranchPrepared, in.BasePrepared),
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
// them against the revision's tracked paths: an over-qualified path loses
// leading components, and a path emitted relative to a subdirectory is lifted
// back to its root-relative form when exactly one tracked path ends with it.
// Unmatched paths are kept, fail-closed.
func ExtractLocations(output string, tracked []string) []string {
	return extractLocationsForProbe(makeProbe{Output: output}, tracked)
}

// extractLocationsForProbe preserves legacy extraction unless this is a
// controller-prepared probe with a verified standard-library allowance. Such
// external locations are toolchain diagnostics, not repository baseline
// locations, and therefore must not affect count comparisons.
func extractLocationsForProbe(probe makeProbe, tracked []string) []string {
	probe.Output = suppressGoTestVerboseNoise(probe.Output)
	known := newTrackedIndex(tracked)
	dirs := newMakeDirectoryTracker(probe.WorkDir)

	type sighting struct {
		path     string
		lineNo   string
		prefix   string
		anchored bool
	}
	seen := make([]sighting, 0, 8)
	for _, line := range strings.Split(probe.Output, "\n") {
		if dirs.observe(line) {
			continue
		}
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
		prefix, anchored := dirs.prefix()
		seen = append(seen, sighting{
			path:     pathpkg.Clean(filepath.ToSlash(path)),
			lineNo:   lineNo,
			prefix:   prefix,
			anchored: anchored,
		})
	}

	// The suffix lift is decided once for the whole probe, not per diagnostic.
	// A probe ran in one working directory at a time, so lifts implying
	// directories that exclude one another cannot all be right, and each of
	// them looks perfectly unique on its own.
	unanchored := make([]string, 0, len(seen))
	for _, s := range seen {
		if !s.anchored {
			unanchored = append(unanchored, s.path)
		}
	}
	allowLift := known.liftsAgreeOnOneDirectory(unanchored)

	out := make([]string, 0, len(seen))
	for _, s := range seen {
		out = append(out, known.resolveDiagnosticPath(s.path, s.prefix, s.anchored, allowLift)+":"+s.lineNo)
	}
	return uniqueSorted(out)
}

// unattributedPrefix marks a diagnostic whose file could not be tied to a
// tracked path on evidence.
//
// Such a location stays in the list, because the count rules still need it: a
// new test failure reported only as "foo_test.go:12" is a real regression, and
// dropping it would hide one. What it must not do is reach rule (a) -- the rule
// that hard-fails a branch -- on a path chosen by coincidence. Prefixing it
// with a string no repository-relative path can equal keeps it counted and
// keeps it out of the changed-paths comparison.
//
// pathpkg.Clean never produces a leading "<", so a collision would take a
// tracked file deliberately named "<unattributed>".
const unattributedPrefix = "<unattributed>/"

// makeDirectoryTracker follows make's -w markers so a diagnostic can be read
// against the directory make was actually in.
//
// This is the evidence the suffix lift never had. make announces each sub-make
// with "Entering directory '/abs/path'" and closes it with a matching "Leaving
// directory", so the markers nest and a stack follows them exactly.
//
// A recipe doing its own "cd apps/api && mix compile" produces no marker: the
// shell changed directory, not make, and make has nothing to announce. Those
// diagnostics stay on the suffix lift, which is why the lift is kept rather
// than deleted -- but they now have to agree with one another before it is
// applied.
type makeDirectoryTracker struct {
	root  string
	stack []string
}

func newMakeDirectoryTracker(root string) *makeDirectoryTracker {
	if root != "" {
		if abs, err := filepath.Abs(root); err == nil {
			root = abs
		}
	}
	return &makeDirectoryTracker{root: root}
}

// observe consumes a directory marker and reports whether the line was one.
//
// Marker lines must not also be scanned for locations. They carry an absolute
// path, and an absolute path in tool output is what foreignDiagnosticLocations
// exists to reject -- make's own bookkeeping would be read as a diagnostic
// pointing outside the repository.
func (m *makeDirectoryTracker) observe(line string) bool {
	if dir, ok := makeDirectoryMarker(line, "Entering directory "); ok {
		m.stack = append(m.stack, dir)
		return true
	}
	if _, ok := makeDirectoryMarker(line, "Leaving directory "); ok {
		if len(m.stack) > 0 {
			m.stack = m.stack[:len(m.stack)-1]
		}
		return true
	}
	return false
}

// prefix returns the current directory relative to where the probe was
// launched, and whether make said where it was at all.
//
// An empty prefix with anchored true means the repository root. anchored false
// means there is no evidence to use, and the caller falls back to the suffix
// lift.
func (m *makeDirectoryTracker) prefix() (string, bool) {
	// An empty root is the only real absence of evidence: ExtractLocations has
	// no probe behind it and cannot say where anything ran.
	if m.root == "" {
		return "", false
	}
	// An empty stack is not absence. make was invoked in the repository root
	// -- runMakeTargetOnce sets cmd.Dir to it, and the tracked list is
	// relative to that same root -- so a diagnostic seen before any sub-make
	// is root-relative as printed. Reading this as "unknown" is what let a
	// root-run build's own dist/bundle.js get lifted onto an unrelated
	// src/dist/bundle.js: the suffix was unique, and nothing asked where make
	// actually was.
	if len(m.stack) == 0 {
		return "", true
	}
	rel, err := filepath.Rel(m.root, m.stack[len(m.stack)-1])
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return "", true
	}
	// A sub-make outside the repository says nothing about a repository path,
	// so it is absence of evidence rather than evidence of the root.
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

// isMakeDirectoryMarkerLine reports whether a line is one of make's -w
// directory announcements rather than tool output.
func isMakeDirectoryMarkerLine(line string) bool {
	if _, ok := makeDirectoryMarker(line, "Entering directory "); ok {
		return true
	}
	_, ok := makeDirectoryMarker(line, "Leaving directory ")
	return ok
}

func makeDirectoryMarker(line, kind string) (string, bool) {
	i := strings.Index(line, kind)
	if i < 0 {
		return "", false
	}
	// Require make's own prefix -- "make: " or "make[1]: " -- so a tool that
	// merely prints the phrase cannot move the tracker onto a directory make
	// never entered.
	if !strings.HasPrefix(strings.TrimSpace(line[:i]), "make") {
		return "", false
	}
	rest := strings.TrimSpace(line[i+len(kind):])
	// GNU make quotes the path as '...' under LC_ALL=C and as `...' in some
	// older builds.
	rest = strings.TrimSuffix(rest, "'")
	rest = strings.TrimPrefix(rest, "`")
	rest = strings.TrimPrefix(rest, "'")
	if rest == "" {
		return "", false
	}
	return rest, true
}

// trackedIndex answers the two questions normalizeTrackedPath needs about a
// revision's tracked paths: is this exact path tracked, and — for a tool that
// ran from a subdirectory — is there exactly one tracked path that ends with
// it. The suffix side is indexed by base name so a repository with tens of
// thousands of files costs a map lookup per diagnostic, not a full scan.
type trackedIndex struct {
	exact  map[string]struct{}
	byBase map[string][]string
}

func newTrackedIndex(tracked []string) trackedIndex {
	idx := trackedIndex{
		exact:  make(map[string]struct{}, len(tracked)),
		byBase: make(map[string][]string, len(tracked)),
	}
	for _, p := range tracked {
		p = filepath.ToSlash(p)
		idx.exact[p] = struct{}{}
		base := p
		if i := strings.LastIndexByte(p, '/'); i >= 0 {
			base = p[i+1:]
		}
		idx.byBase[base] = append(idx.byBase[base], p)
	}
	return idx
}

// resolveDiagnosticPath maps one diagnostic path onto a tracked path, using the
// directory make reported being in whenever there is one.
func (t trackedIndex) resolveDiagnosticPath(path, prefix string, anchored, allowLift bool) string {
	resolved := t.resolveOnEvidence(path, prefix, anchored, allowLift)
	if strings.ContainsRune(resolved, '/') {
		return resolved
	}
	// A name with no directory left is not evidence of a file. Go's testing
	// package decorates every t.Log line with filepath.Base, so
	// "version_test.go:10" is the ordinary output of a test that skipped
	// cleanly -- and this repository has a version_test.go at its root that
	// such a line matches exactly. Attributing it there hands a skip message to
	// rule (a), which hard-fails the branch for having touched the file.
	//
	// Knowing make was at the root does not rescue it. The decoration strips
	// the directory whatever the working directory was, so the name says
	// nothing about which package emitted it.
	//
	// The cost is real and is accepted on purpose: a diagnostic about a genuine
	// root-level source file now falls to the count rules instead of being
	// attributed. In this repository that is 2 files out of 498 tracked .go
	// files. The alternative is blocking innocent branches, which is the
	// failure this gate exists to prevent rather than to cause.
	return unattributedPrefix + resolved
}

func (t trackedIndex) resolveOnEvidence(path, prefix string, anchored, allowLift bool) string {
	if anchored {
		// Evidence beats coincidence. When make said where it was, the only
		// lift worth considering is the one that directory implies; a suffix
		// that matches somewhere else is exactly the accident being removed.
		if prefix != "" {
			if candidate := prefix + "/" + path; t.has(candidate) {
				return candidate
			}
		}
		if t.has(path) {
			return path
		}
		// Over-qualification is a separate phenomenon from the working
		// directory -- a wrapper prepending components prints a longer path
		// than the repository knows -- so stripping still applies.
		return t.stripToTracked(path)
	}
	// No evidence to prefer, so this is the historical rule unchanged:
	// exact, then lift, then strip. Routing through normalizeTrackedPath keeps
	// that rule in one place and keeps its tests describing live behavior --
	// a second copy here would let them pass while the gate did something
	// else, which is the failure mode this package keeps rediscovering.
	if allowLift {
		return normalizeTrackedPath(path, t)
	}
	if t.has(path) {
		return path
	}
	return t.stripToTracked(path)
}

// liftsAgreeOnOneDirectory reports whether the suffix lifts available for these
// paths could all have come from a single working directory.
//
// liftToTracked answers one path at a time, and the set can contradict itself
// while every answer in it looks unique. With apps/api/lib/foo.ex and
// apps/web/lib/bar.ex tracked, "lib/foo.ex" lifts to apps/api and "lib/bar.ex"
// to apps/web. One probe had one working directory, so at least one of those is
// wrong -- and a wrong lift is a hard block on a file the branch may never have
// touched. When the implied directories exclude one another, no lift in the
// probe is trusted.
//
// Nesting is not a contradiction. A path can be reported over-qualified, which
// implies a shallower directory for the same file: "api/lib/foo.ex" and
// "lib/foo.ex" lift to the same tracked path while implying "apps" and
// "apps/api". Agreement therefore means every implied directory lies on one
// root-to-leaf chain, not that they are all equal.
func (t trackedIndex) liftsAgreeOnOneDirectory(paths []string) bool {
	implied := make([]string, 0, len(paths))
	for _, p := range paths {
		lifted, ok := t.liftToTracked(p)
		if !ok {
			continue
		}
		dir := strings.TrimSuffix(lifted, "/"+p)
		if dir == lifted || dir == "" {
			continue
		}
		implied = append(implied, dir)
	}
	for i := range implied {
		for j := i + 1; j < len(implied); j++ {
			if !directoriesNest(implied[i], implied[j]) {
				return false
			}
		}
	}
	return true
}

func directoriesNest(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

func (t trackedIndex) has(path string) bool {
	_, ok := t.exact[path]
	return ok
}

// stripToTracked drops leading components until what remains is tracked,
// answering the over-qualified direction. It returns the fully stripped base
// name when nothing matches, which resolveDiagnosticPath then refuses to
// attribute.
func (t trackedIndex) stripToTracked(path string) string {
	original := path
	for {
		slash := strings.IndexByte(path, '/')
		if slash < 0 {
			// Nothing along the way was tracked, so no suffix of this path is
			// a better name for it than the path itself. Returning the last
			// component would invent a name the tool never printed and the
			// repository does not have -- and, because a bare name cannot be
			// attributed, would quietly downgrade a perfectly specific
			// diagnostic into an unattributed one.
			return original
		}
		path = path[slash+1:]
		if t.has(path) {
			return path
		}
	}
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

// liftToTracked resolves a diagnostic path emitted relative to a subdirectory
// back to its repository-root form. It answers only when exactly one tracked
// path ends with the given path: several candidates means the evidence does
// not say which file the tool meant, and picking one would attribute a
// diagnostic to a file the branch may never have touched.
//
// Several candidates means several distinct ones. The tracked list is not
// guaranteed to be a set, and a name repeated in it is still one file: refusing
// on the second sighting of the same path would turn a duplicate entry into a
// lost lift, which is ambiguity reported where none exists.
//
// A path with no directory component of its own is refused outright, however
// unique its match. Go's testing package decorates every t.Log and t.Errorf
// line with filepath.Base, so "a_test.go:10" is the ordinary output of a test
// that skipped cleanly — and suppressGoTestVerboseNoise cannot always blank
// it, because an interleaved "=== CONT" clears the pending buffer before the
// trailer that would identify the line as noise arrives. Lifting a bare base
// name would resolve that line to a real tracked path and hand it to rule (a),
// which is a hard failure: a branch that merely touched a test file would be
// blocked by a skip message present on both sides. A suffix long enough to
// carry a directory is weak evidence of the tool's working directory; a bare
// file name is no evidence at all, so the safe answer is the pre-existing
// one — do not lift, and let the changed-paths rule stay silent.
//
// Two costs come with that refusal, and neither is hypothetical.
//
// The refusal covers only this lift. One line up, normalizeTrackedPath still
// answers an exact tracked match, and at a repository root a bare name can be
// exactly tracked — this repository has version_test.go there. A cleanly
// skipping root-level test therefore still reaches rule (a) and hard-fails the
// branch. The gate below narrows that class; it does not close it, and saying
// the changed-paths rule stays silent would overstate what this does.
//
// The refusal also forfeits real attribution. A recipe doing
// "cd src && gcc -c foo.c" makes gcc print foo.c:12:5: with no directory of
// its own, and that diagnostic is now permanently unattributable even though
// src/foo.c is tracked and unambiguous. The trade is deliberate: a wrong
// attribution hard-blocks an innocent branch, while a missing one only falls
// back to the count heuristic. Both costs are paid off by the same fix —
// reading the tool's real working directory rather than guessing it from a
// suffix.
func (t trackedIndex) liftToTracked(path string) (string, bool) {
	i := strings.LastIndexByte(path, '/')
	if i < 0 {
		return "", false
	}
	base := path[i+1:]
	var found string
	for _, candidate := range t.byBase[base] {
		if !strings.HasSuffix(candidate, "/"+path) {
			continue
		}
		if found != "" && candidate != found {
			return "", false
		}
		found = candidate
	}
	return found, found != ""
}

// normalizeTrackedPath maps a diagnostic path onto the revision's tracked
// paths. Tools disagree about what a path is relative to, in both directions:
// a wrapper may print an over-qualified path, and a recipe that does
// "cd apps/api && mix compile" makes its tool print lib/foo.ex for a file the
// repository knows as apps/api/lib/foo.ex.
//
// Stripping alone only handled the first direction, so on a repository whose
// Makefile cds into a subdirectory — the shape missingCD already exists to
// cope with — every diagnostic path was ground down to a bare base name that
// matches no changed path. The changed-paths rule, the one piece of
// EvaluateBaseline that is direct evidence of harm rather than a count
// heuristic, could therefore never fire there at all.
//
// The lift is attempted on the path as emitted, before any component is
// stripped. Stripping first would grind a build-artifact copy such as
// _build/dev/lib/foo.ex down to lib/foo.ex and then lift that onto the source
// file it was generated from, which is a different file with a different
// history. Lifting first leaves it alone, because no tracked path ends with
// the artifact's full prefix.
//
// That ordering is not a guarantee, only the cheap half of one. It protects an
// artifact tree nested deeper than its source; an artifact emitted as
// dist/bundle.js still lifts onto a tracked src/dist/bundle.js, because a
// unique suffix match is a coincidence and this function cannot tell a
// coincidence from a working directory. The sound fix is to read the tool's
// real cwd — make's "Entering directory" markers, or the recipe cd that
// missingCD already parses — and treat a known prefix as exact.
//
// The lift is deliberately incomplete in the other direction too. A recipe
// doing "cd apps/api && mix compile" that reports mix.exs:3 — a name with no
// directory of its own — stays unlifted, because liftToTracked refuses a bare
// base name outright and cannot make an exception for one file. That is a
// false negative the changed-paths rule absorbs quietly; the alternative is
// the false positive that gate exists to prevent.
//
// The incoming path is cleaned rather than merely stripped of a leading "./",
// so lib//foo.ex and sub/../lib/foo.ex reach the same tracked file that
// lib/foo.ex does. Cleaning cannot invent a directory, so it never turns a
// bare base name into a liftable one.
func normalizeTrackedPath(path string, tracked trackedIndex) string {
	path = pathpkg.Clean(filepath.ToSlash(path))
	if _, ok := tracked.exact[path]; ok {
		return path
	}
	if lifted, ok := tracked.liftToTracked(path); ok {
		return lifted
	}
	return tracked.stripToTracked(path)
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
