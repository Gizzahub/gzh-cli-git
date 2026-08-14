package gitea

import (
	"context"
	"fmt"
	"sync"

	"code.gitea.io/sdk/gitea"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/provider"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/ratelimit"
)

// Provider implements the provider.Provider interface for Gitea.
type Provider struct {
	client      *gitea.Client
	token       string
	baseURL     string
	rateLimiter *ratelimit.Limiter
	mu          sync.RWMutex
}

// ProviderOptions configures the Gitea Provider.
type ProviderOptions struct {
	Token   string
	BaseURL string // Gitea instance URL (required)
}

// NewProvider creates a new Gitea provider.
func NewProvider(token, baseURL string) (*Provider, error) {
	return NewProviderWithOptions(ProviderOptions{
		Token:   token,
		BaseURL: baseURL,
	})
}

// NewProviderWithOptions creates a new Gitea provider with custom options.
//
// Construction is offline: the Gitea SDK's NewClient defaults to probing
// /api/v1/version, which would make object creation require a live server.
// github/gitlab NewProvider do not network; SetGiteaVersion("") matches that
// contract by skipping the constructor version check. Feature gates that need
// a real version can still be enforced later via a dedicated Ping/Validate.
func NewProviderWithOptions(opts ProviderOptions) (*Provider, error) {
	if opts.BaseURL == "" {
		return nil, fmt.Errorf("baseURL is required for Gitea provider")
	}

	p := &Provider{
		token:       opts.Token,
		baseURL:     opts.BaseURL,
		rateLimiter: ratelimit.NewLimiter(1000), // Gitea default estimate
	}

	if err := p.initClient(); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Provider) initClient() error {
	// SetGiteaVersion("") sets ignoreVersion so NewClient does not call the
	// network. Without it, NewClient always hits /api/v1/version and fails for
	// unreachable hosts (dry-run, offline unit tests, bad DNS).
	opts := []gitea.ClientOption{gitea.SetGiteaVersion("")}
	if p.token != "" {
		opts = append(opts, gitea.SetToken(p.token))
	}

	client, err := gitea.NewClient(p.baseURL, opts...)
	if err != nil {
		return fmt.Errorf("failed to create Gitea client: %w", err)
	}

	p.client = client
	return nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "gitea"
}

// SetToken sets the authentication token.
func (p *Provider) SetToken(token string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.token = token
	return p.initClient()
}

// ValidateToken validates the current token.
func (p *Provider) ValidateToken(ctx context.Context) (bool, error) {
	if p.token == "" {
		return false, nil
	}

	// GetMyUserInfo returns the authenticated user
	_, _, err := p.client.GetMyUserInfo()
	if err != nil {
		return false, fmt.Errorf("failed to validate Gitea token: %w", err)
	}
	return true, nil
}

// ListOrganizationRepos lists all repositories in a Gitea organization.
func (p *Provider) ListOrganizationRepos(ctx context.Context, org string) ([]*provider.Repository, error) {
	var allRepos []*provider.Repository

	page := 1
	for {
		repos, resp, err := p.client.ListOrgRepos(org, gitea.ListOrgReposOptions{
			ListOptions: gitea.ListOptions{
				Page:     page,
				PageSize: 50,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list repos for org %s: %w", org, err)
		}

		for _, repo := range repos {
			allRepos = append(allRepos, convertGiteaRepo(repo))
		}

		if resp == nil || resp.NextPage == 0 || len(repos) == 0 {
			break
		}
		page = resp.NextPage
	}

	return allRepos, nil
}

// ListUserRepos lists all repositories for a user.
func (p *Provider) ListUserRepos(ctx context.Context, user string) ([]*provider.Repository, error) {
	var allRepos []*provider.Repository

	page := 1
	for {
		repos, resp, err := p.client.ListUserRepos(user, gitea.ListReposOptions{
			ListOptions: gitea.ListOptions{
				Page:     page,
				PageSize: 50,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list repos for user %s: %w", user, err)
		}

		for _, repo := range repos {
			allRepos = append(allRepos, convertGiteaRepo(repo))
		}

		if resp == nil || resp.NextPage == 0 || len(repos) == 0 {
			break
		}
		page = resp.NextPage
	}

	return allRepos, nil
}

// GetRepository gets a single repository from Gitea.
func (p *Provider) GetRepository(ctx context.Context, owner, repo string) (*provider.Repository, error) {
	giteaRepo, _, err := p.client.GetRepo(owner, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to get repo %s/%s: %w", owner, repo, err)
	}

	return convertGiteaRepo(giteaRepo), nil
}

// ListOrganizations lists organizations the authenticated user belongs to.
func (p *Provider) ListOrganizations(ctx context.Context) ([]*provider.Organization, error) {
	var allOrgs []*provider.Organization

	page := 1
	for {
		orgs, resp, err := p.client.ListMyOrgs(gitea.ListOrgsOptions{
			ListOptions: gitea.ListOptions{
				Page:     page,
				PageSize: 50,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list organizations: %w", err)
		}

		for _, org := range orgs {
			allOrgs = append(allOrgs, &provider.Organization{
				Name:        org.UserName,
				Description: org.Description,
				URL:         org.Website,
			})
		}

		if resp == nil || resp.NextPage == 0 || len(orgs) == 0 {
			break
		}
		page = resp.NextPage
	}

	return allOrgs, nil
}

// GetRateLimit returns current rate limit status
// Gitea doesn't have a dedicated rate limit API, so we return estimated values.
func (p *Provider) GetRateLimit(ctx context.Context) (*provider.RateLimit, error) {
	remaining, limit, resetTime := p.rateLimiter.Status()
	return &provider.RateLimit{
		Limit:     limit,
		Remaining: remaining,
		Reset:     resetTime,
		Used:      limit - remaining,
	}, nil
}

func convertGiteaRepo(repo *gitea.Repository) *provider.Repository {
	visibility := "public"
	if repo.Private {
		visibility = "private"
	} else if repo.Internal {
		visibility = "internal"
	}

	return &provider.Repository{
		Name:          repo.Name,
		FullName:      repo.FullName,
		CloneURL:      repo.CloneURL,
		SSHURL:        repo.SSHURL,
		HTMLURL:       repo.HTMLURL,
		Description:   repo.Description,
		DefaultBranch: repo.DefaultBranch,
		Private:       repo.Private,
		Archived:      repo.Archived,
		Fork:          repo.Fork,
		Disabled:      false,
		Language:      "", // Gitea SDK doesn't expose language
		Size:          repo.Size,
		Stars:         repo.Stars,
		Topics:        nil, // Gitea SDK doesn't expose topics in Repository
		Visibility:    visibility,
		CreatedAt:     repo.Created,
		UpdatedAt:     repo.Updated,
		PushedAt:      repo.Updated, // Gitea doesn't have separate pushed_at
	}
}
