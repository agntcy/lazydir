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

// ── Records multi-select and batch actions (issue #34) ───────────────────────
//
// The selection is separate from the clipboard: clipboard marks deliberately
// survive a server switch (they are pasted onto another node), while a
// selection only ever targets records of the query it was made under.

// selectedMark prefixes a selected row in the Records panel.
const selectedMark = "●"

// batchConfirmListMax caps how many records a batch confirmation lists by name.
const batchConfirmListMax = 5

// batchKind is the action a batch applies to the selected records.
type batchKind int

const (
	batchDelete batchKind = iota
	batchPublish
	batchUnpublish
)

// verb returns the capitalised action name used in popups.
func (k batchKind) verb() string {
	switch k {
	case batchPublish:
		return "Publish"
	case batchUnpublish:
		return "Unpublish"
	}
	return "Delete"
}

// progressing returns the lower-case action name shown while a batch runs.
func (k batchKind) progressing() string {
	switch k {
	case batchPublish:
		return "publishing"
	case batchUnpublish:
		return "unpublishing"
	}
	return "deleting"
}

// pastTense returns the action name used in the failure summary.
func (k batchKind) pastTense() string {
	switch k {
	case batchPublish:
		return "Published"
	case batchUnpublish:
		return "Unpublished"
	}
	return "Deleted"
}

// batchFailure is one record a batch could not act on.
type batchFailure struct {
	record *dirclient.RecordSummary
	err    error
}

// queryKey identifies the record set a selection was made from: the server
// plus the server-side queries, independent of their order.
func queryKey(serverAddr string, queries []dirclient.Query) string {
	parts := make([]string, 0, len(queries))
	for _, q := range queries {
		parts = append(parts, fmt.Sprintf("%d|%t|%s", q.Category, q.Negate, q.Value))
	}
	sort.Strings(parts)
	return serverAddr + "\n" + strings.Join(parts, "\n")
}

// syncSelectionKey clears the selection when the query changes (filters, name
// filter, or server), so a batch never acts on records the user can no longer
// see. A refresh of the same query keeps it. It reports whether a non-empty
// selection was cleared, so the caller can redraw the options bar. Must be
// called on the GUI goroutine.
func (app *Gui) syncSelectionKey(key string) bool {
	if key == app.state.selectionKey {
		return false
	}
	cleared := len(app.state.selection) > 0
	app.state.selection = nil
	app.state.selectionKey = key
	return cleared
}

// resetSelectionForServerSwitch drops the selection and releases a running
// batch when the user switches directory servers, whether or not the new
// connection succeeds. The old batch keeps running against its own client,
// but its late callbacks are ignored (see batchClientCurrent). Must be called
// on the GUI goroutine.
func (app *Gui) resetSelectionForServerSwitch() {
	app.state.selection = nil
	app.state.selectionKey = ""
	app.endBatch(app.state.batchRunning)
}

// batchClientCurrent reports whether a batch's results still belong on
// screen: a batch that finishes after a server switch must not touch the new
// server's rows, selection, or popups. Must be called on the GUI goroutine.
func (app *Gui) batchClientCurrent(client *dirclient.Client) bool {
	return client == app.state.client
}

func (app *Gui) isSelected(cid string) bool {
	_, ok := app.state.selection[cid]
	return ok
}

// toggleSelected adds or removes r from the selection. A snapshot is stored so
// the record still takes part in a batch after a refresh unloads it.
func (app *Gui) toggleSelected(r *dirclient.RecordSummary) {
	if app.isSelected(r.CID) {
		delete(app.state.selection, r.CID)
		return
	}
	if app.state.selection == nil {
		app.state.selection = map[string]*dirclient.RecordSummary{}
	}
	snap := *r
	app.state.selection[r.CID] = &snap
}

// selectAllLoaded selects every loaded record, or clears the selection when
// all of them are selected already. Records beyond the loaded pages are not
// included.
func (app *Gui) selectAllLoaded() {
	records := app.state.page.records
	all := len(records) > 0
	for _, r := range records {
		if !app.isSelected(r.CID) {
			all = false
			break
		}
	}
	if all {
		app.state.selection = nil
		return
	}
	for _, r := range records {
		if !app.isSelected(r.CID) {
			app.toggleSelected(r)
		}
	}
}

