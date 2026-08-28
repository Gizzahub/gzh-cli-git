// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// Readiness is the target-owned integration gate contract. It deliberately has
// no command, argument, environment, or cwd fields: V1 has one fixed runner
// invocation and accepts no caller-controlled execution surface.
type Readiness struct {
	Version int    `yaml:"version" json:"version"`
	Runner  string `yaml:"runner" json:"runner"`
}

// CanonicalReadiness is a stable, strict representation used to compare the
// target declaration to the source declaration before executing a runner.
func CanonicalReadiness(r Readiness) (string, error) {
	if err := ValidateReadiness(r); err != nil {
		return "", err
	}
	b, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("canonicalize readiness: %w", err)
	}
	return string(b), nil
}

// ReadinessDigest returns the SHA-256 digest of the canonical contract.
func ReadinessDigest(r Readiness) (string, error) {
	canonical, err := CanonicalReadiness(r)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:]), nil
}

// ValidateReadiness validates the closed V1 contract.
func ValidateReadiness(r Readiness) error {
	if r.Version != 1 {
		return fmt.Errorf("branch.readiness.version must be 1")
	}
	if err := validateReadinessRunner(r.Runner); err != nil {
		return err
	}
	return nil
}

func validateReadinessRunner(runner string) error {
	if runner == "" || strings.TrimSpace(runner) != runner {
		return fmt.Errorf("branch.readiness.runner is required")
	}
	if path.IsAbs(runner) || strings.ContainsRune(runner, '\\') {
		return fmt.Errorf("branch.readiness.runner must be a repository-relative path")
	}
	for _, r := range runner {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("branch.readiness.runner contains control characters")
		}
	}
	clean := path.Clean(runner)
	if clean != runner || clean == "." || strings.HasPrefix(clean, "../") || clean == ".." {
		return fmt.Errorf("branch.readiness.runner must not contain traversal")
	}
	if !strings.HasPrefix(clean, ".gz-git/readiness/") {
		return fmt.Errorf("branch.readiness.runner must be within .gz-git/readiness/")
	}
	return nil
}

// ParseReadinessDocument extracts branch.readiness from a repo-root config.
// Unknown fields in the readiness node are rejected even though the larger
// project configuration remains backwards-compatible and extensible.
func ParseReadinessDocument(data []byte, isJSON bool) (Readiness, bool, error) {
	if isJSON {
		return parseReadinessJSON(data)
	}
	return parseReadinessYAML(data)
}

func parseReadinessJSON(data []byte) (Readiness, bool, error) {
	root, rootCounts, err := parseJSONObjectCounts(data)
	if err != nil {
		return Readiness{}, false, fmt.Errorf("parse config: %w", err)
	}
	if rootCounts["branch"] > 1 {
		return Readiness{}, false, fmt.Errorf("duplicate JSON key %q", "branch")
	}
	branch, ok := root["branch"]
	if !ok {
		return Readiness{}, false, nil
	}
	if trimmed := bytes.TrimSpace(branch); len(trimmed) == 0 || trimmed[0] != '{' {
		return Readiness{}, false, nil
	}
	bm, branchCounts, err := parseJSONObjectCounts(branch)
	if err != nil {
		return Readiness{}, false, fmt.Errorf("parse branch: %w", err)
	}
	if branchCounts["readiness"] > 1 {
		return Readiness{}, false, fmt.Errorf("duplicate JSON key %q", "readiness")
	}
	raw, ok := bm["readiness"]
	if !ok {
		return Readiness{}, false, nil
	}
	fields, err := parseJSONObjectUnique(raw)
	if err != nil {
		return Readiness{}, false, fmt.Errorf("branch.readiness must be an object")
	}
	if len(fields) != 2 || fields["version"] == nil || fields["runner"] == nil {
		return Readiness{}, false, fmt.Errorf("branch.readiness has unknown or missing fields")
	}
	var r Readiness
	if err := json.Unmarshal(raw, &r); err != nil {
		return Readiness{}, false, fmt.Errorf("parse branch.readiness: %w", err)
	}
	return r, true, ValidateReadiness(r)
}

func parseReadinessYAML(data []byte) (Readiness, bool, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return Readiness{}, false, fmt.Errorf("parse config: %w", err)
	}
	if len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return Readiness{}, false, nil
	}
	if mappingKeyCount(root.Content[0], "branch") > 1 {
		return Readiness{}, false, fmt.Errorf("duplicate YAML key %q", "branch")
	}
	branch := mappingValue(root.Content[0], "branch")
	if branch == nil || branch.Kind != yaml.MappingNode {
		return Readiness{}, false, nil
	}
	if mappingKeyCount(branch, "readiness") > 1 {
		return Readiness{}, false, fmt.Errorf("duplicate YAML key %q", "readiness")
	}
	node := mappingValue(branch, "readiness")
	if node == nil {
		return Readiness{}, false, nil
	}
	if node.Kind != yaml.MappingNode || len(node.Content) != 4 || mappingValue(node, "version") == nil || mappingValue(node, "runner") == nil {
		return Readiness{}, false, fmt.Errorf("branch.readiness has unknown or missing fields")
	}
	if err := rejectDuplicateYAMLKeys(node); err != nil {
		return Readiness{}, false, err
	}
	var r Readiness
	if err := node.Decode(&r); err != nil {
		return Readiness{}, false, fmt.Errorf("parse branch.readiness: %w", err)
	}
	return r, true, ValidateReadiness(r)
}

func parseJSONObjectUnique(data []byte) (map[string]json.RawMessage, error) {
	out, counts, err := parseJSONObjectCounts(data)
	if err != nil {
		return nil, err
	}
	for _, count := range counts {
		if count > 1 {
			return nil, fmt.Errorf("duplicate or invalid JSON key")
		}
	}
	return out, nil
}

func parseJSONObjectCounts(data []byte) (values map[string]json.RawMessage, counts map[string]int, err error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil, nil, fmt.Errorf("expected JSON object")
	}
	values = map[string]json.RawMessage{}
	counts = map[string]int{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, nil, fmt.Errorf("invalid JSON key")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, nil, err
		}
		values[key] = value
		counts[key]++
	}
	if _, err := dec.Token(); err != nil {
		return nil, nil, err
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return nil, nil, fmt.Errorf("trailing JSON data")
	}
	return values, counts, nil
}

func rejectDuplicateYAMLKeys(node *yaml.Node) error {
	seen := map[string]bool{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if seen[key] {
			return fmt.Errorf("duplicate YAML key %q", key)
		}
		seen[key] = true
	}
	return nil
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func mappingKeyCount(node *yaml.Node, key string) int {
	count := 0
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			count++
		}
	}
	return count
}
