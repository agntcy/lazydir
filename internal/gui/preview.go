// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/agntcy/lazydir/internal/dirclient"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/jesseduffield/gocui"
)

// lazydirStyle is a chroma style derived from "tango" with punctuation
// remapped to plain white so that { } [ ] ( ) , : ; are readable on dark
// terminals instead of the default bold-black that tango uses.
var lazydirStyle = func() *chroma.Style {
	base := styles.Get("tango")
	if base == nil {
		base = styles.Fallback
	}
	b := base.Builder()
	b.Add(chroma.Punctuation, "#ffffff bold")
	s, err := b.Build()
	if err != nil {
		return base
	}
	return s
}()

// jsonLexer and termFormatter are cached at package level so that
// highlightJSON avoids re-creating them on every call.
var (
	jsonLexer     chroma.Lexer
	termFormatter chroma.Formatter
)

func init() {
	l := lexers.Get("json")
	if l == nil {
		l = lexers.Fallback
	}
	jsonLexer = chroma.Coalesce(l)
	f := formatters.Get("terminal16")
	if f == nil {
		f = formatters.Fallback
	}
	termFormatter = f
}

// ── Preview panel: data fetching ──────────────────────────────────────────────

func (app *Gui) pullRecord(subtitle, cid string) {
	ctx := context.Background()
	jsonStr, err := app.state.client.PullJSON(ctx, cid)
	app.g.Update(func(g *gocui.Gui) error {
		if err != nil {
			app.renderPreviewText(g, "Error", err.Error())
			return nil
		}
		app.renderPreviewJSON(g, subtitle, jsonStr)
		return nil
	})
}

// autoPreviewRecord fires a background pull for the record currently under the
// cursor, resetting the preview scroll position first. For group headers it
// previews the first record in the group.
func (app *Gui) autoPreviewRecord(g *gocui.Gui) {
	rows := app.state.recordDisplayRows
	if app.state.recordCursor >= len(rows) {
		return
	}
	row := rows[app.state.recordCursor]
	var rec *dirclient.RecordSummary
	if row.record != nil {
		rec = row.record
	} else {
		rec = app.firstRecordInGroup(row.groupName)
	}
	if rec == nil || rec.CID == "" {
		return
	}
	subtitle := rec.Name
	if subtitle == "" {
		subtitle = rec.CID
	}
	if rec.Version != "" {
		subtitle += " " + rec.Version
	}
	if pv, err := g.View(viewPreview); err == nil {
		_ = pv.SetOrigin(0, 0)
	}
	go app.pullRecord(subtitle, rec.CID)
}

// firstRecordInGroup returns the latest-version record in the named group.
// When expanded, the first child row is already the latest (sorted).
// When collapsed, it scans filteredRecords and picks the highest version.
func (app *Gui) firstRecordInGroup(name string) *dirclient.RecordSummary {
	if app.state.recordGroupExpanded[name] {
		found := false
		for _, row := range app.state.recordDisplayRows {
			if row.record == nil && row.groupName == name {
				found = true
				continue
			}
			if found {
				if row.record != nil {
					return row.record
				}
				break
			}
		}
	}
	var best *dirclient.RecordSummary
	for _, r := range app.state.filteredRecords {
		rName := r.Name
		if rName == "" {
			rName = r.CID
		}
		if rName != name {
			continue
		}
		if best == nil || compareVersions(r.Version, best.Version) > 0 {
			best = r
		}
	}
	return best
}

// ── Preview panel: rendering ──────────────────────────────────────────────────

// renderPreviewText sets plain text content in the preview panel.
func (app *Gui) renderPreviewText(g *gocui.Gui, subtitle, content string) {
	app.state.previewSubtitle = subtitle
	app.state.previewContent = content
	app.state.previewTree = nil
	app.state.previewCursor = 0
	app.setPreviewWrap(g, true)
	app.writePreview(g, true)
}

