// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/integrate"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

var auditNow = time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

func auditScan(repos ...repository.RepositoryStatusResult) *repository.BulkStatusResult {
	return &repository.BulkStatusResult{Repositories: repos}
}

// decodeAudit runs the audit and parses stdout, which must always be valid JSON
// regardless of the exit code — findings are output, not a broken run.
func decodeAudit(t *testing.T, out *bytes.Buffer) repository.AuditResult {
	t.Helper()
	var got repository.AuditResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, out.String())
	}
	return got
}

func TestRunInfoAudit_CleanWorkspaceExitsZero(t *testing.T) {
	var out bytes.Buffer
	scan := auditScan(
		repository.RepositoryStatusResult{
			Path: "/ws/a", RelativePath: "a", Branch: "master", Upstream: "origin/master",
		},
	)
	enr := map[string]infoEnrichment{
		"/ws/a": {Base: repository.BaseBranchInfo{Name: "master", Source: "heuristic"}},
	}

	err := runInfoAudit(&out, scan, enr, "/ws", nil, auditNow)
	if err != nil {
		t.Fatalf("clean workspace returned error: %v", err)
	}

	got := decodeAudit(t, &out)
	if got.Schema != repository.AuditSchema {
		t.Errorf("schema = %q, want %q", got.Schema, repository.AuditSchema)
	}
	if got.Directory != "/ws" {
		t.Errorf("directory = %q, want /ws", got.Directory)
	}
	if got.Summary.WithFindings != 0 {
		t.Errorf("with_findings = %d, want 0", got.Summary.WithFindings)
	}
}

func TestRunInfoAudit_FindingsExitOne(t *testing.T) {
	var out bytes.Buffer
	scan := auditScan(
		repository.RepositoryStatusResult{
			Path: "/ws/a", RelativePath: "a", Branch: "feat/x", Upstream: "origin/feat/x",
			CommitsBehind: 4,
		},
	)
	enr := map[string]infoEnrichment{
		"/ws/a": {Base: repository.BaseBranchInfo{Name: "master", Source: "heuristic", Behind: 9}},
	}

	err := runInfoAudit(&out, scan, enr, "/ws", nil, auditNow)
	if code := cliutil.ExitCodeForError(err); code != cliutil.ExitToolError {
		t.Errorf("exit code = %d, want %d (grep convention: findings present)", code, cliutil.ExitToolError)
	}

	// Even when the command "fails", stdout must still parse: the exit code is
	// how a shell learns there were findings, not a signal that output is junk.
	got := decodeAudit(t, &out)
	if got.Summary.WithFindings != 1 {
		t.Errorf("with_findings = %d, want 1", got.Summary.WithFindings)
	}
	if got.Summary.FindingsByCode[repository.CodeBranchBehindBase] != 1 {
		t.Errorf("missing %s: %v", repository.CodeBranchBehindBase, got.Summary.FindingsByCode)
	}
}

func TestRunInfoAudit_IntegrationUpstreamJSONContract(t *testing.T) {
	var out bytes.Buffer
	scan := auditScan(repository.RepositoryStatusResult{
		Path: "/ws/a", RelativePath: "a", Branch: "dev/a/b/c",
		Upstream: "origin/develop", CommitsAhead: 4,
	})
	enr := map[string]infoEnrichment{
		"/ws/a": {
			Base:                       repository.BaseBranchInfo{Name: "main", Source: "config.defaultBranch[0]"},
			Integration:                integrate.Resolution{Participates: true, Name: "develop", Source: "config[0]"},
			UpstreamTargetsIntegration: true,
			UpstreamRemote:             "origin",
			TaskRemoteExists:           true,
		},
	}

	err := runInfoAudit(&out, scan, enr, "/ws", nil, auditNow)
	if code := cliutil.ExitCodeForError(err); code != cliutil.ExitToolError {
		t.Fatalf("exit code = %d, want findings exit %d", code, cliutil.ExitToolError)
	}
	got := decodeAudit(t, &out)
	if !got.Repositories[0].Complete {
		t.Error("audit_complete = false for a non-blocking upstream finding")
	}
	if got.Summary.FindingsByCode[repository.CodeUpstreamTargetsIntegration] != 1 {
		t.Fatalf("summary = %+v", got.Summary.FindingsByCode)
	}
	for _, finding := range got.Repositories[0].Findings {
		switch finding.Code {
		case repository.CodeUpstreamTargetsIntegration:
			if finding.Evidence["integration"] != "develop" || finding.Fix == nil || finding.Fix.Autofix {
				t.Fatalf("integration finding contract = %+v", finding)
			}
		case repository.CodeUnpushedCommits, repository.CodeUpstreamDiverged, repository.CodeUpstreamBehind:
			t.Fatalf("integration upstream also emitted %s", finding.Code)
		}
	}
}

