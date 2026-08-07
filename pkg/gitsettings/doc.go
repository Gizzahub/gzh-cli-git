// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

// Package gitsettings audits and applies the git configuration settings that
// make a multi-device, multi-agent workflow safe.
//
// The recommended set is intentionally small and opinionated: each setting
// removes a class of mistake that shows up when the same branch is worked on
// from more than one machine (accidental merge commits from pull, stale remote
// branch references, missing upstream on first push, repeated conflict
// resolution).
//
// Values are read with "git config --get" so unset and mismatched settings are
// distinguished; settings that need a newer git than the one installed are
// reported as unsupported rather than silently skipped.
//
// Usage:
//
//	statuses, err := gitsettings.Inspect(ctx, exec, gitsettings.ScopeGlobal, "")
//	applied, err := gitsettings.Apply(ctx, exec, gitsettings.ScopeGlobal, "", statuses)
package gitsettings
