package cmd

import (
	"path/filepath"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/branch"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

type cleanupBranchJSONOutput struct {
	Schema         string                        `json:"schema"`
	DryRun         bool                          `json:"dry_run"`
	BotsOnly       bool                          `json:"bots_only"`
	DeleteRemote   bool                          `json:"delete_remote"`
	TotalScanned   int                           `json:"total_scanned"`
	TotalProcessed int                           `json:"total_processed"`
	DurationMs     int64                         `json:"duration_ms"`
	Summary        map[string]int                `json:"summary"`
	Repositories   []cleanupBranchRepoJSONOutput `json:"repositories"`
}

type cleanupBranchRepoJSONOutput struct {
	Path     string                          `json:"path"`
	Branch   string                          `json:"branch,omitempty"`
	Status   string                          `json:"status"`
	Error    string                          `json:"error,omitempty"`
	Branches []repository.CleanupBranchEntry `json:"branches,omitempty"`
	// FailedBranches carries the per-branch refusals and their remedy text. The
	// repository-level Error above is a different thing — the gate sets Status
	// and FailedBranches, never Error — so without this field a machine caller
	// saw the candidate list shrink with no stated reason, in the one channel
	// where there is no scrollback to fall back on.
	FailedBranches []repository.CleanupFailureEntry `json:"failed_branches,omitempty"`
	// RetireRefusals carries trunks the non-canonical gate examined and
	// declined. It is separate from FailedBranches because nothing was
	// attempted: a caller that treats failed_branches as an error signal must
	// not start reporting healthy repositories as broken. Without this field the
	// machine channel has the same gap the terminal had — a shorter candidate
	// list and no stated reason.
	RetireRefusals []repository.RetireRefusalEntry `json:"retire_refusals,omitempty"`
}

func writeCleanupBranchJSON(output cleanupBranchJSONOutput) {
	writeBulkOutput(cleanupBranchBulkFlags.Format, output)
}

func cleanupBranchJSONFromBulk(result *repository.BulkCleanupResult, dryRun bool) cleanupBranchJSONOutput {
	out := cleanupBranchJSONOutput{
		Schema:         cleanupBranchSchema,
		DryRun:         dryRun,
		BotsOnly:       cleanupBranchBots,
		DeleteRemote:   cleanupBranchRemote,
		TotalScanned:   result.TotalScanned,
		TotalProcessed: result.TotalProcessed,
		DurationMs:     result.Duration.Milliseconds(),
		Summary:        result.Summary,
		Repositories:   make([]cleanupBranchRepoJSONOutput, 0, len(result.Repositories)),
	}
	for _, repo := range result.Repositories {
		entry := cleanupBranchRepoJSONOutput{
			Path:           repo.RelativePath,
			Branch:         repo.Branch,
			Status:         repo.Status,
			Branches:       repo.Branches,
			FailedBranches: repo.FailedBranches,
			RetireRefusals: repo.RetireRefusals,
		}
		if repo.Error != nil {
			entry.Error = repo.Error.Error()
		}
		out.Repositories = append(out.Repositories, entry)
	}
	return out
}

func cleanupBranchJSONFromSingle(
	repo *repository.Repository,
	currentBranch string,
	entries []repository.CleanupBranchEntry,
	status string,
	errMsg string,
	start time.Time,
) cleanupBranchJSONOutput {
	summary := map[string]int{status: 1}
	path := "."
	if repo != nil && repo.Path != "" {
		path = filepath.Base(repo.Path)
	}
	return cleanupBranchJSONOutput{
		Schema:         cleanupBranchSchema,
		DryRun:         cleanupBranchDryRun,
		BotsOnly:       cleanupBranchBots,
		DeleteRemote:   cleanupBranchRemote,
		TotalScanned:   1,
		TotalProcessed: 1,
		DurationMs:     time.Since(start).Milliseconds(),
		Summary:        summary,
		Repositories: []cleanupBranchRepoJSONOutput{{
			Path:     path,
			Branch:   currentBranch,
			Status:   status,
			Error:    errMsg,
			Branches: entries,
		}},
	}
}

// retireRefusalEntriesFromReport converts the single engine's refusals to the
// bulk engine's wire shape, so one schema describes both paths.
func retireRefusalEntriesFromReport(report *branch.CleanupReport) []repository.RetireRefusalEntry {
	if report == nil || len(report.Refused) == 0 {
		return nil
	}
	out := make([]repository.RetireRefusalEntry, 0, len(report.Refused))
	for _, r := range report.Refused {
		loc := "local"
		if r.IsRemote {
			loc = "remote"
		}
		out = append(out, repository.RetireRefusalEntry{Name: r.Branch, Location: loc, Reason: r.Reason})
	}
	return out
}

func cleanupEntriesFromReport(report *branch.CleanupReport) []repository.CleanupBranchEntry {
	if report == nil {
		return nil
	}
	var out []repository.CleanupBranchEntry
	appendEntries := func(list []*branch.Branch, reason string) {
		for _, b := range list {
			if b == nil {
				continue
			}
			loc := "local"
			if b.IsRemote {
				loc = "remote"
			}
			// The basis travels with the entry for the same reason the human
			// preview grew a label: a consumer reading "master, non-canonical"
			// cannot tell a local trunk from another machine's ref, and the two
			// carry different risk. Only non-canonical entries have one — the
			// other reasons are not measured against a target ref.
			targetRef, targetSHA := reportBasis(report, b)
			out = append(out, repository.CleanupBranchEntry{
				Name:      b.Name,
				Reason:    reason,
				Location:  loc,
				Kind:      repository.BotKind(b.Name),
				TargetRef: targetRef,
				TargetSHA: targetSHA,
			})
		}
	}
	appendEntries(report.Merged, "merged")
	appendEntries(report.Stale, "stale")
	appendEntries(report.Orphaned, "gone")
	appendEntries(report.Superseded, "superseded")
	appendEntries(report.NonCanonical, "non-canonical")
	return out
}

// reportBasis looks up what a candidate was measured against. It shares its
// lookup with retireBasisLabel deliberately: the human line and the JSON field
// must not be able to disagree about the same branch.
func reportBasis(report *branch.CleanupReport, b *branch.Branch) (targetRef, targetSHA string) {
	ref := b.Ref
	if ref == "" {
		ref = b.Name
	}
	for _, basis := range report.Bases {
		if basis.Ref == ref {
			return basis.TargetRef, basis.TargetSHA
		}
	}
	return "", ""
}

func filterCleanupEntries(entries []repository.CleanupBranchEntry, names []string) []repository.CleanupBranchEntry {
	if len(names) == 0 {
		return nil
	}
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	var out []repository.CleanupBranchEntry
	for _, e := range entries {
		if want[e.Name] {
			out = append(out, e)
		}
	}
	return out
}
