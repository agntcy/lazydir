// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/agntcy/lazydir/internal/dirclient"
	"github.com/jesseduffield/gocui"
)

// networkSearchLimit caps a routing search; a full page means the list may be
// truncated.
const networkSearchLimit = 100

// Network-mode empty-state notices.
const (
	noticePickFilter       = "Pick a skill, domain or module to search the network"
	noticeNoPeers          = "No peers announced matching records"
	noticeSearchNotAllowed = "Network search not allowed on this server"
	noticePeerUnreachable  = "Couldn't reach the peer"
	noticeNoDirAddr        = "Peer published no directory address; can't preview or sync"
)

// peerPreviewTimeout bounds fetching a network record from its peer.
const peerPreviewTimeout = 4 * time.Second

// States shown at the bottom of a network row's info preview.
const (
	netStateFetching    = "Fetching from peer…"
	netStateUnreachable = noticePeerUnreachable + "\nS sync it into this node"
	netStateNoAddress   = noticeNoDirAddr
)

// browseMode selects which records the Filters, Records and Preview panels
// show: this node's directory or the network via routing search.
type browseMode int

const (
	browseLocal browseMode = iota
	browseNetwork
)

// netState is the network-mode state, kept separate from the This-node state
// so neither mode's code path writes the other's fields.
type netState struct {
	filters   netFilterState
	selection map[string]dirclient.NetworkResult // CID → result, multi-select

	// results of the last routing search, sorted by score. gen identifies the
	// current search; cancel stops it when a newer one supersedes it.
	results []dirclient.NetworkResult
	gen     uint64
	loading bool
	cancel  context.CancelFunc
	// notice replaces the rows when there is nothing to list.
	notice string
	cursor int
	// kept are synced rows that stay listed after they drop out of results.
	kept   map[string]dirclient.NetworkResult
	status map[string]netSyncEntry
	// syncCancel stops every running batch sync on a server switch.
	// Independent of the search cancel.
	syncCancel context.CancelFunc
	// localStale is set when a sync lands, so This node reloads on return.
	localStale bool

	// previewGen identifies the current peer preview; peers caches peer
	// connections by directory address.
	previewGen uint64
	peers      map[string]*dirclient.PeerClient
}

// netSyncStatus is the sync state of a network row.
type netSyncStatus int

const (
	netSyncing netSyncStatus = iota + 1
	netSynced
	netFailed
)

type netSyncEntry struct {
	status netSyncStatus
	err    string // why a failed sync failed
}

// modeLine is the non-selectable third line of the Connections panel.
func modeLine(m browseMode) string {
	if m == browseNetwork {
		return "Mode: Network    (n: this node)"
	}
	return "Mode: This node  (n: network)"
}

// switchBrowseMode flips the browse mode and clears both multi-selects, since a
// selection made in one mode is meaningless in the other. Leaving network mode
// stops its search; running syncs carry on. State only; no rendering.
func (app *Gui) switchBrowseMode() {
	if app.state.browse == browseLocal {
		app.state.browse = browseNetwork
	} else {
		app.state.browse = browseLocal
		ns := &app.state.net
		ns.gen++
		if ns.cancel != nil {
			ns.cancel()
			ns.cancel = nil
		}
		ns.loading = false
	}
	app.state.selection = nil
	app.state.net.selection = nil
}

// toggleBrowseMode is the global `n` handler.
func (app *Gui) toggleBrowseMode(g *gocui.Gui, v *gocui.View) error {
	app.switchBrowseMode()
	if app.state.browse == browseNetwork {
		app.startNetworkTaxonomyFetch()
		app.startNetworkSearch()
	} else if app.state.net.localStale && app.state.client != nil {
		// Records synced from the network belong in this node's list now.
		app.state.net.localStale = false
		app.invalidatePublishEnrichment()
		app.startQuery(true)
		app.startFilterValuesFetch(true)
	}
	app.renderFiltersView(g)
	app.renderRecordsView(g)
	app.renderDirectory(g)
	app.renderStatus(g)
	return nil
}

// clampMinScore clamps a minimum match score to 1..max(n,1), n being the
// number of queries.
func clampMinScore(s uint32, n int) uint32 {
	hi := uint32(max(n, 1))
	return min(max(s, 1), hi)
}

