// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/agntcy/lazydir/internal/dirclient"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestToggleBrowseMode_KeepsLocalFilters(t *testing.T) {
	app := &Gui{}
	app.state.filters = newFilterState()
	app.state.filters.applied[filterSkills] = map[string]filterMode{"x": modeInclude}

	app.switchBrowseMode()
	if app.state.browse != browseNetwork {
		t.Fatalf("browse = %v, want browseNetwork", app.state.browse)
	}
	app.switchBrowseMode()

	if app.state.browse != browseLocal {
		t.Fatalf("browse = %v, want browseLocal", app.state.browse)
	}
	if got := app.state.filters.applied[filterSkills]["x"]; got != modeInclude {
		t.Errorf("local filter changed: %v", got)
	}
}

func TestToggleBrowseMode_KeepsNetworkFilters(t *testing.T) {
	app := &Gui{}
	app.state.net.filters = newNetFilterState()
	app.state.net.filters.applied[filterSkills] = map[string]bool{"x": true}
	app.state.net.filters.versions[ver070] = true

	app.switchBrowseMode()
	app.switchBrowseMode()

	if !app.state.net.filters.applied[filterSkills]["x"] || !app.state.net.filters.versions[ver070] {
		t.Errorf("network filters changed: applied %v, versions %v",
			app.state.net.filters.applied, app.state.net.filters.versions)
	}
}

func TestToggleBrowseMode_ClearsSelections(t *testing.T) {
	app := &Gui{}
	app.state.selection = map[string]*dirclient.RecordSummary{"local-1": {CID: "local-1"}}
	app.state.net.selection = map[string]dirclient.NetworkResult{"net-1": {}}

	app.switchBrowseMode()

	if len(app.state.selection) != 0 || len(app.state.net.selection) != 0 {
		t.Errorf("selections not cleared: %v %v", app.state.selection, app.state.net.selection)
	}
}

func TestModeLine(t *testing.T) {
	app := &Gui{theme: defaultTheme}
	tests := []struct {
		mode  browseMode
		color string
		name  string
	}{
		{browseLocal, defaultTheme.Color4, "node"},
		{browseNetwork, defaultTheme.Color1, "network"},
	}
	for _, tt := range tests {
		app.state.browse = tt.mode
		want := tt.color + "●" + defaultTheme.Reset + " Mode:      " + tt.name
		if got := app.modeLine(); got != want {
			t.Errorf("modeLine(%d) = %q, want %q", tt.mode, got, want)
		}
	}
}

func TestConnInfoText_ModeRow(t *testing.T) {
	app := &Gui{theme: defaultTheme}
	app.state.connCursor = connRowMode
	app.state.browse = browseNetwork
	app.state.oasfError = "oasf down"
	text, hasErr := app.connInfoText()
	if hasErr || !strings.Contains(text, "network") || strings.Contains(text, "oasf down") {
		t.Errorf("connInfoText() = %q, %v; want the mode without the OASF error", text, hasErr)
	}
}

func newNetSearchApp() *Gui {
	app := &Gui{}
	app.state.browse = browseNetwork
	app.state.net.filters = newNetFilterState()
	return app
}

func TestApplyNetworkResults_DropsStaleGen(t *testing.T) {
	app := newNetSearchApp()
	app.state.net.gen = 2
	old := []dirclient.NetworkResult{{CID: "keep"}}
	app.state.net.results = old

	app.applyNetworkResults(1, nil, []dirclient.NetworkResult{{CID: "stale"}}, nil, nil)

	if len(app.state.net.results) != 1 || app.state.net.results[0].CID != "keep" {
		t.Errorf("results = %v, want unchanged", app.state.net.results)
	}
}

func TestApplyNetworkResults_PermissionDenied(t *testing.T) {
	app := newNetSearchApp()
	app.state.net.gen = 1
	err := status.Error(codes.PermissionDenied, "nope")

	app.applyNetworkResults(1, nil, nil, nil, err)

	if got, want := app.state.net.notice, "Network search not allowed on this server"; got != want {
		t.Errorf("notice = %q, want %q", got, want)
	}
}

func TestApplyNetworkResults_Empty(t *testing.T) {
	app := newNetSearchApp()
	app.state.net.gen = 1
	qs := []dirclient.NetworkQuery{{Category: dirclient.FilterSkill, Value: "a"}}

	app.applyNetworkResults(1, qs, nil, nil, nil)

	if got, want := app.state.net.notice, "No peers announced matching records"; got != want {
		t.Errorf("notice = %q, want %q", got, want)
	}
}

