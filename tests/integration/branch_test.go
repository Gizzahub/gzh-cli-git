package integration

import (
	"strings"
	"testing"
)

func TestBranchListCommand(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("list current branch", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("branch", "list")

		// Should show at least the current branch
		AssertContains(t, output, "master")
	})

	t.Run("list all branches", func(t *testing.T) {
		// Create additional branches
		repo.GitBranch("feature-1")
		repo.GitBranch("feature-2")

		output := repo.RunGzhGitSuccess("branch", "list", "--all")

		AssertContains(t, output, "master")
		AssertContains(t, output, "feature-1")
		AssertContains(t, output, "feature-2")
	})

	t.Run("with verbose flag", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("branch", "list", "--verbose")

		// Verbose mode should show more details
		AssertContains(t, output, "master")
	})
}

func TestBranchCreateCommand(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("create new branch", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("branch", "create", "feature/new-feature", "--base", "master")

		AssertContains(t, output, "Created")
		AssertContains(t, output, "feature/new-feature")

		// Verify branch exists
		listOutput := repo.RunGzhGitSuccess("branch", "list", "--all")
		AssertContains(t, listOutput, "feature/new-feature")
	})

	t.Run("create branch from specific ref", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("branch", "create", "feature/from-master", "--base", "master")

		AssertContains(t, output, "Created")
		AssertContains(t, output, "feature/from-master")
	})

	t.Run("create already existing branch", func(t *testing.T) {
		repo.GitBranch("existing-branch")

		output := repo.RunGzhGitExpectError("branch", "create", "existing-branch")

		AssertContains(t, output, "already exists")
	})

	t.Run("force create existing branch", func(t *testing.T) {
		repo.GitBranch("force-test")

		output := repo.RunGzhGitSuccess("branch", "create", "force-test", "--force")

		AssertContains(t, output, "Created")
	})

	t.Run("create with tracking", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("branch", "create", "feature/track-test", "--track")

		AssertContains(t, output, "Created")
		AssertContains(t, output, "feature/track-test")
	})
}

func TestBranchDeleteCommand(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("delete merged branch", func(t *testing.T) {
		// Create and merge a branch
		repo.GitBranch("to-delete")

		output := repo.RunGzhGitSuccess("branch", "delete", "to-delete")

		AssertContains(t, output, "Deleted")
		AssertContains(t, output, "to-delete")

		// Verify branch is gone
		listOutput := repo.RunGzhGitSuccess("branch", "list", "--all")
		AssertNotContains(t, listOutput, "to-delete")
	})

	t.Run("delete non-existent branch", func(t *testing.T) {
		output := repo.RunGzhGitExpectError("branch", "delete", "non-existent")

		AssertContains(t, output, "not found")
	})

	t.Run("delete current branch", func(t *testing.T) {
		output := repo.RunGzhGitExpectError("branch", "delete", "master")

		// Should fail - can't delete current branch
		if !strings.Contains(output, "current") && !strings.Contains(output, "checked out") {
			t.Logf("Expected error about current branch, got: %s", output)
		}
	})

	t.Run("force delete unmerged branch", func(t *testing.T) {
		// Create branch with unique commit
		repo.GitBranch("unmerged")
		repo.GitCheckout("unmerged")
		repo.WriteFile("unique.txt", "unique content")
		repo.GitAdd("unique.txt")
		repo.GitCommit("Add unique file")
		repo.GitCheckout("master")

		output := repo.RunGzhGitSuccess("branch", "delete", "unmerged", "--force")

		AssertContains(t, output, "Deleted")
	})
}

func TestBranchWorkflow(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("complete feature branch workflow", func(t *testing.T) {
		// 1. Create feature branch
		repo.RunGzhGitSuccess("branch", "create", "feature/workflow-test", "--base", "master")

		// 2. Checkout the branch
		repo.GitCheckout("feature/workflow-test")

		// 3. Make changes
		repo.WriteFile("feature.go", "package main\n")
		repo.GitAdd("feature.go")
		repo.GitCommit("Add feature")

		// 4. List branches
		listOutput := repo.RunGzhGitSuccess("branch", "list", "--all")
		AssertContains(t, listOutput, "feature/workflow-test")

		// 5. Switch back to master
		repo.GitCheckout("master")

		// 6. Delete feature branch
		repo.RunGzhGitSuccess("branch", "delete", "feature/workflow-test", "--force")

		// 7. Verify deletion
		finalList := repo.RunGzhGitSuccess("branch", "list", "--all")
		AssertNotContains(t, finalList, "feature/workflow-test")
	})
}
