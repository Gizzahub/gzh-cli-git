// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"context"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
)

func TestResolveFromFacts_CorpusABCD(t *testing.T) {
	// Rows match tests/conformance/testdata/integration-branch-corpus.tsv.
	// A: undeclared follows upstream HEAD (develop) without assuming origin.
	// B: a local develop without remote HEAD does not participate.
	// C: a typo config does not fall through to develop.
	// D: release/2.0 keeps the slash (the first component is not a remote).
	cases := []struct {
		id           string
		config       string
		refs         []string
		remote       string
		defaultName  string
		participates bool
		bare         string
	}{
		{"A", "", []string{"refs/remotes/upstream/develop"}, "upstream", "develop", true, "develop"},
		{"B", "", []string{"refs/heads/develop"}, "", "", false, ""},
		{"C", "developp", []string{"refs/remotes/origin/develop"}, "origin", "", false, ""},
		{"D", "release/2.0", []string{"refs/remotes/origin/release/2.0"}, "origin", "", true, "release/2.0"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			var remotes []string
			if tc.remote != "" {
				remotes = []string{tc.remote}
			}
			var cfg []string
			if tc.config != "" {
				cfg = []string{tc.config}
			}
			got := ResolveFromFacts(Facts{
				Config:      cfg,
				Refs:        tc.refs,
				Remotes:     remotes,
				DefaultName: tc.defaultName,
			})
			if got.Participates != tc.participates || got.Name != tc.bare {
				t.Fatalf("ResolveFromFacts(%s) = {participates=%v name=%q}, want {participates=%v name=%q}",
					tc.id, got.Participates, got.Name, tc.participates, tc.bare)
			}
			if !tc.participates && got.Source != SourceNone {
				t.Fatalf("source = %q, want %s", got.Source, SourceNone)
			}
		})
	}
}

func TestResolveFromFacts_UndeclaredFollowsDefaultHead(t *testing.T) {
	got := ResolveFromFacts(Facts{
		Refs: []string{
			"refs/remotes/origin/master",
			"refs/remotes/origin/develop",
		},
		Remotes:     []string{"origin"},
		DefaultName: "master",
	})
	if !got.Participates || got.Name != "master" || got.Source != SourceHeuristic {
		t.Fatalf("got %+v, want heuristic master", got)
	}
}

func TestResolveFromFacts_DeclaredDevelopNotOverriddenByMasterHead(t *testing.T) {
	got := ResolveFromFacts(Facts{
		Config:      []string{"develop"},
		Refs:        []string{"refs/remotes/origin/master", "refs/remotes/origin/develop"},
		Remotes:     []string{"origin"},
		DefaultName: "master",
	})
	if !got.Participates || got.Name != "develop" || got.Source != "config[0]" {
		t.Fatalf("got %+v, want config[0] develop", got)
	}
}

func TestNormalizeName_KeepsReleaseSlash(t *testing.T) {
	got := NormalizeName("release/2.0", []string{"origin"})
	if got != "release/2.0" {
		t.Fatalf("NormalizeName(release/2.0) = %q, want release/2.0", got)
	}
	got = NormalizeName("origin/develop", []string{"origin"})
	if got != "develop" {
		t.Fatalf("NormalizeName(origin/develop) = %q, want develop", got)
	}
	got = NormalizeName("refs/remotes/upstream/develop", []string{"upstream"})
	if got != "develop" {
		t.Fatalf("NormalizeName(refs/remotes/upstream/develop) = %q, want develop", got)
	}
}

func TestResolveIntegrationBranch_UsesOriginHeadNotDevelop(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	runGit(t, fx.Clone, "branch", "develop")
	runGit(t, fx.Clone, "push", "-u", fx.Remote, "develop")

	got, err := ResolveIntegrationBranch(context.Background(), gitcmd.NewExecutor(), fx.Clone, nil)
	if err != nil {
		t.Fatalf("ResolveIntegrationBranch: %v", err)
	}
	if !got.Participates || got.Name != "main" || got.Source != SourceHeuristic {
		t.Fatalf("got %+v, want heuristic main (origin/HEAD), not develop", got)
	}
}

func TestResolveIntegrationBranch_EmptyOriginHeadIsNone(t *testing.T) {
	fx := testutil.TempWorktreeWithBareOrigin(t)
	runGit(t, fx.Clone, "remote", "set-head", fx.Remote, "--delete")
	runGit(t, fx.Clone, "branch", "develop")

	got, err := ResolveIntegrationBranch(context.Background(), gitcmd.NewExecutor(), fx.Clone, nil)
	if err != nil {
		t.Fatalf("ResolveIntegrationBranch: %v", err)
	}
	if got.Participates || got.Source != SourceNone {
		t.Fatalf("got %+v, want source=none when origin/HEAD is missing", got)
	}
}

func TestResolveIntegrationBranch_MissingIsReportable(t *testing.T) {
	dir := testutil.TempGitRepoWithCommit(t)
	got, err := ResolveIntegrationBranch(context.Background(), gitcmd.NewExecutor(), dir, nil)
	if err != nil {
		t.Fatalf("ResolveIntegrationBranch: %v", err)
	}
	if got.Participates || got.Source != SourceNone {
		t.Fatalf("got %+v, want source=none", got)
	}
}
