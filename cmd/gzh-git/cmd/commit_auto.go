package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-git/pkg/commit"
	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

var (
	autoTemplate string
	autoScope    string
	autoType     string
	autoDryRun   bool
	autoEdit     bool
)

// autoCmd represents the commit auto command
var autoCmd = &cobra.Command{
	Use:   "auto",
	Short: "Automatically generate and create a commit",
	Long: `Analyze staged changes and automatically generate a commit message
following conventional commit format.

This command will:
1. Analyze staged changes (git diff --cached)
2. Infer commit type (feat, fix, docs, etc.)
3. Detect scope from changed files
4. Generate a descriptive message
5. Validate the message
6. Create the commit (unless --dry-run)`,
	Example: `  # Auto-commit staged changes
  git add .
  gzh-git commit auto

  # Preview message without committing
  gzh-git commit auto --dry-run

  # Override detected type and scope
  gzh-git commit auto --type feat --scope auth

  # Use different template
  gzh-git commit auto --template semantic`,
	RunE: runCommitAuto,
}

func init() {
	commitCmd.AddCommand(autoCmd)

	autoCmd.Flags().StringVar(&autoTemplate, "template", "conventional", "template to use (conventional|semantic)")
	autoCmd.Flags().StringVar(&autoScope, "scope", "", "override detected scope")
	autoCmd.Flags().StringVar(&autoType, "type", "", "override detected type")
	autoCmd.Flags().BoolVar(&autoDryRun, "dry-run", false, "show message without committing")
	autoCmd.Flags().BoolVar(&autoEdit, "edit", false, "open editor to edit message before committing")
}

func runCommitAuto(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get repository path
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Create client
	client := repository.NewClient()

	// Check if it's a repository
	if !client.IsRepository(ctx, absPath) {
		return fmt.Errorf("not a git repository: %s", absPath)
	}

	// Open repository
	repo, err := client.Open(ctx, absPath)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Load template
	templateMgr := commit.NewTemplateManager()
	tmpl, err := templateMgr.Load(ctx, autoTemplate)
	if err != nil {
		return fmt.Errorf("failed to load template: %w", err)
	}

	// Create generator
	gen := commit.NewGenerator()

	// Generate commit message
	message, err := gen.Generate(ctx, repo, commit.GenerateOptions{
		Template:  tmpl,
		MaxLength: 72,
	})
	if err != nil {
		if err == commit.ErrNoChanges {
			return fmt.Errorf("no changes staged for commit\nUse 'git add <file>...' to stage changes")
		}
		return fmt.Errorf("failed to generate commit message: %w", err)
	}

	// Validate the message
	validator := commit.NewValidator()
	result, err := validator.Validate(ctx, message, tmpl)
	if err != nil {
		return fmt.Errorf("failed to validate message: %w", err)
	}

	// Show generated message
	if !quiet {
		fmt.Printf("\n📝 Generated Commit Message:\n\n")
		fmt.Printf("  %s\n\n", message)

		if result.Warnings != nil && len(result.Warnings) > 0 {
			fmt.Println("⚠️  Warnings:")
			for _, warning := range result.Warnings {
				fmt.Printf("  - %s\n", warning.Message)
				if warning.Suggestion != "" {
					fmt.Printf("    Suggestion: %s\n", warning.Suggestion)
				}
			}
			fmt.Println()
		}
	}

	// Check for validation errors
	if !result.Valid {
		fmt.Fprintln(os.Stderr, "\n❌ Validation failed:")
		for _, err := range result.Errors {
			fmt.Fprintf(os.Stderr, "  - %s", err.Message)
			if err.Line > 0 {
				fmt.Fprintf(os.Stderr, " (line %d)", err.Line)
			}
			fmt.Fprintln(os.Stderr)
		}
		return fmt.Errorf("commit message validation failed")
	}

	// Dry run mode
	if autoDryRun {
		if !quiet {
			fmt.Println("✅ Dry run mode - no commit created")
		}
		return nil
	}

	// Edit mode (future enhancement - for now just show warning)
	if autoEdit {
		fmt.Fprintln(os.Stderr, "⚠️  --edit flag not yet implemented")
		fmt.Fprintln(os.Stderr, "Using generated message as-is")
	}

	// Create commit using git command
	if !quiet {
		fmt.Println("\n🔨 Creating commit...")
	}

	// Execute git commit directly using os/exec
	gitCmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
	gitCmd.Dir = repo.Path

	output, err := gitCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create commit: %w\nOutput: %s", err, string(output))
	}

	if !quiet {
		fmt.Println("\n✅ Commit created successfully!")
		fmt.Println(string(output))
	}

	return nil
}
