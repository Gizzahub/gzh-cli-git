// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package scanner

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/provider"
)

// GitRepoScanner scans directories for git repositories.
type GitRepoScanner struct {
	RootPath         string
	MaxDepth         int
	RespectGitIgnore bool
	ExcludePatterns  []string
	IncludePatterns  []string
}

// ScannedRepo represents a discovered git repository.
type ScannedRepo struct {
	Name    string
	Path    string
	Remotes map[string]string // Remote name -> URL (e.g., "origin" -> "git@github.com:user/repo.git")
	Depth   int               // Depth from root
	Branch  string            // Current branch name (empty if detached or unknown)
}

// Scan discovers all git repositories under RootPath.
func (s *GitRepoScanner) Scan(ctx context.Context) ([]*ScannedRepo, error) {
	absRoot, err := filepath.Abs(s.RootPath)
	if err != nil {
		return nil, fmt.Errorf("resolve root path: %w", err)
	}

	var repos []*ScannedRepo

	// Load .gitignore patterns if needed
	var gitignorePatterns []string
	if s.RespectGitIgnore {
		gitignorePatterns = s.loadGitIgnorePatterns(absRoot)
	}

	err = s.scanDir(ctx, absRoot, absRoot, 0, gitignorePatterns, &repos)
	if err != nil {
		return nil, err
	}

	return repos, nil
}

func (s *GitRepoScanner) scanDir(ctx context.Context, root, current string, depth int, gitignorePatterns []string, repos *[]*ScannedRepo) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Check depth limit
	if depth > s.MaxDepth {
		return nil
	}

	// Check if current directory is a git repo
	gitDir := filepath.Join(current, ".git")
	if isDir(gitDir) {
		// Found a git repository
		repo := s.analyzeRepo(current, depth)
		*repos = append(*repos, repo)
	}

	// Scan subdirectories
	entries, err := os.ReadDir(current)
	if err != nil {
		return fmt.Errorf("read directory %s: %w", current, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		subPath := filepath.Join(current, name)

		// Skip .git directories
		if name == ".git" {
			continue
		}

		// Check exclusion patterns
		if s.shouldExclude(subPath, root, gitignorePatterns) {
			continue
		}

		// Check inclusion patterns (override exclusion)
		if s.shouldInclude(subPath, root) {
			// Force include
		} else if s.shouldExclude(subPath, root, gitignorePatterns) {
			continue
		}

		// Recursively scan subdirectory
		err = s.scanDir(ctx, root, subPath, depth+1, gitignorePatterns, repos)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *GitRepoScanner) analyzeRepo(repoPath string, depth int) *ScannedRepo {
	name := filepath.Base(repoPath)

	// Get remotes (name -> URL map)
	remotes, err := s.getRemotes(repoPath)
	if err != nil {
		// Don't fail, just use empty remotes
		remotes = make(map[string]string)
	}

	branch := s.getCurrentBranch(repoPath)

	return &ScannedRepo{
		Name:    name,
		Path:    repoPath,
		Remotes: remotes,
		Depth:   depth,
		Branch:  branch,
	}
}

func (s *GitRepoScanner) getRemotes(repoPath string) (map[string]string, error) {
	gitConfig := filepath.Join(repoPath, ".git", "config")

	file, err := os.Open(gitConfig)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	remotes := make(map[string]string)
	var currentRemote string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Check for remote section: [remote "origin"]
		if strings.HasPrefix(line, "[remote \"") && strings.HasSuffix(line, "\"]") {
			// Extract remote name
			currentRemote = strings.TrimSuffix(strings.TrimPrefix(line, "[remote \""), "\"]")
			continue
		}

		// End of section
		if strings.HasPrefix(line, "[") {
			currentRemote = ""
			continue
		}

		// Parse URL in remote section
		if currentRemote != "" && strings.HasPrefix(line, "url = ") {
			url := strings.TrimPrefix(line, "url = ")
			remotes[currentRemote] = url
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return remotes, nil
}

func (s *GitRepoScanner) getCurrentBranch(repoPath string) string {
	headPath := filepath.Join(repoPath, ".git", "HEAD")
	data, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}

	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "ref: ") {
		return ""
	}

	ref := strings.TrimPrefix(line, "ref: ")
	if after, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
		return after
	}

	return ""
}

func (s *GitRepoScanner) loadGitIgnorePatterns(root string) []string {
	gitignorePath := filepath.Join(root, ".gitignore")

	file, err := os.Open(gitignorePath)
	if err != nil {
		return nil
	}
	defer file.Close()

	var patterns []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		patterns = append(patterns, line)
	}

	return patterns
}

func (s *GitRepoScanner) shouldExclude(path, root string, gitignorePatterns []string) bool {
	// Get relative path from root
	relPath, err := filepath.Rel(root, path)
	if err != nil {
		relPath = path
	}

	// Check user-defined exclude patterns
	for _, pattern := range s.ExcludePatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}

		// Simple pattern matching
		if matchesPattern(relPath, pattern) {
			return true
		}
	}

	// Check .gitignore patterns if enabled
	if s.RespectGitIgnore {
		for _, pattern := range gitignorePatterns {
			if matchesPattern(relPath, pattern) {
				return true
			}
		}
	}

	return false
}

func (s *GitRepoScanner) shouldInclude(path, root string) bool {
	if len(s.IncludePatterns) == 0 {
		return false
	}

	relPath, err := filepath.Rel(root, path)
	if err != nil {
		relPath = path
	}

	for _, pattern := range s.IncludePatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}

		if matchesPattern(relPath, pattern) {
			return true
		}
	}

	return false
}

// matchesPattern performs simple glob-like pattern matching.
func matchesPattern(path, pattern string) bool {
	// Remove leading/trailing slashes
	path = strings.Trim(path, "/")
	pattern = strings.Trim(pattern, "/")

	// Exact match
	if path == pattern {
		return true
	}

	// Directory prefix match (pattern/)
	if before, ok := strings.CutSuffix(pattern, "/"); ok {
		dirPattern := before
		if strings.HasPrefix(path, dirPattern+"/") || path == dirPattern {
			return true
		}
	}

	// Wildcard match (pattern/*)
	if before, ok := strings.CutSuffix(pattern, "/*"); ok {
		dirPattern := before
		if strings.HasPrefix(path, dirPattern+"/") {
			return true
		}
	}

	// Simple glob match
	matched, err := filepath.Match(pattern, filepath.Base(path))
	return err == nil && matched
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// ToRepositories converts ScannedRepos to provider.Repository format.
func ToRepositories(scanned []*ScannedRepo) []*provider.Repository {
	repos := make([]*provider.Repository, 0, len(scanned))

	for _, s := range scanned {
		repo := &provider.Repository{
			Name:     s.Name,
			FullName: s.Name,
			CloneURL: "",
		}

		// Use origin remote as primary, fallback to first available remote
		if url, ok := s.Remotes["origin"]; ok {
			repo.CloneURL = url
		} else if len(s.Remotes) > 0 {
			// Use first available remote
			for _, url := range s.Remotes {
				repo.CloneURL = url
				break
			}
		}

		repos = append(repos, repo)
	}

	return repos
}
