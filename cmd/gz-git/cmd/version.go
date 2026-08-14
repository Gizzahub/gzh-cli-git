package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	gzhcligitforge "github.com/gizzahub/gzh-cli-gitforge"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
)

// versionCmd represents the version command.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Long: cliutil.QuickStartHelp(`  # Show full version info
  gz-git version

  # Show short version number
  gz-git version --short`),
	Run: func(cmd *cobra.Command, args []string) {
		short, _ := cmd.Flags().GetBool("short")

		if short {
			fmt.Println(gzhcligitforge.ShortVersion())
			return
		}

		fmt.Println(gzhcligitforge.VersionString())
		fmt.Printf("\nGo version: %s\n", gzhcligitforge.VersionInfo()["goVersion"])
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)

	versionCmd.Flags().BoolP("short", "s", false, "Print only the version number")
}
