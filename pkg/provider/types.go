// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var (
	// ErrTokenForbidden indicates that the provider rejected the credentials.
	ErrTokenForbidden = errors.New("token validation forbidden")
	// ErrTokenValidationRateLimited indicates that validation was rejected by a
	// provider rate limit.
	ErrTokenValidationRateLimited = errors.New("token validation rate limited")
	// ErrTokenValidationUnreachable indicates that the validation endpoint could
	// not be reached (for example, a transport or DNS failure).
	ErrTokenValidationUnreachable = errors.New("token validation endpoint unreachable")
	// ErrTokenValidationAPI indicates an API response that was neither an
	// authentication rejection nor a rate-limit response.
	ErrTokenValidationAPI = errors.New("token validation API error")
	// ErrTokenValidationCanceled indicates that the caller canceled validation.
	ErrTokenValidationCanceled = errors.New("token validation canceled")
)

// ClassifyTokenValidationError wraps a provider validation failure with a
// stable, provider-independent sentinel. The response status and headers are
// optional when the request failed before an HTTP response was received.
//
// A 403 is treated as a credential rejection unless the provider explicitly
// reports that its rate limit is exhausted. 401 remains the caller's
// responsibility because providers historically return (false, nil) for it.
func ClassifyTokenValidationError(providerName string, status int, headers http.Header, cause error) error {
	kind := ErrTokenValidationAPI
	if errors.Is(cause, context.Canceled) {
		kind = ErrTokenValidationCanceled
	} else {
		switch {
		case status == http.StatusTooManyRequests || (status == http.StatusForbidden && hasRateLimitEvidence(headers)):
			kind = ErrTokenValidationRateLimited
		case status == http.StatusForbidden:
			kind = ErrTokenForbidden
		case status == 0:
			kind = ErrTokenValidationUnreachable
		case status >= http.StatusInternalServerError:
			kind = ErrTokenValidationAPI
		}
	}

	if cause == nil {
		return fmt.Errorf("%s: %w", providerName, kind)
	}
	return fmt.Errorf("%s: %w: %w", providerName, kind, cause)
}

func hasRateLimitEvidence(headers http.Header) bool {
	if headers == nil {
		return false
	}
	for _, name := range []string{"X-RateLimit-Remaining", "RateLimit-Remaining"} {
		for key, values := range headers {
			if strings.EqualFold(key, name) && len(values) > 0 && strings.TrimSpace(values[0]) == "0" {
				return true
			}
		}
		if strings.TrimSpace(headers.Get(name)) == "0" {
			return true
		}
	}
	for key, values := range headers {
		if strings.EqualFold(key, "Retry-After") && len(values) > 0 {
			return strings.TrimSpace(values[0]) != ""
		}
	}
	return strings.TrimSpace(headers.Get("Retry-After")) != ""
}

// Repository represents a repository from any Git platform.
type Repository struct {
	Name          string
	FullName      string
	CloneURL      string
	SSHURL        string
	HTMLURL       string
	Description   string
	DefaultBranch string
	Private       bool
	Archived      bool
	Fork          bool
	Disabled      bool
	Language      string
	Size          int
	Stars         int
	Topics        []string
	Visibility    string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	PushedAt      time.Time
}

// Organization represents an organization or group from any Git platform.
type Organization struct {
	Name        string
	Description string
	URL         string
}

// SyncOptions configures repository synchronization.
type SyncOptions struct {
	TargetPath      string
	Parallel        int
	IncludeArchived bool
	IncludeForks    bool
	IncludePrivate  bool
	DryRun          bool
}

// SyncResult represents the result of syncing a single repository.
type SyncResult struct {
	Repository *Repository
	Action     SyncAction
	Error      error
}

// SyncAction represents what action was taken during sync.
type SyncAction string

// SyncAction values describe the outcome of a repository sync operation.
const (
	ActionCloned  SyncAction = "cloned"
	ActionUpdated SyncAction = "updated"
	ActionSkipped SyncAction = "skipped"
	ActionFailed  SyncAction = "failed"
)

// RateLimit represents API rate limit information.
type RateLimit struct {
	Limit     int
	Remaining int
	Reset     time.Time
	Used      int
}

// ListOptions common pagination options.
type ListOptions struct {
	Page    int
	PerPage int
}

// Provider defines the interface for Git platform providers.
type Provider interface {
	// Name returns the provider name (github, gitlab, gitea)
	Name() string

	// ListOrganizationRepos lists all repositories in an organization/group
	ListOrganizationRepos(ctx context.Context, org string) ([]*Repository, error)

	// ListUserRepos lists all repositories for a user
	ListUserRepos(ctx context.Context, user string) ([]*Repository, error)

	// GetRepository gets a single repository
	GetRepository(ctx context.Context, owner, repo string) (*Repository, error)

	// ListOrganizations lists organizations the authenticated user belongs to
	ListOrganizations(ctx context.Context) ([]*Organization, error)

	// GetRateLimit returns current rate limit status
	GetRateLimit(ctx context.Context) (*RateLimit, error)
}

// ProviderWithAuth extends Provider with authentication capabilities.
type ProviderWithAuth interface {
	Provider

	// SetToken sets the authentication token
	SetToken(token string) error

	// ValidateToken validates the current token
	ValidateToken(ctx context.Context) (bool, error)
}

// Syncer handles repository synchronization operations.
type Syncer interface {
	// SyncOrganization syncs all repositories from an organization
	SyncOrganization(ctx context.Context, provider Provider, org string, opts SyncOptions) ([]SyncResult, error)
}
