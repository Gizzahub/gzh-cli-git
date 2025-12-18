// Package repository provides Git repository operations.
package repository

// Status constants for bulk operations.
// These provide consistent status values across all bulk operations.
//
// Status Display Guidelines:
//   - Changes occurred: "N↓ fetched", "N↓ pulled", "N↑ pushed"
//   - No changes: "up-to-date" (unified across all commands)
//   - Icons: ✓ (changes), = (no changes), ✗ (error), ⚠ (warning), ⊘ (skipped)
const (
	// StatusError indicates an error occurred during the operation.
	StatusError = "error"

	// StatusSkipped indicates the repository was skipped (e.g., uncommitted changes).
	StatusSkipped = "skipped"

	// StatusUpToDate indicates the repository is already up to date (no changes needed).
	// This is the unified status for "no changes" across fetch, pull, and push.
	StatusUpToDate = "up-to-date"

	// StatusUpdated indicates the repository was successfully updated (generic).
	StatusUpdated = "updated"

	// StatusSuccess indicates the operation completed successfully (generic).
	// Prefer specific statuses (StatusFetched, StatusPulled, StatusPushed) when possible.
	StatusSuccess = "success"

	// StatusFetched indicates commits were successfully fetched from remote.
	StatusFetched = "fetched"

	// StatusPulled indicates commits were successfully pulled from remote.
	StatusPulled = "pulled"

	// StatusPushed indicates commits were successfully pushed to remote.
	StatusPushed = "pushed"

	// StatusCloned indicates the repository was successfully cloned.
	StatusCloned = "cloned"

	// StatusRebased indicates the repository was successfully rebased.
	StatusRebased = "rebased"

	// StatusReset indicates the repository was successfully reset.
	StatusReset = "reset"

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

	// StatusWouldPush indicates the operation would push (dry-run mode).
	StatusWouldPush = "would-push"

	// StatusNothingToPush is deprecated. Use StatusUpToDate instead.
	// Kept for backward compatibility.
	StatusNothingToPush = "nothing-to-push"

	// StatusClean indicates the repository working tree is clean.
	StatusClean = "clean"

	// StatusDirty indicates the repository has uncommitted changes.
	StatusDirty = "dirty"

	// StatusConflict indicates the repository has merge/rebase conflicts.
	StatusConflict = "conflict"

	// StatusRebaseInProgress indicates a rebase operation is in progress.
	StatusRebaseInProgress = "rebase-in-progress"

	// StatusMergeInProgress indicates a merge operation is in progress.
	StatusMergeInProgress = "merge-in-progress"
)

// IsSuccessStatus returns true if the status indicates a successful operation.
func IsSuccessStatus(status string) bool {
	switch status {
	case StatusUpdated, StatusSuccess, StatusUpToDate,
		StatusFetched, StatusPulled, StatusPushed,
		StatusCloned, StatusRebased, StatusReset:
		return true
	default:
		return false
	}
}

// IsDryRunStatus returns true if the status indicates a dry-run simulation.
func IsDryRunStatus(status string) bool {
	switch status {
	case StatusWouldUpdate, StatusWouldFetch, StatusWouldPull, StatusWouldPush:
		return true
	default:
		return false
	}
}

// IsErrorStatus returns true if the status indicates an error.
func IsErrorStatus(status string) bool {
	return status == StatusError
}
