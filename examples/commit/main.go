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
	commitManager := commit.NewManager()

	// Open repository
	repo, err := repoClient.Open(ctx, repoPath)
	if err != nil {
		log.Fatalf("Failed to open repository: %v", err)
	}

	fmt.Printf("Repository: %s\n\n", repo.Path)

	// Example 1: Validate a commit message
	fmt.Println("=== Example 1: Validate Commit Message ===")
	message := "feat(cli): add new status command"

	err = commitManager.ValidateMessage(ctx, repo, message)
	if err != nil {
		fmt.Printf("✗ Invalid message: %v\n", err)
	} else {
		fmt.Printf("✓ Valid message: %s\n", message)
	}
	fmt.Println()

	// Example 2: List available templates
	fmt.Println("=== Example 2: List Available Templates ===")
	templates, err := commitManager.ListTemplates(ctx)
	if err != nil {
		log.Printf("Warning: Failed to list templates: %v", err)
	} else {
		for _, tmpl := range templates {
			fmt.Printf("  - %s: %s\n", tmpl.Name, tmpl.Description)
		}
	}
	fmt.Println()

	// Example 3: Show template details
	fmt.Println("=== Example 3: Show Template Details ===")
	templateName := "conventional"
	template, err := commitManager.GetTemplate(ctx, templateName)
	if err != nil {
		log.Printf("Warning: Failed to get template: %v", err)
	} else {
		fmt.Printf("Template: %s\n", template.Name)
		fmt.Printf("Description: %s\n", template.Description)
		fmt.Printf("Format: %s\n", template.Format)
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
		msg, err := commitManager.AutoGenerateMessage(ctx, repo)
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
}
