// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package provider

import (
	"context"
	"errors"
)

// ErrPullRequestNotFound means no open PR/MR matched the head/base pair.
var ErrPullRequestNotFound = errors.New("pull request not found")

// PullRequest is a forge-neutral PR or merge request.
type PullRequest struct {
	Number int
	URL    string
	Title  string
	Head   string
	Base   string
	Draft  bool
	Kind   string // "pull" or "merge_request"
}

// CreatePullRequestInput is the minimum create surface.
type CreatePullRequestInput struct {
	Owner     string
	Repo      string
	Title     string
	Body      string
	Head      string
	Base      string
	Draft     bool
	Reviewers []string
	Labels    []string
}

// PullRequester creates and looks up pull requests / merge requests.
// It is a sibling of Provider so listing-only mocks stay valid.
type PullRequester interface {
	CreatePullRequest(ctx context.Context, in CreatePullRequestInput) (*PullRequest, error)
	FindPullRequest(ctx context.Context, owner, repo, head, base string) (*PullRequest, error)
}
