// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package gitea

import (
	"testing"
)

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

	valid, err := p.ValidateToken(nil)
	if err != nil {
		t.Errorf("ValidateToken returned error: %v", err)
	}
	if valid {
		t.Error("ValidateToken should return false for empty token")
	}
}

// Note: Integration tests that require network access should be in a separate
// file with build tag: //go:build integration
