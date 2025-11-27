package branch

import "errors"

// Common errors for branch operations.
var (
	// ErrBranchExists indicates the branch already exists.
	ErrBranchExists = errors.New("branch already exists")

	// ErrBranchNotFound indicates the branch doesn't exist.
	ErrBranchNotFound = errors.New("branch not found")

	// ErrInvalidName indicates invalid branch name.
	ErrInvalidName = errors.New("invalid branch name")

	// ErrInvalidRef indicates invalid starting ref.
	ErrInvalidRef = errors.New("invalid starting ref")

	// ErrProtectedBranch indicates operation on protected branch.
	ErrProtectedBranch = errors.New("cannot modify protected branch")

	// ErrBranchUnmerged indicates branch has unmerged changes.
	ErrBranchUnmerged = errors.New("branch has unmerged changes")

	// ErrBranchIsHead indicates branch is currently checked out.
	ErrBranchIsHead = errors.New("cannot delete currently checked out branch")

	// ErrDetachedHead indicates repository is in detached HEAD state.
	ErrDetachedHead = errors.New("repository in detached HEAD state")

	// ErrUpstreamNotSet indicates upstream branch is not configured.
	ErrUpstreamNotSet = errors.New("upstream branch not set")

	// ErrRemoteNotFound indicates remote doesn't exist.
	ErrRemoteNotFound = errors.New("remote not found")

	// ErrOperationCancelled indicates user cancelled the operation.
	ErrOperationCancelled = errors.New("operation cancelled by user")
)
