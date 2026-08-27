// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// WorkspacePushAccess resolves the nearest configured workspace declaration
// for repoPath. It walks through every ancestor config rather than stopping at
// a repository's own config: a parent workspace owns the access decision for a
// third-party child checkout.
func WorkspacePushAccess(repoPath string) (WorkspaceAccess, string, error) {
	return WorkspacePushAccessContext(context.Background(), repoPath)
}

// WorkspacePushAccessContext is WorkspacePushAccess with caller cancellation
// for the repository-local Git metadata query.
func WorkspacePushAccessContext(ctx context.Context, repoPath string) (WorkspaceAccess, string, error) {
	if access, found, err := workspaceAccessMarker(ctx, repoPath); err != nil || found {
		return access, filepath.Join(repoPath, ".git", "config"), err
	}

	repoAbs, err := canonicalWorkspacePath(repoPath)
	if err != nil {
		return "", "", err
	}

	// Preserve the lexical path for config discovery. A workspace may expose an
	// external checkout through a symlink whose owning config is not an ancestor
	// of the resolved physical path.
	start, err := filepath.Abs(repoPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve workspace path %s: %w", repoPath, err)
	}
	var resolvedAccess WorkspaceAccess
	var resolvedSource string
	for {
		configDir, configPath, findErr := findConfigUpward(start, projectConfigNames())
		if findErr != nil {
			return "", "", findErr
		}
		if configPath == "" {
			return resolvedAccess, resolvedSource, nil
		}
		sourcePath, sourceErr := canonicalWorkspacePath(configPath)
		if sourceErr != nil {
			return "", "", sourceErr
		}

		access, found, loadErr := workspaceAccessInFile(configPath, repoAbs)
		if loadErr != nil {
			return "", sourcePath, loadErr
		}
		if found {
			// A read-only declaration is a hard boundary. Continue past a
			// read-write match so an outer owner can still forbid writes.
			if access.IsReadOnly() {
				return access, sourcePath, nil
			}
			resolvedAccess, resolvedSource = access, sourcePath
		}

		parent := filepath.Dir(configDir)
		if parent == configDir {
			return "", "", nil
		}
		start = parent
	}
}

func workspaceAccessInFile(configPath, repoAbs string) (WorkspaceAccess, bool, error) {
	return workspaceAccessInFileVisited(configPath, repoAbs, map[string]bool{})
}

func workspaceAccessInFileVisited(configPath, repoAbs string, visited map[string]bool) (WorkspaceAccess, bool, error) {
	canonicalConfig, err := canonicalWorkspacePath(configPath)
	if err != nil {
		return "", false, err
	}
	if visited[canonicalConfig] {
		return "", false, fmt.Errorf("workspace access config parent cycle at %s", canonicalConfig)
	}
	visited[canonicalConfig] = true

	// configPath comes only from bounded upward discovery of fixed project
	// config names, never directly from configuration content.
	// #nosec G304 -- configPath is a discovered fixed-name local project config.
	data, err := os.ReadFile(canonicalConfig)
	if err != nil {
		return "", false, fmt.Errorf("read workspace access config %s: %w", configPath, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return "", false, fmt.Errorf("parse workspace access config %s: %w", configPath, err)
	}

	access, found, err := findWorkspaceAccess(filepath.Dir(canonicalConfig), cfg.Workspaces, repoAbs)
	if err != nil || (found && access.IsReadOnly()) {
		return access, found, err
	}
	if cfg.Parent != "" {
		parentPath := resolveWorkspaceAccessPath(filepath.Dir(canonicalConfig), cfg.Parent)
		parentAccess, parentFound, parentErr := workspaceAccessInFileVisited(parentPath, repoAbs, visited)
		if parentErr != nil || parentFound {
			return parentAccess, parentFound, parentErr
		}
	}
	return access, found, nil
}

func findWorkspaceAccess(base string, workspaces map[string]*Workspace, repoAbs string) (WorkspaceAccess, bool, error) {
	var resolved WorkspaceAccess
	var foundMatch bool
	for name, ws := range workspaces {
		if ws == nil {
			continue
		}

		rel := ws.Path
		if rel == "" {
			rel = name
		}
		declared, err := canonicalWorkspacePath(resolveWorkspaceAccessPath(base, rel))
		if err != nil {
			return "", false, err
		}
		if pathContains(declared, repoAbs) {
			if !ws.Access.IsValid() {
				return "", false, fmt.Errorf("workspace %q has invalid access %q", name, ws.Access)
			}
			if ws.Access.IsReadOnly() {
				return ws.Access, true, nil
			}
			if access, found, err := findWorkspaceAccess(declared, ws.Workspaces, repoAbs); err != nil || found {
				if err != nil || access.IsReadOnly() {
					return access, found, err
				}
				resolved, foundMatch = access, found
			}
			if ws.Access != "" {
				resolved, foundMatch = ws.Access, true
			}
		}
	}
	return resolved, foundMatch, nil
}

// RecordWorkspaceReadOnly stores the resolved policy in repository-local Git
// metadata. This preserves an external absolute workspace's contract when a
// later push is invoked from outside the owning config tree.
func RecordWorkspaceReadOnly(ctx context.Context, repoPath string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "config", "--local", "gz-git.workspaceAccess", string(WorkspaceAccessReadOnly)) // #nosec G204 -- fixed git config write for a selected repository.
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("record read-only workspace access for %s: %s: %w", repoPath, strings.TrimSpace(string(output)), err)
	}
	return nil
}

// ClearWorkspaceAccessMarker removes a previously persisted read-only contract
// after the owning workspace has successfully synced as read-write.
func ClearWorkspaceAccessMarker(ctx context.Context, repoPath string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "config", "--local", "--unset-all", "gz-git.workspaceAccess") // #nosec G204 -- fixed git config update for a selected repository.
	if output, err := cmd.CombinedOutput(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && (exitErr.ExitCode() == 1 || exitErr.ExitCode() == 5) {
			// Git uses 5 when --unset-all found no matching key in some
			// versions/config layouts; the desired postcondition already holds.
			return nil
		}
		return fmt.Errorf("clear workspace access marker for %s: %s: %w", repoPath, strings.TrimSpace(string(output)), err)
	}
	return nil
}

func workspaceAccessMarker(ctx context.Context, repoPath string) (WorkspaceAccess, bool, error) {
	probe := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--git-dir") // #nosec G204 -- fixed git query in the selected repository.
	if err := probe.Run(); err != nil {
		return "", false, nil //nolint:nilerr // a non-repository path can still inherit an ancestor workspace config
	}
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "config", "--local", "--get", "gz-git.workspaceAccess") // #nosec G204 -- fixed git config query in the selected repository.
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read repository workspace access marker for %s: %w", repoPath, err)
	}
	access := WorkspaceAccess(strings.TrimSpace(string(output)))
	if access == WorkspaceAccessReadWrite {
		// Only a deny marker is authoritative; allow decisions must still be
		// checked against every owning ancestor config.
		return "", false, nil
	}
	if access != WorkspaceAccessReadOnly {
		return "", false, fmt.Errorf("repository %s has invalid gz-git.workspaceAccess %q", repoPath, access)
	}
	return access, true, nil
}

func pathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func resolveWorkspaceAccessPath(base, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if len(path) >= 2 && path[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return filepath.Join(base, path)
}

func canonicalWorkspacePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve workspace path %s: %w", path, err)
	}
	if resolved, evalErr := filepath.EvalSymlinks(abs); evalErr == nil {
		return filepath.Clean(resolved), nil
	}
	return filepath.Clean(abs), nil
}
