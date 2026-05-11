// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"github.com/agntcy/lazydir/internal/config"
	"github.com/agntcy/lazydir/internal/dirclient"
	"github.com/agntcy/lazydir/internal/oasf"
	"github.com/jesseduffield/gocui"
)

func (app *Gui) bindKeys(g *gocui.Gui) error {
	// ── Global ───────────────────────────────────────────────────────────────
	for _, key := range []interface{}{gocui.KeyCtrlC, 'q'} {
		if err := g.SetKeybinding("", key, gocui.ModNone, quit); err != nil {
			return err
		}
	}
	if err := g.SetKeybinding("", gocui.KeyTab, gocui.ModNone, app.cycleFocusForward); err != nil {
		return err
	}
	if err := g.SetKeybinding("", gocui.KeyBacktab, gocui.ModNone, app.cycleFocusBackward); err != nil {
		return err
	}
	if err := g.SetKeybinding("", '1', gocui.ModNone, app.focusView(viewDirectory)); err != nil {
		return err
	}
	if err := g.SetKeybinding("", '2', gocui.ModNone, app.focusView(viewFilters)); err != nil {
		return err
	}
	if err := g.SetKeybinding("", '3', gocui.ModNone, app.focusView(viewRecords)); err != nil {
		return err
	}
	if err := g.SetKeybinding("", '0', gocui.ModNone, app.focusView(viewPreview)); err != nil {
		return err
	}
	if err := g.SetKeybinding("", 'r', gocui.ModNone, app.refresh); err != nil {
		return err
	}

	// ── Input prompt (shared) ────────────────────────────────────────────────
	// enter and esc are static; the actual work is done via the onConfirm/onCancel
	// callbacks set at open time — no dynamic key binding ever needed.
	if err := g.SetKeybinding(viewInput, gocui.KeyEnter, gocui.ModNone, app.inputConfirm); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewInput, gocui.KeyEsc, gocui.ModNone, app.inputCancel); err != nil {
		return err
	}
	// Let ctrl+c quit even from the input view.
	if err := g.SetKeybinding(viewInput, gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}

	// ── Connections panel ────────────────────────────────────────────────────
	for _, key := range []interface{}{gocui.KeyArrowUp, 'k'} {
		if err := g.SetKeybinding(viewDirectory, key, gocui.ModNone, app.connCursorUp); err != nil {
			return err
		}
	}
	for _, key := range []interface{}{gocui.KeyArrowDown, 'j'} {
		if err := g.SetKeybinding(viewDirectory, key, gocui.ModNone, app.connCursorDown); err != nil {
			return err
		}
	}
	if err := g.SetKeybinding(viewDirectory, 'c', gocui.ModNone, app.openServerSelectPopup); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewDirectory, gocui.KeyEnter, gocui.ModNone, app.openServerSelectPopup); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewDirectory, 'i', gocui.ModNone, app.connToggleInfo); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewDirectory, gocui.KeyEsc, gocui.ModNone, app.connDismissInfo); err != nil {
		return err
	}

	// ── Server selection popup ──────────────────────────────────────────────
	for _, key := range []interface{}{gocui.KeyArrowUp, 'k'} {
		if err := g.SetKeybinding(viewServerMenu, key, gocui.ModNone, app.serverMenuUp); err != nil {
			return err
		}
	}
	for _, key := range []interface{}{gocui.KeyArrowDown, 'j'} {
		if err := g.SetKeybinding(viewServerMenu, key, gocui.ModNone, app.serverMenuDown); err != nil {
			return err
		}
	}
	if err := g.SetKeybinding(viewServerMenu, gocui.KeyEnter, gocui.ModNone, app.serverMenuSelect); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewServerMenu, gocui.KeyEsc, gocui.ModNone, app.serverMenuClose); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewServerMenu, 'q', gocui.ModNone, app.serverMenuClose); err != nil {
		return err
	}

	// ── Auth/error popup ────────────────────────────────────────────────────
	if err := g.SetKeybinding(viewAuthPopup, gocui.KeyEsc, gocui.ModNone, app.dismissAuthPopup); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewAuthPopup, gocui.KeyEnter, gocui.ModNone, app.dismissAuthPopup); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewAuthPopup, 'q', gocui.ModNone, app.dismissAuthPopup); err != nil {
		return err
	}

	// ── Filters panel ────────────────────────────────────────────────────────
	for _, key := range []interface{}{gocui.KeyArrowUp, 'k'} {
		if err := g.SetKeybinding(viewFilters, key, gocui.ModNone, app.filterCursorUp); err != nil {
			return err
		}
	}
	for _, key := range []interface{}{gocui.KeyArrowDown, 'j'} {
		if err := g.SetKeybinding(viewFilters, key, gocui.ModNone, app.filterCursorDown); err != nil {
			return err
		}
	}
	if err := g.SetKeybinding(viewFilters, gocui.KeyEnter, gocui.ModNone, app.filterEnter); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewFilters, gocui.KeySpace, gocui.ModNone, app.filterToggleOption); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewFilters, gocui.KeyEsc, gocui.ModNone, app.filterEsc); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewFilters, '/', gocui.ModNone, app.filterOpenSearch); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewFilters, 'i', gocui.ModNone, app.filterToggleInfo); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewFilters, 'x', gocui.ModNone, app.filterClearAll); err != nil {
		return err
	}

	// ── Records panel ────────────────────────────────────────────────────────
	for _, key := range []interface{}{gocui.KeyArrowUp, 'k'} {
		if err := g.SetKeybinding(viewRecords, key, gocui.ModNone, app.recordCursorUp); err != nil {
			return err
		}
	}
	for _, key := range []interface{}{gocui.KeyArrowDown, 'j'} {
		if err := g.SetKeybinding(viewRecords, key, gocui.ModNone, app.recordCursorDown); err != nil {
			return err
		}
	}
	if err := g.SetKeybinding(viewRecords, gocui.KeyEnter, gocui.ModNone, app.recordSelect); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewRecords, '/', gocui.ModNone, app.openFilterDialog); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewRecords, gocui.KeyEsc, gocui.ModNone, app.clearFilter); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewRecords, 'i', gocui.ModNone, app.recordToggleInfo); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewRecords, 'y', gocui.ModNone, app.openCopyMenu); err != nil {
		return err
	}

	// ── Info popup ──────────────────────────────────────────────────────────
	if err := g.SetKeybinding(viewInfoPopup, gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewInfoPopup, gocui.KeyEsc, gocui.ModNone, app.infoPopupClose); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewInfoPopup, 'i', gocui.ModNone, app.infoPopupClose); err != nil {
		return err
	}
	for _, key := range []interface{}{gocui.KeyArrowUp, 'k'} {
		if err := g.SetKeybinding(viewInfoPopup, key, gocui.ModNone, app.previewScrollUp); err != nil {
			return err
		}
	}
	for _, key := range []interface{}{gocui.KeyArrowDown, 'j'} {
		if err := g.SetKeybinding(viewInfoPopup, key, gocui.ModNone, app.previewScrollDown); err != nil {
			return err
		}
	}

	// ── Copy menu popup ─────────────────────────────────────────────────────
	if err := g.SetKeybinding(viewCopyMenu, 'c', gocui.ModNone, app.copyCID); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewCopyMenu, 'a', gocui.ModNone, app.copyRecordJSON); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewCopyMenu, gocui.KeyEsc, gocui.ModNone, app.closeCopyMenu); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewCopyMenu, gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}

	// ── Preview panel ────────────────────────────────────────────────────────
	for _, key := range []interface{}{gocui.KeyArrowUp, 'k'} {
		if err := g.SetKeybinding(viewPreview, key, gocui.ModNone, app.previewScrollUp); err != nil {
			return err
		}
	}
	for _, key := range []interface{}{gocui.KeyArrowDown, 'j'} {
		if err := g.SetKeybinding(viewPreview, key, gocui.ModNone, app.previewScrollDown); err != nil {
			return err
		}
	}
	if err := g.SetKeybinding(viewPreview, gocui.MouseWheelUp, gocui.ModNone, app.previewScrollUp); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewPreview, gocui.MouseWheelDown, gocui.ModNone, app.previewScrollDown); err != nil {
		return err
	}

	// Mouse wheel scrolling on list panels
	if err := g.SetKeybinding(viewFilters, gocui.MouseWheelUp, gocui.ModNone, app.filterCursorUp); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewFilters, gocui.MouseWheelDown, gocui.ModNone, app.filterCursorDown); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewRecords, gocui.MouseWheelUp, gocui.ModNone, app.recordCursorUp); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewRecords, gocui.MouseWheelDown, gocui.ModNone, app.recordCursorDown); err != nil {
		return err
	}

	// Mouse click focuses the clicked panel; records and filters get
	// specialised handlers that also update the cursor / open categories.
	for _, name := range []string{viewDirectory, viewPreview} {
		n := name
		if err := g.SetKeybinding(n, gocui.MouseLeft, gocui.ModNone, app.focusView(n)); err != nil {
			return err
		}
	}
	if err := g.SetKeybinding(viewRecords, gocui.MouseLeft, gocui.ModNone, app.recordMouseClick); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewFilters, gocui.MouseLeft, gocui.ModNone, app.filterMouseClick); err != nil {
		return err
	}

	// ? opens help popup for all main panels
	for _, name := range []string{"", viewDirectory, viewFilters, viewRecords, viewPreview} {
		if err := g.SetKeybinding(name, '?', gocui.ModNone, app.openHelp); err != nil {
			return err
		}
	}
	// esc and ? close the help popup
	if err := g.SetKeybinding(viewHelp, gocui.KeyEsc, gocui.ModNone, app.closeHelp); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewHelp, '?', gocui.ModNone, app.closeHelp); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewHelp, gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}
	// scroll help popup
	for _, key := range []interface{}{gocui.KeyArrowUp, 'k'} {
		if err := g.SetKeybinding(viewHelp, key, gocui.ModNone, app.previewScrollUp); err != nil {
			return err
		}
	}
	for _, key := range []interface{}{gocui.KeyArrowDown, 'j'} {
		if err := g.SetKeybinding(viewHelp, key, gocui.ModNone, app.previewScrollDown); err != nil {
			return err
		}
	}

	return nil
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}

