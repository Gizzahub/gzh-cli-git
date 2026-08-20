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

	if len(opts.ConfigValues) == 0 {
		decl, err := config.LoadRepoRootTaskPattern(root)
		if err != nil {
			return nil, err
		}
		opts.ConfigValues = decl.IntegrationBranch
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
	remotes, err := g.remotes(ctx)
	if err != nil {
		return nil, err
	}
	report.Remote = preferredRemote(remotes)

	noFetch := opts.NoFetch || opts.Quiet
	if !noFetch && report.Remote != "" {
		if err := g.fetchPrune(ctx, report.Remote); err != nil {
			return nil, err
		}
	}

	base, source, ok, err := resolveQueueBase(ctx, g, opts.Base, report.Remote)
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

	integ, err := ResolveIntegrationBranch(ctx, exec, g.dir, opts.ConfigValues)
	if err != nil {
		return nil, err
	}
	report.Integration = integ

	refs, err := collectQueueRefs(ctx, g, report.Remote, base, integ)
	if err != nil {
		return nil, err
	}
	if opts.Quiet && len(refs) > QuietMaxBranches {
		report.QuietSkipped = true
		return report, nil
	}

	return fillQueueEntries(ctx, g, report, refs, baseSHA, expiry, now, opts.Quiet)
}

func fillQueueEntries(ctx context.Context, g gitRepo, report *QueueReport, refs []string, baseSHA string, expiry int, now time.Time, quiet bool) (*QueueReport, error) {
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

func resolveQueueBase(ctx context.Context, g gitRepo, flagBase, remote string) (name, source string, ok bool, err error) {
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

func collectQueueRefs(ctx context.Context, g gitRepo, remote, base string, integ Resolution) ([]string, error) {
	prefixes := []string{"refs/heads"}
	if remote != "" {
		prefixes = append(prefixes, "refs/remotes/"+remote)
	}
	raw, err := g.shortRefs(ctx, prefixes...)
	if err != nil {
		return nil, err
	}

	exclude, err := queueExcludeSet(ctx, g, remote, base, integ)
	if err != nil {
		return nil, err
	}

	var out []string
	seen := make(map[string]struct{}, len(raw))
	for _, ref := range raw {
		if _, skip := exclude[ref]; skip {
			continue
		}
		if remote != "" && strings.HasPrefix(ref, remote+"/") {
			local := strings.TrimPrefix(ref, remote+"/")
			localSHA, localOK, err := g.revParse(ctx, "refs/heads/"+local)
			if err != nil {
				return nil, err
			}
			remoteSHA, remoteOK, err := g.revParse(ctx, ref)
			if err != nil {
				return nil, err
			}
			if localOK && remoteOK && localSHA == remoteSHA {
				continue
			}
		}
		if _, dup := seen[ref]; dup {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	sort.Strings(out)
	return out, nil
}

func queueExcludeSet(ctx context.Context, g gitRepo, remote, base string, integ Resolution) (map[string]struct{}, error) {
	exclude := map[string]struct{}{base: {}}
	remotes := []string(nil)
	if remote != "" {
		remotes = []string{remote}
	}
	if bare := NormalizeName(base, remotes); bare != "" && bare != base {
		exclude[bare] = struct{}{}
		if remote != "" {
			exclude[remote+"/"+bare] = struct{}{}
		}
	}
	if remote != "" {
		exclude[remote] = struct{}{}
		exclude[remote+"/HEAD"] = struct{}{}
	}
	if !integ.Participates || integ.Name == "" {
		return exclude, nil
	}
	exclude[integ.Name] = struct{}{}
	if remote != "" {
		exclude[remote+"/"+integ.Name] = struct{}{}
	}
	allRemotes, err := g.shortRefs(ctx, "refs/remotes")
	if err != nil {
		return nil, err
	}
	suffix := "/" + integ.Name
	for _, r := range allRemotes {
		if strings.HasSuffix(r, suffix) {
			exclude[r] = struct{}{}
		}
	}
	return exclude, nil
}

func queueRefAllowed(ref string) bool {
	return gitcmd.SanitizeBranchName(ref) == nil
}

func inspectQueueRef(ctx context.Context, g gitRepo, ref, baseSHA string, expiryDays int, now time.Time) (QueueEntry, bool, error) {
	if !queueRefAllowed(ref) {
		return QueueEntry{}, true, nil
	}
	_, ok, err := g.revParse(ctx, ref)
	if err != nil {
		return QueueEntry{}, false, err
	}
	if !ok {
		return QueueEntry{}, true, nil
	}

	ahead, behind, err := g.aheadBehind(ctx, ref, baseSHA)
	if err != nil {
		return QueueEntry{}, false, err
	}

	entry := QueueEntry{
		Ref:    ref,
		Ahead:  ahead,
		Behind: behind,
	}

	mb, err := g.mergeBase(ctx, ref, baseSHA)
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
		clean, err := g.mergeTreeClean(ctx, baseSHA, ref)
		if err != nil {
			return QueueEntry{}, false, err
		}
		if clean {
			entry.MergeState = "clean"
		} else {
			entry.MergeState = "CONFLICT"
		}
	}

	epoch, err := g.commitTime(ctx, ref)
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
