package history

import "errors"

var (
	// ErrEmptyHistory indicates repository has no commit history
	ErrEmptyHistory = errors.New("repository has no commit history")

	// ErrInvalidDateRange indicates invalid date range (since > until)
	ErrInvalidDateRange = errors.New("invalid date range (since > until)")

	// ErrFileNotFound indicates file not found in repository history
	ErrFileNotFound = errors.New("file not found in repository history")

	// ErrInvalidFormat indicates invalid output format
	ErrInvalidFormat = errors.New("invalid output format")

	// ErrNoContributors indicates no contributors found
	ErrNoContributors = errors.New("no contributors found")

	// ErrInvalidAuthor indicates invalid author name or email
	ErrInvalidAuthor = errors.New("invalid author name or email")

	// ErrInvalidBranch indicates invalid branch name
	ErrInvalidBranch = errors.New("invalid branch name")
)
