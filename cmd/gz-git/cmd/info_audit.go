// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// defaultStaleStashAfter is when a stash stops being "I'll get back to this"
// and becomes work nobody knows exists. A stash lives in one clone and is
// invisible to every other machine, so age is the only signal that it has been
// forgotten.
const defaultStaleStashAfter = 14 * 24 * time.Hour

// runInfoAudit emits the machine-readable audit document.
//
// Exit codes follow the diagnostic convention already documented in
// pkg/cliutil/exit.go and used by `conflict detect` — 0 nothing found,
// 1 findings present, 2 execution error. Blockers are not a third exit code:
// the tool ran correctly when it reports one, and an agent learns the
// difference from audit_complete and summary.blockers, which it must read
// anyway before acting.
func runInfoAudit(
	w io.Writer,
	result *repository.BulkStatusResult,
	enrichment map[string]infoEnrichment,
	directory string,
	autofixOverrides map[string]bool,
	now time.Time,
) error {
	audit := buildAudit(result, enrichment, directory, autofixOverrides, now)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(audit); err != nil {
		return cliutil.NewExitError(cliutil.ExitPartialFailed,
			fmt.Errorf("failed to write audit: %w", err))
	}

	if audit.Summary.WithFindings > 0 {
		// Findings are the command's output, not a failure to produce it. The
		// message goes to the error stream so a caller piping stdout into a JSON
		// parser gets clean input.
		return cliutil.NewExitError(cliutil.ExitToolError,
			fmt.Errorf("%d of %d repositories have findings",
				audit.Summary.WithFindings, audit.Summary.Total))
	}
	return nil
}

// buildAudit converts a scan plus its enrichment into the audit document.
func buildAudit(
	result *repository.BulkStatusResult,
	enrichment map[string]infoEnrichment,
	directory string,
	autofixOverrides map[string]bool,
	now time.Time,
) repository.AuditResult {
	policy := repository.AutofixPolicyFrom(autofixOverrides)

	repos := make([]repository.AuditRepo, 0, len(result.Repositories))
	for i := range result.Repositories {
		status := &result.Repositories[i]
		enr := enrichment[status.Path]

		repos = append(repos, repository.EvaluateRepo(repository.AuditInput{
			Name:              auditRepoName(status),
			Path:              status.Path,
			Status:            status,
			Base:              enr.Base,
			Worktrees:         enr.Worktrees,
			PrunableWorktrees: enr.PrunableWorktrees,
			MergedBranches:    enr.MergedBranches,
			EnrichErr:         enr.Err,
			StaleStashAfter:   defaultStaleStashAfter,
			Now:               now,
			AutofixPolicy:     policy,
		}))
	}

	// Sorted by name so two runs over the same workspace produce byte-identical
	// documents. An agent diffing successive audits should see only what
	// actually changed, not the order the scanner happened to finish in.
	sort.Slice(repos, func(i, j int) bool { return repos[i].Name < repos[j].Name })

	return repository.AuditResult{
		Schema:       repository.AuditSchema,
		Directory:    directory,
		Repositories: repos,
		Summary:      repository.Summarize(repos),
	}
}

// auditRepoName prefers the path relative to the scan root, which is what a
// user typed and what identifies the repository within this workspace. It falls
// back to the absolute path rather than emitting an empty name.
func auditRepoName(status *repository.RepositoryStatusResult) string {
	if status.RelativePath != "" {
		return status.RelativePath
	}
	return status.Path
}
