// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

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

	// StatusBaseBlocked indicates the repository's own branch updated cleanly
	// but its local base ref diverged from the remote and was left untouched.
	// It is a distinct status rather than a note on a success because a base ref
	// holding commits the remote has never seen is exactly the state a user
	// needs to look at, and a row that renders as "up-to-date" is not looked at.
	StatusBaseBlocked = "base-blocked"

	// StatusBaseSynced indicates a local base ref was advanced to its remote.
	// Moving a ref is a write to the user's repository, and a write reported as
	// "up-to-date" is a write the user never sees. This status exists so the row
	// survives the renderer's issue filter and can say which ref moved and by how
	// much, rather than repairing refs invisibly.
	StatusBaseSynced = "base-synced"

	// StatusWouldUpdate indicates the operation would update (dry-run mode).
	StatusWouldUpdate = "would-update"

	// StatusWouldFetch indicates the operation would fetch (dry-run mode).
	StatusWouldFetch = "would-fetch"

	// StatusWouldPull indicates the operation would pull (dry-run mode).
	StatusWouldPull = "would-pull"

	// StatusWouldPush indicates the operation would push (dry-run mode).
	StatusWouldPush = "would-push"

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

	// StatusSwitched indicates the branch was successfully switched.
	StatusSwitched = "switched"

	// StatusAlreadyOnBranch indicates the repository is already on the target branch.
	StatusAlreadyOnBranch = "already-on-branch"

	// StatusBranchCreated indicates a new branch was created and switched to.
	StatusBranchCreated = "branch-created"

	// StatusWouldSwitch indicates the operation would switch (dry-run mode).
	StatusWouldSwitch = "would-switch"

	// StatusBranchNotFound indicates the target branch was not found.
	StatusBranchNotFound = "branch-not-found"

	// StatusAuthRequired indicates the operation failed due to authentication requirements.
	// This typically occurs when HTTPS credentials are not configured or have expired.
	StatusAuthRequired = "auth-required"

	// StatusBlocked indicates the operation was refused by a configured policy
	// rather than by git or the remote.
	StatusBlocked = "blocked"

	// StatusCleaned indicates untracked/ignored files were removed.
	StatusCleaned = "cleaned"

	// StatusWouldClean indicates files would be removed (dry-run mode).
	StatusWouldClean = "would-clean"

	// StatusNothingToClean indicates no untracked/ignored files to remove.
	StatusNothingToClean = "nothing-to-clean"
)

// IsSuccessStatus returns true if the status indicates a successful operation.
func IsSuccessStatus(status string) bool {
	switch status {
	case StatusUpdated, StatusSuccess, StatusUpToDate,
		StatusFetched, StatusPulled, StatusPushed,
		StatusCloned, StatusRebased, StatusReset,
		StatusSwitched, StatusAlreadyOnBranch, StatusBranchCreated,
		StatusCleaned, StatusNothingToClean, StatusBaseSynced:
		return true
	default:
		return false
	}
}

// IsDryRunStatus returns true if the status indicates a dry-run simulation.
func IsDryRunStatus(status string) bool {
	switch status {
	case StatusWouldUpdate, StatusWouldFetch, StatusWouldPull, StatusWouldPush, StatusWouldSwitch, StatusWouldClean:
		return true
	default:
		return false
	}
}

// IsErrorStatus returns true if the status indicates an error.
func IsErrorStatus(status string) bool {
	return status == StatusError
}
