// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/agntcy/lazydir/internal/dirclient"
	"github.com/jesseduffield/gocui"
)

// ── Popup helpers (shared open/close logic) ──────────────────────────────────

// showPopup makes a popup view visible, brings it to the top, tracks the
// previous view for restoring focus later, and sets it as the current view.
func (app *Gui) showPopup(g *gocui.Gui, name string) {
	v, err := g.View(name)
	if err != nil {
		return
	}
	if cv := g.CurrentView(); cv != nil && cv.Name() != name {
		app.state.popupPrevView = cv.Name()
	}
	v.Visible = true
	_, _ = g.SetViewOnTop(name)
	_, _ = g.SetCurrentView(name)
}

// hidePopup hides a popup view and restores focus to the previously active view.
func (app *Gui) hidePopup(g *gocui.Gui, name string) {
	v, err := g.View(name)
	if err != nil {
		return
	}
	v.Visible = false
	g.SelFrameColor = app.theme.ActiveBorderColor
	g.SelFgColor = app.theme.ActiveBorderColor
	target := app.state.popupPrevView
	if target == "" {
		target = viewRecords
	}
	_, _ = g.SetCurrentView(target)
}

// ── Info popup ────────────────────────────────────────────────────────────────

func (app *Gui) infoPopupClose(g *gocui.Gui, v *gocui.View) error {
	target := app.state.popupPrevView
	if target == "" {
		target = viewRecords
	}
	_ = app.closeInfoPopup(g, v)
	return app.focusTo(g, target)
}

func (app *Gui) hideInfoPopupIfVisible(g *gocui.Gui) {
	ipv, err := g.View(viewInfoPopup)
	if err != nil || !ipv.Visible {
		return
	}
	ipv.Visible = false
	ipv.FrameColor = gocui.ColorDefault
	ipv.TitleColor = gocui.ColorDefault
	g.SelFrameColor = app.theme.ActiveBorderColor
	g.SelFgColor = app.theme.ActiveBorderColor
	app.clearRecordInlineDesc()
	app.clearInlineDesc()
	app.state.popupPrevView = ""
	app.state.infoPopupPanel = ""
}

func (app *Gui) openInfoPopup(g *gocui.Gui, sourcePanel string) {
	ipv, err := g.View(viewInfoPopup)
	if err != nil {
		return
	}
	if cv := g.CurrentView(); cv != nil && cv.Name() != viewInfoPopup {
		app.state.popupPrevView = cv.Name()
	}
	app.state.infoPopupPanel = sourcePanel
	ipv.Clear()
	_ = ipv.SetOrigin(0, 0)
	ipv.Visible = true
	app.renderInfoPopup(g)
	_, _ = g.SetViewOnTop(viewInfoPopup)
}

func (app *Gui) closeInfoPopup(g *gocui.Gui, v *gocui.View) error {
	ipv, err := g.View(viewInfoPopup)
	if err != nil {
		return nil
	}
	ipv.Visible = false
	ipv.FrameColor = gocui.ColorDefault
	ipv.TitleColor = gocui.ColorDefault
	g.SelFrameColor = app.theme.ActiveBorderColor
	g.SelFgColor = app.theme.ActiveBorderColor
	app.clearRecordInlineDesc()
	app.clearInlineDesc()
	app.state.popupPrevView = ""
	app.state.infoPopupPanel = ""
	app.clearFailedSyncRecords(g)
	return nil
}

// clearFailedSyncRecords removes records with StatusFailed from fullCache
// when the error popup is dismissed.
func (app *Gui) clearFailedSyncRecords(g *gocui.Gui) {
	hasFailed := false
	for _, r := range app.state.fullCache {
		if r.Status == dirclient.StatusFailed {
			hasFailed = true
			break
		}
	}
	if !hasFailed {
		return
	}
	app.removeRecordsByStatus(dirclient.StatusFailed)
	app.applyFiltersSilent()
	app.renderStatus(g)
}

func (app *Gui) renderInfoPopup(g *gocui.Gui) {
	ipv, err := g.View(viewInfoPopup)
	if err != nil || !ipv.Visible {
		return
	}
	ipv.Clear()
	_ = ipv.SetOrigin(0, 0)

	hasError := false

	switch app.state.infoPopupPanel {
	case viewDirectory:
		text, isErr := app.connInfoText()
		fmt.Fprint(ipv, text)
		hasError = isErr
	case viewFilters:
		if app.state.filters.inlineDescLoading {
			fmt.Fprintf(ipv, "%sloading…%s", app.theme.Color4, app.theme.Reset)
		} else if app.state.filters.inlineDescText != "" {
			fmt.Fprint(ipv, app.state.filters.inlineDescText)
			hasError = app.state.filters.inlineDescError
		}
	case viewRecords:
		if app.state.recordInfoLoading {
			fmt.Fprintf(ipv, "%sloading…%s", app.theme.Color4, app.theme.Reset)
		} else if app.state.recordInfoText != "" {
			fmt.Fprint(ipv, app.state.recordInfoText)
			hasError = app.state.recordInfoError
		}
	}

	if hasError {
		ipv.FrameColor = gocui.ColorRed
		ipv.TitleColor = gocui.ColorRed
	} else {
		ipv.FrameColor = gocui.ColorGreen
		ipv.TitleColor = gocui.ColorGreen
	}
	g.SelFrameColor = ipv.FrameColor
	g.SelFgColor = ipv.TitleColor
}

