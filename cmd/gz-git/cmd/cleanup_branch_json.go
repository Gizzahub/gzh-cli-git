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
			Path:     repo.RelativePath,
			Branch:   repo.Branch,
			Status:   repo.Status,
			Branches: repo.Branches,
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
			out = append(out, repository.CleanupBranchEntry{
				Name:     b.Name,
				Reason:   reason,
				Location: loc,
				Kind:     repository.BotKind(b.Name),
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
