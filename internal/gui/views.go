// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/agntcy/lazydir/internal/dirclient"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/jesseduffield/gocui"
)

// indent1 is the single-level indentation used for child rows everywhere in
// the TUI (applied filter selections, inline descriptions, inline record info).
const indent1 = "    "

// recordDisplayRow describes one rendered line in the records panel.
// It is either a group header (when record is nil) or a record entry.
type recordDisplayRow struct {
	groupName string                   // non-empty for group headers
	record    *dirclient.RecordSummary // non-nil for actual record entries
	grouped   bool                     // true if this record is part of a multi-version group
}

// lazydirStyle is a chroma style derived from "tango" with punctuation
// remapped to plain white so that { } [ ] ( ) , : ; are readable on dark
// terminals instead of the default bold-black that tango uses.
var lazydirStyle = func() *chroma.Style {
	base := styles.Get("tango")
	if base == nil {
		base = styles.Fallback
	}
	b := base.Builder()
	// "bold" alone inherits the token's foreground but ensures it is bright;
	// "#ffffff bold" forces bright white — readable on any dark background.
	b.Add(chroma.Punctuation, "#ffffff bold")
	s, err := b.Build()
	if err != nil {
		return base
	}
	return s
}()

// renderFiltersView redraws the [2] Filters panel as a collapsible tree of
// filter categories and their options.
func (app *Gui) renderFiltersView(g *gocui.Gui) {
	v, err := g.View(viewFilters)
	if err != nil {
		return
	}
	v.Clear()
	app.renderFiltersList(g, v)
}

// renderFiltersList draws the unified filter tree: each category has a
// collapse/expand triangle, child options are indented, and selected options
// are rendered in the category's color instead of a [ ]/[x] checkbox.
func (app *Gui) renderFiltersList(g *gocui.Gui, v *gocui.View) {
	title := "[2] Filters"
	if app.state.filters.filterQuery != "" {
		title += fmt.Sprintf("  /: %s", app.state.filters.filterQuery)
	}
	v.Title = title

	rows := app.filteredListRows()
	fs := &app.state.filters

	if fs.listCursor < 0 {
		fs.listCursor = 0
	}
	if max := len(rows) - 1; max >= 0 && fs.listCursor > max {
		fs.listCursor = max
	}

	lineNum := 0
	targetLine := 0
	for i, r := range rows {
		if i == fs.listCursor {
			targetLine = lineNum
		}

		if r.option == "" {
			triangle := triangleCollapsed
			if fs.expanded[r.category] || fs.filterQuery != "" {
				triangle = triangleExpanded
			}
			fmt.Fprintf(v, " %s %s\n", triangle, r.category.title())
		} else {
			app.renderFilterOption(v, r, fs.applied[r.category])
		}
		lineNum++
	}

	_ = v.SetOrigin(0, 0)
	_, viewH := v.Size()
	if targetLine >= viewH {
		_ = v.SetOrigin(0, targetLine-viewH+1)
		_ = v.SetCursor(0, viewH-1)
	} else {
		_ = v.SetCursor(0, targetLine)
	}
}

// renderFilterOption renders one option row. Class categories show "ID Caption"
// when enrichment data is available; other categories show the raw option label.
// Selected options use the category's color.
func (app *Gui) renderFilterOption(v *gocui.View, r listRow, applied map[string]bool) {
	entries := app.classEntriesFor(r.category)
	selected := applied[r.option]
	color := ""
	if selected {
		color = app.theme.filterColor(r.category)
	}

	if e, ok := entries[r.option]; ok && e.Caption != "" {
		idStr := fmt.Sprintf("%d", e.ID)
		caption := e.Caption
		if color != "" {
			fmt.Fprintf(v, "%s%s%s %s%s\n", indent1, color, idStr, caption, app.theme.Reset)
		} else {
			fmt.Fprintf(v, "%s%s %s\n", indent1, idStr, caption)
		}
		return
	}

	if color != "" {
		fmt.Fprintf(v, "%s%s%s%s\n", indent1, color, r.option, app.theme.Reset)
	} else {
		fmt.Fprintf(v, "%s%s\n", indent1, r.option)
	}
}