func TestApplyNetworkResults_MarksLocal(t *testing.T) {
	app := newNetSearchApp()
	app.state.net.gen = 1
	res := []dirclient.NetworkResult{{CID: "c1"}, {CID: "c2"}}

	app.applyNetworkResults(1, nil, res, map[string]bool{"c1": true}, nil)

	if got := app.state.net.status["c1"].status; got != netSynced {
		t.Errorf("status[c1] = %v, want netSynced", got)
	}
	if _, ok := app.state.net.status["c2"]; ok {
		t.Errorf("status[c2] set, want absent")
	}
}

func TestApplyNetworkResults_KeepsExistingStatus(t *testing.T) {
	app := newNetSearchApp()
	app.state.net.gen = 1
	app.state.net.status = map[string]netSyncEntry{"c1": {status: netSyncing}}

	app.applyNetworkResults(1, nil, []dirclient.NetworkResult{{CID: "c1"}}, map[string]bool{"c1": true}, nil)

	if got := app.state.net.status["c1"].status; got != netSyncing {
		t.Errorf("status[c1] = %v, want netSyncing kept", got)
	}
}

func TestStartNetworkSearch_NoQueries(t *testing.T) {
	app := newNetSearchApp()
	app.state.net.results = []dirclient.NetworkResult{{CID: "c1"}}

	app.startNetworkSearch()

	if len(app.state.net.results) != 0 {
		t.Errorf("results = %v, want cleared", app.state.net.results)
	}
	if got, want := app.state.net.notice, "Pick a skill, domain or module to search the network"; got != want {
		t.Errorf("notice = %q, want %q", got, want)
	}
}

func TestNewNetFilterState_MinScoreDefault(t *testing.T) {
	if got := newNetFilterState().minScore; got != 1 {
		t.Errorf("minScore = %d, want 1", got)
	}
}

func TestNetworkRows_NoDuplicateKeptRows(t *testing.T) {
	q := dirclient.NetworkQuery{Category: dirclient.FilterSkill, Value: "a"}
	results := []dirclient.NetworkResult{{CID: "c1", Score: 1, Matched: []dirclient.NetworkQuery{q}}}
	kept := map[string]dirclient.NetworkResult{
		"c1": {CID: "c1", Matched: []dirclient.NetworkQuery{q}},
		"c2": {CID: "c2", Matched: []dirclient.NetworkQuery{q}},
	}

	rows := networkRows(results, kept, []dirclient.NetworkQuery{q}, 1)

	var cids []string
	for _, r := range rows {
		cids = append(cids, r.CID)
	}
	if strings.Join(cids, ",") != "c1,c2" {
		t.Errorf("rows = %v, want [c1 c2]", cids)
	}
}

func TestNetworkRows_KeptBelowMinScoreHidden(t *testing.T) {
	a := dirclient.NetworkQuery{Category: dirclient.FilterSkill, Value: "a"}
	b := dirclient.NetworkQuery{Category: dirclient.FilterDomain, Value: "b"}
	kept := map[string]dirclient.NetworkResult{
		"c1": {CID: "c1", Matched: []dirclient.NetworkQuery{a}},
		"c2": {CID: "c2", Matched: []dirclient.NetworkQuery{a, b}},
	}

	rows := networkRows(nil, kept, []dirclient.NetworkQuery{a, b}, 2)

	if len(rows) != 1 || rows[0].CID != "c2" {
		t.Errorf("rows = %v, want only c2", rows)
	}
}

func TestClampMinScore(t *testing.T) {
	for _, tc := range []struct {
		s    uint32
		n    int
		want uint32
	}{{0, 3, 1}, {5, 3, 3}, {2, 0, 1}, {2, 3, 2}} {
		if got := clampMinScore(tc.s, tc.n); got != tc.want {
			t.Errorf("clampMinScore(%d, %d) = %d, want %d", tc.s, tc.n, got, tc.want)
		}
	}
}

func TestNetworkTitle(t *testing.T) {
	app := newNetSearchApp()
	app.state.net.filters.applied[filterSkills] = map[string]bool{"a": true, "b": true}
	app.state.net.filters.applied[filterDomains] = map[string]bool{"c": true}
	app.state.net.filters.minScore = 2
	for i := range networkSearchLimit {
		app.state.net.results = append(app.state.net.results, dirclient.NetworkResult{CID: fmt.Sprintf("c%d", i)})
	}
	app.state.net.selection = map[string]dirclient.NetworkResult{"c1": {}}

	want := "[3] Records · network (100) min 2/3 [1 selected]  may be truncated"
	if got := app.networkTitle(); got != want {
		t.Errorf("title = %q, want %q", got, want)
	}
}

