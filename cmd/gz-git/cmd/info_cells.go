// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// This file decides what each column says; info_render.go decides where the
// columns go. The split follows what makes them change: layout changes when
// alignment or compression rules change, and these change when the meaning of
// a state changes. Every renderer here returns a blank cell for the normal
// case, which is the rule the layout then compresses on.

const (
	maxOtherShown  = 3  // branch names listed before collapsing to "+N"
	maxBranchWidth = 28 // branch names longer than this are middle-elided
)

// buildInfoRow reduces one repository to its cells.
func buildInfoRow(
	repo *repository.RepositoryStatusResult,
	enr infoEnrichment,
	useRelativePath bool,
) infoRow {
	name := filepath.Base(repo.Path)
	if useRelativePath && repo.RelativePath != "" {
		name = repo.RelativePath
	}

	cells := make([]infoCell, len(infoColumns))
	cells[colRepo] = infoCell{text: name}
	cells[colBranch] = branchCell(repo)
	cells[colBase] = baseCell(repo.Branch, enr)
	cells[colWorktree] = worktreeCell(enr)
	cells[colDirty] = dirtyCell(repo)
	cells[colRemote] = remoteCell(repo)
	cells[colOther] = otherBranchesCell(repo, enr)
	cells[colRemoteOnly] = remoteOnlyBranchesCell(repo, enr)

	return infoRow{
		marker:   infoMarker(repo, enr),
		cells:    cells,
		baseName: enr.Base.Name,
	}
}

// infoMarker classifies the row. Blocked outranks attention: a repository in
// the middle of a rebase cannot be acted on until that resolves, so it should
// not read the same as one that merely has edits.
func infoMarker(repo *repository.RepositoryStatusResult, enr infoEnrichment) string {
	if repo.Error != nil || enr.Err != nil ||
		repo.RebaseInProgress || repo.MergeInProgress || len(repo.ConflictFiles) > 0 {
		return markerBlocked
	}
	if hasLocalChanges(repo) ||
		repo.CommitsAhead > 0 || repo.CommitsBehind > 0 ||
		enr.Base.Behind > 0 {
		return markerAttn
	}
	return markerNone
}

// hasLocalChanges is the single definition of "this working tree is not clean",
// shared by the attention marker and the DIRTY column. They must agree: a row
// that prints "~3" without a marker, or carries a marker with nothing to show
// for it, teaches the reader to distrust both.
//
// TrackedChangedFiles is checked alongside the individual counts because it is
// the aggregate the scanner fills in, and a caller may populate either.
func hasLocalChanges(repo *repository.RepositoryStatusResult) bool {
	return repo.TrackedChangedFiles > 0 ||
		repo.StagedFiles > 0 ||
		repo.UnstagedFiles > 0 ||
		repo.UntrackedFiles > 0 ||
		repo.StashCount > 0
}

// branchCell shows the current branch in the shape a shell prompt uses: the
// name, then its position against the upstream as arrows on the same line.
// "develop ↑2" is one fact about one branch in the form `git status -sb`
// trained readers on, and an in-sync branch prints its name alone — the blank
// where arrows would go is the "nothing to report" of this table.
//
// The cell takes the divergence's color when there is one: the name is the
// stable part and the arrows are the news. A detached HEAD has no branch to
// compare against, so it keeps its own red form with no arrows appended.
func branchCell(repo *repository.RepositoryStatusResult) infoCell {
	if repo.Branch == "" {
		sha := repo.HeadSHA
		if sha == "" {
			sha = "?"
		}
		return infoCell{text: "detached@" + sha, color: cliutil.ColorRed}
	}
	name := elideMiddle(repo.Branch, maxBranchWidth)
	div := upstreamDivergence(repo)
	if div.text == "" {
		return infoCell{text: name, color: cliutil.ColorCyan}
	}
	return infoCell{text: name + " " + div.text, color: div.color}
}

