// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agntcy/lazydir/internal/dirclient"
	"github.com/jesseduffield/gocui"
)

// ── Records panel handlers ────────────────────────────────────────────────────

// cursorRecord returns the record under the current cursor position, or nil
// if the cursor is on a group header, sync-pending row, or out of range.
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

// recordExpand expands the group header under the cursor. On non-group rows
// it is a no-op.
func (app *Gui) recordExpand(g *gocui.Gui, v *gocui.View) error {
	rows := app.state.recordDisplayRows
	if app.state.recordCursor >= len(rows) {
		return nil
	}
	row := rows[app.state.recordCursor]
	if row.groupName != "" && !app.state.recordGroupExpanded[row.groupName] {
		app.state.recordGroupExpanded[row.groupName] = true
		app.buildRecordDisplayRows()
		app.renderRecordsView(g)
		app.autoPreviewRecord(g)
	}
	return nil
}

// recordCollapse collapses the current group. When the cursor is on a child
// record it collapses the parent group and moves the cursor to its header.
func (app *Gui) recordCollapse(g *gocui.Gui, v *gocui.View) error {
	rows := app.state.recordDisplayRows
	if app.state.recordCursor >= len(rows) {
		return nil
	}
	row := rows[app.state.recordCursor]

	if row.grouped {
		for i := app.state.recordCursor - 1; i >= 0; i-- {
			if rows[i].groupName != "" {
				app.state.recordCursor = i
				row = rows[i]
				break
			}
		}
	}
	if row.groupName != "" && app.state.recordGroupExpanded[row.groupName] {
		app.state.recordGroupExpanded[row.groupName] = false
		app.buildRecordDisplayRows()
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
	return app.focusTo(g, viewPreview)
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
	if app.state.filterQuery != "" {
		app.state.filterQuery = ""
		app.state.recordCursor = 0
		app.applyNameFilter()
		app.renderRecordsView(g)
		return nil
	}
	rows := app.state.recordDisplayRows
	if app.state.recordCursor < len(rows) {
		row := rows[app.state.recordCursor]
		if row.grouped || (row.groupName != "" && app.state.recordGroupExpanded[row.groupName]) {
			return app.recordCollapse(g, v)
		}
	}
	if len(app.state.clipboard) > 0 {
		return app.clipboardClear(g, v)
	}
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

	if r.Status != dirclient.StatusLocal {
		return app.recordDeleteSync(g)
	}

	name := r.Name
	if name == "" {
		name = r.CID
	}
	version := ""
	if r.Version != "" {
		version = " " + r.Version
	}

	body := fmt.Sprintf("Delete %s%s?", name, version)

	cid := r.CID
	app.openConfirmPopup(g, "Delete record", body, func() {
		go app.deleteRecord(cid)
	})
	return nil
}

// recordDeleteSync handles deletion when the cursor is on a non-local record.
// It shows a confirmation to cancel the entire sync operation.
func (app *Gui) recordDeleteSync(g *gocui.Gui) error {
	syncing, reconciling := app.syncCounts()
	total := syncing + reconciling
	body := fmt.Sprintf("Cancel sync? This will cancel all %d record(s) currently being synced.", total)

	app.openConfirmPopup(g, "Cancel sync", body, func() {
		go app.cancelSync()
	})
	return nil
}

func (app *Gui) cancelSync() {
	var cancelFn context.CancelFunc
	var syncID string
	var client *dirclient.Client

	app.g.Update(func(g *gocui.Gui) error {
		cancelFn = app.state.syncCancelFunc
		syncID = app.state.syncID
		client = app.state.client
		app.removeRecordsByStatus(dirclient.StatusSyncing)
		app.removeRecordsByStatus(dirclient.StatusReconciling)
		app.clearSyncState()
		app.applyFiltersSilent()
		app.renderStatus(g)
		return nil
	})

	if cancelFn != nil {
		cancelFn()
	}
	if syncID != "" && client != nil {
		_ = client.DeleteSync(context.Background(), syncID)
	}
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

// ── Clipboard (copy-paste between nodes) ─────────────────────────────────────

// clipboardToggle adds or removes the current record from the clipboard.
// The full RecordSummary is stored so paste has name/version/skills/etc.
func (app *Gui) clipboardToggle(g *gocui.Gui, v *gocui.View) error {
	r := app.cursorRecord()
	if r == nil || r.CID == "" {
		return nil
	}
	if app.state.client == nil {
		return nil
	}

	if app.state.clipboard == nil {
		app.state.clipboard = map[string]*dirclient.RecordSummary{}
	}

	cid := r.CID
	if _, ok := app.state.clipboard[cid]; ok {
		delete(app.state.clipboard, cid)
		if len(app.state.clipboard) == 0 {
			app.clearClipboard()
		}
	} else {
		snap := *r
		app.state.clipboard[cid] = &snap
		app.state.clipboardSource = app.state.serverAddr
		app.state.clipboardSourceURL = app.sourceURL()
	}
	app.renderRecordsView(g)
	app.renderStatus(g)
	return nil
}

// sourceURL constructs the URL of the currently connected directory server
// suitable for use as the remoteDirectoryUrl in CreateSync.
func (app *Gui) sourceURL() string {
	addr := app.state.serverAddr
	if addr == "" {
		return ""
	}
	if strings.Contains(addr, "://") {
		return addr
	}
	if app.state.authMode == authModeInsecure {
		return "http://" + addr
	}
	return "https://" + addr
}

// clipboardPaste syncs all clipboard records to the current node via CreateSync.
func (app *Gui) clipboardPaste(g *gocui.Gui, v *gocui.View) error {
	if len(app.state.clipboard) == 0 {
		return nil
	}
	if app.state.client == nil {
		return nil
	}
	if app.hasSyncingRecords() {
		return nil
	}

	if app.state.clipboardSource == app.state.serverAddr {
		app.openConfirmPopup(g, "Paste records",
			fmt.Sprintf("Cannot paste: source and target are the same server\n(%s)", app.state.serverAddr),
			nil,
		)
		return nil
	}

	count := len(app.state.clipboard)

	entries := make([]*dirclient.RecordSummary, 0, count)
	for _, r := range app.state.clipboard {
		entries = append(entries, r)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].Version < entries[j].Version
	})

	var body strings.Builder
	fmt.Fprintf(&body, "Sync %d record(s) from %s?\n\n", count, app.state.clipboardSource)
	for i, r := range entries {
		if i >= 5 {
			fmt.Fprintf(&body, "  … and %d more\n", count-5)
			break
		}
		label := r.Name
		if r.Version != "" {
			label += "@" + r.Version
		}
		fmt.Fprintf(&body, "  • %s\n", label)
	}
	app.openConfirmPopup(g, "Sync records", body.String(), func() {
		app.startSync(g)
	})
	return nil
}

