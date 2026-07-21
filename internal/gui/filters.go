// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/agntcy/lazydir/internal/dirclient"
	"github.com/agntcy/lazydir/internal/oasf"
)

// filterValueAggregator collects the unique values seen in the streamed
// records for each filter category. Values become available in the [2]
// Filters options view as soon as a record carrying them arrives.
type filterValueAggregator struct {
	skills        map[string]bool
	domains       map[string]bool
	modules       map[string]bool
	versions      map[string]bool
	schemaVersion map[string]bool
	authors       map[string]bool
}

func newFilterValueAggregator() *filterValueAggregator {
	return &filterValueAggregator{
		skills:        map[string]bool{},
		domains:       map[string]bool{},
		modules:       map[string]bool{},
		versions:      map[string]bool{},
		schemaVersion: map[string]bool{},
		authors:       map[string]bool{},
	}
}

// add folds one record's filterable fields into the aggregator.
func (a *filterValueAggregator) add(r *dirclient.RecordSummary) {
	for _, v := range r.Skills {
		a.skills[v] = true
	}
	for _, v := range r.Domains {
		a.domains[v] = true
	}
	for _, v := range r.Modules {
		a.modules[v] = true
	}
	for _, v := range r.Authors {
		if v != "" {
			a.authors[v] = true
		}
	}
	if r.SchemaVersion != "" {
		a.schemaVersion[r.SchemaVersion] = true
	}
	if r.Version != "" {
		a.versions[r.Version] = true
	}
}

// rebuildActiveFilterValues recomputes activeFilterValues from the current
// filtered record set so the Filters panel only shows relevant options.
func (app *Gui) rebuildActiveFilterValues() {
	a := newFilterValueAggregator()
	for _, r := range app.state.records {
		a.add(r)
	}
	app.state.activeFilterValues = a
}

// filterCategory identifies a filterable record field shown in the [2] Filters
// panel. The list is presented in this order.
type filterCategory int

const (
	filterSkills filterCategory = iota
	filterDomains
	filterModules
	filterOASFVersion
	filterVersion
	filterAuthor
	filterTrustedVerified
)

// allFilterCategories is the canonical ordered list of filter categories.
var allFilterCategories = []filterCategory{
	filterSkills,
	filterDomains,
	filterModules,
	filterOASFVersion,
	filterVersion,
	filterAuthor,
	filterTrustedVerified,
}

// title returns the human-readable label used as the row text in the filter
// list view and as the title in the options view.
func (c filterCategory) title() string {
	switch c {
	case filterSkills:
		return "Skills"
	case filterDomains:
		return "Domains"
	case filterModules:
		return "Modules"
	case filterOASFVersion:
		return "OASF version"
	case filterVersion:
		return "Version"
	case filterAuthor:
		return "Author"
	case filterTrustedVerified:
		return "Trusted / Verified"
	}
	return ""
}

// filterMode is the applied state of a single filter option. Absence of a key
// from the applied map means the option is not applied at all.
type filterMode int

const (
	modeInclude filterMode = iota + 1 // applied — keep records that match
	modeExclude                       // negated — hide records that match
)

// filterState owns all mutable state for the [2] Filters panel and the set of
// active filters that the records pane filters against. The map keys are
// option labels (e.g. skill name, version string, "yes"/"no").
type filterState struct {
	// listCursor indexes the visible rows (categories + their child options).
	listCursor int

	// expanded tracks which categories have their options dropdown open.
	expanded map[filterCategory]bool

	// applied[category][option] -> include/exclude mode. Absent = not applied.
	applied map[filterCategory]map[string]filterMode

	// inline description toggle (press 'i' on an option row)
	inlineDesc        string // option name currently expanded, "" if none
	inlineDescText    string // cached description text
	inlineDescError   bool   // true when inlineDescText is an error message
	inlineDescLoading bool   // fetch in progress

	// / search query — searches option labels across all non-boolean categories
	filterQuery string
}

func newFilterState() filterState {
	return filterState{
		expanded: map[filterCategory]bool{},
		applied:  map[filterCategory]map[string]filterMode{},
	}
}

// optionsFor returns the option labels available for a given category. When
// filters are active it reads from activeFilterValues (the narrowed set) so
// only relevant options are shown; otherwise it falls back to the full
// filterValues aggregator. Categories that already have active selections
// always use the full set so the user can add more values to broaden the
// search (standard faceted-search behaviour). Currently-applied selections
// are always included so the user can deselect them.
//
// Class categories (skills, domains, modules) are sorted by OASF ID when
// enrichment data is available; other categories are sorted alphabetically.
func (app *Gui) optionsFor(c filterCategory) []string {
	a := app.state.activeFilterValues
	if a == nil {
		a = app.state.filterValues
	}
	if a == nil {
		return nil
	}

	if c == filterTrustedVerified {
		return []string{"trusted", "verified"}
	}

	raw := aggregatorFieldFor(a, c)
	applied := app.state.filters.applied[c]

	merged := make(map[string]bool, len(raw)+len(applied))
	for k := range raw {
		merged[k] = true
	}
	for k := range applied {
		merged[k] = true
	}

	out := make([]string, 0, len(merged))
	for k := range merged {
		out = append(out, k)
	}

	entries := app.classEntriesFor(c)
	if len(entries) > 0 {
		sort.Slice(out, func(i, j int) bool {
			return entries[out[i]].ID < entries[out[j]].ID
		})
	} else {
		sort.Strings(out)
	}
	return out
}

