// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"fmt"
	"strings"

	"github.com/jesseduffield/gocui"
)

const (
	viewDirectory  = "directory"
	viewFilters    = "filters"
	viewRecords    = "records"
	viewPreview    = "preview"
	viewOptions    = "options"    // bottom bar: context keybindings (like lazygit)
	viewInput      = "input"      // shared editable prompt view, shown on demand
	viewHelp       = "help"       // ? popup overlay, shown on demand
	viewCopyMenu   = "copymenu"   // copy-options popup, shown on demand
	viewInfoPopup  = "infopopup"  // info popup, shown on demand (i key)
	viewServerMenu = "servermenu" // server selection popup, shown on demand (c key)
	viewAuthPopup  = "authpopup"  // OIDC auth popup, shown during device flow
)

// roundedFrame is a 6-rune set that gives every panel rounded corners: ╭─╮╰─╯
var roundedFrame = []rune{'─', '│', '╭', '╮', '╰', '╯'}

// listViews are the panels that show a highlighted cursor row.
var listViews = []string{viewDirectory, viewFilters, viewRecords}

// rightColumnPopups are popup views rendered over the preview panel.
var rightColumnPopups = []string{viewInfoPopup, viewCopyMenu, viewServerMenu, viewAuthPopup}

// layout is the gocui Manager — called on every redraw/resize.
func (g *Gui) layout(gui *gocui.Gui) error {
	maxX, maxY := gui.Size()

	// The bottom bar occupies the last row (maxY-1).
	// All panels extend down to maxY-2, so they sit flush against it.
	bottomY0 := maxY - 2
	bottomY1 := maxY
	panelBottom := maxY - 2

	leftW := g.leftColumnWidth(maxX)
	rightX0 := leftW

	optionsX1 := maxX - 1

	// [1] Connections shows two lines (Directory, OASF).
	// Height = frame(2) + 2 content lines.
	dirH := 4

	// The input prompt, when visible, steals a 3-row slot on the left column
	// above the panel that requested it (the "host"). Panels below the host
	// are all shifted down by inputSlot rows.
	const inputSlot = 3
	inputHost := g.inputHostView()
	showInput := g.state.inputVisible

	var (
		dirY0, dirY1     = 0, dirH - 1
		filtersY0        int
		filtersY1        int
		recordY0         int
		inputX0, inputY0 = 0, 0
		inputX1, inputY1 = 0, 0
	)

	slotOffsetDir := 0
	slotOffsetFilters := 0
	slotOffsetRecord := 0
	if showInput {
		switch inputHost {
		case viewDirectory:
			slotOffsetDir = inputSlot
			slotOffsetFilters = inputSlot
			slotOffsetRecord = inputSlot
		case viewFilters:
			slotOffsetFilters = inputSlot
			slotOffsetRecord = inputSlot
		case viewRecords, viewPreview, "":
			slotOffsetRecord = inputSlot
		}
	}

	// Connections panel placement.
	dirY0 += slotOffsetDir
	dirY1 += slotOffsetDir

	// Filters panel: vertical space left between Connections and Records.
	filtersY0 = dirY1 + 1 + (slotOffsetFilters - slotOffsetDir)
	filtersH := (panelBottom - dirY1 - 1 - slotOffsetFilters + slotOffsetDir) / 2
	if filtersH < 3 {
		filtersH = 3
	}
	filtersY1 = filtersY0 + filtersH - 1

	// Records panel starts right after Filters, plus any extra slot above it.
	recordY0 = filtersY1 + 1 + (slotOffsetRecord - slotOffsetFilters)
	if recordY0 >= panelBottom {
		recordY0 = panelBottom - 3
	}

	// Input prompt coordinates on the left column.
	if showInput {
		inputX0 = 0
		inputX1 = leftW - 1
		switch inputHost {
		case viewDirectory:
			inputY0 = 0
		case viewFilters:
			inputY0 = dirY1 + 1
		default:
			inputY0 = filtersY1 + 1
		}
		inputY1 = inputY0 + inputSlot - 1
	}

	// [1] Connections panel
	if v, err := gui.SetView(viewDirectory, 0, dirY0, leftW-1, dirY1, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
		v.Title = "[1] Connections"
		v.Frame = true
		v.Wrap = false
		v.Highlight = false
		v.SelBgColor = g.theme.SelectedRowBg
		v.SelFgColor = gocui.ColorDefault
		v.FrameRunes = roundedFrame
		g.renderDirectory(gui)
	}

	// [2] Filters panel
	if v, err := gui.SetView(viewFilters, 0, filtersY0, leftW-1, filtersY1, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
		v.Title = "[2] Filters"
		v.Frame = true
		v.Highlight = false
		v.SelBgColor = g.theme.SelectedRowBg
		v.SelFgColor = gocui.ColorDefault
		v.FrameRunes = roundedFrame
	}

	// [3] Records panel — extends to panelBottom
	if v, err := gui.SetView(viewRecords, 0, recordY0, leftW-1, panelBottom, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
		v.Title = "[3] Records"
		v.Frame = true
		v.Highlight = false
		v.SelBgColor = g.theme.SelectedRowBg
		v.SelFgColor = gocui.ColorDefault
		v.FrameRunes = roundedFrame
	}

	// Preview panel — extends to panelBottom
	if v, err := gui.SetView(viewPreview, rightX0, 0, maxX-1, panelBottom, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
		v.Title = "[0] Preview"
		v.Frame = true
		v.Wrap = true
		v.FrameRunes = roundedFrame
		v.CanScrollPastBottom = true
	}

	// Bottom-left: options bar — properties set every layout call (no frame)
	if _, err := gui.SetView(viewOptions, 0, bottomY0, optionsX1, bottomY1, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
	}
	if v, _ := gui.View(viewOptions); v != nil {
		v.Frame = false
	}

	// ── Input prompt — left column above its host panel ──────────────────

	if !showInput {
		inputX0, inputX1 = 0, leftW-1
		inputY0, inputY1 = -inputSlot-1, -2
	}
	if v, err := gui.SetView(viewInput, inputX0, inputY0, inputX1, inputY1, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
		v.Editable = true
		v.KeybindOnEdit = true
		v.Wrap = false
		v.Frame = true
		v.FrameRunes = roundedFrame
		v.Visible = false
	}
	if v, _ := gui.View(viewInput); v != nil && v.Visible {
		_, _ = gui.SetViewOnTop(viewInput)
	}

	// ── Popups — positioned in the preview (right) column ────────────────

	// Info popup — sized to content, positioned in the right column.
	ipX0, ipY0, ipX1, ipY1 := 0, -10, 10, -1
	if ipv, _ := gui.View(viewInfoPopup); ipv != nil && ipv.Visible {
		ipText := g.infoPopupText()
		ipMaxW := maxX - 1 - rightX0 - 2
		ipCW, ipCH := popupContentSize(ipText, ipMaxW)
		ipX0, ipY0, ipX1, ipY1 = popupRect(gui, g.state.infoPopupPanel,
			ipCW, ipCH, rightX0, maxX, panelBottom, dirY0, filtersY0, recordY0)
	}
	if v, err := gui.SetView(viewInfoPopup, ipX0, ipY0, ipX1, ipY1, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
		v.Title = " Info "
		v.Frame = true
		v.FrameRunes = roundedFrame
		v.Wrap = true
		v.Visible = false
	}

	// Copy-menu popup — positioned in the right column.
	cmX0, cmY0, cmX1, cmY1 := 0, -6, 28, -1
	if cmv, _ := gui.View(viewCopyMenu); cmv != nil && cmv.Visible {
		cmX0, cmY0, cmX1, cmY1 = popupRect(gui, viewRecords,
			24, 2, rightX0, maxX, panelBottom, dirY0, filtersY0, recordY0)
	}
	if v, err := gui.SetView(viewCopyMenu, cmX0, cmY0, cmX1, cmY1, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
		v.Title = " Copy options "
		v.Frame = true
		v.FrameRunes = roundedFrame
		v.Wrap = false
		v.Visible = false
	}

	// Server selection popup — positioned in the right column.
	{
		smX0, smY0, smX1, smY1 := 0, -8, 40, -1
		if g.state.serverMenuVisible {
			smCW := 20
			for _, item := range g.state.serverMenuItems {
				if l := len(item) + 2; l > smCW {
					smCW = l
				}
			}
			smCH := len(g.state.serverMenuItems)
			if smCH < 1 {
				smCH = 1
			}
			smX0, smY0, smX1, smY1 = popupRect(gui, viewDirectory,
				smCW, smCH, rightX0, maxX, panelBottom, dirY0, filtersY0, recordY0)
		}
		if v, err := gui.SetView(viewServerMenu, smX0, smY0, smX1, smY1, 0); err != nil {
			if !gocui.IsUnknownView(err) {
				return err
			}
			v.Title = " Select server "
			v.Frame = true
			v.FrameRunes = roundedFrame
			v.Wrap = false
			v.Visible = false
			v.Highlight = true
			v.SelBgColor = g.theme.SelectedRowBg
			v.SelFgColor = gocui.ColorDefault
		} else {
			v.Visible = g.state.serverMenuVisible
			if g.state.serverMenuVisible {
				v.Clear()
				for _, item := range g.state.serverMenuItems {
					fmt.Fprintln(v, " "+item)
				}
				_ = v.SetCursor(0, g.state.serverMenuCursor)
			}
		}
	}

	// OIDC auth popup — positioned in the right column.
	{
		authX0, authY0, authX1, authY1 := 0, -10, 50, -1
		if av, _ := gui.View(viewAuthPopup); av != nil && av.Visible {
			authMaxW := maxX - 1 - rightX0 - 2
			authCW, authCH := popupContentSize(g.state.authPopupText, authMaxW)
			authX0, authY0, authX1, authY1 = popupRect(gui, viewDirectory,
				authCW, authCH, rightX0, maxX, panelBottom, dirY0, filtersY0, recordY0)
		}
		if v, err := gui.SetView(viewAuthPopup, authX0, authY0, authX1, authY1, 0); err != nil {
			if !gocui.IsUnknownView(err) {
				return err
			}
			v.Title = " OIDC Login "
			v.Frame = true
			v.FrameRunes = roundedFrame
			v.Wrap = true
			v.Visible = false
		} else {
			_, _ = gui.SetView(viewAuthPopup, authX0, authY0, authX1, authY1, 0)
		}
	}

	// Help popup overlay — centered, shown/hidden on demand.
	helpW := 54
	helpH := 22
	helpX0 := (maxX - helpW) / 2
	helpY0 := (maxY - helpH) / 2
	if v, err := gui.SetView(viewHelp, helpX0, helpY0, helpX0+helpW, helpY0+helpH, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
		v.Title = " Keybindings  (esc/? to close) "
		v.Frame = true
		v.FrameRunes = roundedFrame
		v.Wrap = false
		v.Visible = false
	}

	// Dim the preview panel when a popup overlays it.
	g.updatePreviewDim(gui)

	// First-time init: populate bottom bar and set focus.
	if gui.CurrentView() == nil {
		g.renderStatus(gui)
		g.renderDirectory(gui)
		if _, err := gui.SetCurrentView(viewDirectory); err != nil {
			return err
		}
		g.syncHighlight(gui, viewDirectory)
	}

	return nil
}

