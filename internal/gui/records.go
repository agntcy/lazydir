// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/agntcy/lazydir/internal/dirclient"
	"github.com/jesseduffield/gocui"
)

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
	rec := row.record
	if rec.CID == "" {
		return nil
	}
	subtitle := rec.Name
	if subtitle == "" {
		subtitle = rec.CID
	}
	if rec.Version != "" {
		subtitle += " " + rec.Version
	}
	go app.pullRecord(subtitle, rec.CID)
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

// recordDelete opens a confirmation popup for the currently selected record.
// On confirmation, it issues a Delete RPC and removes the record from local
// caches so the UI updates immediately without a full refresh.
func (app *Gui) recordDelete(g *gocui.Gui, v *gocui.View) error {
	r := app.cursorRecord()
	if r == nil || r.CID == "" {
		return nil
	}
	if app.state.client == nil {
		return nil
	}

	name := r.Name
	if name == "" {
		name = r.CID
	}
	version := ""
	if r.Version != "" {
		version = " " + r.Version
	}

	body := fmt.Sprintf("Delete %s%s?\n\n  %sy%s  confirm   %sn / esc%s  cancel",
		name, version,
		app.theme.Color2, app.theme.Reset,
		app.theme.Color2, app.theme.Reset)

	cid := r.CID
	app.openConfirmPopup(g, "Delete record", body, func() {
		go app.deleteRecord(cid)
	})
	return nil
}

func (app *Gui) deleteRecord(cid string) {
	client := app.state.client
	if client == nil {
		return
	}

	err := client.Delete(context.Background(), cid)
	app.g.Update(func(g *gocui.Gui) error {
		if err != nil {
			app.state.recordInfoCID = cid
			app.state.recordInfoText = err.Error()
			app.state.recordInfoError = true
			app.state.recordInfoLoading = false
			app.openInfoPopup(g, viewRecords)
			return nil
		}
		app.removeRecordFromState(cid)
		app.renderRecordsView(g)
		app.renderFiltersView(g)
		app.autoPreviewRecord(g)
		return nil
	})
}

// removeRecordFromState purges a record by CID from fullCache, records,
// and filteredRecords, rebuilds display rows, and refreshes active filter
// values so the Filters panel stays consistent.
func (app *Gui) removeRecordFromState(cid string) {
	app.state.fullCache = removeRecordByCID(app.state.fullCache, cid)
	app.state.records = removeRecordByCID(app.state.records, cid)
	app.state.filteredRecords = removeRecordByCID(app.state.filteredRecords, cid)
	if app.state.activeFilterValues != nil {
		app.rebuildActiveFilterValues()
	}
	app.buildRecordDisplayRows()
	if max := len(app.state.recordDisplayRows) - 1; app.state.recordCursor > max && max >= 0 {
		app.state.recordCursor = max
	}
}

func removeRecordByCID(records []*dirclient.RecordSummary, cid string) []*dirclient.RecordSummary {
	out := make([]*dirclient.RecordSummary, 0, len(records))
	for _, r := range records {
		if r.CID != cid {
			out = append(out, r)
		}
	}
	return out
}

func (app *Gui) clearRecordInlineDesc() {
	app.state.recordInfoCID = ""
	app.state.recordInfoText = ""
	app.state.recordInfoError = false
	app.state.recordInfoLoading = false
}
