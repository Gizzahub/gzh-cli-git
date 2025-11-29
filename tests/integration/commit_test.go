package integration

import (
	"strings"
	"testing"
)

func TestCommitAutoCommand(t *testing.T) {
	t.Run("with staged changes", func(t *testing.T) {
		repo := NewTestRepo(t)
		repo.SetupWithCommits()

		// Create and stage changes
		repo.WriteFile("feature.go", "package main\n\nfunc NewFeature() {}\n")
		repo.GitAdd("feature.go")

		output := repo.RunGzhGitSuccess("commit", "auto", "--dry-run")

		// Should generate a commit message
		AssertContains(t, output, "feat")
	})

	t.Run("no staged changes", func(t *testing.T) {
		repo := NewTestRepo(t)
		repo.SetupWithCommits()

		output := repo.RunGzhGitExpectError("commit", "auto")

		// Should fail with no changes
		if !strings.Contains(output, "no changes") && !strings.Contains(output, "nothing to commit") && !strings.Contains(output, "failed") {
			t.Logf("Expected 'no changes' error, got: %s", output)
		}
	})

	t.Run("with template", func(t *testing.T) {
		repo := NewTestRepo(t)
		repo.SetupWithCommits()

		repo.WriteFile("docs/readme.md", "# Documentation\n")
		repo.GitAdd("docs/readme.md")

		output := repo.RunGzhGitSuccess("commit", "auto", "--template", "conventional", "--dry-run")

		// Should use conventional template
		AssertContains(t, output, "docs")
	})

	t.Run("dry run mode", func(t *testing.T) {
		repo := NewTestRepo(t)
		repo.SetupWithCommits()

		repo.WriteFile("test.txt", "test content")
		repo.GitAdd("test.txt")

		output := repo.RunGzhGitSuccess("commit", "auto", "--dry-run")

		// Should show message but not commit
		AssertContains(t, output, "Generated")
		AssertNotContains(t, output, "created successfully")
	})
}

func TestCommitValidateCommand(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("valid conventional commit", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("commit", "validate", "feat(api): add user endpoint")

		AssertContains(t, output, "Valid commit message")
	})

	t.Run("invalid format", func(t *testing.T) {
		output := repo.RunGzhGitExpectError("commit", "validate", "bad commit message")

		// Should show validation errors
		AssertContains(t, output, "Invalid commit message")
	})

	t.Run("missing type", func(t *testing.T) {
		output := repo.RunGzhGitExpectError("commit", "validate", "add new feature")

		AssertContains(t, output, "Invalid commit message")
	})

	t.Run("subject too long", func(t *testing.T) {
		longSubject := "feat: " + strings.Repeat("a", 100)
		output := repo.RunGzhGitExpectError("commit", "validate", longSubject)

		AssertContains(t, output, "Invalid commit message")
	})

	t.Run("with custom template", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("commit", "validate",
			"--template", "conventional",
			"fix(core): resolve memory leak")

		AssertContains(t, output, "Valid commit message")
	})
}

func TestCommitTemplateCommands(t *testing.T) {
	repo := NewTestRepo(t)

	t.Run("list templates", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("commit", "template", "list")

		// Should show built-in templates
		AssertContains(t, output, "conventional")
		AssertContains(t, output, "semantic")
	})

	t.Run("show template", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("commit", "template", "show", "conventional")

		// Should show template details
		AssertContains(t, output, "Template: conventional")
		AssertContains(t, output, "Format:")
	})

	t.Run("show non-existent template", func(t *testing.T) {
		output := repo.RunGzhGitExpectError("commit", "template", "show", "nonexistent")

		AssertContains(t, output, "not found")
	})

	t.Run("show semantic template", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("commit", "template", "show", "semantic")

		AssertContains(t, output, "Template: semantic")
	})
}
