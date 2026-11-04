// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

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
	if got, want := modeLine(browseLocal), "Mode: This node  (n: network)"; got != want {
		t.Errorf("local = %q, want %q", got, want)
	}
	if got, want := modeLine(browseNetwork), "Mode: Network    (n: this node)"; got != want {
		t.Errorf("network = %q, want %q", got, want)
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

func TestNetworkInfoText(t *testing.T) {
	r := dirclient.NetworkResult{
		CID:     "baf-cid",
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
	app.state.previewContent = "before"
	r := dirclient.NetworkResult{CID: "baf-cid", DirAddr: "h:8888"}

	if app.applyPeerPreview(1, r, `{"a":1}`, nil) {
		t.Error("stale gen rendered")
	}
	if app.state.previewContent != "before" {
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
