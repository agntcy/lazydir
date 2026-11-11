// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agntcy/lazydir/internal/dirclient"
	"github.com/jesseduffield/gocui"
	"github.com/mattn/go-runewidth"
)

// syncedAgo renders roughly how long ago the displayed records were last
// synced, in compact coarse units (e.g. "< 1m ago", "5m ago", "2h ago",
// "3d ago"), as a relative staleness indicator for the records panel title.
// The panel is redrawn on a timer (see uiRefreshLoop) so this value stays
// current.
func syncedAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "< 1m ago"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	}
}

// indent1 is the single-level indentation used for child rows everywhere in
// the TUI (applied filter selections, inline descriptions, inline record info).
const indent1 = "    "

// recordDisplayRow describes one rendered line in the records panel. In the
// server-side pagination model there is exactly one row per record (no version
// grouping), so record is always non-nil.
type recordDisplayRow struct {
	record *dirclient.RecordSummary
}

// renderFiltersView redraws the [2] Filters panel as a collapsible tree of
// filter categories and their options.
func (app *Gui) renderFiltersView(g *gocui.Gui) {
	v, err := g.View(viewFilters)
	if err != nil {
		return
	}
	v.Clear()
	app.renderFiltersList(g, v)
}

// renderFiltersList draws the unified filter tree: each category has a
// collapse/expand triangle, child options are indented, and selected options
// are rendered in the category's color instead of a [ ]/[x] checkbox.
func (app *Gui) renderFiltersList(g *gocui.Gui, v *gocui.View) {
	title := titleFilters
	// Non-fatal: ListFilterValues failed (e.g. Unimplemented on an older
	// server, or a transient error on refetch). Queries still work.
	title += filterValuesNotice(app.state.filterValuesLoaded, app.state.filterValuesFailed)
	if app.state.filters.filterQuery != "" {
		title += fmt.Sprintf("  /: %s", app.state.filters.filterQuery)
	}
	v.Title = title

	rows := app.filteredListRows()
	fs := &app.state.filters

	if fs.listCursor < 0 {
		fs.listCursor = 0
	}
	if max := len(rows) - 1; max >= 0 && fs.listCursor > max {
		fs.listCursor = max
	}

	lineNum := 0
	targetLine := 0
	for i, r := range rows {
		if i == fs.listCursor {
			targetLine = lineNum
		}

		if r.option == "" {
			triangle := triangleCollapsed
			if fs.expanded[r.category] || fs.filterQuery != "" {
				triangle = triangleExpanded
			}
			fmt.Fprintf(v, " %s %s\n", triangle, r.category.title())
		} else {
			app.renderFilterOption(v, r, fs.applied[r.category])
		}
		lineNum++
	}

	_ = v.SetOrigin(0, 0)
	_, viewH := v.Size()
	if targetLine >= viewH {
		_ = v.SetOrigin(0, targetLine-viewH+1)
		_ = v.SetCursor(0, viewH-1)
	} else {
		_ = v.SetCursor(0, targetLine)
	}
}

// renderFilterOption renders one option row into the filter view. Class
// categories show "ID Caption" when enrichment data is available; other
// categories show the raw option label. Included options use the category
// color; excluded (negated) options add strikethrough.
func (app *Gui) renderFilterOption(v *gocui.View, r listRow, applied map[string]filterMode) {
	app.writeFilterOption(v, r, applied[r.option])
}

// writeFilterOption is the io.Writer-based core of renderFilterOption, split
// out so it can be unit-tested without a gocui.View.
func (app *Gui) writeFilterOption(w io.Writer, r listRow, mode filterMode) {
	entries := app.classEntriesFor(r.category)

	prefix := ""
	suffix := ""
	switch mode {
	case modeInclude:
		prefix = app.theme.filterColor(r.category)
		suffix = app.theme.Reset
	case modeExclude:
		prefix = app.theme.filterColor(r.category) + app.theme.Strike
		suffix = app.theme.Reset
	}

	if e, ok := entries[r.option]; ok && e.Caption != "" {
		idStr := fmt.Sprintf("%d", e.ID)
		if prefix != "" {
			fmt.Fprintf(w, "%s%s%s %s%s\n", indent1, prefix, idStr, e.Caption, suffix)
		} else {
			fmt.Fprintf(w, "%s%s %s\n", indent1, idStr, e.Caption)
		}
		return
	}

	if prefix != "" {
		fmt.Fprintf(w, "%s%s%s%s\n", indent1, prefix, r.option, suffix)
	} else {
		fmt.Fprintf(w, "%s%s\n", indent1, r.option)
	}
}

