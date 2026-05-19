// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"fmt"
	"strings"
)

// binding describes a single keybinding for display purposes.
type binding struct {
	key         string // e.g. "enter", "↑↓", "h/l"
	description string // e.g. "select", "navigate"
	showInBar   bool   // shown in the bottom options bar
}

// panelBindings holds the ordered list of bindings for one panel.
type panelBindings struct {
	title    string    // panel display name used in the help popup title
	bindings []binding // all bindings, in display order
}

// globalBindings appear in every panel's options bar and help popup.
var globalBindings = []binding{
	{"tab / shift+tab", "cycle focus", true},
	{"1 / 2 / 3 / 0", "jump to panel", false},
	{"r", "refresh", false},
	{"?: help", "show keybindings", true},
	{"q / ctrl+c", "quit", true},
}

// perPanelBindings defines bindings per named view.
var perPanelBindings = map[string]panelBindings{
	viewDirectory: {
		title: "[1] Connections",
		bindings: []binding{
			{"↑↓ / j k", "navigate", true},
			{"c / enter", "select server", true},
			{"i", "connection info", true},
		},
	},
	viewFilters: {
		title: "[2] Filters",
		bindings: []binding{
			{"↑↓ / j k", "navigate", true},
			{"enter", "expand / toggle filter", true},
			{"space", "toggle filter", true},
			{"/", "search options", true},
			{"i", "toggle class info", true},
			{"x", "clear all filters", true},
			{"esc", "clear search", true},
			{"wheel", "scroll", false},
		},
	},
	viewRecords: {
		title: "[3] Records",
		bindings: []binding{
			{"↑↓ / j k", "navigate", true},
			{"/", "filter by name", true},
			{"i", "toggle record info", true},
			{"y", "yank/copy options", true},
			{"esc", "clear filter", false},
			{"enter", "confirm filter", false},
			{"wheel", "scroll", false},
		},
	},
	viewPreview: {
		title: "[0] Preview",
		bindings: []binding{
			{"↑↓ / j k", "navigate", true},
			{"enter", "expand / collapse", true},
			{"e", "expand all", true},
			{"E", "collapse all", true},
			{"wheel", "scroll", false},
		},
	},
}

// optionsBarText returns the keybinding hints for the bottom-left options bar,
// truncated with "…" if they don't fit in availableWidth.
// Format matches lazygit: "description: key | description: key"
func optionsBarText(focused string, availableWidth int) string {
	var items []string

	if pb, ok := perPanelBindings[focused]; ok {
		for _, b := range pb.bindings {
			if b.showInBar {
				items = append(items, fmt.Sprintf("%s: %s", b.description, b.key))
			}
		}
	}
	for _, b := range globalBindings {
		if b.showInBar {
			items = append(items, fmt.Sprintf("%s: %s", b.description, b.key))
		}
	}

	sep := " | "
	var sb strings.Builder
	for i, item := range items {
		candidate := item
		if i > 0 {
			candidate = sep + item
		}
		if availableWidth > 0 && sb.Len()+len(candidate) > availableWidth {
			if sb.Len() > 0 {
				sb.WriteString(sep + "…")
			}
			break
		}
		sb.WriteString(candidate)
	}
	return sb.String()
}

// helpPopupLines returns the aligned lines for the ? keybindings popup.
func helpPopupLines(focused string) []string {
	pb, ok := perPanelBindings[focused]
	if !ok {
		pb = panelBindings{title: focused}
	}

	// Compute widest key label across all bindings for column alignment.
	maxKey := 0
	for _, b := range pb.bindings {
		if len(b.key) > maxKey {
			maxKey = len(b.key)
		}
	}
	for _, b := range globalBindings {
		if len(b.key) > maxKey {
			maxKey = len(b.key)
		}
	}

	format := fmt.Sprintf("  %%-%ds  %%s", maxKey)
	divider := "  " + strings.Repeat("─", maxKey+22)

	var lines []string

	if len(pb.bindings) > 0 {
		lines = append(lines, fmt.Sprintf("  %s", pb.title))
		lines = append(lines, divider)
		for _, b := range pb.bindings {
			lines = append(lines, fmt.Sprintf(format, b.key, b.description))
		}
		lines = append(lines, "")
	}

	lines = append(lines, "  Global")
	lines = append(lines, divider)
	for _, b := range globalBindings {
		// skip the "?: help" meta-entry — it's always available and obvious
		if b.key == "?: help" {
			continue
		}
		lines = append(lines, fmt.Sprintf(format, b.key, b.description))
	}
	// jump shortcuts not in globalBindings to avoid bar clutter
	lines = append(lines, fmt.Sprintf(format, "1 / 2 / 3 / 0", "jump to panel"))

	return lines
}