// upstreamDivergence renders the fragment that follows the branch name: HEAD's
// position against its upstream.
//
// The two ways a comparison can be missing look identical from ahead/behind
// alone but have different repairs, so they render differently. With a remote
// configured, an untracked branch is one `push -u` away and the fragment says
// so. With no remote at all there is nothing to push to; the REMOTE column
// already reports that, and repeating it here would say one thing twice.
func upstreamDivergence(repo *repository.RepositoryStatusResult) infoCell {
	if repo.Upstream == "" {
		if repo.RemoteURL == "" {
			return infoCell{}
		}
		return infoCell{text: "no upstream", color: cliutil.ColorYellow}
	}
	return divergenceText(repo.CommitsAhead, repo.CommitsBehind)
}

// divergenceText renders an ahead/behind pair with no opinion about what is
// being compared, which is why the base column can share it. In sync prints
// nothing at all — the blank is the signal.
func divergenceText(ahead, behind int) infoCell {
	switch {
	case ahead > 0 && behind > 0:
		return infoCell{text: fmt.Sprintf("↑%d ↓%d", ahead, behind), color: cliutil.ColorRed}
	case ahead > 0:
		return infoCell{text: fmt.Sprintf("↑%d", ahead), color: cliutil.ColorYellow}
	case behind > 0:
		return infoCell{text: fmt.Sprintf("↓%d", behind), color: cliutil.ColorYellow}
	default:
		return infoCell{}
	}
}

// baseCell renders divergence from the integration branch. The name is kept in
// the cell here; hoistUniformBase strips it when it is uniform across the scan.
//
// Sitting on the base branch prints nothing: HEAD cannot diverge from itself,
// so the name would repeat down the whole table to say "0, as always". The
// BRANCH column already shows where the user is.
func baseCell(currentBranch string, enr infoEnrichment) infoCell {
	if enr.Base.Name == "" {
		return infoCell{text: "no base", color: cliutil.ColorGray}
	}
	if enr.Base.Name == currentBranch {
		return infoCell{}
	}

	div := divergenceText(enr.Base.Ahead, enr.Base.Behind)
	if div.text == "" {
		return infoCell{text: enr.Base.Name, color: cliutil.ColorGray}
	}
	return infoCell{text: enr.Base.Name + " " + div.text, color: div.color}
}

// worktreeCell prints the linked worktree count, blank when there are none.
func worktreeCell(enr infoEnrichment) infoCell {
	if enr.LinkedWorktrees == 0 {
		return infoCell{}
	}
	return infoCell{text: fmt.Sprintf("%d", enr.LinkedWorktrees), color: cliutil.ColorMagenta}
}

// dirtyCell compresses the working tree into +staged ~unstaged ?untracked, and
// promotes an in-progress rebase/merge or conflicts over the file counts, since
// those determine what the user can do next.
func dirtyCell(repo *repository.RepositoryStatusResult) infoCell {
	switch {
	case len(repo.ConflictFiles) > 0:
		return infoCell{text: fmt.Sprintf("conflict:%d", len(repo.ConflictFiles)), color: cliutil.ColorRed}
	case repo.RebaseInProgress:
		return infoCell{text: "REBASING", color: cliutil.ColorRed}
	case repo.MergeInProgress:
		return infoCell{text: "MERGING", color: cliutil.ColorRed}
	}

	var parts []string
	if repo.StagedFiles > 0 {
		parts = append(parts, fmt.Sprintf("+%d", repo.StagedFiles))
	}
	if repo.UnstagedFiles > 0 {
		parts = append(parts, fmt.Sprintf("~%d", repo.UnstagedFiles))
	}
	if repo.UntrackedFiles > 0 {
		parts = append(parts, fmt.Sprintf("?%d", repo.UntrackedFiles))
	}
	if repo.StashCount > 0 {
		parts = append(parts, fmt.Sprintf("stash:%d", repo.StashCount))
	}
	if len(parts) == 0 {
		return infoCell{}
	}
	return infoCell{text: strings.Join(parts, " "), color: cliutil.ColorYellow}
}

