// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package workspacecli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncCommandFreshCloneInstallsIntegrationParticipation(t *testing.T) {
	workspace, configPath, clonePath := syncParticipationCommandFixture(t)
	t.Chdir(workspace)

	var output bytes.Buffer
	cmd := CommandFactory{}.newSyncCmd()
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"-c", configPath, "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fresh workspace sync: %v\n%s", err, output.String())
	}

	assertSyncGitConfig(t, clonePath, "workflow.integrationBranch", "master")
	assertSyncGitConfig(t, clonePath, "gz-git.managedWorkflowIntegrationBranch", "master")
}

func TestSyncCommandMachineOutputPrecedesParticipationConflict(t *testing.T) {
	for _, format := range []string{"json", "llm"} {
		t.Run(format, func(t *testing.T) {
			workspace, configPath, clonePath := syncParticipationCommandFixture(t)
			t.Chdir(workspace)
			runSyncParticipationCommand(t, configPath, "json")
			runSyncParticipationGit(t, clonePath, "config", "--local", "workflow.integrationBranch", "develop")

			var output bytes.Buffer
			var errorOutput bytes.Buffer
			cmd := CommandFactory{}.newSyncCmd()
			cmd.SilenceUsage = true
			cmd.SetOut(&output)
			cmd.SetErr(&errorOutput)
			cmd.SetArgs([]string{"-c", configPath, "--format", format})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "workspace integration participation reconcile failed") {
				t.Fatalf("conflict error = %v, stdout=%q stderr=%q", err, output.String(), errorOutput.String())
			}

			if format == "json" {
				var parsed SyncResultJSON
				if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &parsed); err != nil {
					t.Fatalf("structured JSON missing before conflict: %v\n%s", err, output.String())
				}
				if parsed.Total != 1 || parsed.Succeeded != 1 {
					t.Fatalf("JSON result = total %d succeeded %d, want 1/1", parsed.Total, parsed.Succeeded)
				}
			} else {
				llm := strings.ToLower(output.String())
				if !strings.Contains(llm, "total") || !strings.Contains(llm, "succeeded") || !strings.Contains(llm, "fresh") {
					t.Fatalf("LLM result missing before conflict: %q", output.String())
				}
			}
		})
	}
}

func syncParticipationCommandFixture(t *testing.T) (workspace, configPath, clonePath string) {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	origin := filepath.Join(root, "origin.git")
	workspace = filepath.Join(root, "workspace")
	clonePath = filepath.Join(workspace, "fresh")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	runSyncParticipationGit(t, source, "init", "--initial-branch=master")
	runSyncParticipationGit(t, source, "config", "user.email", "test@example.com")
	runSyncParticipationGit(t, source, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(source, "README"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".gz-git.yaml"), []byte("branch:\n  integrationBranch: [master]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runSyncParticipationGit(t, source, "add", "README", ".gz-git.yaml")
	runSyncParticipationGit(t, source, "commit", "-m", "seed integration declaration")
	runSyncParticipationGit(t, root, "clone", "--bare", source, origin)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath = filepath.Join(workspace, "workspace.yaml")
	config := fmt.Sprintf("version: 1\nkind: repositories\nstrategy: reset\nrepositories:\n  - name: fresh\n    url: %q\n    path: fresh\n", origin)
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	return workspace, configPath, clonePath
}

func runSyncParticipationCommand(t *testing.T, configPath, format string) {
	t.Helper()
	var output bytes.Buffer
	cmd := CommandFactory{}.newSyncCmd()
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"-c", configPath, "--format", format})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("workspace sync: %v\n%s", err, output.String())
	}
}

func runSyncParticipationGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %s: %v", strings.Join(args, " "), output, err)
	}
}
