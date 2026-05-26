// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

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
	_, _ = g.SetCurrentView(viewInfoPopup)
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
	return nil
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
		fmt.Fprintf(ipv, "\n\n  %sy%s  copy error   %si / esc%s  close",
			app.theme.Color2, app.theme.Reset,
			app.theme.Color2, app.theme.Reset)
		ipv.FrameColor = gocui.ColorRed
		ipv.TitleColor = gocui.ColorRed
	} else {
		ipv.FrameColor = gocui.ColorGreen
		ipv.TitleColor = gocui.ColorGreen
	}
	g.SelFrameColor = ipv.FrameColor
	g.SelFgColor = ipv.TitleColor
}

// infoPopupCopyError copies the current error message to the system clipboard.
func (app *Gui) infoPopupCopyError(g *gocui.Gui, v *gocui.View) error {
	ipv, _ := g.View(viewInfoPopup)
	if ipv == nil || !ipv.Visible {
		return nil
	}
	var errText string
	switch app.state.infoPopupPanel {
	case viewDirectory:
		if app.state.connCursor == 0 {
			errText = app.state.dirError
		} else {
			errText = app.state.oasfError
		}
	case viewFilters:
		if app.state.filters.inlineDescError {
			errText = app.state.filters.inlineDescText
		}
	case viewRecords:
		if app.state.recordInfoError {
			errText = app.state.recordInfoText
		}
	}
	if errText == "" {
		return nil
	}
	if err := writeClipboard(stripANSI(errText)); err != nil {
		ipv.Clear()
		fmt.Fprintf(ipv, "%sFailed to copy:%s %v", app.theme.Color6, app.theme.Reset, err)
		return nil
	}
	return app.infoPopupClose(g, v)
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
	if err := writeClipboard(r.CID); err != nil {
		cv, _ := g.View(viewCopyMenu)
		if cv != nil {
			cv.Clear()
			fmt.Fprintf(cv, "  %sFailed to copy:%s %v", app.theme.Color6, app.theme.Reset, err)
		}
		return nil
	}
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
			return nil
		})
		return
	}
	if err := writeClipboard(jsonStr); err != nil {
		app.g.Update(func(g *gocui.Gui) error {
			app.state.recordInfoCID = cid
			app.state.recordInfoText = "Failed to copy: " + err.Error()
			app.state.recordInfoError = true
			app.state.recordInfoLoading = false
			app.openInfoPopup(g, viewRecords)
			return nil
		})
	}
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
