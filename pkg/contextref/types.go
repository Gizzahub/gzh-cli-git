// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package contextref

import (
	"encoding/json"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
)

// Capability and schema identifiers allocated by TASK-168 / D6 W7.
const (
	CapabilityID       = "context-reference-observe-v1"
	ObservationSchema  = "gz-git.context-observation/v1"
	ManifestSchema     = "gz-git.context-reference/v1"
	ManifestFile       = ".gz-git-context.yaml"
	CECapabilityID     = "ce.task.gate-doctor/v2"
	CEDoctorSchema     = 2
	ReleasedCETag      = "v0.8.3"
	ReleasedCECommit   = "ac7445978423df45cb77ffaea0e34f7725e744b2"
	OutcomeObserved    = "observed"
	OutcomeAbsent      = "absent"
	OutcomeUnsupported = "unsupported"
	OutcomeFault       = "fault"
	DomainTransport    = "gz-git-transport"
	DomainCEInvocation = "ce-invocation"
	ReasonNotDeclared  = "context.not-declared"
	ReasonUntracked    = "context.manifest-untracked"
	ReasonMissingFile  = "context.manifest-not-materialized"
	ReasonUnsupported  = "context.unsupported-platform"
	ReasonInvalid      = "context.manifest-invalid"
	ReasonLimit        = "context.limit-exceeded"
	ReasonPathInvalid  = "context.path-invalid"
	ReasonSymlink      = "context.symlink"
	ReasonNonRegular   = "context.non-regular"
	ReasonDuplicate    = "context.duplicate-path"
	ReasonIndexOnly    = "index-only"
	ReasonDirty        = "dirty"
	ReasonChanged      = "context.changed"
	ReasonUntrackedEP  = "context.untracked-path"
	ReasonBadMode      = "context.non-blob-mode"
)

const (
	maxManifestBytes   = 64 * 1024
	maxEntrypoints     = 32
	maxEntrypointBytes = 1 * 1024 * 1024
	maxAggregateBytes  = 4 * 1024 * 1024
	maxCEStreamBytes   = 1 * 1024 * 1024
	defaultCETimeout   = 10 * time.Second
	maxCETimeout       = 30 * time.Second
)

// Options configure a single-repository observation.
type Options struct {
	Dir     string
	Git     *gitcmd.Executor
	CE      *CEDescriptor
	Timeout time.Duration
}

// CEDescriptor is a trusted CE installation. Path must be absolute. Digest
// is sha256:<64 lowercase hex> of the regular non-symlink executable.
type CEDescriptor struct {
	Path         string
	Digest       string
	CapabilityID string
}

// Observation is the gz-git.context-observation/v1 envelope.
type Observation struct {
	Schema           string    `json:"schema"`
	CapabilityID     string    `json:"capability_id"`
	ReleasedCETag    string    `json:"released_ce_tag"`
	ReleasedCECommit string    `json:"released_ce_commit"`
	WorktreeRoot     string    `json:"worktree_root"`
	Context          Component `json:"context"`
	CE               Component `json:"ce"`
	ExitCode         int       `json:"exit_code"`
}

// Component is one aggregation result. ComponentOutcome describes transport
// only; repository state is carried by reason codes.
type Component struct {
	ComponentOutcome string          `json:"component_outcome"`
	Reason           string          `json:"reason,omitempty"`
	FaultDomain      string          `json:"fault_domain,omitempty"`
	ProviderStatus   string          `json:"provider_status,omitempty"`
	Manifest         *ManifestState  `json:"manifest,omitempty"`
	Entrypoints      []Entrypoint    `json:"entrypoints,omitempty"`
	Report           json.RawMessage `json:"report,omitempty"`
}

// ManifestState is the four-state matrix plus identities.
type ManifestState struct {
	Schema         string `json:"schema,omitempty"`
	Tracked        bool   `json:"tracked"`
	Present        bool   `json:"present"`
	HEAD           string `json:"head,omitempty"`
	Index          string `json:"index,omitempty"`
	WorktreeDigest string `json:"worktree_digest,omitempty"`
	Dirty          bool   `json:"dirty"`
	GitMode        string `json:"git_mode,omitempty"`
	Size           int64  `json:"size,omitempty"`
}

// Entrypoint is one context.entrypoints observation. Worktree bytes are
// never emitted.
type Entrypoint struct {
	Path           string `json:"path"`
	Reason         string `json:"reason,omitempty"`
	HEAD           string `json:"head,omitempty"`
	Index          string `json:"index,omitempty"`
	WorktreeDigest string `json:"worktree_digest,omitempty"`
	Dirty          bool   `json:"dirty"`
	IndexOnly      bool   `json:"index_only,omitempty"`
	GitMode        string `json:"git_mode,omitempty"`
	Size           int64  `json:"size,omitempty"`
}

type manifestDocument struct {
	Schema  string `yaml:"schema"`
	Context struct {
		Entrypoints []struct {
			Path string `yaml:"path"`
		} `yaml:"entrypoints"`
	} `yaml:"context"`
}

type gitBlob struct {
	Mode string
	OID  string
	Algo string
}

type fileIdent struct {
	size  int64
	mtime int64
	ctime int64
	dev   uint64
	ino   uint64
}
