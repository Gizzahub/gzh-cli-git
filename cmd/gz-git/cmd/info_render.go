// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"io"
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

	markerNone    = "  "
	markerAttn    = "🔸" // dirty, diverged, or behind its base
	markerBlocked = "🔻" // mid-rebase/merge, conflicts, or a scan error
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