// syncHighlight enables the row-highlight cursor on the focused list view only,
// and disables it on all others — giving a clear visual focus cue. The focused
// panel's border is painted green via g.SelFrameColor set at init time.
func (g *Gui) syncHighlight(gui *gocui.Gui, focused string) {
	for _, name := range listViews {
		v, err := gui.View(name)
		if err != nil {
			continue
		}
		v.Highlight = (name == focused)
	}
}

func (g *Gui) renderStatus(gui *gocui.Gui) {
	focused := ""
	if cv := gui.CurrentView(); cv != nil {
		focused = cv.Name()
	}

	if v, err := gui.View(viewOptions); err == nil {
		v.Clear()
		fmt.Fprintf(v, "%s%s%s", g.theme.Color5, optionsBarText(focused, v.InnerWidth()), g.theme.Reset)
	}
}

// renderDirectory refreshes the [1] Connections panel with both the Directory
// and OASF endpoints the app is currently talking to. A sync indicator is
// appended to the Directory line while the records stream is in flight.
// connIcon returns the colored status indicator for a connection.
func (g *Gui) connIcon(status connStatus) string {
	switch status {
	case connOK:
		return g.theme.Color4 + "●" + g.theme.Reset
	case connFailed:
		return g.theme.Color6 + "●" + g.theme.Reset
	default:
		return g.theme.Color6 + "○" + g.theme.Reset
	}
}

