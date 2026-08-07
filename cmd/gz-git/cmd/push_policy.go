// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/identity"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// pushOverrides are the per-invocation flags that can loosen or tighten the
// configured policy. An empty string leaves the configured value alone.
type pushOverrides struct {
	forceMode   string
	foreignWork string
}

// resolvePushPolicy assembles the policy every push runs under.
//
// A policy is always returned, even with no configuration present, because the
// lease-only default is what makes "--force" and a "+" refspec behave the same
// way. Without it a "+" refspec would quietly bypass --force-with-lease.
func resolvePushPolicy(effective *config.EffectiveConfig, overrides pushOverrides) (*repository.PushPolicy, error) {
	policy := &repository.PushPolicy{}
	if effective != nil && effective.Push.Policy != nil {
		*policy = *effective.Push.Policy
	}

	if overrides.forceMode != "" {
		mode, err := repository.ValidateForceMode(overrides.forceMode)
		if err != nil {
			return nil, err
		}
		policy.ForceMode = mode
	}

	if _, err := repository.ValidateForceMode(string(policy.ForceMode)); err != nil {
		return nil, err
	}

	if overrides.foreignWork != "" {
		mode, err := repository.ValidateForeignWorkMode(overrides.foreignWork)
		if err != nil {
			return nil, err
		}
		policy.ForeignWork = mode
	}

	if _, err := repository.ValidateForeignWorkMode(string(policy.ForeignWork)); err != nil {
		return nil, err
	}

	return policy, nil
}

// pushGuards is the policy plus the identity it needs to apply the foreign-work
// rule. They always travel together onto a BulkPushOptions, so commands that
// hand a push off to a helper pass one value rather than two.
type pushGuards struct {
	policy   *repository.PushPolicy
	identity identity.Identity
}

// resolvePushGuards builds both from configuration and this invocation's flags.
func resolvePushGuards(effective *config.EffectiveConfig, overrides pushOverrides) (pushGuards, error) {
	policy, err := resolvePushPolicy(effective, overrides)
	if err != nil {
		return pushGuards{}, err
	}
	return pushGuards{policy: policy, identity: pushIdentity(effective)}, nil
}

// pushIdentity names the machine a push runs from, so the foreign-work check
// can tell this machine's commits from another's.
func pushIdentity(effective *config.EffectiveConfig) identity.Identity {
	if effective == nil {
		// Without a loaded config nothing has resolved the hostname yet, and an
		// unnamed machine skips the check rather than guessing at it.
		return identity.Resolve(nil)
	}
	return effective.Identity
}

// addForceModeFlag registers the flag that overrides the configured force mode
// for one invocation.
func addForceModeFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVar(target, "force-mode", "",
		"how force pushes are treated: lease-only (default), allow, deny")
}

// addForeignWorkFlag registers the flag that overrides what happens to a force
// push landing on another machine's commits.
func addForeignWorkFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVar(target, "foreign-work", "",
		"force push over commits from another device or agent: block (default), allow")
}
