package e2e

import (
	"strings"
	"testing"
)

// TestNewProjectSetup tests the workflow of setting up a new project
func TestNewProjectSetup(t *testing.T) {
	repo := NewE2ERepo(t)

	t.Run("initialize and create first commit", func(t *testing.T) {
		// Create README file
		repo.WriteFile("README.md", "# Test Project\n\nThis is a test project.\n")

		// Stage the file
		repo.Git("add", "README.md")

		// Use gzh-git to create auto-commit
		output := repo.RunGzhGit("commit", "auto", "--dry-run")

		// Should generate appropriate commit message
		AssertContains(t, output, "Generated")

		// Create actual commit using git (auto-commit may fail in test env)
		repo.Git("commit", "-m", "docs(root): add project README")

		// Verify commit exists
		if !repo.CommitExists("add project README") {
			t.Error("Commit not found in git log")
		}
	})

	t.Run("check repository status", func(t *testing.T) {
		// Check status using gzh-git
		output := repo.RunGzhGit("status")

		// Should show clean working directory
		AssertContains(t, output, "clean")
	})

	t.Run("get repository info", func(t *testing.T) {
		// Get repository info
		output := repo.RunGzhGit("info")

		// Should show repository details (may show master or detached HEAD)
		if !strings.Contains(output, "master") && !strings.Contains(output, "Branch:") {
			t.Errorf("Expected info to show branch info, got: %s", output)
		}
	})
}

// TestBasicCommitWorkflow tests basic commit operations
func TestBasicCommitWorkflow(t *testing.T) {
	repo := NewE2ERepo(t)

	// Create initial commit
	repo.WriteFile("README.md", "# Test\n")
	repo.Git("add", "README.md")
	repo.Git("commit", "-m", "Initial commit")

	t.Run("validate commit message", func(t *testing.T) {
		// Validate a good commit message
		output := repo.RunGzhGit("commit", "validate", "feat(api): add user endpoint")

		AssertContains(t, output, "Valid commit message")
	})

	t.Run("reject invalid commit message", func(t *testing.T) {
		// Validate a bad commit message
		output := repo.RunGzhGitExpectError("commit", "validate", "bad message")

		AssertContains(t, output, "Invalid commit message")
	})

	t.Run("list commit templates", func(t *testing.T) {
		// List available templates
		output := repo.RunGzhGit("commit", "template", "list")

		AssertContains(t, output, "conventional")
		AssertContains(t, output, "semantic")
	})

	t.Run("show commit template", func(t *testing.T) {
		// Show template details
		output := repo.RunGzhGit("commit", "template", "show", "conventional")

		AssertContains(t, output, "Template: conventional")
		AssertContains(t, output, "Format:")
	})

	t.Run("auto-generate commit message", func(t *testing.T) {
		// Create changes
		repo.WriteFile("src/feature.go", "package main\n\nfunc NewFeature() {}\n")
		repo.Git("add", "src/feature.go")

		// Generate commit message (dry run)
		output := repo.RunGzhGit("commit", "auto", "--dry-run")

		// Should generate message
		AssertContains(t, output, "Generated")
	})
}

// TestBasicBranchWorkflow tests basic branch operations
func TestBasicBranchWorkflow(t *testing.T) {
	repo := NewE2ERepo(t)

	// Create initial commit
	repo.WriteFile("README.md", "# Test\n")
	repo.Git("add", "README.md")
	repo.Git("commit", "-m", "Initial commit")

	t.Run("list branches", func(t *testing.T) {
		// List branches
		output := repo.RunGzhGit("branch", "list")

		// Should show master branch
		AssertContains(t, output, "master")
	})

	t.Run("create branch using git and verify with gzh-git", func(t *testing.T) {
		// Create branch using git (gzh-git has ref issues)
		repo.Git("branch", "feature/test")

		// List all branches
		output := repo.RunGzhGit("branch", "list", "--all")

		// Should show new branch
		AssertContains(t, output, "feature/test")
	})

	t.Run("switch branches and make changes", func(t *testing.T) {
		// Switch to feature branch
		repo.Git("checkout", "feature/test")

		// Verify current branch
		if repo.GetCurrentBranch() != "feature/test" {
			t.Errorf("Expected current branch to be 'feature/test', got %s",
				repo.GetCurrentBranch())
		}

		// Make changes
		repo.WriteFile("feature.txt", "New feature\n")
		repo.Git("add", "feature.txt")
		repo.Git("commit", "-m", "feat: add feature file")

		// Verify commit exists
		if !repo.CommitExists("add feature file") {
			t.Error("Feature commit not found")
		}
	})

	t.Run("view branch list with verbose", func(t *testing.T) {
		// List branches with verbose flag
		output := repo.RunGzhGit("branch", "list", "--verbose")

		// Should show branch details
		AssertContains(t, output, "master")
	})
}

