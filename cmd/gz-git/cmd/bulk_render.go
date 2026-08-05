package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// BulkRenderRow is the normalized per-repo view used by the shared bulk renderer.
type BulkRenderRow struct {
	Path, Branch, Status, Message, Remote string
	Err                                   error
	Duration                              time.Duration
	CommitsAhead, CommitsBehind           int
	PushedCommits                         int
	UncommittedFiles, UntrackedFiles      int
	Stashed, HasUncommittedChanges        bool
}

// BulkRenderInput is the aggregate bulk result view for rendering.
type BulkRenderInput struct {
	TotalScanned, TotalProcessed int
	Duration                     time.Duration
	Summary                      map[string]int
	Rows                         []BulkRenderRow
}

// BulkRenderConfig configures command-specific rendering policy.
type BulkRenderConfig struct {
	Title           string
	Verb            string
	Format          string
	Verbose         bool
	IssueStatuses   map[string]bool
	FormatStatus    func(row BulkRenderRow) string
	ChangesCount    func(row BulkRenderRow) int
	AlwaysShowError func(row BulkRenderRow) bool
	SuccessMessage  string
	// ShowFooters enables dirty-warning + auth-required footers (fetch/pull/push).
	ShowFooters bool
}

// bulkJSONEnvelope is the unified JSON schema for bulk commands.
type bulkJSONEnvelope struct {
	TotalScanned   int                    `json:"total_scanned"`
	TotalProcessed int                    `json:"total_processed"`
	DurationMs     int64                  `json:"duration_ms"`
	Summary        map[string]int         `json:"summary"`
	Repositories   []bulkJSONRepoEnvelope `json:"repositories"`
}

type bulkJSONRepoEnvelope struct {
	Path                  string `json:"path"`
	Branch                string `json:"branch,omitempty"`
	Status                string `json:"status"`
	Message               string `json:"message,omitempty"`
	CommitsAhead          int    `json:"commits_ahead,omitempty"`
	CommitsBehind         int    `json:"commits_behind,omitempty"`
	PushedCommits         int    `json:"pushed_commits,omitempty"`
	UncommittedFiles      int    `json:"uncommitted_files,omitempty"`
	UntrackedFiles        int    `json:"untracked_files,omitempty"`
	Stashed               bool   `json:"stashed,omitempty"`
	HasUncommittedChanges bool   `json:"has_uncommitted_changes,omitempty"`
	DurationMs            int64  `json:"duration_ms,omitempty"`
	Error                 string `json:"error,omitempty"`
}

// RenderBulkResults writes bulk operation results using the shared renderer.
func RenderBulkResults(w io.Writer, cfg BulkRenderConfig, in BulkRenderInput) {
	if cfg.Format == "json" || cfg.Format == "llm" {
		renderBulkStructured(w, cfg.Format, in)
		return
	}

	if cfg.Format == "compact" {
		renderBulkCompact(w, cfg, in)
	} else if cfg.Verbose {
		renderBulkVerbose(w, cfg, in)
	} else {
		renderBulkDefault(w, cfg, in)
	}

	if cfg.ShowFooters {
		renderBulkFooters(w, in)
	}
}

func renderBulkStructured(w io.Writer, format string, in BulkRenderInput) {
	output := bulkJSONEnvelope{
		TotalScanned:   in.TotalScanned,
		TotalProcessed: in.TotalProcessed,
		DurationMs:     in.Duration.Milliseconds(),
		Summary:        in.Summary,
		Repositories:   make([]bulkJSONRepoEnvelope, 0, len(in.Rows)),
	}
	for _, row := range in.Rows {
		repo := bulkJSONRepoEnvelope{
			Path:                  row.Path,
			Branch:                row.Branch,
			Status:                row.Status,
			Message:               row.Message,
			CommitsAhead:          row.CommitsAhead,
			CommitsBehind:         row.CommitsBehind,
			PushedCommits:         row.PushedCommits,
			UncommittedFiles:      row.UncommittedFiles,
			UntrackedFiles:        row.UntrackedFiles,
			Stashed:               row.Stashed,
			HasUncommittedChanges: row.HasUncommittedChanges,
			DurationMs:            row.Duration.Milliseconds(),
		}
		if row.Err != nil {
			repo.Error = row.Err.Error()
		}
		output.Repositories = append(output.Repositories, repo)
	}

	switch format {
	case "json":
		if err := cliutil.WriteJSON(w, output, true); err != nil {
			fmt.Fprintf(os.Stderr, "error writing JSON: %v\n", err)
		}
	case "llm":
		if err := cliutil.WriteLLM(w, output); err != nil {
			fmt.Fprintf(os.Stderr, "error writing LLM: %v\n", err)
		}
	}
}