func TestNetworkRowLine_Markers(t *testing.T) {
	r := dirclient.NetworkResult{CID: "bafyabcdefghijklmnop", PeerID: "12D3KooWabcdefghXYZ1", DirAddr: "peer:8888", Score: 1}
	for _, tc := range []struct {
		name string
		r    dirclient.NetworkResult
		st   netSyncEntry
		want string
	}{
		{"synced", r, netSyncEntry{status: netSynced}, "✓ synced"},
		{"syncing", r, netSyncEntry{status: netSyncing}, "⟳ syncing"},
		{"failed", r, netSyncEntry{status: netFailed}, "✗ sync failed"},
		{"no address", dirclient.NetworkResult{CID: r.CID, PeerID: r.PeerID, Score: 1}, netSyncEntry{}, "no address"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			line := networkRowLine(tc.r, 2, tc.st, false, 80)
			if !strings.Contains(line, tc.want) {
				t.Errorf("line %q lacks %q", line, tc.want)
			}
			if !strings.Contains(line, "bafyabcd…mnop  peer 12D3KooW…XYZ1  1/2") {
				t.Errorf("line %q lacks short CID/peer/score", line)
			}
		})
	}
}

func TestWriteNetRecords_NoticeIsOnlyLine(t *testing.T) {
	app := newNetSearchApp()
	app.state.net.notice = "No peers announced matching records"
	var b bytes.Buffer

	if n := app.writeNetRecords(&b, 80); n != 0 {
		t.Errorf("rows = %d, want 0", n)
	}
	want := "  " + app.theme.Color10 + "No peers announced matching records" + app.theme.Reset + "\n"
	if got := b.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestAdjustMinScore_ClampsToQueries(t *testing.T) {
	app := newNetSearchApp()
	app.state.net.filters.applied[filterSkills] = map[string]bool{"a": true, "b": true}

	app.adjustMinScore(1)
	app.adjustMinScore(1)
	if got := app.state.net.filters.minScore; got != 2 {
		t.Errorf("after ++: minScore = %d, want 2", got)
	}
	app.adjustMinScore(-1)
	app.adjustMinScore(-1)
	if got := app.state.net.filters.minScore; got != 1 {
		t.Errorf("after --: minScore = %d, want 1", got)
	}
}

// Fixtures for the network preview tests.
const (
	testCID           = "baf-cid"
	testPreviewBefore = "before"
)

func TestNetworkInfoText(t *testing.T) {
	r := dirclient.NetworkResult{
		CID:     testCID,
		PeerID:  "12D3-peer",
		DirAddr: "h:8888",
		Matched: []dirclient.NetworkQuery{{Category: dirclient.FilterSkill, Value: "a/b"}},
	}
	got := networkInfoText(r, Theme{}, netStateFetching)
	for _, want := range []string{"Address  h:8888", "  skill  a/b", "Fetching from peer…"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}

	r.DirAddr = ""
	if got := networkInfoText(r, Theme{}, netStateNoAddress); strings.Contains(got, "Address") {
		t.Errorf("empty DirAddr still has an Address line:\n%s", got)
	}
}

func TestNetworkPreview_DropsStaleGen(t *testing.T) {
	app := newNetSearchApp()
	app.state.net.previewGen = 2
	app.state.previewContent = testPreviewBefore
	r := dirclient.NetworkResult{CID: testCID, DirAddr: "h:8888"}

	if app.applyPeerPreview(1, r, `{"a":1}`, nil) {
		t.Error("stale gen rendered")
	}
	if app.state.previewContent != testPreviewBefore {
		t.Errorf("previewContent = %q, want unchanged", app.state.previewContent)
	}

	if !app.applyPeerPreview(2, r, "", fmt.Errorf("boom")) {
		t.Fatal("current gen not rendered")
	}
	if !strings.Contains(app.state.previewContent, noticePeerUnreachable) {
		t.Errorf("previewContent = %q, want unreachable state", app.state.previewContent)
	}
}

func TestNetworkPreview_DropsAfterLeavingNetworkMode(t *testing.T) {
	app := newNetSearchApp()
	app.state.net.previewGen = 1
	app.state.previewContent = "this-node preview"
	app.state.browse = browseLocal

	if app.applyPeerPreview(1, dirclient.NetworkResult{CID: "c"}, `{}`, nil) {
		t.Error("rendered over the This-node preview")
	}
	if app.state.previewContent != "this-node preview" {
		t.Errorf("previewContent = %q, want unchanged", app.state.previewContent)
	}
}

func TestAutoPreviewRecord_NetworkIgnoresLocalRows(t *testing.T) {
	app := newNetSearchApp()
	app.state.recordDisplayRows = []recordDisplayRow{{record: &dirclient.RecordSummary{CID: "local"}}}
	app.state.net.results = []dirclient.NetworkResult{{CID: "net", PeerID: "p"}}
	app.state.net.filters.applied[filterSkills] = map[string]bool{"a/b": true}

	app.autoPreviewRecord(nil)

	if strings.Contains(app.state.previewContent, "local") || !strings.Contains(app.state.previewContent, noticeNoDirAddr) {
		t.Errorf("previewContent = %q, want the network row's no-address state", app.state.previewContent)
	}
}

func TestNetworkPreview_SyncedDropsStale(t *testing.T) {
	app := newNetSearchApp()
	app.state.net.previewGen = 2
	app.state.previewContent = testPreviewBefore
	r := dirclient.NetworkResult{CID: testCID}

	if app.applyNodePreview(1, r, `{"a":1}`, nil) {
		t.Error("stale gen rendered")
	}
	app.state.browse = browseLocal
	if app.applyNodePreview(2, r, `{"a":1}`, nil) {
		t.Error("rendered after leaving network mode")
	}
	if app.state.previewContent != testPreviewBefore {
		t.Errorf("previewContent = %q, want unchanged", app.state.previewContent)
	}

	app.state.browse = browseNetwork
	if !app.applyNodePreview(2, r, `{"a":1}`, nil) {
		t.Fatal("current gen not rendered")
	}
	if app.state.previewSubtitle != testCID || app.state.previewTree == nil {
		t.Errorf("subtitle %q, tree %v; want node JSON", app.state.previewSubtitle, app.state.previewTree)
	}
}

func planSyncRows() ([]dirclient.NetworkResult, map[string]netSyncEntry) {
	rows := []dirclient.NetworkResult{
		{CID: "c1", DirAddr: "p1"},
		{CID: "c2", DirAddr: "p2"},
		{CID: "c3", DirAddr: "p1"},
		{CID: "c4"},
		{CID: "c5", DirAddr: "p1"},
	}
	return rows, map[string]netSyncEntry{"c5": {status: netSynced}}
}

func TestPlanNetworkSync(t *testing.T) {
	rows, status := planSyncRows()
	batches, skipped, noAddr := planNetworkSync(rows, status, authModeInsecure)
	want := []peerBatch{
		{addr: "p1", url: "http://p1", cids: []string{"c1", "c3"}},
		{addr: "p2", url: "http://p2", cids: []string{"c2"}},
	}
	if fmt.Sprint(batches) != fmt.Sprint(want) {
		t.Errorf("batches = %v, want %v", batches, want)
	}
	if skipped != 1 || noAddr != 1 {
		t.Errorf("skipped %d noAddr %d, want 1 1", skipped, noAddr)
	}

	for _, mode := range []string{"none", ""} {
		if b, _, _ := planNetworkSync(rows, status, mode); b[0].url != "http://p1" {
			t.Errorf("auth %q: url = %q, want http", mode, b[0].url)
		}
	}
	if b, _, _ := planNetworkSync(rows, status, "oidc"); b[0].url != "https://p1" || b[1].url != "https://p2" {
		t.Errorf("oidc urls = %q %q, want https", b[0].url, b[1].url)
	}
}

func TestPlanNetworkSync_SkipsSyncingKeepsFailed(t *testing.T) {
	rows := []dirclient.NetworkResult{{CID: "a", DirAddr: "p"}, {CID: "b", DirAddr: "p"}}
	status := map[string]netSyncEntry{"a": {status: netSyncing}, "b": {status: netFailed}}
	batches, skipped, _ := planNetworkSync(rows, status, authModeInsecure)
	if skipped != 1 || len(batches) != 1 || fmt.Sprint(batches[0].cids) != "[b]" {
		t.Errorf("batches %v skipped %d; want [b] retried, a skipped", batches, skipped)
	}
}

func TestNetworkSyncConfirmBody(t *testing.T) {
	rows, status := planSyncRows()
	batches, skipped, noAddr := planNetworkSync(rows, status, authModeInsecure)
	got := networkSyncConfirmBody(batches, skipped, noAddr)
	want := "Sync 3 record(s) into this node?\n\n" +
		"  p1: 2\n" +
		"  p2: 1\n\n" +
		"Skipped: 1 (already synced or syncing)\n" +
		"No address: 1"
	if got != want {
		t.Errorf("body =\n%q\nwant\n%q", got, want)
	}
	if got := networkSyncConfirmBody(batches, 0, 0); got != "Sync 3 record(s) into this node?\n\n  p1: 2\n  p2: 1" {
		t.Errorf("body without skips = %q", got)
	}
}

func TestResetNetworkForServerSwitch(t *testing.T) {
	app := newNetSearchApp()
	ns := &app.state.net
	searchCancelled, syncCancelled := false, false
	ns.cancel = func() { searchCancelled = true }
	ns.syncCancel = func() { syncCancelled = true }
	peer, err := dirclient.ConnectPeer(dirclient.Config{AuthMode: authModeInsecure}, "127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	r := dirclient.NetworkResult{CID: "c1", DirAddr: "p1"}
	ns.peers = map[string]*dirclient.PeerClient{"127.0.0.1:1": peer}
	ns.results = []dirclient.NetworkResult{r}
	ns.selection = map[string]dirclient.NetworkResult{"c1": r}
	ns.status = map[string]netSyncEntry{"c1": {status: netSyncing}}
	ns.kept = map[string]dirclient.NetworkResult{"c1": r}
	ns.notice = noticeNoPeers
	ns.gen, ns.previewGen = 3, 5

	app.resetNetworkForServerSwitch()

	if !searchCancelled || !syncCancelled {
		t.Errorf("search cancelled %v, syncs cancelled %v; want both", searchCancelled, syncCancelled)
	}
	if ns.cancel != nil || ns.syncCancel != nil {
		t.Error("cancel funcs kept")
	}
	if len(ns.peers)+len(ns.results)+len(ns.selection)+len(ns.status)+len(ns.kept) != 0 || ns.notice != "" {
		t.Errorf("state kept: peers %v results %v selection %v status %v kept %v notice %q",
			ns.peers, ns.results, ns.selection, ns.status, ns.kept, ns.notice)
	}
	if ns.gen != 4 || ns.previewGen != 6 {
		t.Errorf("gen %d previewGen %d, want 4 6", ns.gen, ns.previewGen)
	}
}

func TestApplyNetworkSync(t *testing.T) {
	app := newNetSearchApp()
	ns := &app.state.net
	r1 := dirclient.NetworkResult{CID: "c1", DirAddr: "p1"}
	r2 := dirclient.NetworkResult{CID: "c2", DirAddr: "p2"}
	ns.status = map[string]netSyncEntry{"c1": {status: netSyncing}, "c2": {status: netSyncing}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app.applyNetworkSync(ctx, []dirclient.NetworkResult{r1}, "")
	if ns.status["c1"].status != netSynced || ns.kept["c1"].CID != "c1" {
		t.Errorf("completed: status %v kept %v", ns.status["c1"], ns.kept)
	}

	app.applyNetworkSync(ctx, []dirclient.NetworkResult{r2}, "peer p2, sync s1: boom")
	if e := ns.status["c2"]; e.status != netFailed || e.err != "peer p2, sync s1: boom" {
		t.Errorf("failed: %+v", e)
	}
	if _, ok := ns.kept["c2"]; ok {
		t.Error("failed row kept")
	}

	ns.status["c2"] = netSyncEntry{status: netSyncing}
	cancel()
	app.applyNetworkSync(ctx, []dirclient.NetworkResult{r2}, "")
	if ns.status["c2"].status != netSyncing {
		t.Errorf("cancelled sync landed: %v", ns.status["c2"])
	}
}

func TestNetSyncErr(t *testing.T) {
	if got := netSyncErr("p1", "s1", "boom"); got != "peer p1, sync s1: boom" {
		t.Errorf("got %q", got)
	}
	if got := netSyncErr("p1", "", "boom"); got != "peer p1: boom" {
		t.Errorf("without id got %q", got)
	}
}

func TestSwitchBrowseMode_LeavingNetworkCancelsSearch(t *testing.T) {
	app := newNetSearchApp()
	ns := &app.state.net
	searchCancelled, syncCancelled := false, false
	ns.cancel = func() { searchCancelled = true }
	ns.syncCancel = func() { syncCancelled = true }
	ns.loading = true
	ns.gen = 2

	app.switchBrowseMode()

	if !searchCancelled || ns.cancel != nil || ns.loading {
		t.Errorf("search not cancelled: called %v cancel %v loading %v", searchCancelled, ns.cancel != nil, ns.loading)
	}
	if ns.gen != 3 {
		t.Errorf("gen = %d, want 3", ns.gen)
	}
	if syncCancelled || ns.syncCancel == nil {
		t.Error("leaving network mode cancelled syncs")
	}
}

func TestApplyNetworkResults_PrunesSelection(t *testing.T) {
	app := newNetSearchApp()
	ns := &app.state.net
	const kept = "here"
	ns.selection = map[string]dirclient.NetworkResult{"gone": {CID: "gone"}, kept: {CID: kept}}

	app.applyNetworkResults(ns.gen, nil, []dirclient.NetworkResult{{CID: kept}}, nil, nil)

	if _, ok := ns.selection["gone"]; ok || len(ns.selection) != 1 {
		t.Errorf("selection = %v, want only kept", ns.selection)
	}
}

func TestNetSelection_ToggleAndSelectAll(t *testing.T) {
	app := newNetSearchApp()
	ns := &app.state.net
	ns.results = []dirclient.NetworkResult{{CID: "a"}, {CID: "b"}}
	ns.cursor = 1

	app.toggleNetSelected()
	if _, ok := ns.selection["b"]; !ok || len(ns.selection) != 1 {
		t.Fatalf("toggle: %v", ns.selection)
	}
	app.toggleNetSelected()
	if len(ns.selection) != 0 {
		t.Fatalf("untoggle: %v", ns.selection)
	}

	app.selectAllNetRows()
	if len(ns.selection) != 2 {
		t.Fatalf("select all: %v", ns.selection)
	}
	app.selectAllNetRows()
	if len(ns.selection) != 0 {
		t.Fatalf("select all again should clear: %v", ns.selection)
	}
	if len(app.state.selection) != 0 {
		t.Error("network selection wrote the This-node selection")
	}
}

func TestNetSyncTargets_SelectionInRowOrder(t *testing.T) {
	app := newNetSearchApp()
	ns := &app.state.net
	ns.results = []dirclient.NetworkResult{{CID: "a"}, {CID: "b"}, {CID: "c"}}
	ns.cursor = 1
	if got := app.netSyncTargets(); len(got) != 1 || got[0].CID != "b" {
		t.Errorf("no selection: %v, want cursor row b", got)
	}
	ns.selection = map[string]dirclient.NetworkResult{"c": {CID: "c"}, "a": {CID: "a"}}
	if got := app.netSyncTargets(); len(got) != 2 || got[0].CID != "a" || got[1].CID != "c" {
		t.Errorf("selection: %v, want a c", got)
	}
}

func TestRecordsPanelTitle_ModeAware(t *testing.T) {
	app := newNetSearchApp()
	app.state.page.records = []*dirclient.RecordSummary{{CID: "local"}}
	app.state.net.results = []dirclient.NetworkResult{{CID: "a"}, {CID: "b"}}

	if got, want := app.recordsPanelTitle(), app.networkTitle(); got != want {
		t.Errorf("network title = %q, want %q", got, want)
	}
	app.state.browse = browseLocal
	if got, want := app.recordsPanelTitle(), app.recordsTitle(); got != want {
		t.Errorf("local title = %q, want %q", got, want)
	}
}

func TestApplyNetworkResults_PresenceUpgradesFailed(t *testing.T) {
	app := newNetSearchApp()
	app.state.net.gen = 1
	app.state.net.status = map[string]netSyncEntry{"c1": {status: netFailed, err: "sync failed"}}

	app.applyNetworkResults(1, nil, []dirclient.NetworkResult{{CID: "c1"}}, map[string]bool{"c1": true}, nil)

	if got := app.state.net.status["c1"]; got.status != netSynced || got.err != "" {
		t.Errorf("status[c1] = %+v, want netSynced without error", got)
	}
}

func TestClosePeers(t *testing.T) {
	app := newNetSearchApp()
	peer, err := dirclient.ConnectPeer(dirclient.Config{AuthMode: authModeInsecure}, "127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	app.state.net.peers = map[string]*dirclient.PeerClient{"127.0.0.1:1": peer}

	app.closePeers()
	app.closePeers()

	if len(app.state.net.peers) != 0 {
		t.Errorf("peers = %v, want none", app.state.net.peers)
	}
}

func TestApplyLocalPreview_DropsAfterModeSwitch(t *testing.T) {
	app := newNetSearchApp()
	app.state.previewContent = testPreviewBefore

	if app.applyLocalPreview(browseLocal, "rec", `{"a":1}`, nil) {
		t.Error("local pull rendered over network mode")
	}
	if app.state.previewContent != testPreviewBefore {
		t.Errorf("previewContent = %q, want unchanged", app.state.previewContent)
	}

	app.state.browse = browseLocal
	if !app.applyLocalPreview(browseLocal, "rec", `{"a":1}`, nil) {
		t.Fatal("current mode not rendered")
	}
	if app.state.previewSubtitle != "rec" || app.state.previewTree == nil {
		t.Errorf("subtitle %q, tree %v; want record JSON", app.state.previewSubtitle, app.state.previewTree)
	}
}

func TestPreviewForModeSwitch(t *testing.T) {
	app := newNetSearchApp()
	app.state.previewContent = testPreviewBefore
	app.state.previewSubtitle = "rec"
	app.state.net.previewGen = 1

	// No network row yet: the This-node preview is cleared and a peer fetch
	// in flight is superseded.
	app.previewForModeSwitch(nil)
	if app.state.previewContent != "" || app.state.previewSubtitle != "" || app.state.net.previewGen != 2 {
		t.Errorf("content %q subtitle %q previewGen %d; want cleared, 2",
			app.state.previewContent, app.state.previewSubtitle, app.state.net.previewGen)
	}

	// A network cursor row is previewed.
	app.state.net.filters.applied[filterSkills] = map[string]bool{"x/y": true}
	app.state.net.results = []dirclient.NetworkResult{{CID: "net", PeerID: "p"}}
	app.previewForModeSwitch(nil)
	if !strings.Contains(app.state.previewContent, noticeNoDirAddr) {
		t.Errorf("previewContent = %q, want the network row's state", app.state.previewContent)
	}

	// Back on This node without rows: the network preview is cleared.
	app.state.browse = browseLocal
	app.previewForModeSwitch(nil)
	if app.state.previewContent != "" {
		t.Errorf("previewContent = %q, want cleared", app.state.previewContent)
	}
}

func TestApplyNetworkSync_SyncedTransition(t *testing.T) {
	r := dirclient.NetworkResult{CID: "c1", DirAddr: "p1"}
	for _, tc := range []struct {
		name   string
		browse browseMode
	}{
		// In network mode: This node reloads on return.
		{"network mode", browseNetwork},
		// On This node without a client: no reload ran, so it stays stale.
		{"This node, not connected", browseLocal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newNetSearchApp()
			app.state.browse = tc.browse
			ns := &app.state.net
			ns.gen = 7
			ns.status = map[string]netSyncEntry{"c1": {status: netSyncing}}

			app.applyNetworkSync(context.Background(), []dirclient.NetworkResult{r}, "")

			if st := ns.status["c1"]; st.status != netSynced || st.gen != 7 || ns.kept["c1"].CID != "c1" {
				t.Errorf("status %+v kept %v, want synced at gen 7 and kept", st, ns.kept)
			}
			if !ns.localStale {
				t.Error("localStale cleared without a This-node reload")
			}
		})
	}
}

// CIDs of TestApplyLocalPresence.
const (
	cidHeld    = "held"
	cidGone    = "gone"
	cidLate    = "late"
	cidRunning = "running"
	cidFailed  = "failed"
)

func TestApplyLocalPresence(t *testing.T) {
	res := []dirclient.NetworkResult{{CID: cidHeld}, {CID: cidGone}, {CID: cidLate}, {CID: cidRunning}, {CID: cidFailed}}
	app := newNetSearchApp()
	ns := &app.state.net
	ns.status = map[string]netSyncEntry{
		cidGone:    {status: netSynced, gen: 2},                 // synced before search 5, deleted since
		cidLate:    {status: netSynced, gen: 5},                 // synced while search 5 ran
		cidRunning: {status: netSyncing},                        // sync still running
		cidFailed:  {status: netFailed, err: "earlier failure"}, // failed earlier, now held
	}
	ns.kept = map[string]dirclient.NetworkResult{cidGone: {CID: cidGone}, cidLate: {CID: cidLate}}

	app.applyLocalPresence(5, res, map[string]bool{cidHeld: true, cidFailed: true})

	want := map[string]netSyncStatus{cidHeld: netSynced, cidLate: netSynced, cidRunning: netSyncing, cidFailed: netSynced}
	for cid, st := range want {
		if ns.status[cid].status != st {
			t.Errorf("status[%s] = %v, want %v", cid, ns.status[cid].status, st)
		}
	}
	if _, ok := ns.status[cidGone]; ok {
		t.Error("deleted record kept its synced marker")
	}
	if _, ok := ns.kept[cidGone]; ok {
		t.Error("deleted record still kept")
	}
	if _, ok := ns.kept[cidLate]; !ok {
		t.Error("record synced during the search dropped")
	}

	// A failed presence check (nil) changes nothing.
	app.applyLocalPresence(6, res, nil)
	if ns.status[cidHeld].status != netSynced {
		t.Error("failed presence check cleared a marker")
	}
}

func TestStartNetworkSearch_ClearsPreview(t *testing.T) {
	app := newNetSearchApp()
	app.state.previewContent = "old record"
	gen := app.state.net.previewGen

	app.startNetworkSearch() // no filters: no search runs

	if app.state.previewContent != "" || app.state.net.previewGen == gen {
		t.Errorf("previewContent %q previewGen %d, want cleared and bumped", app.state.previewContent, app.state.net.previewGen)
	}
}

func TestForgetNetSynced(t *testing.T) {
	app := newNetSearchApp()
	ns := &app.state.net
	ns.status = map[string]netSyncEntry{"a": {status: netSynced}, "b": {status: netSyncing}}
	ns.kept = map[string]dirclient.NetworkResult{"a": {CID: "a"}}

	app.forgetNetSynced("a", "b")

	if _, ok := ns.status["a"]; ok || len(ns.kept) != 0 {
		t.Errorf("status %v kept %v, want a forgotten", ns.status, ns.kept)
	}
	if ns.status["b"].status != netSyncing {
		t.Error("running sync forgotten")
	}
}

func TestApplyNetworkSync_FailedKeepsLocalFresh(t *testing.T) {
	app := newNetSearchApp()
	app.state.net.status = map[string]netSyncEntry{"c1": {status: netSyncing}}

	app.applyNetworkSync(context.Background(), []dirclient.NetworkResult{{CID: "c1"}}, "peer p1: down")

	if app.state.net.localStale {
		t.Error("failed sync marked This node stale")
	}
}

func TestReloadLocalRecords_NotConnected(t *testing.T) {
	app := newNetSearchApp()
	if app.reloadLocalRecords(false) {
		t.Error("reload ran without a client")
	}
}

func TestContainsAll(t *testing.T) {
	have := map[string]bool{"a": true, "b": true, "c": true}
	if !containsAll(have, []string{"c", "a"}) || !containsAll(have, nil) {
		t.Error("subset not contained")
	}
	if containsAll(have, []string{"a", "d"}) {
		t.Error("missing CID contained")
	}
}

func TestFitNetworkRow(t *testing.T) {
	const cid, peer, score, marker = "baeareih…mscu", "12D3KooW…U4tv", "1/2", "✓ synced"
	for _, tc := range []struct {
		width int
		want  string
	}{
		{80, cid + "  peer " + peer + "  " + score + "  " + marker},
		{46, cid + "  " + peer + "  " + score + "  " + marker},
		{37, cid + "  " + score + "  " + marker},
	} {
		if got := fitNetworkRow(cid, peer, score, marker, tc.width); got != tc.want {
			t.Errorf("width %d: got %q, want %q", tc.width, got, tc.want)
		}
	}
	if got := fitNetworkRow(cid, peer, score, "", 80); got != cid+"  peer "+peer+"  "+score {
		t.Errorf("no marker: got %q", got)
	}
}

func TestNetworkSyncConfirmBody_CapsPeers(t *testing.T) {
	var batches []peerBatch
	for i := range batchConfirmListMax + 3 {
		batches = append(batches, peerBatch{addr: fmt.Sprintf("peer%d:8888", i), cids: []string{fmt.Sprintf("c%d", i)}})
	}
	body := networkSyncConfirmBody(batches, 0, 0)
	if strings.Contains(body, fmt.Sprintf("peer%d:", batchConfirmListMax)) || !strings.Contains(body, "… and 3 more peers") {
		t.Errorf("body = %q, want %d peers listed and the rest summarized", body, batchConfirmListMax)
	}
	if !strings.HasPrefix(body, fmt.Sprintf("Sync %d record(s)", batchConfirmListMax+3)) {
		t.Errorf("body = %q, want the full record count", body)
	}
}

func TestStartNetworkSearch_NoFilterPrunesSelection(t *testing.T) {
	app := newNetSearchApp()
	app.state.net.selection = map[string]dirclient.NetworkResult{"c1": {CID: "c1"}}

	app.startNetworkSearch() // no filters: no rows

	if len(app.state.net.selection) != 0 {
		t.Errorf("selection = %v, want pruned", app.state.net.selection)
	}
}

func TestWaitForIndexed(t *testing.T) {
	calls := 0
	present := func(context.Context) (map[string]bool, error) {
		calls++
		if calls < 3 {
			return map[string]bool{"a": true}, nil
		}
		return map[string]bool{"a": true, "b": true}, nil
	}
	if !waitForIndexed(context.Background(), present, []string{"a", "b"}, time.Millisecond, time.Second) {
		t.Error("indexed records not reported")
	}

	// A stalled check is interrupted by the timeout.
	stalled := func(ctx context.Context) (map[string]bool, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	start := time.Now()
	if waitForIndexed(context.Background(), stalled, []string{"a"}, time.Millisecond, 50*time.Millisecond) {
		t.Error("stalled check reported indexed")
	}
	if time.Since(start) > 2*time.Second {
		t.Error("timeout did not interrupt the stalled check")
	}
}
