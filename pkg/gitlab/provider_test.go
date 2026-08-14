// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package gitlab

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/provider"
)

func TestProvider_ValidateToken_ClassifiesFailures(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		headerName string
		headerVal  string
		wantErr    error
	}{
		{name: "ordinary forbidden", status: http.StatusForbidden, wantErr: provider.ErrTokenForbidden},
		{name: "exhausted forbidden", status: http.StatusForbidden, headerName: "RateLimit-Remaining", headerVal: "0", wantErr: provider.ErrTokenValidationRateLimited},
		{name: "too many requests", status: http.StatusTooManyRequests, wantErr: provider.ErrTokenValidationRateLimited},
		{name: "server error", status: http.StatusBadGateway, wantErr: provider.ErrTokenValidationAPI},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.headerName != "" {
					w.Header().Set(tt.headerName, tt.headerVal)
				}
				w.WriteHeader(tt.status)
			}))
			defer server.Close()

			p, err := NewProvider("test-token", server.URL)
			if err != nil {
				t.Fatalf("NewProvider: %v", err)
			}
			valid, err := p.ValidateToken(context.Background())
			if valid || !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateToken = (%v, %v), want false and errors.Is(..., %v)", valid, err, tt.wantErr)
			}
		})
	}
}

func TestProvider_ValidateToken_TransportFailureIsUnreachable(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()

	p, err := NewProvider("test-token", server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	valid, err := p.ValidateToken(context.Background())
	if valid || !errors.Is(err, provider.ErrTokenValidationUnreachable) {
		t.Fatalf("ValidateToken = (%v, %v), want false and unreachable error", valid, err)
	}
}

func TestProvider_ValidateToken_DistinguishesInvalidTokenFromAPIErrors(t *testing.T) {
	status := http.StatusUnauthorized
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	defer server.Close()

	p, err := NewProvider("test-token", server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	valid, err := p.ValidateToken(context.Background())
	if valid {
		t.Fatal("ValidateToken returned valid for unauthorized response")
	}
	if err != nil {
		t.Fatalf("ValidateToken returned error for invalid token: %v", err)
	}

	status = http.StatusInternalServerError
	valid, err = p.ValidateToken(context.Background())
	if valid || err == nil {
		t.Fatalf("ValidateToken = (%v, %v), want (false, API error)", valid, err)
	}
}

func TestProvider_ListOrganizationRepos_Contract(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/v4/groups/acme/projects" {
			t.Errorf("request path = %q, want /api/v4/groups/acme/projects", r.URL.Path)
		}
		if got := r.Header.Get("Private-Token"); got != "test-token" {
			t.Errorf("Private-Token = %q, want test-token", got)
		}
		if got := r.URL.Query().Get("per_page"); got != "100" {
			t.Errorf("per_page = %q, want 100", got)
		}
		if got := r.URL.Query().Get("include_subgroups"); got != "true" {
			t.Errorf("include_subgroups = %q, want true", got)
		}

		if requests == 1 {
			if got := r.URL.Query().Get("page"); got != "" {
				t.Errorf("first page = %q, want omitted", got)
			}
			w.Header().Set("X-Next-Page", "2")
			_, _ = w.Write([]byte(`[
{"path":"retired","path_with_namespace":"acme/retired","marked_for_deletion_on":"2026-08-01T00:00:00Z"},
{"path":"active","path_with_namespace":"acme/active","http_url_to_repo":"https://gitlab.example/acme/active.git","ssh_url_to_repo":"git@gitlab.example:acme/active.git","web_url":"https://gitlab.example/acme/active","description":"active project","default_branch":"develop","visibility":"private","archived":true,"star_count":7,"topics":["go","cli"],"created_at":"2026-08-10T12:00:00Z","last_activity_at":"2026-08-11T13:00:00Z"}
]`))
			return
		}

		if got := r.URL.Query().Get("page"); got != "2" {
			t.Errorf("second page = %q, want 2", got)
		}
		_, _ = w.Write([]byte(`[{"path":"next","path_with_namespace":"acme/next","visibility":"public"}]`))
	}))
	defer server.Close()

	p, err := NewProvider("test-token", server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	repos, err := p.ListOrganizationRepos(context.Background(), "acme")
	if err != nil {
		t.Fatalf("ListOrganizationRepos: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("got %d repositories, want 2 after filtering pending deletion", len(repos))
	}
	if repos[0].Name != "active" || repos[1].Name != "next" {
		t.Fatalf("repository names = [%q, %q], want [active, next]", repos[0].Name, repos[1].Name)
	}
	if repos[0].CloneURL != "https://gitlab.example/acme/active.git" || repos[0].SSHURL != "git@gitlab.example:acme/active.git" {
		t.Errorf("repository URLs = (%q, %q), want API-provided clone URLs", repos[0].CloneURL, repos[0].SSHURL)
	}
	if repos[0].Visibility != "private" || !repos[0].Private || !repos[0].Archived || repos[0].Stars != 7 {
		t.Errorf("repository metadata = (visibility=%q private=%t archived=%t stars=%d), want private/true/true/7", repos[0].Visibility, repos[0].Private, repos[0].Archived, repos[0].Stars)
	}
	if !repos[0].CreatedAt.Equal(time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)) || !repos[0].UpdatedAt.Equal(time.Date(2026, time.August, 11, 13, 0, 0, 0, time.UTC)) {
		t.Errorf("repository timestamps = (%s, %s), want API timestamps", repos[0].CreatedAt, repos[0].UpdatedAt)
	}
	if strings.Join(repos[0].Topics, ",") != "go,cli" {
		t.Errorf("repository topics = %v, want [go cli]", repos[0].Topics)
	}
}
