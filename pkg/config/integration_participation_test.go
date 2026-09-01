// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPlanIntegrationParticipation(t *testing.T) {
	tests := []struct {
		name  string
		state IntegrationParticipationState
		want  IntegrationParticipationAction
	}{
		{"fresh declaration installs", IntegrationParticipationState{Desired: "master"}, IntegrationParticipationInstall},
		{"unmanaged matching is not adopted", IntegrationParticipationState{Current: "master", Desired: "master"}, IntegrationParticipationNoop},
		{"unmanaged different conflicts", IntegrationParticipationState{Current: "develop", Desired: "master"}, IntegrationParticipationConflict},
		{"managed declaration updates", IntegrationParticipationState{Current: "develop", Marker: "develop", Desired: "master"}, IntegrationParticipationUpdate},
		{"managed matching noops", IntegrationParticipationState{Current: "master", Marker: "master", Desired: "master"}, IntegrationParticipationNoop},
		{"marker mismatch conflicts", IntegrationParticipationState{Current: "master", Marker: "develop", Desired: "master"}, IntegrationParticipationConflict},
		{"managed declaration removal", IntegrationParticipationState{Current: "master", Marker: "master"}, IntegrationParticipationRemove},
		{"orphan marker removal", IntegrationParticipationState{Marker: "master"}, IntegrationParticipationRemove},
		{"manual setting survives absent declaration", IntegrationParticipationState{Current: "master"}, IntegrationParticipationNoop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PlanIntegrationParticipation(tt.state); got != tt.want {
				t.Fatalf("PlanIntegrationParticipation(%+v) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestReconcileIntegrationParticipationInstallsAndUpdatesManagedState(t *testing.T) {
	repo := initParticipationRepo(t)
	runParticipationGit(t, repo, "remote", "add", "team/upstream", repo)
	writeRepoConfig(t, repo, "branch:\n  integrationBranch: [missing, team/upstream/remote-only]\n")
	runParticipationGit(t, repo, "update-ref", "refs/remotes/team/upstream/remote-only", "HEAD")

	action, err := ReconcileIntegrationParticipation(t.Context(), repo)
	if err != nil || action != IntegrationParticipationInstall {
		t.Fatalf("initial reconcile = %q, %v", action, err)
	}
	assertTestGitConfig(t, repo, workflowIntegrationBranchKey, "remote-only")
	assertTestGitConfig(t, repo, managedIntegrationBranchKey, "remote-only")

	writeRepoConfig(t, repo, "branch:\n  integrationBranch: [develop]\n")
	runParticipationGit(t, repo, "branch", "develop")
	action, err = ReconcileIntegrationParticipation(t.Context(), repo)
	if err != nil || action != IntegrationParticipationUpdate {
		t.Fatalf("update reconcile = %q, %v", action, err)
	}
	assertTestGitConfig(t, repo, workflowIntegrationBranchKey, "develop")
	assertTestGitConfig(t, repo, managedIntegrationBranchKey, "develop")
}

func TestReconcileIntegrationParticipationResumesInterruptedTransition(t *testing.T) {
	repo := initParticipationRepo(t)
	writeRepoConfig(t, repo, "branch:\n  integrationBranch: [master]\n")
	setTestGitConfig(t, repo, managedIntegrationBranchKey, `{"from":"","to":"master"}`)

	action, err := ReconcileIntegrationParticipation(t.Context(), repo)
	if err != nil || action != IntegrationParticipationInstall {
		t.Fatalf("resume before current write = %q, %v", action, err)
	}
	assertTestGitConfig(t, repo, workflowIntegrationBranchKey, "master")
	assertTestGitConfig(t, repo, managedIntegrationBranchKey, "master")

	runParticipationGit(t, repo, "branch", "develop")
	writeRepoConfig(t, repo, "branch:\n  integrationBranch: [develop]\n")
	setTestGitConfig(t, repo, managedIntegrationBranchKey, `{"from":"master","to":"develop"}`)
	setTestGitConfig(t, repo, workflowIntegrationBranchKey, "develop")
	action, err = ReconcileIntegrationParticipation(t.Context(), repo)
	if err != nil || action != IntegrationParticipationUpdate {
		t.Fatalf("resume before final marker write = %q, %v", action, err)
	}
	assertTestGitConfig(t, repo, managedIntegrationBranchKey, "develop")
}

func TestReconcileIntegrationParticipationUsesRootDeclarationOnly(t *testing.T) {
	repo := initParticipationRepo(t)
	writeRepoConfig(t, repo, "branch:\n  integrationBranch: [master]\n")
	if err := os.MkdirAll(filepath.Join(repo, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "nested", ".gz-git.yaml"), []byte("branch:\n  integrationBranch: [develop]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runParticipationGit(t, repo, "branch", "develop")

	action, err := ReconcileIntegrationParticipation(t.Context(), repo)
	if err != nil || action != IntegrationParticipationInstall {
		t.Fatalf("reconcile = %q, %v", action, err)
	}
	assertTestGitConfig(t, repo, workflowIntegrationBranchKey, "master")
}

func TestReconcileIntegrationParticipationRejectsUnresolvedOrManualConflictWithoutWrites(t *testing.T) {
	repo := initParticipationRepo(t)
	writeRepoConfig(t, repo, "branch:\n  integrationBranch: [missing]\n")
	if _, err := ReconcileIntegrationParticipation(t.Context(), repo); err == nil {
		t.Fatal("unresolved declaration unexpectedly succeeded")
	}
	assertTestGitConfig(t, repo, workflowIntegrationBranchKey, "")

	writeRepoConfig(t, repo, "branch:\n  integrationBranch: [master]\n")
	setTestGitConfig(t, repo, workflowIntegrationBranchKey, "develop")
	if action, err := ReconcileIntegrationParticipation(t.Context(), repo); err == nil || action != IntegrationParticipationConflict {
		t.Fatalf("manual conflict = %q, %v", action, err)
	}
	assertTestGitConfig(t, repo, workflowIntegrationBranchKey, "develop")
	assertTestGitConfig(t, repo, managedIntegrationBranchKey, "")
}

func TestReconcileIntegrationParticipationRemovesOnlyManagedState(t *testing.T) {
	repo := initParticipationRepo(t)
	setTestGitConfig(t, repo, workflowIntegrationBranchKey, "master")
	setTestGitConfig(t, repo, managedIntegrationBranchKey, "master")

	action, err := ReconcileIntegrationParticipation(t.Context(), repo)
	if err != nil || action != IntegrationParticipationRemove {
		t.Fatalf("remove reconcile = %q, %v", action, err)
	}
	assertTestGitConfig(t, repo, workflowIntegrationBranchKey, "")
	assertTestGitConfig(t, repo, managedIntegrationBranchKey, "")
}

func initParticipationRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runParticipationGit(t, repo, "init", "--initial-branch=master")
	runParticipationGit(t, repo, "config", "user.email", "test@example.com")
	runParticipationGit(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runParticipationGit(t, repo, "add", "README")
	runParticipationGit(t, repo, "commit", "-m", "initial")
	return repo
}

func runParticipationGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...) //nolint:gosec // tests control every argument.
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s: %v", args, output, err)
	}
}

func setTestGitConfig(t *testing.T, repo, key, value string) {
	t.Helper()
	runParticipationGit(t, repo, "config", "--local", key, value)
}

func assertTestGitConfig(t *testing.T, repo, key, want string) {
	t.Helper()
	output, err := exec.CommandContext(t.Context(), "git", "-C", repo, "config", "--local", "--get", key).Output() //nolint:gosec // fixed test Git command.
	if want == "" {
		if err == nil {
			t.Fatalf("git config %s = %q, want absent", key, output)
		}
		return
	}
	if err != nil {
		t.Fatalf("git config %s: %v", key, err)
	}
	if got := string(output); got != want+"\n" {
		t.Fatalf("git config %s = %q, want %q", key, got, want+"\n")
	}
}
