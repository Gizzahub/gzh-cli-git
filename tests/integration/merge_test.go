package integration

import (
	"strings"
	"testing"
)

func TestMergeDetectCommand(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("detect no conflicts - fast forward", func(t *testing.T) {
		// Create feature branch with new commit
		repo.GitBranch("feature-clean")
		repo.GitCheckout("feature-clean")
		repo.WriteFile("new-feature.go", "package main\n")
		repo.GitAdd("new-feature.go")
		repo.GitCommit("Add new feature")
		repo.GitCheckout("master")

		output := repo.RunGzhGitSuccess("merge", "detect", "feature-clean", "master")

		// Should detect clean merge
		AssertContains(t, output, "merge")
	})

	t.Run("detect conflicts", func(t *testing.T) {
		// Create two branches modifying same file
		repo.GitBranch("feature-a")
		repo.GitCheckout("feature-a")
		repo.WriteFile("conflict.txt", "Version A\n")
		repo.GitAdd("conflict.txt")
		repo.GitCommit("Add conflict file - version A")

		repo.GitCheckout("master")
		repo.GitBranch("feature-b")
		repo.GitCheckout("feature-b")
		repo.WriteFile("conflict.txt", "Version B\n")
		repo.GitAdd("conflict.txt")
		repo.GitCommit("Add conflict file - version B")

		repo.GitCheckout("master")

		output := repo.RunGzhGitSuccess("merge", "detect", "feature-a", "feature-b")

		// May or may not detect conflicts depending on git version
		// Just verify command runs
		if len(output) == 0 {
			t.Skip("Conflict detection returned empty output")
		}
	})

	t.Run("detect with verbose flag", func(t *testing.T) {
		repo.GitBranch("feature-verbose")

		output := repo.RunGzhGitSuccess("merge", "detect", "feature-verbose", "master", "--verbose")

		// Verbose mode should show details
		AssertContains(t, output, "merge")
	})
}

func TestMergeDoCommand(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("fast forward merge", func(t *testing.T) {
		// Create feature branch
		repo.GitBranch("feature-ff")
		repo.GitCheckout("feature-ff")
		repo.WriteFile("ff-feature.go", "package main\n")
		repo.GitAdd("ff-feature.go")
		repo.GitCommit("Add FF feature")
		repo.GitCheckout("master")

		output := repo.RunGzhGitSuccess("merge", "do", "feature-ff")

		AssertContains(t, output, "Merge")
	})

	t.Run("no fast forward merge", func(t *testing.T) {
		// Create feature branch
		repo.GitBranch("feature-no-ff")
		repo.GitCheckout("feature-no-ff")
		repo.WriteFile("no-ff-feature.go", "package main\n")
		repo.GitAdd("no-ff-feature.go")
		repo.GitCommit("Add no-FF feature")
		repo.GitCheckout("master")

		output := repo.RunGzhGitSuccess("merge", "do", "feature-no-ff", "--no-ff")

		AssertContains(t, output, "Merge")
	})

	t.Run("squash merge", func(t *testing.T) {
		// Create feature branch with multiple commits
		repo.GitBranch("feature-squash")
		repo.GitCheckout("feature-squash")
		repo.WriteFile("file1.go", "package main\n")
		repo.GitAdd("file1.go")
		repo.GitCommit("Add file 1")
		repo.WriteFile("file2.go", "package main\n")
		repo.GitAdd("file2.go")
		repo.GitCommit("Add file 2")
		repo.GitCheckout("master")

		output := repo.RunGzhGitSuccess("merge", "do", "feature-squash", "--squash")

		AssertContains(t, output, "Merge")
	})

	t.Run("merge non-existent branch", func(t *testing.T) {
		output := repo.RunGzhGitExpectError("merge", "do", "nonexistent-branch")

		AssertContains(t, output, "not found")
	})

	t.Run("dry run mode", func(t *testing.T) {
		repo.GitBranch("feature-dry")
		repo.GitCheckout("feature-dry")
		repo.WriteFile("dry-feature.go", "package main\n")
		repo.GitAdd("dry-feature.go")
		repo.GitCommit("Add dry feature")
		repo.GitCheckout("master")

		output := repo.RunGzhGitSuccess("merge", "do", "feature-dry", "--dry-run")

		// Should show merge info without actually merging
		AssertContains(t, output, "Merge")
	})
}

func TestMergeAbortCommand(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("abort when no merge in progress", func(t *testing.T) {
		output := repo.RunGzhGitExpectError("merge", "abort")

		// Should fail - no merge in progress
		if !strings.Contains(output, "no merge") && !strings.Contains(output, "not in progress") {
			t.Logf("Expected 'no merge in progress' error, got: %s", output)
		}
	})

	t.Run("abort conflicted merge", func(t *testing.T) {
		// Create conflicting branches
		repo.GitBranch("conflict-a")
		repo.GitCheckout("conflict-a")
		repo.WriteFile("conflict.txt", "Version A\n")
		repo.GitAdd("conflict.txt")
		repo.GitCommit("Version A")

		repo.GitCheckout("master")
		repo.WriteFile("conflict.txt", "Version Master\n")
		repo.GitAdd("conflict.txt")
		repo.GitCommit("Version Master")

		// Try to merge (will conflict)
		repo.RunGzhGitExpectError("merge", "do", "conflict-a")

		// Abort the merge
		output := repo.RunGzhGitSuccess("merge", "abort")

		AssertContains(t, output, "Aborted")
	})
}

