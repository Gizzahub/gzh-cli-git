package repository

// This file contains additional type definitions that support the repository package.
// Types in this file are supplementary to the main interfaces defined in interfaces.go

// Default values for bulk operations
const (
	// DefaultBulkParallel is the default number of parallel workers
	DefaultBulkParallel = 5

	// DefaultBulkMaxDepth is the default maximum directory depth to scan
	// maxDepth=1 scans only the root directory (depth 0)
	// maxDepth=2 scans root + immediate children (depth 0, 1)
	// Default is 2 to find repositories in immediate subdirectories
	DefaultBulkMaxDepth = 2
)

// OperationType represents the type of Git operation being performed.
type OperationType string

const (
	// OperationClone represents a clone operation.
	OperationClone OperationType = "clone"

	// OperationPull represents a pull operation.
	OperationPull OperationType = "pull"

	// OperationFetch represents a fetch operation.
	OperationFetch OperationType = "fetch"

	// OperationPush represents a push operation.
	OperationPush OperationType = "push"

	// OperationReset represents a reset operation.
	OperationReset OperationType = "reset"

	// OperationStatus represents a status operation.
	OperationStatus OperationType = "status"
)

// String returns the string representation of the operation type.
func (o OperationType) String() string {
	return string(o)
}

// FileStatus represents the status of a file in the working tree.
type FileStatus string

const (
	// FileStatusUnmodified indicates the file is unchanged.
	FileStatusUnmodified FileStatus = " "

	// FileStatusModified indicates the file has been modified.
	FileStatusModified FileStatus = "M"

	// FileStatusAdded indicates the file has been added.
	FileStatusAdded FileStatus = "A"

	// FileStatusDeleted indicates the file has been deleted.
	FileStatusDeleted FileStatus = "D"

	// FileStatusRenamed indicates the file has been renamed.
	FileStatusRenamed FileStatus = "R"

	// FileStatusCopied indicates the file has been copied.
	FileStatusCopied FileStatus = "C"

	// FileStatusUntracked indicates the file is untracked.
	FileStatusUntracked FileStatus = "?"

	// FileStatusIgnored indicates the file is ignored.
	FileStatusIgnored FileStatus = "!"

	// FileStatusConflict indicates the file has a merge conflict.
	FileStatusConflict FileStatus = "U"
)

// String returns the string representation of the file status.
func (f FileStatus) String() string {
	return string(f)
}

// RemoteType represents the type of Git remote.
type RemoteType string

const (
	// RemoteTypeHTTPS represents an HTTPS remote.
	RemoteTypeHTTPS RemoteType = "https"

	// RemoteTypeSSH represents an SSH remote.
	RemoteTypeSSH RemoteType = "ssh"

	// RemoteTypeGit represents a git:// protocol remote.
	RemoteTypeGit RemoteType = "git"

	// RemoteTypeFile represents a local file path remote.
	RemoteTypeFile RemoteType = "file"
)

// String returns the string representation of the remote type.
func (r RemoteType) String() string {
	return string(r)
}

// Ref represents a Git reference (branch, tag, or commit).
type Ref struct {
	// Name is the reference name (e.g., "refs/heads/main", "main").
	Name string

	// Hash is the commit hash this reference points to.
	Hash string

	// Type is the reference type (branch, tag, remote).
	Type RefType
}

// RefType represents the type of Git reference.
type RefType string

const (
	// RefTypeBranch represents a local branch reference.
	RefTypeBranch RefType = "branch"

	// RefTypeRemoteBranch represents a remote branch reference.
	RefTypeRemoteBranch RefType = "remote-branch"

	// RefTypeTag represents a tag reference.
	RefTypeTag RefType = "tag"

	// RefTypeCommit represents a direct commit reference.
	RefTypeCommit RefType = "commit"
)

// String returns the string representation of the reference type.
func (r RefType) String() string {
	return string(r)
}

// Remote represents a Git remote configuration.
type Remote struct {
	// Name is the remote name (e.g., "origin").
	Name string

	// URL is the remote URL.
	URL string

	// Type is the remote type (https, ssh, git, file).
	Type RemoteType

	// FetchRefs are the fetch refspecs.
	FetchRefs []string

	// PushRefs are the push refspecs.
	PushRefs []string
}

// Config represents repository configuration.
type Config struct {
	// User is the user configuration.
	User UserConfig

	// Core is the core Git configuration.
	Core CoreConfig

	// Remote contains remote configurations.
	Remote map[string]Remote

	// Branch contains branch configurations.
	Branch map[string]BranchConfig
}

// UserConfig represents user-related Git configuration.
type UserConfig struct {
	// Name is the user's name.
	Name string

	// Email is the user's email.
	Email string

	// SigningKey is the GPG signing key.
	SigningKey string
}

// CoreConfig represents core Git configuration.
type CoreConfig struct {
	// RepositoryFormatVersion is the repository format version.
	RepositoryFormatVersion int

	// FileMode indicates if file mode is tracked.
	FileMode bool

	// Bare indicates if this is a bare repository.
	Bare bool

	// LogAllRefUpdates indicates if all ref updates are logged.
	LogAllRefUpdates bool

	// IgnoreCase indicates if file names are case-insensitive.
	IgnoreCase bool

	// PrecomposeUnicode indicates if Unicode is precomposed.
	PrecomposeUnicode bool
}

// BranchConfig represents branch-specific configuration.
type BranchConfig struct {
	// Remote is the default remote for this branch.
	Remote string

	// Merge is the default merge ref for this branch.
	Merge string

	// Rebase indicates if rebase should be used instead of merge.
	Rebase bool
}

// ValidationError represents an input validation error.
type ValidationError struct {
	// Field is the field that failed validation.
	Field string

	// Value is the invalid value.
	Value string

	// Reason describes why the value is invalid.
	Reason string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return "validation error: " + e.Field + " (" + e.Value + "): " + e.Reason
}

// Is implements error comparison.
func (e *ValidationError) Is(target error) bool {
	_, ok := target.(*ValidationError)
	return ok
}
