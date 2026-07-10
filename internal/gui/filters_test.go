// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"bytes"
	"strings"
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

func TestToggleAppliedCycles(t *testing.T) {
	app := &Gui{state: appState{filters: newFilterState()}}

	app.toggleApplied(filterSkills, "nlp")
	if got := app.state.filters.applied[filterSkills]["nlp"]; got != modeInclude {
		t.Fatalf("after 1st toggle = %d, want modeInclude", got)
	}
	app.toggleApplied(filterSkills, "nlp")
	if got := app.state.filters.applied[filterSkills]["nlp"]; got != modeExclude {
		t.Fatalf("after 2nd toggle = %d, want modeExclude", got)
	}
	app.toggleApplied(filterSkills, "nlp")
	if _, ok := app.state.filters.applied[filterSkills]; ok {
		t.Fatalf("after 3rd toggle category should be removed, got %v", app.state.filters.applied)
	}
}

func TestMatchesFiltersIncludeExclude(t *testing.T) {
	rec := &dirclient.RecordSummary{
		Skills:        []string{"nlp", "vision"},
		Version:       "1.0.0",
		SchemaVersion: "0.7.0",
	}
	tests := []struct {
		name    string
		applied map[filterCategory]map[string]filterMode
		want    bool
	}{
		{"include match", map[filterCategory]map[string]filterMode{filterSkills: {"nlp": modeInclude}}, true},
		{"include miss", map[filterCategory]map[string]filterMode{filterSkills: {"audio": modeInclude}}, false},
		{"exclude drops match", map[filterCategory]map[string]filterMode{filterSkills: {"nlp": modeExclude}}, false},
		{"exclude keeps non-match", map[filterCategory]map[string]filterMode{filterSkills: {"audio": modeExclude}}, true},
		{"scalar include", map[filterCategory]map[string]filterMode{filterVersion: {"1.0.0": modeInclude}}, true},
		{"scalar exclude", map[filterCategory]map[string]filterMode{filterVersion: {"1.0.0": modeExclude}}, false},
		{"mixed include+exclude", map[filterCategory]map[string]filterMode{filterSkills: {"nlp": modeInclude, "vision": modeExclude}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesFilters(rec, tt.applied); got != tt.want {
				t.Errorf("matchesFilters = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenderFilterOptionStrike(t *testing.T) {
	app := &Gui{theme: defaultTheme}
	app.theme.Strike = "\033[9m"

	var incl, excl bytes.Buffer
	app.writeFilterOption(&incl, listRow{category: filterSkills, option: "nlp"}, modeInclude)
	app.writeFilterOption(&excl, listRow{category: filterSkills, option: "nlp"}, modeExclude)

	if strings.Contains(incl.String(), "\033[9m") {
		t.Errorf("include row should not contain strike code: %q", incl.String())
	}
	if !strings.Contains(excl.String(), "\033[9m") {
		t.Errorf("exclude row should contain strike code: %q", excl.String())
	}

	// Not-applied (zero-value filterMode) must not emit any escape codes.
	var none bytes.Buffer
	app.writeFilterOption(&none, listRow{category: filterSkills, option: "nlp"}, filterMode(0))
	if strings.ContainsRune(none.String(), '\033') {
		t.Errorf("not-applied row should not contain escape codes: %q", none.String())
	}

	// ID+Caption class-entry branch under modeExclude: caption is rendered and
	// wrapped in the strike code.
	app.state.classEntries = map[oasf.ClassType]map[string]oasf.ClassEntry{
		oasf.ClassTypeSkill: {
			"nlp": {ID: 1, Name: "nlp", Caption: "Natural Language Processing"},
		},
	}
	var caption bytes.Buffer
	app.writeFilterOption(&caption, listRow{category: filterSkills, option: "nlp"}, modeExclude)
	if !strings.Contains(caption.String(), "Natural Language Processing") {
		t.Errorf("caption row should contain the class caption: %q", caption.String())
	}
	if !strings.Contains(caption.String(), "\033[9m") {
		t.Errorf("caption row under exclude should contain strike code: %q", caption.String())
	}
}

func TestMarkTrustedVerified(t *testing.T) {
	records := []*dirclient.RecordSummary{
		{CID: "a"}, {CID: "b"}, {CID: "c"},
	}
	markTrustedVerified(records, []string{"a", "c"}, []string{"b"})

	want := map[string][2]bool{
		"a": {true, false},
		"b": {false, true},
		"c": {true, false},
	}
	for _, r := range records {
		if got := [2]bool{r.Trusted, r.Verified}; got != want[r.CID] {
			t.Errorf("cid %s = {trusted:%v verified:%v}, want %v", r.CID, r.Trusted, r.Verified, want[r.CID])
		}
	}
}

func TestMatchesTrustedVerified(t *testing.T) {
	trusted := &dirclient.RecordSummary{CID: "a", Trusted: true}
	verified := &dirclient.RecordSummary{CID: "b", Verified: true}
	plain := &dirclient.RecordSummary{CID: "c"}

	inclTrusted := map[filterCategory]map[string]filterMode{
		filterTrustedVerified: {"trusted": modeInclude},
	}
	exclTrusted := map[filterCategory]map[string]filterMode{
		filterTrustedVerified: {"trusted": modeExclude},
	}
	inclVerified := map[filterCategory]map[string]filterMode{
		filterTrustedVerified: {"verified": modeInclude},
	}

	if !matchesFilters(trusted, inclTrusted) {
		t.Error("include trusted should keep a trusted record")
	}
	if matchesFilters(plain, inclTrusted) {
		t.Error("include trusted should drop a non-trusted record")
	}
	if matchesFilters(trusted, exclTrusted) {
		t.Error("exclude trusted should drop a trusted record")
	}
	if !matchesFilters(plain, exclTrusted) {
		t.Error("exclude trusted should keep a non-trusted record")
	}
	if !matchesFilters(verified, inclVerified) {
		t.Error("include verified should keep a verified record")
	}
}
