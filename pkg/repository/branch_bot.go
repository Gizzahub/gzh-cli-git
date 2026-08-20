// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import "strings"

// Bot branch prefixes used by Dependabot, Renovate, and github-actions.
// Matching is on the branch name after a leading remote prefix is stripped.
const (
	BotKindDependabot    = "dependabot"
	BotKindRenovate      = "renovate"
	BotKindGitHubActions = "github-actions"
)

var botBranchPrefixes = []struct {
	prefix string
	kind   string
}{
	{"dependabot/", BotKindDependabot},
	{"renovate/", BotKindRenovate},
	{"github-actions/", BotKindGitHubActions},
}

// IsBotBranch reports whether name is a leftover automation branch.
// A leading origin/ or other remote prefix is ignored so both
// dependabot/go_modules/x and origin/dependabot/go_modules/x match.
func IsBotBranch(name string) bool {
	return BotKind(name) != ""
}

// BotKind returns "dependabot", "renovate", "github-actions", or "" if name
// is not a bot branch.
func BotKind(name string) string {
	n := botMatchName(name)
	if n == "" {
		return ""
	}
	for _, bot := range botBranchPrefixes {
		if strings.HasPrefix(n, bot.prefix) {
			return bot.kind
		}
	}
	return ""
}

func hasBotPrefix(name string) bool {
	for _, bot := range botBranchPrefixes {
		if strings.HasPrefix(name, bot.prefix) {
			return true
		}
	}
	return false
}

// botMatchName strips refs/ prefixes and a leading remote segment when the
// remainder itself looks like a bot branch (origin/dependabot/x).
func botMatchName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	name = strings.TrimPrefix(name, "refs/remotes/")
	name = strings.TrimPrefix(name, "refs/heads/")
	name = strings.TrimPrefix(name, "remotes/")
	if i := strings.IndexByte(name, '/'); i > 0 {
		rest := name[i+1:]
		if hasBotPrefix(rest) {
			return rest
		}
	}
	return name
}
