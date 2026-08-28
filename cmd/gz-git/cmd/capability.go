package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

const integrateReadinessV1Capability = "integrate-readiness-v1"

func newCapabilityCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "capability NAME",
		Short: "Probe a machine-readable CLI capability",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !supportsCapability(args[0]) {
				return fmt.Errorf("unsupported capability %q", args[0])
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), args[0]); err != nil {
				return fmt.Errorf("write capability result: %w", err)
			}
			return nil
		},
	}
}

func supportsCapability(name string) bool {
	switch name {
	case integrateReadinessV1Capability:
		return true
	default:
		return false
	}
}

func init() {
	rootCmd.AddCommand(newCapabilityCommand())
}
