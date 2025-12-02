// Package main demonstrates commit automation using gzh-cli-git library.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/gizzahub/gzh-cli-git/pkg/commit"
	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

func main() {
	ctx := context.Background()

	// Get repository path from args or use current directory
	repoPath := "."
	if len(os.Args) >= 2 {
		repoPath = os.Args[1]
	}

	// Create clients
	repoClient := repository.NewClient()
	templateMgr := commit.NewTemplateManager()
	validator := commit.NewValidator()
	generator := commit.NewGenerator()

	// Open repository
	repo, err := repoClient.Open(ctx, repoPath)
	if err != nil {
		log.Fatalf("Failed to open repository: %v", err)
	}

	fmt.Printf("Repository: %s\n\n", repo.Path)

	// Example 1: List available templates
	fmt.Println("=== Example 1: List Available Templates ===")
	templates, err := templateMgr.List(ctx)
	if err != nil {
		log.Printf("Warning: Failed to list templates: %v", err)
	} else {
		for _, tmplName := range templates {
			fmt.Printf("  - %s\n", tmplName)
		}
	}
	fmt.Println()

	// Example 2: Load and show template details
	fmt.Println("=== Example 2: Show Template Details ===")
	templateName := "conventional"
	template, err := templateMgr.Load(ctx, templateName)
	if err != nil {
		log.Printf("Warning: Failed to load template: %v", err)
	} else {
		fmt.Printf("Template: %s\n", template.Name)
		fmt.Printf("Description: %s\n", template.Description)
		fmt.Printf("Format: %s\n", template.Format)
		if len(template.Examples) > 0 {
			fmt.Println("Examples:")
			for _, ex := range template.Examples {
				fmt.Printf("  %s\n", ex)
			}
		}
	}
	fmt.Println()

	// Example 3: Validate a commit message
	fmt.Println("=== Example 3: Validate Commit Message ===")
	message := "feat(cli): add new status command"

	if template != nil {
		result, err := validator.Validate(ctx, message, template)
		if err != nil {
			fmt.Printf("✗ Validation error: %v\n", err)
		} else if result.Valid {
			fmt.Printf("✓ Valid message: %s\n", message)
		} else {
			fmt.Printf("✗ Invalid message: %s\n", message)
			for _, ve := range result.Errors {
				fmt.Printf("  Error: %s\n", ve.Message)
			}
		}
		for _, w := range result.Warnings {
			fmt.Printf("  Warning: %s\n", w.Message)
		}
	}
	fmt.Println()

	// Example 4: Auto-generate commit message
	fmt.Println("=== Example 4: Auto-Generate Commit Message ===")

	// Check if there are staged changes
	status, err := repoClient.GetStatus(ctx, repo)
	if err != nil {
		log.Fatalf("Failed to get status: %v", err)
	}

	if len(status.StagedFiles) == 0 {
		fmt.Println("No staged changes to commit")
		fmt.Println("Tip: Stage some files first with 'git add <files>'")
	} else {
		msg, err := generator.Generate(ctx, repo, commit.GenerateOptions{})
		if err != nil {
			log.Printf("Warning: Failed to generate message: %v", err)
		} else {
			fmt.Println("Generated commit message:")
			fmt.Println(msg)
			fmt.Println()
			fmt.Println("To use this message:")
			fmt.Printf("  git commit -m \"%s\"\n", msg)
		}
	}
	fmt.Println()

	// Example 5: Render a template
	fmt.Println("=== Example 5: Render Template ===")
	if template != nil {
		values := map[string]string{
			"type":        "feat",
			"scope":       "api",
			"description": "add user authentication endpoint",
		}
		rendered, err := templateMgr.Render(ctx, template, values)
		if err != nil {
			fmt.Printf("Failed to render: %v\n", err)
		} else {
			fmt.Printf("Rendered message: %s\n", rendered)
		}
	}
}