// startSync injects clipboard records into fullCache with StatusSyncing,
// clears the clipboard, and kicks off the sync operation in the background.
func (app *Gui) startSync(g *gocui.Gui) {
	sourceURL := app.state.clipboardSourceURL

	existingCIDs := map[string]bool{}
	for _, r := range app.state.fullCache {
		existingCIDs[r.CID] = true
	}

	cids := make([]string, 0, len(app.state.clipboard))
	for cid, snap := range app.state.clipboard {
		cids = append(cids, cid)
		if existingCIDs[cid] {
			continue
		}
		r := *snap
		r.Status = dirclient.StatusSyncing
		app.state.fullCache = append(app.state.fullCache, &r)
	}

	app.state.syncCIDs = cids
	app.clearClipboard()

	ctx, cancel := context.WithCancel(context.Background())
	app.state.syncCancelFunc = cancel

	app.applyFiltersSilent()
	app.renderStatus(g)

	go app.runSync(ctx, sourceURL, cids)
}

// runSync calls CreateSync and polls until completion or failure.
func (app *Gui) runSync(ctx context.Context, sourceURL string, cids []string) {
	targetClient := app.state.client
	if targetClient == nil {
		app.syncFailed("Not connected to target server")
		return
	}

	syncID, err := targetClient.CreateSync(ctx, sourceURL, cids)
	if err != nil {
		app.syncFailed("Sync failed: " + err.Error())
		return
	}

	app.g.Update(func(g *gocui.Gui) error {
		app.state.syncID = syncID
		return nil
	})

	app.pollSync(ctx, targetClient, syncID)
}

// pollSync polls the sync operation until it completes or fails.
func (app *Gui) pollSync(ctx context.Context, client *dirclient.Client, syncID string) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}

		info, err := client.GetSyncInfo(ctx, syncID)
		if err != nil {
			app.syncFailed("Sync status check failed: " + err.Error())
			return
		}

		switch info.Status {
		case dirclient.SyncCompleted:
			app.g.Update(func(g *gocui.Gui) error {
				app.setRecordStatus(app.state.syncCIDs, dirclient.StatusReconciling, "")
				app.applyFiltersSilent()
				app.renderStatus(g)
				return nil
			})
			go app.pollReconcile(ctx)
			return
		case dirclient.SyncFailed:
			msg := fmt.Sprintf("Sync failed\n\n"+
				"  id:     %s\n"+
				"  source: %s\n"+
				"  target: %s\n"+
				"  last:   %s\n\n"+
				"Ensure you are authenticated (dirctl auth login).",
				syncID, info.RemoteDirectoryURL,
				app.state.serverAddr, info.LastUpdateTime)
			app.syncFailed(msg)
			return
		}
	}
}

