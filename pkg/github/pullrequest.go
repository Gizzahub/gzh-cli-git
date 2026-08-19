// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package github

import (
	"context"
	"fmt"
	"net/http"

	gh "github.com/google/go-github/v88/github"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/provider"
)

// CreatePullRequest creates a GitHub pull request. Reviewers and labels are
// applied after create when supplied.
func (p *Provider) CreatePullRequest(ctx context.Context, in provider.CreatePullRequestInput) (*provider.PullRequest, error) {
	if in.Base == "" {
		repo, _, err := p.client.Repositories.Get(ctx, in.Owner, in.Repo)
		if err != nil {
			return nil, fmt.Errorf("get default branch: %w", err)
		}
		in.Base = repo.GetDefaultBranch()
	}
	pr, resp, err := p.client.PullRequests.Create(ctx, in.Owner, in.Repo, &gh.NewPullRequest{
		Title: gh.Ptr(in.Title),
		Head:  gh.Ptr(in.Head),
		Base:  gh.Ptr(in.Base),
		Body:  gh.Ptr(in.Body),
		Draft: gh.Ptr(in.Draft),
	})
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnprocessableEntity {
			existing, findErr := p.FindPullRequest(ctx, in.Owner, in.Repo, in.Head, in.Base)
			if findErr == nil {
				return existing, nil
			}
		}
		return nil, fmt.Errorf("create pull request: %w", err)
	}
	out := convertPullRequest(pr)
	if len(in.Labels) > 0 {
		if _, _, err := p.client.Issues.AddLabelsToIssue(ctx, in.Owner, in.Repo, pr.GetNumber(), in.Labels); err != nil {
			return out, fmt.Errorf("created PR %d but adding labels failed: %w", pr.GetNumber(), err)
		}
	}
	if len(in.Reviewers) > 0 {
		if _, _, err := p.client.PullRequests.RequestReviewers(ctx, in.Owner, in.Repo, pr.GetNumber(), gh.ReviewersRequest{
			Reviewers: in.Reviewers,
		}); err != nil {
			return out, fmt.Errorf("created PR %d but requesting reviewers failed: %w", pr.GetNumber(), err)
		}
	}
	return out, nil
}

// FindPullRequest returns the open PR for head/base, or ErrPullRequestNotFound.
func (p *Provider) FindPullRequest(ctx context.Context, owner, repo, head, base string) (*provider.PullRequest, error) {
	opts := &gh.PullRequestListOptions{
		State: "open",
		Base:  base,
		Head:  owner + ":" + head,
		ListOptions: gh.ListOptions{
			PerPage: 10,
		},
	}
	prs, _, err := p.client.PullRequests.List(ctx, owner, repo, opts)
	if err != nil {
		return nil, fmt.Errorf("list pull requests: %w", err)
	}
	for _, pr := range prs {
		if pr.GetHead().GetRef() == head && (base == "" || pr.GetBase().GetRef() == base) {
			return convertPullRequest(pr), nil
		}
	}
	return nil, provider.ErrPullRequestNotFound
}

func convertPullRequest(pr *gh.PullRequest) *provider.PullRequest {
	return &provider.PullRequest{
		Number: pr.GetNumber(),
		URL:    pr.GetHTMLURL(),
		Title:  pr.GetTitle(),
		Head:   pr.GetHead().GetRef(),
		Base:   pr.GetBase().GetRef(),
		Draft:  pr.GetDraft(),
		Kind:   "pull",
	}
}
