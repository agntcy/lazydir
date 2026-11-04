// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"github.com/agntcy/lazydir/internal/dirclient"
	"github.com/jesseduffield/gocui"
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
	selection map[string]dirclient.NetworkResult // CID → result, multi-select
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
	app.renderFiltersView(g)
	app.renderRecordsView(g)
	app.renderDirectory(g)
	app.renderStatus(g)
	return nil
}
