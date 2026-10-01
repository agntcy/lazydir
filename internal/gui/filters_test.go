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

// Option values, versions and captions reused across the GUI tests.
const (
	optNLP            = "nlp"
	optSecurity       = "security"
	optTranslation    = "translation"
	ver100            = "1.0.0"
	ver070            = "0.7.0"
	classSkillSchema  = "schema.oasf/skill"
	classRuntimeModel = "runtime/model"
	captionModel      = "Model"
	captionNLP        = "Natural Language Processing"
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
	a.skills[optNLP] = true
	a.domains[optSecurity] = true
	a.modules["auth"] = true
	a.schemaVersion["1.0"] = true
	a.authors["alice"] = true

	tests := []struct {
		cat  filterCategory
		want string
	}{
		{filterSkills, optNLP},
		{filterDomains, optSecurity},
		{filterModules, "auth"},
		{filterOASFVersion, "1.0"},
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

func TestNewFilterValuesFrom(t *testing.T) {
	t.Parallel()

	a := newFilterValuesFrom(map[dirclient.FilterCategory][]string{
		dirclient.FilterSkill:         {optNLP, optTranslation},
		dirclient.FilterDomain:        {optSecurity},
		dirclient.FilterModule:        {classRuntimeModel},
		dirclient.FilterAuthor:        {"carol", ""},
		dirclient.FilterSchemaVersion: {ver100, ver070},
		dirclient.FilterName:          {"ignored"},
	})

	tests := []struct {
		name string
		got  map[string]bool
		want []string
	}{
		{"skillSet", a.skills, []string{optNLP, optTranslation}},
		{"domainSet", a.domains, []string{optSecurity}},
		{"moduleSet", a.modules, []string{classRuntimeModel}},
		{"authorSet", a.authors, []string{"carol"}},
		{"schemaVersionSet", a.schemaVersion, []string{ver100, ver070}},
	}
	for _, tt := range tests {
		if len(tt.got) != len(tt.want) {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
			continue
		}
		for _, w := range tt.want {
			if !tt.got[w] {
				t.Errorf("%s missing %q", tt.name, w)
			}
		}
	}

	for _, set := range []map[string]bool{a.skills, a.domains, a.modules, a.authors, a.schemaVersion} {
		if set["ignored"] {
			t.Error("unknown category value leaked into a filter set")
		}
	}

	if e := newFilterValuesFrom(nil); len(e.skills)+len(e.domains)+len(e.modules)+len(e.authors)+len(e.schemaVersion) != 0 {
		t.Error("nil input should yield an empty aggregator")
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
			summaries: summariesWithVersions(ver100, ver070, ver100, "0.8.0"),
			want:      []string{ver100, ver070, "0.8.0"},
		},
		{
			name:      "already fetched versions skipped",
			summaries: summariesWithVersions(ver100, ver070),
			fetched:   map[string]bool{ver100: true},
			want:      []string{ver070},
		},
		{
			name:      "empty versions ignored",
			summaries: summariesWithVersions("", ver070, ""),
			want:      []string{ver070},
		},
		{
			name:      "nothing new",
			summaries: summariesWithVersions(ver100),
			fetched:   map[string]bool{ver100: true},
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
		classSkillSchema: {Name: classSkillSchema, Caption: "Skill", Version: ver100},
	}
	src := map[string]oasf.ClassEntry{
		classSkillSchema:  {Name: classSkillSchema, Caption: "Skill (old)", Version: ver070},
		classRuntimeModel: {Name: classRuntimeModel, Caption: captionModel, Version: ver070},
	}

	got := mergeClassEntries(dst, src)

	if e := got[classRuntimeModel]; e.Caption != captionModel || e.Version != ver070 {
		t.Errorf("runtime/model = %+v, want caption=Model version=0.7.0", e)
	}
	if e := got[classSkillSchema]; e.Caption != "Skill" || e.Version != ver100 {
		t.Errorf("existing entry was overwritten: %+v", e)
	}
}

func TestMergeClassEntriesNilDst(t *testing.T) {
	t.Parallel()

	src := map[string]oasf.ClassEntry{classRuntimeModel: {Name: classRuntimeModel, Caption: captionModel}}
	got := mergeClassEntries(nil, src)
	if got[classRuntimeModel].Caption != captionModel {
		t.Errorf("merge into nil dst dropped entry: %+v", got)
	}
}

func TestToggleAppliedCycles(t *testing.T) {
	app := &Gui{state: appState{filters: newFilterState()}}

	app.toggleApplied(filterSkills, optNLP)
	if got := app.state.filters.applied[filterSkills][optNLP]; got != modeInclude {
		t.Fatalf("after 1st toggle = %d, want modeInclude", got)
	}
	app.toggleApplied(filterSkills, optNLP)
	if got := app.state.filters.applied[filterSkills][optNLP]; got != modeExclude {
		t.Fatalf("after 2nd toggle = %d, want modeExclude", got)
	}
	app.toggleApplied(filterSkills, optNLP)
	if _, ok := app.state.filters.applied[filterSkills]; ok {
		t.Fatalf("after 3rd toggle category should be removed, got %v", app.state.filters.applied)
	}
}

func TestRenderFilterOptionStrike(t *testing.T) {
	app := &Gui{theme: defaultTheme}
	app.theme.Strike = "\033[9m"

	var incl, excl bytes.Buffer
	app.writeFilterOption(&incl, listRow{category: filterSkills, option: optNLP}, modeInclude)
	app.writeFilterOption(&excl, listRow{category: filterSkills, option: optNLP}, modeExclude)

	if strings.Contains(incl.String(), "\033[9m") {
		t.Errorf("include row should not contain strike code: %q", incl.String())
	}
	if !strings.Contains(excl.String(), "\033[9m") {
		t.Errorf("exclude row should contain strike code: %q", excl.String())
	}

	// Not-applied (zero-value filterMode) must not emit any escape codes.
	var none bytes.Buffer
	app.writeFilterOption(&none, listRow{category: filterSkills, option: optNLP}, filterMode(0))
	if strings.ContainsRune(none.String(), '\033') {
		t.Errorf("not-applied row should not contain escape codes: %q", none.String())
	}

	// ID+Caption class-entry branch under modeExclude: caption is rendered and
	// wrapped in the strike code.
	app.state.classEntries = map[oasf.ClassType]map[string]oasf.ClassEntry{
		oasf.ClassTypeSkill: {
			optNLP: {ID: 1, Name: optNLP, Caption: captionNLP},
		},
	}
	var caption bytes.Buffer
	app.writeFilterOption(&caption, listRow{category: filterSkills, option: optNLP}, modeExclude)
	if !strings.Contains(caption.String(), captionNLP) {
		t.Errorf("caption row should contain the class caption: %q", caption.String())
	}
	if !strings.Contains(caption.String(), "\033[9m") {
		t.Errorf("caption row under exclude should contain strike code: %q", caption.String())
	}
}

func TestBuildServerQueries_IncludeExclude(t *testing.T) {
	fs := filterState{applied: map[filterCategory]map[string]filterMode{
		filterSkills:  {optNLP: modeInclude},
		filterDomains: {"finance": modeExclude},
	}}
	got := buildServerQueries(fs, "")

	want := map[dirclient.FilterCategory]dirclient.Query{
		dirclient.FilterSkill:  {Category: dirclient.FilterSkill, Value: optNLP, Negate: false},
		dirclient.FilterDomain: {Category: dirclient.FilterDomain, Value: "finance", Negate: true},
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d queries, got %d", len(want), len(got))
	}
	for _, q := range got {
		if want[q.Category] != q {
			t.Errorf("unexpected query: %+v", q)
		}
	}
}

func TestBuildServerQueries_TrustedVerified(t *testing.T) {
	fs := filterState{applied: map[filterCategory]map[string]filterMode{
		filterTrustedVerified: {"trusted": modeInclude, "verified": modeExclude},
	}}
	got := buildServerQueries(fs, "")
	seen := map[dirclient.FilterCategory]dirclient.Query{}
	for _, q := range got {
		seen[q.Category] = q
	}
	if q := seen[dirclient.FilterTrusted]; q.Value != valueTrue || q.Negate {
		t.Errorf("trusted mapping wrong: %+v", q)
	}
	if q := seen[dirclient.FilterVerified]; q.Value != valueTrue || !q.Negate {
		t.Errorf("verified mapping wrong: %+v", q)
	}
}

func TestBuildServerQueries_NameWildcard(t *testing.T) {
	got := buildServerQueries(filterState{applied: map[filterCategory]map[string]filterMode{}}, "assistant")
	if len(got) != 1 || got[0].Category != dirclient.FilterName || got[0].Value != "*assistant*" {
		t.Fatalf("unexpected name query: %+v", got)
	}
}
