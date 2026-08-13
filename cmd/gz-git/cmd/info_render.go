// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// The compact view is built on one rule: normal states print nothing.
//
// The previous layout spent the same twelve lines on a repository that was
// clean and current as on one that was mid-rebase with three worktrees, so
// scanning thirteen repositories meant reading 160 lines to find the two that
// mattered. Here an in-sync column is blank, a clean tree is blank, and zero
// worktrees is blank — whatever is left on the line is, by construction, the
// thing worth looking at.
const (
	// markerWidth is the fixed width of the leading attention column. Emoji
	// with Emoji_Presentation occupy two terminal cells, so the column is
	// either exactly one such emoji or exactly two spaces — never a mix with
	// single-width characters, which is what breaks column alignment.
	markerWidth = 2

	markerNone     = "  "
	markerAttn     = "🔸" // dirty, diverged, or behind its base
	markerBlocked  = "🔻" // mid-rebase/merge, conflicts, or a scan error
	maxOtherShown  = 3   // branch names listed before collapsing to "+N"
	maxBranchWidth = 28  // branch names longer than this are middle-elided
)

// infoCell is one table cell: the plain text used for width arithmetic, plus
// the color it is drawn in. Keeping the two apart is what makes alignment
// survive the color gate — ANSI escapes are bytes with no display width, so
// padding computed on a pre-colored string is wrong by exactly the length of
// the escape sequences, and the error is invisible whenever colors are off.
type infoCell struct {
	text  string
	color string
}

// render pads the cell to width using its plain text, then colors the result.
func (c infoCell) render(width int) string {
	pad := width - utf8.RuneCountInString(c.text)
	if pad < 0 {
		pad = 0
	}
	if c.color == "" || c.text == "" {
		return c.text + strings.Repeat(" ", pad)
	}
	return c.color + c.text + cliutil.ColorReset + strings.Repeat(" ", pad)
}

// infoRow is one repository reduced to the columns the compact view prints.
type infoRow struct {
	marker   string
	cells    []infoCell
	baseName string // for deciding whether to hoist the base name into the header
}

// infoColumns are the fixed-position columns, in print order. "OTHER BRANCHES"
// is deliberately last and unpadded so it can absorb variable-length content
// without pushing anything else out of alignment.
var infoColumns = []string{"REPOSITORY", "BRANCH", "UPSTREAM", "BASE", "WT", "DIRTY", "REMOTE", "OTHER BRANCHES"}

const (
	colRepo = iota
	colBranch
	colUpstream
	colBase
	colWorktree
	colDirty
	colRemote
	colOther
)

// renderInfoCompact writes the one-line-per-repository table.
func renderInfoCompact(
	w io.Writer,
	result *repository.BulkStatusResult,
	enrichment map[string]infoEnrichment,
	useRelativePath bool,
) {
	if len(result.Repositories) == 0 {
		fmt.Fprintln(w, "No repositories found.")
		return
	}

	// The dominant remote owner is established first so each row can stay
	// silent about it and speak up only when it differs.
	majorityOwner, majorityCount := majorityRemoteOwner(result.Repositories)

	rows := make([]infoRow, 0, len(result.Repositories))
	attention := 0
	for i := range result.Repositories {
		repo := &result.Repositories[i]
		row := buildInfoRow(repo, enrichment[repo.Path], majorityOwner, useRelativePath)
		if row.marker != markerNone {
			attention++
		}
		rows = append(rows, row)
	}

	headers := hoistUniformBase(infoColumns, rows)
	headers, rows = dropEmptyColumns(headers, rows)
	widths := computeInfoWidths(headers, rows)

	fmt.Fprintln(w)
	fmt.Fprintln(w, summarizeInfoScan(result, attention))
	fmt.Fprintln(w)
	fmt.Fprintln(w, renderInfoHeader(headers, widths))

	for _, row := range rows {
		line := row.marker
		for i, cell := range row.cells {
			line += " " + cell.render(widths[i])
		}
		fmt.Fprintln(w, strings.TrimRight(line, " "))
	}

	if note := remoteFooter(majorityOwner, majorityCount, len(result.Repositories)); note != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, note)
	}
	fmt.Fprintln(w)
}

