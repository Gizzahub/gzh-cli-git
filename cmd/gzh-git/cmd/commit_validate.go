package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-git/pkg/commit"
)

var (
	validateTemplate string
	validateFile     string
)

// validateCmd represents the commit validate command
var validateCmd = &cobra.Command{
	Use:   "validate <message>",
	Short: "Validate a commit message",
	Long: `Validate a commit message against template rules.

The message is validated for:
  - Format compliance (conventional commits, etc.)
  - Length constraints
  - Required fields
  - Pattern matching

Returns exit code 0 if valid, 1 if invalid.`,
	Example: `  # Validate a message
  gzh-git commit validate "feat(auth): add login"

  # Validate from file
  gzh-git commit validate --file .git/COMMIT_EDITMSG

  # Use different template
  gzh-git commit validate "Version 1.0.0" --template semantic`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCommitValidate,
}

func init() {
	commitCmd.AddCommand(validateCmd)

	validateCmd.Flags().StringVar(&validateTemplate, "template", "conventional", "template to validate against")
	validateCmd.Flags().StringVar(&validateFile, "file", "", "read message from file")
}

func runCommitValidate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	var message string

	// Get message from file or argument
	if validateFile != "" {
		content, err := os.ReadFile(validateFile)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		message = string(content)
	} else if len(args) > 0 {
		message = args[0]
	} else {
		return fmt.Errorf("no message provided\nUse: gzh-git commit validate <message> or --file <path>")
	}

	// Load template
	templateMgr := commit.NewTemplateManager()
	tmpl, err := templateMgr.Load(ctx, validateTemplate)
	if err != nil {
		return fmt.Errorf("failed to load template: %w", err)
	}

	// Validate
	validator := commit.NewValidator()
	result, err := validator.Validate(ctx, message, tmpl)
	if err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	// Display results
	if !quiet {
		fmt.Printf("\n📋 Validating message:\n")
		fmt.Printf("  %s\n\n", message)
	}

	if result.Valid {
		if !quiet {
			fmt.Println("✅ Valid commit message")
		}

		// Show warnings if any
		if result.Warnings != nil && len(result.Warnings) > 0 {
			fmt.Println("\n⚠️  Warnings:")
			for _, warning := range result.Warnings {
				fmt.Printf("  - %s\n", warning.Message)
				if warning.Suggestion != "" {
					fmt.Printf("    Suggestion: %s\n", warning.Suggestion)
				}
			}
		}

		return nil
	}

	// Invalid message
	if !quiet {
		fmt.Println("❌ Invalid commit message")
		fmt.Println("Errors:")
	}

	for _, err := range result.Errors {
		fmt.Printf("  - %s", err.Message)
		if err.Line > 0 {
			fmt.Printf(" (line %d)", err.Line)
		}
		fmt.Println()
	}

	// Show warnings too
	if result.Warnings != nil && len(result.Warnings) > 0 {
		fmt.Println("\nWarnings:")
		for _, warning := range result.Warnings {
			fmt.Printf("  - %s\n", warning.Message)
			if warning.Suggestion != "" {
				fmt.Printf("    Suggestion: %s\n", warning.Suggestion)
			}
		}
	}

	os.Exit(1)
	return nil
}
