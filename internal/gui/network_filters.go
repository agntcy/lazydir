// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/agntcy/lazydir/internal/dirclient"
	"github.com/agntcy/lazydir/internal/oasf"
	"github.com/jesseduffield/gocui"
)

// netFilterState owns the network-mode Filters panel. Values are OASF taxonomy
// names (full slash paths), sent verbatim as routing queries. There is no
// exclude in network mode, so a value is either applied or absent.
type netFilterState struct {
	cursor int
	// expanded is keyed by netExpandKey: category headers and taxonomy nodes.
	expanded map[string]bool
	// applied[category][name]; only Skills, Domains and Modules.
	applied map[filterCategory]map[string]bool
	// versions are the OASF versions whose taxonomies are offered; none
	// selected means the server's default version.
	versions map[string]bool
	query    string
	// minScore is the routing search's minimum match score, clamped to the
	// number of queries when used.
	minScore uint32
}

func newNetFilterState() netFilterState {
	return netFilterState{
		expanded: map[string]bool{},
		applied:  map[filterCategory]map[string]bool{},
		versions: map[string]bool{},
		minScore: 1,
	}
}

// netClassCategories are the categories whose values come from the taxonomy.
var netClassCategories = []filterCategory{filterSkills, filterDomains, filterModules}

// netExpandKey keys the expanded map. A category header uses an empty name;
// prefixing the category keeps equal names in different taxonomies apart.
func netExpandKey(c filterCategory, name string) string {
	return fmt.Sprintf("%d:%s", c, name)
}

func classTypeFor(c filterCategory) oasf.ClassType {
	switch c {
	case filterDomains:
		return oasf.ClassTypeDomain
	case filterModules:
		return oasf.ClassTypeModule
	}
	return oasf.ClassTypeSkill
}

