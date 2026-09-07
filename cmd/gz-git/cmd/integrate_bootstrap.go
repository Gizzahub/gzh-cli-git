package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/integrate"
)

var (
	bootstrapPlanBranch, bootstrapPlanTarget, bootstrapPlanIssuer, bootstrapPlanOutput string
	bootstrapPlanExpiry                                                                time.Duration
	bootstrapApplyFile                                                                 string
	bootstrapApplyConfirm                                                              string
)

var (
	integrateBootstrapCmd     = &cobra.Command{Use: "bootstrap", Short: "Bootstrap a target-owned readiness contract"}
	integrateBootstrapPlanCmd = &cobra.Command{
		Use: "plan", Short: "Create a read-only, expiring bootstrap plan", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := integrate.BootstrapPlanFor(cmdContext(cmd), gitcmd.NewExecutor(), integrate.BootstrapOptions{RepoPath: ".", Branch: bootstrapPlanBranch, Target: bootstrapPlanTarget, Issuer: bootstrapPlanIssuer, Expiry: bootstrapPlanExpiry})
			if err != nil {
				return cliutil.NewExitError(2, err)
			}
			if err := integrate.WriteBootstrapPlan(bootstrapPlanOutput, p); err != nil {
				return cliutil.NewExitError(2, err)
			}
			digest := integrate.BootstrapPlanDigest(p)
			if digest == "" {
				return cliutil.NewExitError(2, fmt.Errorf("encode bootstrap plan digest"))
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "CONFIRM_DIGEST %s\n", digest)
			return nil
		},
	}
)

var integrateBootstrapApplyCmd = &cobra.Command{
	Use: "apply", Short: "Apply an exact bootstrap plan with a lease", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if bootstrapApplyFile == "" {
			return cliutil.NewExitError(2, fmt.Errorf("--plan is required"))
		}
		p, err := integrate.ReadBootstrapPlan(bootstrapApplyFile)
		if err != nil {
			return cliutil.NewExitError(2, err)
		}
		if err := integrate.BootstrapApply(cmdContext(cmd), gitcmd.NewExecutor(), p, ".", bootstrapApplyConfirm); err != nil {
			return cliutil.NewExitError(1, err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "BOOTSTRAPPED", p.TargetRef, p.SourceSHA)
		return nil
	},
}

func init() {
	integrateCmd.AddCommand(integrateBootstrapCmd)
	integrateBootstrapCmd.AddCommand(integrateBootstrapPlanCmd, integrateBootstrapApplyCmd)
	integrateBootstrapPlanCmd.Flags().StringVar(&bootstrapPlanBranch, "branch", "", "source branch")
	integrateBootstrapPlanCmd.Flags().StringVar(&bootstrapPlanTarget, "target", "", "target ref")
	integrateBootstrapPlanCmd.Flags().StringVar(&bootstrapPlanIssuer, "issuer", "", "human or automation identity recorded in the plan")
	integrateBootstrapPlanCmd.Flags().DurationVar(&bootstrapPlanExpiry, "expires-in", 15*time.Minute, "plan lifetime")
	integrateBootstrapPlanCmd.Flags().StringVarP(&bootstrapPlanOutput, "output", "o", "", "write plan JSON to this file (stdout when omitted)")
	integrateBootstrapApplyCmd.Flags().StringVar(&bootstrapApplyFile, "plan", "", "plan JSON file")
	integrateBootstrapApplyCmd.Flags().StringVar(&bootstrapApplyConfirm, "confirm", "", "explicit human confirmation: canonical plan digest")
}