// ── Input prompt handlers ─────────────────────────────────────────────────────

func (app *Gui) inputConfirm(g *gocui.Gui, v *gocui.View) error {
	value := strings.TrimSpace(v.TextArea.GetContent())
	cb := app.state.onInputConfirm
	app.closeInput()
	if cb != nil {
		cb(value)
	}
	return nil
}

func (app *Gui) inputCancel(g *gocui.Gui, v *gocui.View) error {
	cb := app.state.onInputCancel
	app.closeInput()
	if cb != nil {
		cb()
	}
	return nil
}

// ── Focus helpers ─────────────────────────────────────────────────────────────

var focusOrder = []string{viewDirectory, viewFilters, viewRecords, viewPreview}

// focusTo sets the current view and updates highlight state on list panels.
// When focus arrives at or leaves the records panel, the preview is refreshed
// so it always shows the record under the cursor.
func (app *Gui) focusTo(g *gocui.Gui, name string) error {
	_, err := g.SetCurrentView(name)
	if err != nil {
		return err
	}
	app.syncHighlight(g, name)
	app.renderStatus(g)
	app.autoPreviewRecord(g)
	return nil
}

func (app *Gui) focusView(name string) func(*gocui.Gui, *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		return app.focusTo(g, name)
	}
}

func (app *Gui) cycleFocusForward(g *gocui.Gui, v *gocui.View) error {
	return app.cycleFocus(g, 1)
}