// ── Help popup ────────────────────────────────────────────────────────────────

func (app *Gui) openHelp(g *gocui.Gui, v *gocui.View) error {
	hv, err := g.View(viewHelp)
	if err != nil {
		return nil
	}
	if cv := g.CurrentView(); cv != nil && cv.Name() != viewHelp {
		app.state.popupPrevView = cv.Name()
	}
	hv.Clear()
	_ = hv.SetOrigin(0, 0)
	for _, line := range helpPopupLines(app.state.popupPrevView) {
		fmt.Fprintln(hv, line)
	}
	hv.Visible = true
	_, _ = g.SetCurrentView(viewHelp)
	_, _ = g.SetViewOnTop(viewHelp)
	return nil
}

func (app *Gui) closeHelp(g *gocui.Gui, v *gocui.View) error {
	app.hidePopup(g, viewHelp)
	return nil
}

// ── Copy menu popup ───────────────────────────────────────────────────────────

func (app *Gui) openCopyMenu(g *gocui.Gui, v *gocui.View) error {
	if app.cursorRecord() == nil {
		return nil
	}
	cv, err := g.View(viewCopyMenu)
	if err != nil {
		return nil
	}
	cv.Clear()
	fmt.Fprintf(cv, "  %sc%s  copy CID\n", app.theme.Color2, app.theme.Reset)
	fmt.Fprintf(cv, "  %sa%s  copy record JSON", app.theme.Color2, app.theme.Reset)
	app.showPopup(g, viewCopyMenu)
	return nil
}

func (app *Gui) closeCopyMenu(g *gocui.Gui, v *gocui.View) error {
	app.hidePopup(g, viewCopyMenu)
	return nil
}

func (app *Gui) copyCID(g *gocui.Gui, v *gocui.View) error {
	r := app.cursorRecord()
	if r == nil || r.CID == "" {
		return app.closeCopyMenu(g, v)
	}
	_ = writeClipboard(r.CID)
	return app.closeCopyMenu(g, v)
}

func (app *Gui) copyRecordJSON(g *gocui.Gui, v *gocui.View) error {
	r := app.cursorRecord()
	if r == nil || r.CID == "" {
		return app.closeCopyMenu(g, v)
	}
	cid := r.CID
	if err := app.closeCopyMenu(g, v); err != nil {
		return err
	}
	go app.fetchAndCopyJSON(cid)
	return nil
}

func (app *Gui) fetchAndCopyJSON(cid string) {
	ctx := context.Background()
	jsonStr, err := app.state.client.PullJSON(ctx, cid)
	if err != nil {
		app.g.Update(func(g *gocui.Gui) error {
			app.state.recordInfoCID = cid
			app.state.recordInfoText = "Failed to fetch record: " + err.Error()
			app.state.recordInfoError = true
			app.state.recordInfoLoading = false
			app.openInfoPopup(g, viewRecords)
			_, _ = g.SetCurrentView(viewInfoPopup)
			app.renderInfoPopup(g)
			return nil
		})
		return
	}
	_ = writeClipboard(jsonStr)
}

// ── Confirmation popup ────────────────────────────────────────────────────────

func (app *Gui) openConfirmPopup(g *gocui.Gui, title, body string, onConfirm func()) {
	app.state.confirmPopupText = body
	app.state.onConfirmAction = onConfirm

	cv, err := g.View(viewConfirmPopup)
	if err != nil {
		return
	}
	cv.Clear()
	cv.Title = " " + title + " "
	fmt.Fprint(cv, body)
	app.showPopup(g, viewConfirmPopup)
}

func (app *Gui) closeConfirmPopup(g *gocui.Gui, v *gocui.View) error {
	app.state.onConfirmAction = nil
	app.state.confirmPopupText = ""
	app.hidePopup(g, viewConfirmPopup)
	return nil
}

func (app *Gui) confirmPopupYes(g *gocui.Gui, v *gocui.View) error {
	cb := app.state.onConfirmAction
	app.state.onConfirmAction = nil
	app.state.confirmPopupText = ""
	app.hidePopup(g, viewConfirmPopup)
	if cb != nil {
		cb()
	}
	return nil
}

func writeClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "windows":
		cmd = exec.Command("clip")
	default:
		cmd = exec.Command("xclip", "-selection", "clipboard")
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