// startNetworkSearch runs the routing search for the network filters,
// superseding any search in flight, then checks which results this node holds.
// With no filters it clears the results and shows a hint instead. Must be
// called on the GUI goroutine.
func (app *Gui) startNetworkSearch() {
	ns := &app.state.net
	ns.gen++
	if ns.cancel != nil {
		ns.cancel()
		ns.cancel = nil
	}
	ns.results = nil
	ns.loading = false
	ns.notice = ""
	qs := app.networkQueries()
	client := app.state.client
	if len(qs) == 0 || client == nil {
		if len(qs) == 0 {
			ns.cursor = 0
			ns.notice = noticePickFilter
		}
		app.renderNetRecords()
		return
	}
	gen := ns.gen
	minScore := clampMinScore(ns.filters.minScore, len(qs))
	ctx, cancel := context.WithCancel(context.Background())
	ns.cancel = cancel
	ns.loading = true
	app.renderNetRecords()

	go func() {
		res, err := client.SearchNetwork(ctx, qs, minScore, networkSearchLimit)
		var local map[string]bool
		if err == nil && len(res) > 0 {
			// Presence is best effort: without it rows just lack the marker.
			local, _ = client.LocalCIDs(ctx, qs)
		}
		app.g.Update(func(*gocui.Gui) error {
			app.applyNetworkResults(gen, qs, res, local, err)
			return nil
		})
	}()
}

// applyNetworkResults lands a routing search answering qs. Results of a
// superseded search are dropped. Must be called on the GUI goroutine.
func (app *Gui) applyNetworkResults(gen uint64, qs []dirclient.NetworkQuery, res []dirclient.NetworkResult, local map[string]bool, err error) {
	ns := &app.state.net
	if gen != ns.gen {
		return
	}
	ns.loading = false
	ns.cancel = nil
	ns.notice = ""
	ns.results = nil
	switch {
	case dirclient.IsPermissionDenied(err):
		ns.notice = noticeSearchNotAllowed
	case err != nil:
		if app.g != nil && app.state.browse == browseNetwork {
			app.showRecordsError(app.g, queryErrorText(err))
		}
	default:
		ns.results = res
		for cid := range local {
			// A row on this node is synced, even if an earlier sync of it
			// failed; only a running sync keeps its own state.
			switch ns.status[cid].status {
			case netSyncing, netSynced:
				continue
			}
			if ns.status == nil {
				ns.status = map[string]netSyncEntry{}
			}
			ns.status[cid] = netSyncEntry{status: netSynced}
		}
		minScore := clampMinScore(ns.filters.minScore, len(qs))
		if len(networkRows(res, ns.kept, qs, minScore)) == 0 {
			ns.notice = noticeNoPeers
		}
	}
	app.pruneNetSelection()
	app.renderNetRecords()
}

// pruneNetSelection drops selected rows that are no longer listed, so a sync
// never acts on rows the user can't see. Must be called on the GUI goroutine.
func (app *Gui) pruneNetSelection() {
	ns := &app.state.net
	if len(ns.selection) == 0 {
		return
	}
	listed := map[string]bool{}
	for _, r := range app.netRecordRows() {
		listed[r.CID] = true
	}
	for cid := range ns.selection {
		if !listed[cid] {
			delete(ns.selection, cid)
		}
	}
}

// networkRows lists results (already sorted by score), then the kept rows
// missing from them whose matched queries overlap qs at least minScore times,
// scored by that overlap. Rows are deduplicated by CID.
func networkRows(results []dirclient.NetworkResult, kept map[string]dirclient.NetworkResult, qs []dirclient.NetworkQuery, minScore uint32) []dirclient.NetworkResult {
	seen := map[string]bool{}
	var rows []dirclient.NetworkResult
	for _, r := range results {
		if seen[r.CID] {
			continue
		}
		seen[r.CID] = true
		rows = append(rows, r)
	}
	want := map[dirclient.NetworkQuery]bool{}
	for _, q := range qs {
		want[q] = true
	}
	cids := make([]string, 0, len(kept))
	for cid := range kept {
		cids = append(cids, cid)
	}
	sort.Strings(cids)
	for _, cid := range cids {
		if seen[cid] {
			continue
		}
		r := kept[cid]
		var score uint32
		for _, m := range r.Matched {
			if want[m] {
				score++
			}
		}
		if score >= minScore {
			seen[cid] = true
			r.Score = score
			rows = append(rows, r)
		}
	}
	return rows
}