func (app *Gui) cycleFocusBackward(g *gocui.Gui, v *gocui.View) error {
	return app.cycleFocus(g, -1)
}

func (app *Gui) cycleFocus(g *gocui.Gui, dir int) error {
	cur := g.CurrentView()
	curName := ""
	if cur != nil {
		curName = cur.Name()
	}
	idx := 0
	for i, name := range focusOrder {
		if name == curName {
			idx = i
			break
		}
	}
	next := (idx + dir + len(focusOrder)) % len(focusOrder)
	return app.focusTo(g, focusOrder[next])
}

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

// filterEsc clears the search query when active. Otherwise it does nothing —
// filters are removed by toggling them off with enter.
func (app *Gui) filterEsc(g *gocui.Gui, v *gocui.View) error {
	if app.state.filters.filterQuery != "" {
		app.state.filters.filterQuery = ""
		app.state.filters.listCursor = 0
		app.renderFiltersView(g)
		return nil
	}
	return nil
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

// ── Records panel handlers ────────────────────────────────────────────────────

// cursorRecord returns the record under the current cursor position, or nil
// if the cursor is on a group header or out of range.
func (app *Gui) cursorRecord() *dirclient.RecordSummary {
	rows := app.state.recordDisplayRows
	if app.state.recordCursor >= len(rows) {
		return nil
	}
	return rows[app.state.recordCursor].record
}

func (app *Gui) recordMouseClick(g *gocui.Gui, v *gocui.View) error {
	app.hideInfoPopupIfVisible(g)
	if err := app.focusTo(g, viewRecords); err != nil {
		return err
	}
	_, cy := v.Cursor()
	_, oy := v.Origin()
	idx := oy + cy
	rows := app.state.recordDisplayRows
	if idx >= 0 && idx < len(rows) {
		app.state.recordCursor = idx
		row := rows[idx]
		if row.record == nil {
			app.state.recordGroupExpanded[row.groupName] = !app.state.recordGroupExpanded[row.groupName]
			app.buildRecordDisplayRows()
		}
		app.renderRecordsView(g)
		app.autoPreviewRecord(g)
	}
	return nil
}

func (app *Gui) recordCursorUp(g *gocui.Gui, v *gocui.View) error {
	if app.state.recordCursor > 0 {
		app.state.recordCursor--
		app.renderRecordsView(g)
		app.autoPreviewRecord(g)
	}
	return nil
}

func (app *Gui) recordCursorDown(g *gocui.Gui, v *gocui.View) error {
	rows := app.state.recordDisplayRows
	if app.state.recordCursor < len(rows)-1 {
		app.state.recordCursor++
		app.renderRecordsView(g)
		app.autoPreviewRecord(g)
	}
	return nil
}

func (app *Gui) recordSelect(g *gocui.Gui, v *gocui.View) error {
	rows := app.state.recordDisplayRows
	if app.state.recordCursor >= len(rows) {
		return nil
	}
	row := rows[app.state.recordCursor]
	if row.record == nil {
		app.state.recordGroupExpanded[row.groupName] = !app.state.recordGroupExpanded[row.groupName]
		app.buildRecordDisplayRows()
		app.renderRecordsView(g)
		app.autoPreviewRecord(g)
		return nil
	}
	cid := row.record.CID
	if cid == "" {
		return nil
	}
	go app.pullRecord(cid)
	return nil
}

func (app *Gui) openFilterDialog(g *gocui.Gui, v *gocui.View) error {
	prevQuery := app.state.filterQuery
	app.openInput("Filter records (/)", app.state.filterQuery,
		func(value string) {
			app.g.Update(func(g *gocui.Gui) error {
				app.state.filterQuery = value
				app.state.recordCursor = 0
				app.applyNameFilter()
				app.renderRecordsView(g)
				return nil
			})
		},
		func() {
			app.g.Update(func(g *gocui.Gui) error {
				app.state.filterQuery = prevQuery
				app.state.recordCursor = 0
				app.applyNameFilter()
				app.renderRecordsView(g)
				return nil
			})
		},
		func(value string) {
			app.state.filterQuery = value
			app.state.recordCursor = 0
			app.applyNameFilter()
			app.renderRecordsView(app.g)
		},
	)
	return nil
}

func (app *Gui) clearFilter(g *gocui.Gui, v *gocui.View) error {
	app.state.filterQuery = ""
	app.state.recordCursor = 0
	app.applyNameFilter()
	app.renderRecordsView(g)
	return nil
}

// recordToggleInfo opens/closes the info popup for the currently highlighted
// record, fetching details via the directory's PullInfo RPC.
func (app *Gui) recordToggleInfo(g *gocui.Gui, v *gocui.View) error {
	r := app.cursorRecord()
	if r == nil {
		return nil
	}
	cid := r.CID
	if cid == "" {
		return nil
	}

	if app.state.recordInfoCID == cid {
		_ = app.closeInfoPopup(g, v)
		return app.focusTo(g, viewRecords)
	}

	app.state.recordInfoCID = cid
	app.state.recordInfoText = ""
	app.state.recordInfoLoading = true
	app.openInfoPopup(g, viewRecords)
	_, _ = g.SetCurrentView(viewInfoPopup)

	go app.fetchRecordInfo(cid)
	return nil
}

func (app *Gui) fetchRecordInfo(cid string) {
	client := app.state.client
	if client == nil {
		return
	}

	info, err := client.PullInfo(context.Background(), cid)
	app.g.Update(func(g *gocui.Gui) error {
		if app.state.recordInfoCID != cid {
			return nil
		}
		app.state.recordInfoLoading = false
		if err != nil {
			app.state.recordInfoText = err.Error()
			app.state.recordInfoError = true
		} else {
			app.state.recordInfoText = formatRecordInfo(info, app.theme)
			app.state.recordInfoError = false
		}
		app.renderInfoPopup(g)
		return nil
	})
}

// formatRecordInfo renders a RecordInfo as colored, human-readable lines.
// The CID is omitted here because it's already shown in the preview panel title.
func formatRecordInfo(info *dirclient.RecordInfo, t Theme) string {
	var sb strings.Builder
	first := true

	if len(info.Annotations) > 0 {
		fmt.Fprintf(&sb, "%sAnnotations:%s", t.Color1, t.Reset)
		first = false
		keys := make([]string, 0, len(info.Annotations))
		for k := range info.Annotations {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&sb, "\n%s%s%s:%s %s", indent1, t.Color1, k, t.Reset, info.Annotations[k])
		}
	}

	if info.SchemaVersion != "" {
		if !first {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "%sSchema version:%s %s", t.Color4, t.Reset, info.SchemaVersion)
		first = false
	}
	if info.CreatedAt != "" {
		if !first {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "%sCreated at:%s %s", t.Color3, t.Reset, info.CreatedAt)
	}

	return sb.String()
}