// recordsTitle builds the [3] Records panel title, reflecting the current
// record count, name filter, and (once loaded) the relative staleness of the
// displayed data.
func (app *Gui) recordsTitle() string {
	loaded := len(app.state.page.records)

	title := titleRecords
	if app.state.page.totalKnown {
		title += fmt.Sprintf(" (%d/%d)", loaded, app.state.page.total)
	} else if loaded > 0 {
		title += fmt.Sprintf(" (%d)", loaded)
	}
	if n := len(app.state.selection); n > 0 {
		title += fmt.Sprintf(" [%d selected]", n)
	}
	if app.state.filterQuery != "" {
		title += fmt.Sprintf("  /: %s", app.state.filterQuery)
	}
	if app.state.loadedOnce && !app.state.dataFetchedAt.IsZero() {
		title += fmt.Sprintf("  [⟳ %s]", syncedAgo(app.state.dataFetchedAt))
	}
	return title
}

// renderRecordsView redraws the [3] Records panel and updates its title to
// reflect the current record count, stream state, and name filter.
func (app *Gui) renderRecordsView(g *gocui.Gui) {
	v, err := g.View(viewRecords)
	if err != nil {
		return
	}
	v.Clear()

	v.Title = app.recordsTitle()

	viewW, viewH := v.Size()
	wrap := viewW < oneLineRowMinWidth
	nameW := viewW - rowGutterWidth - 2 - versionColWidth - 2 - badgeColWidth
	if wrap {
		// Badges go on their own line, so the name only shares with the version.
		nameW = viewW - rowGutterWidth - 2 - versionColWidth
	}
	nameW = max(nameW, minNameColWidth)
	app.state.recordRowHeight = 1
	if wrap {
		app.state.recordRowHeight = 2
	}
	height := app.state.recordRowHeight

	rows := app.state.recordDisplayRows
	if app.state.recordCursor >= len(rows) && len(rows) > 0 {
		app.state.recordCursor = len(rows) - 1
	}

	clipBg := "\033[41m"
	yellow := "\033[33m"
	red := "\033[31m"
	green := ansiGreen

	for _, row := range rows {
		rec := row.record
		if rec == nil {
			fmt.Fprint(v, strings.Repeat("\n", height))
			continue
		}

		_, inClip := app.state.clipboard[rec.CID]
		name := rec.Name
		if name == "" {
			name = rec.CID
		}
		version := rec.Version
		if version == "" {
			version = "n/a"
		}

		statusColor := ""
		switch status, _ := app.recordStatus(rec.CID); status {
		case dirclient.StatusSyncing, dirclient.StatusReconciling:
			statusColor = yellow
		case dirclient.StatusFailed:
			statusColor = red
		}

		name = runewidth.Truncate(name, nameW, "…")
		version = runewidth.Truncate(version, versionColWidth, "…")
		lineColor := ""
		switch {
		case statusColor != "":
			lineColor = statusColor
		case inClip:
			lineColor = clipBg
		case rec.Published:
			lineColor = green
		}
		selected := app.isSelected(rec.CID)
		badges := trustBadges(rec, app.theme, rowStyle(lineColor, selected))
		for _, line := range recordRowLines(lineColor, selected, wrap, nameW, viewW, name, version, badges) {
			fmt.Fprintln(v, line)
		}
	}

	origin, cursor := rowScroll(app.state.recordCursor, height, viewH)
	_ = v.SetOrigin(0, origin)
	_ = v.SetCursor(0, cursor)
}

// rowScroll returns the view origin and cursor line that show every line of
// the row at index cursor, each row being height lines tall, in a view viewH
// lines high. The cursor sits on the row's first line.
func rowScroll(cursor, height, viewH int) (origin, line int) {
	first := cursor * height
	if last := first + height - 1; last >= viewH {
		origin = last - viewH + 1
	}
	return origin, first - origin
}

// rowGutterWidth is the width of the column ahead of each record row that
// holds the selection marker, so text lines up whether a row is selected or not.
const rowGutterWidth = 2

