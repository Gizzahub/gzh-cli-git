// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package gitea

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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
		{name: "exhausted forbidden", status: http.StatusForbidden, headerName: "X-RateLimit-Remaining", headerVal: "0", wantErr: provider.ErrTokenValidationRateLimited},
		{name: "too many requests", status: http.StatusTooManyRequests, wantErr: provider.ErrTokenValidationRateLimited},
		{name: "server error", status: http.StatusBadGateway, wantErr: provider.ErrTokenValidationAPI},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/user" {
					t.Errorf("request path = %q, want /api/v1/user", r.URL.Path)
				}
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

func TestProvider_ValidateToken_RejectsMalformedSuccessBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()

	p, err := NewProvider("test-token", server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	valid, err := p.ValidateToken(context.Background())
	if valid || !errors.Is(err, provider.ErrTokenValidationAPI) {
		t.Fatalf("ValidateToken = (%v, %v), want false and API validation error", valid, err)
	}
}

func TestProvider_ValidateToken_RejectsRedirectedLoginBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/user" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>login</html>"))
	}))
	defer server.Close()

	p, err := NewProvider("test-token", server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	valid, err := p.ValidateToken(context.Background())
	if valid || !errors.Is(err, provider.ErrTokenValidationAPI) {
		t.Fatalf("ValidateToken = (%v, %v), want false and API validation error", valid, err)
	}
}

func TestProvider_ValidateToken_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p, err := NewProvider("test-token", "https://gitea.example.invalid")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	valid, err := p.ValidateToken(ctx)
	if valid || !errors.Is(err, provider.ErrTokenValidationCanceled) {
		t.Fatalf("ValidateToken = (%v, %v), want false and canceled error", valid, err)
	}
}

func TestNewProvider_RequiresBaseURL(t *testing.T) {
	_, err := NewProvider("token", "")
	if err == nil {
		t.Error("Expected error when baseURL is empty")
	}
}

// TestNewProvider_NoNetworkOnConstruct proves NewProvider succeeds with a fake
// base URL and no server. Regression for constructor version probing that used
// to dial /api/v1/version and fail offline unit tests.
func TestNewProvider_NoNetworkOnConstruct(t *testing.T) {
	p, err := NewProvider("test-token", "https://gitea.example.invalid")
	if err != nil {
		t.Fatalf("NewProvider with unreachable host: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if p.Name() != "gitea" {
		t.Errorf("Name() = %q, want gitea", p.Name())
	}
	if p.baseURL != "https://gitea.example.invalid" {
		t.Errorf("baseURL = %q, want fake URL", p.baseURL)
	}
	if p.client == nil {
		t.Error("client should be constructed offline")
	}
	if p.token != "test-token" {
		t.Errorf("token = %q, want test-token", p.token)
	}
}

func TestProviderOptions(t *testing.T) {
	opts := ProviderOptions{
		Token:   "test-token",
		BaseURL: "https://gitea.example.com",
	}

	if opts.Token != "test-token" {
		t.Errorf("Token = %q, want %q", opts.Token, "test-token")
	}
	if opts.BaseURL != "https://gitea.example.com" {
		t.Errorf("BaseURL = %q, want %q", opts.BaseURL, "https://gitea.example.com")
	}
}

func TestProvider_Name(t *testing.T) {
	// Test Name() method directly on a minimal provider struct
	// without creating a real client
	p := &Provider{}
	if p.Name() != "gitea" {
		t.Errorf("Name() = %q, want %q", p.Name(), "gitea")
	}
}

func TestProvider_ValidateToken_EmptyToken(t *testing.T) {
	// Test with empty token without network call
	p := &Provider{
		token: "",
	}

	valid, err := p.ValidateToken(context.TODO())
	if err != nil {
		t.Errorf("ValidateToken returned error: %v", err)
	}
	if valid {
		t.Error("ValidateToken should return false for empty token")
	}
}

func TestProvider_ValidateToken_DistinguishesInvalidTokenFromAPIErrors(t *testing.T) {
	status := http.StatusUnauthorized
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/user" {
			t.Errorf("request path = %q, want /api/v1/user", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "token test-token" {
			t.Errorf("Authorization = %q, want token test-token", got)
		}
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

func TestProvider_ListOrganizations_UsesNameAndPagination(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/v1/user/orgs" {
			t.Errorf("request path = %q, want /api/v1/user/orgs", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "token test-token" {
			t.Errorf("Authorization = %q, want token test-token", got)
		}
		if got := r.URL.Query().Get("limit"); got != "50" {
			t.Errorf("limit = %q, want 50", got)
		}
		if requests == 1 {
			if got := r.URL.Query().Get("page"); got != "1" {
				t.Errorf("first page = %q, want 1", got)
			}
			w.Header().Set("Link", `<`+r.URL.Path+`?page=2&limit=50>; rel="next"`)
			_, _ = w.Write([]byte(`[{"name":"canonical","username":"legacy","description":"first","website":"https://gitea.example/canonical"}]`))
			return
		}

		if got := r.URL.Query().Get("page"); got != "2" {
			t.Errorf("second page = %q, want 2", got)
		}
		_, _ = w.Write([]byte(`[{"name":"second","username":"legacy-second"}]`))
	}))
	defer server.Close()

	p, err := NewProvider("test-token", server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	orgs, err := p.ListOrganizations(context.Background())
	if err != nil {
		t.Fatalf("ListOrganizations: %v", err)
	}
	if len(orgs) != 2 {
		t.Fatalf("got %d organizations, want 2", len(orgs))
	}
	if orgs[0].Name != "canonical" || orgs[1].Name != "second" {
		t.Fatalf("organization names = [%q, %q], want [canonical, second]", orgs[0].Name, orgs[1].Name)
	}
	if orgs[0].URL != "https://gitea.example/canonical" || orgs[0].Description != "first" {
		t.Errorf("first organization = %#v, want mapped description and URL", orgs[0])
	}
}

// Note: Integration tests that require network access should be in a separate
// file with build tag: //go:build integration
