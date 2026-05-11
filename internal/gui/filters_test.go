// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"testing"

	"github.com/agntcy/lazydir/internal/dirclient"
)

func TestFilterCategoryTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cat  filterCategory
		want string
	}{
		{filterSkills, "Skills"},
		{filterDomains, "Domains"},
		{filterModules, "Modules"},
		{filterOASFVersion, "OASF version"},
		{filterVersion, "Version"},
		{filterAuthor, "Author"},
		{filterTrustedVerified, "Trusted / Verified"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := tt.cat.title(); got != tt.want {
				t.Errorf("title() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCategoryToFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cat  filterCategory
		want dirclient.FilterCategory
	}{
		{filterSkills, dirclient.FilterSkill},
		{filterDomains, dirclient.FilterDomain},
		{filterModules, dirclient.FilterModule},
		{filterOASFVersion, dirclient.FilterSchemaVersion},
		{filterVersion, dirclient.FilterVersion},
		{filterAuthor, dirclient.FilterAuthor},
	}
	for _, tt := range tests {
		t.Run(tt.cat.title(), func(t *testing.T) {
			t.Parallel()
			if got := categoryToFilter(tt.cat); got != tt.want {
				t.Errorf("categoryToFilter(%d) = %d, want %d", tt.cat, got, tt.want)
			}
		})
	}
}

func TestAggregatorFieldFor(t *testing.T) {
	t.Parallel()

	a := newFilterValueAggregator()
	a.skills["nlp"] = true
	a.domains["security"] = true
	a.modules["auth"] = true
	a.schemaVersion["1.0"] = true
	a.versions["v2.1"] = true
	a.authors["alice"] = true

	tests := []struct {
		cat  filterCategory
		want string
	}{
		{filterSkills, "nlp"},
		{filterDomains, "security"},
		{filterModules, "auth"},
		{filterOASFVersion, "1.0"},
		{filterVersion, "v2.1"},
		{filterAuthor, "alice"},
	}
	for _, tt := range tests {
		t.Run(tt.cat.title(), func(t *testing.T) {
			t.Parallel()
			m := aggregatorFieldFor(a, tt.cat)
			if !m[tt.want] {
				t.Errorf("aggregatorFieldFor(%d) missing %q", tt.cat, tt.want)
			}
		})
	}

	if m := aggregatorFieldFor(a, filterTrustedVerified); m != nil {
		t.Error("expected nil for filterTrustedVerified")
	}
}

func TestAggregator_Add(t *testing.T) {
	t.Parallel()

	a := newFilterValueAggregator()
	a.add(&dirclient.RecordSummary{
		Skills:        []string{"nlp", "translation"},
		Domains:       []string{"security"},
		Modules:       []string{"auth"},
		Authors:       []string{"alice", ""},
		Version:       "1.0.0",
		SchemaVersion: "1.0",
	})
	a.add(&dirclient.RecordSummary{
		Skills:  []string{"nlp"},
		Authors: []string{"bob"},
		Version: "2.0.0",
	})

	if len(a.skills) != 2 {
		t.Errorf("skills = %v, want 2 entries", a.skills)
	}
	if len(a.authors) != 2 {
		t.Errorf("authors = %v, want 2 (empty string excluded)", a.authors)
	}
	if len(a.versions) != 2 {
		t.Errorf("versions = %v, want 2 entries", a.versions)
	}
	if !a.skills["nlp"] || !a.skills["translation"] {
		t.Error("expected nlp and translation in skills")
	}
}

func TestNewFilterState(t *testing.T) {
	t.Parallel()

	fs := newFilterState()
	if fs.expanded == nil || fs.applied == nil {
		t.Error("expected initialized maps")
	}
	if fs.listCursor != 0 || fs.filterQuery != "" {
		t.Error("expected zero-value defaults")
	}
}