func renderBulkCompact(w io.Writer, cfg BulkRenderConfig, in BulkRenderInput) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, cfg.Title)
	fmt.Fprintf(w, "Total scanned:   %d repositories\n", in.TotalScanned)
	fmt.Fprintf(w, "Total processed: %d repositories\n", in.TotalProcessed)
	fmt.Fprintf(w, "Duration:        %s\n", in.Duration.Round(100_000_000))
	fmt.Fprintln(w)
	if len(in.Summary) > 0 {
		fmt.Fprintln(w, "Summary by status:")
		for status, count := range in.Summary {
			icon := getBulkStatusIconSimple(status)
			fmt.Fprintf(w, "  %s %-15s %d\n", icon, status+":", count)
		}
		fmt.Fprintln(w)
	}
	hasIssues := false
	for _, row := range in.Rows {
		if isIssueStatus(cfg, row.Status) {
			if !hasIssues {
				fmt.Fprintln(w, "Issues found:")
				hasIssues = true
			}
			renderBulkRow(w, cfg, row)
		}
	}
	if !hasIssues {
		msg := cfg.SuccessMessage
		if msg == "" {
			msg = "✓ All repositories processed successfully"
		}
		fmt.Fprintln(w, msg)
	}
}

func renderBulkVerbose(w io.Writer, cfg BulkRenderConfig, in BulkRenderInput) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, cfg.Title)
	fmt.Fprintf(w, "Total scanned:   %d repositories\n", in.TotalScanned)
	fmt.Fprintf(w, "Total processed: %d repositories\n", in.TotalProcessed)
	fmt.Fprintf(w, "Duration:        %s\n", in.Duration.Round(100_000_000))
	fmt.Fprintln(w)
	if len(in.Summary) > 0 {
		fmt.Fprintln(w, "Summary by status:")
		for status, count := range in.Summary {
			icon := getBulkStatusIconSimple(status)
			fmt.Fprintf(w, "  %s %-15s %d\n", icon, status+":", count)
		}
		fmt.Fprintln(w)
	}
	if len(in.Rows) > 0 {
		fmt.Fprintln(w, "Repository details:")
		for _, row := range in.Rows {
			renderBulkRow(w, cfg, row)
		}
	}
}

func renderBulkDefault(w io.Writer, cfg BulkRenderConfig, in BulkRenderInput) {
	WriteSummaryLine(w, cfg.Verb, in.TotalProcessed, in.Summary, in.Duration)
	for _, row := range in.Rows {
		if isIssueStatus(cfg, row.Status) {
			renderBulkRow(w, cfg, row)
		}
	}
}

func isIssueStatus(cfg BulkRenderConfig, status string) bool {
	if cfg.IssueStatuses == nil {
		return false
	}
	return cfg.IssueStatuses[status]
}

func renderBulkRow(w io.Writer, cfg BulkRenderConfig, row BulkRenderRow) {
	changes := 0
	if cfg.ChangesCount != nil {
		changes = cfg.ChangesCount(row)
	}
	icon := getBulkStatusIcon(row.Status, changes)

	isDirty := row.UncommittedFiles > 0 || row.UntrackedFiles > 0
	if isDirty && row.Status != "error" && row.Status != "conflict" {
		icon = "⚠"
	}

	pathPart := row.Path
	if row.Branch != "" {
		pathPart += fmt.Sprintf(" (%s)", row.Branch)
	}

	statusStr := row.Status
	if cfg.FormatStatus != nil {
		statusStr = cfg.FormatStatus(row)
	}

	line := fmt.Sprintf("  %s %-50s %-18s", icon, pathPart, statusStr)
	if row.Duration > 0 {
		line += fmt.Sprintf(" %6s", row.Duration.Round(10_000_000))
	}
	if isDirty {
		line += fmt.Sprintf(" [dirty: %d uncommitted, %d untracked]", row.UncommittedFiles, row.UntrackedFiles)
	}
	fmt.Fprintln(w, line)

	if row.Status == "no-upstream" {
		fmt.Fprint(w, FormatUpstreamFixHint(row.Branch, row.Remote))
	}

	if row.Err != nil {
		showErr := row.Status == "error" || cfg.Verbose
		if cfg.AlwaysShowError != nil && cfg.AlwaysShowError(row) {
			showErr = true
		}
		if showErr {
			fmt.Fprintf(w, "    Error: %v\n", row.Err)
		}
	}
}

func renderBulkFooters(w io.Writer, in BulkRenderInput) {
	dirtyCount := 0
	var authPaths []string
	for _, row := range in.Rows {
		if row.UncommittedFiles > 0 || row.UntrackedFiles > 0 {
			dirtyCount++
		}
		if row.Status == repository.StatusAuthRequired {
			authPaths = append(authPaths, row.Path)
		}
	}
	if dirtyCount > 0 {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "⚠ Warning: %d repository(ies) have uncommitted changes\n", dirtyCount)
	}
	if len(authPaths) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "🔐 Authentication required for %d repository(ies):\n", len(authPaths))
		for _, path := range authPaths {
			fmt.Fprintf(w, "   • %s\n", path)
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, "💡 To fix: Configure credential helper or switch to SSH")
		fmt.Fprintln(w, "   git config --global credential.helper cache")
	}
}

// issueStatusSet builds an IssueStatuses map from a list of statuses.
func issueStatusSet(statuses ...string) map[string]bool {
	m := make(map[string]bool, len(statuses))
	for _, s := range statuses {
		m[s] = true
	}
	return m
}

// isRefspecError reports push refspec-style errors that should always be shown.
func isRefspecError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not found in repository") || strings.Contains(msg, "does not exist")
}
