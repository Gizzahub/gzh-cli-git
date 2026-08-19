// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package provider

import (
	"fmt"
	"net/url"
	"strings"
)

// ForgeRemote is a parsed git remote pointing at a forge.
type ForgeRemote struct {
	Provider string // github, gitlab, gitea, or empty if unknown
	Host     string
	Owner    string
	Repo     string
	BaseURL  string // empty for github.com / gitlab.com defaults
}

// ParseForgeRemote extracts provider, owner, and repo from a git remote URL.
func ParseForgeRemote(raw string) (ForgeRemote, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ForgeRemote{}, fmt.Errorf("empty remote URL")
	}

	host, path, err := splitRemote(raw)
	if err != nil {
		return ForgeRemote{}, err
	}
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")
	if path == "" {
		return ForgeRemote{}, fmt.Errorf("remote URL %q has no repository path", raw)
	}
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return ForgeRemote{}, fmt.Errorf("remote URL %q is not owner/repo", raw)
	}
	repo := parts[len(parts)-1]
	owner := strings.Join(parts[:len(parts)-1], "/")
	if repo == "" || owner == "" {
		return ForgeRemote{}, fmt.Errorf("remote URL %q is not owner/repo", raw)
	}

	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if h, _, ok := strings.Cut(host, ":"); ok && !strings.Contains(h, "/") {
		// host:port already split in splitRemote for scp; keep hostname only
		if !strings.Contains(host, "/") {
			host = h
		}
	}

	out := ForgeRemote{
		Host:     host,
		Owner:    owner,
		Repo:     repo,
		Provider: classifyForgeHost(host),
	}
	if out.Provider != "github" || host != "github.com" {
		if out.Provider == "gitlab" && host == "gitlab.com" {
			return out, nil
		}
		if host != "github.com" && host != "gitlab.com" {
			out.BaseURL = "https://" + host
		}
	}
	return out, nil
}

func classifyForgeHost(host string) string {
	switch {
	case host == "github.com" || strings.HasPrefix(host, "github.") || strings.HasSuffix(host, ".ghe.com"):
		return "github"
	case host == "gitlab.com" || strings.Contains(host, "gitlab"):
		return "gitlab"
	case host == "gitea.com" || host == "codeberg.org" || strings.Contains(host, "gitea"):
		return "gitea"
	default:
		return ""
	}
}

func splitRemote(raw string) (host, path string, err error) {
	if strings.Contains(raw, "://") {
		u, perr := url.Parse(raw)
		if perr != nil {
			return "", "", fmt.Errorf("parse remote URL: %w", perr)
		}
		if u.Host == "" || u.Path == "" {
			return "", "", fmt.Errorf("remote URL %q missing host or path", raw)
		}
		return u.Hostname(), u.Path, nil
	}
	// scp-like git@host:path
	if strings.HasPrefix(raw, "git@") || strings.Count(raw, ":") == 1 && !strings.HasPrefix(raw, "/") {
		at := strings.Index(raw, "@")
		colon := strings.LastIndex(raw, ":")
		if colon < 0 || colon < at {
			return "", "", fmt.Errorf("unsupported remote URL %q", raw)
		}
		host = raw[at+1 : colon]
		if at < 0 {
			host = raw[:colon]
		}
		return host, raw[colon+1:], nil
	}
	return "", "", fmt.Errorf("unsupported remote URL %q", raw)
}