// renderPreviewJSON parses JSON into a collapsible tree (root expanded by
// default), syntax-highlights the visible lines, and writes them to the
// preview panel.
func (app *Gui) renderPreviewJSON(g *gocui.Gui, subtitle, jsonStr string) {
	app.state.previewSubtitle = subtitle
	tree, err := parseJSONTree(jsonStr)
	if err != nil {
		app.state.previewTree = nil
		app.state.previewContent = highlightJSON(jsonStr)
		app.setPreviewWrap(g, true)
	} else {
		app.state.previewTree = tree
		app.state.previewContent = app.highlightTree(tree)
		app.setPreviewWrap(g, false)
	}
	app.state.previewCursor = 0
	app.writePreview(g, true)
}

// setPreviewWrap enables or disables line wrapping on the preview view.
// Wrapping is on for plain text and off for JSON trees so that the gocui
// cursor row always maps 1:1 to the logical tree line index.
func (app *Gui) setPreviewWrap(g *gocui.Gui, on bool) {
	v, err := g.View(viewPreview)
	if err != nil {
		return
	}
	v.Wrap = on
}

// writePreview renders the stored preview content into the preview view.
// When a right-column popup is active the content is dimmed so the popup
// stands out visually. If resetScroll is true the view scrolls back to top.
// The gocui cursor is positioned on previewCursor so the highlight row is
// always visible regardless of which panel has focus.
func (app *Gui) writePreview(g *gocui.Gui, resetScroll bool) {
	v, err := g.View(viewPreview)
	if err != nil {
		return
	}
	v.Title = previewTitle(app.state.previewSubtitle)

	dimmed := app.shouldDimPreview(g)
	app.state.previewDimmed = dimmed
	if dimmed {
		v.FrameColor = app.theme.DimFrameColor
		v.TitleColor = app.theme.DimFrameColor
	} else {
		v.FrameColor = gocui.ColorDefault
		v.TitleColor = gocui.ColorDefault
	}

	v.Clear()
	if resetScroll {
		_ = v.SetOrigin(0, 0)
	}

	content := app.state.previewContent
	if dimmed && content != "" {
		content = dimText(content, app.theme.DimCode)
	}
	fmt.Fprint(v, content)

	if app.state.previewTree != nil {
		app.positionPreviewCursor(v)
	}
}

// positionPreviewCursor sets the gocui view cursor on the preview panel to
// match previewCursor, scrolling the origin if needed. This makes the
// highlight row visible even when the preview panel is not focused.
func (app *Gui) positionPreviewCursor(v *gocui.View) {
	cursor := app.state.previewCursor
	_, viewH := v.Size()
	_, oy := v.Origin()

	if cursor < oy {
		_ = v.SetOrigin(0, cursor)
		oy = cursor
	} else if cursor >= oy+viewH {
		oy = cursor - viewH + 1
		_ = v.SetOrigin(0, oy)
	}
	_ = v.SetCursor(0, cursor-oy)
}

// refreshPreviewTree re-renders the JSON tree after a toggle and writes the
// result to the preview view, preserving the current scroll position.
func (app *Gui) refreshPreviewTree(g *gocui.Gui) {
	tree := app.state.previewTree
	if tree == nil {
		return
	}
	app.state.previewContent = app.highlightTree(tree)
	app.writePreview(g, false)
}

// treeRenderCtx builds the render context for the JSON tree from the current
// app state, providing OASF class entries and theme for caption enrichment.
func (app *Gui) treeRenderCtx() *jsonRenderCtx {
	return &jsonRenderCtx{
		classEntries: app.state.classEntries,
		theme:        &app.theme,
	}
}

func previewTitle(subtitle string) string {
	if subtitle == "" {
		return "[0] Preview"
	}
	return "[0] Preview — " + subtitle
}

// ── Preview panel: cursor navigation and tree interaction ─────────────────────

const defaultScrollStep = 3

func (app *Gui) previewScrollUp(g *gocui.Gui, v *gocui.View) error {
	return app.scrollViewUp(v)
}

func (app *Gui) previewScrollDown(g *gocui.Gui, v *gocui.View) error {
	return app.scrollViewDown(v)
}

