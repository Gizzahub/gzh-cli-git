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
	// SourceHeuristic means the remote-HEAD fallback won.
	SourceHeuristic = "heuristic"
	// SourceConfigPrefix is the prefix of Source when a declared candidate won.
	// The full value is "config[i]".
	SourceConfigPrefix = "config["
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
	// Empty means "no declaration" and the remote-HEAD heuristic may run.
	Config []string
	// Refs are existing ref names (full, e.g. refs/remotes/origin/develop).
	Refs []string
	// Remotes are registered remote names. origin is not assumed.
	Remotes []string
	// DefaultName is the bare default-branch name from remote HEAD
	// (origin/HEAD → master). Empty when HEAD is missing or dangling.
	DefaultName string
}

// ResolveFromFacts interprets integration-branch participation from already
// gathered facts. A declared name that does not exist does not fall back to
// the default branch — a typo is non-participation, not a different branch.
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

	defaultName := strings.TrimSpace(f.DefaultName)
	if defaultName != "" && nameExists(defaultName, f.Remotes, refs) {
		return Resolution{
			Participates: true,
			Name:         defaultName,
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
	if _, branch, ok := SplitRemoteBranch(name, remotes); ok {
		return branch
	}
	remoteRef := strings.HasPrefix(name, "refs/remotes/")
	switch {
	case strings.HasPrefix(name, "refs/heads/"):
		return strings.TrimPrefix(name, "refs/heads/")
	case strings.HasPrefix(name, "refs/remotes/"):
		name = strings.TrimPrefix(name, "refs/remotes/")
	}
	if remoteRef {
		// Compatibility fallback for callers that have a full remote ref but no
		// remote inventory. With an inventory, SplitRemoteBranch above handles
		// slash-containing remote names without guessing at the first segment.
		if i := strings.IndexByte(name, '/'); i >= 0 {
			return name[i+1:]
		}
		return name
	}
	return name
}

// SplitRemoteBranch separates a tracking ref using the longest registered
// remote prefix. Git permits slash-containing remote names, so splitting on
// the first slash would mistake team/upstream/main for remote team and branch
// upstream/main.
func SplitRemoteBranch(raw string, remotes []string) (remote, branch string, ok bool) {
	name := strings.TrimSpace(raw)
	if remote, branch, ok := splitRegisteredRemote(name, remotes); ok {
		return remote, branch, true
	}
	if stripped, found := strings.CutPrefix(name, "refs/remotes/"); found {
		return splitRegisteredRemote(stripped, remotes)
	}
	return "", "", false
}

func splitRegisteredRemote(name string, remotes []string) (remote, branch string, ok bool) {
	best := ""
	for _, candidate := range remotes {
		candidate = strings.TrimSpace(candidate)
		if len(candidate) <= len(best) || !strings.HasPrefix(name, candidate+"/") {
			continue
		}
		best = candidate
	}
	if best == "" {
		return "", "", false
	}
	return best, strings.TrimPrefix(name, best+"/"), true
}

// UpstreamTargetsIntegration reports the unsafe tracking relationship where
// work on a non-integration branch tracks the integration branch itself. The
// comparison is name-based rather than SHA-based so a freshly created branch
// is caught even while both refs still point at the same commit.
func UpstreamTargetsIntegration(branch, upstream string, resolution Resolution, remotes []string) bool {
	if !resolution.Participates || strings.TrimSpace(branch) == "" || strings.TrimSpace(upstream) == "" {
		return false
	}
	branchName := NormalizeName(branch, remotes)
	if branchName == resolution.Name {
		return false
	}
	return NormalizeName(upstream, remotes) == resolution.Name
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
// interprets them. configValues come from the caller. A missing
// integration branch is not an error.
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
	defaultName := ""
	if remote := preferredRemote(remotes); remote != "" {
		def, ok, err := g.symbolicRef(ctx, "refs/remotes/"+remote+"/HEAD")
		if err != nil {
			return Resolution{Source: SourceNone}, err
		}
		if ok {
			defaultName = NormalizeName(def, remotes)
		}
	}
	return ResolveFromFacts(Facts{
		Config:      configValues,
		Refs:        refs,
		Remotes:     remotes,
		DefaultName: defaultName,
	}), nil
}