// Records row column widths. A longer version is cut with an ellipsis so the
// badge column stays aligned.
const (
	minNameColWidth = 8
	versionColWidth = 10
	badgeColWidth   = 5 // "T V S"
	// oneLineRowMinWidth is the narrowest panel that fits a whole row on one
	// line; below it the badges wrap onto a second line.
	oneLineRowMinWidth = rowGutterWidth + minNameColWidth + 2 + versionColWidth + 2 + badgeColWidth
)

// rowStyle is the ANSI style a Records row is drawn with: its line color, in
// bold when the row is selected.
func rowStyle(lineColor string, selected bool) string {
	if selected {
		return lineColor + ansiBold
	}
	return lineColor
}

// trustBadges renders the T(rusted) V(erified) S(afe) badge column for a
// record. A set badge is colored; an unset one is a dim dot. A record a
// scanner flagged unsafe shows a red "!" in the S slot, so "not scanned"
// (dot) stays apart from "unsafe". After each glyph the foreground is reset
// and restore re-applied, so the row's own style carries on.
func trustBadges(r *dirclient.RecordSummary, t Theme, restore string) string {
	glyph := func(color, g string) string {
		return color + g + ansiDefaultFg + restore
	}
	dot := glyph(t.Color10, "·")
	badge := func(on bool, letter string) string {
		if on {
			return glyph(t.Color9, letter)
		}
		return dot
	}
	safe := dot
	switch r.Scan {
	case dirclient.ScanSafe:
		safe = glyph(t.Color9, "S")
	case dirclient.ScanUnsafe:
		safe = glyph(t.Color6, "!")
	}
	return badge(r.Trusted, "T") + " " + badge(r.Verified, "V") + " " + safe
}

// recordRowLines formats one Records row: name, version and the trust badges
// on one line, or, with wrap, name and version on the first line and the
// badges on a second one. lineColor (may be empty) colors the whole row.
func recordRowLines(lineColor string, selected, wrap bool, nameW, viewW int, name, version, badges string) []string {
	if !wrap {
		return []string{recordRowLine(lineColor, selected, nameW, viewW, name, version, badges)}
	}
	return []string{
		styledRowLine(lineColor, selected, true, viewW, runewidth.FillRight(name, nameW)+"  "+version),
		styledRowLine(lineColor, selected, false, viewW, badges),
	}
}

// recordRowLine formats a Records row that fits on one line.
func recordRowLine(lineColor string, selected bool, nameW, viewW int, name, version, badges string) string {
	text := runewidth.FillRight(name, nameW) + "  " + runewidth.FillRight(version, versionColWidth) + "  " + badges
	return styledRowLine(lineColor, selected, true, viewW, text)
}

// styledRowLine lays text out behind the selection gutter. A selected row is
// bold, and its first line (mark) shows a green marker in the gutter; after
// the marker only the foreground is reset, so a background color such as the
// clipboard's carries on. Colored and selected lines are padded to the panel
// width so their color spans the row.
func styledRowLine(lineColor string, selected, mark bool, viewW int, text string) string {
	if lineColor == "" && !selected {
		return strings.Repeat(" ", rowGutterWidth) + text
	}
	gutter := strings.Repeat(" ", rowGutterWidth)
	if selected && mark {
		gutter = ansiGreen + selectedMark + ansiDefaultFg + lineColor + strings.Repeat(" ", rowGutterWidth-1)
	}
	// Pad by display width: the badges carry ANSI codes, and wide characters
	// take two cells.
	pad := max(viewW-rowGutterWidth-runewidth.StringWidth(stripANSI(text)), 0)
	return rowStyle(lineColor, selected) + gutter + text + strings.Repeat(" ", pad) + ansiReset
}

// wrapText splits text into lines that fit within maxWidth, breaking on word
// boundaries where possible. Newlines in the input are preserved.
func wrapText(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return strings.Split(text, "\n")
	}
	var result []string
	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph == "" {
			result = append(result, "")
			continue
		}
		for len(paragraph) > maxWidth {
			cut := maxWidth
			for cut > 0 && paragraph[cut] != ' ' {
				cut--
			}
			if cut == 0 {
				cut = maxWidth
			}
			result = append(result, paragraph[:cut])
			paragraph = strings.TrimLeft(paragraph[cut:], " ")
		}
		if paragraph != "" {
			result = append(result, paragraph)
		}
	}
	return result
}