func TestRunInfoAudit_BlockerIsNotAnExecutionError(t *testing.T) {
	var out bytes.Buffer
	scan := auditScan(
		repository.RepositoryStatusResult{
			Path: "/ws/a", RelativePath: "a", Branch: "feat/x",
			RebaseInProgress: true,
		},
	)

	err := runInfoAudit(&out, scan, map[string]infoEnrichment{}, "/ws", nil, auditNow)
	// A repository mid-rebase is a finding about the repository, not a failure
	// of the tool; exit 2 is reserved for the latter.
	if code := cliutil.ExitCodeForError(err); code != cliutil.ExitToolError {
		t.Errorf("exit code = %d, want %d", code, cliutil.ExitToolError)
	}

	got := decodeAudit(t, &out)
	if got.Summary.Blockers != 1 || got.Summary.Incomplete != 1 {
		t.Errorf("blockers/incomplete = %d/%d, want 1/1", got.Summary.Blockers, got.Summary.Incomplete)
	}
	if got.Repositories[0].Complete {
		t.Error("audit_complete = true for a repository mid-rebase")
	}
	if got.Repositories[0].IncompleteReason != repository.CodeRebaseInProgress {
		t.Errorf("incomplete_reason = %q, want %q",
			got.Repositories[0].IncompleteReason, repository.CodeRebaseInProgress)
	}
}

func TestBuildAudit_SortedByName(t *testing.T) {
	scan := auditScan(
		repository.RepositoryStatusResult{Path: "/ws/z", RelativePath: "zeta", Branch: "master"},
		repository.RepositoryStatusResult{Path: "/ws/a", RelativePath: "alpha", Branch: "master"},
		repository.RepositoryStatusResult{Path: "/ws/m", RelativePath: "mid", Branch: "master"},
	)

	got := buildAudit(scan, map[string]infoEnrichment{}, "/ws", nil, auditNow)

	want := []string{"alpha", "mid", "zeta"}
	for i, name := range want {
		if got.Repositories[i].Name != name {
			t.Fatalf("repositories = %v, want sorted %v",
				[]string{got.Repositories[0].Name, got.Repositories[1].Name, got.Repositories[2].Name}, want)
		}
	}
}

func TestBuildAudit_EnrichErrorBecomesBlocker(t *testing.T) {
	scan := auditScan(
		repository.RepositoryStatusResult{Path: "/ws/a", RelativePath: "a", Branch: "feat/x", Upstream: "origin/feat/x"},
	)
	enr := map[string]infoEnrichment{
		"/ws/a": {Err: errors.New("worktree list failed")},
	}

	got := buildAudit(scan, enr, "/ws", nil, auditNow)

	if got.Repositories[0].Complete {
		t.Error("audit_complete = true despite enrichment failing; the base facts are missing")
	}
	if got.Repositories[0].IncompleteReason != repository.CodeScanError {
		t.Errorf("incomplete_reason = %q, want %q",
			got.Repositories[0].IncompleteReason, repository.CodeScanError)
	}
}

func TestBuildAudit_ConfigOverridesAutofixPerCode(t *testing.T) {
	scan := auditScan(
		repository.RepositoryStatusResult{
			Path: "/ws/a", RelativePath: "a", Branch: "feat/x", Upstream: "origin/feat/x",
		},
	)
	enr := map[string]infoEnrichment{
		"/ws/a": {Base: repository.BaseBranchInfo{Name: "master", Source: "heuristic", Behind: 3}},
	}

	// Default: BRANCH_BEHIND_BASE is auto-fixable.
	base := buildAudit(scan, enr, "/ws", nil, auditNow)
	fix := auditFinding(t, base, repository.CodeBranchBehindBase)
	if !fix.Autofix {
		t.Error("BRANCH_BEHIND_BASE not auto-fixable by default")
	}

	// A project that rebases by hand turns off that one code and nothing else.
	off := buildAudit(scan, enr, "/ws",
		map[string]bool{repository.CodeBranchBehindBase: false}, auditNow)
	if auditFinding(t, off, repository.CodeBranchBehindBase).Autofix {
		t.Error("config override did not disable BRANCH_BEHIND_BASE autofix")
	}
}

// auditFinding returns the remediation for a code in the first repository.
func auditFinding(t *testing.T, res repository.AuditResult, code string) *repository.Remediation {
	t.Helper()
	for _, f := range res.Repositories[0].Findings {
		if f.Code == code {
			if f.Fix == nil {
				t.Fatalf("finding %s carries no remediation", code)
			}
			return f.Fix
		}
	}
	t.Fatalf("no %s finding in %+v", code, res.Repositories[0].Findings)
	return nil
}
