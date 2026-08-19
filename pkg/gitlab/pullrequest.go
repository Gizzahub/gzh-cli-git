// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package gitlab

import (
	"context"
	"fmt"
	"net/http"
	"path"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/provider"
)

func projectID(owner, repo string) string {
	return path.Join(owner, repo)
}

// CreatePullRequest creates a GitLab merge request.
func (p *Provider) CreatePullRequest(ctx context.Context, in provider.CreatePullRequestInput) (*provider.PullRequest, error) {
	pid := projectID(in.Owner, in.Repo)
	if in.Base == "" {
		proj, _, err := p.client.Projects.GetProject(pid, nil, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("get default branch: %w", err)
		}
		in.Base = proj.DefaultBranch
	}
	opts := &gitlab.CreateMergeRequestOptions{
		Title:        gitlab.Ptr(in.Title),
		Description:  gitlab.Ptr(in.Body),
		SourceBranch: gitlab.Ptr(in.Head),
		TargetBranch: gitlab.Ptr(in.Base),
	}
	if len(in.Labels) > 0 {
		labels := gitlab.LabelOptions(in.Labels)
		opts.Labels = &labels
	}
	mr, resp, err := p.client.MergeRequests.CreateMergeRequest(pid, opts, gitlab.WithContext(ctx))
	if err != nil {
		if resp != nil && (resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusBadRequest) {
			existing, findErr := p.FindPullRequest(ctx, in.Owner, in.Repo, in.Head, in.Base)
			if findErr == nil {
				return existing, nil
			}
		}
		return nil, fmt.Errorf("create merge request: %w", err)
	}
	return convertMergeRequest(mr), nil
}

// FindPullRequest returns the open MR for head/base, or ErrPullRequestNotFound.
func (p *Provider) FindPullRequest(ctx context.Context, owner, repo, head, base string) (*provider.PullRequest, error) {
	opts := &gitlab.ListProjectMergeRequestsOptions{
		State:        gitlab.Ptr("opened"),
		SourceBranch: gitlab.Ptr(head),
		TargetBranch: gitlab.Ptr(base),
		ListOptions:  gitlab.ListOptions{PerPage: 10},
	}
	mrs, _, err := p.client.MergeRequests.ListProjectMergeRequests(projectID(owner, repo), opts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("list merge requests: %w", err)
	}
	for _, mr := range mrs {
		if mr.SourceBranch == head && (base == "" || mr.TargetBranch == base) {
			return convertBasicMergeRequest(mr), nil
		}
	}
	return nil, provider.ErrPullRequestNotFound
}

func convertMergeRequest(mr *gitlab.MergeRequest) *provider.PullRequest {
	return &provider.PullRequest{
		Number: int(mr.IID),
		URL:    mr.WebURL,
		Title:  mr.Title,
		Head:   mr.SourceBranch,
		Base:   mr.TargetBranch,
		Draft:  mr.Draft,
		Kind:   "merge_request",
	}
}

func convertBasicMergeRequest(mr *gitlab.BasicMergeRequest) *provider.PullRequest {
	return &provider.PullRequest{
		Number: int(mr.IID),
		URL:    mr.WebURL,
		Title:  mr.Title,
		Head:   mr.SourceBranch,
		Base:   mr.TargetBranch,
		Draft:  mr.Draft,
		Kind:   "merge_request",
	}
}
