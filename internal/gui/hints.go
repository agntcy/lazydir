// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"fmt"
	"strings"
)

// Repeated hint labels, key names and panel titles shared by the binding tables
// and the panel views.
const (
	hintNavigate = "navigate"
	hintExpand   = "expand"
	hintCollapse = "collapse"
	hintScroll   = "scroll"
	keyEnter     = "enter"
	keyEsc       = "esc"
	keyNavigate  = "↑↓ / j k"
	keyWheel     = "wheel"
	keySpace     = "space"
	keyMinScore  = "+ / -"

	titleFilters = "[2] Filters"
	titleRecords = "[3] Records"
	titlePreview = "[0] Preview"
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
			{keyNavigate, hintNavigate, true},
			{"c / enter", "select server / mode", true},
			{"i", "connection info", true},
		},
	},
	viewFilters: {
		title: titleFilters,
		bindings: []binding{
			{keyNavigate, hintNavigate, true},
			{keyEnter, "expand / cycle filter", true},
			{"l", hintExpand, false},
			{"h", hintCollapse, false},
			{keySpace, "cycle include/exclude/off", true},
			{"/", "search options", true},
			{"i", "toggle class info", true},
			{"x", "clear all filters", true},
			{keyEsc, "collapse / clear search", true},
			{keyWheel, hintScroll, false},
		},
	},
	viewRecords: {
		title: titleRecords,
		bindings: []binding{
			{keyNavigate, hintNavigate, true},
			{keyEnter, "open preview", true},
			{"/", "filter by name", true},
			{"i", "toggle record info", true},
			{"y", "yank/copy options", true},
			{keySpace, "select record", true},
			{"ctrl+a", "select all loaded", false},
			{"C", "clipboard toggle", true},
			{"V", "paste records", true},
			{"d", "delete record / selection", true},
			{"P", "publish record / selection", true},
			{"U", "unpublish record / selection", true},
			{"X", "cancel sync", false},
			{keyEsc, "clear filter / selection / clipboard", false},
			{keyWheel, hintScroll, false},
		},
	},
	viewPreview: {
		title: titlePreview,
		bindings: []binding{
			{keyNavigate, hintNavigate, true},
			{keyEnter, "expand / collapse", true},
			{"l", hintExpand, false},
			{"h", hintCollapse, false},
			{"e", "expand all", true},
			{"E", "collapse all", true},
			{keyEsc, "back to records", true},
			{keyWheel, hintScroll, false},
		},
	},
	viewCopyMenu: {
		title: "Copy options",
		bindings: []binding{
			{keyNavigate, hintNavigate, true},
			{keyEnter, "select", true},
			{keyEsc, "close", true},
		},
	},
	viewConfirmPopup: {
		title: "Confirm",
		bindings: []binding{
			{keyNavigate, hintNavigate, true},
			{keyEnter, "select", true},
			{keyEsc, "close", true},
		},
	},
}

// networkPerPanelBindings overrides perPanelBindings for the panels whose keys
// differ in network mode. Directory and the popups are shared.
var networkPerPanelBindings = map[string]panelBindings{
	viewFilters: networkFiltersBindings,
	viewRecords: networkRecordsBindings,
	viewPreview: networkPreviewBindings,
}

// networkPreviewBindings adds S to the Preview keys: the preview of an
// unreachable peer offers it.
var networkPreviewBindings = panelBindings{
	title: titlePreview,
	bindings: []binding{
		{keyNavigate, hintNavigate, true},
		{keyEnter, "expand / collapse", true},
		{"l", hintExpand, false},
		{"h", hintCollapse, false},
		{"e", "expand all", true},
		{"E", "collapse all", true},
		{"S", "sync record / selection", true},
		{keyEsc, "back to records", true},
		{keyWheel, hintScroll, false},
	},
}

var networkFiltersBindings = panelBindings{
	title: titleFilters,
	bindings: []binding{
		{keyNavigate, hintNavigate, true},
		{keyEnter, "expand / toggle", true},
		{"l", hintExpand, false},
		{"h", hintCollapse, false},
		{keySpace, "toggle", true},
		{"/", "search options", true},
		{"x", "clear all filters and versions", true},
		{keyMinScore, "min match score", true},
		{keyEsc, "collapse / clear search", true},
		{keyWheel, hintScroll, false},
	},
}

var networkRecordsBindings = panelBindings{
	title: titleRecords,
	bindings: []binding{
		{keyNavigate, hintNavigate, true},
		{keyEnter, "open preview", true},
		{"i", "sync failure info", false},
		{keySpace, "select record", true},
		{"ctrl+a", "select all", false},
		{"S", "sync record / selection", true},
		{keyMinScore, "min match score", true},
		{keyEsc, "clear selection", false},
		{keyWheel, hintScroll, false},
	},
}

// bindingsFor returns the bindings shown for a panel, using the network-mode
// table when network is set and the panel has one.
func bindingsFor(focused string, network bool) (panelBindings, bool) {
	if network {
		if pb, ok := networkPerPanelBindings[focused]; ok {
			return pb, true
		}
	}
	pb, ok := perPanelBindings[focused]
	return pb, ok
}

// barStatus is the transient state shown ahead of the hints in the options bar.
type barStatus struct {
	selected    int
	copied      int
	syncing     int
	reconciling int
	progress    string // running batch, e.g. "deleting 3/5"
}

// optionsBarText returns the keybinding hints for the bottom-left options bar,
// truncated with "…" if they don't fit in availableWidth.
// Format matches lazygit: "description: key | description: key"
func optionsBarText(focused string, availableWidth int, network bool, st barStatus) string {
	var items []string

	if st.progress != "" {
		items = append(items, st.progress)
	}
	if st.selected > 0 {
		items = append(items, fmt.Sprintf("%d selected", st.selected))
	}
	if st.copied > 0 {
		items = append(items, fmt.Sprintf("%d copied", st.copied))
	}
	if st.syncing > 0 {
		items = append(items, fmt.Sprintf("%d syncing", st.syncing))
	}
	if st.reconciling > 0 {
		items = append(items, fmt.Sprintf("%d reconciling", st.reconciling))
	}
	// X cancels a This-node paste sync; it does nothing in network mode.
	if st.syncing+st.reconciling > 0 && focused == viewRecords && !network {
		items = append(items, "cancel sync: X")
	}

	if pb, ok := bindingsFor(focused, network); ok {
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
func helpPopupLines(focused string, network bool) []string {
	pb, ok := bindingsFor(focused, network)
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
