// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// resolvePushPolicy assembles the policy every push runs under.
//
// A policy is always returned, even with no configuration present, because the
// lease-only default is what makes "--force" and a "+" refspec behave the same
// way. Without it a "+" refspec would quietly bypass --force-with-lease.
func resolvePushPolicy(effective *config.EffectiveConfig, forceMode string) (*repository.PushPolicy, error) {
	policy := &repository.PushPolicy{}
	if effective != nil && effective.Push.Policy != nil {
		*policy = *effective.Push.Policy
	}

	if forceMode != "" {
		mode, err := repository.ValidateForceMode(forceMode)
		if err != nil {
			return nil, err
		}
		policy.ForceMode = mode
	}

	if _, err := repository.ValidateForceMode(string(policy.ForceMode)); err != nil {
		return nil, err
	}

	return policy, nil
}

// addForceModeFlag registers the flag that overrides the configured force mode
// for one invocation.
func addForceModeFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVar(target, "force-mode", "",
		"how force pushes are treated: lease-only (default), allow, deny")
}
