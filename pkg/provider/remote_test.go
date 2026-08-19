// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package provider

import "testing"

func TestParseForgeRemote(t *testing.T) {
	tests := []struct {
		raw  string
		want ForgeRemote
	}{
		{
			raw:  "git@github.com:Gizzahub/gzh-cli.git",
			want: ForgeRemote{Provider: "github", Host: "github.com", Owner: "Gizzahub", Repo: "gzh-cli"},
		},
		{
			raw:  "https://github.com/Gizzahub/gzh-cli.git",
			want: ForgeRemote{Provider: "github", Host: "github.com", Owner: "Gizzahub", Repo: "gzh-cli"},
		},
		{
			raw:  "ssh://git@gitlab.polypia.net:2224/devbox/gzh-cli-devbox.git",
			want: ForgeRemote{Provider: "gitlab", Host: "gitlab.polypia.net", Owner: "devbox", Repo: "gzh-cli-devbox", BaseURL: "https://gitlab.polypia.net"},
		},
		{
			raw:  "https://gitlab.com/group/sub/project.git",
			want: ForgeRemote{Provider: "gitlab", Host: "gitlab.com", Owner: "group/sub", Repo: "project"},
		},
		{
			raw:  "git@gitea.example.com:org/repo.git",
			want: ForgeRemote{Provider: "gitea", Host: "gitea.example.com", Owner: "org", Repo: "repo", BaseURL: "https://gitea.example.com"},
		},
		{
			raw:  "https://github.example.com/acme/app.git",
			want: ForgeRemote{Provider: "github", Host: "github.example.com", Owner: "acme", Repo: "app", BaseURL: "https://github.example.com"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := ParseForgeRemote(tt.raw)
			if err != nil {
				t.Fatalf("ParseForgeRemote: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseForgeRemote_Rejects(t *testing.T) {
	for _, raw := range []string{"", "not-a-url", "https://github.com/onlyone"} {
		if _, err := ParseForgeRemote(raw); err == nil {
			t.Errorf("ParseForgeRemote(%q) = nil, want error", raw)
		}
	}
}