func (app *Gui) scrollStep() int {
	if app.cfg.ScrollStep > 0 {
		return app.cfg.ScrollStep
	}
	return defaultScrollStep
}

func (app *Gui) scrollViewUp(v *gocui.View) error {
	if v == nil {
		return nil
	}
	step := app.scrollStep()
	_, oy := v.Origin()
	if oy > 0 {
		newOY := oy - step
		if newOY < 0 {
			newOY = 0
		}
		_ = v.SetOrigin(0, newOY)
	}
	return nil
}

func (app *Gui) scrollViewDown(v *gocui.View) error {
	if v == nil {
		return nil
	}
	step := app.scrollStep()
	_, oy := v.Origin()
	_ = v.SetOrigin(0, oy+step)
	return nil
}

// previewCursorUp moves the preview cursor up by one line and updates the
// gocui cursor so the highlight row follows.
func (app *Gui) previewCursorUp(g *gocui.Gui, v *gocui.View) error {
	if app.state.previewTree == nil {
		return app.scrollViewUp(v)
	}
	if app.state.previewCursor > 0 {
		app.state.previewCursor--
	}
	app.positionPreviewCursor(v)
	return nil
}

// previewCursorDown moves the preview cursor down by one line and updates the
// gocui cursor so the highlight row follows.
func (app *Gui) previewCursorDown(g *gocui.Gui, v *gocui.View) error {
	if app.state.previewTree == nil {
		return app.scrollViewDown(v)
	}
	maxLine := len(app.state.previewTree.lines) - 1
	if maxLine < 0 {
		return nil
	}
	if app.state.previewCursor < maxLine {
		app.state.previewCursor++
	}
	app.positionPreviewCursor(v)
	return nil
}

// previewToggleNode expands or collapses the JSON tree node on the current
// cursor line.
func (app *Gui) previewToggleNode(g *gocui.Gui, v *gocui.View) error {
	tree := app.state.previewTree
	if tree == nil {
		return nil
	}
	if !tree.toggleLine(app.state.previewCursor) {
		return nil
	}
	app.refreshPreviewTree(g)
	clampPreviewCursor(app, tree)
	return nil
}

func (app *Gui) previewExpandAll(g *gocui.Gui, v *gocui.View) error {
	tree := app.state.previewTree
	if tree == nil {
		return nil
	}
	tree.expandAll()
	app.refreshPreviewTree(g)
	clampPreviewCursor(app, tree)
	return nil
}

func (app *Gui) previewCollapseAll(g *gocui.Gui, v *gocui.View) error {
	tree := app.state.previewTree
	if tree == nil {
		return nil
	}
	tree.collapseAll()
	app.state.previewCursor = 0
	app.refreshPreviewTree(g)
	return nil
}

// previewMouseClick handles mouse clicks in the preview panel: focuses the
// panel, moves the cursor to the clicked line, and toggles the node if it's
// collapsible.
func (app *Gui) previewMouseClick(g *gocui.Gui, v *gocui.View) error {
	if err := app.focusTo(g, viewPreview); err != nil {
		return err
	}
	tree := app.state.previewTree
	if tree == nil {
		return nil
	}
	_, cy := v.Cursor()
	_, oy := v.Origin()
	line := oy + cy
	if line < 0 || line >= len(tree.lines) {
		return nil
	}
	app.state.previewCursor = line
	if tree.toggleLine(line) {
		app.refreshPreviewTree(g)
		clampPreviewCursor(app, tree)
	} else {
		app.positionPreviewCursor(v)
	}
	return nil
}

func clampPreviewCursor(app *Gui, tree *jsonTree) {
	maxLine := len(tree.lines) - 1
	if maxLine < 0 {
		app.state.previewCursor = 0
	} else if app.state.previewCursor > maxLine {
		app.state.previewCursor = maxLine
	}
}

// ── Preview panel: syntax highlighting ────────────────────────────────────────

