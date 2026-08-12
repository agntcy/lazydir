// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package dirclient

import (
	"strings"
	"testing"

	corev1 "github.com/agntcy/dir/api/core/v1"
	routingv1 "github.com/agntcy/dir/api/routing/v1"
	searchv1 "github.com/agntcy/dir/api/search/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestQueryToRPC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		category FilterCategory
		value    string
		wantType searchv1.RecordQueryType
	}{
		{"skill", FilterSkill, "nlp", searchv1.RecordQueryType_RECORD_QUERY_TYPE_SKILL_NAME},
		{"domain", FilterDomain, "security", searchv1.RecordQueryType_RECORD_QUERY_TYPE_DOMAIN_NAME},
		{"module", FilterModule, "auth", searchv1.RecordQueryType_RECORD_QUERY_TYPE_MODULE_NAME},
		{"schema version", FilterSchemaVersion, "1.0", searchv1.RecordQueryType_RECORD_QUERY_TYPE_SCHEMA_VERSION},
		{"version", FilterVersion, "2.0", searchv1.RecordQueryType_RECORD_QUERY_TYPE_VERSION},
		{"author", FilterAuthor, "alice", searchv1.RecordQueryType_RECORD_QUERY_TYPE_AUTHOR},
		{"trusted", FilterTrusted, "true", searchv1.RecordQueryType_RECORD_QUERY_TYPE_TRUSTED},
		{"verified", FilterVerified, "true", searchv1.RecordQueryType_RECORD_QUERY_TYPE_VERIFIED},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			q := Query{Category: tt.category, Value: tt.value}
			rpc := q.toRPC()
			if rpc.GetType() != tt.wantType {
				t.Errorf("toRPC().Type = %v, want %v", rpc.GetType(), tt.wantType)
			}
			if rpc.GetValue() != tt.value {
				t.Errorf("toRPC().Value = %q, want %q", rpc.GetValue(), tt.value)
			}
		})
	}
}

func TestConnect_BadAddress(t *testing.T) {
	t.Parallel()

	// The gRPC dialer accepts empty addresses lazily, so connect to a
	// clearly unreachable address and verify Ping fails.
	ctx := t.Context()
	c, err := Connect(ctx, Config{ServerAddress: "127.0.0.1:1"})
	if err != nil {
		// Some environments reject this immediately; that's fine.
		return
	}
	defer c.Close()

	if err := c.Ping(ctx); err == nil {
		t.Fatal("expected Ping to fail on unreachable address")
	}
}

func TestClose_NilClient(t *testing.T) {
	t.Parallel()

	c := &Client{}
	c.Close()
}

// makeRecord builds a minimal record carrying one skill, domain and module for
// the given OASF schema version.
func makeRecord(t *testing.T, schemaVersion string) *corev1.Record {
	t.Helper()

	data, err := structpb.NewStruct(map[string]any{
		"schema_version": schemaVersion,
		"name":           "test-agent",
		"version":        "1.0.0",
		"authors":        []any{"alice"},
		"skills":         []any{map[string]any{"name": "natural_language_processing"}},
		"domains":        []any{map[string]any{"name": "biotechnology"}},
		"modules":        []any{map[string]any{"name": "runtime/model"}},
	})
	if err != nil {
		t.Fatalf("building struct: %v", err)
	}

	return &corev1.Record{Data: data}
}

// TestExtractSummaryAcrossSchemaVersions guards against the regression where
// skills/domains/modules were only collected from v1 (1.x) records, leaving
// older 0.7.x (v1alpha1) and 0.8.x (v1alpha2) records absent from the filters.
func TestExtractSummaryAcrossSchemaVersions(t *testing.T) {
	t.Parallel()

	versions := []string{"0.7.0", "0.8.0", "1.0.0"}

	for _, v := range versions {
		t.Run(v, func(t *testing.T) {
			t.Parallel()

			s := extractSummary(makeRecord(t, v))
			if s == nil {
				t.Fatalf("extractSummary returned nil for schema version %s", v)
			}

			if want, got := "test-agent", s.Name; got != want {
				t.Errorf("Name = %q, want %q", got, want)
			}
			if len(s.Skills) != 1 || s.Skills[0] != "natural_language_processing" {
				t.Errorf("Skills = %v, want [natural_language_processing]", s.Skills)
			}
			if len(s.Domains) != 1 || s.Domains[0] != "biotechnology" {
				t.Errorf("Domains = %v, want [biotechnology]", s.Domains)
			}
			if len(s.Modules) != 1 || s.Modules[0] != "runtime/model" {
				t.Errorf("Modules = %v, want [runtime/model]", s.Modules)
			}
		})
	}
}

