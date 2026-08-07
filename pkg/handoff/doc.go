// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

// Package handoff decides whether work can safely move to another machine.
//
// It answers a different question than status or doctor. Those report how
// healthy a repository is; handoff reports whether anything in it exists only
// on this machine. Uncommitted files, unpushed commits, and stash entries are
// all invisible to every other device and to every agent, so leaving them
// behind is how work gets lost or duplicated.
//
// Assess is a pure function over the results of a bulk status scan, so the
// classification is testable without touching git.
//
// Blockers are split into two kinds. Auto-fixable ones (uncommitted work,
// unpushed commits) are exactly what "handoff end" resolves by committing and
// pushing. The rest — conflicts, an interrupted rebase or merge, a detached
// HEAD, a stash, a repository with no remote — need a decision that only the
// person at this machine can make, and no automation should guess at.
package handoff
