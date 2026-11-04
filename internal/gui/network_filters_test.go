// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/agntcy/lazydir/internal/dirclient"
	"github.com/agntcy/lazydir/internal/oasf"
)

const (
	nodeCatA  = "cat_a"
	nodeChild = "cat_a/child"
	nodeOther = "cat_a/other"
	hdrSkills = "#Skills"
)

// newNetFilterApp returns a Gui in network mode whose default OASF version
// carries the given skill tree.
func newNetFilterApp(skills []oasf.TreeNode) *Gui {
	app := &Gui{}
	app.state.browse = browseNetwork
	app.state.net.filters = newNetFilterState()
	app.state.netVersions = []string{ver100}
	app.state.netDefaultVersion = ver100
	app.state.netTaxonomy = map[string]map[oasf.ClassType][]oasf.TreeNode{
		ver100: {oasf.ClassTypeSkill: skills},
	}
	return app
}

func skillTree() []oasf.TreeNode {
	return []oasf.TreeNode{{
		ID: 1, Name: nodeCatA, Caption: "Cat A", Category: true,
		Children: []oasf.TreeNode{{ID: 11, Name: nodeChild, Caption: "Child"}},
	}}
}

// rowLabel identifies a netRow in test failure messages and comparisons.
func rowLabel(r netRow) string {
	switch {
	case r.node != nil:
		return r.node.Name
	case r.value != "":
		return r.value
	default:
		return "#" + r.category.title()
	}
}

func TestMergeTrees(t *testing.T) {
	t.Parallel()

	a := []oasf.TreeNode{{ID: 1, Name: nodeCatA, Caption: "A", Children: []oasf.TreeNode{
		{ID: 12, Name: nodeOther},
	}}}
	b := []oasf.TreeNode{
		{ID: 2, Name: "cat_b"},
		{ID: 1, Name: nodeCatA, Caption: "A later", Children: []oasf.TreeNode{
			{ID: 11, Name: nodeChild},
		}},
	}

	got := mergeTrees(a, b)

	if len(got) != 2 || got[0].Name != nodeCatA || got[1].Name != "cat_b" {
		t.Fatalf("top level = %+v, want cat_a then cat_b", got)
	}
	if got[0].Caption != "A" {
		t.Errorf("caption = %q, want first-seen %q", got[0].Caption, "A")
	}
	kids := got[0].Children
	if len(kids) != 2 || kids[0].Name != nodeChild || kids[1].Name != nodeOther {
		t.Errorf("children = %+v, want child (11) then other (12)", kids)
	}
	if len(a[0].Children) != 1 {
		t.Errorf("input tree mutated: %+v", a[0].Children)
	}
}

func TestNetFilterRows_TreeAndDimmed(t *testing.T) {
	t.Parallel()

	app := newNetFilterApp(skillTree())

	rows := app.netFilterRows()
	var labels []string
	for _, r := range rows {
		labels = append(labels, rowLabel(r))
	}
	want := []string{hdrSkills, "#Domains", "#Modules", "#OASF version", "#Author", "#Trusted / Verified / Safe"}
	if !reflect.DeepEqual(labels, want) {
		t.Fatalf("rows = %v, want %v", labels, want)
	}
	for _, r := range rows {
		wantDim := r.category == filterAuthor || r.category == filterTrustSafety
		if r.dimmed != wantDim {
			t.Errorf("%s dimmed = %v, want %v", rowLabel(r), r.dimmed, wantDim)
		}
	}

	fs := &app.state.net.filters
	fs.expanded[netExpandKey(filterSkills, "")] = true
	fs.expanded[netExpandKey(filterSkills, nodeCatA)] = true

	rows = app.netFilterRows()
	if rowLabel(rows[1]) != nodeCatA || rows[1].depth != 1 {
		t.Errorf("rows[1] = %s depth %d, want cat_a depth 1", rowLabel(rows[1]), rows[1].depth)
	}
	if rowLabel(rows[2]) != nodeChild || rows[2].depth != 2 {
		t.Errorf("rows[2] = %s depth %d, want cat_a/child depth 2", rowLabel(rows[2]), rows[2].depth)
	}
	if rowLabel(rows[3]) != "#Domains" {
		t.Errorf("rows[3] = %s, want Domains header", rowLabel(rows[3]))
	}
}

