// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/internal/safefs"
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
	fsRoot, err := safefs.OpenRoot(root)
	if err != nil {
		return decl, fmt.Errorf("open repo root: %w", err)
	}
	defer func() { _ = fsRoot.Close() }()

	if err := reportNonRootTaskPattern(fsRoot, root, &decl); err != nil {
		return decl, err
	}

	path, err := statRepoRootConfig(fsRoot, root)
	if err != nil {
		return decl, err
	}
	if path == "" {
		decl.Facts = append(decl.Facts, factNoDeclaration)
		return decl, nil
	}

	patterns, integration, err := readRootBranchDecl(fsRoot, filepath.Base(path))
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

// MatchTaskPattern reports whether name is in the namespace declared by
// pattern. The prefix is everything before the first '*'. That is how
// devenv's `dev/*/*/*` matches `dev/a/b/c` (DECISION-004). A trailing-*
// prefix compare on the whole pattern would treat the middle stars as
// literal characters and miss every real task branch.
//
// Pattern "*" (empty prefix) never matches here; load rejects it.
func MatchTaskPattern(name, pattern string) bool {
	if pattern == name {
		return true
	}
	star := strings.IndexByte(pattern, '*')
	if star <= 0 {
		return false
	}
	prefix := pattern[:star]
	return strings.HasPrefix(name, prefix)
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

func statRepoRootConfig(root *safefs.Root, rootPath string) (string, error) {
	for _, ext := range []string{".yaml", ".yml", ".json"} {
		name := ProjectConfigFileName + ext
		st, err := root.Stat(name)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", fmt.Errorf("stat %s: %w", filepath.Join(rootPath, name), err)
		}
		if st.IsDir() {
			continue
		}
		return filepath.Join(rootPath, name), nil
	}
	return "", nil
}

func readRootBranchDecl(root *safefs.Root, path string) (patterns, integration []string, err error) {
	data, err := root.ReadFile(path)
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
		if i := strings.IndexByte(pattern, '*'); i == 0 {
			return fmt.Errorf("%s: taskPattern %q matches every name", path, pattern)
		}
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

func reportNonRootTaskPattern(root *safefs.Root, rootPath string, decl *TaskPatternDecl) error {
	return reportNonRootTaskPatternAt(root, rootPath, ".", decl)
}

func reportNonRootTaskPatternAt(root *safefs.Root, rootPath, rel string, decl *TaskPatternDecl) error {
	entries, err := root.ReadDir(rel)
	if err != nil {
		return fmt.Errorf("scan %s: %w", filepath.Join(rootPath, rel), err)
	}

	for _, entry := range entries {
		name := entry.Name()
		entryRel := filepath.Join(rel, name)
		entryPath := filepath.Join(rootPath, entryRel)
		if entry.IsDir() {
			if name == ".git" || name == "vendor" || name == "node_modules" {
				continue
			}
			if err := reportNonRootTaskPatternAt(root, rootPath, entryRel, decl); err != nil {
				return err
			}
			continue
		}

		if name != ProjectConfigFileName+".yaml" &&
			name != ProjectConfigFileName+".yml" &&
			name != ProjectConfigFileName+".json" {
			continue
		}
		if filepath.Dir(entryPath) == rootPath {
			continue
		}
		if patterns, _, err := readRootBranchDecl(root, entryRel); err == nil && len(patterns) > 0 {
			decl.Facts = append(decl.Facts, "ignored non-root taskPattern: "+entryPath)
		}
	}

	return nil
}