// netRecordRows returns the rows of the network Records panel for the current
// filters.
func (app *Gui) netRecordRows() []dirclient.NetworkResult {
	ns := &app.state.net
	qs := app.networkQueries()
	return networkRows(ns.results, ns.kept, qs, clampMinScore(ns.filters.minScore, len(qs)))
}

// shortID abbreviates a CID or peer ID to its first 8 and last 4 characters.
func shortID(s string) string {
	if len(s) <= 13 {
		return s
	}
	return s[:8] + "…" + s[len(s)-4:]
}

// networkRowLine formats one network Records row: short CID, short peer ID,
// match score out of total queries, and a sync or address marker.
func networkRowLine(r dirclient.NetworkResult, total int, st netSyncEntry, selected bool, viewW int) string {
	marker, color := "", ""
	switch {
	case st.status == netSynced:
		marker = "✓ synced"
	case st.status == netSyncing:
		marker, color = "⟳ syncing", ansiYellow
	case st.status == netFailed:
		marker, color = "✗ sync failed", ansiRed
	case r.DirAddr == "":
		marker = "no address"
	}
	text := fmt.Sprintf("%s  peer %s  %d/%d", shortID(r.CID), shortID(r.PeerID), r.Score, total)
	if marker != "" {
		text += "  " + marker
	}
	return styledRow(color, selected, viewW, text)
}

// networkTitle builds the network-mode Records panel title.
func (app *Gui) networkTitle() string {
	ns := &app.state.net
	title := fmt.Sprintf("%s · network (%d)", titleRecords, len(app.netRecordRows()))
	if n := len(app.networkQueries()); n > 0 {
		title += fmt.Sprintf(" min %d/%d", clampMinScore(ns.filters.minScore, n), n)
	}
	if n := len(ns.selection); n > 0 {
		title += fmt.Sprintf(" [%d selected]", n)
	}
	if len(ns.results) == networkSearchLimit {
		title += "  may be truncated"
	}
	if ns.loading {
		title += "  searching…"
	}
	return title
}

// renderNetRecords redraws the Records panel when the GUI is running.
func (app *Gui) renderNetRecords() {
	if app.g == nil {
		return
	}
	app.renderRecordsView(app.g)
}

// writeNetRecords writes the network Records rows, or the notice dimmed as the
// only line when there are none, and returns the number of rows. The cursor is
// clamped to the rows.
func (app *Gui) writeNetRecords(w io.Writer, viewW int) int {
	ns := &app.state.net
	rows := app.netRecordRows()
	if len(rows) == 0 {
		ns.cursor = 0
		if ns.notice != "" {
			fmt.Fprintf(w, "%s%s%s%s\n", strings.Repeat(" ", rowGutterWidth), app.theme.Color10, ns.notice, app.theme.Reset)
		}
		return 0
	}
	ns.cursor = min(max(ns.cursor, 0), len(rows)-1)
	total := len(app.networkQueries())
	for _, r := range rows {
		_, selected := ns.selection[r.CID]
		fmt.Fprintln(w, networkRowLine(r, total, ns.status[r.CID], selected, viewW))
	}
	return len(rows)
}

// renderNetRecordsView draws the network-mode Records panel into v.
func (app *Gui) renderNetRecordsView(v *gocui.View) {
	v.Title = app.networkTitle()
	viewW, viewH := v.Size()
	app.writeNetRecords(v, viewW)
	cursor := app.state.net.cursor
	if cursor >= viewH {
		_ = v.SetOrigin(0, cursor-viewH+1)
		_ = v.SetCursor(0, viewH-1)
	} else {
		_ = v.SetOrigin(0, 0)
		_ = v.SetCursor(0, cursor)
	}
}

func (app *Gui) netRecordCursorUp(g *gocui.Gui) {
	if app.state.net.cursor > 0 {
		app.state.net.cursor--
		app.renderRecordsView(g)
		app.autoPreviewRecord(g)
	}
}

