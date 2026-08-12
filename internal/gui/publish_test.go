// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"testing"

	"github.com/agntcy/lazydir/internal/dirclient"
)

func TestMarkPublished(t *testing.T) {
	t.Parallel()

	records := []*dirclient.RecordSummary{
		{CID: "a", Published: true}, // absent from set -> cleared
		{CID: "b"},                  // present -> set
		{CID: "c"},                  // absent -> stays false
	}
	markPublished(records, map[string]bool{"b": true})

	if records[0].Published {
		t.Errorf("record a: Published = true, want false (absent from set)")
	}
	if !records[1].Published {
		t.Errorf("record b: Published = false, want true")
	}
	if records[2].Published {
		t.Errorf("record c: Published = true, want false")
	}
}

func TestSetRecordPublished(t *testing.T) {
	t.Parallel()

	app := &Gui{}
	app.state.page.records = []*dirclient.RecordSummary{
		{CID: "a"},
		{CID: "b"},
	}

	app.setRecordPublished("b", true)
	if app.state.page.records[0].Published {
		t.Errorf("record a: Published = true, want false")
	}
	if !app.state.page.records[1].Published {
		t.Errorf("record b: Published = false, want true")
	}

	app.setRecordPublished("b", false)
	if app.state.page.records[1].Published {
		t.Errorf("record b after unpublish: Published = true, want false")
	}
}

// TestPublishOverrideSurvivesStaleEnrichment reproduces the race where a
// publish-enrichment snapshot taken before the user's publish reached the
// server's routing index would otherwise clobber the optimistic Published=true
// back to false. The override must protect the just-published record.
func TestPublishOverrideSurvivesStaleEnrichment(t *testing.T) {
	t.Parallel()

	app := &Gui{}
	app.state.page.records = []*dirclient.RecordSummary{{CID: "x"}}
	app.setRecordPublished("x", true) // optimistic publish

	// Stale enrichment: the server's routing index has not caught up, so the
	// returned set does not yet include "x".
	staleSet := map[string]bool{}
	markPublished(app.state.page.records, staleSet)
	app.applyPublishOverrides(staleSet)

	if !app.state.page.records[0].Published {
		t.Error("stale enrichment clobbered optimistic publish; want Published=true")
	}
}

// TestUnpublishOverrideSurvivesStaleEnrichment is the symmetric case: an
// enrichment snapshot that still lists the record as published must not undo
// the user's optimistic unpublish.
func TestUnpublishOverrideSurvivesStaleEnrichment(t *testing.T) {
	t.Parallel()

	app := &Gui{}
	app.state.page.records = []*dirclient.RecordSummary{{CID: "x", Published: true}}
	app.setRecordPublished("x", false) // optimistic unpublish

	staleSet := map[string]bool{"x": true} // index still shows it published
	markPublished(app.state.page.records, staleSet)
	app.applyPublishOverrides(staleSet)

	if app.state.page.records[0].Published {
		t.Error("stale enrichment clobbered optimistic unpublish; want Published=false")
	}
}

// TestPublishOverrideClearsWhenServerAgrees verifies the override is self-
// healing: once an enrichment set agrees with the optimistic value, the
// override is dropped so future enrichments are authoritative again.
func TestPublishOverrideClearsWhenServerAgrees(t *testing.T) {
	t.Parallel()

	app := &Gui{}
	app.state.page.records = []*dirclient.RecordSummary{{CID: "x"}}
	app.setRecordPublished("x", true)

	set := map[string]bool{"x": true} // server has caught up
	markPublished(app.state.page.records, set)
	app.applyPublishOverrides(set)

	if !app.state.page.records[0].Published {
		t.Error("record x: Published = false, want true")
	}
	if _, ok := app.state.publishOverrides["x"]; ok {
		t.Error("override for x should be cleared once the server set agrees")
	}
}
