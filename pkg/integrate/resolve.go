// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"context"
	"fmt"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
)

const (
	// SourceNone means no integration branch participates.
	SourceNone = "none"
	// SourceHeuristic means the develop fallback won.
	SourceHeuristic = "heuristic"
	// SourceConfigPrefix is the prefix of Source when a declared candidate won.
	// The full value is "config[i]".
	SourceConfigPrefix = "config["

	heuristicIntegrationName = "develop"
)

// Resolution is the integration-branch answer for one repository.
type Resolution struct {
	// Participates is true when a named integration branch exists.
	Participates bool
	// Name is the bare branch name (release/2.0 keeps its slash).
	Name string
	// Source is "config[i]", "heuristic", or "none". Never empty.
	Source string
}

// Facts are the inputs the resolver needs. The conformance corpus and unit
// tests feed these without talking to git.
type Facts struct {
	// Config is the ordered list of declared integration-branch names.
	// Empty means "no declaration" and the develop heuristic may run.
	Config []string
	// Refs are existing ref names (full, e.g. refs/remotes/origin/develop).
	Refs []string
	// Remotes are registered remote names. origin is not assumed.
	Remotes []string
}

// ResolveFromFacts interprets integration-branch participation from already
// gathered facts. A declared name that does not exist does not fall back to
// develop — a typo is non-participation, not a different branch.
func ResolveFromFacts(f Facts) Resolution {
	refs := make(map[string]struct{}, len(f.Refs))
	for _, r := range f.Refs {
		r = strings.TrimSpace(r)
		if r != "" {
			refs[r] = struct{}{}
		}
	}

	declared := false
	for i, raw := range f.Config {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		declared = true
		name := NormalizeName(raw, f.Remotes)
		if name == "" {
			continue
		}
		if nameExists(name, f.Remotes, refs) {
			return Resolution{
				Participates: true,
				Name:         name,
				Source:       fmt.Sprintf("%s%d]", SourceConfigPrefix, i),
			}
		}
	}
	if declared {
		return Resolution{Source: SourceNone}
	}

	if nameExists(heuristicIntegrationName, f.Remotes, refs) {
		return Resolution{
			Participates: true,
			Name:         heuristicIntegrationName,
			Source:       SourceHeuristic,
		}
	}
	return Resolution{Source: SourceNone}
}

// NormalizeName strips only a remote prefix. The first slash of release/2.0
// is part of the branch name and is kept unless the head component is a
// registered remote.
func NormalizeName(raw string, remotes []string) string {
	name := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(name, "refs/heads/"):
		return strings.TrimPrefix(name, "refs/heads/")
	case strings.HasPrefix(name, "refs/remotes/"):
		rest := strings.TrimPrefix(name, "refs/remotes/")
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			return rest[i+1:]
		}
		return rest
	}

	if i := strings.IndexByte(name, '/'); i > 0 {
		head, tail := name[:i], name[i+1:]
		if isRemoteName(head, remotes) {
			return tail
		}
	}
	return name
}

func nameExists(name string, remotes []string, refs map[string]struct{}) bool {
	if _, ok := refs["refs/heads/"+name]; ok {
		return true
	}
	for _, remote := range remotes {
		if remote == "" {
			continue
		}
		if _, ok := refs["refs/remotes/"+remote+"/"+name]; ok {
			return true
		}
	}
	return false
}

func isRemoteName(candidate string, remotes []string) bool {
	for _, remote := range remotes {
		if remote == candidate {
			return true
		}
	}
	return false
}

// ResolveIntegrationBranch gathers remotes and refs from repoPath and
// interprets them. configValues come from the caller (empty in P2).
// A missing integration branch is not an error.
func ResolveIntegrationBranch(ctx context.Context, exec *gitcmd.Executor, repoPath string, configValues []string) (Resolution, error) {
	if exec == nil {
		return Resolution{Source: SourceNone}, fmt.Errorf("git executor is nil")
	}
	g := newGitRepo(exec, repoPath)
	remotes, err := g.remotes(ctx)
	if err != nil {
		return Resolution{Source: SourceNone}, err
	}
	refs, err := g.refNames(ctx)
	if err != nil {
		return Resolution{Source: SourceNone}, err
	}
	return ResolveFromFacts(Facts{
		Config:  configValues,
		Refs:    refs,
		Remotes: remotes,
	}), nil
}
