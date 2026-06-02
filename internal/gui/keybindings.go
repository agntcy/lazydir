// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"strings"

	"github.com/jesseduffield/gocui"
)

func (app *Gui) bindKeys(g *gocui.Gui) error {
	// ── Global ───────────────────────────────────────────────────────────────
	for _, key := range []interface{}{gocui.KeyCtrlC, 'q'} {
		if err := g.SetKeybinding("", key, gocui.ModNone, quit); err != nil {
			return err
		}
	}
	if err := g.SetKeybinding("", gocui.KeyTab, gocui.ModNone, app.cycleFocusForward); err != nil {
		return err
	}
	if err := g.SetKeybinding("", gocui.KeyBacktab, gocui.ModNone, app.cycleFocusBackward); err != nil {
		return err
	}
	if err := g.SetKeybinding("", '1', gocui.ModNone, app.focusView(viewDirectory)); err != nil {
		return err
	}
	if err := g.SetKeybinding("", '2', gocui.ModNone, app.focusView(viewFilters)); err != nil {
		return err
	}
	if err := g.SetKeybinding("", '3', gocui.ModNone, app.focusView(viewRecords)); err != nil {
		return err
	}
	if err := g.SetKeybinding("", '0', gocui.ModNone, app.focusView(viewPreview)); err != nil {
		return err
	}
	if err := g.SetKeybinding("", 'r', gocui.ModNone, app.refresh); err != nil {
		return err
	}

	// ── Input prompt (shared) ────────────────────────────────────────────────
	if err := g.SetKeybinding(viewInput, gocui.KeyEnter, gocui.ModNone, app.inputConfirm); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewInput, gocui.KeyEsc, gocui.ModNone, app.inputCancel); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewInput, gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}

	// ── Connections panel ────────────────────────────────────────────────────
	for _, key := range []interface{}{gocui.KeyArrowUp, 'k'} {
		if err := g.SetKeybinding(viewDirectory, key, gocui.ModNone, app.connCursorUp); err != nil {
			return err
		}
	}
	for _, key := range []interface{}{gocui.KeyArrowDown, 'j'} {
		if err := g.SetKeybinding(viewDirectory, key, gocui.ModNone, app.connCursorDown); err != nil {
			return err
		}
	}
	if err := g.SetKeybinding(viewDirectory, 'c', gocui.ModNone, app.openServerSelectPopup); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewDirectory, gocui.KeyEnter, gocui.ModNone, app.openServerSelectPopup); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewDirectory, 'i', gocui.ModNone, app.connToggleInfo); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewDirectory, 'y', gocui.ModNone, app.infoPopupCopyError); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewDirectory, gocui.KeyEsc, gocui.ModNone, app.connDismissInfo); err != nil {
		return err
	}

	// ── Server selection popup ──────────────────────────────────────────────
	for _, key := range []interface{}{gocui.KeyArrowUp, 'k'} {
		if err := g.SetKeybinding(viewServerMenu, key, gocui.ModNone, app.serverMenuUp); err != nil {
			return err
		}
	}
	for _, key := range []interface{}{gocui.KeyArrowDown, 'j'} {
		if err := g.SetKeybinding(viewServerMenu, key, gocui.ModNone, app.serverMenuDown); err != nil {
			return err
		}
	}
	if err := g.SetKeybinding(viewServerMenu, gocui.KeyEnter, gocui.ModNone, app.serverMenuSelect); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewServerMenu, gocui.KeyEsc, gocui.ModNone, app.serverMenuClose); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewServerMenu, 'q', gocui.ModNone, app.serverMenuClose); err != nil {
		return err
	}

	// ── Auth/error popup ────────────────────────────────────────────────────
	if err := g.SetKeybinding(viewAuthPopup, gocui.KeyEsc, gocui.ModNone, app.dismissAuthPopup); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewAuthPopup, gocui.KeyEnter, gocui.ModNone, app.dismissAuthPopup); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewAuthPopup, 'q', gocui.ModNone, app.dismissAuthPopup); err != nil {
		return err
	}

	// ── Filters panel ────────────────────────────────────────────────────────
	for _, key := range []interface{}{gocui.KeyArrowUp, 'k'} {
		if err := g.SetKeybinding(viewFilters, key, gocui.ModNone, app.filterCursorUp); err != nil {
			return err
		}
	}
	for _, key := range []interface{}{gocui.KeyArrowDown, 'j'} {
		if err := g.SetKeybinding(viewFilters, key, gocui.ModNone, app.filterCursorDown); err != nil {
			return err
		}
	}
	if err := g.SetKeybinding(viewFilters, gocui.KeyEnter, gocui.ModNone, app.filterEnter); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewFilters, gocui.KeySpace, gocui.ModNone, app.filterToggleOption); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewFilters, gocui.KeyEsc, gocui.ModNone, app.filterEsc); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewFilters, '/', gocui.ModNone, app.filterOpenSearch); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewFilters, 'i', gocui.ModNone, app.filterToggleInfo); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewFilters, 'x', gocui.ModNone, app.filterClearAll); err != nil {
		return err
	}

	// ── Records panel ────────────────────────────────────────────────────────
	for _, key := range []interface{}{gocui.KeyArrowUp, 'k'} {
		if err := g.SetKeybinding(viewRecords, key, gocui.ModNone, app.recordCursorUp); err != nil {
			return err
		}
	}
	for _, key := range []interface{}{gocui.KeyArrowDown, 'j'} {
		if err := g.SetKeybinding(viewRecords, key, gocui.ModNone, app.recordCursorDown); err != nil {
			return err
		}
	}
	if err := g.SetKeybinding(viewRecords, gocui.KeyEnter, gocui.ModNone, app.recordSelect); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewRecords, '/', gocui.ModNone, app.openFilterDialog); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewRecords, gocui.KeyEsc, gocui.ModNone, app.clearFilter); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewRecords, 'i', gocui.ModNone, app.recordToggleInfo); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewRecords, 'y', gocui.ModNone, app.openCopyMenu); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewRecords, 'd', gocui.ModNone, app.recordDelete); err != nil {
		return err
	}

	// ── Info popup ──────────────────────────────────────────────────────────
	if err := g.SetKeybinding(viewInfoPopup, gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewInfoPopup, gocui.KeyEsc, gocui.ModNone, app.infoPopupClose); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewInfoPopup, 'i', gocui.ModNone, app.infoPopupClose); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewInfoPopup, 'y', gocui.ModNone, app.infoPopupCopyError); err != nil {
		return err
	}
	for _, key := range []interface{}{gocui.KeyArrowUp, 'k'} {
		if err := g.SetKeybinding(viewInfoPopup, key, gocui.ModNone, app.previewScrollUp); err != nil {
			return err
		}
	}
	for _, key := range []interface{}{gocui.KeyArrowDown, 'j'} {
		if err := g.SetKeybinding(viewInfoPopup, key, gocui.ModNone, app.previewScrollDown); err != nil {
			return err
		}
	}

	// ── Copy menu popup ─────────────────────────────────────────────────────
	if err := g.SetKeybinding(viewCopyMenu, 'c', gocui.ModNone, app.copyCID); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewCopyMenu, 'a', gocui.ModNone, app.copyRecordJSON); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewCopyMenu, gocui.KeyEsc, gocui.ModNone, app.closeCopyMenu); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewCopyMenu, gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}

	// ── Confirmation popup ─────────────────────────────────────────────────
	if err := g.SetKeybinding(viewConfirmPopup, 'y', gocui.ModNone, app.confirmPopupYes); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewConfirmPopup, gocui.KeyEnter, gocui.ModNone, app.confirmPopupYes); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewConfirmPopup, 'n', gocui.ModNone, app.closeConfirmPopup); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewConfirmPopup, gocui.KeyEsc, gocui.ModNone, app.closeConfirmPopup); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewConfirmPopup, gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}

	// ── Preview panel ────────────────────────────────────────────────────────
	for _, key := range []interface{}{gocui.KeyArrowUp, 'k'} {
		if err := g.SetKeybinding(viewPreview, key, gocui.ModNone, app.previewCursorUp); err != nil {
			return err
		}
	}
	for _, key := range []interface{}{gocui.KeyArrowDown, 'j'} {
		if err := g.SetKeybinding(viewPreview, key, gocui.ModNone, app.previewCursorDown); err != nil {
			return err
		}
	}
	if err := g.SetKeybinding(viewPreview, gocui.KeyEnter, gocui.ModNone, app.previewToggleNode); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewPreview, 'e', gocui.ModNone, app.previewExpandAll); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewPreview, 'E', gocui.ModNone, app.previewCollapseAll); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewPreview, gocui.MouseWheelUp, gocui.ModNone, app.previewScrollUp); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewPreview, gocui.MouseWheelDown, gocui.ModNone, app.previewScrollDown); err != nil {
		return err
	}

	// Mouse wheel scrolling on list panels
	if err := g.SetKeybinding(viewFilters, gocui.MouseWheelUp, gocui.ModNone, app.filterCursorUp); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewFilters, gocui.MouseWheelDown, gocui.ModNone, app.filterCursorDown); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewRecords, gocui.MouseWheelUp, gocui.ModNone, app.recordCursorUp); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewRecords, gocui.MouseWheelDown, gocui.ModNone, app.recordCursorDown); err != nil {
		return err
	}

	// Mouse click focuses the clicked panel; records, filters, and preview
	// get specialised handlers that also update the cursor / toggle nodes.
	for _, name := range []string{viewDirectory} {
		n := name
		if err := g.SetKeybinding(n, gocui.MouseLeft, gocui.ModNone, app.focusView(n)); err != nil {
			return err
		}
	}
	if err := g.SetKeybinding(viewPreview, gocui.MouseLeft, gocui.ModNone, app.previewMouseClick); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewRecords, gocui.MouseLeft, gocui.ModNone, app.recordMouseClick); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewFilters, gocui.MouseLeft, gocui.ModNone, app.filterMouseClick); err != nil {
		return err
	}

	// ? opens help popup for all main panels
	for _, name := range []string{"", viewDirectory, viewFilters, viewRecords, viewPreview} {
		if err := g.SetKeybinding(name, '?', gocui.ModNone, app.openHelp); err != nil {
			return err
		}
	}
	if err := g.SetKeybinding(viewHelp, gocui.KeyEsc, gocui.ModNone, app.closeHelp); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewHelp, '?', gocui.ModNone, app.closeHelp); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewHelp, gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}
	for _, key := range []interface{}{gocui.KeyArrowUp, 'k'} {
		if err := g.SetKeybinding(viewHelp, key, gocui.ModNone, app.previewScrollUp); err != nil {
			return err
		}
	}
	for _, key := range []interface{}{gocui.KeyArrowDown, 'j'} {
		if err := g.SetKeybinding(viewHelp, key, gocui.ModNone, app.previewScrollDown); err != nil {
			return err
		}
	}

	return nil
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}

