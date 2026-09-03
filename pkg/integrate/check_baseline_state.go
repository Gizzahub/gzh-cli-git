// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
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
	if enumerationAccountsForEveryLine(base, baseTracked) {
		return BaseMeasured
	}
	return BaseMeasurementUnknown
}

// enumerationAccountsForEveryLine reports whether the output is an enumeration
// of tracked files rather than a run that merely mentioned one on its way to
// dying.
//
// Naming a tracked file is necessary evidence and not sufficient evidence. A
// Makefile recipe is free to echo the path it is about to check and then die
// because the checker was never installed:
//
//	scripts/deploy.sh
//	/bin/sh: shellcheck: command not found
//	make: *** [check] Error 127
//
// That run named a tracked file and measured nothing, so the test cannot be
// "does some line match".
//
// An earlier version of this file answered that with a list of launch-failure
// wordings, screened before the match. That was wrong in a way worth recording,
// because the reasoning that justified it sounded safe: a screen only ever
// withdraws BaseMeasured, so a wording it fails to recognize was supposed to
// land on the downgradable side. It does not. The screen withdraws, but the
// match that follows it grants, and the two compose the other way round — an
// unrecognized wording falls through to a line that says `scripts/deploy.sh`
// and grants BaseMeasured on it. `Permission denied`, `No such file or
// directory` and a Python traceback all did exactly that, each producing an
// undowngradable "diagnostic count increased (0 -> N)": the TASK-134 failure,
// restored narrowly, once per wording nobody had thought of yet.
//
// The rule here is positional instead, and needs no wordlist. Every non-empty
// line must be one of: make's own structural output, a diff structure line, or
// a path tracked at the target SHA. An unaccounted line is tolerated BEFORE the
// first tracked path and disqualifying AFTER it. The asymmetry is the whole
// idea, and it comes from the order the shell necessarily prints in:
//
//   - make echoes each recipe before running it, so `gofmt -l ./...` precedes
//     the tool's own output. Rejecting unaccounted lines outright would make
//     this function inert in every repository that does not `@`-silence its
//     recipes, which is most of them — green tests, dead feature.
//   - a failure that killed the tool necessarily follows any path the recipe
//     had already echoed.
//
// So a leading unknown line is an echo and a trailing one is a symptom.
//
// One semantic exception to "make's output is structural": make reports the
// shell's 127 as `Error 127`, and 127 is POSIX for a command that was never
// found. That trailer disqualifies however well-formed it looks, which is what
// catches an echo-then-die whose shell printed nothing of its own.
//
// The failure direction is now genuinely one-way. A structural line this
// function does not recognize reads as unaccounted, which yields
// BaseMeasurementUnknown, which leaves the operator the escape hatch they had
// before the discriminator existed. A tool that prints a legitimate summary
// line after its findings ("3 files need formatting") is read as unmeasured for
// that reason, and that is the correct side to be wrong on.
//
// A path with a :line on it is not this signature. Those are ordinary
// diagnostics, ExtractLocations already counts them, and their presence means
// len(base) != 0 so none of this is consulted.
func enumerationAccountsForEveryLine(probe makeProbe, tracked []string) bool {
	if len(tracked) == 0 {
		return false
	}
	known := make(map[string]struct{}, len(tracked))
	for _, p := range tracked {
		known[filepath.ToSlash(p)] = struct{}{}
	}
	// A path printed inside a sub-make is relative to that sub-directory,
	// while tracked is relative to the repository root. Without the rebase,
	// `x.go` from a recursive `make check` never matches the tracked
	// `sub/x.go` and a genuine measured zero reads as unmeasured. This is the
	// same correction extractLocationsForProbe already applies, and the two
	// functions have to agree about what a printed path means.
	dirs := newMakeDirectoryTracker(probe.WorkDir)
	// Diff markers, longest first: "--- a/x" must not be read as the file
	// literally named "a/x".
	markers := []string{"--- a/", "+++ b/", "--- ", "+++ "}

	seenTrackedPath := false
	inDiff := false
	for _, line := range strings.Split(probe.Output, "\n") {
		if dirs.observe(line) {
			continue
		}
		trimmed := strings.TrimSpace(stripANSI(line))
		if trimmed == "" {
			continue
		}

		if code, ok := makeRecipeError(trimmed); ok {
			if code == 127 {
				return false
			}
			continue
		}

		candidate := trimmed
		isDiffHeader := false
		for _, m := range markers {
			if rest, found := strings.CutPrefix(candidate, m); found {
				candidate = strings.TrimSpace(rest)
				isDiffHeader = true
				break
			}
		}
		if isDiffHeader {
			inDiff = true
		}
		if strings.HasPrefix(trimmed, "@@") {
			inDiff = true
			continue
		}

		if prefix, anchored := dirs.prefix(); anchored && prefix != "" {
			candidate = prefix + "/" + candidate
		}
		if _, ok := known[candidate]; ok {
			seenTrackedPath = true
			continue
		}
		if isDiffHeader {
			// A diff header naming something untracked is still diff
			// structure, not a stray line.
			continue
		}
		if inDiff && isDiffBodyLine(line) {
			continue
		}

		// Unaccounted. Before the first tracked path this is a recipe echo;
		// after it, it is the symptom of a run that stopped measuring.
		if seenTrackedPath {
			return false
		}
	}
	return seenTrackedPath
}

// makeRecipeError parses make's own recipe-failure trailer and returns the exit
// status the recipe reported.
//
//	make: *** [check] Error 1
//	make[1]: *** [Makefile:2: check] Error 127
func makeRecipeError(trimmed string) (int, bool) {
	m := makeRecipeErrorPattern.FindStringSubmatch(trimmed)
	if m == nil {
		return 0, false
	}
	code, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return code, true
}

var makeRecipeErrorPattern = regexp.MustCompile(`^make(?:\[\d+\])?: \*\*\* \[[^\]]*\] Error (\d+)$`)

// isDiffBodyLine reports whether a line is unified-diff payload rather than a
// line of its own. Checked against the raw line: leading whitespace is a
// context line and TrimSpace would erase the distinction.
func isDiffBodyLine(raw string) bool {
	if raw == "" {
		return false
	}
	switch raw[0] {
	case '+', '-', ' ', '\\':
		return true
	}
	return false
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