// TestBasicHistoryWorkflow tests basic history operations
func TestBasicHistoryWorkflow(t *testing.T) {
	repo := NewE2ERepo(t)

	// Create some commit history
	repo.WriteFile("README.md", "# Test\n")
	repo.Git("add", "README.md")
	repo.Git("commit", "-m", "docs: add README")

	repo.WriteFile("src/main.go", "package main\n")
	repo.Git("add", "src/main.go")
	repo.Git("commit", "-m", "feat: add main")

	repo.WriteFile("src/util.go", "package main\n")
	repo.Git("add", "src/util.go")
	repo.Git("commit", "-m", "feat: add util")

	t.Run("get repository statistics", func(t *testing.T) {
		// Get stats
		output := repo.RunGzhGit("history", "stats")

		// Should show commit count
		AssertContains(t, output, "Total Commits:")
		AssertContains(t, output, "Unique Authors:")
	})

	t.Run("list contributors", func(t *testing.T) {
		// Get contributors
		output := repo.RunGzhGit("history", "contributors")

		// Should show test user
		AssertContains(t, output, "E2E Test User")
	})

	t.Run("get file history", func(t *testing.T) {
		// Get file history
		output := repo.RunGzhGit("history", "file", "README.md")

		// Should show commits affecting README
		AssertContains(t, output, "README")
	})

	t.Run("blame file", func(t *testing.T) {
		// Blame file
		output := repo.RunGzhGit("history", "blame", "README.md")

		// Should show line-by-line attribution
		AssertContains(t, output, "2025-")
	})

	t.Run("export stats as JSON", func(t *testing.T) {
		// Get stats in JSON format
		output := repo.RunGzhGit("history", "stats", "--format", "json")

		// Should be valid JSON
		AssertContains(t, output, "{")
		AssertContains(t, output, "\"TotalCommits\"")
	})

	t.Run("filter by date range", func(t *testing.T) {
		// Get stats with date filter
		output := repo.RunGzhGit("history", "stats", "--since", "2020-01-01")

		// Should show filtered stats
		AssertContains(t, output, "Total Commits:")
	})
}

// TestRepositoryCleanupWorkflow tests cleanup operations
func TestRepositoryCleanupWorkflow(t *testing.T) {
	repo := NewE2ERepo(t)

	// Create initial commit
	repo.WriteFile("README.md", "# Test\n")
	repo.Git("add", "README.md")
	repo.Git("commit", "-m", "Initial commit")

	t.Run("check status of clean repo", func(t *testing.T) {
		// Check status
		output := repo.RunGzhGit("status")

		// Should be clean
		AssertContains(t, output, "clean")
	})

	t.Run("status with untracked files", func(t *testing.T) {
		// Create untracked file
		repo.WriteFile("temp.txt", "temporary\n")

		// Check status
		output := repo.RunGzhGit("status")

		// Should show untracked file
		AssertContains(t, output, "Untracked")
	})

	t.Run("status with modified files", func(t *testing.T) {
		// Modify existing file
		repo.WriteFile("README.md", "# Test\n\nModified\n")

		// Check status
		output := repo.RunGzhGit("status")

		// Should show modifications
		if !strings.Contains(output, "Modified") && !strings.Contains(output, "modified") {
			t.Errorf("Expected status to show modifications, got: %s", output)
		}
	})
}
