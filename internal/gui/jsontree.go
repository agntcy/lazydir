// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/agntcy/lazydir/internal/oasf"
)

const (
	triangleCollapsed = "▶"
	triangleExpanded  = "▼"
)

// jsonNode represents one node in a collapsible JSON tree.
type jsonNode struct {
	key      string      // field name (empty for array elements and root)
	value    interface{} // raw parsed value (used for primitives)
	children []*jsonNode // ordered child nodes for objects/arrays
	nodeType jsonType
	expanded bool
}

type jsonType int

const (
	jsonObject jsonType = iota
	jsonArray
	jsonPrimitive
)

// jsonTree holds the root of a collapsible JSON tree and the per-line node
// index used for expand/collapse interaction.
type jsonTree struct {
	root  *jsonNode
	lines []jsonTreeLine // computed by renderLines, indexed by display row
}

// jsonTreeLine maps a visible display row back to the node it represents,
// so key handlers can toggle expand/collapse on the correct node.
type jsonTreeLine struct {
	node  *jsonNode
	depth int
}

// jsonRenderCtx provides OASF enrichment data and theme colors so the tree
// renderer can annotate collapsed skills/domains/modules with their caption
// and apply depth-based bracket coloring.
type jsonRenderCtx struct {
	classEntries map[oasf.ClassType]map[string]oasf.ClassEntry
	theme        *Theme
}

// bracketPalette is a fixed set of ANSI colors cycled by nesting depth so
// that matching open/close brackets share the same color.
var bracketPalette = []string{
	"\033[1;33m", // bold yellow
	"\033[1;35m", // bold magenta
	"\033[1;36m", // bold cyan
	"\033[1;32m", // bold green
	"\033[1;34m", // bold blue
	"\033[1;91m", // bold bright red
}

func bracketColor(depth int) string {
	return bracketPalette[depth%len(bracketPalette)]
}

const jsonIndent = "  "

// classFieldTypes maps JSON field names from the record schema to their OASF
// class types and the child field that holds the class name.
var classFieldTypes = map[string]oasf.ClassType{
	"skills":  oasf.ClassTypeSkill,
	"domains": oasf.ClassTypeDomain,
	"modules": oasf.ClassTypeModule,
}

// parseJSONTree parses a JSON string into a collapsible tree structure.
// The root object is expanded by default, and skills/domains/modules arrays
// are also auto-expanded so their items are visible at first glance.
func parseJSONTree(src string) (*jsonTree, error) {
	var raw interface{}
	if err := json.Unmarshal([]byte(src), &raw); err != nil {
		return nil, err
	}
	root := buildNode("", raw)
	if root.nodeType != jsonPrimitive {
		root.expanded = true
	}
	autoExpandClassArrays(root)
	return &jsonTree{root: root}, nil
}

// autoExpandClassArrays expands skills/domains/modules arrays that are
// direct children of the root so their items are visible by default.
func autoExpandClassArrays(root *jsonNode) {
	if root.nodeType != jsonObject {
		return
	}
	for _, child := range root.children {
		if _, ok := classFieldTypes[child.key]; ok && child.nodeType == jsonArray {
			child.expanded = true
		}
	}
}

func buildNode(key string, val interface{}) *jsonNode {
	switch v := val.(type) {
	case map[string]interface{}:
		node := &jsonNode{key: key, nodeType: jsonObject}
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			node.children = append(node.children, buildNode(k, v[k]))
		}
		return node
	case []interface{}:
		node := &jsonNode{key: key, nodeType: jsonArray}
		for _, item := range v {
			node.children = append(node.children, buildNode("", item))
		}
		return node
	default:
		return &jsonNode{key: key, value: val, nodeType: jsonPrimitive}
	}
}

// renderLines produces the visible display lines for the tree in its current
// expand/collapse state, and populates t.lines for cursor-based interaction.
// The optional render context enables OASF caption enrichment.
func (t *jsonTree) renderLines(rc *jsonRenderCtx) string {
	if t.root == nil {
		return "{}"
	}
	var sb strings.Builder
	t.lines = nil
	t.renderNode(&sb, t.root, 0, true, rc)
	return strings.TrimRight(sb.String(), "\n")
}

