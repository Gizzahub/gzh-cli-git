// Package cmd implements the CLI commands for gz-git.
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
)

var (
	// version is set by main.go.
	appVersion string

	// Global flags.
	verbose         bool
	quiet           bool
	profileOverride string // Override active profile
	rootFormat      string // Root command format (local)
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "gz-git",
	Short: "Advanced Git operations CLI",
	Long: `gz-git is a bulk-first Git CLI that runs safe operations across many repositories in parallel.

Two engines:
  • Ad-hoc scan  — status/fetch/pull/push/update/clone (directory tree, no config required)
  • Declarative  — workspace sync / forge from (converge from .gz-git.yaml or forge org)

` + cliutil.QuickStartHelp(`  # Smart sync — auto-inits .gz-git.yaml if absent, then syncs
  gz-git sync

  # Initialize workspace config (scan existing repos)
  gz-git workspace init

  # Check status of all repositories
  gz-git status

  See 'gz-git schema' for configuration reference.`),
	Version: appVersion,
	Run:     runRoot,
}

func runRoot(cmd *cobra.Command, args []string) {
	if rootFormat == "llm" {
		generateLLMDocs(cmd)
		return
	}
	cmd.Help()
}

func generateLLMDocs(cmd *cobra.Command) {
	fmt.Println("# GZ-Git CLI Tool Specification")
	fmt.Println("\nThis document defines the capabilities and interface of the gz-git CLI for AI Agents.")
	fmt.Println("Hierarchy: Top-level commands (##) -> Subcommands (###)")

	fmt.Println("\n## Global Flags")
	fmt.Println("- `-v, --verbose`: Enable verbose logging (use for debugging)")
	fmt.Println("- `-q, --quiet`: Suppress output (errors only)")
	fmt.Println("- `--profile <name>`: Switch configuration profile")

	fmt.Println("\n## Available Commands")
	// Start recursion with level 2 (##)
	printCommandRecursive(cmd, 2)
}

func printCommandRecursive(cmd *cobra.Command, level int) {
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || c.Name() == "help" {
			continue
		}

		// Calculate header string based on level
		header := strings.Repeat("#", level)

		// Header
		// Use just the name for subcommands to save horizontal space, but fully qualified usage line is there
		fmt.Printf("\n%s `%s`\n", header, c.Name())
		fmt.Printf("- **Path**: `%s`\n", c.CommandPath())
		fmt.Printf("- **Purpose**: %s\n", c.Short)
		fmt.Printf("- **Usage**: `%s`\n", c.UseLine())

		// Flags
		hasLocalFlags := false
		var flagLines []string
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if f.Hidden {
				return
			}
			hasLocalFlags = true
			var typeStr string
			if f.Value.Type() == "bool" {
				typeStr = ""
			} else {
				typeStr = fmt.Sprintf(" <%s>", f.Value.Type())
			}
			flagLines = append(flagLines, fmt.Sprintf("  - `--%s%s`: %s", f.Name, typeStr, f.Usage))
		})

		if hasLocalFlags {
			fmt.Println("- **Flags**:")
			for _, line := range flagLines {
				fmt.Println(line)
			}
		}

		// Examples (only in verbose mode to save tokens)
		if verbose && c.Long != "" {
			lines := strings.Split(c.Long, "\n")
			inQuickStart := false
			var exampleLines []string

			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if strings.Contains(trimmed, "Quick Start:") {
					inQuickStart = true
					continue
				}
				if inQuickStart {
					if trimmed == "" {
						continue
					}
					exampleLines = append(exampleLines, "  "+line)
				}
			}

			if len(exampleLines) > 0 {
				fmt.Println("- **Examples**:")
				for _, line := range exampleLines {
					fmt.Println(line)
				}
			} else if !inQuickStart && len(lines) < 5 {
				// Fallback description embedded if short
				// Don't print long descriptions to save tokens
			}
		}

		// Recurse with deeper indentation
		printCommandRecursive(c, level+1)
	}
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute(version string) {
	appVersion = version
	rootCmd.Version = version

	setCommandGroups(rootCmd)
	applyUsageTemplateRecursive(rootCmd, buildUsageTemplate())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(cliutil.ExitCodeForError(err))
	}
}

func setCommandGroups(cmd *cobra.Command) {
	coreGroup := &cobra.Group{ID: "core", Title: cliutil.ColorYellowBold + "Core Git Operations" + cliutil.ColorReset}
	mgmtGroup := &cobra.Group{ID: "mgmt", Title: cliutil.ColorYellowBold + "Management & Configuration" + cliutil.ColorReset}
	toolGroup := &cobra.Group{ID: "tool", Title: cliutil.ColorYellowBold + "Additional Tools" + cliutil.ColorReset}

	cmd.AddGroup(coreGroup, mgmtGroup, toolGroup)

	for _, c := range cmd.Commands() {
		// Skip internal commands
		if c.Name() == "help" || c.Name() == "completion" || c.Name() == "version" {
			continue
		}

		switch c.Name() {
		case "clone", "status", "fetch", "pull", "push", "switch", "commit", "update", "diff", "sync", "clean",
			"branch", "stash", "tag", "worktree":
			c.GroupID = coreGroup.ID
		case "workspace", "config", "forge", "schema", "cleanup", "doctor":
			c.GroupID = mgmtGroup.ID
		default:
			c.GroupID = toolGroup.ID
		}
	}
}

func applyUsageTemplateRecursive(cmd *cobra.Command, tmpl string) {
	cmd.SetUsageTemplate(tmpl)
	// Cobra does not propagate SilenceUsage/SilenceErrors to child commands.
	// Set on every command so runtime errors never print usage text.
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	for _, c := range cmd.Commands() {
		applyUsageTemplateRecursive(c, tmpl)
	}
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "quiet output (errors only)")
	rootCmd.PersistentFlags().StringVar(&profileOverride, "profile", "", "override active profile (e.g., --profile work)")

	// Local flags for root command
	rootCmd.Flags().StringVar(&rootFormat, "format", "", "output format for help (supported: llm)")

	// Version template
	rootCmd.SetVersionTemplate(`{{with .Name}}{{printf "%s " .}}{{end}}{{printf "version %s" .Version}}
`)

	// The usage template is applied in Execute() via buildUsageTemplate() so it
	// reflects the resolved color state (see the color gate in pkg/cliutil).
}

// buildUsageTemplate assembles the colored usage template. It is a function
// (not a const) because the colors are runtime vars that the color gate blanks
// when output is not a terminal; building at call time captures the resolved
// color values instead of freezing them at compile time.
func buildUsageTemplate() string {
	return `{{if .Runnable}}` + cliutil.ColorGreenBold + `Usage:` + cliutil.ColorReset + `
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}` + cliutil.ColorGreenBold + `Usage:` + cliutil.ColorReset + `
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

` + cliutil.ColorGreenBold + `Examples:` + cliutil.ColorReset + `
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

` + cliutil.ColorMagentaBold + `Additional Commands:` + cliutil.ColorReset + `{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

` + cliutil.ColorGreenBold + `Flags:` + cliutil.ColorReset + `
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

` + cliutil.ColorGreenBold + `Global Flags:` + cliutil.ColorReset + `
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
}
