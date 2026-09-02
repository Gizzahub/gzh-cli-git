// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
)

const (
	workflowIntegrationBranchKey = "workflow.integrationBranch"
	managedIntegrationBranchKey  = "gz-git.managedWorkflowIntegrationBranch"
)

// IntegrationParticipationAction describes the repository-local change needed
// to align workflow.integrationBranch with its repo-root declaration.
type IntegrationParticipationAction string

const (
	// IntegrationParticipationNoop leaves repository-local configuration unchanged.
	IntegrationParticipationNoop IntegrationParticipationAction = "noop"
	// IntegrationParticipationInstall records a newly managed declaration.
	IntegrationParticipationInstall IntegrationParticipationAction = "install"
	// IntegrationParticipationUpdate changes a previously managed declaration.
	IntegrationParticipationUpdate IntegrationParticipationAction = "update"
	// IntegrationParticipationRemove clears configuration proven to be managed.
	IntegrationParticipationRemove IntegrationParticipationAction = "remove"
	// IntegrationParticipationConflict preserves user-owned or mismatched state.
	IntegrationParticipationConflict IntegrationParticipationAction = "conflict"
)

// IntegrationParticipationState is the complete local state used to decide
// whether a sync may manage workflow.integrationBranch. Marker ownership is
// deliberately explicit so a workspace sync never overwrites a manual choice.
type IntegrationParticipationState struct {
	Current string
	Marker  string
	Desired string
}

type integrationParticipationTransition struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// PlanIntegrationParticipation applies the ownership state machine without
// touching Git configuration. Desired is a normalized bare branch name, or
// empty when the repo-root declaration no longer selects a branch.
func PlanIntegrationParticipation(state IntegrationParticipationState) IntegrationParticipationAction {
	if state.Desired != "" {
		switch {
		case state.Marker == "" && state.Current == "":
			return IntegrationParticipationInstall
		case state.Marker == "" && state.Current == state.Desired:
			// Existing matching configuration is user-owned until explicitly
			// marked by this reconciler; do not adopt it implicitly.
			return IntegrationParticipationNoop
		case state.Marker == "":
			return IntegrationParticipationConflict
		case state.Marker != state.Current:
			return IntegrationParticipationConflict
		case state.Current == state.Desired:
			return IntegrationParticipationNoop
		default:
			return IntegrationParticipationUpdate
		}
	}

	switch {
	case state.Marker == "":
		return IntegrationParticipationNoop
	case state.Marker != state.Current && state.Current != "":
		return IntegrationParticipationConflict
	default:
		// A marker with no current value is an orphan; a matching pair is a
		// formerly managed declaration. Both are safe to remove.
		return IntegrationParticipationRemove
	}
}

// ReconcileIntegrationParticipation reads only the selected repository's
// root declaration and local Git metadata. It returns an error before writing
// when the declaration is invalid, unresolved, or conflicts with user-owned
// configuration.
func ReconcileIntegrationParticipation(ctx context.Context, repoPath string) (IntegrationParticipationAction, error) {
	desired, err := resolveDeclaredIntegrationBranch(ctx, repoPath)
	if err != nil {
		return IntegrationParticipationNoop, err
	}

	state, err := readIntegrationParticipationState(ctx, repoPath)
	if err != nil {
		return IntegrationParticipationNoop, err
	}
	state.Desired = desired
	if transition, ok := parseIntegrationParticipationTransition(state.Marker); ok {
		return resumeIntegrationParticipationTransition(ctx, repoPath, state, transition)
	}
	action := PlanIntegrationParticipation(state)
	if action == IntegrationParticipationConflict {
		return action, fmt.Errorf("workflow integration participation conflict: current %q, managed marker %q, declared %q", state.Current, state.Marker, state.Desired)
	}
	if action == IntegrationParticipationNoop {
		return action, nil
	}

	switch action {
	case IntegrationParticipationInstall, IntegrationParticipationUpdate:
		if err := applyIntegrationParticipationTransition(ctx, repoPath, state.Current, desired); err != nil {
			return action, err
		}
	case IntegrationParticipationRemove:
		// Clear the managed setting first. If this fails, retain the marker so a
		// later successful sync can prove ownership before trying again.
		if state.Current != "" {
			if err := unsetLocalGitConfig(ctx, repoPath, workflowIntegrationBranchKey); err != nil {
				return action, err
			}
		}
		if err := unsetLocalGitConfig(ctx, repoPath, managedIntegrationBranchKey); err != nil {
			return action, err
		}
	case IntegrationParticipationNoop, IntegrationParticipationConflict:
		return action, nil
	}
	return action, nil
}

func applyIntegrationParticipationTransition(ctx context.Context, repoPath, from, to string) error {
	encoded, err := json.Marshal(integrationParticipationTransition{From: from, To: to})
	if err != nil {
		return fmt.Errorf("encode integration participation transition: %w", err)
	}
	if err := setLocalGitConfig(ctx, repoPath, managedIntegrationBranchKey, string(encoded)); err != nil {
		return err
	}
	if err := setLocalGitConfig(ctx, repoPath, workflowIntegrationBranchKey, to); err != nil {
		return err
	}
	return setLocalGitConfig(ctx, repoPath, managedIntegrationBranchKey, to)
}

func parseIntegrationParticipationTransition(marker string) (integrationParticipationTransition, bool) {
	var transition integrationParticipationTransition
	if !strings.HasPrefix(marker, "{") || json.Unmarshal([]byte(marker), &transition) != nil || transition.To == "" {
		return integrationParticipationTransition{}, false
	}
	return transition, true
}