// setRecordStatus updates the Status (and optionally StatusError) of all
// records in fullCache whose CID is in the given list.
func (app *Gui) setRecordStatus(cids []string, status dirclient.RecordStatus, errMsg string) {
	cidSet := map[string]bool{}
	for _, c := range cids {
		cidSet[c] = true
	}
	for _, r := range app.state.fullCache {
		if cidSet[r.CID] {
			r.Status = status
			r.StatusError = errMsg
		}
	}
}

// syncFailed transitions syncing records to failed state and shows an error popup.
func (app *Gui) syncFailed(msg string) {
	app.g.Update(func(g *gocui.Gui) error {
		app.setRecordStatus(app.state.syncCIDs, dirclient.StatusFailed, msg)
		app.state.recordInfoCID = ""
		app.state.recordInfoText = msg
		app.state.recordInfoError = true
		app.state.recordInfoLoading = false
		app.openInfoPopup(g, viewRecords)
		_, _ = g.SetCurrentView(viewInfoPopup)
		app.renderInfoPopup(g)
		app.applyFiltersSilent()
		app.renderStatus(g)
		return nil
	})
}

// pollReconcile periodically does a silent records refresh (preserving cursor)
// and promotes reconciling records to local once they appear in the stream.
func (app *Gui) pollReconcile(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	timeout := time.After(60 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			app.silentRefreshRecords(ctx)

			app.g.Update(func(g *gocui.Gui) error {
				app.promoteReconciledRecords()
				app.applyFiltersSilent()
				app.renderStatus(g)
				return nil
			})

			if !app.hasSyncingRecords() {
				app.g.Update(func(g *gocui.Gui) error {
					app.clearSyncState()
					return nil
				})
				return
			}
		case <-timeout:
			app.syncFailed("Reconciliation timed out — records were synced but the indexer has not picked them up yet. They may appear after a manual refresh (r).")
			return
		}
	}
}

// silentRefreshRecords does a blocking records fetch without resetting the
// cursor or the UI state. It merges fresh data into fullCache, replacing
// placeholder (syncing/reconciling) entries with real server data when
// their CID appears in the stream.
func (app *Gui) silentRefreshRecords(ctx context.Context) {
	client := app.state.client
	if client == nil {
		return
	}

	done := make(chan struct{})
	var allRecords []*dirclient.RecordSummary

	client.Stream(ctx, nil, dirclient.StreamCallbacks{
		OnFirstPage: func(summaries []*dirclient.RecordSummary) {
			allRecords = append(allRecords, summaries...)
		},
		OnBatch: func(batch []*dirclient.RecordSummary) {
			allRecords = append(allRecords, batch...)
		},
		OnDone: func(_ error) {
			close(done)
		},
	})

	select {
	case <-done:
	case <-ctx.Done():
		return
	}

	freshCIDs := make(map[string]bool, len(allRecords))
	for _, r := range allRecords {
		freshCIDs[r.CID] = true
	}

	app.g.Update(func(g *gocui.Gui) error {
		// Use fresh server records as the base; append any pending (non-local)
		// records that haven't appeared in the stream yet.
		result := make([]*dirclient.RecordSummary, 0, len(allRecords))
		result = append(result, allRecords...)
		for _, r := range app.state.fullCache {
			if r.Status != dirclient.StatusLocal && !freshCIDs[r.CID] {
				result = append(result, r)
			}
		}

		app.state.fullCache = result
		app.state.filterValues = newFilterValueAggregator()
		for _, r := range result {
			app.state.filterValues.add(r)
		}
		return nil
	})
}

// promoteReconciledRecords marks reconciling records as local if they now
// appear in fullCache with StatusLocal (i.e. from the fresh stream).
func (app *Gui) promoteReconciledRecords() {
	localCIDs := map[string]bool{}
	for _, r := range app.state.fullCache {
		if r.Status == dirclient.StatusLocal {
			localCIDs[r.CID] = true
		}
	}
	// Remove reconciling entries whose CID now has a local version
	remaining := app.state.fullCache[:0]
	for _, r := range app.state.fullCache {
		if r.Status == dirclient.StatusReconciling && localCIDs[r.CID] {
			continue
		}
		remaining = append(remaining, r)
	}
	app.state.fullCache = remaining
}

// removeRecordsByStatus removes all records with the given status from fullCache.
func (app *Gui) removeRecordsByStatus(status dirclient.RecordStatus) {
	remaining := app.state.fullCache[:0]
	for _, r := range app.state.fullCache {
		if r.Status != status {
			remaining = append(remaining, r)
		}
	}
	app.state.fullCache = remaining
}

// clipboardClear removes all records from the clipboard.
func (app *Gui) clipboardClear(g *gocui.Gui, v *gocui.View) error {
	if len(app.state.clipboard) == 0 {
		return nil
	}
	app.clearClipboard()
	app.renderRecordsView(g)
	app.renderStatus(g)
	return nil
}