// renderRecordsView redraws the [3] Records panel and updates its title to
// reflect the current record count, stream state, and name filter.
func (app *Gui) renderRecordsView(g *gocui.Gui) {
	v, err := g.View(viewRecords)
	if err != nil {
		return
	}
	v.Clear()

	records := app.state.filteredRecords
	total := len(app.state.records)

	// Build the title: [3] Records (N)  /: foo
	title := "[3] Records"
	if total > 0 || app.state.stream == streamDone {
		if app.state.filterQuery != "" {
			title += fmt.Sprintf(" (%d/%d)", len(records), total)
		} else {
			title += fmt.Sprintf(" (%d)", total)
		}
	}
	if app.state.stream == streamErrored {
		title += " (error)"
	}
	if app.state.filterQuery != "" {
		title += fmt.Sprintf("  /: %s", app.state.filterQuery)
	}
	v.Title = title

	viewW, _ := v.Size()
	nameW := viewW - 14
	if nameW < 8 {
		nameW = 8
	}

	rows := app.state.recordDisplayRows
	if app.state.recordCursor >= len(rows) && len(rows) > 0 {
		app.state.recordCursor = len(rows) - 1
	}

	lineNum := 0
	targetLine := 0
	for i, row := range rows {
		if i == app.state.recordCursor {
			targetLine = lineNum
		}

		if row.record == nil {
			triangle := triangleCollapsed
			if app.state.recordGroupExpanded[row.groupName] {
				triangle = triangleExpanded
			}
			name := row.groupName
			if len(name) > nameW-2 {
				name = name[:nameW-3] + "…"
			}
			fmt.Fprintf(v, " %s %s\n", triangle, name)
		} else {
			name := row.record.Name
			if name == "" {
				name = row.record.CID
			}
			version := row.record.Version
			if version == "" {
				version = "n/a"
			}
			if row.grouped {
				fmt.Fprintf(v, "%s%s\n", indent1, version)
			} else {
				if len(name) > nameW {
					name = name[:nameW-1] + "…"
				}
				fmt.Fprintf(v, " %-*s  %s\n", nameW, name, version)
			}
		}
		lineNum++
	}

	_, viewH := v.Size()
	if targetLine >= viewH {
		_ = v.SetOrigin(0, targetLine-viewH+1)
		_ = v.SetCursor(0, viewH-1)
	} else {
		_ = v.SetOrigin(0, 0)
		_ = v.SetCursor(0, targetLine)
	}
}

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

// highlightTree renders a JSON tree with syntax highlighting. Lines that
// contain tree indicators (▶/▼) or OASF captions with ANSI codes are
// highlighted per-segment so the indicators and captions keep their intended
// colors instead of being painted red by the JSON lexer.
func (app *Gui) highlightTree(tree *jsonTree) string {
	raw := tree.renderLines(app.treeRenderCtx())
	var sb strings.Builder
	for _, line := range strings.Split(raw, "\n") {
		sb.WriteString(highlightTreeLine(line))
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n")
}

// highlightTreeLine highlights a single tree line. Lines that contain
// triangle indicators or pre-colored brackets (ANSI escapes) are split
// so chroma only processes the plain JSON key/value portions, while
// triangles, colored brackets, and OASF captions keep their styling.
func highlightTreeLine(line string) string {
	hasTriangle := false
	for _, tri := range []string{triangleCollapsed, triangleExpanded} {
		if strings.Contains(line, tri) {
			hasTriangle = true
			break
		}
	}
	if !hasTriangle && !strings.Contains(line, "\033[") {
		return highlightJSONInline(line)
	}
	return highlightMixedLine(line)
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
		// Check for triangle indicators (multi-byte UTF-8).
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

		// Check for ANSI escape sequence — copy everything from the
		// opening \033[ through the matching \033[0m reset as one opaque
		// span so chroma never sees the styled text.
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

// highlightJSONInline applies chroma JSON highlighting to a fragment and
// strips the trailing newline that chroma appends.
func highlightJSONInline(s string) string {
	h := highlightJSON(s)
	return strings.TrimRight(h, "\n")
}

// dimText strips all ANSI escape sequences from s and wraps the plain text
// in dimCode so the entire content renders at a single uniform brightness.
func dimText(s, dimCode string) string {
	return dimCode + stripANSI(s) + "\033[0m"
}

// stripANSI removes all ANSI CSI escape sequences (\033[…X) from a string.
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

// previewTitle formats the preview panel title, always keeping the [0] Preview
// prefix and appending the current item name when one is provided.
func previewTitle(subtitle string) string {
	if subtitle == "" {
		return "[0] Preview"
	}
	return "[0] Preview — " + subtitle
}

// wrapText splits text into lines that fit within maxWidth, breaking on word
// boundaries where possible. Newlines in the input are preserved.
func wrapText(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return strings.Split(text, "\n")
	}
	var result []string
	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph == "" {
			result = append(result, "")
			continue
		}
		for len(paragraph) > maxWidth {
			cut := maxWidth
			for cut > 0 && paragraph[cut] != ' ' {
				cut--
			}
			if cut == 0 {
				cut = maxWidth
			}
			result = append(result, paragraph[:cut])
			paragraph = strings.TrimLeft(paragraph[cut:], " ")
		}
		if paragraph != "" {
			result = append(result, paragraph)
		}
	}
	return result
}

// highlightJSON returns ANSI-colored JSON using chroma with the terminal's
// own color palette so the output blends with the user's theme.
func highlightJSON(src string) string {
	lexer := lexers.Get("json")
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	formatter := formatters.Get("terminal16")
	if formatter == nil {
		formatter = formatters.Fallback
	}

	iter, err := lexer.Tokenise(nil, src)
	if err != nil {
		return src
	}

	var buf bytes.Buffer
	if err := formatter.Format(&buf, lazydirStyle, iter); err != nil {
		return src
	}

	return buf.String()
}
