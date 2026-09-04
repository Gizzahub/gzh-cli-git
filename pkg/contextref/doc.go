// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

// Package contextref observes a repository's tracked context-reference
// manifest and aggregates CE v2 gate-doctor output.
//
// It reports identity and state only. It does not load agent instructions,
// interpret Skill content, execute descriptor commands, or mutate Git
// configuration, the index, worktree files, modes, or hooks.
//
// The consumed CE contract is origin tag v0.8.3
// (ac7445978423df45cb77ffaea0e34f7725e744b2), capability
// ce.task.gate-doctor/v2. TASK-083 c4913da6 and a local ce --version are
// not that release.
package contextref
