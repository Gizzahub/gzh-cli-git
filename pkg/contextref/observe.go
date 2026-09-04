// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package contextref

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
)

// Observe reports the D6 four-state context-reference matrix and, when a
// CE descriptor is supplied, aggregates CE v2 gate-doctor output.
func Observe(ctx context.Context, opts Options) (Observation, error) {
	obs := baseObservation()
	if err := validateOptions(&opts); err != nil {
		return obs, err
	}
	root, err := gitWorktreeRoot(ctx, opts.Git, opts.Dir)
	if err != nil {
		obs.Context = faultComponent(err.Error())
		obs.ExitCode = cliutil.ExitToolError
		return obs, nil
	}
	obs.WorktreeRoot = root
	fsRoot, err := bindWorktreeRoot(root)
	if err != nil {
		obs.Context = unsupportedOrFault(err)
		obs.ExitCode = exitFor(obs)
		obs.CE = observeCE(ctx, opts, root)
		if obs.CE.ComponentOutcome == OutcomeFault {
			obs.ExitCode = cliutil.ExitToolError
		}
		return obs, nil
	}
	defer func() { _ = fsRoot.Close() }()

	obs.Context = observeManifest(ctx, opts.Git, fsRoot, root)
	obs.CE = observeCE(ctx, opts, root)
	obs.ExitCode = exitFor(obs)
	return obs, nil
}

func baseObservation() Observation {
	return Observation{
		Schema:           ObservationSchema,
		CapabilityID:     CapabilityID,
		ReleasedCETag:    ReleasedCETag,
		ReleasedCECommit: ReleasedCECommit,
		Context:          Component{ComponentOutcome: OutcomeAbsent, Reason: ReasonNotDeclared},
		CE:               Component{ComponentOutcome: OutcomeAbsent},
		ExitCode:         cliutil.ExitOK,
	}
}

func validateOptions(opts *Options) error {
	if opts.Dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("working directory: %w", err)
		}
		opts.Dir = wd
	}
	abs, err := filepath.Abs(opts.Dir)
	if err != nil {
		return fmt.Errorf("resolve dir: %w", err)
	}
	opts.Dir = abs
	if opts.Git == nil {
		opts.Git = gitcmd.NewExecutor()
	}
	if opts.Timeout == 0 {
		opts.Timeout = defaultCETimeout
	}
	if opts.Timeout < 0 || opts.Timeout > maxCETimeout {
		return errTimeoutRange
	}
	if opts.CE == nil {
		return nil
	}
	if !filepath.IsAbs(opts.CE.Path) {
		return errRelativeCE
	}
	if opts.CE.CapabilityID == "" {
		opts.CE.CapabilityID = CECapabilityID
	}
	if !validSHA256Digest(opts.CE.Digest) {
		return errCEDigest
	}
	return nil
}

func observeManifest(ctx context.Context, git *gitcmd.Executor, root *wtRoot, dir string) Component {
	index, tracked, err := gitIndexBlob(ctx, git, dir, ManifestFile)
	if err != nil {
		return faultComponent(err.Error())
	}
	info, present, err := worktreePresent(root, ManifestFile)
	if err != nil {
		return faultComponent(err.Error())
	}
	algo := gitObjectFormat(ctx, git, dir)
	head, err := gitHEADBlob(ctx, git, dir, ManifestFile)
	if err != nil {
		head = gitBlob{}
	}
	state := &ManifestState{
		Tracked: tracked,
		Present: present,
		HEAD:    qualifyOID(algo, head.OID),
		Index:   qualifyOID(algo, index.OID),
		GitMode: index.Mode,
	}
	switch {
	case !tracked && !present:
		return Component{ComponentOutcome: OutcomeObserved, Reason: ReasonNotDeclared, Manifest: state}
	case !tracked && present:
		return Component{ComponentOutcome: OutcomeObserved, Reason: ReasonUntracked, Manifest: state}
	case tracked && !present:
		return Component{ComponentOutcome: OutcomeObserved, Reason: ReasonMissingFile, Manifest: state}
	}
	if info.symlink {
		return Component{ComponentOutcome: OutcomeObserved, Reason: ReasonSymlink, Manifest: state}
	}
	if !info.regular {
		return Component{ComponentOutcome: OutcomeObserved, Reason: ReasonNonRegular, Manifest: state}
	}
	data, err := readRelativeBounded(root, ManifestFile, maxManifestBytes)
	if err != nil {
		return manifestReadError(state, err)
	}
	state.Size = int64(len(data))
	state.WorktreeDigest = contentDigest(data)
	if tracked && regularGitMode(index.Mode) {
		state.Dirty = gitBlobDigest(algo, data) != qualifyOID(algo, index.OID)
	}
	paths, parseErr := parseManifestYAML(data)
	if parseErr != nil {
		return Component{ComponentOutcome: OutcomeObserved, Reason: reasonFor(parseErr), Manifest: state}
	}
	state.Schema = ManifestSchema
	comp := Component{ComponentOutcome: OutcomeObserved, Manifest: state}
	comp.Entrypoints = observeEntrypoints(ctx, git, root, dir, algo, paths)
	return comp
}

