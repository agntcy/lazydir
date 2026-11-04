// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"testing"

	"github.com/agntcy/lazydir/internal/dirclient"
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
