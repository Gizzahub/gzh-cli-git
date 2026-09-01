// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"context"
	"fmt"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
)

const familybookEntPrepareV1 = "familybook-ent-v1"

// controllerBinding is proof that an explicitly-selected controller file
// selected this exact repository. Keeping path and bytes digest makes a
// check-to-run controller change fail closed.
type controllerBinding struct {
	Path, Digest, Workspace, Remote, RemoteURL string
	Integration, TaskPattern                   []string
	PrepareProfile                             string
}

func resolveController(ctx context.Context, g gitRepo, path, branch string) (*controllerBinding, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("controller config path is empty")
	}
	doc, err := config.LoadControllerConfig(path)
	if err != nil {
		return nil, err
	}
	remote, err := detectRemote(ctx, g, branch)
	if err != nil {
		return nil, err
	}
	actual, err := pushEndpoint(ctx, g, remote)
	if err != nil {
		return nil, fmt.Errorf("resolve controller push endpoint: %w", err)
	}
	fetchRaw, err := g.output(ctx, "remote", "get-url", remote)
	if err != nil {
		return nil, fmt.Errorf("resolve controller fetch endpoint: %w", err)
	}
	fetchEndpoint, err := canonicalPushEndpoint(strings.TrimSpace(fetchRaw))
	if err != nil {
		return nil, fmt.Errorf("canonical controller fetch endpoint: %w", err)
	}
	if fetchEndpoint != actual {
		return nil, fmt.Errorf("controller remote %q has different fetch and push endpoints", remote)
	}
	var matched string
	var ws *config.Workspace
	for name, candidate := range doc.Workspaces {
		if candidate == nil || strings.TrimSpace(candidate.URL) == "" {
			continue
		}
		want, canonErr := canonicalPushEndpoint(strings.TrimSpace(candidate.URL))
		if canonErr != nil {
			return nil, fmt.Errorf("controller workspace %q URL: %w", name, canonErr)
		}
		if want == actual {
			if ws != nil {
				return nil, fmt.Errorf("controller config has ambiguous workspace entries for %s", actual)
			}
			matched, ws = name, candidate
		}
	}
	if ws == nil {
		return nil, fmt.Errorf("controller config has no workspace matching remote %s", actual)
	}
	if ws.Access == config.WorkspaceAccessReadOnly {
		return nil, fmt.Errorf("controller workspace %q is read-only", matched)
	}
	if ws.Integration == nil {
		return nil, fmt.Errorf("controller workspace %q has no integration policy", matched)
	}
	b := &controllerBinding{Path: doc.Path, Digest: doc.Digest, Workspace: matched, Remote: remote, RemoteURL: actual, Integration: append([]string(nil), ws.Branch.IntegrationBranch...), TaskPattern: append([]string(nil), ws.Branch.TaskPattern...), PrepareProfile: ws.Integration.PrepareProfile}
	return b, nil
}

func revalidateController(ctx context.Context, g gitRepo, check *CheckReport) error {
	if check.Controller == nil {
		return nil
	}
	b, err := resolveController(ctx, g, check.Controller.Path, check.Plan.Branch)
	if err != nil {
		return err
	}
	if b.Path != check.Controller.Path || b.Digest != check.Controller.Digest || b.Workspace != check.Controller.Workspace || b.Remote != check.Controller.Remote || b.RemoteURL != check.Controller.RemoteURL || b.PrepareProfile != check.Controller.PrepareProfile || strings.Join(b.Integration, "\x00") != strings.Join(check.Controller.Integration, "\x00") || strings.Join(b.TaskPattern, "\x00") != strings.Join(check.Controller.TaskPattern, "\x00") {
		return fmt.Errorf("controller config changed during readiness; re-run check")
	}
	return nil
}
