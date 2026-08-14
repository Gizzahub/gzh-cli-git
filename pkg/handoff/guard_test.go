// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package handoff

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)

// writeFile creates a file under root, making parent directories as needed.
func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()

	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

// findingFor returns the finding for rel, or a zero value if the guard passed
// the file.
func findingFor(findings []Finding, rel string) Finding {
	for _, f := range findings {
		if f.File == rel {
			return f
		}
	}
	return Finding{}
}

func TestClassifyName(t *testing.T) {
	tests := []struct {
		path  string
		flags bool
	}{
		{".env", true},
		{"config/.env.production", true},
		{".env.example", false},
		{".env.Sample", false},
		{"deploy/id_rsa", true},
		{"certs/server.pem", true},
		{"keys/app.key", true},
		{"gcp-service-account-prod.json", true},
		{".netrc", true},
		{"main.go", false},
		{"README.md", false},
		{"environment.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := classifyName(tt.path) != ""
			if got != tt.flags {
				t.Errorf("classifyName(%q) flagged = %v, want %v", tt.path, got, tt.flags)
			}
		})
	}
}

func TestClassifyArtifact(t *testing.T) {
	tests := []struct {
		path  string
		flags bool
	}{
		{"node_modules/react/index.js", true},
		{"web/dist/bundle.js", true},
		{"target/debug/app", true},
		{"src/__pycache__/mod.cpython-312.pyc", true},
		{"logs/server.log", true},
		{".DS_Store", true},
		{"pkg/handoff/guard.go", false},
		{"docs/build-guide.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := classifyArtifact(tt.path) != ""
			if got != tt.flags {
				t.Errorf("classifyArtifact(%q) flagged = %v, want %v", tt.path, got, tt.flags)
			}
		})
	}
}

func TestInspectFilesDetectsSecretContent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "deploy/terraform.tfvars", "region = \"us-east-1\"\naccess_key = \"AKIAIOSFODNN7EXAMPLE\"\n")
	writeFile(t, root, "internal/app.go", "package internal\n\nconst region = \"us-east-1\"\n")

	findings := inspectFiles(root, []pendingFile{
		{path: "deploy/terraform.tfvars", untracked: true},
		{path: "internal/app.go"},
	})

	got := findingFor(findings, "deploy/terraform.tfvars")
	if got.Kind != FindingSecret {
		t.Fatalf("tfvars finding = %+v, want a secret finding", got)
	}
	if !strings.Contains(got.Detail, "AWS") {
		t.Errorf("detail = %q, want it to name the AWS key", got.Detail)
	}
	if findingFor(findings, "internal/app.go").Kind != "" {
		t.Error("ordinary source was flagged")
	}
}

func TestInspectFilesSkipsBinaryContent(t *testing.T) {
	root := t.TempDir()
	// The same byte sequence a text scan would flag, inside a binary file.
	writeFile(t, root, "assets/icon.bin", "\x00\x01AKIAIOSFODNN7EXAMPLE\x00")

	findings := inspectFiles(root, []pendingFile{{path: "assets/icon.bin", untracked: true}})

	if len(findings) != 0 {
		t.Errorf("findings = %+v, want none — binary files are not content-scanned", findings)
	}
}

func TestInspectFilesFlagsLargeFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "data/dump.sql", strings.Repeat("x", largeFileThreshold+1))

	findings := inspectFiles(root, []pendingFile{{path: "data/dump.sql", untracked: true}})

	if got := findingFor(findings, "data/dump.sql"); got.Kind != FindingLargeFile {
		t.Errorf("finding = %+v, want a large-file finding", got)
	}
}

func TestInspectFilesOnlyFlagsArtifactsWhenUntracked(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "dist/app.js", "console.log(1)\n")

	tracked := inspectFiles(root, []pendingFile{{path: "dist/app.js"}})
	if len(tracked) != 0 {
		t.Errorf("findings = %+v, want none — a tracked file under dist/ was committed on purpose", tracked)
	}

	untracked := inspectFiles(root, []pendingFile{{path: "dist/app.js", untracked: true}})
	if got := findingFor(untracked, "dist/app.js"); got.Kind != FindingArtifact {
		t.Errorf("finding = %+v, want an artifact finding", got)
	}
}

func TestInspectFilesIgnoresMissingPaths(t *testing.T) {
	findings := inspectFiles(t.TempDir(), []pendingFile{{path: "was/renamed/away.go"}})

	if len(findings) != 0 {
		t.Errorf("findings = %+v, want none for a path that no longer exists", findings)
	}
}

func TestInspectFilesDoesNotFollowOutsideSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.env")
	if err := os.WriteFile(outside, []byte("AWS_SECRET_ACCESS_KEY=must-not-be-read\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	findings := inspectFiles(root, []pendingFile{{path: "linked.txt", untracked: true}})
	if len(findings) != 0 {
		t.Fatalf("findings = %+v, want no finding for outside symlink", findings)
	}
}

func TestParsePorcelainZ(t *testing.T) {
	// Modified tracked file, untracked file, staged deletion.
	out := " M pkg/app.go\x00?? .env\x00D  old.go\x00"

	got := parsePorcelainZ(out)

	if len(got) != 2 {
		t.Fatalf("parsed %d files (%+v), want 2 — the deletion has nothing to inspect", len(got), got)
	}
	if got[0].path != "pkg/app.go" || got[0].untracked {
		t.Errorf("got[0] = %+v, want a tracked pkg/app.go", got[0])
	}
	if got[1].path != ".env" || !got[1].untracked {
		t.Errorf("got[1] = %+v, want an untracked .env", got[1])
	}
}

func TestGuardAgainstRealRepository(t *testing.T) {
	repo := testutil.TempGitRepoWithCommit(t)
	writeFile(t, repo, ".env", "DATABASE_PASSWORD=hunter2\n")
	writeFile(t, repo, "node_modules/left-pad/index.js", "module.exports = 1\n")
	writeFile(t, repo, "src/main.go", "package main\n\nfunc main() {}\n")

	findings, err := Guard(context.Background(), gitcmd.NewExecutor(), repo)
	if err != nil {
		t.Fatalf("Guard() error: %v", err)
	}

	if got := findingFor(findings, ".env"); got.Kind != FindingSecret {
		t.Errorf(".env finding = %+v, want a secret finding", got)
	}
	if got := findingFor(findings, "node_modules/left-pad/index.js"); got.Kind != FindingArtifact {
		t.Errorf("node_modules finding = %+v, want an artifact finding — -uall must list files inside untracked directories", got)
	}
	if findingFor(findings, "src/main.go").Kind != "" {
		t.Error("ordinary source was flagged")
	}
}

func TestGuardCleanRepositoryHasNoFindings(t *testing.T) {
	repo := testutil.TempGitRepoWithCommit(t)

	findings, err := Guard(context.Background(), gitcmd.NewExecutor(), repo)
	if err != nil {
		t.Fatalf("Guard() error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %+v, want none", findings)
	}
}
