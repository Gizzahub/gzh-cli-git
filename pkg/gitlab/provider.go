package gitlab

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/xanzy/go-gitlab"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/provider"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/ratelimit"
)

// Provider implements the provider.Provider interface for GitLab.
type Provider struct {
	client      *gitlab.Client
	token       string
	baseURL     string
	sshHost     string // SSH hostname (e.g., "gitlab.polypia.net")
	sshPort     int    // SSH port (e.g., 2224, 0 means default 22)
	rateLimiter *ratelimit.Limiter
	mu          sync.RWMutex
}

// ProviderOptions configures the GitLab Provider.
type ProviderOptions struct {
	Token   string
	BaseURL string // API endpoint (http/https only)
	SSHPort int    // Custom SSH port (0 = default 22)
}

// NewProvider creates a new GitLab provider.
func NewProvider(token, baseURL string) (*Provider, error) {
	return NewProviderWithOptions(ProviderOptions{
		Token:   token,
		BaseURL: baseURL,
	})
}

// NewProviderWithOptions creates a new GitLab provider with custom options.
func NewProviderWithOptions(opts ProviderOptions) (*Provider, error) {
	p := &Provider{
		token:       opts.Token,
		baseURL:     opts.BaseURL,
		sshPort:     opts.SSHPort,
		rateLimiter: ratelimit.NewLimiter(2000), // GitLab default
	}

	// Extract SSH host from baseURL (API endpoint)
	if opts.BaseURL != "" {
		p.sshHost = extractHostFromURL(opts.BaseURL)
	}

	// Set custom SSH port
	p.sshPort = opts.SSHPort

	if err := p.initClient(); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Provider) initClient() error {
	var client *gitlab.Client
	var err error

	if p.baseURL != "" {
		client, err = gitlab.NewClient(p.token, gitlab.WithBaseURL(p.baseURL))
	} else {
		client, err = gitlab.NewClient(p.token)
	}

	if err != nil {
		return fmt.Errorf("failed to create GitLab client: %w", err)
	}

	p.client = client
	return nil
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
	_, _, err := p.client.Users.CurrentUser(gitlab.WithContext(ctx))
	if err != nil {
		return false, fmt.Errorf("failed to validate GitLab token: %w", err)
	}
	return true, nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "gitlab"
}

// SSHPort returns the configured custom SSH port (0 means the default, 22).
// It makes the SSH-port wiring observable so callers/tests can confirm the port
// survived provider construction — the field that historically drifted between
// the forge-sync and doctor provider-creation paths.
func (p *Provider) SSHPort() int {
	return p.sshPort
}

