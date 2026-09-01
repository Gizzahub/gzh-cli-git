// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
)

const (
	// DefaultExpiryDays is the queue age after which a branch is expired.
	DefaultExpiryDays = 7
	// QuietMaxBranches is the hook-style queue size above which --quiet exits.
	QuietMaxBranches = 20
)

// QueueOptions configures a read-only integrate queue scan.
type QueueOptions struct {
	RepoPath   string
	Base       string
	ExpiryDays int
	NoFetch    bool
	Quiet      bool
	// ControllerConfig is an explicitly selected devbox/controller file. It is
	// never discovered from ancestors and, when set, supplies the authoritative
	// remote, integration branch, and optional task-branch namespace.
	ControllerConfig string
	// ConfigValues is the declared integrationBranch list. Empty means
	// load the repo-root .gz-git.yaml, same as check/run.
	ConfigValues []string
	Now          time.Time
}

// QueueEntry is one local or remote task branch that is not the base,
// the remote HEAD, or the integration branch.
type QueueEntry struct {
	Ref        string
	Ahead      int
	Behind     int
	BaseState  string
	MergeState string
	AgeDays    int
	Note       string
	Expired    bool
}

// QueueReport is the read-only answer to "what is waiting to integrate?".
type QueueReport struct {
	Base          string
	BaseSource    string
	BaseMissing   bool
	Integration   Resolution
	Remote        string
	Entries       []QueueEntry
	StaleCount    int
	ConflictCount int
	ExpiredCount  int
	MergedCount   int
	ExpiryDays    int
	QuietSkipped  bool
}

