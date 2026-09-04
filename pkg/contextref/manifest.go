// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package contextref

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"

	"gopkg.in/yaml.v3"
)

func parseManifestYAML(data []byte) ([]string, error) {
	if len(data) > maxManifestBytes {
		return nil, errLimit
	}
	var node yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&node); err != nil {
		return nil, fmt.Errorf("%w: %s", errHostileYAML, err.Error())
	}
	if err := rejectHostileYAML(&node); err != nil {
		return nil, err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errMultipleDocs
	}
	known := yaml.NewDecoder(bytes.NewReader(data))
	known.KnownFields(true)
	var doc manifestDocument
	if err := known.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%w: %s", errHostileYAML, err.Error())
	}
	if doc.Schema != ManifestSchema {
		return nil, fmt.Errorf("%w: schema", errHostileYAML)
	}
	seen := map[string]struct{}{}
	paths := make([]string, 0, len(doc.Context.Entrypoints))
	for _, ep := range doc.Context.Entrypoints {
		if err := validateEntrypointPath(ep.Path); err != nil {
			return nil, err
		}
		if _, dup := seen[ep.Path]; dup {
			return nil, fmt.Errorf("%w: %s", errHostileYAML, ReasonDuplicate)
		}
		seen[ep.Path] = struct{}{}
		paths = append(paths, ep.Path)
	}
	if len(paths) > maxEntrypoints {
		return nil, errLimit
	}
	sort.Strings(paths)
	return paths, nil
}

func rejectHostileYAML(n *yaml.Node) error {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.AliasNode || n.Anchor != "" {
		return errHostileYAML
	}
	if !knownYAMLTag(n.Kind, n.Tag) {
		return errHostileYAML
	}
	if n.Kind == yaml.MappingNode {
		if len(n.Content)%2 != 0 {
			return errHostileYAML
		}
		seen := map[string]struct{}{}
		for i := 0; i < len(n.Content); i += 2 {
			key := n.Content[i]
			if key.Kind != yaml.ScalarNode {
				return errHostileYAML
			}
			if _, dup := seen[key.Value]; dup {
				return errHostileYAML
			}
			seen[key.Value] = struct{}{}
			if err := rejectHostileYAML(key); err != nil {
				return err
			}
			if err := rejectHostileYAML(n.Content[i+1]); err != nil {
				return err
			}
		}
		return nil
	}
	for _, child := range n.Content {
		if err := rejectHostileYAML(child); err != nil {
			return err
		}
	}
	return nil
}

func knownYAMLTag(kind yaml.Kind, tag string) bool {
	switch kind {
	case yaml.DocumentNode:
		return true
	case yaml.MappingNode:
		return tag == "" || tag == "!!map" || tag == "tag:yaml.org,2002:map"
	case yaml.SequenceNode:
		return tag == "" || tag == "!!seq" || tag == "tag:yaml.org,2002:seq"
	case yaml.ScalarNode:
		return tag == "" || tag == "!!str" || tag == "tag:yaml.org,2002:str"
	case yaml.AliasNode:
		return false
	default:
		return false
	}
}