func (app *Gui) netRecordCursorDown(g *gocui.Gui) {
	if app.state.net.cursor < len(app.netRecordRows())-1 {
		app.state.net.cursor++
		app.renderRecordsView(g)
		app.autoPreviewRecord(g)
	}
}

// netCursorRow returns the network row under the cursor.
func (app *Gui) netCursorRow() (dirclient.NetworkResult, bool) {
	rows := app.netRecordRows()
	c := app.state.net.cursor
	if c < 0 || c >= len(rows) {
		return dirclient.NetworkResult{}, false
	}
	return rows[c], true
}

// networkInfoText describes a network row from its search result alone,
// ending with state: whether the record is being fetched, or why it can't be.
func networkInfoText(r dirclient.NetworkResult, t Theme, state string) string {
	var sb strings.Builder
	field := func(label, value string) {
		fmt.Fprintf(&sb, "%s%-7s%s  %s\n", t.Color10, label, t.Reset, value)
	}
	field("CID", r.CID)
	field("Peer", r.PeerID)
	if r.DirAddr != "" {
		field("Address", r.DirAddr)
	}
	sb.WriteString("\nMatched\n")
	for _, m := range r.Matched {
		label, color := "skill", t.Color1
		switch m.Category {
		case dirclient.FilterDomain:
			label, color = "domain", t.Color2
		case dirclient.FilterModule:
			label, color = "module", t.Color3
		}
		fmt.Fprintf(&sb, "  %s%s%s  %s\n", color, label, t.Reset, m.Value)
	}
	sb.WriteString("\n" + state)
	return sb.String()
}

// previewNetworkResult previews a network row. A row synced into this node is
// pulled from it like a local row; otherwise the search info shows at once and
// the full record is fetched from the announcing peer in the background.
func (app *Gui) previewNetworkResult(g *gocui.Gui, r dirclient.NetworkResult) {
	ns := &app.state.net
	// Bumped for synced rows too, so a peer fetch still in flight can't
	// overwrite them.
	ns.previewGen++
	gen := ns.previewGen
	if ns.status[r.CID].status == netSynced {
		client := app.state.client
		if client == nil {
			return
		}
		go func() {
			jsonStr, err := client.PullJSON(context.Background(), r.CID)
			app.g.Update(func(*gocui.Gui) error {
				app.applyNodePreview(gen, r, jsonStr, err)
				return nil
			})
		}()
		return
	}
	if r.DirAddr == "" {
		app.renderPreviewText(g, shortID(r.CID), networkInfoText(r, app.theme, netStateNoAddress))
		return
	}
	app.renderPreviewText(g, shortID(r.CID), networkInfoText(r, app.theme, netStateFetching))
	// grpc.NewClient doesn't dial, so resolving the peer here doesn't block.
	peer, err := app.peerFor(r.DirAddr)
	if err != nil {
		app.applyPeerPreview(gen, r, "", err)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), peerPreviewTimeout)
		defer cancel()
		jsonStr, err := peer.PullJSON(ctx, r.CID)
		app.g.Update(func(*gocui.Gui) error {
			app.applyPeerPreview(gen, r, jsonStr, err)
			return nil
		})
	}()
}

// applyPeerPreview lands a peer fetch for r, reporting whether it rendered.
// Results of a superseded preview, or landing after leaving network mode, are
// dropped. Must be called on the GUI goroutine.
func (app *Gui) applyPeerPreview(gen uint64, r dirclient.NetworkResult, jsonStr string, err error) bool {
	if gen != app.state.net.previewGen || app.state.browse != browseNetwork {
		return false
	}
	if err != nil {
		app.renderPreviewText(app.g, shortID(r.CID), networkInfoText(r, app.theme, netStateUnreachable))
		return true
	}
	app.renderPreviewJSON(app.g, shortID(r.CID)+" · from peer", jsonStr)
	return true
}