// otherBranchesCell summarizes the local branches that are neither the current
// branch nor the base, marking the ones that are checked out in a worktree.
// Those are not stale leftovers — they are active work parked elsewhere, and
// conflating the two is what makes a branch list unreadable.
func otherBranchesCell(repo *repository.RepositoryStatusResult, enr infoEnrichment) infoCell {
	inWorktree := make(map[string]struct{}, len(enr.WorktreeBranches))
	for _, b := range enr.WorktreeBranches {
		inWorktree[b] = struct{}{}
	}

	others := make([]string, 0, len(repo.LocalBranches))
	for _, b := range repo.LocalBranches {
		if b == repo.Branch || b == enr.Base.Name {
			continue
		}
		others = append(others, b)
	}
	if len(others) == 0 {
		return infoCell{}
	}
	sort.Strings(others)

	// Worktree-backed branches sort first: they are the ones with a checkout
	// behind them, so they are what the user most likely wants named.
	sort.SliceStable(others, func(i, j int) bool {
		_, iw := inWorktree[others[i]]
		_, jw := inWorktree[others[j]]
		return iw && !jw
	})

	shown := others
	suffix := ""
	if len(others) > maxOtherShown {
		shown = others[:maxOtherShown]
		suffix = fmt.Sprintf(" +%d", len(others)-maxOtherShown)
	}

	labels := make([]string, 0, len(shown))
	for _, b := range shown {
		label := elideMiddle(b, maxBranchWidth)
		if _, ok := inWorktree[b]; ok {
			label += "*" // has a worktree
		}
		labels = append(labels, label)
	}

	return infoCell{text: strings.Join(labels, ", ") + suffix, color: cliutil.ColorGray}
}

// remoteOnlyBranchesCell summarizes remote-tracking branches with no local
// counterpart. When displayed entries come from one remote their branch names
// are concise; entries from multiple remotes all keep their full refs. Using a
// single labeling mode avoids shorthand/full collisions by construction.
func remoteOnlyBranchesCell(repo *repository.RepositoryStatusResult, enr infoEnrichment) infoCell {
	type remoteBranch struct {
		remote string
		full   string
		branch string
	}
	refs := remoteOnlyTrackingBranches(repo, enr)
	branches := make([]remoteBranch, 0, len(refs))
	for _, full := range refs {
		remote, branch, ok := remoteTrackingBranch(full, repo.Remotes)
		if !ok {
			continue
		}
		branches = append(branches, remoteBranch{remote: remote, full: full, branch: branch})
	}
	if len(branches) == 0 {
		return infoCell{}
	}
	// Top-level branches conventionally carry integration points (develop,
	// release, and similar), while hierarchical names are commonly generated
	// automation branches. Prioritize the former so truncation does not hide
	// the branch most likely to be useful to a human reader; preserve full-ref
	// ordering within each group for stable output across remotes.
	sort.Slice(branches, func(i, j int) bool {
		iTopLevel := !strings.Contains(branches[i].branch, "/")
		jTopLevel := !strings.Contains(branches[j].branch, "/")
		if iTopLevel != jTopLevel {
			return iTopLevel
		}
		return branches[i].full < branches[j].full
	})

	shown := branches
	suffix := ""
	if len(branches) > maxOtherShown {
		shown = branches[:maxOtherShown]
		suffix = fmt.Sprintf(" +%d", len(branches)-maxOtherShown)
	}
	remotes := make(map[string]struct{}, len(shown))
	for _, branch := range shown {
		remotes[branch.remote] = struct{}{}
	}
	rawLabels := make([]string, 0, len(shown))
	for _, branch := range shown {
		if len(remotes) == 1 {
			rawLabels = append(rawLabels, branch.branch)
		} else {
			rawLabels = append(rawLabels, branch.full)
		}
	}
	elidedCounts := make(map[string]int, len(rawLabels))
	for _, label := range rawLabels {
		elidedCounts[elideMiddle(label, maxBranchWidth)]++
	}
	labels := make([]string, 0, len(rawLabels))
	for _, label := range rawLabels {
		elided := elideMiddle(label, maxBranchWidth)
		if elidedCounts[elided] > 1 {
			// REMOTE ONLY is the unpadded trailing column, so preserving the
			// complete ref is preferable to rendering two indistinguishable
			// abbreviations and losing the remote disambiguation contract.
			labels = append(labels, label)
			continue
		}
		labels = append(labels, elided)
	}
	return infoCell{text: strings.Join(labels, ", ") + suffix, color: cliutil.ColorGray}
}

