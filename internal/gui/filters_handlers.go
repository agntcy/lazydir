// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"context"
	"fmt"
	"strings"

	"github.com/agntcy/lazydir/internal/oasf"
	"github.com/jesseduffield/gocui"
)

// ── Filters panel handlers ────────────────────────────────────────────────────

func (app *Gui) filterMouseClick(g *gocui.Gui, v *gocui.View) error {
	app.hideInfoPopupIfVisible(g)
	if err := app.focusTo(g, viewFilters); err != nil {
		return err
	}
	_, cy := v.Cursor()
	_, oy := v.Origin()
	idx := oy + cy

	rows := app.filteredListRows()
	if idx < 0 || idx >= len(rows) {
		return nil
	}
	app.state.filters.listCursor = idx

	row := rows[idx]
	if row.option == "" {
		app.state.filters.expanded[row.category] = !app.state.filters.expanded[row.category]
		app.clearInlineDesc()
	} else {
		app.toggleApplied(row.category, row.option)
		app.applyFilters()
	}
	app.renderFiltersView(g)
	return nil
}

func (app *Gui) filterCursorUp(g *gocui.Gui, v *gocui.View) error {
	if app.state.filters.listCursor > 0 {
		app.state.filters.listCursor--
	}
	app.renderFiltersView(g)
	return nil
}

func (app *Gui) filterCursorDown(g *gocui.Gui, v *gocui.View) error {
	rows := app.filteredListRows()
	if app.state.filters.listCursor < len(rows)-1 {
		app.state.filters.listCursor++
	}
	app.renderFiltersView(g)
	return nil
}

// filterEnter toggles expand/collapse on category headers and toggles
// filter selection on option rows.
func (app *Gui) filterEnter(g *gocui.Gui, v *gocui.View) error {
	rows := app.filteredListRows()
	if app.state.filters.listCursor >= len(rows) {
		return nil
	}
	row := rows[app.state.filters.listCursor]

	if row.option == "" {
		app.state.filters.expanded[row.category] = !app.state.filters.expanded[row.category]
		app.clearInlineDesc()
	} else {
		app.toggleApplied(row.category, row.option)
		app.applyFilters()
	}
	app.renderFiltersView(g)
	return nil
}

// filterToggleOption toggles filter selection on the option under the cursor.
// On category headers it does nothing (use enter to expand/collapse).
func (app *Gui) filterToggleOption(g *gocui.Gui, v *gocui.View) error {
	rows := app.filteredListRows()
	if app.state.filters.listCursor >= len(rows) {
		return nil
	}
	row := rows[app.state.filters.listCursor]
	if row.option == "" {
		return nil
	}
	app.toggleApplied(row.category, row.option)
	app.applyFilters()
	app.renderFiltersView(g)
	return nil
}

// filterClearAll removes all applied filter selections at once.
func (app *Gui) filterClearAll(g *gocui.Gui, v *gocui.View) error {
	if len(app.state.filters.applied) == 0 {
		return nil
	}
	app.state.filters.applied = map[filterCategory]map[string]bool{}
	app.applyFilters()
	app.renderFiltersView(g)
	return nil
}

// filterExpand expands the category header under the cursor. On an option
// row it is a no-op (the option is already "inside" an expanded category).
func (app *Gui) filterExpand(g *gocui.Gui, v *gocui.View) error {
	rows := app.filteredListRows()
	if app.state.filters.listCursor >= len(rows) {
		return nil
	}
	row := rows[app.state.filters.listCursor]
	if row.option != "" {
		return nil
	}
	if !app.state.filters.expanded[row.category] {
		app.state.filters.expanded[row.category] = true
		app.clearInlineDesc()
		app.renderFiltersView(g)
	}
	return nil
}

// filterCollapse collapses the current category. When the cursor is on an
// option row it collapses the parent category and moves the cursor to it.
func (app *Gui) filterCollapse(g *gocui.Gui, v *gocui.View) error {
	rows := app.filteredListRows()
	if app.state.filters.listCursor >= len(rows) {
		return nil
	}
	row := rows[app.state.filters.listCursor]

	if row.option != "" {
		for i := app.state.filters.listCursor - 1; i >= 0; i-- {
			if rows[i].option == "" {
				app.state.filters.listCursor = i
				row = rows[i]
				break
			}
		}
	}
	if row.option == "" && app.state.filters.expanded[row.category] {
		app.state.filters.expanded[row.category] = false
		app.clearInlineDesc()
		app.renderFiltersView(g)
	}
	return nil
}

// filterEsc clears the search query when active, or collapses the current
// category if the cursor is on a child option row.
func (app *Gui) filterEsc(g *gocui.Gui, v *gocui.View) error {
	if app.state.filters.filterQuery != "" {
		app.state.filters.filterQuery = ""
		app.state.filters.listCursor = 0
		app.renderFiltersView(g)
		return nil
	}
	return app.filterCollapse(g, v)
}

