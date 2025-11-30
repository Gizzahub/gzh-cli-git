// Package parser provides parsers for Git command output.
// This package contains parsers for various Git commands including
// status, diff, log, and other Git operations. All parsers are designed
// to handle edge cases and provide structured output.
package parser

import (
	"fmt"
	"strings"

	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

// ParseStatus parses the output of "git status --porcelain".
// The porcelain format is designed to be easy for scripts to parse.
//
// Format:
// XY PATH
// where X = index status, Y = worktree status
//
// Status codes:
// ' ' = unmodified
// M = modified
// A = added
// D = deleted
// R = renamed
// C = copied
// U = updated but unmerged
// ? = untracked
// ! = ignored
//
// Example output:
//
//	M  README.md
//	A  newfile.go
//	?? untracked.txt
//	R  old.txt -> new.txt
func ParseStatus(output string) (*repository.Status, error) {
	status := &repository.Status{
		IsClean:        true,
		ModifiedFiles:  []string{},
		StagedFiles:    []string{},
		UntrackedFiles: []string{},
		ConflictFiles:  []string{},
		DeletedFiles:   []string{},
		RenamedFiles:   []repository.RenamedFile{},
	}

	if output == "" {
		// Empty output means clean working tree
		return status, nil
	}

	lines := SplitLines(output)
	for i, line := range lines {
		if IsEmptyLine(line) {
			continue
		}

		// Minimum length: "XY PATH" = 3 characters + space + path
		if len(line) < 4 {
			return nil, &ParseError{
				Line:    i,
				Content: line,
				Reason:  "line too short for status format",
			}
		}

		indexStatus := rune(line[0])
		worktreeStatus := rune(line[1])
		filePath := strings.TrimSpace(line[3:])

		// Handle renamed files (format: "old -> new")
		if indexStatus == 'R' || worktreeStatus == 'R' {
			parts := strings.Split(filePath, " -> ")
			if len(parts) == 2 {
				status.RenamedFiles = append(status.RenamedFiles, repository.RenamedFile{
					OldPath: strings.TrimSpace(parts[0]),
					NewPath: strings.TrimSpace(parts[1]),
				})
				status.StagedFiles = append(status.StagedFiles, parts[1])
				status.IsClean = false
				continue
			}
		}

		// Parse status codes
		if err := parseStatusCode(status, indexStatus, worktreeStatus, filePath); err != nil {
			return nil, &ParseError{
				Line:    i,
				Content: line,
				Reason:  err.Error(),
			}
		}
	}

	return status, nil
}

// parseStatusCode interprets the two-character status code.
func parseStatusCode(status *repository.Status, index, worktree rune, path string) error {
	// Index status (staged changes)
	switch index {
	case 'M': // Modified in index
		status.StagedFiles = append(status.StagedFiles, path)
		status.IsClean = false
	case 'A': // Added to index
		status.StagedFiles = append(status.StagedFiles, path)
		status.IsClean = false
	case 'D': // Deleted from index
		status.StagedFiles = append(status.StagedFiles, path)
		status.DeletedFiles = append(status.DeletedFiles, path)
		status.IsClean = false
	case 'R': // Renamed in index
		status.StagedFiles = append(status.StagedFiles, path)
		status.IsClean = false
	case 'C': // Copied in index
		status.StagedFiles = append(status.StagedFiles, path)
		status.IsClean = false
	case 'U': // Unmerged (conflict)
		status.ConflictFiles = append(status.ConflictFiles, path)
		status.IsClean = false
	case '?': // Untracked
		status.UntrackedFiles = append(status.UntrackedFiles, path)
		status.IsClean = false
	case '!': // Ignored
		// We typically don't track ignored files in status
	case ' ': // Unchanged in index
		// No action needed for index
	default:
		return fmt.Errorf("unknown index status code: %c", index)
	}

	// Worktree status (unstaged changes)
	switch worktree {
	case 'M': // Modified in worktree
		status.ModifiedFiles = append(status.ModifiedFiles, path)
		status.IsClean = false
	case 'D': // Deleted from worktree
		status.DeletedFiles = append(status.DeletedFiles, path)
		status.IsClean = false
	case 'U': // Unmerged (conflict)
		status.ConflictFiles = append(status.ConflictFiles, path)
		status.IsClean = false
	case '?': // Untracked (second character for untracked files)
		// Already handled by index status
	case ' ': // Unchanged in worktree
		// No action needed
	default:
		// Some status codes only appear in index, not worktree
		if worktree != 'A' && worktree != 'R' && worktree != 'C' {
			return fmt.Errorf("unknown worktree status code: %c", worktree)
		}
	}

	return nil
}

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
// Format: "HASH|AUTHOR|EMAIL|TIMESTAMP|SUBJECT"
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
