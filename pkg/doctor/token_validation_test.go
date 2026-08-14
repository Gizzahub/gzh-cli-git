// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package doctor

import (
	"errors"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/provider"
)

func TestTokenValidationResult_ClassifiesProviderErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus Status
		wantIn     string
	}{
		{name: "invalid credentials", err: provider.ErrTokenForbidden, wantStatus: StatusError, wantIn: "invalid or forbidden"},
		{name: "rate limited", err: provider.ErrTokenValidationRateLimited, wantStatus: StatusWarning, wantIn: "rate limited"},
		{name: "unreachable", err: provider.ErrTokenValidationUnreachable, wantStatus: StatusUnreachable, wantIn: "API unreachable"},
		{name: "api failure", err: provider.ErrTokenValidationAPI, wantStatus: StatusError, wantIn: "validation error"},
		{name: "legacy external provider error", err: errors.New("connection failed"), wantStatus: StatusUnreachable, wantIn: "API unreachable"},
		{name: "legacy invalid result", err: nil, wantStatus: StatusError, wantIn: "invalid or expired"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenValidationResult("work", "github", tt.err)
			if result.Status != tt.wantStatus {
				t.Fatalf("status = %s, want %s", result.Status, tt.wantStatus)
			}
			if !strings.Contains(result.Message, tt.wantIn) {
				t.Fatalf("message = %q, want substring %q", result.Message, tt.wantIn)
			}
		})
	}
}
