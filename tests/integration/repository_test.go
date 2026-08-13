package integration

import (
	"testing"
)

func TestStatusCommand(t *testing.T) {
	// NOTE: gz-git status is a DIAGNOSTIC status command that scans directories for repositories.
	// It outputs diagnostic health format with comprehensive checks.

	t.Run("clean repository", func(t *testing.T) {
		repo := NewTestRepo(t)
		repo.SetupWithCommits()

		output := repo.RunGzhGitSuccess("status")

		// Diagnostic status output format
		AssertContains(t, output, "Status")
		AssertContains(t, output, "All 1 repositories are healthy")
	})

	t.Run("with uncommitted changes", func(t *testing.T) {
		repo := NewTestRepo(t)
		repo.SetupWithCommits()
		repo.WriteFile("modified.txt", "changed content")
		repo.GitAdd("modified.txt")

		output := repo.RunGzhGitSuccess("status")

		// Diagnostic status shows warning (staged changes are uncommitted and need attention)
		// Note: Local test repos show "no-upstream" warning which takes precedence in recommendations
		AssertContains(t, output, "Status")
		AssertContains(t, output, "warning")
		AssertContains(t, output, "modified")
	})

	t.Run("with untracked files", func(t *testing.T) {
		repo := NewTestRepo(t)
		repo.SetupWithCommits()
		repo.WriteFile("untracked.txt", "untracked content")

		output := repo.RunGzhGitSuccess("status")

		// Diagnostic status shows healthy (untracked files alone are not considered dirty)
		AssertContains(t, output, "Status")
		AssertContains(t, output, "healthy")
	})
}

func TestInfoCommand(t *testing.T) {
	repo := NewTestRepo(t)
	repo.SetupWithCommits()

	t.Run("basic repository info", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("info")

		// The default view is one line per repository under a column header.
		AssertContains(t, output, "REPOSITORY")
		AssertContains(t, output, "BRANCH")
		AssertContains(t, output, "master")
	})

	t.Run("with multiple branches", func(t *testing.T) {
		repo.GitBranch("feature-1")
		repo.GitBranch("feature-2")

		output := repo.RunGzhGitSuccess("info")

		// Branches other than the current one are summarized in a single column
		// rather than listed per repository block.
		AssertContains(t, output, "OTHER BRANCHES")
		AssertContains(t, output, "feature-1")
		AssertContains(t, output, "feature-2")
	})

	t.Run("detail view", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("info", "--full")

		AssertContains(t, output, "📦")               // Repository indicator
		AssertContains(t, output, "Current Branch:") // Branch info
		AssertContains(t, output, "Status:")         // Status field
		AssertContains(t, output, "Base:")           // Integration branch
	})

	t.Run("verbose detail output", func(t *testing.T) {
		output := repo.RunGzhGitSuccess("info", "--full", "--verbose")

		AssertContains(t, output, "📦")               // Repository indicator
		AssertContains(t, output, "Current Branch:") // Branch info
		// Verbose mode should show more details like Author
		AssertContains(t, output, "Author:")
	})

	t.Run("audit emits a machine-readable document", func(t *testing.T) {
		// This fixture tracks no upstream, so the audit always has a finding and
		// the command exits non-zero by the diagnostic convention. The document
		// is still emitted in full — that is what distinguishes "findings" from
		// "the tool failed", and the assertions below check exactly that.
		output := repo.RunGzhGitExpectError("info", "--audit")

		AssertContains(t, output, `"schema": "gz-git.info.audit/v1"`)
		AssertContains(t, output, `"audit_complete": true`)
		AssertContains(t, output, "NO_UPSTREAM")
	})
}

func TestCloneCommand(t *testing.T) {
	t.Run("invalid URL shows error in results", func(t *testing.T) {
		tmpDir := t.TempDir()
		repo := &TestRepo{Path: tmpDir, T: t}

		// Bulk clone mode: errors are shown in results AND the command
		// exits non-zero so scripts/CI can detect partial failure.
		// Use --url flag pattern (consistent with commit --messages)
		output := repo.RunGzhGitExpectError("clone", "--url", "invalid-url")

		// Should show error status in results
		AssertContains(t, output, "error")
		AssertContains(t, output, "Total failed")
	})

	t.Run("clone from local repository", func(t *testing.T) {
		// Create source repository
		source := NewTestRepo(t)
		source.SetupWithCommits()

		// Create target directory
		targetDir := t.TempDir()

		// Clone should work with local path (bulk mode)
		// Pattern: gz-git clone [directory] --url <url>
		target := &TestRepo{Path: targetDir, T: t}
		output := target.RunGzhGitSuccess("clone", "--url", source.Path)

		AssertContains(t, output, "Cloning")
		AssertContains(t, output, "Bulk Clone Results")
	})

	t.Run("clone multiple URLs", func(t *testing.T) {
		// Create two source repositories
		source1 := NewTestRepo(t)
		source1.SetupWithCommits()

		source2 := NewTestRepo(t)
		source2.SetupWithCommits()

		// Create target directory
		targetDir := t.TempDir()

		// Clone both repositories using multiple --url flags
		target := &TestRepo{Path: targetDir, T: t}
		output := target.RunGzhGitSuccess("clone", "--url", source1.Path, "--url", source2.Path)

		AssertContains(t, output, "Cloning 2 repositories")
		AssertContains(t, output, "Total cloned")
	})
}

func TestStatusNotARepository(t *testing.T) {
	// NOTE: gz-git status is a BULK status command that scans directories for repositories.
	// It does NOT fail when run in a non-git directory; it simply reports "no repositories found".
	tmpDir := t.TempDir()
	repo := &TestRepo{Path: tmpDir, T: t}

	output := repo.RunGzhGitSuccess("status")

	// Diagnostic status completes successfully but finds no repositories
	AssertContains(t, output, "No repositories found")
}
