// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
	"gopkg.in/yaml.v3"
)

const factNoDeclaration = "no declaration"

// TaskPatternDecl is the repo-root taskPattern load result.
//
// Missing file → empty Patterns plus a reportable "no declaration" fact.
// That is not "everything is reclaimable".
type TaskPatternDecl struct {
	Patterns          []string
	IntegrationBranch BranchList
	Source            string
	Facts             []string
}

// LoadRepoRootTaskPattern stats only <repoRoot>/.gz-git.{yaml,yml,json}.
// It does not call findConfigUpward and is not the 5-layer merger.
//
// A non-root .gz-git.yaml that declares taskPattern is ignored and its
// path is reported. A pattern that equals a literal protected name
// (main, master, develop, development) rejects the load. Overlap with
// built-in protect patterns hotfix/* and release/* is allowed.
func LoadRepoRootTaskPattern(repoRoot string) (TaskPatternDecl, error) {
	var decl TaskPatternDecl
	if strings.TrimSpace(repoRoot) == "" {
		return decl, fmt.Errorf("repo root is empty")
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return decl, fmt.Errorf("resolve repo root: %w", err)
	}

	if err := reportNonRootTaskPattern(root, &decl); err != nil {
		return decl, err
	}

	path, err := statRepoRootConfig(root)
	if err != nil {
		return decl, err
	}
	if path == "" {
		decl.Facts = append(decl.Facts, factNoDeclaration)
		return decl, nil
	}

	patterns, integration, err := readRootBranchDecl(path)
	if err != nil {
		return decl, err
	}
	if err := rejectLiteralProtected(patterns, path); err != nil {
		return decl, err
	}

	decl.Patterns = append([]string(nil), patterns...)
	decl.IntegrationBranch = append(BranchList(nil), integration...)
	decl.Source = path
	if len(decl.Patterns) == 0 {
		decl.Facts = append(decl.Facts, factNoDeclaration)
	}
	return decl, nil
}

// MatchTaskPattern reports whether name matches pattern using a trailing *
// prefix compare, the same rule as repository.matchBranchPattern.
func MatchTaskPattern(name, pattern string) bool {
	if pattern == name {
		return true
	}
	if pattern != "" && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(name) >= len(prefix) && name[:len(prefix)] == prefix
	}
	return false
}

// MatchesAnyTaskPattern reports whether name matches any declared pattern.
func MatchesAnyTaskPattern(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if MatchTaskPattern(name, pattern) {
			return true
		}
	}
	return false
}

func statRepoRootConfig(root string) (string, error) {
	for _, ext := range []string{".yaml", ".yml", ".json"} {
		path := filepath.Join(root, ProjectConfigFileName+ext)
		st, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", fmt.Errorf("stat %s: %w", path, err)
		}
		if st.IsDir() {
			continue
		}
		return path, nil
	}
	return "", nil
}

func readRootBranchDecl(path string) (patterns, integration []string, err error) {
	data, err := os.ReadFile(path) //nolint:gosec // caller-supplied repo-root config path
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".json" {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if b, ok := raw["branch"]; ok {
			patterns, integration = decodeJSONBranchFields(b)
		}
		return patterns, integration, nil
	}

	var file struct {
		Branch *BranchConfig `yaml:"branch"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if file.Branch != nil {
		patterns = append([]string(nil), file.Branch.TaskPattern...)
		integration = append([]string(nil), file.Branch.IntegrationBranch...)
	}
	return patterns, integration, nil
}

func decodeJSONBranchFields(raw json.RawMessage) (patterns, integration []string) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, nil
	}
	return coerceStringList(m["taskPattern"]), coerceStringList(m["integrationBranch"])
}

func coerceStringList(v any) []string {
	switch t := v.(type) {
	case string:
		t = strings.TrimSpace(t)
		if t == "" {
			return nil
		}
		parts := strings.Split(t, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func rejectLiteralProtected(patterns []string, path string) error {
	literals := literalProtectedNames()
	for _, pattern := range patterns {
		for _, lit := range literals {
			if pattern == lit {
				return fmt.Errorf("%s: taskPattern %q equals protected name %q", path, pattern, lit)
			}
		}
	}
	return nil
}

func literalProtectedNames() []string {
	var out []string
	for _, p := range repository.ProtectedBranches {
		if p != "" && !strings.Contains(p, "*") {
			out = append(out, p)
		}
	}
	return out
}

func reportNonRootTaskPattern(root string, decl *TaskPatternDecl) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		base := d.Name()
		if base != ProjectConfigFileName+".yaml" &&
			base != ProjectConfigFileName+".yml" &&
			base != ProjectConfigFileName+".json" {
			return nil
		}
		if filepath.Dir(path) == root {
			return nil
		}
		has, err := fileDeclaresTaskPattern(path)
		if err != nil {
			return err
		}
		if has {
			decl.Facts = append(decl.Facts, "ignored non-root taskPattern: "+path)
		}
		return nil
	})
}

func fileDeclaresTaskPattern(path string) (bool, error) {
	patterns, _, err := readRootBranchDecl(path)
	if err != nil {
		// Unreadable nested files are reported, not applied.
		return false, nil
	}
	return len(patterns) > 0, nil
}
