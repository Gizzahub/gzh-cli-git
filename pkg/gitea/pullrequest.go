// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package gitea

import (
	"context"
	"fmt"

	"code.gitea.io/sdk/gitea"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/provider"
)

// CreatePullRequest creates a Gitea pull request.
func (p *Provider) CreatePullRequest(ctx context.Context, in provider.CreatePullRequestInput) (*provider.PullRequest, error) {
	_ = ctx
	if in.Base == "" {
		repo, _, err := p.client.GetRepo(in.Owner, in.Repo)
		if err != nil {
			return nil, fmt.Errorf("get default branch: %w", err)
		}
		in.Base = repo.DefaultBranch
	}
	pr, _, err := p.client.CreatePullRequest(in.Owner, in.Repo, gitea.CreatePullRequestOption{
		Title: in.Title,
		Head:  in.Head,
		Base:  in.Base,
		Body:  in.Body,
	})
	if err != nil {
		existing, findErr := p.FindPullRequest(ctx, in.Owner, in.Repo, in.Head, in.Base)
		if findErr == nil {
			return existing, nil
		}
		return nil, fmt.Errorf("create pull request: %w", err)
	}
	return convertGiteaPR(pr), nil
}

// FindPullRequest returns the open PR for head/base, or ErrPullRequestNotFound.
func (p *Provider) FindPullRequest(ctx context.Context, owner, repo, head, base string) (*provider.PullRequest, error) {
	_ = ctx
	prs, _, err := p.client.ListRepoPullRequests(owner, repo, gitea.ListPullRequestsOptions{
		State: gitea.StateOpen,
	})
	if err != nil {
		return nil, fmt.Errorf("list pull requests: %w", err)
	}
	for _, pr := range prs {
		if pr.Head != nil && pr.Head.Name == head && (base == "" || (pr.Base != nil && pr.Base.Name == base)) {
			return convertGiteaPR(pr), nil
		}
	}
	return nil, provider.ErrPullRequestNotFound
}

func convertGiteaPR(pr *gitea.PullRequest) *provider.PullRequest {
	out := &provider.PullRequest{
		Number: int(pr.Index),
		URL:    pr.HTMLURL,
		Title:  pr.Title,
		Draft:  false,
		Kind:   "pull",
	}
	if pr.Head != nil {
		out.Head = pr.Head.Name
	}
	if pr.Base != nil {
		out.Base = pr.Base.Name
	}
	return out
}