// ── Input prompt handlers ─────────────────────────────────────────────────────

func (app *Gui) inputConfirm(g *gocui.Gui, v *gocui.View) error {
	value := strings.TrimSpace(v.TextArea.GetContent())
	cb := app.state.onInputConfirm
	app.closeInput()
	if cb != nil {
		cb(value)
	}
	return nil
}

func (app *Gui) inputCancel(g *gocui.Gui, v *gocui.View) error {
	cb := app.state.onInputCancel
	app.closeInput()
	if cb != nil {
		cb()
	}
	return nil
}

// ── Focus helpers ─────────────────────────────────────────────────────────────

var focusOrder = []string{viewDirectory, viewFilters, viewRecords, viewPreview}

// focusTo sets the current view and updates highlight state on list panels.
// When focus arrives at or leaves the records panel, the preview is refreshed
// so it always shows the record under the cursor. Focusing the preview panel
// itself does not re-fetch (which would reset the tree state).
func (app *Gui) focusTo(g *gocui.Gui, name string) error {
	_, err := g.SetCurrentView(name)
	if err != nil {
		return err
	}
	app.syncHighlight(g, name)
	app.renderStatus(g)
	if name != viewPreview {
		app.autoPreviewRecord(g)
	}
	return nil
}

func (app *Gui) focusView(name string) func(*gocui.Gui, *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		return app.focusTo(g, name)
	}
}

func (app *Gui) cycleFocusForward(g *gocui.Gui, v *gocui.View) error {
	return app.cycleFocus(g, 1)
}

func (app *Gui) cycleFocusBackward(g *gocui.Gui, v *gocui.View) error {
	return app.cycleFocus(g, -1)
}

func (app *Gui) cycleFocus(g *gocui.Gui, dir int) error {
	cur := g.CurrentView()
	curName := ""
	if cur != nil {
		curName = cur.Name()
	}
	idx := 0
	for i, name := range focusOrder {
		if name == curName {
			idx = i
			break
		}
	}
	next := (idx + dir + len(focusOrder)) % len(focusOrder)
	return app.focusTo(g, focusOrder[next])
}

// ── Refresh ───────────────────────────────────────────────────────────────────

func (app *Gui) refresh(g *gocui.Gui, v *gocui.View) error {
	if app.state.client == nil {
		return nil
	}
	app.startRecordsStream()
	return nil
}
