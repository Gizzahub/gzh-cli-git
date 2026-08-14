// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