// aggregatorFieldFor returns the value set for a given category from an
// aggregator instance.
func aggregatorFieldFor(a *filterValueAggregator, c filterCategory) map[string]bool {
	switch c {
	case filterSkills:
		return a.skills
	case filterDomains:
		return a.domains
	case filterModules:
		return a.modules
	case filterOASFVersion:
		return a.schemaVersion
	case filterVersion:
		return a.versions
	case filterAuthor:
		return a.authors
	}
	return nil
}

// classEntriesFor returns the OASF class entry map for a class filter
// category, or nil if the category is not a class or enrichment is unavailable.
func (app *Gui) classEntriesFor(c filterCategory) map[string]oasf.ClassEntry {
	if app.state.classEntries == nil {
		return nil
	}
	switch c {
	case filterSkills:
		return app.state.classEntries[oasf.ClassTypeSkill]
	case filterDomains:
		return app.state.classEntries[oasf.ClassTypeDomain]
	case filterModules:
		return app.state.classEntries[oasf.ClassTypeModule]
	}
	return nil
}

// appliedFor returns the sorted list of currently selected option labels for
// the given category — used to render the indented rows under each category.
func (app *Gui) appliedFor(c filterCategory) []string {
	set := app.state.filters.applied[c]
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// toggleApplied cycles an option through not-applied → include → exclude →
// not-applied for the given category.
func (app *Gui) toggleApplied(c filterCategory, value string) {
	set := app.state.filters.applied[c]
	if set == nil {
		set = map[string]filterMode{}
		app.state.filters.applied[c] = set
	}
	switch set[value] {
	case modeInclude:
		set[value] = modeExclude
	case modeExclude:
		delete(set, value)
		if len(set) == 0 {
			delete(app.state.filters.applied, c)
		}
	default: // not applied
		set[value] = modeInclude
	}
}

// listRow describes one rendered line in the filter list view. Either it is a
// category header (option == ""), or an indented selected option row.
type listRow struct {
	category filterCategory
	option   string // empty for category headers
}

// listRows builds the visible rows for the unified filter tree: each category
// header is followed by either all available options (when expanded) or only
// the currently selected options (when collapsed).
func (app *Gui) listRows() []listRow {
	var rows []listRow
	for _, c := range allFilterCategories {
		rows = append(rows, listRow{category: c})
		if app.state.filters.expanded[c] {
			for _, opt := range app.optionsFor(c) {
				rows = append(rows, listRow{category: c, option: opt})
			}
		} else {
			for _, opt := range app.appliedFor(c) {
				rows = append(rows, listRow{category: c, option: opt})
			}
		}
	}
	return rows
}

func categoryToFilter(c filterCategory) dirclient.FilterCategory {
	switch c {
	case filterSkills:
		return dirclient.FilterSkill
	case filterDomains:
		return dirclient.FilterDomain
	case filterModules:
		return dirclient.FilterModule
	case filterOASFVersion:
		return dirclient.FilterSchemaVersion
	case filterVersion:
		return dirclient.FilterVersion
	case filterAuthor:
		return dirclient.FilterAuthor
	}
	return dirclient.FilterSkill
}

// filteredListRows returns the rows to display. When no query is active it
// delegates to listRows (respecting the expanded/collapsed state). When a
// search query is active it ignores the expanded state and shows every option
// whose label matches the query, grouped under its category header. The
// Trusted / Verified category is excluded from search results. For class
// categories the search also matches against the OASF caption and ID.
func (app *Gui) filteredListRows() []listRow {
	q := app.state.filters.filterQuery
	if q == "" {
		return app.listRows()
	}
	q = strings.ToLower(q)
	var rows []listRow
	for _, c := range allFilterCategories {
		if c == filterTrustedVerified {
			continue
		}
		entries := app.classEntriesFor(c)
		var matching []string
		for _, opt := range app.optionsFor(c) {
			if app.optionMatchesQuery(opt, q, entries) {
				matching = append(matching, opt)
			}
		}
		if len(matching) > 0 {
			rows = append(rows, listRow{category: c})
			for _, opt := range matching {
				rows = append(rows, listRow{category: c, option: opt})
			}
		}
	}
	return rows
}

// optionMatchesQuery checks whether an option matches the search query. For
// class categories with enrichment data it matches against name, caption,
// and numeric ID.
func (app *Gui) optionMatchesQuery(opt, q string, entries map[string]oasf.ClassEntry) bool {
	if strings.Contains(strings.ToLower(opt), q) {
		return true
	}
	if entries == nil {
		return false
	}
	e, ok := entries[opt]
	if !ok {
		return false
	}
	if strings.Contains(strings.ToLower(e.Caption), q) {
		return true
	}
	if strings.Contains(fmt.Sprintf("%d", e.ID), q) {
		return true
	}
	return false
}
