// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"strings"
	"testing"

	"github.com/agntcy/lazydir/internal/oasf"
)

func TestParseJSONTree_InvalidJSON(t *testing.T) {
	t.Parallel()
	_, err := parseJSONTree("not json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseJSONTree_EmptyObject(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree("{}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := tree.renderLines(nil)
	if !strings.Contains(out, "{}") {
		t.Errorf("expected empty object, got:\n%s", out)
	}
}

func TestParseJSONTree_RootExpandedByDefault(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree(`{"name": "test", "version": "1.0"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := tree.renderLines(nil)
	if !strings.Contains(out, triangleExpanded) {
		t.Errorf("expected root to be expanded (▼), got:\n%s", out)
	}
	if !strings.Contains(out, `"name"`) {
		t.Errorf("expected top-level fields visible, got:\n%s", out)
	}
}

func TestParseJSONTree_NestedCollapsedByDefault(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree(`{"name": "test", "nested": {"a": 1, "b": 2}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := tree.renderLines(nil)
	if !strings.Contains(out, triangleCollapsed) {
		t.Errorf("expected collapsed nested objects (▶), got:\n%s", out)
	}
	if strings.Contains(out, `"a"`) {
		t.Errorf("expected nested fields hidden when collapsed, got:\n%s", out)
	}
}

func TestParseJSONTree_RootExpandedFirstLevel(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree(`{"a": 1, "b": 2}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := tree.renderLines(nil)
	// root opening + 2 fields + closing bracket = 4
	if len(tree.lines) != 4 {
		t.Errorf("expected 4 display lines, got %d", len(tree.lines))
	}
	if !strings.Contains(out, `"a": 1`) {
		t.Errorf("expected field 'a' visible, got:\n%s", out)
	}
}

func TestToggleLine_CollapsesRoot(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree(`{"name": "test", "version": "1.0"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tree.renderLines(nil)
	toggled := tree.toggleLine(0)
	if !toggled {
		t.Fatal("expected toggle to succeed on root object")
	}

	out := tree.renderLines(nil)
	if !strings.Contains(out, triangleCollapsed) {
		t.Errorf("expected collapsed indicator (▶) after toggle, got:\n%s", out)
	}
	if len(tree.lines) != 1 {
		t.Errorf("expected 1 line when collapsed, got %d", len(tree.lines))
	}
}

func TestToggleLine_ExpandsNested(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree(`{"nested": {"a": 1}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tree.renderLines(nil)
	nestedIdx := -1
	for i, l := range tree.lines {
		if l.node.key == "nested" {
			nestedIdx = i
			break
		}
	}
	if nestedIdx < 0 {
		t.Fatal("could not find nested node line")
	}

	toggled := tree.toggleLine(nestedIdx)
	if !toggled {
		t.Fatal("expected toggle to succeed on nested object")
	}

	out := tree.renderLines(nil)
	if !strings.Contains(out, `"a": 1`) {
		t.Errorf("expected nested field visible after expand, got:\n%s", out)
	}
}

func TestToggleLine_PrimitiveNoop(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree(`{"name": "test"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tree.renderLines(nil)
	if len(tree.lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(tree.lines))
	}

	toggled := tree.toggleLine(1)
	if toggled {
		t.Error("expected toggle on primitive to be a no-op")
	}
}

func TestExpandAll(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree(`{"a": {"b": {"c": 1}}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tree.expandAll()
	out := tree.renderLines(nil)
	if strings.Contains(out, triangleCollapsed) {
		t.Errorf("expected no collapsed nodes after expandAll, got:\n%s", out)
	}
	if !strings.Contains(out, `"c": 1`) {
		t.Errorf("expected deeply nested value visible, got:\n%s", out)
	}
}

func TestCollapseAll(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree(`{"a": {"b": 1}, "c": [1, 2]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tree.expandAll()
	tree.collapseAll()
	out := tree.renderLines(nil)
	if strings.Contains(out, triangleExpanded) {
		t.Errorf("expected no expanded nodes after collapseAll, got:\n%s", out)
	}
	if len(tree.lines) != 1 {
		t.Errorf("expected 1 display line when fully collapsed, got %d", len(tree.lines))
	}
}

func TestParseJSONTree_Array(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree(`[1, "two", null, true]`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := tree.renderLines(nil)
	if !strings.Contains(out, "1") || !strings.Contains(out, `"two"`) ||
		!strings.Contains(out, "null") || !strings.Contains(out, "true") {
		t.Errorf("expected all array elements visible when root expanded, got:\n%s", out)
	}
}

func TestParseJSONTree_NestedArrays(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree(`{"items": [{"id": 1}, {"id": 2}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tree.expandAll()
	out := tree.renderLines(nil)
	if !strings.Contains(out, `"id": 1`) || !strings.Contains(out, `"id": 2`) {
		t.Errorf("expected nested array objects visible, got:\n%s", out)
	}
}

func TestToggleLine_OutOfRange(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree(`{"a": 1}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tree.renderLines(nil)

	if tree.toggleLine(-1) {
		t.Error("expected no toggle for negative index")
	}
	if tree.toggleLine(999) {
		t.Error("expected no toggle for out-of-range index")
	}
}

func TestParseJSONTree_EmptyArray(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree(`{"list": []}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := tree.renderLines(nil)
	if !strings.Contains(out, "[]") {
		t.Errorf("expected empty array literal, got:\n%s", out)
	}
}

func TestFormatPrimitive(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input interface{}
		want  string
	}{
		{nil, "null"},
		{"hello", `"hello"`},
		{float64(42), "42"},
		{float64(3.14), "3.14"},
		{true, "true"},
		{false, "false"},
	}
	for _, tt := range tests {
		got := formatPrimitive(tt.input)
		if got != tt.want {
			t.Errorf("formatPrimitive(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSingularFieldCount(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree(`{"outer": {"only": 1}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := tree.renderLines(nil)
	if !strings.Contains(out, "1 field") {
		t.Errorf("expected singular '1 field', got:\n%s", out)
	}
	if strings.Contains(out, "1 fields") {
		t.Errorf("expected singular form, not '1 fields', got:\n%s", out)
	}
}

func TestSingularItemCount(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree(`{"list": [42]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := tree.renderLines(nil)
	if !strings.Contains(out, "1 item") {
		t.Errorf("expected singular '1 item', got:\n%s", out)
	}
	if strings.Contains(out, "1 items") {
		t.Errorf("expected singular form, not '1 items', got:\n%s", out)
	}
}

func TestLineCountTracking(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree(`{"a": 1, "b": {"c": 2}, "d": [3]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Root expanded: { + "a" + "b" collapsed + "d" collapsed + } = 5
	tree.renderLines(nil)
	if len(tree.lines) != 5 {
		t.Errorf("root expanded: expected 5 lines, got %d", len(tree.lines))
	}

	tree.root.expanded = false
	tree.renderLines(nil)
	if len(tree.lines) != 1 {
		t.Errorf("collapsed: expected 1 line, got %d", len(tree.lines))
	}

	tree.expandAll()
	tree.renderLines(nil)
	// { + "a" + {b + "c" + }b + [d + 3 + ]d + } = 9
	if len(tree.lines) != 9 {
		t.Errorf("all expanded: expected 9 lines, got %d", len(tree.lines))
	}
}

func TestSkillsArrayAutoExpanded(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree(`{"skills": [{"name": "nlp"}, {"name": "translation"}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := tree.renderLines(nil)
	// The skills array should be expanded, showing collapsed child objects.
	// Each child should be a collapsed object line.
	skillsNode := tree.root.children[0]
	if skillsNode.key != "skills" {
		t.Fatalf("expected first child to be 'skills', got %q", skillsNode.key)
	}
	if !skillsNode.expanded {
		t.Error("expected skills array to be auto-expanded")
	}
	// Each child object should be collapsed.
	for i, child := range skillsNode.children {
		if child.expanded {
			t.Errorf("expected skills[%d] object to be collapsed", i)
		}
	}
	// Without OASF context, shows field count.
	if !strings.Contains(out, "1 field") {
		t.Errorf("expected collapsed skill objects to show field count, got:\n%s", out)
	}
}

func TestClassObjectCaptionResolution(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree(`{"skills": [{"name": "nlp", "id": 1}, {"name": "translation", "id": 2}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	theme := defaultTheme
	rc := &jsonRenderCtx{
		classEntries: map[oasf.ClassType]map[string]oasf.ClassEntry{
			oasf.ClassTypeSkill: {
				"nlp":         {ID: 1, Name: "nlp", Caption: "Natural Language Processing"},
				"translation": {ID: 2, Name: "translation", Caption: "Translation"},
			},
		},
		theme: &theme,
	}

	out := tree.renderLines(rc)
	if !strings.Contains(out, "Natural Language Processing") {
		t.Errorf("expected OASF caption for collapsed skill object, got:\n%s", out)
	}
	if !strings.Contains(out, "Translation") {
		t.Errorf("expected OASF caption for collapsed skill object, got:\n%s", out)
	}
}

func TestClassObjectCaptionDisappearsOnExpand(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree(`{"skills": [{"name": "nlp", "id": 1}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	theme := defaultTheme
	rc := &jsonRenderCtx{
		classEntries: map[oasf.ClassType]map[string]oasf.ClassEntry{
			oasf.ClassTypeSkill: {
				"nlp": {ID: 1, Name: "nlp", Caption: "Natural Language Processing"},
			},
		},
		theme: &theme,
	}

	// Before expand: caption visible.
	out := tree.renderLines(rc)
	if !strings.Contains(out, "Natural Language Processing") {
		t.Fatalf("expected caption before expand, got:\n%s", out)
	}

	// Expand the skill object.
	skillObj := tree.root.children[0].children[0]
	skillObj.expanded = true
	out = tree.renderLines(rc)
	if strings.Contains(out, "Natural Language Processing") {
		t.Errorf("expected caption to disappear after expand, got:\n%s", out)
	}
	if !strings.Contains(out, `"name"`) {
		t.Errorf("expected actual fields visible after expand, got:\n%s", out)
	}
}

func TestClassCaptionFallbackToName(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree(`{"domains": [{"name": "security"}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	theme := defaultTheme
	rc := &jsonRenderCtx{
		classEntries: map[oasf.ClassType]map[string]oasf.ClassEntry{
			oasf.ClassTypeDomain: {
				"security": {ID: 1, Name: "security", Caption: ""},
			},
		},
		theme: &theme,
	}

	out := tree.renderLines(rc)
	if !strings.Contains(out, "security") {
		t.Errorf("expected fallback to name when caption is empty, got:\n%s", out)
	}
}

func TestClassCaptionNoContext(t *testing.T) {
	t.Parallel()
	tree, err := parseJSONTree(`{"skills": [{"name": "nlp"}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tree.renderLines(nil)
	if !strings.Contains(out, "1 field") {
		t.Errorf("expected fallback to field count without render context, got:\n%s", out)
	}
}

func TestDomainsModulesAutoExpanded(t *testing.T) {
	t.Parallel()
	src := `{"domains": [{"name": "sec"}], "modules": [{"name": "auth"}], "other": [1, 2]}`
	tree, err := parseJSONTree(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, child := range tree.root.children {
		switch child.key {
		case "domains", "modules":
			if !child.expanded {
				t.Errorf("expected %q array to be auto-expanded", child.key)
			}
		case "other":
			if child.expanded {
				t.Errorf("expected %q array to remain collapsed", child.key)
			}
		}
	}
}
