// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
)

// schemaCmd represents the schema command.
var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Show configuration schema reference",
	Long: cliutil.QuickStartHelp(`  # View schema
  gz-git schema

  # Save default config template
  gz-git schema > .gz-git.yaml`),
	Example: ``,
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(config.ExampleConfig)
	},
}

func init() {
	rootCmd.AddCommand(schemaCmd)
}