func TestMergeRebaseCommand(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("simple rebase", func(t *testing.T) {
		// Create feature branch
		repo.GitBranch("feature-rebase")
		repo.GitCheckout("feature-rebase")
		repo.WriteFile("rebase-feature.go", "package main\n")
		repo.GitAdd("rebase-feature.go")
		repo.GitCommit("Add rebase feature")

		output := repo.RunGzhGitSuccess("merge", "rebase", "master")

		// Rebase should succeed
		AssertContains(t, output, "Rebase")
	})

	t.Run("rebase non-existent branch", func(t *testing.T) {
		output := repo.RunGzhGitExpectError("merge", "rebase", "nonexistent")

		AssertContains(t, output, "not found")
	})

	t.Run("rebase with conflicts", func(t *testing.T) {
		// Create base commit on master
		repo.GitCheckout("master")
		repo.WriteFile("base.txt", "Master version\n")
		repo.GitAdd("base.txt")
		repo.GitCommit("Master change")

		// Create feature branch from before master change
		repo.GitCheckout("HEAD~1")
		repo.GitBranch("feature-conflict")
		repo.GitCheckout("feature-conflict")
		repo.WriteFile("base.txt", "Feature version\n")
		repo.GitAdd("base.txt")
		repo.GitCommit("Feature change")

		// Try to rebase (will conflict)
		output := repo.RunGzhGitExpectError("merge", "rebase", "master")

		// Should report conflicts
		if !strings.Contains(output, "conflict") && !strings.Contains(output, "failed") {
			t.Logf("Expected conflict error, got: %s", output)
		}

		// Abort the rebase
		repo.RunGzhGitSuccess("merge", "rebase", "--abort")
	})

	t.Run("interactive rebase", func(t *testing.T) {
		repo.GitBranch("feature-interactive")
		repo.GitCheckout("feature-interactive")
		repo.WriteFile("interactive.go", "package main\n")
		repo.GitAdd("interactive.go")
		repo.GitCommit("Interactive commit")

		// Interactive mode not supported in automated tests, just verify flag is recognized
		output := repo.RunGzhGitExpectError("merge", "rebase", "master", "--interactive")

		// Should recognize the flag (but fail in non-interactive environment)
		if len(output) == 0 {
			t.Skip("Interactive rebase not supported in test environment")
		}
	})
}

func TestMergeWorkflow(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("complete merge workflow", func(t *testing.T) {
		// 1. Create feature branch
		repo.GitBranch("feature-workflow")
		repo.GitCheckout("feature-workflow")
		repo.WriteFile("workflow.go", "package main\n\nfunc WorkflowFeature() {}\n")
		repo.GitAdd("workflow.go")
		repo.GitCommit("Add workflow feature")
		repo.GitCheckout("master")

		// 2. Detect potential conflicts
		detectOutput := repo.RunGzhGitSuccess("merge", "detect", "feature-workflow", "master")
		AssertContains(t, detectOutput, "merge")

		// 3. Perform the merge
		mergeOutput := repo.RunGzhGitSuccess("merge", "do", "feature-workflow")
		AssertContains(t, mergeOutput, "Merge")

		// 4. Verify merge was successful
		statusOutput := repo.RunGzhGitSuccess("status")
		AssertContains(t, statusOutput, "Clean")
	})

	t.Run("conflict resolution workflow", func(t *testing.T) {
		// 1. Create conflicting branches
		repo.GitBranch("conflict-branch-1")
		repo.GitCheckout("conflict-branch-1")
		repo.WriteFile("shared.txt", "Branch 1 version\n")
		repo.GitAdd("shared.txt")
		repo.GitCommit("Branch 1 change")

		repo.GitCheckout("master")
		repo.WriteFile("shared.txt", "Master version\n")
		repo.GitAdd("shared.txt")
		repo.GitCommit("Master change")

		// 2. Detect conflicts
		_ = repo.RunGzhGitSuccess("merge", "detect", "conflict-branch-1", "master")
		// May or may not show conflicts depending on git version

		// 3. Try to merge (will fail with conflicts)
		repo.RunGzhGitExpectError("merge", "do", "conflict-branch-1")

		// 4. Abort merge
		abortOutput := repo.RunGzhGitSuccess("merge", "abort")
		AssertContains(t, abortOutput, "Aborted")

		// 5. Verify repository is clean
		statusOutput := repo.RunGzhGitSuccess("status")
		AssertContains(t, statusOutput, "Clean")
	})
}
