// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	providerapi "github.com/gizzahub/gzh-cli-gitforge/pkg/provider"
)

func TestProvider_ValidateToken_ClassifiesFailures(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		headerName string
		headerVal  string
		wantErr    error
	}{
		{name: "ordinary forbidden", status: http.StatusForbidden, wantErr: providerapi.ErrTokenForbidden},
		{name: "exhausted forbidden", status: http.StatusForbidden, headerName: "X-RateLimit-Remaining", headerVal: "0", wantErr: providerapi.ErrTokenValidationRateLimited},
		{name: "too many requests", status: http.StatusTooManyRequests, wantErr: providerapi.ErrTokenValidationRateLimited},
		{name: "server error", status: http.StatusBadGateway, wantErr: providerapi.ErrTokenValidationAPI},
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

			p := mustNewProvider(t, "test-token", server.URL)
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

	p := mustNewProvider(t, "test-token", server.URL)
	valid, err := p.ValidateToken(context.Background())
	if valid || !errors.Is(err, providerapi.ErrTokenValidationUnreachable) {
		t.Fatalf("ValidateToken = (%v, %v), want false and unreachable error", valid, err)
	}
}

func mustNewProvider(t *testing.T, token, baseURL string) *Provider {
	t.Helper()
	p, err := NewProvider(token, baseURL)
	if err != nil {
		t.Fatalf("NewProvider(%q, %q) unexpected error: %v", token, baseURL, err)
	}
	return p
}

func TestNewProvider(t *testing.T) {
	provider := mustNewProvider(t, "test-token", "")

	if provider.Name() != "github" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "github")
	}

	if provider.token != "test-token" {
		t.Errorf("token = %q, want %q", provider.token, "test-token")
	}

	if provider.baseURL != "" {
		t.Errorf("baseURL = %q, want empty", provider.baseURL)
	}

	if provider.client == nil {
		t.Error("client should not be nil")
	}
}

func TestNewProvider_EmptyToken(t *testing.T) {
	provider := mustNewProvider(t, "", "")

	if provider.Name() != "github" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "github")
	}

	if provider.client == nil {
		t.Error("client should not be nil even with empty token")
	}
}

func TestNewProvider_EnterpriseBaseURL(t *testing.T) {
	provider := mustNewProvider(t, "token", "https://github.example.com")

	if provider.baseURL != "https://github.example.com" {
		t.Errorf("baseURL = %q, want %q", provider.baseURL, "https://github.example.com")
	}
	if provider.client == nil {
		t.Error("client should not be nil for enterprise URL")
	}
}

func TestNewProvider_NormalizesBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://github.example.com/":    "https://github.example.com",
		"  https://github.example.com  ": "https://github.example.com",
		"https://github.example.com//":   "https://github.example.com",
	}
	for input, want := range cases {
		p := mustNewProvider(t, "token", input)
		if p.baseURL != want {
			t.Errorf("parseBaseURL(%q) = %q, want %q", input, p.baseURL, want)
		}
		if p.client == nil {
			t.Errorf("client is nil for baseURL %q", input)
		}
	}
}

func TestNewProvider_InvalidBaseURLFailClosed(t *testing.T) {
	cases := []string{
		"https://",
		"http://",
		"ftp://github.example.com",
		"github.example.com",
		"javascript:alert(1)",
	}
	for _, input := range cases {
		p, err := NewProvider("token", input)
		if err == nil {
			t.Errorf("NewProvider(%q) expected error, got provider baseURL=%q", input, p.baseURL)
			continue
		}
		if p != nil {
			t.Errorf("NewProvider(%q) expected nil provider on error", input)
		}
		if !strings.Contains(err.Error(), "invalid baseURL") && !strings.Contains(err.Error(), "failed to create GitHub client") {
			t.Errorf("NewProvider(%q) error = %v, want invalid baseURL or client create failure", input, err)
		}
	}
}

func TestNewProviderWithOptions(t *testing.T) {
	p, err := NewProviderWithOptions(ProviderOptions{
		Token:   "tok",
		BaseURL: "https://ghe.acme.io",
	})
	if err != nil {
		t.Fatalf("NewProviderWithOptions: %v", err)
	}
	if p.token != "tok" {
		t.Errorf("token = %q, want %q", p.token, "tok")
	}
	if p.baseURL != "https://ghe.acme.io" {
		t.Errorf("baseURL = %q, want %q", p.baseURL, "https://ghe.acme.io")
	}
	if p.client == nil {
		t.Error("client should not be nil")
	}
}

func TestProvider_SetToken(t *testing.T) {
	provider := mustNewProvider(t, "initial-token", "https://github.example.com")

	err := provider.SetToken("new-token")
	if err != nil {
		t.Errorf("SetToken failed: %v", err)
	}

	if provider.token != "new-token" {
		t.Errorf("token = %q, want %q", provider.token, "new-token")
	}

	if provider.baseURL != "https://github.example.com" {
		t.Errorf("baseURL = %q, want %q", provider.baseURL, "https://github.example.com")
	}
	if provider.client == nil {
		t.Error("client should not be nil after SetToken")
	}
}

func TestProvider_ValidateToken_EmptyToken(t *testing.T) {
	provider := mustNewProvider(t, "", "")

	valid, err := provider.ValidateToken(context.TODO())
	if err != nil {
		t.Errorf("ValidateToken returned error: %v", err)
	}
	if valid {
		t.Error("ValidateToken should return false for empty token")
	}
}

func TestProvider_ValidateToken_DistinguishesInvalidTokenFromAPIErrors(t *testing.T) {
	status := http.StatusUnauthorized
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	defer server.Close()

	provider := mustNewProvider(t, "test-token", server.URL)
	valid, err := provider.ValidateToken(context.Background())
	if valid {
		t.Fatal("ValidateToken returned valid for unauthorized response")
	}
	if err != nil {
		t.Fatalf("ValidateToken returned error for invalid token: %v", err)
	}

	status = http.StatusInternalServerError
	valid, err = provider.ValidateToken(context.Background())
	if valid || err == nil {
		t.Fatalf("ValidateToken = (%v, %v), want (false, API error)", valid, err)
	}
}

func TestProvider_Name(t *testing.T) {
	provider := mustNewProvider(t, "token", "")

	if provider.Name() != "github" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "github")
	}
}

func TestBaseURLAccessor(t *testing.T) {
	if got := mustNewProvider(t, "token", "").BaseURL(); got != "" {
		t.Errorf("BaseURL() = %q, want empty", got)
	}
	want := "https://github.example.com"
	if got := mustNewProvider(t, "token", want).BaseURL(); got != want {
		t.Errorf("BaseURL() = %q, want %q", got, want)
	}
}

func TestEnterpriseUploadURL(t *testing.T) {
	cases := map[string]string{
		"https://github.example.com":  "https://github.example.com/api/uploads",
		"https://github.example.com/": "https://github.example.com/api/uploads",
		"https://ghe.io:8443":         "https://ghe.io:8443/api/uploads",
	}
	for base, want := range cases {
		if got := enterpriseUploadURL(base); got != want {
			t.Errorf("enterpriseUploadURL(%q) = %q, want %q", base, got, want)
		}
	}
}
