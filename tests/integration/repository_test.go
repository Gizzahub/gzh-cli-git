package integration

import (
	"strings"
	"testing"
)

func TestStatusCommand(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("clean repository", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("status")

		AssertContains(t, output, "clean")
	})

	t.Run("with unstaged changes", func(t *testing.T) {
		repo.WriteFile("modified.txt", "changed content")

		output := repo.RunGzhGitSuccess("status")

		AssertContains(t, output, "Untracked")
		AssertContains(t, output, "modified.txt")
	})

	t.Run("with staged changes", func(t *testing.T) {
		repo.WriteFile("staged.txt", "new file")
		repo.GitAdd("staged.txt")

		output := repo.RunGzhGitSuccess("status")

		AssertContains(t, output, "Changes to be committed")
		AssertContains(t, output, "staged.txt")
	})

	t.Run("with untracked files", func(t *testing.T) {
		repo.WriteFile("untracked.txt", "untracked content")

		output := repo.RunGzhGitSuccess("status")

		AssertContains(t, output, "Untracked files")
		AssertContains(t, output, "untracked.txt")
	})
}

func TestInfoCommand(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("basic repository info", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("info")

		AssertContains(t, output, "Repository:")
		AssertContains(t, output, "Branch:")
		AssertContains(t, output, "Status:")
	})

	t.Run("with multiple branches", func(t *testing.T) {
		repo.GitBranch("feature-1")
		repo.GitBranch("feature-2")

		output := repo.RunGzhGitSuccess("info")

		AssertContains(t, output, "Repository:")
	})

	t.Run("verbose output", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("info", "--verbose")

		AssertContains(t, output, "Repository:")
		// Verbose mode should show more details
	})
}

func TestCloneCommand(t *testing.T) {
	t.Run("invalid URL", func(t *testing.T) {
		tmpDir := t.TempDir()
		repo := &TestRepo{Path: tmpDir, T: t}

		output := repo.RunGzhGitExpectError("clone", "invalid-url", tmpDir)

		// Should fail with appropriate error
		if !strings.Contains(output, "failed") && !strings.Contains(output, "error") {
			t.Errorf("Expected error message, got: %s", output)
		}
	})

	t.Run("clone from local repository", func(t *testing.T) {
		// Create source repository
		source := NewTestRepo(t)
		source.SetupWithCommits()

		// Create target directory
		targetDir := t.TempDir()

		// Clone should work with local path
		target := &TestRepo{Path: targetDir, T: t}
		output := target.RunGzhGitSuccess("clone", source.Path, targetDir)

		AssertContains(t, output, "Cloning")
	})
}

func TestStatusNotARepository(t *testing.T) {
	tmpDir := t.TempDir()
	repo := &TestRepo{Path: tmpDir, T: t}

	output := repo.RunGzhGitExpectError("status")

	// Should fail because it's not a Git repository
	if !strings.Contains(output, "not a git repository") && !strings.Contains(output, "failed") {
		t.Errorf("Expected 'not a repository' error, got: %s", output)
	}
}
