// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ControllerConfig is an explicitly selected, non-inheriting devbox policy.
// It does not load parent configs or recursively discover workspaces.
type ControllerConfig struct {
	Path, Digest string
	Workspaces   map[string]*Workspace
}

// LoadControllerConfig loads one explicitly selected controller file without
// applying repository config discovery or inheritance.
func LoadControllerConfig(path string) (*ControllerConfig, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve controller config: %w", err)
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("resolve controller config identity: %w", err)
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("read controller config: %w", err)
	}
	var doc struct {
		Workspaces map[string]*Workspace `yaml:"workspaces"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse controller config: %w", err)
	}
	if len(doc.Workspaces) == 0 {
		return nil, fmt.Errorf("controller config has no workspaces")
	}
	for name, ws := range doc.Workspaces {
		if ws == nil {
			continue
		}
		if ws.Integration == nil {
			continue
		}
		if strings.TrimSpace(ws.URL) == "" {
			return nil, fmt.Errorf("controller workspace %q requires url", name)
		}
		if ws.Access != "" && ws.Access != WorkspaceAccessReadWrite && ws.Access != WorkspaceAccessReadOnly {
			return nil, fmt.Errorf("controller workspace %q has invalid access %q", name, ws.Access)
		}
		if ws.Integration.PrepareProfile != "" && ws.Integration.PrepareProfile != "familybook-ent-v1" {
			return nil, fmt.Errorf("controller workspace %q has unsupported preparation profile %q", name, ws.Integration.PrepareProfile)
		}
		if ws.Branch == nil || len(ws.Branch.IntegrationBranch) != 1 {
			return nil, fmt.Errorf("controller workspace %q requires exactly one branch.integrationBranch", name)
		}
		branch := strings.TrimSpace(ws.Branch.IntegrationBranch[0])
		if branch == "" {
			return nil, fmt.Errorf("controller workspace %q has empty integration branch", name)
		}
		ws.Branch.IntegrationBranch = BranchList{branch}
		if err := rejectLiteralProtected(ws.Branch.TaskPattern, name); err != nil {
			return nil, err
		}
	}
	return &ControllerConfig{Path: abs, Digest: fmt.Sprintf("%x", sha256.Sum256(raw)), Workspaces: doc.Workspaces}, nil
}