// applyNodePreview lands a pull of synced row r from this node, reporting
// whether it rendered. Results of a superseded preview, or landing after
// leaving network mode, are dropped. Must be called on the GUI goroutine.
func (app *Gui) applyNodePreview(gen uint64, r dirclient.NetworkResult, jsonStr string, err error) bool {
	if gen != app.state.net.previewGen || app.state.browse != browseNetwork {
		return false
	}
	if err != nil {
		app.renderPreviewError(app.g, err.Error())
		return true
	}
	app.renderPreviewJSON(app.g, shortID(r.CID), jsonStr)
	return true
}

// peerFor returns the cached connection to the peer at addr, connecting with
// the current connection's transport settings on first use. Must be called on
// the GUI goroutine.
func (app *Gui) peerFor(addr string) (*dirclient.PeerClient, error) {
	ns := &app.state.net
	if p, ok := ns.peers[addr]; ok {
		return p, nil
	}
	client := app.state.client
	if client == nil {
		return nil, fmt.Errorf("not connected")
	}
	p, err := dirclient.ConnectPeer(client.Config, addr)
	if err != nil {
		return nil, err
	}
	if ns.peers == nil {
		ns.peers = map[string]*dirclient.PeerClient{}
	}
	ns.peers[addr] = p
	return p, nil
}

// adjustMinScore moves the minimum match score by delta within 1..queries and
// reruns the search when it changed. Network mode only.
func (app *Gui) adjustMinScore(delta int) {
	if app.state.browse != browseNetwork {
		return
	}
	fs := &app.state.net.filters
	n := len(app.networkQueries())
	cur := clampMinScore(fs.minScore, n)
	next := cur
	switch {
	case delta > 0:
		next = clampMinScore(cur+1, n)
	case delta < 0 && cur > 1:
		next = cur - 1
	}
	if next == cur {
		return
	}
	fs.minScore = next
	app.startNetworkSearch()
}

func (app *Gui) minScoreUp(*gocui.Gui, *gocui.View) error {
	app.adjustMinScore(1)
	return nil
}

func (app *Gui) minScoreDown(*gocui.Gui, *gocui.View) error {
	app.adjustMinScore(-1)
	return nil
}

// netSyncPollInterval is how often a network batch sync's status is checked.
const netSyncPollInterval = time.Second

// peerBatch is the network rows synced from one peer in a single CreateSync.
type peerBatch struct {
	addr, url string
	cids      []string
}

// peerURL is the remote directory URL of a peer address, using the scheme of
// the current connection's auth mode.
func peerURL(addr, authMode string) string {
	if strings.Contains(addr, "://") {
		return addr
	}
	switch authMode {
	case authModeInsecure, "none", "":
		return "http://" + addr
	}
	return "https://" + addr
}

// planNetworkSync groups rows into one batch per peer address, sorted by
// address with CIDs in row order. Rows already syncing or synced are skipped;
// rows whose peer announced no address can't be synced.
func planNetworkSync(rows []dirclient.NetworkResult, status map[string]netSyncEntry, authMode string) (batches []peerBatch, skipped, noAddr int) {
	byAddr := map[string]*peerBatch{}
	for _, r := range rows {
		switch status[r.CID].status {
		case netSyncing, netSynced:
			skipped++
			continue
		}
		if r.DirAddr == "" {
			noAddr++
			continue
		}
		b, ok := byAddr[r.DirAddr]
		if !ok {
			b = &peerBatch{addr: r.DirAddr, url: peerURL(r.DirAddr, authMode)}
			byAddr[r.DirAddr] = b
		}
		b.cids = append(b.cids, r.CID)
	}
	for _, b := range byAddr {
		batches = append(batches, *b)
	}
	sort.Slice(batches, func(i, j int) bool { return batches[i].addr < batches[j].addr })
	return batches, skipped, noAddr
}