// ListOrganizationRepos lists all repositories in a GitLab group.
func (p *Provider) ListOrganizationRepos(ctx context.Context, group string) ([]*provider.Repository, error) {
	var allRepos []*provider.Repository

	opts := &gitlab.ListGroupProjectsOptions{
		ListOptions:      gitlab.ListOptions{PerPage: 100},
		IncludeSubGroups: new(true),
	}

	for {
		projects, resp, err := p.client.Groups.ListGroupProjects(group, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("failed to list projects for group %s: %w", group, err)
		}

		for _, project := range projects {
			// Skip projects scheduled for deletion
			if project.MarkedForDeletionAt != nil {
				continue
			}
			allRepos = append(allRepos, p.convertGitLabProject(project))
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allRepos, nil
}

// GetRepository gets a single repository from GitLab.
func (p *Provider) GetRepository(ctx context.Context, owner, repo string) (*provider.Repository, error) {
	projectPath := fmt.Sprintf("%s/%s", owner, repo)
	project, _, err := p.client.Projects.GetProject(projectPath, nil, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to get project %s: %w", projectPath, err)
	}

	return p.convertGitLabProject(project), nil
}

// ListOrganizations lists groups the authenticated user belongs to.
func (p *Provider) ListOrganizations(ctx context.Context) ([]*provider.Organization, error) {
	var allOrgs []*provider.Organization

	opts := &gitlab.ListGroupsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100},
	}

	for {
		groups, resp, err := p.client.Groups.ListGroups(opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("failed to list groups: %w", err)
		}

		for _, group := range groups {
			allOrgs = append(allOrgs, &provider.Organization{
				Name:        group.Path,
				Description: group.Description,
				URL:         group.WebURL,
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

	opts := &gitlab.ListProjectsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100},
	}

	for {
		projects, resp, err := p.client.Projects.ListUserProjects(user, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("failed to list projects for user %s: %w", user, err)
		}

		for _, project := range projects {
			// Skip projects scheduled for deletion
			if project.MarkedForDeletionAt != nil {
				continue
			}
			allRepos = append(allRepos, p.convertGitLabProject(project))
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allRepos, nil
}

// GetRateLimit returns current rate limit status
// GitLab doesn't have a dedicated rate limit API, so we return estimated values.
func (p *Provider) GetRateLimit(ctx context.Context) (*provider.RateLimit, error) {
	remaining, limit, resetTime := p.rateLimiter.Status()
	return &provider.RateLimit{
		Limit:     limit,
		Remaining: remaining,
		Reset:     resetTime,
		Used:      limit - remaining,
	}, nil
}

func (p *Provider) convertGitLabProject(project *gitlab.Project) *provider.Repository {
	var createdAt, updatedAt, pushedAt time.Time
	if project.CreatedAt != nil {
		createdAt = *project.CreatedAt
	}
	if project.LastActivityAt != nil {
		updatedAt = *project.LastActivityAt
		pushedAt = *project.LastActivityAt
	}

	// Use GitLab API's SSH URL (already includes correct port from GitLab config)
	// Only override if custom SSH port is explicitly specified
	sshURL := project.SSHURLToRepo
	if p.sshPort > 0 {
		// User explicitly specified SSH port - override API response
		if p.sshHost != "" {
			sshURL = p.buildSSHURL(project.PathWithNamespace)
		}
	}

	return &provider.Repository{
		Name:          project.Path,
		FullName:      project.PathWithNamespace,
		CloneURL:      project.HTTPURLToRepo,
		SSHURL:        sshURL,
		HTMLURL:       project.WebURL,
		Description:   project.Description,
		DefaultBranch: project.DefaultBranch,
		Private:       project.Visibility != gitlab.PublicVisibility,
		Archived:      project.Archived,
		Fork:          project.ForkedFromProject != nil,
		Disabled:      false,
		Language:      "",
		Size:          0,
		Stars:         project.StarCount,
		Topics:        project.Topics,
		Visibility:    string(project.Visibility),
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
		PushedAt:      pushedAt,
	}
}

// extractHostFromURL extracts hostname from API base URL.
// Base URL should be the API endpoint (http/https).
// Examples:
//   - "https://gitlab.polypia.net" -> "gitlab.polypia.net"
//   - "https://gitlab.polypia.net:8443" -> "gitlab.polypia.net"
//   - "https://gitlab.com/api/v4" -> "gitlab.com"
func extractHostFromURL(baseURL string) string {
	// Parse as standard URL
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}

	// Extract hostname (removes port if present)
	return u.Hostname()
}

// buildSSHURL constructs SSH URL for a project.
// Format: ssh://git@host:port/path/to/repo.git
func (p *Provider) buildSSHURL(projectPath string) string {
	if p.sshHost == "" {
		return ""
	}

	// Ensure path ends with .git
	if !strings.HasSuffix(projectPath, ".git") {
		projectPath += ".git"
	}

	// Build SSH URL
	if p.sshPort > 0 && p.sshPort != 22 {
		return fmt.Sprintf("ssh://git@%s:%d/%s", p.sshHost, p.sshPort, projectPath)
	}

	// Standard SSH URL (port 22)
	return fmt.Sprintf("git@%s:%s", p.sshHost, projectPath)
}
