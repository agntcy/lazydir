// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package oasf

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sdkschema "github.com/agntcy/oasf-sdk/pkg/schema"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	taxonomy := map[string]sdkschema.TaxonomyItem{
		"natural_language": {
			ID:          1,
			Name:        "natural_language",
			Caption:     "Natural Language",
			Description: "Natural language processing skills.",
			Classes: map[string]sdkschema.TaxonomyItem{
				"translation": {
					ID:          10,
					Name:        "translation",
					Caption:     "Translation",
					Description: "Translation between languages.",
				},
			},
		},
		"code_generation": {
			ID:          2,
			Name:        "code_generation",
			Caption:     "Code Generation",
			Description: "Code generation skills.",
		},
	}

	versions := sdkschema.VersionsResponse{
		Default: sdkschema.VersionInfo{SchemaVersion: "1.0.0"},
		Versions: []sdkschema.VersionInfo{
			{SchemaVersion: "1.0.0"},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/versions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(versions)
	})
	mux.HandleFunc("/api/1.0.0/skill_categories", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(taxonomy)
	})
	mux.HandleFunc("/api/1.0.0/domain_categories", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]sdkschema.TaxonomyItem{})
	})
	mux.HandleFunc("/api/1.0.0/module_categories", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]sdkschema.TaxonomyItem{})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv
}

func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()

	c, err := NewClient(Config{ServerAddress: serverURL, Timeout: 5})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	return c
}

func TestNewClient_EmptyAddress(t *testing.T) {
	t.Parallel()

	_, err := NewClient(Config{})
	if err == nil {
		t.Fatal("expected error for empty address")
	}
}

func TestNewClient_Valid(t *testing.T) {
	t.Parallel()

	c, err := NewClient(Config{ServerAddress: "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.ServerAddress() != "https://example.com" {
		t.Errorf("ServerAddress() = %q", c.ServerAddress())
	}
}

func TestPing(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	c := newTestClient(t, srv.URL)

	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestPing_Unreachable(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, "http://127.0.0.1:1")
	if err := c.Ping(context.Background()); err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestFetch_Found(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	c := newTestClient(t, srv.URL)
	ctx := context.Background()

	info, err := c.Fetch(ctx, ClassTypeSkill, "translation", "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if info.Name != "translation" {
		t.Errorf("Name = %q, want translation", info.Name)
	}
	if info.Caption != "Translation" {
		t.Errorf("Caption = %q, want Translation", info.Caption)
	}
	if info.ID != 10 {
		t.Errorf("ID = %d, want 10", info.ID)
	}
	if len(info.Ancestors) != 1 || info.Ancestors[0].Name != "natural_language" {
		t.Errorf("Ancestors = %+v, want [natural_language]", info.Ancestors)
	}
}

func TestFetch_NotFound(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	c := newTestClient(t, srv.URL)

	_, err := c.Fetch(context.Background(), ClassTypeSkill, "nonexistent", "")
	if err == nil {
		t.Fatal("expected error for missing class")
	}
}

func TestFetch_Cached(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	c := newTestClient(t, srv.URL)
	ctx := context.Background()

	info1, err := c.Fetch(ctx, ClassTypeSkill, "code_generation", "")
	if err != nil {
		t.Fatalf("first Fetch: %v", err)
	}

	info2, err := c.Fetch(ctx, ClassTypeSkill, "code_generation", "")
	if err != nil {
		t.Fatalf("second Fetch: %v", err)
	}
	if info1 != info2 {
		t.Error("expected cached result to be the same pointer")
	}
}

func TestFetch_EmptyDescription(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	c := newTestClient(t, srv.URL)

	// Override test server to return item with empty description.
	info, err := c.Fetch(context.Background(), ClassTypeSkill, "code_generation", "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if info.Description != "Code generation skills." {
		t.Errorf("Description = %q", info.Description)
	}
}

func TestFetchAll(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	c := newTestClient(t, srv.URL)

	entries, err := c.FetchAll(context.Background(), ClassTypeSkill, "")
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries (natural_language, translation, code_generation), got %d", len(entries))
	}
	if e, ok := entries["translation"]; !ok || e.ID != 10 {
		t.Errorf("translation entry = %+v", entries["translation"])
	}
}

func TestFindItemWithPath_Root(t *testing.T) {
	t.Parallel()

	items := map[string]sdkschema.TaxonomyItem{
		"a": {ID: 1, Name: "a", Caption: "A"},
	}

	item, ancestors, ok := findItemWithPath(items, "a")
	if !ok {
		t.Fatal("expected to find item")
	}
	if item.Name != "a" || item.ID != 1 {
		t.Errorf("item = %+v", item)
	}
	if len(ancestors) != 0 {
		t.Errorf("ancestors = %+v, want empty", ancestors)
	}
}

func TestFindItemWithPath_Nested(t *testing.T) {
	t.Parallel()

	items := map[string]sdkschema.TaxonomyItem{
		"parent": {
			ID: 1, Name: "parent", Caption: "Parent",
			Classes: map[string]sdkschema.TaxonomyItem{
				"child": {
					ID: 2, Name: "child", Caption: "Child",
					Classes: map[string]sdkschema.TaxonomyItem{
						"grandchild": {ID: 3, Name: "grandchild", Caption: "Grandchild"},
					},
				},
			},
		},
	}

	item, ancestors, ok := findItemWithPath(items, "grandchild")
	if !ok {
		t.Fatal("expected to find grandchild")
	}
	if item.ID != 3 {
		t.Errorf("item.ID = %d, want 3", item.ID)
	}
	if len(ancestors) != 2 {
		t.Fatalf("ancestors len = %d, want 2", len(ancestors))
	}
	if ancestors[0].Name != "parent" || ancestors[1].Name != "child" {
		t.Errorf("ancestors = %+v", ancestors)
	}
}

func TestFindItemWithPath_NotFound(t *testing.T) {
	t.Parallel()

	items := map[string]sdkschema.TaxonomyItem{
		"a": {ID: 1, Name: "a"},
	}

	_, _, ok := findItemWithPath(items, "missing")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestCollectEntries(t *testing.T) {
	t.Parallel()

	items := map[string]sdkschema.TaxonomyItem{
		"root": {
			ID: 1, Name: "root", Caption: "Root",
			Classes: map[string]sdkschema.TaxonomyItem{
				"leaf": {ID: 2, Name: "leaf", Caption: "Leaf"},
			},
		},
	}

	out := map[string]ClassEntry{}
	collectEntries(items, out)

	if len(out) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(out))
	}
	if out["root"].ID != 1 || out["leaf"].ID != 2 {
		t.Errorf("entries = %+v", out)
	}
}
