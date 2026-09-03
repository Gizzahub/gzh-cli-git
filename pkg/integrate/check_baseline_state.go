// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"fmt"
	"path/filepath"
	"strings"
)

// BaseMeasurement says what zero file:line diagnostics from a failed target
// tip actually mean.
//
// len(base) == 0 conflates two states that need opposite handling:
//
//   - The run died before it could analyze anything — mix without deps/, a
//     test runner without node_modules/, a venv-less pytest. No baseline
//     exists, so the gate was skipped, and --allow-skipped-checks may
//     downgrade it. This is the state BaselineUnmeasurable was added for.
//   - The run went all the way through and reported a failure in a shape that
//     carries no file:line at all — gofmt -l, gci diff. That is a measurement
//     of zero, so a branch that emits N has genuinely worsened it, and
//     --allow-skipped-checks must not excuse that: the flag's contract is
//     "this check was skipped", and this check was not.
//
// The burden of proof sits on the second state, and that direction is the
// whole design. Removing the operator's escape hatch is the destructive
// action: get it wrong and a repository cannot land anything, which is the
// failure TASK-134 fixed and which this must not reintroduce. Leaving the
// hatch in place when it was not warranted costs one downgradable warning.
// So "measured" has to be positively evidenced, and everything else — an
// unrecognized failure, a crash, a bare `exit 1`, a caller that filled
// nothing in — stays unmeasured.
type BaseMeasurement int

const (
	// BaseMeasurementUnknown is the zero value, and it reads as unmeasured.
	// A caller that says nothing gets exactly the behavior it had before this
	// field existed: an empty base is an absent baseline. EvaluateBaseline and
	// BaselineInput are both exported, so the zero value is what every caller
	// outside this package already passes, and silently flipping their verdict
	// from a downgradable skip to a hard count failure is not a change this
	// field is entitled to make on their behalf.
	BaseMeasurementUnknown BaseMeasurement = iota
	// BaseMeasured is set only when the target-tip probe showed evidence it
	// enumerated repository files, which it could not have done without
	// reaching the analysis stage. Its zero is then a real zero.
	BaseMeasured
)

// PrepareState names the tree a probe was measured in.
//
// It exists because the two probes are not prepared alike. With no controller
// profile the branch is measured in the live working directory, where deps/,
// node_modules/ and .venv already sit from earlier runs, while the baseline is
// measured in a worktree checked out fresh at the target SHA, where none of
// them do. That asymmetry — not the target tip's own health — is what makes a
// bootstrap-hungry checker die on the baseline side only, so it has to be
// reportable as the reason rather than left as an unstated premise.
type PrepareState string

const (
	// PrepareStateUnknown means the caller did not say, so no claim about
	// symmetry can be made in either direction.
	PrepareStateUnknown PrepareState = ""
	// PrepareStateWorkingDir is the live repository working directory, with
	// whatever build artifacts previous runs left in it.
	PrepareStateWorkingDir PrepareState = "the live working directory"
	// PrepareStatePristine is a worktree checked out fresh at one SHA and
	// never bootstrapped.
	PrepareStatePristine PrepareState = "a pristine worktree"
	// PrepareStateProfilePrepared is a fresh worktree that runPrepareProfile
	// then bootstrapped. Both sides get it, so those two probes are symmetric
	// and the asymmetry clause below stays silent.
	PrepareStateProfilePrepared PrepareState = "a profile-prepared worktree"
)

// baseMeasurement classifies a target-tip probe: is there evidence its run
// reached the stage where it could have reported a file:line diagnostic?
//
// Only a positive signature answers "yes". TASK-141 ruled out every
// discriminator that was already here — toolCrashSignature matches Go runtime
// panics only, "output is non-empty" is true of both states, and probe.Err is
// non-nil on both by the time either call site reaches this — so the evidence
// has to be new, and the only honest source of it is the output itself.
func baseMeasurement(base makeProbe, baseTracked []string) BaseMeasurement {
	// Screened first, and deliberately so. A tool that never launched cannot
	// have measured anything, however much of the repository the recipe named
	// on its way to failing, and reading such a run as a measured zero is what
	// takes the operator's escape hatch away — the TASK-134 failure. This is a
	// prose signature and shares that family's weakness, but it only ever
	// withdraws BaseMeasured and never grants it, so a shape it fails to
	// recognize lands on the downgradable side rather than on the side where
	// the repository cannot land anything.
	if toolNeverLaunched(base.Output) != "" {
		return BaseMeasurementUnknown
	}
	if enumeratedTrackedFile(base, baseTracked) != "" {
		return BaseMeasured
	}
	return BaseMeasurementUnknown
}