// connSync returns " ↻" when the connection is in the trying state.
func connSync(status connStatus) string {
	if status == connTrying {
		return " ↻"
	}
	return ""
}

func (g *Gui) renderDirectory(gui *gocui.Gui) {
	v, err := gui.View(viewDirectory)
	if err != nil {
		return
	}
	v.Clear()

	dirSync := connSync(g.state.dirStatus)
	fmt.Fprintf(v, " %s Directory: %s%s\n", g.connIcon(g.state.dirStatus), g.state.serverAddr, dirSync)

	oasfAddr := g.state.oasfAddr
	if oasfAddr == "" {
		oasfAddr = "(not configured)"
	}
	fmt.Fprintf(v, " %s OASF:      %s%s", g.connIcon(g.state.oasfStatus), oasfAddr, connSync(g.state.oasfStatus))

	_ = v.SetCursor(0, g.state.connCursor)
}

// leftColumnWidth returns the pixel width of the left panel column,
// applying the configured split ratio with clamping and minimum.
func (g *Gui) leftColumnWidth(maxX int) int {
	splitRatio := g.cfg.SplitRatio
	if splitRatio <= 0 || splitRatio >= 1 {
		splitRatio = 0.33
	}
	leftW := int(float64(maxX) * splitRatio)
	if leftW < 10 {
		leftW = 10
	}
	return leftW
}