// selectedRecords returns the selected records: loaded ones first, in page
// order and as their live page record (current Published flag), then any
// selected records no longer loaded, ordered by CID.
func (app *Gui) selectedRecords() []*dirclient.RecordSummary {
	out := make([]*dirclient.RecordSummary, 0, len(app.state.selection))
	seen := make(map[string]bool, len(app.state.selection))
	for _, r := range app.state.page.records {
		if app.isSelected(r.CID) {
			out = append(out, r)
			seen[r.CID] = true
		}
	}
	var rest []string
	for cid := range app.state.selection {
		if !seen[cid] {
			rest = append(rest, cid)
		}
	}
	sort.Strings(rest)
	for _, cid := range rest {
		out = append(out, app.state.selection[cid])
	}
	return out
}

// planBatch splits records into the ones the action applies to and a count of
// skipped ones. Records still syncing are always skipped; publish needs an
// unpublished record, unpublish a published one.
func planBatch(kind batchKind, records []*dirclient.RecordSummary, status func(string) dirclient.RecordStatus) ([]*dirclient.RecordSummary, int) {
	var act []*dirclient.RecordSummary
	for _, r := range records {
		if status(r.CID) != dirclient.StatusLocal {
			continue
		}
		switch kind {
		case batchPublish:
			if r.Published {
				continue
			}
		case batchUnpublish:
			if !r.Published {
				continue
			}
		}
		act = append(act, r)
	}
	return act, len(records) - len(act)
}

// recordLabel is the "name version" text used to name a record in popups.
func recordLabel(r *dirclient.RecordSummary) string {
	name := r.Name
	if name == "" {
		name = r.CID
	}
	if r.Version != "" {
		return name + " " + r.Version
	}
	return name
}

// batchConfirmBody is the confirmation text for a batch: the count, the first
// few records by name, and how many selected records are skipped.
func batchConfirmBody(kind batchKind, records []*dirclient.RecordSummary, skipped int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %d records?\n", kind.verb(), len(records))
	for i, r := range records {
		if i == batchConfirmListMax {
			fmt.Fprintf(&sb, "\n  … and %d more", len(records)-batchConfirmListMax)
			break
		}
		fmt.Fprintf(&sb, "\n  %s", recordLabel(r))
	}
	if skipped > 0 {
		fmt.Fprintf(&sb, "\n\n%d skipped (does not apply or still syncing)", skipped)
	}
	return sb.String()
}

// batchFailureText summarises a batch that partly failed.
func batchFailureText(kind batchKind, total int, failures []batchFailure) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %d of %d records. %d failed:\n", kind.pastTense(), total-len(failures), total, len(failures))
	for _, f := range failures {
		fmt.Fprintf(&sb, "\n%s: %v", recordLabel(f.record), f.err)
	}
	return sb.String()
}

// deleteEach deletes records one by one, since the Delete RPC takes a single
// CID, and keeps going past failures. progress is called after each attempt.
func deleteEach(ctx context.Context, records []*dirclient.RecordSummary, del func(context.Context, string) error, progress func(done, total int)) ([]string, []batchFailure) {
	var deleted []string
	var failures []batchFailure
	for i, r := range records {
		if err := del(ctx, r.CID); err != nil {
			failures = append(failures, batchFailure{record: r, err: err})
		} else {
			deleted = append(deleted, r.CID)
		}
		progress(i+1, len(records))
	}
	return deleted, failures
}

// ── Key handlers ─────────────────────────────────────────────────────────────

// recordToggleSelect selects or unselects the highlighted record (space).
func (app *Gui) recordToggleSelect(g *gocui.Gui, _ *gocui.View) error {
	r := app.cursorRecord()
	if r == nil || r.CID == "" {
		return nil
	}
	app.toggleSelected(r)
	app.renderRecordsView(g)
	app.renderStatus(g)
	return nil
}

// recordSelectAll selects every loaded record, or clears the selection when
// all are selected (ctrl+a).
func (app *Gui) recordSelectAll(g *gocui.Gui, _ *gocui.View) error {
	app.selectAllLoaded()
	app.renderRecordsView(g)
	app.renderStatus(g)
	return nil
}

// clearSelection empties the selection and redraws.
func (app *Gui) clearSelection(g *gocui.Gui) {
	app.state.selection = nil
	app.renderRecordsView(g)
	app.renderStatus(g)
}

