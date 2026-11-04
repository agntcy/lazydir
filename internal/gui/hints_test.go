// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"strings"
	"testing"

	"github.com/agntcy/lazydir/internal/dirclient"
	"github.com/jesseduffield/gocui"
)

func hasKey(pb panelBindings, key string) bool {
	for _, b := range pb.bindings {
		if strings.Contains(b.key, key) {
			return true
		}
	}
	return false
}

func TestBindingsFor_Network(t *testing.T) {
	net, ok := bindingsFor(viewRecords, true)
	if !ok || !hasKey(net, "S") || hasKey(net, "d") || !hasKey(net, "+ / -") {
		t.Fatalf("network records bindings wrong: %+v", net)
	}
	local, ok := bindingsFor(viewRecords, false)
	if !ok || !hasKey(local, "d") || hasKey(local, "S") || hasKey(local, "+ / -") {
		t.Fatalf("local records bindings wrong: %+v", local)
	}
	nf, _ := bindingsFor(viewFilters, true)
	if !hasKey(nf, "+ / -") || hasKey(nf, "i") {
		t.Fatalf("network filters bindings wrong: %+v", nf)
	}
	lf, _ := bindingsFor(viewFilters, false)
	if hasKey(lf, "+ / -") || !hasKey(lf, "i") {
		t.Fatalf("local filters bindings wrong: %+v", lf)
	}
	// Panels without a network variant fall back to the shared table.
	if _, ok := bindingsFor(viewPreview, true); !ok {
		t.Fatal("preview bindings missing in network mode")
	}
}

func TestOptionsBarText_NetworkShowsSync(t *testing.T) {
	if got := optionsBarText(viewRecords, 0, true, barStatus{}); !strings.Contains(got, ": S") {
		t.Fatalf("expected sync hint in network mode, got %q", got)
	}
	if got := optionsBarText(viewRecords, 0, false, barStatus{}); strings.Contains(got, ": S") {
		t.Fatalf("unexpected sync hint in This-node mode, got %q", got)
	}
}

func TestHelpPopupLines_Network(t *testing.T) {
	net := strings.Join(helpPopupLines(viewRecords, true), "\n")
	if !strings.Contains(net, "sync failure info") {
		t.Fatalf("network help missing sync failure info:\n%s", net)
	}
	local := strings.Join(helpPopupLines(viewRecords, false), "\n")
	if strings.Contains(local, "sync failure info") {
		t.Fatalf("local help shows network binding:\n%s", local)
	}
}

func TestLocalOnly_NoopInNetworkMode(t *testing.T) {
	app := &Gui{}
	calls := 0
	h := app.localOnly(func(*gocui.Gui, *gocui.View) error { calls++; return nil })
	if err := h(nil, nil); err != nil || calls != 1 {
		t.Fatalf("local mode: err=%v calls=%d", err, calls)
	}
	app.state.browse = browseNetwork
	if err := h(nil, nil); err != nil || calls != 1 {
		t.Fatalf("network mode must not call handler: err=%v calls=%d", err, calls)
	}
}

func TestClearNetSelection(t *testing.T) {
	app := &Gui{}
	if app.clearNetSelection() {
		t.Fatal("nothing to clear should report false")
	}
	app.state.net.selection = map[string]dirclient.NetworkResult{"c1": {}}
	if !app.clearNetSelection() || len(app.state.net.selection) != 0 {
		t.Fatalf("net selection not cleared: %v", app.state.net.selection)
	}
}

func TestOptionsBarText_CancelSyncThisNodeOnly(t *testing.T) {
	st := barStatus{syncing: 1}
	if got := optionsBarText(viewRecords, 0, false, st); !strings.Contains(got, "cancel sync: X") {
		t.Errorf("This node: missing cancel sync hint in %q", got)
	}
	if got := optionsBarText(viewRecords, 0, true, st); strings.Contains(got, "cancel sync: X") {
		t.Errorf("network: unexpected cancel sync hint in %q", got)
	}
}