func (t *jsonTree) renderNode(sb *strings.Builder, node *jsonNode, depth int, last bool, rc *jsonRenderCtx) {
	t.renderNodeInCtx(sb, node, depth, last, rc, "")
}

func (t *jsonTree) renderNodeInCtx(sb *strings.Builder, node *jsonNode, depth int, last bool, rc *jsonRenderCtx, parentKey string) {
	indent := strings.Repeat(jsonIndent, depth)
	suffix := ","
	if last {
		suffix = ""
	}
	bc := bracketColor(depth)
	rst := "\033[0m"

	switch node.nodeType {
	case jsonPrimitive:
		t.emitLine(sb, node, depth)
		valStr := formatPrimitive(node.value)
		if node.key != "" {
			fmt.Fprintf(sb, "%s\"%s\": %s%s\n", indent, node.key, valStr, suffix)
		} else {
			fmt.Fprintf(sb, "%s%s%s\n", indent, valStr, suffix)
		}

	case jsonObject:
		t.emitLine(sb, node, depth)
		if len(node.children) == 0 {
			if node.key != "" {
				fmt.Fprintf(sb, "%s\"%s\": %s{}%s%s\n", indent, node.key, bc, rst, suffix)
			} else {
				fmt.Fprintf(sb, "%s%s{}%s%s\n", indent, bc, rst, suffix)
			}
			return
		}

		triangle := triangleCollapsed
		if node.expanded {
			triangle = triangleExpanded
		}

		summary := collapsedObjectSuffix(node, suffix, parentKey, bc, rst, rc)
		if node.key != "" {
			fmt.Fprintf(sb, "%s%s \"%s\": %s{%s%s\n", indent, triangle, node.key, bc, rst, summary)
		} else {
			fmt.Fprintf(sb, "%s%s %s{%s%s\n", indent, triangle, bc, rst, summary)
		}
		if node.expanded {
			for i, child := range node.children {
				t.renderNodeInCtx(sb, child, depth+1, i == len(node.children)-1, rc, node.key)
			}
			t.emitLine(sb, nil, depth)
			fmt.Fprintf(sb, "%s%s}%s%s\n", indent, bc, rst, suffix)
		}

	case jsonArray:
		t.emitLine(sb, node, depth)
		if len(node.children) == 0 {
			if node.key != "" {
				fmt.Fprintf(sb, "%s\"%s\": %s[]%s%s\n", indent, node.key, bc, rst, suffix)
			} else {
				fmt.Fprintf(sb, "%s%s[]%s%s\n", indent, bc, rst, suffix)
			}
			return
		}

		triangle := triangleCollapsed
		if node.expanded {
			triangle = triangleExpanded
		}

		summary := collapsedArraySuffix(node, suffix, bc, rst)
		if node.key != "" {
			fmt.Fprintf(sb, "%s%s \"%s\": %s[%s%s\n", indent, triangle, node.key, bc, rst, summary)
		} else {
			fmt.Fprintf(sb, "%s%s %s[%s%s\n", indent, triangle, bc, rst, summary)
		}
		if node.expanded {
			for i, child := range node.children {
				t.renderNodeInCtx(sb, child, depth+1, i == len(node.children)-1, rc, node.key)
			}
			t.emitLine(sb, nil, depth)
			fmt.Fprintf(sb, "%s%s]%s%s\n", indent, bc, rst, suffix)
		}
	}
}

// emitLine records a line→node mapping. Every output line gets exactly one
// entry so that tree.lines[visualRow] is always correct. Closing bracket
// lines pass nil — toggle is a no-op on them.
func (t *jsonTree) emitLine(_ *strings.Builder, node *jsonNode, depth int) {
	t.lines = append(t.lines, jsonTreeLine{node: node, depth: depth})
}