// CollectQueue lists unfinished task branches. An empty queue is success.
// A missing base is a reportable state, not a guessed origin/master.
func CollectQueue(ctx context.Context, exec *gitcmd.Executor, opts QueueOptions) (*QueueReport, error) {
	if exec == nil {
		return nil, fmt.Errorf("git executor is nil")
	}
	dir := strings.TrimSpace(opts.RepoPath)
	if dir == "" {
		dir = "."
	}
	g := newGitRepo(exec, dir)
	if !g.isRepo(ctx) {
		return nil, fmt.Errorf("not a git repository: %s", dir)
	}
	root, err := g.toplevel(ctx)
	if err != nil {
		return nil, err
	}
	g.dir = root

	controller, err := resolveQueueController(ctx, g, root, &opts)
	if err != nil {
		return nil, err
	}

	expiry := opts.ExpiryDays
	if expiry <= 0 {
		expiry = DefaultExpiryDays
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	report := &QueueReport{ExpiryDays: expiry}
	remote, err := resolveQueueRemote(ctx, g, controller)
	if err != nil {
		return nil, err
	}
	report.Remote = remote

	noFetch := opts.NoFetch || opts.Quiet
	if !noFetch && report.Remote != "" {
		if err := g.fetchPrune(ctx, report.Remote); err != nil {
			return nil, err
		}
	}

	base, source, ok, err := resolveQueueBase(ctx, g, opts.Base, report.Remote, controller)
	if err != nil {
		return nil, err
	}
	report.Base = base
	report.BaseSource = source
	if !ok {
		report.BaseMissing = true
		return report, nil
	}

	baseSHA, ok, err := g.revParse(ctx, base)
	if err != nil {
		return nil, err
	}
	if !ok {
		report.BaseMissing = true
		return report, nil
	}

	integ, err := resolveQueueIntegration(ctx, exec, g.dir, controller, opts.ConfigValues)
	if err != nil {
		return nil, err
	}
	report.Integration = integ

	patterns := []string(nil)
	if controller != nil {
		patterns = controller.TaskPattern
	}
	refs, err := collectQueueRefs(ctx, g, report.Remote, base, integ, patterns)
	if err != nil {
		return nil, err
	}
	if opts.Quiet && len(refs) > QuietMaxBranches {
		report.QuietSkipped = true
		return report, nil
	}

	return fillQueueEntries(ctx, g, report, refs, baseSHA, expiry, now, opts.Quiet)
}

func resolveQueueController(ctx context.Context, g gitRepo, root string, opts *QueueOptions) (*controllerBinding, error) {
	if strings.TrimSpace(opts.ControllerConfig) == "" {
		if len(opts.ConfigValues) != 0 {
			return nil, nil //nolint:nilnil // no explicit controller was selected
		}
		decl, err := config.LoadRepoRootTaskPattern(root)
		if err != nil {
			return nil, err
		}
		opts.ConfigValues = decl.IntegrationBranch
		return nil, nil //nolint:nilnil // no explicit controller was selected
	}
	branch, err := g.currentBranch(ctx)
	if err != nil {
		return nil, err
	}
	controller, err := resolveController(ctx, g, opts.ControllerConfig, branch)
	if err != nil {
		return nil, err
	}
	opts.ConfigValues = append([]string(nil), controller.Integration...)
	return controller, nil
}

func resolveQueueRemote(ctx context.Context, g gitRepo, controller *controllerBinding) (string, error) {
	if controller != nil {
		return controller.Remote, nil
	}
	remotes, err := g.remotes(ctx)
	if err != nil {
		return "", err
	}
	return preferredRemote(remotes), nil
}

func resolveQueueIntegration(ctx context.Context, exec *gitcmd.Executor, repoPath string, controller *controllerBinding, configValues []string) (Resolution, error) {
	if controller != nil {
		return Resolution{Participates: true, Name: controller.Integration[0], Source: SourceConfigPrefix + "0]"}, nil
	}
	return ResolveIntegrationBranch(ctx, exec, repoPath, configValues)
}

func fillQueueEntries(ctx context.Context, g gitRepo, report *QueueReport, refs []queueRef, baseSHA string, expiry int, now time.Time, quiet bool) (*QueueReport, error) {
	for _, ref := range refs {
		entry, skip, err := inspectQueueRef(ctx, g, ref, baseSHA, expiry, now)
		if err != nil {
			return nil, err
		}
		if skip {
			continue
		}
		if quiet && entry.Note == "" {
			continue
		}
		report.Entries = append(report.Entries, entry)
		if strings.HasPrefix(entry.BaseState, "stale") {
			report.StaleCount++
		}
		if entry.MergeState == "CONFLICT" {
			report.ConflictCount++
		}
		if entry.Expired {
			report.ExpiredCount++
		}
		if entry.Ahead == 0 {
			report.MergedCount++
		}
	}
	return report, nil
}

func resolveQueueBase(ctx context.Context, g gitRepo, flagBase, remote string, controller *controllerBinding) (name, source string, ok bool, err error) {
	if controller != nil {
		declared := controller.Integration[0]
		if strings.TrimSpace(flagBase) != "" {
			if err := gitcmd.SanitizeBranchName(flagBase); err != nil {
				return flagBase, "flag", false, fmt.Errorf("invalid --base: %w", err)
			}
			accepted := []string{
				declared,
				"refs/heads/" + declared,
				remote + "/" + declared,
				"refs/remotes/" + remote + "/" + declared,
			}
			if !containsString(accepted, flagBase) {
				return flagBase, "flag", false, fmt.Errorf("--base must equal controller integration branch %s", declared)
			}
			return remote + "/" + declared, "controller-flag", true, nil
		}
		return remote + "/" + declared, "controller", true, nil
	}
	if strings.TrimSpace(flagBase) != "" {
		if err := gitcmd.SanitizeBranchName(flagBase); err != nil {
			return flagBase, "flag", false, fmt.Errorf("invalid --base: %w", err)
		}
		_, exists, err := g.revParse(ctx, flagBase)
		if err != nil {
			return flagBase, "flag", false, err
		}
		return flagBase, "flag", exists, nil
	}
	if remote == "" {
		return "", SourceNone, false, nil
	}
	short, exists, err := g.symbolicRef(ctx, "refs/remotes/"+remote+"/HEAD")
	if err != nil {
		return "", SourceNone, false, err
	}
	if !exists {
		return "", SourceNone, false, nil
	}
	return short, "remote-head", true, nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// queueRef retains the full ref identity until inspection. display preserves
// QueueEntry.Ref's historical short-ref rendering for callers.
type queueRef struct {
	full, display, branch string
	remote                bool
}

func collectQueueRefs(ctx context.Context, g gitRepo, remote, base string, integ Resolution, taskPatterns []string) ([]queueRef, error) {
	raw, err := g.refNames(ctx)
	if err != nil {
		return nil, err
	}

	exclude := queueExcludeSet(raw, remote, base, integ)

	var out []queueRef
	for _, full := range raw {
		ref, include := makeQueueRef(full, remote)
		if !include {
			continue
		}
		if _, skip := exclude[ref.full]; skip {
			continue
		}
		if len(taskPatterns) > 0 && !config.MatchesAnyTaskPattern(ref.branch, taskPatterns) {
			continue
		}
		if ref.remote {
			localSHA, localOK, err := g.revParse(ctx, "refs/heads/"+ref.branch)
			if err != nil {
				return nil, err
			}
			remoteSHA, remoteOK, err := g.revParse(ctx, ref.full)
			if err != nil {
				return nil, err
			}
			if localOK && remoteOK && localSHA == remoteSHA {
				continue
			}
		}
		out = append(out, ref)
	}
	disambiguateQueueRefDisplays(out, remote)
	sort.Slice(out, func(i, j int) bool {
		if out[i].display == out[j].display {
			return out[i].full < out[j].full
		}
		return out[i].display < out[j].display
	})
	return out, nil
}

// disambiguateQueueRefDisplays leaves the historical short names alone unless
// a local branch name is textually identical to a selected remote-tracking
// ref. In that exceptional case QueueEntry.Ref must still identify one row.
func disambiguateQueueRefDisplays(refs []queueRef, remote string) {
	counts := make(map[string]int, len(refs))
	for _, ref := range refs {
		counts[ref.display]++
	}
	for i := range refs {
		if counts[refs[i].display] < 2 {
			continue
		}
		if refs[i].remote {
			refs[i].display = "remotes/" + remote + "/" + refs[i].branch
			continue
		}
		refs[i].display = "heads/" + refs[i].branch
	}
}

func makeQueueRef(full, remote string) (queueRef, bool) {
	if branch, ok := strings.CutPrefix(full, "refs/heads/"); ok {
		return queueRef{full: full, display: branch, branch: branch}, true
	}
	prefix := "refs/remotes/" + remote + "/"
	if remote != "" && strings.HasPrefix(full, prefix) {
		branch := strings.TrimPrefix(full, prefix)
		return queueRef{full: full, display: remote + "/" + branch, branch: branch, remote: true}, true
	}
	return queueRef{}, false
}

func queueExcludeSet(refs []string, remote, base string, integ Resolution) map[string]struct{} {
	exclude := make(map[string]struct{})
	baseBranch := NormalizeName(base, []string{remote})
	if baseBranch == "" {
		baseBranch = base
	}
	// Keep an explicit full ref excluded too. This restores the legacy
	// --base refs/heads/... and refs/remotes/<remote>/... behavior while the
	// variants below cover its local/selected-remote counterpart.
	exclude[base] = struct{}{}
	exclude["refs/heads/"+baseBranch] = struct{}{}
	if remote != "" {
		exclude["refs/remotes/"+remote+"/"+baseBranch] = struct{}{}
		exclude["refs/remotes/"+remote+"/HEAD"] = struct{}{}
	}
	if !integ.Participates || integ.Name == "" {
		return exclude
	}
	exclude["refs/heads/"+integ.Name] = struct{}{}
	if remote != "" {
		exclude["refs/remotes/"+remote+"/"+integ.Name] = struct{}{}
	}
	suffix := "/" + integ.Name
	for _, ref := range refs {
		if strings.HasPrefix(ref, "refs/remotes/") && strings.HasSuffix(ref, suffix) {
			exclude[ref] = struct{}{}
		}
	}
	return exclude
}

func queueRefAllowed(ref string) bool {
	return gitcmd.SanitizeBranchName(ref) == nil
}

func inspectQueueRef(ctx context.Context, g gitRepo, ref queueRef, baseSHA string, expiryDays int, now time.Time) (QueueEntry, bool, error) {
	if !queueRefAllowed(ref.branch) {
		return QueueEntry{}, true, nil
	}
	_, ok, err := g.revParse(ctx, ref.full)
	if err != nil {
		return QueueEntry{}, false, err
	}
	if !ok {
		return QueueEntry{}, true, nil
	}

	ahead, behind, err := g.aheadBehind(ctx, ref.full, baseSHA)
	if err != nil {
		return QueueEntry{}, false, err
	}

	entry := QueueEntry{
		Ref:    ref.display,
		Ahead:  ahead,
		Behind: behind,
	}

	mb, err := g.mergeBase(ctx, ref.full, baseSHA)
	if err != nil {
		return QueueEntry{}, false, err
	}
	if mb == baseSHA {
		entry.BaseState = "current"
	} else {
		n, err := g.revCount(ctx, mb+".."+baseSHA)
		if err != nil {
			return QueueEntry{}, false, err
		}
		entry.BaseState = fmt.Sprintf("stale(%d)", n)
	}

	switch ahead {
	case 0:
		entry.MergeState = "-"
	default:
		clean, err := g.mergeTreeClean(ctx, baseSHA, ref.full)
		if err != nil {
			return QueueEntry{}, false, err
		}
		if clean {
			entry.MergeState = "clean"
		} else {
			entry.MergeState = "CONFLICT"
		}
	}

	epoch, err := g.commitTime(ctx, ref.full)
	if err != nil {
		return QueueEntry{}, false, err
	}
	age := int((now.Unix() - epoch) / 86400)
	if age < 0 {
		age = 0
	}
	entry.AgeDays = age

	expirySeconds := int64(expiryDays) * 86400
	switch {
	case ahead == 0:
		entry.Note = "merged — can delete"
	case now.Unix()-epoch > expirySeconds:
		entry.Note = fmt.Sprintf("EXPIRED (>%dd) — rebase or drop", expiryDays)
		entry.Expired = true
	case entry.BaseState != "current":
		entry.Note = "stale base — rebase needed"
	}
	return entry, false, nil
}

// FormatQueue renders the branch-status table. Quiet already filtered rows.
func FormatQueue(r *QueueReport) string {
	if r == nil {
		return ""
	}
	if r.BaseMissing {
		if r.Base != "" {
			return fmt.Sprintf("integrate queue: base ref not found: %s (source=%s)\n", r.Base, r.BaseSource)
		}
		return "integrate queue: no base ref (missing remote HEAD; pass --base)\n"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%-50s %5s %6s %-11s %-9s %6s  %s\n",
		"BRANCH", "AHEAD", "BEHIND", "BASE", "MERGE", "AGE", "NOTE")
	for _, e := range r.Entries {
		fmt.Fprintf(&b, "%-50s %5d %6d %-11s %-9s %5dd  %s\n",
			e.Ref, e.Ahead, e.Behind, e.BaseState, e.MergeState, e.AgeDays, e.Note)
	}
	fmt.Fprintf(&b, "\nbase=%s  expiry=%dd  stale=%d  conflict=%d  expired=%d  merged=%d\n",
		r.Base, r.ExpiryDays, r.StaleCount, r.ConflictCount, r.ExpiredCount, r.MergedCount)
	if r.Integration.Participates {
		fmt.Fprintf(&b, "integration=%s  source=%s\n", r.Integration.Name, r.Integration.Source)
	}
	return b.String()
}
