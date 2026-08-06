// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package gitsettings

import (
	"context"
	"fmt"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
)

// exitCodeNotFound is what "git config --get" returns when the key is unset.
const exitCodeNotFound = 1

// Inspect reads the current value of every recommended setting in the given
// scope and compares it against the recommendation.
//
// dir is the repository directory for ScopeLocal and is ignored for
// ScopeGlobal.
func Inspect(ctx context.Context, executor *gitcmd.Executor, scope Scope, dir string) (*Report, error) {
	version, err := executor.GetGitVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to determine git version: %w", err)
	}

	if scope == ScopeGlobal {
		dir = ""
	}

	report := &Report{
		Scope:      scope,
		GitVersion: version,
		Statuses:   make([]Status, 0, len(recommended)),
	}

	for _, setting := range recommended {
		status, err := inspectOne(ctx, executor, scope, dir, setting, version)
		if err != nil {
			return nil, err
		}
		report.Statuses = append(report.Statuses, status)
	}

	return report, nil
}

func inspectOne(
	ctx context.Context,
	executor *gitcmd.Executor,
	scope Scope,
	dir string,
	setting Setting,
	version string,
) (Status, error) {
	status := Status{Setting: setting}

	if setting.MinGit != "" && !versionAtLeast(version, setting.MinGit) {
		status.State = StateUnsupported
		return status, nil
	}

	result, err := executor.Run(ctx, dir, "config", scope.Flag(), "--get", setting.Key)
	if err != nil {
		return status, fmt.Errorf("failed to read git config %s: %w", setting.Key, err)
	}

	switch {
	case result.ExitCode == exitCodeNotFound:
		status.State = StateUnset
	case result.ExitCode != 0:
		return status, fmt.Errorf(
			"failed to read git config %s (exit %d): %s",
			setting.Key, result.ExitCode, strings.TrimSpace(result.Stderr),
		)
	default:
		status.Current = strings.TrimSpace(result.Stdout)
		if valuesEqual(status.Current, setting.Want) {
			status.State = StateOK
		} else {
			status.State = StateMismatch
		}
	}

	return status, nil
}

// Apply writes the recommended value for every status that needs a change.
// Statuses that already match, or that the installed git does not support, are
// left untouched. It returns the statuses that were written.
func Apply(
	ctx context.Context,
	executor *gitcmd.Executor,
	scope Scope,
	dir string,
	statuses []Status,
) ([]Status, error) {
	if scope == ScopeGlobal {
		dir = ""
	}

	applied := make([]Status, 0, len(statuses))
	for _, status := range statuses {
		if !status.NeedsChange() {
			continue
		}

		result, err := executor.Run(ctx, dir, "config", scope.Flag(), status.Key, status.Want)
		if err != nil {
			return applied, fmt.Errorf("failed to set git config %s: %w", status.Key, err)
		}
		if result.ExitCode != 0 {
			return applied, fmt.Errorf(
				"failed to set git config %s (exit %d): %s",
				status.Key, result.ExitCode, strings.TrimSpace(result.Stderr),
			)
		}

		applied = append(applied, status)
	}

	return applied, nil
}
