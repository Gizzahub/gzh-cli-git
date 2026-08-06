// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/gitsettings"
)

var (
	recommendedApply  bool
	recommendedLocal  bool
	recommendedFormat string
)

var configRecommendedCmd = &cobra.Command{
	Use:   "recommended",
	Short: "Audit and apply the git settings a multi-device workflow needs",
	Long: cliutil.QuickStartHelp(`  # Audit global git config (read-only)
  gz-git config recommended

  # Apply the missing settings globally
  gz-git config recommended --apply

  # Audit or apply for the current repository only
  gz-git config recommended --local
  gz-git config recommended --local --apply

  # Machine-readable audit
  gz-git config recommended --format json

Exit Codes:
  0  every recommended setting already matches
  1  one or more settings differ from the recommendation
  2  the audit could not run (git missing, config unreadable)

Without --apply nothing is written; the command only reports drift, which
makes it safe to run in CI as a workstation conformance gate.`),
	RunE: runConfigRecommended,
}

func init() {
	configCmd.AddCommand(configRecommendedCmd)

	configRecommendedCmd.Flags().BoolVar(&recommendedApply, "apply", false,
		"write the recommended values (default is a read-only audit)")
	configRecommendedCmd.Flags().BoolVar(&recommendedLocal, "local", false,
		"target the current repository's config instead of the global one")
	configRecommendedCmd.Flags().StringVar(&recommendedFormat, "format", "", "output format (json)")
}

func runConfigRecommended(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	executor := gitcmd.NewExecutor()

	scope := gitsettings.ScopeGlobal
	dir := ""
	if recommendedLocal {
		scope = gitsettings.ScopeLocal
		cwd, err := os.Getwd()
		if err != nil {
			return cliutil.NewExitError(2, fmt.Errorf("failed to resolve working directory: %w", err))
		}
		if !executor.IsGitRepository(ctx, cwd) {
			return cliutil.NewExitError(2, fmt.Errorf("--local requires a git repository: %s is not one", cwd))
		}
		dir = cwd
	}

	report, err := gitsettings.Inspect(ctx, executor, scope, dir)
	if err != nil {
		return cliutil.NewExitError(2, err)
	}

	if !recommendedApply {
		return reportRecommended(report)
	}

	applied, err := gitsettings.Apply(ctx, executor, scope, dir, report.Statuses)
	if err != nil {
		printAppliedSettings(report.Scope, applied)
		return cliutil.NewExitError(2, err)
	}

	if recommendedFormat == "json" {
		return encodeJSON(map[string]any{"scope": report.Scope, "applied": applied})
	}

	printAppliedSettings(report.Scope, applied)
	return nil
}

// reportRecommended prints the audit and maps drift onto exit code 1.
func reportRecommended(report *gitsettings.Report) error {
	if recommendedFormat == "json" {
		if err := encodeJSON(report); err != nil {
			return cliutil.NewExitError(2, err)
		}
	} else {
		printRecommendedReport(report)
	}

	if pending := report.Pending(); len(pending) > 0 {
		return cliutil.NewExitError(1,
			fmt.Errorf("%d of %d recommended git settings need changes", len(pending), len(report.Statuses)))
	}
	return nil
}

func encodeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printRecommendedReport(report *gitsettings.Report) {
	fmt.Println()
	fmt.Printf("%sgit workflow settings%s (%s scope, git %s)\n",
		cliutil.ColorCyanBold, cliutil.ColorReset, report.Scope, report.GitVersion)
	fmt.Println()

	var ok, pending, unsupported int
	for _, s := range report.Statuses {
		switch s.State {
		case gitsettings.StateOK:
			ok++
			fmt.Printf("  %s✓%s %-22s %s\n", cliutil.ColorGreen, cliutil.ColorReset, s.Key, s.Current)
		case gitsettings.StateUnsupported:
			unsupported++
			fmt.Printf("  %s⊘%s %-22s requires git %s+\n",
				cliutil.ColorGray, cliutil.ColorReset, s.Key, s.MinGit)
		case gitsettings.StateUnset, gitsettings.StateMismatch:
			pending++
			fmt.Printf("  %s⚠%s %-22s %s → %s\n",
				cliutil.ColorYellow, cliutil.ColorReset, s.Key, currentOrUnset(s), s.Want)
			fmt.Printf("      %s%s%s\n", cliutil.ColorGray, s.Why, cliutil.ColorReset)
		}
	}

	fmt.Println()
	fmt.Printf("%d settings: %d ok, %d need changes", len(report.Statuses), ok, pending)
	if unsupported > 0 {
		fmt.Printf(", %d unsupported by git %s", unsupported, report.GitVersion)
	}
	fmt.Println()

	if pending > 0 {
		fmt.Printf("\nApply with: gz-git config recommended%s --apply\n", localFlagSuffix(report.Scope))
	}
	fmt.Println()
}

func printAppliedSettings(scope gitsettings.Scope, applied []gitsettings.Status) {
	fmt.Println()
	if len(applied) == 0 {
		fmt.Printf("All recommended git settings already match (%s scope).\n\n", scope)
		return
	}

	for _, s := range applied {
		fmt.Printf("  %s✓%s %s = %s\n", cliutil.ColorGreen, cliutil.ColorReset, s.Key, s.Want)
	}
	fmt.Printf("\nApplied %d setting(s) to the %s git config.\n\n", len(applied), scope)
}

func currentOrUnset(s gitsettings.Status) string {
	if s.State == gitsettings.StateUnset {
		return "(unset)"
	}
	return s.Current
}

func localFlagSuffix(scope gitsettings.Scope) string {
	if scope == gitsettings.ScopeLocal {
		return " --local"
	}
	return ""
}
