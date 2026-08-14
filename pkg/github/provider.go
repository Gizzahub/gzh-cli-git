package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/google/go-github/v88/github"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/provider"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/ratelimit"
)

// Provider implements the provider.Provider interface for GitHub.
type Provider struct {
	client      *github.Client
	token       string
	baseURL     string // Enterprise base URL; empty means github.com
	rateLimiter *ratelimit.Limiter
	mu          sync.RWMutex
}

// ProviderOptions configures the GitHub Provider.
type ProviderOptions struct {
	Token   string
	BaseURL string // GitHub Enterprise Server URL; empty for github.com
}

// NewProvider creates a new GitHub provider.
//
// A non-empty baseURL targets a GitHub Enterprise Server instance. An empty
// baseURL keeps the default github.com behavior (backward compatible).
// Invalid non-empty base URLs return an error (fail-closed; no silent fallback).
func NewProvider(token, baseURL string) (*Provider, error) {
	return NewProviderWithOptions(ProviderOptions{
		Token:   token,
		BaseURL: baseURL,
	})
}

// NewProviderWithOptions creates a new GitHub provider with custom options.
func NewProviderWithOptions(opts ProviderOptions) (*Provider, error) {
	baseURL, err := parseBaseURL(opts.BaseURL)
	if err != nil {
		return nil, err
	}

	p := &Provider{
		token:       opts.Token,
		baseURL:     baseURL,
		rateLimiter: ratelimit.NewLimiter(5000), // GitHub default
	}
	if err := p.initClient(p.token, p.baseURL); err != nil {
		return nil, err
	}
	return p, nil
}

// BaseURL returns the configured Enterprise base URL, or "" for github.com.
func (p *Provider) BaseURL() string {
	return p.baseURL
}

// initClient builds the github.Client. When baseURL is set the client is pointed
// at a GitHub Enterprise Server instance via WithEnterpriseURLs; the uploads
// endpoint follows the GHE convention (baseURL + "/api/uploads").
//
// Malformed Enterprise URLs return an error (fail-closed). Callers never receive
// a provider that silently talks to github.com after a bad GHE base URL.
func (p *Provider) initClient(token, baseURL string) error {
	var opts []github.ClientOptionsFunc
	if token != "" {
		opts = append(opts, github.WithAuthToken(token))
	}
	if baseURL != "" {
		opts = append(opts, github.WithEnterpriseURLs(baseURL, enterpriseUploadURL(baseURL)))
	}

	client, err := github.NewClient(opts...)
	if err != nil {
		return fmt.Errorf("failed to create GitHub client: %w", err)
	}
	p.client = client
	return nil
}

// SetToken sets the authentication token, preserving the configured base URL.
func (p *Provider) SetToken(token string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.token = token
	return p.initClient(token, p.baseURL)
}

// ValidateToken validates the current token.
func (p *Provider) ValidateToken(ctx context.Context) (bool, error) {
	if p.token == "" {
		return false, nil
	}
	_, resp, err := p.client.Users.Get(ctx, "")
	if err != nil {
		if resp != nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
			return false, nil //nolint:nilerr // authentication rejection means the API is reachable but the token is invalid.
		}
		return false, fmt.Errorf("failed to validate GitHub token: %w", err)
	}
	return true, nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "github"
}

// ListOrganizationRepos lists all repositories in a GitHub organization.
func (p *Provider) ListOrganizationRepos(ctx context.Context, org string) ([]*provider.Repository, error) {
	var allRepos []*provider.Repository

	opts := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		repos, resp, err := p.client.Repositories.ListByOrg(ctx, org, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list repos for org %s: %w", org, err)
		}

		for _, repo := range repos {
			allRepos = append(allRepos, convertGitHubRepo(repo))
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allRepos, nil
}

// GetRepository gets a single repository from GitHub.
func (p *Provider) GetRepository(ctx context.Context, owner, repo string) (*provider.Repository, error) {
	ghRepo, _, err := p.client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to get repo %s/%s: %w", owner, repo, err)
	}

	return convertGitHubRepo(ghRepo), nil
}

// ListOrganizations lists organizations the authenticated user belongs to.
func (p *Provider) ListOrganizations(ctx context.Context) ([]*provider.Organization, error) {
	var allOrgs []*provider.Organization

	opts := &github.ListOptions{PerPage: 100}

	for {
		orgs, resp, err := p.client.Organizations.List(ctx, "", opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list organizations: %w", err)
		}

		for _, org := range orgs {
			allOrgs = append(allOrgs, &provider.Organization{
				Name:        org.GetLogin(),
				Description: org.GetDescription(),
				URL:         org.GetHTMLURL(),
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allOrgs, nil
}

// ListUserRepos lists all repositories for a user.
func (p *Provider) ListUserRepos(ctx context.Context, user string) ([]*provider.Repository, error) {
	var allRepos []*provider.Repository

	opts := &github.RepositoryListByUserOptions{
		ListOptions: github.ListOptions{PerPage: 100},
		Type:        "all",
	}

	for {
		repos, resp, err := p.client.Repositories.ListByUser(ctx, user, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list repos for user %s: %w", user, err)
		}

		for _, repo := range repos {
			allRepos = append(allRepos, convertGitHubRepo(repo))
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allRepos, nil
}

// GetRateLimit returns current rate limit status.
func (p *Provider) GetRateLimit(ctx context.Context) (*provider.RateLimit, error) {
	limits, _, err := p.client.RateLimit.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get rate limit: %w", err)
	}

	core := limits.Core
	return &provider.RateLimit{
		Limit:     core.Limit,
		Remaining: core.Remaining,
		Reset:     core.Reset.Time,
		Used:      core.Limit - core.Remaining,
	}, nil
}

func convertGitHubRepo(repo *github.Repository) *provider.Repository {
	return &provider.Repository{
		Name:          repo.GetName(),
		FullName:      repo.GetFullName(),
		CloneURL:      repo.GetCloneURL(),
		SSHURL:        repo.GetSSHURL(),
		HTMLURL:       repo.GetHTMLURL(),
		Description:   repo.GetDescription(),
		DefaultBranch: repo.GetDefaultBranch(),
		Private:       repo.GetPrivate(),
		Archived:      repo.GetArchived(),
		Fork:          repo.GetFork(),
		Disabled:      repo.GetDisabled(),
		Language:      repo.GetLanguage(),
		Size:          repo.GetSize(),
		Stars:         repo.GetStargazersCount(),
		Topics:        repo.Topics,
		Visibility:    repo.GetVisibility(),
		CreatedAt:     repo.GetCreatedAt().Time,
		UpdatedAt:     repo.GetUpdatedAt().Time,
		PushedAt:      repo.GetPushedAt().Time,
	}
}

// parseBaseURL trims whitespace/trailing slashes and validates http(s) URLs with a host.
// Empty input is valid and means github.com. Non-empty invalid values fail closed.
func parseBaseURL(baseURL string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", nil
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid baseURL %q: %w", baseURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("invalid baseURL %q: must use http or https scheme", baseURL)
	}
	if u.Host == "" {
		return "", fmt.Errorf("invalid baseURL %q: host is required", baseURL)
	}

	return strings.TrimRight(baseURL, "/"), nil
}

// enterpriseUploadURL derives the GHE uploads endpoint from the REST base URL.
// GitHub Enterprise Server serves uploads at "<baseURL>/api/uploads" by convention.
func enterpriseUploadURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/api/uploads"
}