// networkSyncConfirmBody is the confirmation text for a network sync: the
// count, the records per peer, and the rows left out.
func networkSyncConfirmBody(batches []peerBatch, skipped, noAddr int) string {
	n := 0
	for _, b := range batches {
		n += len(b.cids)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Sync %d record(s) into this node?\n", n)
	for _, b := range batches {
		fmt.Fprintf(&sb, "\n  %s: %d", b.addr, len(b.cids))
	}
	if left := netSyncLeftOut(skipped, noAddr); left != "" {
		sb.WriteString("\n\n" + left)
	}
	return sb.String()
}

// netSyncLeftOut lists the rows a network sync leaves out, one line per
// reason, or "" when none are.
func netSyncLeftOut(skipped, noAddr int) string {
	var lines []string
	if skipped > 0 {
		lines = append(lines, fmt.Sprintf("Skipped: %d (already synced or syncing)", skipped))
	}
	if noAddr > 0 {
		lines = append(lines, fmt.Sprintf("No address: %d", noAddr))
	}
	return strings.Join(lines, "\n")
}

// netSyncErr describes a failed batch sync; id is empty when the sync was
// never created.
func netSyncErr(addr, id, reason string) string {
	if id == "" {
		return fmt.Sprintf("peer %s: %s", addr, reason)
	}
	return fmt.Sprintf("peer %s, sync %s: %s", addr, id, reason)
}

// netSyncTargets returns the selected rows in row order, or the cursor row
// when nothing is selected.
func (app *Gui) netSyncTargets() []dirclient.NetworkResult {
	ns := &app.state.net
	if len(ns.selection) == 0 {
		if r, ok := app.netCursorRow(); ok {
			return []dirclient.NetworkResult{r}
		}
		return nil
	}
	var out []dirclient.NetworkResult
	for _, r := range app.netRecordRows() {
		if _, ok := ns.selection[r.CID]; ok {
			out = append(out, r)
		}
	}
	return out
}

// networkSync is the `S` handler: it confirms and syncs the selected network
// rows, or the cursor row, into this node.
func (app *Gui) networkSync(g *gocui.Gui, _ *gocui.View) error {
	if app.state.browse != browseNetwork || app.state.client == nil {
		return nil
	}
	rows := app.netSyncTargets()
	if len(rows) == 0 {
		return nil
	}
	batches, skipped, noAddr := planNetworkSync(rows, app.state.net.status, app.state.authMode)
	if len(batches) == 0 {
		app.openConfirmPopup(g, "Sync records", "Nothing to sync.\n\n"+netSyncLeftOut(skipped, noAddr), nil)
		return nil
	}
	app.openConfirmPopup(g, "Sync records", networkSyncConfirmBody(batches, skipped, noAddr), func() {
		app.startNetworkSync(batches, rows)
	})
	return nil
}

// startNetworkSync marks the batches' rows syncing and runs one CreateSync
// per peer in the background. Must be called on the GUI goroutine.
func (app *Gui) startNetworkSync(batches []peerBatch, rows []dirclient.NetworkResult) {
	client := app.state.client
	if client == nil {
		return
	}
	ns := &app.state.net
	// Chained onto syncCancel so a server switch stops this sync along with
	// the earlier ones still running.
	ctx, cancel := context.WithCancel(context.Background())
	if prev := ns.syncCancel; prev != nil {
		ns.syncCancel = func() { prev(); cancel() }
	} else {
		ns.syncCancel = cancel
	}
	byCID := make(map[string]dirclient.NetworkResult, len(rows))
	for _, r := range rows {
		byCID[r.CID] = r
	}
	if ns.status == nil {
		ns.status = map[string]netSyncEntry{}
	}
	for _, b := range batches {
		batchRows := make([]dirclient.NetworkResult, 0, len(b.cids))
		for _, cid := range b.cids {
			ns.status[cid] = netSyncEntry{status: netSyncing}
			batchRows = append(batchRows, byCID[cid])
		}
		go app.runNetworkSync(ctx, client, b, batchRows)
	}
	ns.selection = nil
	app.renderNetRecords()
}

// runNetworkSync syncs one peer batch into this node through client, which
// the caller captured on the GUI goroutine, and lands the outcome.
func (app *Gui) runNetworkSync(ctx context.Context, client *dirclient.Client, b peerBatch, rows []dirclient.NetworkResult) {
	errMsg := ""
	syncID, err := client.CreateSync(ctx, b.url, b.cids)
	if err == nil {
		var info *dirclient.SyncInfo
		info, err = waitForSync(ctx, client, syncID, netSyncPollInterval)
		if err == nil && info.Status == dirclient.SyncFailed {
			errMsg = netSyncErr(b.addr, syncID, "sync failed, last update "+info.LastUpdateTime)
		}
	}
	if err != nil {
		errMsg = netSyncErr(b.addr, syncID, err.Error())
	}
	app.g.Update(func(*gocui.Gui) error {
		app.applyNetworkSync(ctx, rows, errMsg)
		return nil
	})
}

// applyNetworkSync lands a batch sync of rows: synced rows stay listed even
// when they drop out of the results; failed ones keep errMsg for `i`. Results
// of syncs cancelled by a server switch are dropped. Must be called on the GUI
// goroutine.
func (app *Gui) applyNetworkSync(ctx context.Context, rows []dirclient.NetworkResult, errMsg string) {
	if ctx.Err() != nil {
		return
	}
	ns := &app.state.net
	if ns.status == nil {
		ns.status = map[string]netSyncEntry{}
	}
	for _, r := range rows {
		if errMsg != "" {
			ns.status[r.CID] = netSyncEntry{status: netFailed, err: errMsg}
			continue
		}
		ns.status[r.CID] = netSyncEntry{status: netSynced}
		if ns.kept == nil {
			ns.kept = map[string]dirclient.NetworkResult{}
		}
		ns.kept[r.CID] = r
	}
	if errMsg == "" {
		ns.localStale = true
	}
	app.renderNetRecords()
}

// netRecordToggleInfo is `i` in network mode: on a failed row it toggles the
// Records info popup with the sync error; other rows have no info.
func (app *Gui) netRecordToggleInfo(g *gocui.Gui, v *gocui.View) error {
	r, ok := app.netCursorRow()
	if !ok {
		return nil
	}
	st := app.state.net.status[r.CID]
	if st.status != netFailed {
		return nil
	}
	if app.state.recordInfoCID == r.CID {
		_ = app.closeInfoPopup(g, v)
		return app.focusTo(g, viewRecords)
	}
	app.state.recordInfoCID = r.CID
	app.state.recordInfoText = st.err
	app.state.recordInfoError = true
	app.state.recordInfoLoading = false
	app.openInfoPopup(g, viewRecords)
	return nil
}

// toggleNetSelected selects or unselects the network row under the cursor.
func (app *Gui) toggleNetSelected() {
	r, ok := app.netCursorRow()
	if !ok {
		return
	}
	ns := &app.state.net
	if _, sel := ns.selection[r.CID]; sel {
		delete(ns.selection, r.CID)
		return
	}
	if ns.selection == nil {
		ns.selection = map[string]dirclient.NetworkResult{}
	}
	ns.selection[r.CID] = r
}

// selectAllNetRows selects every listed network row, or clears the selection
// when all of them are selected already.
func (app *Gui) selectAllNetRows() {
	ns := &app.state.net
	rows := app.netRecordRows()
	all := len(rows) > 0
	for _, r := range rows {
		if _, ok := ns.selection[r.CID]; !ok {
			all = false
			break
		}
	}
	if all {
		ns.selection = nil
		return
	}
	if ns.selection == nil {
		ns.selection = map[string]dirclient.NetworkResult{}
	}
	for _, r := range rows {
		ns.selection[r.CID] = r
	}
}

// startNetworkSearchIfActive reruns the network search when network mode is
// showing, e.g. once a (re)connection lands. Must be called on the GUI
// goroutine.
func (app *Gui) startNetworkSearchIfActive() {
	if app.state.browse == browseNetwork {
		app.startNetworkSearch()
	}
}

// resetNetworkForServerSwitch drops all network state tied to the old server:
// it stops the search and every batch sync, closes peer connections, and
// supersedes pending searches and previews. Must be called on the GUI
// goroutine.
func (app *Gui) resetNetworkForServerSwitch() {
	ns := &app.state.net
	if ns.cancel != nil {
		ns.cancel()
		ns.cancel = nil
	}
	if ns.syncCancel != nil {
		ns.syncCancel()
		ns.syncCancel = nil
	}
	for _, p := range ns.peers {
		p.Close()
	}
	ns.peers = nil
	ns.results = nil
	ns.selection = nil
	ns.status = nil
	ns.kept = nil
	ns.notice = ""
	ns.loading = false
	ns.cursor = 0
	ns.localStale = false
	ns.gen++
	ns.previewGen++
}