// mergeTrees unions taxonomy trees by Name, recursively. The first tree to
// supply a name keeps its ID and caption; children are merged the same way and
// every level is ordered by ID. The inputs are not modified.
func mergeTrees(trees ...[]oasf.TreeNode) []oasf.TreeNode {
	var out []oasf.TreeNode
	idx := map[string]int{}
	for _, t := range trees {
		for _, n := range t {
			if i, ok := idx[n.Name]; ok {
				out[i].Children = mergeTrees(out[i].Children, n.Children)
				continue
			}
			idx[n.Name] = len(out)
			n.Children = mergeTrees(n.Children)
			out = append(out, n)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// activeNetVersions returns the selected OASF versions sorted, or the server's
// default version when none is selected.
func (app *Gui) activeNetVersions() []string {
	sel := app.state.net.filters.versions
	if len(sel) == 0 {
		return []string{app.state.netDefaultVersion}
	}
	out := make([]string, 0, len(sel))
	for v := range sel {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// netTree returns the taxonomy for a class category, merged across the active
// OASF versions that have been fetched.
func (app *Gui) netTree(c filterCategory) []oasf.TreeNode {
	ct := classTypeFor(c)
	var trees [][]oasf.TreeNode
	for _, v := range app.activeNetVersions() {
		trees = append(trees, app.state.netTaxonomy[v][ct])
	}
	return mergeTrees(trees...)
}

// netTaxonomyLoaded reports whether any active version has a cached taxonomy.
func (app *Gui) netTaxonomyLoaded() bool {
	for _, v := range app.activeNetVersions() {
		if _, ok := app.state.netTaxonomy[v]; ok {
			return true
		}
	}
	return false
}

// findNode returns the node with the given name anywhere in tree, or nil.
func findNode(tree []oasf.TreeNode, name string) *oasf.TreeNode {
	for i := range tree {
		if tree[i].Name == name {
			return &tree[i]
		}
		if n := findNode(tree[i].Children, name); n != nil {
			return n
		}
	}
	return nil
}

// netRow is one line of the network-mode Filters panel. A header has neither
// node nor value. value holds version rows and applied values missing from the
// taxonomy.
type netRow struct {
	category filterCategory
	node     *oasf.TreeNode
	value    string
	depth    int
	dimmed   bool
}

func (r netRow) isHeader() bool { return r.node == nil && r.value == "" }

// name is the query value of a class row, or the version of a version row.
func (r netRow) name() string {
	if r.node != nil {
		return r.node.Name
	}
	return r.value
}

// netFilterRows builds the visible rows: Skills, Domains and Modules as
// taxonomy trees, OASF version, then Author and Trusted / Verified / Safe as
// dimmed headers. A collapsed category shows only its applied values; applied
// values missing from the taxonomy are listed flat under their category. With a
// search query, every matching value is listed flat instead.
func (app *Gui) netFilterRows() []netRow {
	fs := &app.state.net.filters
	if fs.query != "" {
		return app.netQueryRows(strings.ToLower(fs.query))
	}
	var rows []netRow
	for _, c := range netClassCategories {
		rows = append(rows, netRow{category: c})
		tree := app.netTree(c)
		applied := sortedKeys(fs.applied[c])
		if fs.expanded[netExpandKey(c, "")] {
			rows = app.appendNetNodes(rows, c, tree, 1)
			for _, name := range applied {
				if findNode(tree, name) == nil {
					rows = append(rows, netRow{category: c, value: name, depth: 1})
				}
			}
			continue
		}
		for _, name := range applied {
			if n := findNode(tree, name); n != nil {
				rows = append(rows, netRow{category: c, node: n, depth: 1})
			} else {
				rows = append(rows, netRow{category: c, value: name, depth: 1})
			}
		}
	}

	rows = append(rows, netRow{category: filterOASFVersion})
	versions := sortedKeys(fs.versions)
	if fs.expanded[netExpandKey(filterOASFVersion, "")] {
		versions = app.netVersionOptions()
	}
	for _, v := range versions {
		rows = append(rows, netRow{category: filterOASFVersion, value: v, depth: 1})
	}

	rows = append(rows,
		netRow{category: filterAuthor, dimmed: true},
		netRow{category: filterTrustSafety, dimmed: true},
	)
	return rows
}

// appendNetNodes appends tree's nodes at depth, descending into the expanded
// ones.
func (app *Gui) appendNetNodes(rows []netRow, c filterCategory, tree []oasf.TreeNode, depth int) []netRow {
	for i := range tree {
		n := &tree[i]
		rows = append(rows, netRow{category: c, node: n, depth: depth})
		if app.state.net.filters.expanded[netExpandKey(c, n.Name)] {
			rows = app.appendNetNodes(rows, c, n.Children, depth+1)
		}
	}
	return rows
}

// netVersionOptions lists the server's OASF versions plus any selected version
// it no longer offers, so the selection can be cleared.
func (app *Gui) netVersionOptions() []string {
	out := append([]string(nil), app.state.netVersions...)
	for _, v := range sortedKeys(app.state.net.filters.versions) {
		found := false
		for _, o := range out {
			if o == v {
				found = true
				break
			}
		}
		if !found {
			out = append(out, v)
		}
	}
	return out
}

// netQueryRows lists every value matching q (already lower-cased) by name,
// caption or ID, flat and grouped under its category header. Dimmed categories
// are left out.
func (app *Gui) netQueryRows(q string) []netRow {
	var rows []netRow
	for _, c := range netClassCategories {
		tree := app.netTree(c)
		var matches []netRow
		var walk func(nodes []oasf.TreeNode)
		walk = func(nodes []oasf.TreeNode) {
			for i := range nodes {
				n := &nodes[i]
				if nodeMatchesQuery(n, q) {
					matches = append(matches, netRow{category: c, node: n, depth: 1})
				}
				walk(n.Children)
			}
		}
		walk(tree)
		for _, name := range sortedKeys(app.state.net.filters.applied[c]) {
			if findNode(tree, name) == nil && strings.Contains(strings.ToLower(name), q) {
				matches = append(matches, netRow{category: c, value: name, depth: 1})
			}
		}
		if len(matches) > 0 {
			rows = append(rows, netRow{category: c})
			rows = append(rows, matches...)
		}
	}

	var versions []netRow
	for _, v := range app.netVersionOptions() {
		if strings.Contains(strings.ToLower(v), q) {
			versions = append(versions, netRow{category: filterOASFVersion, value: v, depth: 1})
		}
	}
	if len(versions) > 0 {
		rows = append(rows, netRow{category: filterOASFVersion})
		rows = append(rows, versions...)
	}
	return rows
}

func nodeMatchesQuery(n *oasf.TreeNode, q string) bool {
	return strings.Contains(strings.ToLower(n.Name), q) ||
		strings.Contains(strings.ToLower(n.Caption), q) ||
		strings.Contains(fmt.Sprintf("%d", n.ID), q)
}

// sortedKeys returns the keys of set in ascending order.
func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// netToggle turns the value under r on or off. A version row changes which
// taxonomies are offered; a class value changes the routing queries. Headers
// and dimmed rows are ignored.
func (app *Gui) netToggle(r netRow) {
	if r.dimmed || r.isHeader() {
		return
	}
	fs := &app.state.net.filters
	name := r.name()
	if r.category == filterOASFVersion {
		if fs.versions[name] {
			delete(fs.versions, name)
		} else {
			fs.versions[name] = true
		}
		app.startNetworkTaxonomyFetch()
		return
	}
	set := fs.applied[r.category]
	if set[name] {
		delete(set, name)
		if len(set) == 0 {
			delete(fs.applied, r.category)
		}
	} else {
		if set == nil {
			set = map[string]bool{}
			fs.applied[r.category] = set
		}
		set[name] = true
	}
	app.startNetworkSearch()
}

// networkQueries returns one routing query per applied value, sorted by
// category then value.
func (app *Gui) networkQueries() []dirclient.NetworkQuery {
	var qs []dirclient.NetworkQuery
	for _, c := range netClassCategories {
		for _, v := range sortedKeys(app.state.net.filters.applied[c]) {
			qs = append(qs, dirclient.NetworkQuery{Category: categoryToFilter(c), Value: v})
		}
	}
	return qs
}

// ── Rendering ────────────────────────────────────────────────────────────────

// renderNetFiltersList draws the network-mode Filters panel.
func (app *Gui) renderNetFiltersList(v *gocui.View) {
	fs := &app.state.net.filters
	title := titleFilters + filterValuesNotice(app.netTaxonomyLoaded(), app.state.netTaxonomyFailed)
	if fs.query != "" {
		title += fmt.Sprintf("  /: %s", fs.query)
	}
	v.Title = title

	rows := app.netFilterRows()
	if fs.cursor < 0 {
		fs.cursor = 0
	}
	if max := len(rows) - 1; max >= 0 && fs.cursor > max {
		fs.cursor = max
	}
	for _, r := range rows {
		app.writeNetRow(v, r)
	}

	_ = v.SetOrigin(0, 0)
	_, viewH := v.Size()
	if fs.cursor >= viewH {
		_ = v.SetOrigin(0, fs.cursor-viewH+1)
		_ = v.SetCursor(0, viewH-1)
	} else {
		_ = v.SetCursor(0, fs.cursor)
	}
}

// writeNetRow renders one network Filters row. Taxonomy nodes show "ID
// Caption" with a triangle when they have children; applied values and
// selected versions use the category color.
func (app *Gui) writeNetRow(w io.Writer, r netRow) {
	fs := &app.state.net.filters
	t := app.theme
	if r.isHeader() {
		if r.dimmed {
			fmt.Fprintf(w, "   %s%s   not in network mode%s\n", t.Color10, r.category.title(), t.Reset)
			return
		}
		triangle := triangleCollapsed
		if fs.expanded[netExpandKey(r.category, "")] || fs.query != "" {
			triangle = triangleExpanded
		}
		fmt.Fprintf(w, " %s %s\n", triangle, r.category.title())
		return
	}

	indent := strings.Repeat(indent1, r.depth)
	var label string
	var selected bool
	switch {
	case r.category == filterOASFVersion:
		label = r.value
		if r.value == app.state.netDefaultVersion {
			label += " (default)"
		}
		selected = fs.versions[r.value]
	case r.node != nil:
		caption := r.node.Caption
		if caption == "" {
			caption = r.node.Name
		}
		label = fmt.Sprintf("%d %s", r.node.ID, caption)
		if len(r.node.Children) > 0 && fs.query == "" {
			marker := triangleCollapsed
			if fs.expanded[netExpandKey(r.category, r.node.Name)] {
				marker = triangleExpanded
			}
			label = marker + " " + label
		}
		selected = fs.applied[r.category][r.node.Name]
	default:
		label = r.value
		selected = fs.applied[r.category][r.value]
	}
	if selected {
		fmt.Fprintf(w, "%s%s%s%s\n", indent, t.filterColor(r.category), label, t.Reset)
		return
	}
	fmt.Fprintf(w, "%s%s\n", indent, label)
}

// ── Handlers ─────────────────────────────────────────────────────────────────

// netFilterCurrent returns the rows and the row under the cursor, ok false when
// the cursor is past the end.
func (app *Gui) netFilterCurrent() ([]netRow, netRow, bool) {
	rows := app.netFilterRows()
	c := app.state.net.filters.cursor
	if c < 0 || c >= len(rows) {
		return rows, netRow{}, false
	}
	return rows, rows[c], true
}

// netExpandable reports whether r can be expanded: an active category header
// or a taxonomy node with children. Search results are flat.
func (app *Gui) netExpandable(r netRow) bool {
	if r.dimmed || app.state.net.filters.query != "" {
		return false
	}
	return r.isHeader() || (r.node != nil && len(r.node.Children) > 0)
}

func (app *Gui) netExpandKeyFor(r netRow) string {
	if r.isHeader() {
		return netExpandKey(r.category, "")
	}
	return netExpandKey(r.category, r.node.Name)
}

func (app *Gui) netFilterCursorUp(g *gocui.Gui) {
	if app.state.net.filters.cursor > 0 {
		app.state.net.filters.cursor--
	}
	app.renderFiltersView(g)
}

func (app *Gui) netFilterCursorDown(g *gocui.Gui) {
	if app.state.net.filters.cursor < len(app.netFilterRows())-1 {
		app.state.net.filters.cursor++
	}
	app.renderFiltersView(g)
}

// netFilterActivate is enter and mouse click on the row under the cursor.
func (app *Gui) netFilterActivate(g *gocui.Gui) {
	_, r, ok := app.netFilterCurrent()
	if !ok {
		return
	}
	app.netActivate(r)
	app.renderFiltersView(g)
}

// netActivate toggles expansion of a header or a taxonomy node with children,
// and toggles selection of any other value. Space still selects a parent node.
func (app *Gui) netActivate(r netRow) {
	switch {
	case app.netExpandable(r):
		key := app.netExpandKeyFor(r)
		app.state.net.filters.expanded[key] = !app.state.net.filters.expanded[key]
	case !r.isHeader():
		app.netToggle(r)
	}
}

func (app *Gui) netFilterToggleOption(g *gocui.Gui) {
	_, r, ok := app.netFilterCurrent()
	if !ok || r.isHeader() {
		return
	}
	app.netToggle(r)
	app.renderFiltersView(g)
}

func (app *Gui) netFilterExpand(g *gocui.Gui) {
	_, r, ok := app.netFilterCurrent()
	if !ok || !app.netExpandable(r) {
		return
	}
	app.state.net.filters.expanded[app.netExpandKeyFor(r)] = true
	app.renderFiltersView(g)
}

// netFilterCollapse collapses the expanded row under the cursor, or else moves
// to its parent row and collapses that.
func (app *Gui) netFilterCollapse(g *gocui.Gui) {
	rows, r, ok := app.netFilterCurrent()
	if !ok {
		return
	}
	fs := &app.state.net.filters
	collapsible := app.netExpandable(r) && fs.expanded[app.netExpandKeyFor(r)]
	if !collapsible && r.depth > 0 {
		for i := fs.cursor - 1; i >= 0; i-- {
			if rows[i].depth < r.depth {
				fs.cursor = i
				r = rows[i]
				break
			}
		}
	}
	if app.netExpandable(r) {
		delete(fs.expanded, app.netExpandKeyFor(r))
	}
	app.renderFiltersView(g)
}

func (app *Gui) netFilterEsc(g *gocui.Gui) {
	if app.state.net.filters.query != "" {
		app.state.net.filters.query = ""
		app.state.net.filters.cursor = 0
		app.renderFiltersView(g)
		return
	}
	app.netFilterCollapse(g)
}

func (app *Gui) netFilterOpenSearch() {
	prevQuery := app.state.net.filters.query
	set := func(value string) {
		app.state.net.filters.query = value
		app.state.net.filters.cursor = 0
		app.renderFiltersView(app.g)
	}
	app.openInput("Search filters (/)", prevQuery,
		func(value string) {
			app.g.Update(func(*gocui.Gui) error { set(value); return nil })
		},
		func() {
			app.g.Update(func(*gocui.Gui) error { set(prevQuery); return nil })
		},
		set,
	)
}

// netFilterClearAll clears the applied values and the selected versions.
func (app *Gui) netFilterClearAll(g *gocui.Gui) {
	fs := &app.state.net.filters
	if len(fs.applied) == 0 && len(fs.versions) == 0 {
		return
	}
	hadApplied := len(fs.applied) > 0
	fs.applied = map[filterCategory]map[string]bool{}
	fs.versions = map[string]bool{}
	app.startNetworkTaxonomyFetch()
	if hadApplied {
		app.startNetworkSearch()
	}
	app.renderFiltersView(g)
}
