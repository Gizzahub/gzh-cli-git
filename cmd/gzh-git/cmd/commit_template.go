package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-git/pkg/commit"
)

// templateCmd represents the commit template command group
var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Manage commit message templates",
	Long: `List, show, and validate commit message templates.

Templates define the format and validation rules for commit messages.
Built-in templates: conventional, semantic`,
	Example: `  # List all templates
  gzh-git commit template list

  # Show template details
  gzh-git commit template show conventional

  # Validate custom template file
  gzh-git commit template validate mytemplate.yaml`,
}

// templateListCmd lists all available templates
var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available templates",
	Long:  "List all built-in and custom commit message templates.",
	RunE:  runTemplateList,
}

// templateShowCmd shows template details
var templateShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show template details",
	Long:  "Display detailed information about a specific template.",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateShow,
}

// templateValidateCmd validates a custom template file
var templateValidateCmd = &cobra.Command{
	Use:   "validate <file>",
	Short: "Validate a template file",
	Long:  "Validate a custom template YAML file for correctness.",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateValidate,
}

func init() {
	commitCmd.AddCommand(templateCmd)
	templateCmd.AddCommand(templateListCmd)
	templateCmd.AddCommand(templateShowCmd)
	templateCmd.AddCommand(templateValidateCmd)
}

func runTemplateList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	templateMgr := commit.NewTemplateManager()
	templates, err := templateMgr.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list templates: %w", err)
	}

	if len(templates) == 0 {
		fmt.Println("No templates available")
		return nil
	}

	if !quiet {
		fmt.Printf("\n📋 Available Templates (%d):\n\n", len(templates))
	}

	for _, name := range templates {
		tmpl, err := templateMgr.Load(ctx, name)
		if err != nil {
			fmt.Printf("  ❌ %s (failed to load)\n", name)
			continue
		}

		fmt.Printf("  • %s\n", tmpl.Name)
		if tmpl.Description != "" {
			fmt.Printf("    %s\n", tmpl.Description)
		}
		fmt.Println()
	}

	return nil
}

func runTemplateShow(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	templateName := args[0]

	templateMgr := commit.NewTemplateManager()
	tmpl, err := templateMgr.Load(ctx, templateName)
	if err != nil {
		return fmt.Errorf("failed to load template: %w", err)
	}

	// Display template information
	fmt.Printf("\n📋 Template: %s\n\n", tmpl.Name)

	if tmpl.Description != "" {
		fmt.Printf("Description: %s\n\n", tmpl.Description)
	}

	fmt.Printf("Format:\n  %s\n\n", tmpl.Format)

	// Show variables
	if len(tmpl.Variables) > 0 {
		fmt.Println("Variables:")
		for _, v := range tmpl.Variables {
			required := ""
			if v.Required {
				required = " (required)"
			}
			fmt.Printf("  • %s: %s%s\n", v.Name, v.Type, required)
			if v.Description != "" {
				fmt.Printf("    %s\n", v.Description)
			}
			if v.Default != "" {
				fmt.Printf("    Default: %s\n", v.Default)
			}
			if len(v.Options) > 0 {
				fmt.Printf("    Options: %v\n", v.Options)
			}
		}
		fmt.Println()
	}

	// Show validation rules
	if len(tmpl.Rules) > 0 {
		fmt.Println("Validation Rules:")
		for _, rule := range tmpl.Rules {
			fmt.Printf("  • %s\n", rule.Type)
			if rule.Pattern != "" {
				fmt.Printf("    Pattern: %s\n", rule.Pattern)
			}
			if rule.Message != "" {
				fmt.Printf("    Message: %s\n", rule.Message)
			}
		}
		fmt.Println()
	}

	return nil
}

func runTemplateValidate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	templateFile := args[0]

	// Check file exists
	if _, err := os.Stat(templateFile); os.IsNotExist(err) {
		return fmt.Errorf("template file not found: %s", templateFile)
	}

	// Load custom template
	templateMgr := commit.NewTemplateManager()
	tmpl, err := templateMgr.LoadCustom(ctx, templateFile)
	if err != nil {
		if !quiet {
			fmt.Printf("\n❌ Template validation failed:\n")
			fmt.Printf("  %s\n\n", err.Error())
		}
		os.Exit(1)
		return nil
	}

	// Validate template structure
	if err := templateMgr.Validate(ctx, tmpl); err != nil {
		if !quiet {
			fmt.Printf("\n❌ Template validation failed:\n")
			fmt.Printf("  %s\n\n", err.Error())
		}
		os.Exit(1)
		return nil
	}

	// Success
	if !quiet {
		fmt.Printf("\n✅ Template is valid!\n\n")
		fmt.Printf("Template: %s\n", tmpl.Name)
		if tmpl.Description != "" {
			fmt.Printf("Description: %s\n", tmpl.Description)
		}
		fmt.Printf("Variables: %d\n", len(tmpl.Variables))
		fmt.Printf("Rules: %d\n", len(tmpl.Rules))
		fmt.Println()
	}

	return nil
}
