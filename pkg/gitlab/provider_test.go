// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProvider_ValidateToken_ReturnsAPIErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
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
	if err == nil {
		t.Fatal("ValidateToken returned nil error for unauthorized response")
	}
}