// startBatch confirms and runs kind on the selected records it applies to. It
// returns false when nothing is selected, so the caller falls back to acting
// on the highlighted record.
func (app *Gui) startBatch(g *gocui.Gui, kind batchKind) bool {
	if len(app.state.selection) == 0 {
		return false
	}
	if app.state.client == nil {
		return true
	}
	if app.state.batchRunning != 0 {
		app.openConfirmPopup(g, kind.verb()+" records", "A batch is still running ("+app.state.batchProgress+"). Try again when it finishes.", nil)
		return true
	}
	act, skipped := planBatch(kind, app.selectedRecords(), func(cid string) dirclient.RecordStatus {
		s, _ := app.recordStatus(cid)
		return s
	})
	title := kind.verb() + " records"
	if len(act) == 0 {
		app.openConfirmPopup(g, title, fmt.Sprintf("No selected record can be %s.", strings.ToLower(kind.pastTense())), nil)
		return true
	}
	app.openConfirmPopup(g, title, batchConfirmBody(kind, act, skipped), func() {
		app.runBatch(kind, act)
	})
	return true
}

// runBatch executes a confirmed batch in the background. Must be called on
// the GUI goroutine.
func (app *Gui) runBatch(kind batchKind, records []*dirclient.RecordSummary) {
	client := app.state.client
	if client == nil {
		return
	}
	id := app.beginBatch(fmt.Sprintf("%s 0/%d", kind.progressing(), len(records)))
	app.renderStatus(app.g)

	if kind == batchDelete {
		go app.runBatchDelete(client, id, records)
		return
	}
	go app.runBatchPublish(client, id, kind, records)
}

// beginBatch marks a new batch as running and returns its ID. Must be called
// on the GUI goroutine.
func (app *Gui) beginBatch(progress string) uint64 {
	app.state.batchSeq++
	app.state.batchRunning = app.state.batchSeq
	app.state.batchProgress = progress
	return app.state.batchSeq
}

// updateBatchProgress sets the progress text of batch id. gocui runs each
// Update callback on its own goroutine, so they can arrive out of order: a
// progress update arriving after its batch finished is dropped, or it would
// leave the progress text set and block the next batch. Must be called on the
// GUI goroutine.
func (app *Gui) updateBatchProgress(id uint64, progress string) {
	if id == app.state.batchRunning {
		app.state.batchProgress = progress
	}
}

// endBatch marks batch id as finished. Must be called on the GUI goroutine.
func (app *Gui) endBatch(id uint64) {
	if id == app.state.batchRunning {
		app.state.batchRunning = 0
		app.state.batchProgress = ""
	}
}

func (app *Gui) runBatchDelete(client *dirclient.Client, id uint64, records []*dirclient.RecordSummary) {
	deleted, failures := deleteEach(context.Background(), records, client.Delete, func(done, total int) {
		app.g.Update(func(g *gocui.Gui) error {
			app.updateBatchProgress(id, fmt.Sprintf("%s %d/%d", batchDelete.progressing(), done, total))
			app.renderStatus(g)
			return nil
		})
	})
	app.g.Update(func(g *gocui.Gui) error {
		app.endBatch(id)
		if !app.batchClientCurrent(client) {
			app.renderStatus(g)
			return nil
		}
		for _, cid := range deleted {
			app.removeRecordFromState(cid)
		}
		app.state.selection = nil
		if len(deleted) > 0 {
			// Reload the records: pages fetched while the deletes ran, and
			// deleted records that were selected but not loaded, leave the
			// paging offset and total out of step with the server. The
			// silent reload keeps the rows on screen until it lands.
			app.startQuery(false)
			// Deleted records may have carried the last instance of some
			// filter values; refetch so they drop out of the pick-lists.
			app.startFilterValuesFetch(true)
		}
		app.renderRecordsView(g)
		app.renderFiltersView(g)
		app.renderStatus(g)
		app.autoPreviewRecord(g)
		if len(failures) > 0 {
			app.showRecordsError(g, batchFailureText(batchDelete, len(records), failures))
		}
		return nil
	})
}

// runBatchPublish publishes or unpublishes records in one RPC. On failure it
// shows the error and keeps the selection so the batch can be retried.
func (app *Gui) runBatchPublish(client *dirclient.Client, id uint64, kind batchKind, records []*dirclient.RecordSummary) {
	cids := make([]string, 0, len(records))
	for _, r := range records {
		cids = append(cids, r.CID)
	}
	call := client.Publish
	if kind == batchUnpublish {
		call = client.Unpublish
	}
	err := call(context.Background(), cids)
	app.g.Update(func(g *gocui.Gui) error {
		app.endBatch(id)
		if !app.batchClientCurrent(client) {
			app.renderStatus(g)
			return nil
		}
		if err != nil {
			app.renderStatus(g)
			app.showRecordsError(g, err.Error())
			return nil
		}
		for _, cid := range cids {
			app.setRecordPublished(cid, kind == batchPublish)
		}
		app.state.selection = nil
		app.renderRecordsView(g)
		app.renderStatus(g)
		return nil
	})
}
