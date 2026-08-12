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

// recordStatus returns the effective transient status of a record by CID from
// the sync-status overlay. Records fetched via Page always carry StatusLocal,
// so the overlay is the sole source of in-flight status; absent an entry the
// record is treated as local.
func (app *Gui) recordStatus(cid string) (dirclient.RecordStatus, string) {
	if e, ok := app.state.syncStatus[cid]; ok {
		return e.status, e.err
	}
	return dirclient.StatusLocal, ""
}

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
	// Prefetch the next page as the cursor nears the end of the loaded rows.
	const prefetchThreshold = 5
	if !app.state.page.exhausted && !app.state.page.loading &&
		app.state.recordCursor >= len(app.state.recordDisplayRows)-prefetchThreshold {
		app.loadNextPage()
	}
	return nil
}

func (app *Gui) recordSelect(g *gocui.Gui, v *gocui.View) error {
	rows := app.state.recordDisplayRows
	if app.state.recordCursor >= len(rows) {
		return nil
	}
	row := rows[app.state.recordCursor]
	rec := row.record
	if rec == nil || rec.CID == "" {
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
				app.startQuery(true)
				return nil
			})
		},
		func() {
			app.g.Update(func(g *gocui.Gui) error {
				app.state.filterQuery = prevQuery
				app.startQuery(true)
				return nil
			})
		},
		func(value string) {
			app.state.filterQuery = value
			app.startQuery(true)
		},
	)
	return nil
}

