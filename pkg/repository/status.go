// Package repository provides Git repository operations.
package repository

// Status constants for bulk operations.
// These provide consistent status values across all bulk operations.
const (
	// StatusError indicates an error occurred during the operation.
	StatusError = "error"

	// StatusSkipped indicates the repository was skipped (e.g., uncommitted changes).
	StatusSkipped = "skipped"

	// StatusUpToDate indicates the repository is already up to date.
	StatusUpToDate = "up-to-date"

	// StatusUpdated indicates the repository was successfully updated.
	StatusUpdated = "updated"

	// StatusSuccess indicates the operation completed successfully.
	StatusSuccess = "success"

	// StatusNoRemote indicates no remote is configured for the repository.
	StatusNoRemote = "no-remote"

	// StatusNoUpstream indicates no upstream branch is configured.
	StatusNoUpstream = "no-upstream"

	// StatusWouldUpdate indicates the operation would update (dry-run mode).
	StatusWouldUpdate = "would-update"

	// StatusWouldFetch indicates the operation would fetch (dry-run mode).
	StatusWouldFetch = "would-fetch"

	// StatusWouldPull indicates the operation would pull (dry-run mode).
	StatusWouldPull = "would-pull"
)

// IsSuccessStatus returns true if the status indicates a successful operation.
func IsSuccessStatus(status string) bool {
	switch status {
	case StatusUpdated, StatusSuccess, StatusUpToDate:
		return true
	default:
		return false
	}
}

// IsDryRunStatus returns true if the status indicates a dry-run simulation.
func IsDryRunStatus(status string) bool {
	switch status {
	case StatusWouldUpdate, StatusWouldFetch, StatusWouldPull:
		return true
	default:
		return false
	}
}

// IsErrorStatus returns true if the status indicates an error.
func IsErrorStatus(status string) bool {
	return status == StatusError
}