// toolNeverLaunched returns the line proving the checker was absent rather
// than unhappy, or "" when there is no such line.
//
// Both shapes come from the shell, not from the tool: the shell's own
// "command not found" and the 127 it exits with, which make then reports as
// `Error 127`. A tool that ran and disliked the code exits with its own
// status and prints its own diagnostics, so neither line appears.
func toolNeverLaunched(out string) string {
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(stripANSI(line))
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, "command not found") {
			return trimmed
		}
		// make's own report of the shell's 127. Matched with the surrounding
		// "Error " so a diagnostic that merely contains the number 127 does
		// not read as a launch failure.
		if strings.Contains(trimmed, "Error 127") {
			return trimmed
		}
	}
	return ""
}

// enumeratedTrackedFile returns the first line of tool output that names a
// file tracked at the target SHA and nothing else, or "" when no line does.
//
// The reasoning is that naming a repository file is usually something only a
// tool that read the repository can do. `gofmt -l` prints one tracked path per
// line; `gci -d` and `gofmt -d` print them behind diff markers. A run that died
// for want of deps/ or node_modules/ prints prose about the missing dependency
// instead.
//
// "Usually" is doing real work in that sentence and an earlier version of this
// comment left it out, which made the claim false. A Makefile recipe is free to
// echo the path it is about to check as a progress line and then die because
// the checker was never installed:
//
//	scripts/deploy.sh
//	/bin/sh: shellcheck: command not found
//	make: *** [check] Error 127
//
// That run named a tracked file and measured nothing. Enumeration is therefore
// necessary evidence and not sufficient evidence, so baseMeasurement screens
// for a tool that never launched before it consults this function at all.
//
// A path with a :line on it is not this signature. Those are ordinary
// diagnostics, ExtractLocations already counts them, and their presence means
// len(base) != 0 so none of this is consulted.
//
// It is an allowlist and deliberately narrow. An unrecognized failure shape is
// read as unmeasured, which leaves the operator the escape hatch they had
// before; the opposite default takes it away on a guess. The list is expected
// to grow one measured shape at a time, and the cost of a missing entry is a
// warning rather than a repository that cannot land anything.
func enumeratedTrackedFile(probe makeProbe, tracked []string) string {
	if len(tracked) == 0 {
		return ""
	}
	known := make(map[string]struct{}, len(tracked))
	for _, p := range tracked {
		known[filepath.ToSlash(p)] = struct{}{}
	}
	// A path printed inside a sub-make is relative to that sub-directory,
	// while baseTracked is relative to the repository root. Without the
	// rebase, `x.go` from a recursive `make check` never matches the tracked
	// `sub/x.go` and a genuine measured zero reads as unmeasured. This is the
	// same correction extractLocationsForProbe already applies, and the two
	// functions have to agree about what a printed path means.
	dirs := newMakeDirectoryTracker(probe.WorkDir)
	// Diff markers, longest first: "--- a/x" must not be read as the file
	// literally named "a/x".
	markers := []string{"--- a/", "+++ b/", "--- ", "+++ "}
	for _, line := range strings.Split(probe.Output, "\n") {
		if dirs.observe(line) {
			continue
		}
		trimmed := strings.TrimSpace(stripANSI(line))
		if trimmed == "" {
			continue
		}
		for _, m := range markers {
			if rest, found := strings.CutPrefix(trimmed, m); found {
				trimmed = strings.TrimSpace(rest)
				break
			}
		}
		if prefix, anchored := dirs.prefix(); anchored && prefix != "" {
			trimmed = prefix + "/" + trimmed
		}
		if _, ok := known[trimmed]; ok {
			return trimmed
		}
	}
	return ""
}

// unmeasurableReason states why there is no baseline to compare against.
//
// When the two probes were not prepared alike it names that asymmetry,
// because the asymmetry — not the target tip — is what stopped the checker
// from running: the branch is measured in a tree bootstrap artifacts already
// sit in, the baseline in a tree that has none, so the same `make` target is
// not the same experiment on the two sides. Reporting only "the target tip
// failed" hands the operator a fact about the wrong tree and points the
// investigation at the target commit, which is innocent.
func unmeasurableReason(branchCount int, branchPrepared, basePrepared PrepareState) string {
	reason := fmt.Sprintf(
		"target tip failed without emitting any file:line diagnostic, so there is no baseline to compare (branch had %d)",
		branchCount,
	)
	if branchPrepared == PrepareStateUnknown || basePrepared == PrepareStateUnknown {
		return reason
	}
	if branchPrepared == basePrepared {
		return reason
	}
	return reason + fmt.Sprintf(
		"; the two runs were not prepared alike — the branch ran in %s and the baseline in %s — so bootstrap artifacts present for the branch were absent for the baseline",
		branchPrepared, basePrepared,
	)
}
