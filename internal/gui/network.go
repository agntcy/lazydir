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
	// local is the set of result CIDs this node already holds.
	local map[string]bool
	// kept are synced rows that stay listed after they drop out of results.
	kept   map[string]dirclient.NetworkResult
	status map[string]netSyncEntry

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
}

// modeLine is the non-selectable third line of the Connections panel.
func modeLine(m browseMode) string {
	if m == browseNetwork {
		return "Mode: Network    (n: this node)"
	}
	return "Mode: This node  (n: network)"
}

// switchBrowseMode flips the browse mode and clears both multi-selects, since a
// selection made in one mode is meaningless in the other. State only; no
// rendering.
func (app *Gui) switchBrowseMode() {
	if app.state.browse == browseLocal {
		app.state.browse = browseNetwork
	} else {
		app.state.browse = browseLocal
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
	ns.local = nil
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
	ns.local = nil
	switch {
	case dirclient.IsPermissionDenied(err):
		ns.notice = noticeSearchNotAllowed
	case err != nil:
		if app.g != nil && app.state.browse == browseNetwork {
			app.showRecordsError(app.g, queryErrorText(err))
		}
	default:
		ns.results = res
		ns.local = local
		for cid := range local {
			if _, ok := ns.status[cid]; ok {
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
	app.renderNetRecords()
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