func (app *Gui) clearRecordInlineDesc() {
	app.state.recordInfoCID = ""
	app.state.recordInfoText = ""
	app.state.recordInfoError = false
	app.state.recordInfoLoading = false
}

// ── Preview panel handlers ────────────────────────────────────────────────────

const defaultScrollStep = 3

func (app *Gui) previewScrollUp(g *gocui.Gui, v *gocui.View) error {
	return app.scrollViewUp(v)
}

func (app *Gui) previewScrollDown(g *gocui.Gui, v *gocui.View) error {
	return app.scrollViewDown(v)
}

func (app *Gui) scrollStep() int {
	if app.cfg.ScrollStep > 0 {
		return app.cfg.ScrollStep
	}
	return defaultScrollStep
}

func (app *Gui) scrollViewUp(v *gocui.View) error {
	if v == nil {
		return nil
	}
	step := app.scrollStep()
	_, oy := v.Origin()
	if oy > 0 {
		_ = v.SetOrigin(0, oy-step)
	}
	return nil
}

func (app *Gui) scrollViewDown(v *gocui.View) error {
	if v == nil {
		return nil
	}
	step := app.scrollStep()
	_, oy := v.Origin()
	_ = v.SetOrigin(0, oy+step)
	return nil
}

// ── Connections: directory and OASF server dialogs ────────────────────────────

// ── Connections panel cursor ──────────────────────────────────────────────────

func (app *Gui) connCursorUp(g *gocui.Gui, v *gocui.View) error {
	if app.state.connCursor > 0 {
		app.state.connCursor--
		app.renderDirectory(g)
	}
	return nil
}

func (app *Gui) connCursorDown(g *gocui.Gui, v *gocui.View) error {
	if app.state.connCursor < 1 {
		app.state.connCursor++
		app.renderDirectory(g)
	}
	return nil
}

func (app *Gui) connToggleInfo(g *gocui.Gui, v *gocui.View) error {
	if app.state.infoPopupPanel == viewDirectory {
		_ = app.closeInfoPopup(g, v)
		return app.focusTo(g, viewDirectory)
	}
	app.openInfoPopup(g, viewDirectory)
	_, _ = g.SetCurrentView(viewInfoPopup)
	return nil
}

func (app *Gui) connDismissInfo(g *gocui.Gui, v *gocui.View) error {
	if app.state.infoPopupPanel == viewDirectory {
		_ = app.closeInfoPopup(g, v)
		return app.focusTo(g, viewDirectory)
	}
	return nil
}

// connInfoText builds the info popup content for the selected connection row.
// Returns the text and whether an error is present.
func (app *Gui) connInfoText() (string, bool) {
	t := app.theme
	var sb strings.Builder
	hasError := false
	if app.state.connCursor == 0 {
		fmt.Fprintf(&sb, "%sServer:%s    %s", t.Color1, t.Reset, app.state.serverAddr)
		if app.state.activeDir.OIDCIssuer != "" {
			fmt.Fprintf(&sb, "\n%sAuth:%s      oidc (%s)", t.Color4, t.Reset, app.state.activeDir.OIDCIssuer)
		} else if app.state.authMode != "" {
			fmt.Fprintf(&sb, "\n%sAuth:%s      %s", t.Color4, t.Reset, app.state.authMode)
		} else {
			fmt.Fprintf(&sb, "\n%sAuth:%s      insecure", t.Color4, t.Reset)
		}
		if !app.state.dirLastConnected.IsZero() {
			fmt.Fprintf(&sb, "\n%sConnected:%s %s", t.Color3, t.Reset, app.state.dirLastConnected.Format("15:04:05"))
		}
		if app.state.dirError != "" {
			fmt.Fprintf(&sb, "\n\n%sError:%s %s", t.Color6, t.Reset, app.state.dirError)
			hasError = true
		}
	} else {
		fmt.Fprintf(&sb, "%sServer:%s    %s", t.Color1, t.Reset, app.state.oasfAddr)
		if !app.state.oasfLastConnected.IsZero() {
			fmt.Fprintf(&sb, "\n%sConnected:%s %s", t.Color3, t.Reset, app.state.oasfLastConnected.Format("15:04:05"))
		}
		if app.state.oasfError != "" {
			fmt.Fprintf(&sb, "\n\n%sError:%s %s", t.Color6, t.Reset, app.state.oasfError)
			hasError = true
		}
	}
	return sb.String(), hasError
}