// collapsedObjectSuffix returns a summary for collapsed objects. When the
// object lives inside a class array (skills/domains/modules) and OASF data
// is available, it shows the colored caption instead of "N fields".
// bc/rst are the bracket color and reset codes so the closing } matches.
func collapsedObjectSuffix(node *jsonNode, trailing, parentKey, bc, rst string, rc *jsonRenderCtx) string {
	if node.expanded {
		return ""
	}
	n := len(node.children)

	if ct, ok := classFieldTypes[parentKey]; ok && rc != nil && rc.classEntries != nil {
		if caption := resolveObjectCaption(node, ct, rc); caption != "" {
			return fmt.Sprintf(" %s %s}%s%s", caption, bc, rst, trailing)
		}
	}

	if n == 1 {
		return fmt.Sprintf(" 1 field %s}%s%s", bc, rst, trailing)
	}
	return fmt.Sprintf(" %d fields %s}%s%s", n, bc, rst, trailing)
}

// collapsedArraySuffix returns a summary for collapsed arrays.
// bc/rst are the bracket color and reset codes so the closing ] matches.
func collapsedArraySuffix(node *jsonNode, trailing, bc, rst string) string {
	if node.expanded {
		return ""
	}
	n := len(node.children)
	if n == 1 {
		return fmt.Sprintf(" 1 item %s]%s%s", bc, rst, trailing)
	}
	return fmt.Sprintf(" %d items %s]%s%s", n, bc, rst, trailing)
}

// resolveObjectCaption looks up the OASF caption for a single class object
// (an element of a skills/domains/modules array) by extracting its "name"
// field and looking it up in the class entries cache.
func resolveObjectCaption(node *jsonNode, ct oasf.ClassType, rc *jsonRenderCtx) string {
	entries := rc.classEntries[ct]
	if len(entries) == 0 {
		return ""
	}
	name := extractChildName(node)
	if name == "" {
		return ""
	}
	color := classColor(ct, rc.theme)
	reset := rc.theme.Reset
	if e, ok := entries[name]; ok && e.Caption != "" {
		return color + e.Caption + reset
	}
	return color + name + reset
}

// classColor returns the ANSI color for a class type, matching the filter
// panel convention.
func classColor(ct oasf.ClassType, t *Theme) string {
	switch ct {
	case oasf.ClassTypeSkill:
		return t.Color1
	case oasf.ClassTypeDomain:
		return t.Color2
	case oasf.ClassTypeModule:
		return t.Color3
	}
	return ""
}

// extractChildName finds the "name" string value from an object node's
// children, which is the standard field in skill/domain/module objects.
func extractChildName(node *jsonNode) string {
	if node.nodeType != jsonObject {
		return ""
	}
	for _, c := range node.children {
		if c.key == "name" && c.nodeType == jsonPrimitive {
			if s, ok := c.value.(string); ok {
				return s
			}
		}
	}
	return ""
}

func formatPrimitive(v interface{}) string {
	if v == nil {
		return "null"
	}
	switch val := v.(type) {
	case string:
		return strconv.Quote(val)
	case float64:
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", val)
	}
}

// toggleLine toggles expand/collapse on the node at the given display line.
// Returns true if a toggle happened (i.e. the line had a collapsible node).
// Closing bracket lines (nil node) and primitives are no-ops.
func (t *jsonTree) toggleLine(line int) bool {
	if line < 0 || line >= len(t.lines) {
		return false
	}
	node := t.lines[line].node
	if node == nil || node.nodeType == jsonPrimitive || len(node.children) == 0 {
		return false
	}
	node.expanded = !node.expanded
	return true
}

// expandAll recursively expands all nodes in the tree.
func (t *jsonTree) expandAll() {
	if t.root != nil {
		expandNode(t.root)
	}
}

func expandNode(n *jsonNode) {
	if n.nodeType != jsonPrimitive {
		n.expanded = true
	}
	for _, c := range n.children {
		expandNode(c)
	}
}

// collapseAll recursively collapses all nodes in the tree.
func (t *jsonTree) collapseAll() {
	if t.root != nil {
		collapseNode(t.root)
	}
}

func collapseNode(n *jsonNode) {
	if n.nodeType != jsonPrimitive {
		n.expanded = false
	}
	for _, c := range n.children {
		collapseNode(c)
	}
}
