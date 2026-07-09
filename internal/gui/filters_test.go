// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"testing"

	"github.com/agntcy/lazydir/internal/dirclient"
	"github.com/agntcy/lazydir/internal/oasf"
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

func summariesWithVersions(versions ...string) []*dirclient.RecordSummary {
	out := make([]*dirclient.RecordSummary, 0, len(versions))
	for _, v := range versions {
		out = append(out, &dirclient.RecordSummary{SchemaVersion: v})
	}
	return out
}

func TestDistinctNewSchemaVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		summaries []*dirclient.RecordSummary
		fetched   map[string]bool
		want      []string
	}{
		{
			name:      "all new, deduped and order preserved",
			summaries: summariesWithVersions("1.0.0", "0.7.0", "1.0.0", "0.8.0"),
			want:      []string{"1.0.0", "0.7.0", "0.8.0"},
		},
		{
			name:      "already fetched versions skipped",
			summaries: summariesWithVersions("1.0.0", "0.7.0"),
			fetched:   map[string]bool{"1.0.0": true},
			want:      []string{"0.7.0"},
		},
		{
			name:      "empty versions ignored",
			summaries: summariesWithVersions("", "0.7.0", ""),
			want:      []string{"0.7.0"},
		},
		{
			name:      "nothing new",
			summaries: summariesWithVersions("1.0.0"),
			fetched:   map[string]bool{"1.0.0": true},
			want:      nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := distinctNewSchemaVersions(tt.summaries, tt.fetched)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestMergeClassEntries(t *testing.T) {
	t.Parallel()

	// A value that only exists in an older version fills a gap, while an
	// already-present name keeps its first (existing) entry.
	dst := map[string]oasf.ClassEntry{
		"schema.oasf/skill": {Name: "schema.oasf/skill", Caption: "Skill", Version: "1.0.0"},
	}
	src := map[string]oasf.ClassEntry{
		"schema.oasf/skill": {Name: "schema.oasf/skill", Caption: "Skill (old)", Version: "0.7.0"},
		"runtime/model":     {Name: "runtime/model", Caption: "Model", Version: "0.7.0"},
	}

	got := mergeClassEntries(dst, src)

	if e := got["runtime/model"]; e.Caption != "Model" || e.Version != "0.7.0" {
		t.Errorf("runtime/model = %+v, want caption=Model version=0.7.0", e)
	}
	if e := got["schema.oasf/skill"]; e.Caption != "Skill" || e.Version != "1.0.0" {
		t.Errorf("existing entry was overwritten: %+v", e)
	}
}

func TestMergeClassEntriesNilDst(t *testing.T) {
	t.Parallel()

	src := map[string]oasf.ClassEntry{"runtime/model": {Name: "runtime/model", Caption: "Model"}}
	got := mergeClassEntries(nil, src)
	if got["runtime/model"].Caption != "Model" {
		t.Errorf("merge into nil dst dropped entry: %+v", got)
	}
}