// TestExtractSummarySanitizesNewlines guards against malformed server data
// (e.g. a directory daemon that self-registers with version "v1.6.1\n1.6.1")
// leaking embedded newlines into name/version. Rendered verbatim, such a value
// spills onto an extra row and breaks the Records/Filters panel layout.
func TestExtractSummarySanitizesNewlines(t *testing.T) {
	t.Parallel()

	data, err := structpb.NewStruct(map[string]any{
		"schema_version": "1.0.0",
		"name":           "org.agntcy/directory",
		"version":        "v1.6.1\n1.6.1",
	})
	if err != nil {
		t.Fatalf("building struct: %v", err)
	}

	s := extractSummary(&corev1.Record{Data: data})
	if s == nil {
		t.Fatal("extractSummary returned nil")
	}
	if strings.ContainsAny(s.Version, "\r\n") {
		t.Errorf("Version %q still contains a line break", s.Version)
	}
	if want := "v1.6.1 1.6.1"; s.Version != want {
		t.Errorf("Version = %q, want %q", s.Version, want)
	}
}

func TestSanitizeField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "v1.0.0", "v1.0.0"},
		{"trailing newline", "v1.0.0\n", "v1.0.0"},
		{"embedded newline", "v1.6.1\n1.6.1", "v1.6.1 1.6.1"},
		{"crlf", "a\r\nb", "a b"},
		{"surrounding whitespace", "  v1.0.0  ", "v1.0.0"},
		{"tabs and newlines", "a\t\nb", "a b"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := sanitizeField(tt.in); got != tt.want {
				t.Errorf("sanitizeField(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMinFunc(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b, want int
	}{
		{1, 2, 1},
		{3, 1, 1},
		{5, 5, 5},
		{0, 0, 0},
		{-1, 1, -1},
	}
	for _, tt := range tests {
		if got := min(tt.a, tt.b); got != tt.want {
			t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestRecordSummaryTrustedVerifiedDefaultFalse(t *testing.T) {
	var s RecordSummary
	if s.Trusted || s.Verified {
		t.Fatalf("zero-value RecordSummary should have Trusted=false Verified=false, got %+v", s)
	}
}

func TestBuildRecordRefs(t *testing.T) {
	t.Parallel()

	refs := buildRecordRefs([]string{"cidA", "cidB"})
	got := refs.GetRefs()
	if len(got) != 2 {
		t.Fatalf("len(refs) = %d, want 2", len(got))
	}
	if got[0].GetCid() != "cidA" || got[1].GetCid() != "cidB" {
		t.Errorf("refs = [%q %q], want [cidA cidB]", got[0].GetCid(), got[1].GetCid())
	}
}

func TestBuildRecordRefsEmpty(t *testing.T) {
	t.Parallel()

	refs := buildRecordRefs(nil)
	if len(refs.GetRefs()) != 0 {
		t.Errorf("len(refs) = %d, want 0", len(refs.GetRefs()))
	}
}

func TestCollectPublishedCIDs(t *testing.T) {
	t.Parallel()

	ch := make(chan *routingv1.ListResponse, 3)
	ch <- &routingv1.ListResponse{RecordRef: &corev1.RecordRef{Cid: "cid1"}}
	ch <- &routingv1.ListResponse{RecordRef: &corev1.RecordRef{Cid: "cid2"}}
	ch <- &routingv1.ListResponse{RecordRef: nil} // defensive: skipped
	close(ch)

	got := collectPublishedCIDs(ch)
	if len(got) != 2 || !got["cid1"] || !got["cid2"] {
		t.Errorf("collectPublishedCIDs = %v, want {cid1, cid2}", got)
	}
}

func TestQueryToRPC_Negate(t *testing.T) {
	q := Query{Category: FilterSkill, Value: "nlp", Negate: true}
	rpc := q.toRPC()
	if rpc.GetType() != searchv1.RecordQueryType_RECORD_QUERY_TYPE_SKILL_NAME {
		t.Fatalf("unexpected type: %v", rpc.GetType())
	}
	if !rpc.GetNegate() {
		t.Error("expected Negate=true on rpc query")
	}
}

func TestQueryToRPC_Name(t *testing.T) {
	rpc := Query{Category: FilterName, Value: "web*"}.toRPC()
	if rpc.GetType() != searchv1.RecordQueryType_RECORD_QUERY_TYPE_NAME {
		t.Fatalf("expected NAME type, got %v", rpc.GetType())
	}
	if rpc.GetNegate() {
		t.Error("expected Negate=false by default")
	}
}
