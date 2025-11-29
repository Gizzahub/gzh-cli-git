package integration

import (
	"strings"
	"testing"
)

func TestHistoryStatsCommand(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("basic statistics", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("history", "stats")

		AssertContains(t, output, "Total Commits:")
		AssertContains(t, output, "Contributors:")
	})

	t.Run("with since filter", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("history", "stats", "--since", "1 year ago")

		AssertContains(t, output, "Total Commits:")
	})

	t.Run("with branch filter", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("history", "stats", "--branch", "master")

		AssertContains(t, output, "Total Commits:")
	})

	t.Run("with author filter", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("history", "stats", "--author", "Test User")

		AssertContains(t, output, "Total Commits:")
	})

	t.Run("json format", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("history", "stats", "--format", "json")

		// Should be valid JSON
		AssertContains(t, output, "{")
		AssertContains(t, output, "\"total_commits\"")
	})

	t.Run("csv format", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("history", "stats", "--format", "csv")

		// Should be CSV format
		if !strings.Contains(output, ",") {
			t.Logf("Expected CSV format, got: %s", output)
		}
	})

	t.Run("markdown format", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("history", "stats", "--format", "markdown")

		// Should contain markdown table
		AssertContains(t, output, "|")
	})
}

func TestHistoryContributorsCommand(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("list all contributors", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("history", "contributors")

		AssertContains(t, output, "Test User")
	})

	t.Run("top N contributors", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("history", "contributors", "--top", "5")

		AssertContains(t, output, "Test User")
	})

	t.Run("sort by commits", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("history", "contributors", "--sort", "commits")

		AssertContains(t, output, "Test User")
	})

	t.Run("sort by lines added", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("history", "contributors", "--sort", "additions")

		AssertContains(t, output, "Test User")
	})

	t.Run("with since filter", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("history", "contributors", "--since", "1 month ago")

		AssertContains(t, output, "Test User")
	})

	t.Run("json format", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("history", "contributors", "--format", "json")

		AssertContains(t, output, "{")
		AssertContains(t, output, "\"name\"")
	})
}

func TestHistoryFileCommand(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("file history", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("history", "file", "README.md")

		// Should show commit history for README.md
		AssertContains(t, output, "README")
	})

	t.Run("file with max commits", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("history", "file", "README.md", "--max", "5")

		// Should limit output
		AssertContains(t, output, "README")
	})

	t.Run("non-existent file", func(t *testing.T) {
		output := repo.RunGzhGitExpectError("history", "file", "nonexistent.txt")

		// Should fail gracefully
		if !strings.Contains(output, "not found") && !strings.Contains(output, "does not exist") {
			t.Logf("Expected 'not found' error, got: %s", output)
		}
	})

	t.Run("follow renames", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("history", "file", "README.md", "--follow")

		AssertContains(t, output, "README")
	})
}

func TestHistoryBlameCommand(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("blame file", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("history", "blame", "README.md")

		// Should show line-by-line authorship
		AssertContains(t, output, "Test User")
	})

	t.Run("blame with line range", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("history", "blame", "README.md", "--lines", "1,5")

		AssertContains(t, output, "Test User")
	})

	t.Run("blame non-existent file", func(t *testing.T) {
		output := repo.RunGzhGitExpectError("history", "blame", "nonexistent.txt")

		// Should fail
		if !strings.Contains(output, "not found") && !strings.Contains(output, "does not exist") {
			t.Logf("Expected 'not found' error, got: %s", output)
		}
	})

	t.Run("blame with email", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("history", "blame", "README.md", "--email")

		// Should show email addresses
		AssertContains(t, output, "test@example.com")
	})
}

func TestHistoryWorkflow(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("complete history analysis workflow", func(t *testing.T) {
		// 1. Get overall stats
		statsOutput := repo.RunGzhGitSuccess("history", "stats")
		AssertContains(t, statsOutput, "Total Commits:")

		// 2. Analyze contributors
		contribOutput := repo.RunGzhGitSuccess("history", "contributors")
		AssertContains(t, contribOutput, "Test User")

		// 3. Check file history
		fileOutput := repo.RunGzhGitSuccess("history", "file", "README.md")
		AssertContains(t, fileOutput, "README")

		// 4. Blame a file
		blameOutput := repo.RunGzhGitSuccess("history", "blame", "README.md")
		AssertContains(t, blameOutput, "Test User")
	})

	t.Run("advanced filtering", func(t *testing.T) {
		// Create more commits with different files
		repo.WriteFile("feature.go", "package main\n")
		repo.GitAdd("feature.go")
		repo.GitCommit("Add feature")

		repo.WriteFile("docs/guide.md", "# Guide\n")
		repo.GitAdd("docs/guide.md")
		repo.GitCommit("Add documentation")

		// Filter by recent commits
		output := repo.RunGzhGitSuccess("history", "stats", "--since", "1 day ago")
		AssertContains(t, output, "Total Commits:")

		// Check specific file
		fileOutput := repo.RunGzhGitSuccess("history", "file", "feature.go")
		AssertContains(t, fileOutput, "feature.go")
	})
}