// filterOpenSearch opens the input prompt to search filter options across all
// searchable categories simultaneously.
func (app *Gui) filterOpenSearch(g *gocui.Gui, v *gocui.View) error {
	prevQuery := app.state.filters.filterQuery
	app.openInput("Search filters (/)", app.state.filters.filterQuery,
		func(value string) {
			app.g.Update(func(g *gocui.Gui) error {
				app.state.filters.filterQuery = value
				app.state.filters.listCursor = 0
				app.renderFiltersView(g)
				return nil
			})
		},
		func() {
			app.g.Update(func(g *gocui.Gui) error {
				app.state.filters.filterQuery = prevQuery
				app.state.filters.listCursor = 0
				app.renderFiltersView(g)
				return nil
			})
		},
		func(value string) {
			app.state.filters.filterQuery = value
			app.state.filters.listCursor = 0
			app.renderFiltersView(app.g)
		},
	)
	return nil
}

// filterToggleInfo opens/closes the info popup for the currently highlighted
// skill/domain/module option in the filter tree.
func (app *Gui) filterToggleInfo(g *gocui.Gui, v *gocui.View) error {
	rows := app.filteredListRows()
	fs := &app.state.filters
	if fs.listCursor >= len(rows) {
		return nil
	}
	row := rows[fs.listCursor]
	if row.option == "" {
		return nil
	}

	var ct oasf.ClassType
	switch row.category {
	case filterSkills:
		ct = oasf.ClassTypeSkill
	case filterDomains:
		ct = oasf.ClassTypeDomain
	case filterModules:
		ct = oasf.ClassTypeModule
	default:
		return nil
	}

	name := row.option
	if fs.inlineDesc == name {
		_ = app.closeInfoPopup(g, v)
		return app.focusTo(g, viewFilters)
	}

	fs.inlineDesc = name
	fs.inlineDescText = ""
	fs.inlineDescLoading = true
	app.openInfoPopup(g, viewFilters)

	go app.fetchInlineDesc(ct, name)
	return nil
}

func (app *Gui) fetchInlineDesc(ct oasf.ClassType, name string) {
	client := app.state.oasfClient
	if client == nil {
		app.g.Update(func(g *gocui.Gui) error {
			if app.state.filters.inlineDesc != name {
				return nil
			}
			app.state.filters.inlineDescLoading = false
			app.state.filters.inlineDescText = "OASF not configured"
			app.state.filters.inlineDescError = true
			app.renderInfoPopup(g)
			return nil
		})
		return
	}

	schemaVer := app.state.classEntriesVer
	info, err := client.Fetch(context.Background(), ct, name, schemaVer)
	app.g.Update(func(g *gocui.Gui) error {
		if app.state.filters.inlineDesc != name {
			return nil
		}
		app.state.filters.inlineDescLoading = false
		if err != nil {
			app.state.filters.inlineDescText = err.Error()
			app.state.filters.inlineDescError = true
		} else {
			maxX, _ := g.Size()
			descW := maxX - app.leftColumnWidth(maxX) - 4
			if descW < 40 {
				descW = 40
			}
			app.state.filters.inlineDescText = formatClassInfo(info, descW, app.theme)
			app.state.filters.inlineDescError = false
		}
		app.renderInfoPopup(g)
		return nil
	})
}

// formatClassInfo produces a pre-formatted, ANSI-colored text block showing
// the class hierarchy tree and description.
func formatClassInfo(info *oasf.ClassInfo, descW int, t Theme) string {
	var sb strings.Builder

	const pad = "           "
	fmt.Fprintf(&sb, "%sTaxonomy:%s ", t.Color1, t.Reset)
	ancestors := info.Ancestors
	for depth, a := range ancestors {
		prefix := strings.Repeat("    ", depth)
		connector := "└── "
		if depth == 0 {
			connector = ""
		}
		fmt.Fprintf(&sb, "%s%s%s%s%s %s(%d)%s",
			prefix, t.Color2, connector, t.Color1, a.Caption, t.Color10, a.ID, t.Reset)
		sb.WriteString("\n" + pad)
	}

	selfPrefix := strings.Repeat("    ", len(ancestors))
	selfConnector := "└── "
	if len(ancestors) == 0 {
		selfConnector = ""
	}
	caption := info.Caption
	if caption == "" {
		caption = info.Name
	}
	fmt.Fprintf(&sb, "%s%s%s%s%s %s(%d)%s",
		selfPrefix, t.Color2, selfConnector, t.Color1, caption, t.Color10, info.ID, t.Reset)

	if info.Description != "" {
		const descLabel = "Description: "
		desc := strings.ReplaceAll(info.Description, "\n", " ")
		desc = strings.Join(strings.Fields(desc), " ")
		contentW := descW - len(descLabel)
		if contentW < 10 {
			contentW = 10
		}
		lines := wrapText(desc, contentW)
		if len(lines) > 0 {
			descPad := strings.Repeat(" ", len(descLabel))
			fmt.Fprintf(&sb, "\n%sDescription:%s %s", t.Color5, t.Reset, lines[0])
			for _, dl := range lines[1:] {
				fmt.Fprintf(&sb, "\n%s%s", descPad, dl)
			}
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

func (app *Gui) clearInlineDesc() {
	app.state.filters.inlineDesc = ""
	app.state.filters.inlineDescText = ""
	app.state.filters.inlineDescError = false
	app.state.filters.inlineDescLoading = false
}
