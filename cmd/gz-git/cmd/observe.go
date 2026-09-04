package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/contextref"
)

var (
	observeCEPath   string
	observeCEDigest string
	observeTimeout  time.Duration
)

func newObserveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "observe [dir]",
		Short: "Observe tracked context-reference and CE gate-doctor state",
		Long: cliutil.QuickStartHelp(`  # Observe the current repository
  gz-git observe

  # Observe a repository path
  gz-git observe /path/to/repo

  # Aggregate CE v2 doctor after handshake
  gz-git observe --ce /usr/local/bin/ce --ce-digest sha256:<hex>`) + `

 Exit Codes:
  0  observation succeeded (read JSON for findings)
  1  gz-git transport or CE invocation fault

 This command never exits 2 or 3. It does not install, apply, or mutate
 Git config, the index, worktree files, modes, or hooks.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runObserve,
	}
	cmd.Flags().StringVar(&observeCEPath, "ce", "", "absolute path to a trusted CE executable")
	cmd.Flags().StringVar(&observeCEDigest, "ce-digest", "", "expected sha256:<hex> digest of --ce")
	cmd.Flags().DurationVar(&observeTimeout, "timeout", 10*time.Second, "CE subprocess timeout (max 30s)")
	return cmd
}

func runObserve(cmd *cobra.Command, args []string) error {
	opts := contextref.Options{Timeout: observeTimeout}
	if len(args) == 1 {
		opts.Dir = args[0]
	}
	if observeCEPath != "" {
		opts.CE = &contextref.CEDescriptor{
			Path:   observeCEPath,
			Digest: observeCEDigest,
		}
	}
	obs, err := contextref.Observe(cmdContext(cmd), opts)
	if err != nil {
		return cliutil.NewExitError(cliutil.ExitToolError, err)
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetEscapeHTML(false)
	if err := enc.Encode(obs); err != nil {
		return cliutil.NewExitError(cliutil.ExitToolError, fmt.Errorf("encode observation: %w", err))
	}
	if obs.ExitCode != cliutil.ExitOK {
		return cliutil.NewExitError(obs.ExitCode, fmt.Errorf("observation fault"))
	}
	return nil
}

func init() {
	rootCmd.AddCommand(newObserveCommand())
}
