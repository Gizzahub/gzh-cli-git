// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package parser

import (
	"fmt"
	"strings"
)

// ParseBranchInfo parses the output of "git branch --show-current".
// Returns the current branch name, or empty string if in detached HEAD.
func ParseBranchInfo(output string) string {
	return strings.TrimSpace(output)
}

// ParseRemoteInfo parses the output of "git remote get-url <remote>".
// Returns the remote URL.
func ParseRemoteInfo(output string) string {
	return strings.TrimSpace(output)
}

// ParseUpstreamInfo parses the output of "git rev-parse --abbrev-ref @{upstream}".
// Returns the upstream branch name (e.g., "origin/main").
func ParseUpstreamInfo(output string) string {
	return strings.TrimSpace(output)
}

// ParseAheadBehind parses the output of "git rev-list --left-right --count HEAD...@{upstream}".
// Format: "AHEAD\tBEHIND"
// Example: "2\t3" means 2 commits ahead, 3 commits behind.
func ParseAheadBehind(output string) (ahead, behind int, err error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return 0, 0, nil
	}

	parts := strings.Split(output, "\t")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid ahead-behind format: %s", output)
	}

	ahead = ParseInt(parts[0])
	behind = ParseInt(parts[1])

	return ahead, behind, nil
}

// ParseCommitInfo parses basic commit information from "git log" output.
// Format: "HASH|AUTHOR|EMAIL|TIMESTAMP|SUBJECT".
//
//nolint:gocritic // tooManyResultsChecker: changing to a struct return would break callers outside this package
func ParseCommitInfo(line string) (hash, author, email, subject string, timestamp int64, err error) {
	parts := strings.Split(line, "|")
	if len(parts) < 5 {
		return "", "", "", "", 0, fmt.Errorf("invalid commit info format")
	}

	hash = strings.TrimSpace(parts[0])
	author = strings.TrimSpace(parts[1])
	email = strings.TrimSpace(parts[2])
	timestamp = int64(ParseInt(parts[3]))
	subject = strings.TrimSpace(parts[4])

	// Validate hash
	if _, err := ParseCommitHash(hash); err != nil {
		return "", "", "", "", 0, fmt.Errorf("invalid commit hash: %w", err)
	}

	return hash, author, email, subject, timestamp, nil
}

// ParseFileList parses a list of files (one per line).
// Returns a slice of file paths with whitespace trimmed.
func ParseFileList(output string) []string {
	if output == "" {
		return []string{}
	}

	lines := SplitLines(output)
	files := make([]string, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}

	return files
}

// ParseIsClean determines if a repository is clean based on status output.
// A repository is clean if there are no modified, staged, or untracked files.
func ParseIsClean(output string) bool {
	return strings.TrimSpace(output) == ""
}