// dropEmptyColumns removes any column whose every cell is empty, header
// included. An all-empty column still costs its header's width in horizontal
// space while carrying no information — and under the "normal prints nothing"
// rule, entire columns going empty is the expected case, not an edge case.
// The trailing column is never dropped, so at least one column always remains.
func dropEmptyColumns(headers []string, rows []infoRow) ([]string, []infoRow) {
	keep := make([]int, 0, len(headers))
	for c := range headers {
		if c == len(headers)-1 {
			keep = append(keep, c)
			continue
		}
		for _, row := range rows {
			if row.cells[c].text != "" {
				keep = append(keep, c)
				break
			}
		}
	}

	outHeaders := make([]string, 0, len(keep))
	for _, c := range keep {
		outHeaders = append(outHeaders, headers[c])
	}

	outRows := make([]infoRow, len(rows))
	for i, row := range rows {
		cells := make([]infoCell, 0, len(keep))
		for _, c := range keep {
			cells = append(cells, row.cells[c])
		}
		row.cells = cells
		outRows[i] = row
	}
	return outHeaders, outRows
}

// renderInfoHeader draws the column titles in gray so they recede behind the
// data, and blanks the trailing column's padding.
func renderInfoHeader(headers []string, widths []int) string {
	line := strings.Repeat(" ", markerWidth)
	for i, h := range headers {
		cell := infoCell{text: h, color: cliutil.ColorGray}
		line += " " + cell.render(widths[i])
	}
	return strings.TrimRight(line, " ")
}

// computeInfoWidths sizes each column to its widest plain value. The final
// column is given width 0: it is printed last, so padding it would only add
// trailing whitespace.
func computeInfoWidths(headers []string, rows []infoRow) []int {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, row := range rows {
		for i, cell := range row.cells {
			if n := utf8.RuneCountInString(cell.text); n > widths[i] {
				widths[i] = n
			}
		}
	}
	widths[len(widths)-1] = 0
	return widths
}

// hoistUniformBase moves the base branch name into the column header when every
// repository resolved to the same one, which is the common case in a workspace.
// Repeating "master" down thirteen rows is thirteen copies of one fact; naming
// it once in the header leaves the cells free to show only the divergence.
func hoistUniformBase(headers []string, rows []infoRow) []string {
	out := append([]string(nil), headers...)

	names := make(map[string]struct{})
	for _, row := range rows {
		if row.baseName != "" {
			names[row.baseName] = struct{}{}
		}
	}
	if len(names) != 1 {
		return out
	}

	var only string
	for name := range names {
		only = name
	}
	out[colBase] = "BASE(" + only + ")"

	// The name is now in the header, so strip it from every cell. Indexed
	// rather than ranged: this mutates the caller's rows, and `for _, row`
	// would hide that behind a struct copy that happens to share the cells
	// slice.
	for i := range rows {
		if rows[i].baseName == "" {
			continue
		}
		cell := &rows[i].cells[colBase]
		cell.text = strings.TrimSpace(strings.TrimPrefix(cell.text, only))
	}
	return out
}

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
	cells[colUpstream] = divergenceCell(repo.CommitsAhead, repo.CommitsBehind, repo.Upstream == "")
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

// divergenceCell renders an ahead/behind pair. In sync prints nothing at all —
// the blank is the signal. noUpstream is distinct from in-sync and says so,
// because "nothing to compare against" is a state a user may need to fix.
func divergenceCell(ahead, behind int, noUpstream bool) infoCell {
	if noUpstream {
		return infoCell{text: "no remote", color: cliutil.ColorGray}
	}
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

	div := divergenceCell(enr.Base.Ahead, enr.Base.Behind, false)
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

// summarizeInfoScan is the one-line preamble.
func summarizeInfoScan(result *repository.BulkStatusResult, attention int) string {
	parts := []string{
		fmt.Sprintf("%d repositories", len(result.Repositories)),
		result.Duration.Round(10 * time.Millisecond).String(),
	}
	if attention > 0 {
		parts = append(parts, fmt.Sprintf("%s%d need attention%s", cliutil.ColorYellow, attention, cliutil.ColorReset))
	} else {
		parts = append(parts, cliutil.ColorGreen+"all clean"+cliutil.ColorReset)
	}
	return strings.Join(parts, "  ·  ")
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

// remoteFooter states the dominant remote once, so the REMOTE column can stay
// empty for every repository that follows it.
func remoteFooter(owner string, count, total int) string {
	if owner == "" || count == 0 {
		return ""
	}
	scope := fmt.Sprintf("%d of %d", count, total)
	if count == total {
		scope = fmt.Sprintf("all %d", total)
	}
	return fmt.Sprintf("%sremote: %s (%s)%s", cliutil.ColorGray, owner, scope, cliutil.ColorReset)
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
