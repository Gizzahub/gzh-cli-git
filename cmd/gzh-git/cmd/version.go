package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	gzhcligit "github.com/gizzahub/gzh-cli-git"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Long: `Display the version of gzh-git CLI tool.

Shows the current version number, git commit SHA, and build date.`,
	Run: func(cmd *cobra.Command, args []string) {
		short, _ := cmd.Flags().GetBool("short")

		if short {
			fmt.Println(gzhcligit.ShortVersion())
			return
		}

		fmt.Println(gzhcligit.VersionString())
		fmt.Printf("\nGo version: %s\n", gzhcligit.VersionInfo()["goVersion"])
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)

	versionCmd.Flags().BoolP("short", "s", false, "Print only the version number")
}