// remoteOnlyTrackingBranches returns complete remote-tracking refs rather than
// display labels. It is shared by the table, detailed view, and structured
// output so all formats apply exactly the same exclusion contract.
func remoteOnlyTrackingBranches(repo *repository.RepositoryStatusResult, enr infoEnrichment) []string {
	local := make(map[string]struct{}, len(repo.LocalBranches)+3)
	for _, branch := range repo.LocalBranches {
		local[branch] = struct{}{}
	}
	if _, branch, ok := remoteTrackingBranch(repo.Upstream, repo.Remotes); ok {
		local[branch] = struct{}{}
	}
	if repo.Branch != "" {
		local[repo.Branch] = struct{}{}
	}
	if enr.Base.Name != "" {
		local[enr.Base.Name] = struct{}{}
	}

	branches := make([]string, 0, len(repo.RemoteBranches))
	seen := make(map[string]struct{}, len(repo.RemoteBranches))
	for _, full := range repo.RemoteBranches {
		_, branch, ok := remoteTrackingBranch(full, repo.Remotes)
		if !ok {
			continue
		}
		if _, exists := local[branch]; !exists {
			if _, duplicate := seen[full]; duplicate {
				continue
			}
			seen[full] = struct{}{}
			branches = append(branches, full)
		}
	}
	sort.Slice(branches, func(i, j int) bool {
		_, iBranch, _ := remoteTrackingBranch(branches[i], repo.Remotes)
		_, jBranch, _ := remoteTrackingBranch(branches[j], repo.Remotes)
		iTopLevel := !strings.Contains(iBranch, "/")
		jTopLevel := !strings.Contains(jBranch, "/")
		if iTopLevel != jTopLevel {
			return iTopLevel
		}
		return branches[i] < branches[j]
	})
	return branches
}

// remoteTrackingBranch resolves a remote-tracking ref using configured remote
// names instead of guessing where the remote name ends. Git permits slash in a
// remote name (for example team/foo), so use the longest matching <remote>/
// prefix. If configured remote namespaces overlap (team and team/foo), the ref
// text itself is ambiguous; longest-prefix is the documented deterministic
// policy rather than an attempt to infer Git's original destination.
func remoteTrackingBranch(ref string, remotes map[string]string) (remote, branch string, ok bool) {
	for name := range remotes {
		prefix := name + "/"
		if !strings.HasPrefix(ref, prefix) {
			continue
		}
		if len(name) > len(remote) {
			remote = name
			branch = strings.TrimPrefix(ref, prefix)
		}
	}
	if remote == "" || branch == "" || branch == "HEAD" {
		return "", "", false
	}
	return remote, branch, true
}

// remoteCell reports whether the repository has a remote at all. Presence is
// the normal case, so it says nothing; "no remote" marks the repository that
// cannot push or pull — a freshly `git init`ed one. Which owner a remote
// points at is detail rather than state, and detail lives in --full.
func remoteCell(repo *repository.RepositoryStatusResult) infoCell {
	if repo.RemoteURL == "" {
		return infoCell{text: "no remote", color: cliutil.ColorRed}
	}
	return infoCell{}
}

// elideMiddle shortens s to max runes, keeping both ends. Branch names carry
// their meaning at both ends ("feat/" prefix, ticket suffix), so a trailing
// truncation would drop exactly the part that distinguishes siblings.
func elideMiddle(s string, maxLen int) string {
	n := utf8.RuneCountInString(s)
	if n <= maxLen || maxLen < 5 {
		return s
	}
	runes := []rune(s)
	head := (maxLen - 1) / 2
	tail := maxLen - 1 - head
	return string(runes[:head]) + "…" + string(runes[n-tail:])
}