// ── Server selection popup ───────────────────────────────────────────────────

func (app *Gui) openServerSelectPopup(g *gocui.Gui, v *gocui.View) error {
	var items []string
	if app.state.connCursor == 0 {
		for _, entry := range app.cfg.DirectoryServers {
			label := entry.Address
			if entry.OIDCIssuer != "" {
				label += " (oidc)"
			}
			items = append(items, label)
		}
	} else {
		items = append(items, app.cfg.OASFServers...)
	}
	items = append(items, "Custom...")

	app.state.serverMenuItems = items
	app.state.serverMenuCursor = 0
	app.state.serverMenuVisible = true

	smv, err := g.View(viewServerMenu)
	if err == nil {
		smv.Clear()
		for _, item := range items {
			fmt.Fprintln(smv, " "+item)
		}
		_ = smv.SetCursor(0, 0)
	}
	app.showPopup(g, viewServerMenu)
	return nil
}

func (app *Gui) serverMenuUp(g *gocui.Gui, v *gocui.View) error {
	if app.state.serverMenuCursor > 0 {
		app.state.serverMenuCursor--
		_ = v.SetCursor(0, app.state.serverMenuCursor)
	}
	return nil
}

func (app *Gui) serverMenuDown(g *gocui.Gui, v *gocui.View) error {
	if app.state.serverMenuCursor < len(app.state.serverMenuItems)-1 {
		app.state.serverMenuCursor++
		_ = v.SetCursor(0, app.state.serverMenuCursor)
	}
	return nil
}

func (app *Gui) serverMenuClose(g *gocui.Gui, v *gocui.View) error {
	app.state.serverMenuVisible = false
	app.hidePopup(g, viewServerMenu)
	return nil
}

func (app *Gui) serverMenuSelect(g *gocui.Gui, v *gocui.View) error {
	idx := app.state.serverMenuCursor
	if idx < 0 || idx >= len(app.state.serverMenuItems) {
		return app.serverMenuClose(g, v)
	}

	selected := app.state.serverMenuItems[idx]

	// Close the popup first.
	app.state.serverMenuVisible = false
	app.hidePopup(g, viewServerMenu)

	if selected == "Custom..." {
		if app.state.connCursor == 0 {
			return app.openConnectDialog(g, nil)
		}
		return app.openOASFDialog(g, nil)
	}

	if app.state.connCursor == 0 {
		if idx < len(app.cfg.DirectoryServers) {
			app.connectToDirectory(g, app.cfg.DirectoryServers[idx])
		}
	} else {
		if idx < len(app.cfg.OASFServers) {
			app.connectToOASF(g, app.cfg.OASFServers[idx])
		}
	}
	return nil
}

func (app *Gui) connectToDirectory(g *gocui.Gui, entry config.DirectoryEntry) {
	app.stopReconnectLoop()
	if app.state.cancelLoad != nil {
		app.state.cancelLoad()
		app.state.cancelLoad = nil
	}
	app.state.activeDir = entry
	app.state.serverAddr = entry.Address
	app.state.dirStatus = connTrying
	app.state.stream = streamLoading
	app.state.fullCache = nil
	app.state.records = nil
	app.state.filteredRecords = nil
	app.state.recordDisplayRows = nil
	app.state.recordGroupExpanded = map[string]bool{}
	app.state.recordCursor = 0
	app.state.filters = newFilterState()
	app.state.filterQuery = ""
	app.state.filterValues = newFilterValueAggregator()
	app.state.classEntries = nil
	app.state.classEntriesVer = ""
	app.renderDirectory(g)
	app.renderRecordsView(g)
	app.renderFiltersView(g)

	if entry.OIDCIssuer != "" {
		go app.connectWithOIDC(entry)
	} else {
		authMode := app.state.authMode
		if authMode == "" {
			authMode = "insecure"
		}
		cfg := dirclient.Config{
			ServerAddress: entry.Address,
			AuthMode:      authMode,
			TLSSkipVerify: app.cfg.Directory.TLSSkipVerify,
			TLSCAFile:     app.cfg.Directory.TLSCAFile,
			TLSCertFile:   app.cfg.Directory.TLSCertFile,
			TLSKeyFile:    app.cfg.Directory.TLSKeyFile,
			AuthToken:     app.cfg.Directory.AuthToken,
		}
		go app.connect(cfg)
	}
}

