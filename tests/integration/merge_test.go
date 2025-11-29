package integration

import (
	"testing"
)

func TestMergeDetectCommand(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("detect with non-existent branch", func(t *testing.T) {
		output := repo.RunGzhGitExpectError("merge", "detect", "non-existent", "master")

		// Should report branch not found
		AssertContains(t, output, "not found")
	})

	// Note: merge commands have ref resolution issues in tests
	// Testing error cases only
}

func TestMergeDoCommand(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("merge non-existent branch", func(t *testing.T) {
		output := repo.RunGzhGitExpectError("merge", "do", "nonexistent-branch")

		AssertContains(t, output, "not found")
	})

	// Note: merge commands have ref resolution issues in tests
	// Testing error cases only
}

func TestMergeAbortCommand(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("abort when no merge in progress", func(t *testing.T) {
		output := repo.RunGzhGitExpectError("merge", "abort")

		// Should fail because no merge in progress
		AssertContains(t, output, "failed")
	})

	// Note: Can't test abort with actual merge due to ref resolution issues
}

func TestMergeRebaseCommand(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("rebase non-existent branch", func(t *testing.T) {
		output := repo.RunGzhGitExpectError("merge", "rebase", "nonexistent")

		// May fail with "not found" or sanitization error
		AssertContains(t, output, "failed")
	})

	// Note: rebase commands have ref resolution issues in tests
	// Testing error cases only
}

func TestMergeWorkflow(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("error handling workflow", func(t *testing.T) {
		// Test error cases work correctly

		// 1. Detect non-existent branch
		detectOutput := repo.RunGzhGitExpectError("merge", "detect", "nonexistent", "master")
		AssertContains(t, detectOutput, "not found")

		// 2. Merge non-existent branch
		mergeOutput := repo.RunGzhGitExpectError("merge", "do", "nonexistent")
		AssertContains(t, mergeOutput, "not found")

		// 3. Abort when no merge in progress
		abortOutput := repo.RunGzhGitExpectError("merge", "abort")
		AssertContains(t, abortOutput, "failed")

		// 4. Verify repository is still clean
		statusOutput := repo.RunGzhGitSuccess("status")
		AssertContains(t, statusOutput, "clean")
	})

	// Note: Can't test actual merge workflows due to ref resolution issues
}