// highlightTree renders a JSON tree with syntax highlighting. Plain JSON
// lines (no triangles or ANSI escapes) are batched into a single chroma
// call for performance. Mixed lines are highlighted per-segment so that
// triangles, colored brackets, and OASF captions keep their styling.
func (app *Gui) highlightTree(tree *jsonTree) string {
	raw := tree.renderLines(app.treeRenderCtx())
	lines := strings.Split(raw, "\n")

	isMixed := make([]bool, len(lines))
	var plainBatch []string

	for i, line := range lines {
		if isMixedLine(line) {
			isMixed[i] = true
		} else {
			plainBatch = append(plainBatch, line)
		}
	}

	// Highlight all plain lines in one chroma pass and split back.
	var highlighted []string
	if len(plainBatch) > 0 {
		joined := highlightJSON(strings.Join(plainBatch, "\n"))
		highlighted = strings.Split(joined, "\n")
		// Chroma may append a trailing newline producing an extra empty
		// element — trim it so the count matches.
		if len(highlighted) > len(plainBatch) {
			if highlighted[len(highlighted)-1] == "" {
				highlighted = highlighted[:len(highlighted)-1]
			}
		}
	}

	var sb strings.Builder
	pi := 0
	for i, line := range lines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		if isMixed[i] {
			sb.WriteString(highlightMixedLine(line))
		} else if pi < len(highlighted) {
			sb.WriteString(highlighted[pi])
			pi++
		} else {
			sb.WriteString(line)
		}
	}
	return sb.String()
}

// isMixedLine returns true when the line contains triangle indicators or
// pre-colored ANSI escape sequences that require per-segment highlighting.
func isMixedLine(line string) bool {
	for _, tri := range []string{triangleCollapsed, triangleExpanded} {
		if strings.Contains(line, tri) {
			return true
		}
	}
	return strings.Contains(line, "\033[")
}

// highlightMixedLine processes a line that contains a mix of plain text and
// ANSI-colored segments (brackets, captions, triangles). It highlights only
// the plain-text spans with chroma and passes colored spans through as-is.
func highlightMixedLine(line string) string {
	const ansiReset = "\033[0m"

	var sb strings.Builder
	i := 0
	plainStart := 0

	for i < len(line) {
		foundTri := false
		for _, tri := range []string{triangleCollapsed, triangleExpanded} {
			if strings.HasPrefix(line[i:], tri) {
				if i > plainStart {
					sb.WriteString(highlightJSONInline(line[plainStart:i]))
				}
				sb.WriteString(tri)
				i += len(tri)
				plainStart = i
				foundTri = true
				break
			}
		}
		if foundTri {
			continue
		}

		if i+1 < len(line) && line[i] == '\033' && line[i+1] == '[' {
			if i > plainStart {
				sb.WriteString(highlightJSONInline(line[plainStart:i]))
			}
			end := strings.Index(line[i:], ansiReset)
			if end >= 0 {
				end = i + end + len(ansiReset)
			} else {
				end = len(line)
			}
			sb.WriteString(line[i:end])
			i = end
			plainStart = i
			continue
		}

		i++
	}

	if plainStart < len(line) {
		sb.WriteString(highlightJSONInline(line[plainStart:]))
	}
	return sb.String()
}

func highlightJSONInline(s string) string {
	h := highlightJSON(s)
	return strings.TrimRight(h, "\n")
}

func highlightJSON(src string) string {
	iter, err := jsonLexer.Tokenise(nil, src)
	if err != nil {
		return src
	}

	var buf bytes.Buffer
	if err := termFormatter.Format(&buf, lazydirStyle, iter); err != nil {
		return src
	}

	return buf.String()
}

// ── Text helpers ──────────────────────────────────────────────────────────────

func dimText(s, dimCode string) string {
	return dimCode + stripANSI(s) + "\033[0m"
}

func stripANSI(s string) string {
	var buf strings.Builder
	buf.Grow(len(s))
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
		buf.WriteByte(s[i])
		i++
	}
	return buf.String()
}

func isCSITerminator(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}
