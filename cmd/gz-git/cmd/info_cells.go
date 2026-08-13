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
	majorityOwner string,
	useRelativePath bool,
) infoRow {
	name := filepath.Base(repo.Path)
	if useRelativePath && repo.RelativePath != "" {
		name = repo.RelativePath
	}

	cells := make([]infoCell, len(infoColumns))
	cells[colRepo] = infoCell{text: name}
	cells[colBranch] = branchCell(repo)
	cells[colUpstream] = divergenceCell(repo.CommitsAhead, repo.CommitsBehind,
		repo.Upstream == "", repo.RemoteURL != "")
	cells[colBase] = baseCell(repo.Branch, enr)
	cells[colWorktree] = worktreeCell(enr)
	cells[colDirty] = dirtyCell(repo)
	cells[colRemote] = remoteCell(repo, majorityOwner)
	cells[colOther] = otherBranchesCell(repo, enr)

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

// branchCell shows the current branch, elided if long, and flags detached HEAD.
func branchCell(repo *repository.RepositoryStatusResult) infoCell {
	if repo.Branch == "" {
		sha := repo.HeadSHA
		if sha == "" {
			sha = "?"
		}
		return infoCell{text: "detached@" + sha, color: cliutil.ColorRed}
	}
	return infoCell{text: elideMiddle(repo.Branch, maxBranchWidth), color: cliutil.ColorCyan}
}

// divergenceCell renders HEAD's position against its upstream.
//
// The two ways a comparison can be missing look identical from ahead/behind
// alone but have different repairs, so they render differently. With a remote
// configured, an untracked branch is one `push -u` away and the cell says so.
// With no remote at all there is nothing to push to; the REMOTE column already
// reports that, and repeating it here would spend two columns on one fact.
func divergenceCell(ahead, behind int, noUpstream, hasRemote bool) infoCell {
	if noUpstream {
		if !hasRemote {
			return infoCell{}
		}
		return infoCell{text: "no upstream", color: cliutil.ColorYellow}
	}
	return divergenceText(ahead, behind)
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

// remoteCell names the remote owner only when it is not the workspace's
// dominant one. Printing the same owner on every row spends a column to convey
// a single fact; the footer states that fact once, and this column is left to
// call out the repositories that break the pattern — which is the case a user
// actually needs to see.
func remoteCell(repo *repository.RepositoryStatusResult, majorityOwner string) infoCell {
	owner := remoteOwner(repo.RemoteURL)
	if owner == "" {
		return infoCell{text: "no remote", color: cliutil.ColorRed}
	}
	if sameOwner(owner, majorityOwner) {
		return infoCell{}
	}
	return infoCell{text: owner, color: cliutil.ColorMagenta}
}

// majorityRemoteOwner returns the most common remote owner across the scan and
// how many repositories use it. Owners are grouped case-insensitively because
// forge hosts treat "Gizzahub" and "gizzahub" as the same account, so counting
// them separately would invent a discrepancy that does not exist. The returned
// spelling is the one that occurs most often, ties broken alphabetically for a
// stable result across runs.
func majorityRemoteOwner(repos []repository.RepositoryStatusResult) (owner string, count int) {
	counts := make(map[string]int)
	spellings := make(map[string]map[string]int)

	for i := range repos {
		o := remoteOwner(repos[i].RemoteURL)
		if o == "" {
			continue
		}
		key := strings.ToLower(o)
		counts[key]++
		if spellings[key] == nil {
			spellings[key] = make(map[string]int)
		}
		spellings[key][o]++
	}

	bestKey := ""
	for key, n := range counts {
		if n > counts[bestKey] || (n == counts[bestKey] && key < bestKey) {
			bestKey = key
		}
	}
	if bestKey == "" {
		return "", 0
	}

	best := ""
	for spelling, n := range spellings[bestKey] {
		if n > spellings[bestKey][best] || (n == spellings[bestKey][best] && spelling < best) {
			best = spelling
		}
	}
	return best, counts[bestKey]
}

// sameOwner compares owners the way forge hosts do.
func sameOwner(a, b string) bool {
	return a != "" && strings.EqualFold(a, b)
}

// remoteOwner reduces a remote URL to "host/owner", covering both the scp-like
// form (git@host:owner/repo.git) and the URL form (https://host/owner/repo).
// It returns "" when the URL is empty or does not carry an owner segment.
func remoteOwner(remoteURL string) string {
	url := strings.TrimSuffix(strings.TrimSpace(remoteURL), ".git")
	if url == "" {
		return ""
	}

	// scp-like: strip the user@ prefix and turn the single ":" into "/".
	if !strings.Contains(url, "://") {
		if at := strings.Index(url, "@"); at >= 0 {
			url = url[at+1:]
		}
		url = strings.Replace(url, ":", "/", 1)
	} else {
		url = url[strings.Index(url, "://")+3:]
		if at := strings.Index(url, "@"); at >= 0 {
			url = url[at+1:]
		}
	}

	parts := strings.Split(strings.Trim(url, "/"), "/")
	if len(parts) < 3 {
		return ""
	}
	return parts[0] + "/" + parts[1]
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