// inputHostView resolves which left-column panel the input prompt should
// attach itself to. The prompt is inserted above the host, shifting the
// panels below it down.
func (g *Gui) inputHostView() string {
	host := g.state.prevView
	switch host {
	case viewDirectory, viewFilters, viewRecords:
		return host
	default:
		return viewRecords
	}
}

// ── Preview dimming ──────────────────────────────────────────────────────────

// rightColumnPopupActive returns true if any popup is visible over the preview.
func (app *Gui) rightColumnPopupActive(gui *gocui.Gui) bool {
	for _, name := range rightColumnPopups {
		if v, err := gui.View(name); err == nil && v.Visible {
			return true
		}
	}
	return false
}

// shouldDimPreview returns true when the preview content should be dimmed
// (dimming is enabled and at least one right-column popup is visible).
func (app *Gui) shouldDimPreview(gui *gocui.Gui) bool {
	return app.theme.DimCode != "" && app.rightColumnPopupActive(gui)
}

// updatePreviewDim re-renders the preview with or without dimming when the
// popup overlay state changes. It preserves the current scroll position.
func (app *Gui) updatePreviewDim(gui *gocui.Gui) {
	if app.shouldDimPreview(gui) == app.state.previewDimmed {
		return
	}
	app.writePreview(gui, false)
}

// ── Popup positioning helpers ────────────────────────────────────────────────

// popupRect computes popup frame coordinates (x0, y0, x1, y1) in the preview
// (right) column, vertically anchored to the cursor row of sourcePanel. The
// popup bottom is clamped so it does not exceed panelBottom.
func popupRect(gui *gocui.Gui, sourcePanel string,
	contentW, contentH int,
	rightX0, maxX, panelBottom int,
	dirY0, filtersY0, recordY0 int) (int, int, int, int) {

	availW := maxX - 1 - rightX0
	frameW := contentW + 2
	if frameW > availW {
		frameW = availW
	}
	if frameW < 12 {
		frameW = 12
	}
	x0 := rightX0
	x1 := x0 + frameW

	sourceY0 := recordY0
	switch sourcePanel {
	case viewDirectory:
		sourceY0 = dirY0
	case viewFilters:
		sourceY0 = filtersY0
	}

	cy := 0
	if sv, err := gui.View(sourcePanel); err == nil {
		_, cy = sv.Cursor()
	}
	screenY := sourceY0 + 1 + cy

	frameH := contentH + 2
	y0 := screenY
	y1 := y0 + frameH - 1

	if y1 > panelBottom {
		y0 = panelBottom - frameH + 1
		if y0 < 0 {
			y0 = 0
		}
		y1 = y0 + frameH - 1
		if y1 > panelBottom {
			y1 = panelBottom
		}
	}

	return x0, y0, x1, y1
}

// popupContentSize computes the ideal (contentWidth, contentHeight) for text
// content, given a maximum available content width. Width is the longest line
// (capped at maxW); height is the number of visual lines after wrapping.
func popupContentSize(text string, maxW int) (int, int) {
	if text == "" {
		return 10, 1
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	longest := 0
	for _, line := range lines {
		if l := len(line); l > longest {
			longest = l
		}
	}
	w := longest
	if w > maxW {
		w = maxW
	}
	if w < 10 {
		w = 10
	}
	h := wrappedLineCount(text, w)
	return w, h
}

// infoPopupText returns the current text content for the info popup,
// used by the layout manager to compute popup dimensions.
func (g *Gui) infoPopupText() string {
	switch g.state.infoPopupPanel {
	case viewDirectory:
		text, _ := g.connInfoText()
		return text
	case viewFilters:
		if g.state.filters.inlineDescLoading {
			return "loading…"
		}
		return g.state.filters.inlineDescText
	case viewRecords:
		if g.state.recordInfoLoading {
			return "loading…"
		}
		return g.state.recordInfoText
	}
	return ""
}

// wrappedLineCount counts visual lines a string occupies at a given width.
// It strips ANSI escape sequences before measuring so colored text is sized
// correctly (ANSI codes take zero visual width).
func wrappedLineCount(text string, width int) int {
	total := 0
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		visLen := visualLen(line)
		if visLen == 0 {
			total++
			continue
		}
		total += (visLen-1)/width + 1
	}
	return total
}

// visualLen returns the visual (display) length of a string, excluding ANSI
// escape sequences that take zero terminal columns.
func visualLen(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if i+1 < len(s) && s[i] == '\033' && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && !isCSITerminator(s[j]) {
				j++
			}
			if j < len(s) {
				i = j + 1
				continue
			}
		}
		n++
		i++
	}
	return n
}
