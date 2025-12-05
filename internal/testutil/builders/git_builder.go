// Package builders provides fluent test fixture builders for git operations.
package builders

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// GitRepoBuilder builds test git repositories with fluent interface.
type GitRepoBuilder struct {
	t          *testing.T
	dir        string
	branches   []string
	files      map[string]string
	commits    []string
	remotes    map[string]string
	configUser bool
}

// NewGitRepoBuilder creates a new GitRepoBuilder.
func NewGitRepoBuilder(t *testing.T) *GitRepoBuilder {
	t.Helper()
	return &GitRepoBuilder{
		t:          t,
		dir:        t.TempDir(),
		branches:   []string{},
		files:      make(map[string]string),
		commits:    []string{},
		remotes:    make(map[string]string),
		configUser: true,
	}
}

// WithFile adds a file to be created in the repository.
func (b *GitRepoBuilder) WithFile(name, content string) *GitRepoBuilder {
	b.files[name] = content
	return b
}

// WithBranch adds a branch to be created.
func (b *GitRepoBuilder) WithBranch(name string) *GitRepoBuilder {
	b.branches = append(b.branches, name)
	return b
}

// WithCommit adds a commit message (will create empty commit).
func (b *GitRepoBuilder) WithCommit(message string) *GitRepoBuilder {
	b.commits = append(b.commits, message)
	return b
}

// WithRemote adds a remote to be configured.
func (b *GitRepoBuilder) WithRemote(name, url string) *GitRepoBuilder {
	b.remotes[name] = url
	return b
}

// WithoutUserConfig skips user configuration.
func (b *GitRepoBuilder) WithoutUserConfig() *GitRepoBuilder {
	b.configUser = false
	return b
}

// Build creates the git repository and returns its path.
func (b *GitRepoBuilder) Build() string {
	b.t.Helper()

	// Initialize git repo.
	b.runGit("init")

	// Configure user if needed.
	if b.configUser {
		b.runGit("config", "user.email", "test@test.com")
		b.runGit("config", "user.name", "Test User")
	}

	// Create files.
	for name, content := range b.files {
		path := filepath.Join(b.dir, name)
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.t.Fatalf("failed to create dir %s: %v", dir, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			b.t.Fatalf("failed to write file %s: %v", name, err)
		}
	}

	// Add and commit files if any.
	if len(b.files) > 0 {
		b.runGit("add", ".")
		b.runGit("commit", "-m", "Initial commit")
	}

	// Create additional commits.
	for _, msg := range b.commits {
		b.runGit("commit", "--allow-empty", "-m", msg)
	}

	// Create branches.
	for _, branch := range b.branches {
		b.runGit("branch", branch)
	}

	// Add remotes.
	for name, url := range b.remotes {
		b.runGit("remote", "add", name, url)
	}

	return b.dir
}

func (b *GitRepoBuilder) runGit(args ...string) {
	b.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = b.dir
	if output, err := cmd.CombinedOutput(); err != nil {
		b.t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

// CloneOptionsBuilder builds clone operation options.
type CloneOptionsBuilder struct {
	url      string
	path     string
	branch   string
	depth    int
	bare     bool
	mirror   bool
	progress bool
}

// NewCloneOptionsBuilder creates a new CloneOptionsBuilder.
func NewCloneOptionsBuilder() *CloneOptionsBuilder {
	return &CloneOptionsBuilder{
		depth:    0,
		progress: true,
	}
}

// WithURL sets the repository URL.
func (b *CloneOptionsBuilder) WithURL(url string) *CloneOptionsBuilder {
	b.url = url
	return b
}

// WithPath sets the destination path.
func (b *CloneOptionsBuilder) WithPath(path string) *CloneOptionsBuilder {
	b.path = path
	return b
}

// WithBranch sets the branch to clone.
func (b *CloneOptionsBuilder) WithBranch(branch string) *CloneOptionsBuilder {
	b.branch = branch
	return b
}

// WithDepth sets the clone depth.
func (b *CloneOptionsBuilder) WithDepth(depth int) *CloneOptionsBuilder {
	b.depth = depth
	return b
}

// AsBare sets bare clone mode.
func (b *CloneOptionsBuilder) AsBare() *CloneOptionsBuilder {
	b.bare = true
	return b
}

// AsMirror sets mirror clone mode.
func (b *CloneOptionsBuilder) AsMirror() *CloneOptionsBuilder {
	b.mirror = true
	return b
}

// WithoutProgress disables progress output.
func (b *CloneOptionsBuilder) WithoutProgress() *CloneOptionsBuilder {
	b.progress = false
	return b
}

// Build returns the clone options as a map.
func (b *CloneOptionsBuilder) Build() map[string]interface{} {
	return map[string]interface{}{
		"url":      b.url,
		"path":     b.path,
		"branch":   b.branch,
		"depth":    b.depth,
		"bare":     b.bare,
		"mirror":   b.mirror,
		"progress": b.progress,
	}
}

// BuildArgs returns the git clone arguments.
func (b *CloneOptionsBuilder) BuildArgs() []string {
	args := []string{"clone"}

	if b.branch != "" {
		args = append(args, "-b", b.branch)
	}
	if b.depth > 0 {
		args = append(args, "--depth", string(rune(b.depth)))
	}
	if b.bare {
		args = append(args, "--bare")
	}
	if b.mirror {
		args = append(args, "--mirror")
	}
	if !b.progress {
		args = append(args, "--quiet")
	}

	args = append(args, b.url)
	if b.path != "" {
		args = append(args, b.path)
	}

	return args
}
