// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package provider

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestClassifyTokenValidationError(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		headers http.Header
		cause   error
		wantErr error
	}{
		{name: "forbidden", status: http.StatusForbidden, wantErr: ErrTokenForbidden},
		{name: "forbidden exhausted", status: http.StatusForbidden, headers: http.Header{"X-RateLimit-Remaining": []string{"0"}}, wantErr: ErrTokenValidationRateLimited},
		{name: "too many requests", status: http.StatusTooManyRequests, wantErr: ErrTokenValidationRateLimited},
		{name: "retry after", status: http.StatusForbidden, headers: http.Header{"Retry-After": []string{"60"}}, wantErr: ErrTokenValidationRateLimited},
		{name: "server error", status: http.StatusInternalServerError, wantErr: ErrTokenValidationAPI},
		{name: "transport", status: 0, wantErr: ErrTokenValidationUnreachable},
		{name: "canceled", status: 0, cause: context.Canceled, wantErr: ErrTokenValidationCanceled},
		{name: "other api", status: http.StatusBadRequest, wantErr: ErrTokenValidationAPI},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cause := tt.cause
			if cause == nil {
				cause = errors.New("cause")
			}
			err := ClassifyTokenValidationError("test", tt.status, tt.headers, cause)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want errors.Is(..., %v)", err, tt.wantErr)
			}
			if tt.cause != nil && !errors.Is(err, tt.cause) {
				t.Fatalf("error = %v, want original cause %v", err, tt.cause)
			}
		})
	}
}

func TestSyncAction(t *testing.T) {
	tests := []struct {
		action   SyncAction
		expected string
	}{
		{ActionCloned, "cloned"},
		{ActionUpdated, "updated"},
		{ActionSkipped, "skipped"},
		{ActionFailed, "failed"},
	}

	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			if string(tt.action) != tt.expected {
				t.Errorf("SyncAction = %q, want %q", tt.action, tt.expected)
			}
		})
	}
}

func TestRepository(t *testing.T) {
	now := time.Now()
	repo := &Repository{
		Name:          "test-repo",
		FullName:      "org/test-repo",
		CloneURL:      "https://github.com/org/test-repo.git",
		SSHURL:        "git@github.com:org/test-repo.git",
		HTMLURL:       "https://github.com/org/test-repo",
		Description:   "A test repository",
		DefaultBranch: "main",
		Private:       false,
		Archived:      false,
		Fork:          false,
		Disabled:      false,
		Language:      "Go",
		Size:          1024,
		Topics:        []string{"cli", "git"},
		Visibility:    "public",
		CreatedAt:     now,
		UpdatedAt:     now,
		PushedAt:      now,
	}

	if repo.Name != "test-repo" {
		t.Errorf("Name = %q, want %q", repo.Name, "test-repo")
	}
	if repo.FullName != "org/test-repo" {
		t.Errorf("FullName = %q, want %q", repo.FullName, "org/test-repo")
	}
	if len(repo.Topics) != 2 {
		t.Errorf("Topics length = %d, want 2", len(repo.Topics))
	}
	// Verify remaining fields are set as expected.
	if repo.CloneURL != "https://github.com/org/test-repo.git" {
		t.Errorf("CloneURL = %q, want %q", repo.CloneURL, "https://github.com/org/test-repo.git")
	}
	if repo.SSHURL != "git@github.com:org/test-repo.git" {
		t.Errorf("SSHURL = %q, want %q", repo.SSHURL, "git@github.com:org/test-repo.git")
	}
	if repo.HTMLURL != "https://github.com/org/test-repo" {
		t.Errorf("HTMLURL = %q, want %q", repo.HTMLURL, "https://github.com/org/test-repo")
	}
	if repo.Description != "A test repository" {
		t.Errorf("Description = %q, want %q", repo.Description, "A test repository")
	}
	if repo.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q, want %q", repo.DefaultBranch, "main")
	}
	if repo.Private {
		t.Error("Private should be false")
	}
	if repo.Archived {
		t.Error("Archived should be false")
	}
	if repo.Fork {
		t.Error("Fork should be false")
	}
	if repo.Disabled {
		t.Error("Disabled should be false")
	}
	if repo.Language != "Go" {
		t.Errorf("Language = %q, want %q", repo.Language, "Go")
	}
	if repo.Size != 1024 {
		t.Errorf("Size = %d, want 1024", repo.Size)
	}
	if repo.Visibility != "public" {
		t.Errorf("Visibility = %q, want %q", repo.Visibility, "public")
	}
	if !repo.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", repo.CreatedAt, now)
	}
	if !repo.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want %v", repo.UpdatedAt, now)
	}
	if !repo.PushedAt.Equal(now) {
		t.Errorf("PushedAt = %v, want %v", repo.PushedAt, now)
	}
}

func TestOrganization(t *testing.T) {
	org := &Organization{
		Name:        "test-org",
		Description: "A test organization",
		URL:         "https://github.com/test-org",
	}

	if org.Name != "test-org" {
		t.Errorf("Name = %q, want %q", org.Name, "test-org")
	}
	if org.Description != "A test organization" {
		t.Errorf("Description = %q, want %q", org.Description, "A test organization")
	}
	if org.URL != "https://github.com/test-org" {
		t.Errorf("URL = %q, want %q", org.URL, "https://github.com/test-org")
	}
}

func TestRateLimit(t *testing.T) {
	reset := time.Now().Add(time.Hour)
	rl := &RateLimit{
		Limit:     5000,
		Remaining: 4500,
		Reset:     reset,
		Used:      500,
	}

	if rl.Limit != 5000 {
		t.Errorf("Limit = %d, want 5000", rl.Limit)
	}
	if rl.Remaining != 4500 {
		t.Errorf("Remaining = %d, want 4500", rl.Remaining)
	}
	if rl.Used != 500 {
		t.Errorf("Used = %d, want 500", rl.Used)
	}
	if !rl.Reset.Equal(reset) {
		t.Errorf("Reset = %v, want %v", rl.Reset, reset)
	}
}

func TestSyncOptions(t *testing.T) {
	opts := SyncOptions{
		TargetPath:      "/tmp/repos",
		Parallel:        10,
		IncludeArchived: false,
		IncludeForks:    true,
		IncludePrivate:  true,
		DryRun:          true,
	}

	if opts.TargetPath != "/tmp/repos" {
		t.Errorf("TargetPath = %q, want %q", opts.TargetPath, "/tmp/repos")
	}
	if opts.Parallel != 10 {
		t.Errorf("Parallel = %d, want 10", opts.Parallel)
	}
	if opts.IncludeArchived {
		t.Error("IncludeArchived should be false")
	}
	if !opts.IncludeForks {
		t.Error("IncludeForks should be true")
	}
	if !opts.IncludePrivate {
		t.Error("IncludePrivate should be true")
	}
	if !opts.DryRun {
		t.Error("DryRun should be true")
	}
}

func TestListOptions(t *testing.T) {
	opts := ListOptions{
		Page:    1,
		PerPage: 100,
	}

	if opts.Page != 1 {
		t.Errorf("Page = %d, want 1", opts.Page)
	}
	if opts.PerPage != 100 {
		t.Errorf("PerPage = %d, want 100", opts.PerPage)
	}
}