func TestNetFilterRows_VersionsWhenExpanded(t *testing.T) {
	t.Parallel()

	app := newNetFilterApp(skillTree())
	app.state.netVersions = []string{ver070, ver100}
	app.state.net.filters.expanded[netExpandKey(filterOASFVersion, "")] = true

	var versions []string
	for _, r := range app.netFilterRows() {
		if r.category == filterOASFVersion && r.value != "" {
			versions = append(versions, r.value)
		}
	}
	if !reflect.DeepEqual(versions, []string{ver070, ver100}) {
		t.Errorf("version rows = %v, want [%s %s]", versions, ver070, ver100)
	}
}

func TestNetFilterRows_Query(t *testing.T) {
	t.Parallel()

	app := newNetFilterApp(skillTree())
	app.state.net.filters.query = "child"

	var labels []string
	for _, r := range app.netFilterRows() {
		labels = append(labels, rowLabel(r))
		if r.node != nil && r.depth != 1 {
			t.Errorf("%s depth = %d, want flat (1)", rowLabel(r), r.depth)
		}
	}
	if want := []string{hdrSkills, nodeChild}; !reflect.DeepEqual(labels, want) {
		t.Errorf("rows = %v, want %v", labels, want)
	}
}

func TestNetworkOptions_KeepsAppliedOutsideTaxonomy(t *testing.T) {
	t.Parallel()

	app := newNetFilterApp(skillTree())
	app.state.net.filters.applied[filterSkills] = map[string]bool{"gone/x": true}

	rows := app.netFilterRows()
	for i, r := range rows {
		if r.value == "gone/x" {
			if r.category != filterSkills || rowLabel(rows[i-1]) != hdrSkills {
				t.Errorf("gone/x not under Skills: %+v", rows)
			}
			return
		}
	}
	t.Errorf("no row for applied value gone/x: %+v", rows)
}

func TestNetToggle_NoExclude(t *testing.T) {
	t.Parallel()

	app := newNetFilterApp(skillTree())
	tree := app.netTree(filterSkills)
	row := netRow{category: filterSkills, node: &tree[0], depth: 1}

	app.netToggle(row)
	if !app.state.net.filters.applied[filterSkills][nodeCatA] {
		t.Fatalf("after first toggle: %v, want applied", app.state.net.filters.applied)
	}
	app.netToggle(row)
	if _, ok := app.state.net.filters.applied[filterSkills][nodeCatA]; ok {
		t.Errorf("after second toggle: %v, want absent", app.state.net.filters.applied)
	}
}

func TestNetToggle_VersionAndDimmed(t *testing.T) {
	t.Parallel()

	app := newNetFilterApp(skillTree())

	app.netToggle(netRow{category: filterOASFVersion, value: ver070, depth: 1})
	if !app.state.net.filters.versions[ver070] {
		t.Errorf("versions = %v, want %s selected", app.state.net.filters.versions, ver070)
	}

	app.netToggle(netRow{category: filterAuthor, dimmed: true})
	if len(app.state.net.filters.applied) != 0 {
		t.Errorf("dimmed toggle changed applied: %v", app.state.net.filters.applied)
	}
}

func TestNetworkQueries_Sorted(t *testing.T) {
	t.Parallel()

	app := newNetFilterApp(nil)
	app.state.net.filters.applied[filterDomains] = map[string]bool{"d": true}
	app.state.net.filters.applied[filterSkills] = map[string]bool{"b": true, "a": true}

	got := app.networkQueries()
	want := []dirclient.NetworkQuery{
		{Category: dirclient.FilterSkill, Value: "a"},
		{Category: dirclient.FilterSkill, Value: "b"},
		{Category: dirclient.FilterDomain, Value: "d"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("networkQueries() = %v, want %v", got, want)
	}
}

func TestActiveNetVersions_DefaultWhenNone(t *testing.T) {
	t.Parallel()

	app := newNetFilterApp(nil)
	if got := app.activeNetVersions(); !reflect.DeepEqual(got, []string{ver100}) {
		t.Errorf("no selection: %v, want [%s]", got, ver100)
	}

	app.state.net.filters.versions = map[string]bool{ver100: true, ver070: true}
	if got := app.activeNetVersions(); !reflect.DeepEqual(got, []string{ver070, ver100}) {
		t.Errorf("selection: %v, want sorted [%s %s]", got, ver070, ver100)
	}
}

func TestWriteNetRow_Dimmed(t *testing.T) {
	t.Parallel()

	app := newNetFilterApp(nil)
	var buf bytes.Buffer
	app.writeNetRow(&buf, netRow{category: filterAuthor, dimmed: true})
	if !strings.Contains(buf.String(), "Author   not in network mode") {
		t.Errorf("dimmed row = %q, want it to contain %q", buf.String(), "Author   not in network mode")
	}
}