func (app *Gui) connectWithOIDC(entry config.DirectoryEntry) {
	ctx := context.Background()

	writer := &oidcDeviceWriter{gui: app.g, app: app}

	token, err := dirclient.EnsureOIDCToken(ctx, entry.OIDCIssuer, entry.OIDCClientID, writer)

	// Close the auth popup regardless of outcome.
	app.g.Update(func(g *gocui.Gui) error {
		app.closeAuthPopup(g)
		return nil
	})

	if err != nil {
		app.g.Update(func(g *gocui.Gui) error {
			app.state.dirStatus = connFailed
			app.state.dirError = err.Error()
			app.state.stream = streamIdle
			app.renderDirectory(g)
			app.openInfoPopup(g, viewDirectory)
			return nil
		})
		return
	}

	cfg := dirclient.Config{
		ServerAddress: entry.Address,
		AuthMode:      "oidc",
		TLSSkipVerify: app.cfg.Directory.TLSSkipVerify,
		TLSCAFile:     app.cfg.Directory.TLSCAFile,
		TLSCertFile:   app.cfg.Directory.TLSCertFile,
		TLSKeyFile:    app.cfg.Directory.TLSKeyFile,
		AuthToken:     token,
		OIDCIssuer:    entry.OIDCIssuer,
		OIDCClientID:  entry.OIDCClientID,
	}
	app.connect(cfg)
}

// oidcDeviceWriter intercepts device flow output, extracts the URL and code,
// copies the code to clipboard, opens the browser, and shows an auth popup.
type oidcDeviceWriter struct {
	gui  *gocui.Gui
	app  *Gui
	buf  string
	done bool
}

func (w *oidcDeviceWriter) Write(p []byte) (n int, err error) {
	w.buf += string(p)

	if w.done {
		return len(p), nil
	}

	url, code := parseDeviceFlowOutput(w.buf)
	if url == "" || code == "" {
		return len(p), nil
	}
	w.done = true

	_ = writeClipboard(code)
	openBrowser(url)

	w.gui.Update(func(g *gocui.Gui) error {
		w.app.showAuthPopup(g, url, code)
		return nil
	})

	return len(p), nil
}

