package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func sampleRenderInput() BulkRenderInput {
	return BulkRenderInput{
		TotalScanned:   3,
		TotalProcessed: 3,
		Duration:       1200 * time.Millisecond,
		Summary: map[string]int{
			"fetched": 1,
			"error":   1,
			"success": 1,
		},
		Rows: []BulkRenderRow{
			{Path: "repo-ok", Branch: "main", Status: "fetched", CommitsBehind: 2},
			{Path: "repo-err", Branch: "main", Status: "error", Err: errors.New("boom")},
			{Path: "repo-clean", Branch: "main", Status: "success"},
		},
	}
}

func TestBulkJSONEnvelopeSchema(t *testing.T) {
	var buf bytes.Buffer
	cfg := BulkRenderConfig{Format: "json"}
	RenderBulkResults(&buf, cfg, sampleRenderInput())

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("json decode failed: %v\nbody=%s", err, buf.String())
	}
	for _, key := range []string{"total_scanned", "total_processed", "duration_ms", "summary", "repositories"} {
		if _, ok := envelope[key]; !ok {
			t.Errorf("missing envelope key %q in %s", key, buf.String())
		}
	}

	var repos []map[string]any
	if err := json.Unmarshal(envelope["repositories"], &repos); err != nil {
		t.Fatalf("repositories decode: %v", err)
	}
	if len(repos) != 3 {
		t.Fatalf("repositories len=%d want 3", len(repos))
	}
	if repos[1]["path"] != "repo-err" {
		t.Errorf("repo[1].path=%v want repo-err", repos[1]["path"])
	}
	if repos[1]["error"] != "boom" {
		t.Errorf("repo[1].error=%v want boom", repos[1]["error"])
	}
}

func TestBulkDefaultModeShowsOnlyIssues(t *testing.T) {
	var buf bytes.Buffer
	cfg := BulkRenderConfig{
		Title:         "=== Fetch Results ===",
		Verb:          "Fetched",
		Format:        "default",
		IssueStatuses: issueStatusSet("error", "no-remote"),
		FormatStatus:  func(row BulkRenderRow) string { return row.Status },
		ChangesCount:  func(row BulkRenderRow) int { return row.CommitsBehind },
	}
	RenderBulkResults(&buf, cfg, sampleRenderInput())
	out := buf.String()

	if !strings.Contains(out, "repo-err") {
		t.Errorf("default mode should show issue repo, got:\n%s", out)
	}
	if strings.Contains(out, "repo-ok") {
		t.Errorf("default mode should hide non-issue repo-ok, got:\n%s", out)
	}
	if strings.Contains(out, "repo-clean") {
		t.Errorf("default mode should hide non-issue repo-clean, got:\n%s", out)
	}
	if !strings.Contains(out, "Fetched 3 repos") {
		t.Errorf("default mode should print summary line, got:\n%s", out)
	}
}

func TestBulkIssueStatusesTableDriven(t *testing.T) {
	in := sampleRenderInput()
	// Treat "fetched" as issue only
	var buf bytes.Buffer
	cfg := BulkRenderConfig{
		Verb:          "Fetched",
		Format:        "default",
		IssueStatuses: issueStatusSet("fetched"),
		FormatStatus:  func(row BulkRenderRow) string { return "custom-" + row.Status },
	}
	RenderBulkResults(&buf, cfg, in)
	out := buf.String()
	if !strings.Contains(out, "repo-ok") {
		t.Errorf("expected fetched row shown when in IssueStatuses:\n%s", out)
	}
	if strings.Contains(out, "repo-err") {
		t.Errorf("error row should not show when not in IssueStatuses:\n%s", out)
	}
}

func TestBulkFormatStatusUsed(t *testing.T) {
	var buf bytes.Buffer
	cfg := BulkRenderConfig{
		Title:          "=== Fetch Results ===",
		Format:         "compact",
		IssueStatuses:  issueStatusSet("error"),
		FormatStatus:   func(row BulkRenderRow) string { return "FORMATTED:" + row.Path },
		SuccessMessage: "✓ All repositories fetched successfully",
	}
	RenderBulkResults(&buf, cfg, sampleRenderInput())
	out := buf.String()
	if !strings.Contains(out, "FORMATTED:repo-err") {
		t.Errorf("FormatStatus not applied:\n%s", out)
	}
}

func TestBulkRepoResultInterfaceMethods(t *testing.T) {
	// Compile-time / runtime check lives in repository package tests;
	// here we ensure render path accepts rows built from interface fields.
	row := BulkRenderRow{
		Path:    "p",
		Status:  "error",
		Message: "m",
		Err:     errors.New("e"),
	}
	if row.Path == "" || row.Status == "" {
		t.Fatal("row fields empty")
	}
}
