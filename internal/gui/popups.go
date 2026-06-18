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
	app.renderStatus(g)
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
	app.renderStatus(g)
}

// ── Generic option menu ───────────────────────────────────────────────────────

// openMenu populates the shared menu state, renders options into the named
// view, and shows it as a popup with the cursor on the first row.
func (app *Gui) openMenu(g *gocui.Gui, viewName, title string, options []menuOption) {
	v, err := g.View(viewName)
	if err != nil {
		return
	}
	app.state.menu = menuState{
		cursor:  0,
		options: options,
		view:    viewName,
	}
	v.Title = " " + title + " "
	app.renderMenu(g)
	app.showPopup(g, viewName)
}

// renderMenu redraws the option labels and positions the gocui cursor.
func (app *Gui) renderMenu(g *gocui.Gui) {
	ms := &app.state.menu
	v, err := g.View(ms.view)
	if err != nil {
		return
	}
	v.Clear()
	for _, opt := range ms.options {
		fmt.Fprintf(v, " %s\n", opt.label)
	}
	_ = v.SetCursor(0, ms.cursor)
}

func (app *Gui) menuCursorUp(g *gocui.Gui, v *gocui.View) error {
	ms := &app.state.menu
	if ms.cursor > 0 {
		ms.cursor--
		app.renderMenu(g)
	}
	return nil
}

func (app *Gui) menuCursorDown(g *gocui.Gui, v *gocui.View) error {
	ms := &app.state.menu
	if ms.cursor < len(ms.options)-1 {
		ms.cursor++
		app.renderMenu(g)
	}
	return nil
}

func (app *Gui) menuSelect(g *gocui.Gui, v *gocui.View) error {
	ms := &app.state.menu
	if ms.cursor >= len(ms.options) {
		return nil
	}
	action := ms.options[ms.cursor].action
	app.hidePopup(g, ms.view)
	app.state.menu = menuState{}
	if action != nil {
		action()
	}
	return nil
}

func (app *Gui) menuClose(g *gocui.Gui, v *gocui.View) error {
	ms := &app.state.menu
	app.hidePopup(g, ms.view)
	app.state.menu = menuState{}
	return nil
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
	app.openMenu(g, viewCopyMenu, "Copy options", []menuOption{
		{label: "copy CID", action: app.doCopyCID},
		{label: "copy record JSON", action: app.doCopyRecordJSON},
	})
	return nil
}

func (app *Gui) doCopyCID() {
	r := app.cursorRecord()
	if r == nil || r.CID == "" {
		return
	}
	if err := writeClipboard(r.CID); err != nil {
		app.g.Update(func(g *gocui.Gui) error {
			app.state.recordInfoCID = r.CID
			app.state.recordInfoText = "Failed to copy: " + err.Error()
			app.state.recordInfoError = true
			app.state.recordInfoLoading = false
			app.openInfoPopup(g, viewRecords)
			return nil
		})
	}
}

func (app *Gui) doCopyRecordJSON() {
	r := app.cursorRecord()
	if r == nil || r.CID == "" {
		return
	}
	go app.fetchAndCopyJSON(r.CID)
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

// openConfirmPopup shows a confirmation dialog with body text and navigable
// "confirm" / "cancel" option rows (or just "dismiss" when onConfirm is nil).
func (app *Gui) openConfirmPopup(g *gocui.Gui, title, body string, onConfirm func()) {
	app.state.confirmPopupText = body

	var options []menuOption
	if onConfirm != nil {
		options = []menuOption{
			{label: "confirm", action: onConfirm},
			{label: "cancel", action: nil},
		}
	} else {
		options = []menuOption{
			{label: "dismiss", action: nil},
		}
	}

	app.state.menu = menuState{
		cursor:  0,
		options: options,
		view:    viewConfirmPopup,
	}

	cv, err := g.View(viewConfirmPopup)
	if err != nil {
		return
	}
	cv.Title = " " + title + " "
	app.renderConfirmPopup(g)
	app.showPopup(g, viewConfirmPopup)
}

// renderConfirmPopup redraws the confirm popup body text and appended menu
// options, positioning the gocui cursor on the selected option row.
func (app *Gui) renderConfirmPopup(g *gocui.Gui) {
	ms := &app.state.menu
	cv, err := g.View(ms.view)
	if err != nil {
		return
	}
	cv.Clear()
	body := app.state.confirmPopupText
	fmt.Fprint(cv, body)

	bodyLines := strings.Count(body, "\n")
	if body != "" && !strings.HasSuffix(body, "\n") {
		bodyLines++
	}
	fmt.Fprint(cv, "\n")

	for _, opt := range ms.options {
		fmt.Fprintf(cv, " %s\n", opt.label)
	}
	_ = cv.SetCursor(0, bodyLines+ms.cursor)
}

func (app *Gui) confirmMenuUp(g *gocui.Gui, v *gocui.View) error {
	ms := &app.state.menu
	if ms.cursor > 0 {
		ms.cursor--
		app.renderConfirmPopup(g)
	}
	return nil
}

func (app *Gui) confirmMenuDown(g *gocui.Gui, v *gocui.View) error {
	ms := &app.state.menu
	if ms.cursor < len(ms.options)-1 {
		ms.cursor++
		app.renderConfirmPopup(g)
	}
	return nil
}

func (app *Gui) confirmMenuSelect(g *gocui.Gui, v *gocui.View) error {
	ms := &app.state.menu
	if ms.cursor >= len(ms.options) {
		return nil
	}
	action := ms.options[ms.cursor].action
	app.state.confirmPopupText = ""
	app.hidePopup(g, viewConfirmPopup)
	app.state.menu = menuState{}
	if action != nil {
		action()
	}
	return nil
}

func (app *Gui) confirmMenuClose(g *gocui.Gui, v *gocui.View) error {
	app.state.confirmPopupText = ""
	app.hidePopup(g, viewConfirmPopup)
	app.state.menu = menuState{}
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
