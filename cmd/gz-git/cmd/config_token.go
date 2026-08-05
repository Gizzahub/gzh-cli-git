// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
)

var configTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage forge API tokens in the OS keychain",
	Long: cliutil.QuickStartHelp(`  # Store a GitHub token in the OS keychain
  gz-git config token set github ghp_...

  # Show token (masked by default; --show for full value)
  gz-git config token get github

  # Remove token
  gz-git config token delete github

Precedence for runtime token resolution:
  flag > env > keychain > config profile > defaults

On headless Linux without Secret Service, set/get/delete warn and fall back
gracefully so CI and doctor keep working.`),
}

var tokenShowFull bool

var configTokenSetCmd = &cobra.Command{
	Use:   "set <provider> <token>",
	Short: "Store a token in the OS keychain",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigTokenSet,
}

var configTokenGetCmd = &cobra.Command{
	Use:   "get <provider>",
	Short: "Read a token from the OS keychain",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigTokenGet,
}

var configTokenDeleteCmd = &cobra.Command{
	Use:   "delete <provider>",
	Short: "Delete a token from the OS keychain",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigTokenDelete,
}

func init() {
	configCmd.AddCommand(configTokenCmd)
	configTokenCmd.AddCommand(configTokenSetCmd)
	configTokenCmd.AddCommand(configTokenGetCmd)
	configTokenCmd.AddCommand(configTokenDeleteCmd)
	configTokenGetCmd.Flags().BoolVar(&tokenShowFull, "show", false, "print the full token (default: masked)")
}

func runConfigTokenSet(cmd *cobra.Command, args []string) error {
	provider, token := args[0], args[1]
	if err := config.DefaultTokenStore.Set(provider, token); err != nil {
		fmt.Fprintf(os.Stderr, "warning: keychain unavailable (%v); token not stored\n", err)
		fmt.Fprintf(os.Stderr, "hint: set %s or GZ_GIT_TOKEN in the environment for CI/headless use\n",
			tokenEnvHint(provider))
		return nil
	}
	fmt.Printf("Stored token for provider %q in OS keychain\n", strings.ToLower(provider))
	return nil
}

func runConfigTokenGet(cmd *cobra.Command, args []string) error {
	provider := args[0]
	tok, err := config.DefaultTokenStore.Get(provider)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: keychain unavailable (%v)\n", err)
		return nil
	}
	if tok == "" {
		fmt.Printf("No keychain token for provider %q\n", strings.ToLower(provider))
		return nil
	}
	if tokenShowFull {
		fmt.Println(tok)
		return nil
	}
	fmt.Println(sanitizeToken(tok))
	return nil
}

func runConfigTokenDelete(cmd *cobra.Command, args []string) error {
	provider := args[0]
	if err := config.DefaultTokenStore.Delete(provider); err != nil {
		fmt.Fprintf(os.Stderr, "warning: keychain unavailable (%v)\n", err)
		return nil
	}
	fmt.Printf("Deleted keychain token for provider %q\n", strings.ToLower(provider))
	return nil
}

func tokenEnvHint(provider string) string {
	switch strings.ToLower(provider) {
	case "github":
		return "GITHUB_TOKEN"
	case "gitlab":
		return "GITLAB_TOKEN"
	case "gitea":
		return "GITEA_TOKEN"
	default:
		return "GZ_GIT_TOKEN"
	}
}