func resumeIntegrationParticipationTransition(ctx context.Context, repoPath string, state IntegrationParticipationState, transition integrationParticipationTransition) (IntegrationParticipationAction, error) {
	if state.Desired != transition.To || (state.Current != transition.From && state.Current != transition.To) {
		return IntegrationParticipationConflict, fmt.Errorf("workflow integration participation transition conflict: current %q, pending %q -> %q, declared %q", state.Current, transition.From, transition.To, state.Desired)
	}
	action := IntegrationParticipationUpdate
	if transition.From == "" {
		action = IntegrationParticipationInstall
	}
	if state.Current == transition.From {
		if err := setLocalGitConfig(ctx, repoPath, workflowIntegrationBranchKey, transition.To); err != nil {
			return action, err
		}
	}
	if err := setLocalGitConfig(ctx, repoPath, managedIntegrationBranchKey, transition.To); err != nil {
		return action, err
	}
	return action, nil
}

func resolveDeclaredIntegrationBranch(ctx context.Context, repoPath string) (string, error) {
	decl, err := LoadRepoRootTaskPattern(repoPath)
	if err != nil {
		return "", fmt.Errorf("load repo-root integration declaration: %w", err)
	}
	if len(decl.IntegrationBranch) == 0 {
		return "", nil
	}
	for _, candidate := range decl.IntegrationBranch {
		candidate = strings.TrimSpace(candidate)
		name, err := normalizeDeclaredIntegrationBranch(ctx, repoPath, candidate)
		if err != nil {
			return "", err
		}
		if err := gitcmd.SanitizeBranchName(name); err != nil {
			return "", fmt.Errorf("invalid repo-root integrationBranch %q: %w", candidate, err)
		}
		if integrationBranchExists(ctx, repoPath, name) {
			return name, nil
		}
	}
	return "", fmt.Errorf("repo-root integrationBranch %v has no local or remote-tracking ref", []string(decl.IntegrationBranch))
}

func normalizeDeclaredIntegrationBranch(ctx context.Context, repoPath, candidate string) (string, error) {
	name := strings.TrimSpace(candidate)
	localRef := strings.HasPrefix(name, "refs/heads/")
	if localRef {
		return strings.TrimPrefix(name, "refs/heads/"), nil
	}
	name = strings.TrimPrefix(name, "refs/remotes/")
	output, err := exec.CommandContext(ctx, "git", "-C", repoPath, "remote").Output() // #nosec G204 -- fixed Git query in selected repository.
	if err != nil {
		return "", fmt.Errorf("list Git remotes for %s: %w", repoPath, err)
	}
	best := ""
	for _, remote := range strings.Fields(string(output)) {
		if strings.HasPrefix(name, remote+"/") && len(remote) > len(best) {
			best = remote
		}
	}
	if best != "" {
		name = strings.TrimPrefix(name, best+"/")
	}
	return name, nil
}

func integrationBranchExists(ctx context.Context, repoPath, branch string) bool {
	localRef := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch) // #nosec G204 -- branch was validated with SanitizeBranchName and is passed as argv.
	if err := localRef.Run(); err == nil {
		return true
	}
	output, err := exec.CommandContext(ctx, "git", "-C", repoPath, "remote").Output() // #nosec G204 -- fixed Git query in selected repository.
	if err != nil {
		return false
	}
	for _, remote := range strings.Fields(string(output)) {
		remoteRef := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--verify", "--quiet", "refs/remotes/"+remote+"/"+branch) // #nosec G204 -- remote comes from Git and validated branch is passed as argv.
		if err := remoteRef.Run(); err == nil {
			return true
		}
	}
	return false
}

func readIntegrationParticipationState(ctx context.Context, repoPath string) (IntegrationParticipationState, error) {
	current, err := readLocalGitConfig(ctx, repoPath, workflowIntegrationBranchKey)
	if err != nil {
		return IntegrationParticipationState{}, err
	}
	marker, err := readLocalGitConfig(ctx, repoPath, managedIntegrationBranchKey)
	if err != nil {
		return IntegrationParticipationState{}, err
	}
	return IntegrationParticipationState{Current: current, Marker: marker}, nil
}

func readLocalGitConfig(ctx context.Context, repoPath, key string) (string, error) {
	output, err := exec.CommandContext(ctx, "git", "-C", repoPath, "config", "--local", "--get", key).Output() // #nosec G204 -- fixed local config key in selected repository.
	if err == nil {
		return strings.TrimSpace(string(output)), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return "", nil
	}
	return "", fmt.Errorf("read local Git config %s for %s: %w", key, repoPath, err)
}

func setLocalGitConfig(ctx context.Context, repoPath, key, value string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "config", "--local", key, value) // #nosec G204 -- fixed local config key and validated branch value in selected repository.
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("set local Git config %s for %s: %s: %w", key, repoPath, strings.TrimSpace(string(output)), err)
	}
	return nil
}

func unsetLocalGitConfig(ctx context.Context, repoPath, key string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "config", "--local", "--unset-all", key) // #nosec G204 -- fixed local config key in selected repository.
	if output, err := cmd.CombinedOutput(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && (exitErr.ExitCode() == 1 || exitErr.ExitCode() == 5) {
			return nil
		}
		return fmt.Errorf("unset local Git config %s for %s: %s: %w", key, repoPath, strings.TrimSpace(string(output)), err)
	}
	return nil
}