// parseDeviceFlowOutput extracts verification URL and user code from the
// upstream device flow output format:
// "\nEnter this code at <URL>:\n\n  <CODE>\n\nWaiting for authorization...\n"
func parseDeviceFlowOutput(s string) (url, code string) {
	const prefix = "Enter this code at "
	idx := strings.Index(s, prefix)
	if idx < 0 {
		return "", ""
	}
	rest := s[idx+len(prefix):]

	// The URL ends with ":\n" — find that boundary (not just any colon).
	colonNewline := strings.Index(rest, ":\n")
	if colonNewline < 0 {
		return "", ""
	}
	url = strings.TrimSpace(rest[:colonNewline])

	rest = rest[colonNewline+2:]
	waitIdx := strings.Index(rest, "Waiting for authorization")
	if waitIdx < 0 {
		return url, ""
	}
	code = strings.TrimSpace(rest[:waitIdx])
	return url, code
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func (app *Gui) showAuthPopup(g *gocui.Gui, url, code string) {
	content := fmt.Sprintf(
		" Authenticate in your browser:\n\n   %s\n\n Code: %s  (copied to clipboard)\n\n Waiting for authorization...",
		url, code,
	)
	app.state.authPopupText = content

	v, err := g.View(viewAuthPopup)
	if err != nil {
		return
	}
	v.Clear()
	fmt.Fprint(v, content)
	app.showPopup(g, viewAuthPopup)
}

func (app *Gui) closeAuthPopup(g *gocui.Gui) {
	app.state.authPopupText = ""
	app.hidePopup(g, viewAuthPopup)
}

func (app *Gui) dismissAuthPopup(g *gocui.Gui, v *gocui.View) error {
	app.closeAuthPopup(g)
	return nil
}

func (app *Gui) connectToOASF(g *gocui.Gui, addr string) {
	app.stopOASFReconnectLoop()
	client, err := oasf.NewClient(oasf.Config{ServerAddress: addr})
	if err != nil {
		app.state.oasfStatus = connFailed
		app.state.oasfError = err.Error()
		app.state.oasfAddr = addr
		app.renderDirectory(g)
		return
	}
	app.state.oasfClient = client
	app.state.oasfAddr = addr
	app.state.oasfStatus = connTrying
	app.state.oasfError = ""
	app.state.classEntries = nil
	app.state.classEntriesVer = ""
	app.renderDirectory(g)
	go app.pingOASF(client)
}

func (app *Gui) openConnectDialog(g *gocui.Gui, v *gocui.View) error {
	app.openInput("Connect to directory (enter addr)", app.state.serverAddr,
		func(addr string) {
			if addr == "" {
				return
			}
			app.connectToDirectory(g, config.DirectoryEntry{Address: addr})
		},
		nil, nil,
	)
	return nil
}

// openOASFDialog prompts the user for a new OASF schema server URL. On confirm
// a fresh oasf.Client is constructed and any cached class info is dropped.
func (app *Gui) openOASFDialog(g *gocui.Gui, v *gocui.View) error {
	app.openInput("Connect to OASF server (enter URL)", app.state.oasfAddr,
		func(addr string) {
			if addr == "" {
				return
			}
			app.connectToOASF(g, addr)
		},
		nil, nil,
	)
	return nil
}

// ── Refresh ───────────────────────────────────────────────────────────────────

func (app *Gui) refresh(g *gocui.Gui, v *gocui.View) error {
	if app.state.client == nil {
		return nil
	}
	app.startRecordsStream()
	return nil
}

// ── Async actions ─────────────────────────────────────────────────────────────

func (app *Gui) pullRecord(cid string) {
	ctx := context.Background()
	jsonStr, err := app.state.client.PullJSON(ctx, cid)
	app.g.Update(func(g *gocui.Gui) error {
		if err != nil {
			app.renderPreviewText(g, "Error", err.Error())
			return nil
		}
		app.renderPreviewJSON(g, cid, jsonStr)
		return nil
	})
}

// autoPreviewRecord fires a background pull for the record currently under the
// cursor, resetting the preview scroll position first. For group headers it
// previews the first record in the group.
func (app *Gui) autoPreviewRecord(g *gocui.Gui) {
	rows := app.state.recordDisplayRows
	if app.state.recordCursor >= len(rows) {
		return
	}
	row := rows[app.state.recordCursor]
	var cid string
	if row.record != nil {
		cid = row.record.CID
	} else {
		cid = app.firstCIDInGroup(row.groupName)
	}
	if cid == "" {
		return
	}
	if pv, err := g.View(viewPreview); err == nil {
		_ = pv.SetOrigin(0, 0)
	}
	go app.pullRecord(cid)
}

// firstCIDInGroup returns the CID of the latest-version record in the named
// group. When expanded, the first child row is already the latest (sorted).
// When collapsed, it scans filteredRecords and picks the highest version.
func (app *Gui) firstCIDInGroup(name string) string {
	if app.state.recordGroupExpanded[name] {
		found := false
		for _, row := range app.state.recordDisplayRows {
			if row.record == nil && row.groupName == name {
				found = true
				continue
			}
			if found {
				if row.record != nil {
					return row.record.CID
				}
				break
			}
		}
	}
	var best *dirclient.RecordSummary
	for _, r := range app.state.filteredRecords {
		rName := r.Name
		if rName == "" {
			rName = r.CID
		}
		if rName != name {
			continue
		}
		if best == nil || compareVersions(r.Version, best.Version) > 0 {
			best = r
		}
	}
	if best != nil {
		return best.CID
	}
	return ""
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
	_, _ = g.SetCurrentView(viewInfoPopup)

	go app.fetchInlineDesc(ct, name)
	return nil
}

// fetchInlineDesc fetches the OASF description for name and stores it in
// the filter info state, triggering a popup re-render on completion.
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
// the class hierarchy tree and description, similar to record info display.
func formatClassInfo(info *oasf.ClassInfo, descW int, t Theme) string {
	var sb strings.Builder

	// Taxonomy header + hierarchy tree (first ancestor on the same line)
	const pad = "           " // len("Taxonomy: ") + 1
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

	// Description (first line on the same line as the label)
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

// clearInlineDesc resets the inline description toggle state.
func (app *Gui) clearInlineDesc() {
	app.state.filters.inlineDesc = ""
	app.state.filters.inlineDescText = ""
	app.state.filters.inlineDescError = false
	app.state.filters.inlineDescLoading = false
}

// ── Popup helpers (shared open/close logic) ──────────────────────────────────

// showPopup makes a popup view visible, brings it to the top, tracks the
// previous view for restoring focus later, and sets it as the current view.
func (app *Gui) showPopup(g *gocui.Gui, name string) {
	v, err := g.View(name)
	if err != nil {
		return
	}
	if cv := g.CurrentView(); cv != nil && cv.Name() != name {
		app.state.popupPrevView = cv.Name()
	}
	v.Visible = true
	_, _ = g.SetViewOnTop(name)
	_, _ = g.SetCurrentView(name)
}

// hidePopup hides a popup view and restores focus to the previously active view.
// It also resets g.SelFrameColor/SelFgColor in case the popup had overridden them.
func (app *Gui) hidePopup(g *gocui.Gui, name string) {
	v, err := g.View(name)
	if err != nil {
		return
	}
	v.Visible = false
	g.SelFrameColor = app.theme.ActiveBorderColor
	g.SelFgColor = app.theme.ActiveBorderColor
	target := app.state.popupPrevView
	if target == "" {
		target = viewRecords
	}
	_, _ = g.SetCurrentView(target)
}

// ── Info popup ────────────────────────────────────────────────────────────────

// infoPopupClose handles esc/i from within the focused info popup: close it
// and return focus to whichever panel opened it.
func (app *Gui) infoPopupClose(g *gocui.Gui, v *gocui.View) error {
	target := app.state.popupPrevView
	if target == "" {
		target = viewRecords
	}
	_ = app.closeInfoPopup(g, v)
	return app.focusTo(g, target)
}

// hideInfoPopupIfVisible dismisses the info popup without changing focus,
// useful when focus is being moved by another action (e.g. mouse click).
func (app *Gui) hideInfoPopupIfVisible(g *gocui.Gui) {
	ipv, err := g.View(viewInfoPopup)
	if err != nil || !ipv.Visible {
		return
	}
	ipv.Visible = false
	ipv.FrameColor = gocui.ColorDefault
	ipv.TitleColor = gocui.ColorDefault
	g.SelFrameColor = app.theme.ActiveBorderColor
	g.SelFgColor = app.theme.ActiveBorderColor
	app.clearRecordInlineDesc()
	app.clearInlineDesc()
	app.state.popupPrevView = ""
	app.state.infoPopupPanel = ""
}

// openInfoPopup shows the info popup anchored under the selection in sourcePanel.
func (app *Gui) openInfoPopup(g *gocui.Gui, sourcePanel string) {
	ipv, err := g.View(viewInfoPopup)
	if err != nil {
		return
	}
	if cv := g.CurrentView(); cv != nil && cv.Name() != viewInfoPopup {
		app.state.popupPrevView = cv.Name()
	}
	app.state.infoPopupPanel = sourcePanel
	ipv.Clear()
	_ = ipv.SetOrigin(0, 0)
	ipv.Visible = true
	app.renderInfoPopup(g)
	_, _ = g.SetViewOnTop(viewInfoPopup)
}

// closeInfoPopup hides the info popup and resets state.
func (app *Gui) closeInfoPopup(g *gocui.Gui, v *gocui.View) error {
	ipv, err := g.View(viewInfoPopup)
	if err != nil {
		return nil
	}
	ipv.Visible = false
	ipv.FrameColor = gocui.ColorDefault
	ipv.TitleColor = gocui.ColorDefault
	g.SelFrameColor = app.theme.ActiveBorderColor
	g.SelFgColor = app.theme.ActiveBorderColor
	app.clearRecordInlineDesc()
	app.clearInlineDesc()
	app.state.popupPrevView = ""
	app.state.infoPopupPanel = ""
	return nil
}

// renderInfoPopup updates the content of the info popup based on whichever
// panel triggered it.
func (app *Gui) renderInfoPopup(g *gocui.Gui) {
	ipv, err := g.View(viewInfoPopup)
	if err != nil || !ipv.Visible {
		return
	}
	ipv.Clear()
	_ = ipv.SetOrigin(0, 0)

	hasError := false

	switch app.state.infoPopupPanel {
	case viewDirectory:
		text, isErr := app.connInfoText()
		fmt.Fprint(ipv, text)
		hasError = isErr
	case viewFilters:
		if app.state.filters.inlineDescLoading {
			fmt.Fprintf(ipv, "%sloading…%s", app.theme.Color4, app.theme.Reset)
		} else if app.state.filters.inlineDescText != "" {
			fmt.Fprint(ipv, app.state.filters.inlineDescText)
			hasError = app.state.filters.inlineDescError
		}
	case viewRecords:
		if app.state.recordInfoLoading {
			fmt.Fprintf(ipv, "%sloading…%s", app.theme.Color4, app.theme.Reset)
		} else if app.state.recordInfoText != "" {
			fmt.Fprint(ipv, app.state.recordInfoText)
			hasError = app.state.recordInfoError
		}
	}

	if hasError {
		ipv.FrameColor = gocui.ColorRed
		ipv.TitleColor = gocui.ColorRed
	} else {
		ipv.FrameColor = gocui.ColorGreen
		ipv.TitleColor = gocui.ColorGreen
	}
	// Also update global sel colors so they are correct when the popup is
	// (or becomes) the focused view — gocui ignores v.FrameColor/TitleColor
	// for the current view and uses g.SelFrameColor/SelFgColor instead.
	g.SelFrameColor = ipv.FrameColor
	g.SelFgColor = ipv.TitleColor
}

// ── Help popup ────────────────────────────────────────────────────────────────

func (app *Gui) openHelp(g *gocui.Gui, v *gocui.View) error {
	hv, err := g.View(viewHelp)
	if err != nil {
		return nil
	}
	if cv := g.CurrentView(); cv != nil && cv.Name() != viewHelp {
		app.state.popupPrevView = cv.Name()
	}
	hv.Clear()
	_ = hv.SetOrigin(0, 0)
	for _, line := range helpPopupLines(app.state.popupPrevView) {
		fmt.Fprintln(hv, line)
	}
	hv.Visible = true
	_, _ = g.SetCurrentView(viewHelp)
	_, _ = g.SetViewOnTop(viewHelp)
	return nil
}

func (app *Gui) closeHelp(g *gocui.Gui, v *gocui.View) error {
	app.hidePopup(g, viewHelp)
	return nil
}

// ── Copy menu popup ───────────────────────────────────────────────────────────

func (app *Gui) openCopyMenu(g *gocui.Gui, v *gocui.View) error {
	if app.cursorRecord() == nil {
		return nil
	}
	cv, err := g.View(viewCopyMenu)
	if err != nil {
		return nil
	}
	cv.Clear()
	fmt.Fprintf(cv, "  %sc%s  copy CID\n", app.theme.Color2, app.theme.Reset)
	fmt.Fprintf(cv, "  %sa%s  copy record JSON", app.theme.Color2, app.theme.Reset)
	app.showPopup(g, viewCopyMenu)
	return nil
}

func (app *Gui) closeCopyMenu(g *gocui.Gui, v *gocui.View) error {
	app.hidePopup(g, viewCopyMenu)
	return nil
}

func (app *Gui) copyCID(g *gocui.Gui, v *gocui.View) error {
	r := app.cursorRecord()
	if r == nil || r.CID == "" {
		return app.closeCopyMenu(g, v)
	}
	_ = writeClipboard(r.CID)
	return app.closeCopyMenu(g, v)
}

func (app *Gui) copyRecordJSON(g *gocui.Gui, v *gocui.View) error {
	r := app.cursorRecord()
	if r == nil || r.CID == "" {
		return app.closeCopyMenu(g, v)
	}
	cid := r.CID
	if err := app.closeCopyMenu(g, v); err != nil {
		return err
	}
	go app.fetchAndCopyJSON(cid)
	return nil
}

func (app *Gui) fetchAndCopyJSON(cid string) {
	ctx := context.Background()
	jsonStr, err := app.state.client.PullJSON(ctx, cid)
	if err != nil {
		app.g.Update(func(g *gocui.Gui) error {
			app.renderPreviewText(g, "Error", "Failed to fetch record: "+err.Error())
			return nil
		})
		return
	}
	_ = writeClipboard(jsonStr)
}

// writeClipboard writes text to the system clipboard.
func writeClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "windows":
		cmd = exec.Command("clip")
	default:
		cmd = exec.Command("xclip", "-selection", "clipboard")
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
