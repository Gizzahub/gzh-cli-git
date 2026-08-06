// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package doctor

import (
	"context"
	"fmt"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/gitsettings"
)

// checkGitWorkflowSettings audits the user's global git config against the
// settings a multi-device workflow depends on. The default output is a single
// aggregate check so doctor does not grow seven lines of noise; verbose mode
// expands it to one check per setting.
func checkGitWorkflowSettings(ctx context.Context, verbose bool) []CheckResult {
	executor := gitcmd.NewExecutor()

	report, err := gitsettings.Inspect(ctx, executor, gitsettings.ScopeGlobal, "")
	if err != nil {
		return []CheckResult{{
			Name:     "git workflow settings",
			Category: CategorySystem,
			Status:   StatusWarning,
			Message:  "could not read global git config",
			Detail:   err.Error(),
		}}
	}

	if verbose {
		return verboseSettingChecks(report)
	}

	pending := report.Pending()
	if len(pending) == 0 {
		return []CheckResult{{
			Name:     "git workflow settings",
			Category: CategorySystem,
			Status:   StatusOK,
			Message: fmt.Sprintf("git workflow settings: %d recommended values set",
				len(report.Statuses)-len(report.Unsupported())),
		}}
	}

	keys := make([]string, 0, len(pending))
	for _, s := range pending {
		keys = append(keys, s.Key)
	}

	return []CheckResult{{
		Name:     "git workflow settings",
		Category: CategorySystem,
		Status:   StatusWarning,
		Message: fmt.Sprintf("git workflow settings: %d of %d need changes",
			len(pending), len(report.Statuses)),
		Detail: strings.Join(keys, ", ") + " — fix with: gz-git config recommended --apply",
	}}
}

// verboseSettingChecks expands the audit into one result per setting.
func verboseSettingChecks(report *gitsettings.Report) []CheckResult {
	checks := make([]CheckResult, 0, len(report.Statuses))
	for _, s := range report.Statuses {
		check := CheckResult{
			Name:     s.Key,
			Category: CategorySystem,
		}

		switch s.State {
		case gitsettings.StateOK:
			check.Status = StatusOK
			check.Message = fmt.Sprintf("%s = %s", s.Key, s.Current)
		case gitsettings.StateUnsupported:
			check.Status = StatusSkipped
			check.Message = fmt.Sprintf("%s requires git %s+ (have %s)", s.Key, s.MinGit, report.GitVersion)
		case gitsettings.StateUnset:
			check.Status = StatusWarning
			check.Message = fmt.Sprintf("%s is unset, recommended %s", s.Key, s.Want)
			check.Detail = s.Why
		case gitsettings.StateMismatch:
			check.Status = StatusWarning
			check.Message = fmt.Sprintf("%s = %s, recommended %s", s.Key, s.Current, s.Want)
			check.Detail = s.Why
		}

		checks = append(checks, check)
	}
	return checks
}