func (app *Gui) clearFilter(g *gocui.Gui, v *gocui.View) error {
	if app.state.filterQuery != "" {
		app.state.filterQuery = ""
		app.startQuery(true)
		return nil
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

	if status, _ := app.recordStatus(r.CID); status != dirclient.StatusLocal {
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
		app.renderRecordsView(g)
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

// setRecordPublished flips the Published flag of the current-page record with
// the given CID and records the change as an optimistic override so a concurrent
// enrichment with a stale snapshot cannot clobber it (see applyPublishOverrides).
// Must be called on the GUI goroutine.
func (app *Gui) setRecordPublished(cid string, published bool) {
	for _, r := range app.state.page.records {
		if r.CID == cid {
			r.Published = published
			break
		}
	}
	if app.state.publishOverrides == nil {
		app.state.publishOverrides = map[string]bool{}
	}
	app.state.publishOverrides[cid] = published
}

// applyPublishOverrides re-applies optimistic publish/unpublish results on top
// of a freshly enriched publish set, so a record the user just toggled is not
// clobbered by a ListPublished snapshot taken before the change reached the
// server's routing index. An override is dropped once the server set agrees
// with it, so enrichment becomes authoritative again (self-healing). Must be
// called on the GUI goroutine, right after markPublished.
func (app *Gui) applyPublishOverrides(set map[string]bool) {
	for cid, want := range app.state.publishOverrides {
		if set[cid] == want {
			delete(app.state.publishOverrides, cid)
			continue
		}
		for _, r := range app.state.page.records {
			if r.CID == cid {
				r.Published = want
				break
			}
		}
	}
}

// recordPublish opens a confirmation popup for publishing the selected record.
// It only acts on a local, not-yet-published record; on group headers and
// already-published records it is a no-op.
func (app *Gui) recordPublish(g *gocui.Gui, v *gocui.View) error {
	r := app.cursorRecord()
	if r == nil || r.CID == "" || app.state.client == nil {
		return nil
	}
	if status, _ := app.recordStatus(r.CID); status != dirclient.StatusLocal || r.Published {
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
	body := fmt.Sprintf("Publish %s%s?", name, version)

	cid := r.CID
	app.openConfirmPopup(g, "Publish record", body, func() {
		go app.publishRecord(cid)
	})
	return nil
}

// recordUnpublish opens a confirmation popup for unpublishing the selected
// record. It only acts on a published record; otherwise it is a no-op.
func (app *Gui) recordUnpublish(g *gocui.Gui, v *gocui.View) error {
	r := app.cursorRecord()
	if r == nil || r.CID == "" || app.state.client == nil {
		return nil
	}
	if !r.Published {
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
	body := fmt.Sprintf("Unpublish %s%s?", name, version)

	cid := r.CID
	app.openConfirmPopup(g, "Unpublish record", body, func() {
		go app.unpublishRecord(cid)
	})
	return nil
}

// publishRecord calls the Publish RPC in the background and flips the record's
// Published flag on success. On failure it shows the error in the info popup.
func (app *Gui) publishRecord(cid string) {
	client := app.state.client
	if client == nil {
		return
	}

	err := client.Publish(context.Background(), []string{cid})
	app.g.Update(func(g *gocui.Gui) error {
		if err != nil {
			app.state.recordInfoCID = cid
			app.state.recordInfoText = err.Error()
			app.state.recordInfoError = true
			app.state.recordInfoLoading = false
			app.openInfoPopup(g, viewRecords)
			return nil
		}
		app.setRecordPublished(cid, true)
		app.renderRecordsView(g)
		return nil
	})
}

// unpublishRecord calls the Unpublish RPC in the background and clears the
// record's Published flag on success. On failure it shows the error in the
// info popup.
func (app *Gui) unpublishRecord(cid string) {
	client := app.state.client
	if client == nil {
		return
	}

	err := client.Unpublish(context.Background(), []string{cid})
	app.g.Update(func(g *gocui.Gui) error {
		if err != nil {
			app.state.recordInfoCID = cid
			app.state.recordInfoText = err.Error()
			app.state.recordInfoError = true
			app.state.recordInfoLoading = false
			app.openInfoPopup(g, viewRecords)
			return nil
		}
		app.setRecordPublished(cid, false)
		app.renderRecordsView(g)
		return nil
	})
}

// removeRecordFromState purges a record by CID from the current page and any
// stale sync-status overlay entry, then rebuilds the flat display rows so the
// deleted record disappears immediately without a refetch.
func (app *Gui) removeRecordFromState(cid string) {
	app.state.page.records = removeRecordByCID(app.state.page.records, cid)
	delete(app.state.syncStatus, cid)
	app.rebuildRecordRows()
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

// startSync marks the clipboard CIDs as syncing in the transient overlay,
// clears the clipboard, and kicks off the sync operation in the background.
// Records being pasted from another server are not on this server yet, so they
// do not appear as rows until reconcile + refetch — the sync-progress popup
// provides feedback in the meantime.
func (app *Gui) startSync(g *gocui.Gui) {
	sourceURL := app.state.clipboardSourceURL

	cids := make([]string, 0, len(app.state.clipboard))
	for cid := range app.state.clipboard {
		cids = append(cids, cid)
	}

	app.setRecordStatus(cids, dirclient.StatusSyncing, "")
	app.state.syncCIDs = cids
	app.clearClipboard()

	ctx, cancel := context.WithCancel(context.Background())
	app.state.syncCancelFunc = cancel

	app.renderRecordsView(g)
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
				app.renderRecordsView(g)
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

// setRecordStatus sets the transient sync-status overlay entry for each of the
// given CIDs (overwriting any existing entry). Must be called on the GUI
// goroutine.
func (app *Gui) setRecordStatus(cids []string, status dirclient.RecordStatus, errMsg string) {
	if app.state.syncStatus == nil {
		app.state.syncStatus = map[string]syncStatusEntry{}
	}
	for _, c := range cids {
		app.state.syncStatus[c] = syncStatusEntry{status: status, err: errMsg}
	}
}

// markPublished sets Published on each record according to whether its CID
// appears in the set. Records absent from the set are marked false, so
// re-running enrichment reflects the latest server state.
func markPublished(records []*dirclient.RecordSummary, set map[string]bool) {
	for _, r := range records {
		r.Published = set[r.CID]
	}
}

// startPublishEnrichment resolves the published-CID set from the server in the
// background and stamps the matching current-page records. No-op if already
// enriched, already running, or no client. Failures (e.g. a server without
// routing support) leave publishEnriched false and show no error — publish
// coloring simply does not appear. Must be called on the GUI goroutine.
func (app *Gui) startPublishEnrichment() {
	if app.state.publishEnriched || app.state.publishEnriching || app.state.client == nil {
		return
	}
	app.state.publishEnriching = true
	client := app.state.client
	ctx, cancel := context.WithCancel(context.Background())
	app.state.publishCancel = cancel

	go func() {
		set, err := client.ListPublished(ctx)

		app.g.Update(func(g *gocui.Gui) error {
			if ctx.Err() != nil {
				return nil
			}
			app.state.publishEnriching = false
			if err != nil {
				// Leave publishEnriched false; older servers without routing
				// degrade silently.
				return nil
			}
			app.state.publishedCIDs = set
			markPublished(app.state.page.records, set)
			app.applyPublishOverrides(set)
			app.state.publishEnriched = true
			app.renderRecordsView(g)
			app.renderFiltersView(g)
			return nil
		})
	}()
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
		app.renderRecordsView(g)
		app.renderStatus(g)
		return nil
	})
}

// pollReconcile waits for the just-synced records to appear in the server's
// index, then clears their transient sync-status overlay and refetches the
// current page so they render as normal server rows. It runs on a background
// goroutine; every state read/write goes through app.g.Update to stay on the
// GUI goroutine. Cancellation (user cancel / server switch) arrives via ctx.
func (app *Gui) pollReconcile(ctx context.Context) {
	client := app.state.client
	if client == nil {
		return
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	timeout := time.After(60 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout:
			app.syncFailed("Reconciliation timed out — records were synced but the indexer has not picked them up yet. They may appear after a manual refresh (r).")
			return
		case <-ticker.C:
		}

		// A CID-only search is a cheap way to ask "has the indexer picked these
		// up yet?" without pulling full records.
		cids, err := client.MatchingCIDs(ctx, nil)
		if err != nil {
			continue // transient; retry on the next tick until the timeout fires
		}
		present := make(map[string]bool, len(cids))
		for _, c := range cids {
			present[c] = true
		}

		reconciled := make(chan bool, 1)
		app.g.Update(func(g *gocui.Gui) error {
			// Drop overlay entries whose CID now exists on the server.
			for cid, e := range app.state.syncStatus {
				switch e.status {
				case dirclient.StatusSyncing, dirclient.StatusReconciling:
					if present[cid] {
						delete(app.state.syncStatus, cid)
					}
				}
			}
			reconciled <- !app.hasSyncingRecords()
			return nil
		})

		if <-reconciled {
			app.g.Update(func(g *gocui.Gui) error {
				app.clearSyncState()
				// Refetch the current page so the synced records land as regular
				// server rows (with fresh publish/option data).
				app.startQuery(false)
				app.renderStatus(g)
				return nil
			})
			return
		}
	}
}

// removeRecordsByStatus deletes all sync-status overlay entries with the given
// status. Must be called on the GUI goroutine.
func (app *Gui) removeRecordsByStatus(status dirclient.RecordStatus) {
	for cid, e := range app.state.syncStatus {
		if e.status == status {
			delete(app.state.syncStatus, cid)
		}
	}
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
