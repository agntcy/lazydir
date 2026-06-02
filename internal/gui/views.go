// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"fmt"
	"strings"

	"github.com/agntcy/lazydir/internal/dirclient"
	"github.com/jesseduffield/gocui"
)

// indent1 is the single-level indentation used for child rows everywhere in
// the TUI (applied filter selections, inline descriptions, inline record info).
const indent1 = "    "

// recordDisplayRow describes one rendered line in the records panel.
// It is either a group header (when record is nil) or a record entry.
type recordDisplayRow struct {
	groupName string                   // non-empty for group headers
	record    *dirclient.RecordSummary // non-nil for actual record entries
	grouped   bool                     // true if this record is part of a multi-version group
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
	title := "[2] Filters"
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

// renderFilterOption renders one option row. Class categories show "ID Caption"
// when enrichment data is available; other categories show the raw option label.
// Selected options use the category's color.
func (app *Gui) renderFilterOption(v *gocui.View, r listRow, applied map[string]bool) {
	entries := app.classEntriesFor(r.category)
	selected := applied[r.option]
	color := ""
	if selected {
		color = app.theme.filterColor(r.category)
	}

	if e, ok := entries[r.option]; ok && e.Caption != "" {
		idStr := fmt.Sprintf("%d", e.ID)
		caption := e.Caption
		if color != "" {
			fmt.Fprintf(v, "%s%s%s %s%s\n", indent1, color, idStr, caption, app.theme.Reset)
		} else {
			fmt.Fprintf(v, "%s%s %s\n", indent1, idStr, caption)
		}
		return
	}

	if color != "" {
		fmt.Fprintf(v, "%s%s%s%s\n", indent1, color, r.option, app.theme.Reset)
	} else {
		fmt.Fprintf(v, "%s%s\n", indent1, r.option)
	}
}

// renderRecordsView redraws the [3] Records panel and updates its title to
// reflect the current record count, stream state, and name filter.
func (app *Gui) renderRecordsView(g *gocui.Gui) {
	v, err := g.View(viewRecords)
	if err != nil {
		return
	}
	v.Clear()

	records := app.state.filteredRecords
	total := len(app.state.records)

	title := "[3] Records"
	if total > 0 || app.state.stream == streamDone {
		if app.state.filterQuery != "" {
			title += fmt.Sprintf(" (%d/%d)", len(records), total)
		} else {
			title += fmt.Sprintf(" (%d)", total)
		}
	}
	if app.state.filterQuery != "" {
		title += fmt.Sprintf("  /: %s", app.state.filterQuery)
	}
	v.Title = title

	viewW, _ := v.Size()
	nameW := viewW - 14
	if nameW < 8 {
		nameW = 8
	}

	rows := app.state.recordDisplayRows
	if app.state.recordCursor >= len(rows) && len(rows) > 0 {
		app.state.recordCursor = len(rows) - 1
	}

	clipBg := "\033[41m"
	reset := "\033[0m"
	yellow := "\033[33m"
	red := "\033[31m"

	// Precompute which group names have non-local children.
	groupSyncStatus := map[string]dirclient.RecordStatus{}
	for _, r := range app.state.filteredRecords {
		if r.Status == dirclient.StatusLocal {
			continue
		}
		name := r.Name
		if name == "" {
			name = r.CID
		}
		if cur, ok := groupSyncStatus[name]; !ok || r.Status > cur {
			groupSyncStatus[name] = r.Status
		}
	}

	lineNum := 0
	targetLine := 0

	for i, row := range rows {
		if i == app.state.recordCursor {
			targetLine = lineNum
		}

		if row.groupName != "" {
			triangle := triangleCollapsed
			if app.state.recordGroupExpanded[row.groupName] {
				triangle = triangleExpanded
			}
			name := row.groupName
			if len(name) > nameW-2 {
				name = name[:nameW-3] + "…"
			}
			if gs, ok := groupSyncStatus[row.groupName]; ok {
				color := yellow
				if gs == dirclient.StatusFailed {
					color = red
				}
				line := fmt.Sprintf(" %s %s", triangle, name)
				fmt.Fprintf(v, "%s%-*s%s\n", color, viewW, line, reset)
			} else {
				fmt.Fprintf(v, " %s %s\n", triangle, name)
			}
		} else if row.record != nil {
			_, inClip := app.state.clipboard[row.record.CID]
			name := row.record.Name
			if name == "" {
				name = row.record.CID
			}
			version := row.record.Version
			if version == "" {
				version = "n/a"
			}

			statusColor := ""
			switch row.record.Status {
			case dirclient.StatusSyncing, dirclient.StatusReconciling:
				statusColor = yellow
			case dirclient.StatusFailed:
				statusColor = red
			}

			if row.grouped {
				if statusColor != "" {
					line := fmt.Sprintf("%s%s", indent1, version)
					fmt.Fprintf(v, "%s%-*s%s\n", statusColor, viewW, line, reset)
				} else if inClip {
					line := fmt.Sprintf("%s%s", indent1, version)
					fmt.Fprintf(v, "%s%-*s%s\n", clipBg, viewW, line, reset)
				} else {
					fmt.Fprintf(v, "%s%s\n", indent1, version)
				}
			} else {
				if len(name) > nameW {
					name = name[:nameW-1] + "…"
				}
				if statusColor != "" {
					line := fmt.Sprintf(" %-*s  %s", nameW, name, version)
					fmt.Fprintf(v, "%s%-*s%s\n", statusColor, viewW, line, reset)
				} else if inClip {
					line := fmt.Sprintf(" %-*s  %s", nameW, name, version)
					fmt.Fprintf(v, "%s%-*s%s\n", clipBg, viewW, line, reset)
				} else {
					fmt.Fprintf(v, " %-*s  %s\n", nameW, name, version)
				}
			}
		}
		lineNum++
	}

	_, viewH := v.Size()
	if targetLine >= viewH {
		_ = v.SetOrigin(0, targetLine-viewH+1)
		_ = v.SetCursor(0, viewH-1)
	} else {
		_ = v.SetOrigin(0, 0)
		_ = v.SetCursor(0, targetLine)
	}
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