func observeEntrypoints(ctx context.Context, git *gitcmd.Executor, root *wtRoot, dir, algo string, paths []string) []Entrypoint {
	out := make([]Entrypoint, 0, len(paths))
	var aggregate int
	for _, p := range paths {
		ep := Entrypoint{Path: p}
		index, inIndex, err := gitIndexBlob(ctx, git, dir, p)
		if err != nil {
			ep.Reason = DomainTransport
			out = append(out, ep)
			continue
		}
		head, headErr := gitHEADBlob(ctx, git, dir, p)
		if headErr != nil {
			head = gitBlob{}
		}
		ep.HEAD = qualifyOID(algo, head.OID)
		ep.Index = qualifyOID(algo, index.OID)
		ep.GitMode = index.Mode
		info, present, err := worktreePresent(root, p)
		if err != nil {
			ep.Reason = DomainTransport
			out = append(out, ep)
			continue
		}
		if !inIndex {
			ep.Reason = ReasonUntrackedEP
			out = append(out, ep)
			continue
		}
		if !regularGitMode(index.Mode) {
			ep.Reason = ReasonBadMode
			out = append(out, ep)
			continue
		}
		if !present {
			ep.IndexOnly = true
			ep.Reason = ReasonIndexOnly
			out = append(out, ep)
			continue
		}
		if info.symlink {
			ep.Reason = ReasonSymlink
			out = append(out, ep)
			continue
		}
		if !info.regular {
			ep.Reason = ReasonNonRegular
			out = append(out, ep)
			continue
		}
		remain := maxAggregateBytes - aggregate
		if remain > maxEntrypointBytes {
			remain = maxEntrypointBytes
		}
		if remain <= 0 {
			ep.Reason = ReasonLimit
			out = append(out, ep)
			continue
		}
		data, readErr := readRelativeBounded(root, p, remain)
		if readErr != nil {
			ep.Reason = reasonFor(readErr)
			out = append(out, ep)
			continue
		}
		aggregate += len(data)
		ep.Size = int64(len(data))
		ep.WorktreeDigest = contentDigest(data)
		if gitBlobDigest(algo, data) != qualifyOID(algo, index.OID) {
			ep.Dirty = true
			ep.Reason = ReasonDirty
		}
		out = append(out, ep)
	}
	return out
}

func manifestReadError(state *ManifestState, err error) Component {
	return Component{ComponentOutcome: OutcomeObserved, Reason: reasonFor(err), Manifest: state}
}

func reasonFor(err error) string {
	switch {
	case errors.Is(err, errLimit):
		return ReasonLimit
	case errors.Is(err, errSymlink):
		return ReasonSymlink
	case errors.Is(err, errNonRegular):
		return ReasonNonRegular
	case errors.Is(err, errPath):
		return ReasonPathInvalid
	case errors.Is(err, errChanged):
		return ReasonChanged
	case errors.Is(err, errUnsupportedOpen):
		return ReasonUnsupported
	default:
		return ReasonInvalid
	}
}

func unsupportedOrFault(err error) Component {
	if errors.Is(err, errUnsupportedOpen) {
		return Component{ComponentOutcome: OutcomeUnsupported, Reason: ReasonUnsupported}
	}
	if errors.Is(err, errSymlink) {
		return Component{ComponentOutcome: OutcomeObserved, Reason: ReasonSymlink}
	}
	return faultComponent(err.Error())
}

func faultComponent(detail string) Component {
	reason := detail
	if reason == "" {
		reason = DomainTransport
	}
	return Component{ComponentOutcome: OutcomeFault, Reason: reason, FaultDomain: DomainTransport}
}

func exitFor(obs Observation) int {
	if obs.Context.ComponentOutcome == OutcomeFault || obs.CE.ComponentOutcome == OutcomeFault {
		return cliutil.ExitToolError
	}
	return cliutil.ExitOK
}
