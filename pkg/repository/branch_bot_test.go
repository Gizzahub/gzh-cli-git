// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import "testing"

func TestIsBotBranch(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "dependabot/go_modules/x", want: true},
		{name: "renovate/docker-alpine-3.x", want: true},
		{name: "github-actions/foo", want: true},
		{name: "origin/dependabot/go_modules/x", want: true},
		{name: "origin/renovate/docker-alpine-3.x", want: true},
		{name: "upstream/github-actions/foo", want: true},
		{name: "refs/remotes/origin/dependabot/go_modules/x", want: true},
		{name: "refs/heads/dependabot/go_modules/x", want: true},
		{name: "remotes/origin/renovate/foo", want: true},
		{name: "develop", want: false},
		{name: "feat/x", want: false},
		{name: "origin/feat/x", want: false},
		{name: "feature/dependabot-docs", want: false},
		{name: "dependabot", want: false},
		{name: "", want: false},
		{name: "   ", want: false},
		{name: "main", want: false},
		{name: "master", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBotBranch(tt.name); got != tt.want {
				t.Errorf("IsBotBranch(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestBotKind(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "dependabot/go_modules/x", want: BotKindDependabot},
		{name: "origin/dependabot/npm_and_yarn/foo", want: BotKindDependabot},
		{name: "renovate/docker-alpine-3.x", want: BotKindRenovate},
		{name: "origin/renovate/foo", want: BotKindRenovate},
		{name: "github-actions/pin-actions", want: BotKindGitHubActions},
		{name: "upstream/github-actions/foo", want: BotKindGitHubActions},
		{name: "develop", want: ""},
		{name: "feat/x", want: ""},
		{name: "", want: ""},
		{name: "origin/master", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BotKind(tt.name); got != tt.want {
				t.Errorf("BotKind(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}
