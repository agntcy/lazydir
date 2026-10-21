// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/agntcy/lazydir/internal/dirclient"
)

func named(cid, name string) *dirclient.RecordSummary {
	return &dirclient.RecordSummary{CID: cid, Name: name, Version: "v1"}
}

// selectionApp returns a Gui with records a..e loaded on the current page.
func selectionApp() *Gui {
	app := &Gui{}
	app.state.page.addSummaryPage(&dirclient.SummaryPage{Records: recs(5), HasMore: true})
	app.rebuildRecordRows()
	return app
}

func TestToggleSelected(t *testing.T) {
	app := selectionApp()
	app.toggleSelected(app.state.page.records[1])
	if !app.isSelected("b") || len(app.state.selection) != 1 {
		t.Fatalf("expected b selected, got %v", app.state.selection)
	}
	app.toggleSelected(app.state.page.records[1])
	if app.isSelected("b") || len(app.state.selection) != 0 {
		t.Fatalf("expected b unselected, got %v", app.state.selection)
	}
}

func TestSelectAllLoaded_TogglesOff(t *testing.T) {
	app := selectionApp()
	app.toggleSelected(app.state.page.records[0])
	app.selectAllLoaded()
	if len(app.state.selection) != 5 {
		t.Fatalf("expected all 5 loaded records selected, got %d", len(app.state.selection))
	}
	app.selectAllLoaded()
	if len(app.state.selection) != 0 {
		t.Fatalf("expected selection cleared when all were selected, got %d", len(app.state.selection))
	}
}

func TestSelectedRecords_PageOrderThenUnloaded(t *testing.T) {
	app := selectionApp()
	app.toggleSelected(app.state.page.records[3])
	app.toggleSelected(app.state.page.records[0])
	// A selected record that is no longer loaded (e.g. after a refresh)
	// still takes part, after the loaded ones.
	app.state.selection["zz"] = named("zz", "gone")

	var cids []string
	for _, r := range app.selectedRecords() {
		cids = append(cids, r.CID)
	}
	if got := strings.Join(cids, ","); got != "a,d,zz" {
		t.Fatalf("selectedRecords order = %s, want a,d,zz", got)
	}
}

func TestSelectedRecords_PrefersLiveRecord(t *testing.T) {
	app := selectionApp()
	app.toggleSelected(app.state.page.records[0])
	app.state.page.records[0].Published = true
	if r := app.selectedRecords()[0]; !r.Published {
		t.Fatal("expected the live page record, not the selection snapshot")
	}
}

func TestQueryKey(t *testing.T) {
	a := []dirclient.Query{{Category: dirclient.FilterSkill, Value: "x"}, {Category: dirclient.FilterAuthor, Value: "y", Negate: true}}
	b := []dirclient.Query{a[1], a[0]}
	if queryKey("srv", a) != queryKey("srv", b) {
		t.Error("query order must not change the key")
	}
	if queryKey("srv", a) == queryKey("other", a) {
		t.Error("server must change the key")
	}
	neg := []dirclient.Query{{Category: dirclient.FilterSkill, Value: "x", Negate: true}, a[1]}
	if queryKey("srv", a) == queryKey("srv", neg) {
		t.Error("negation must change the key")
	}
}

func TestSyncSelectionKey(t *testing.T) {
	app := selectionApp()
	app.syncSelectionKey("k1")
	app.toggleSelected(app.state.page.records[0])

	app.syncSelectionKey("k1")
	if len(app.state.selection) != 1 {
		t.Fatal("same query (refresh) must keep the selection")
	}
	app.syncSelectionKey("k2")
	if len(app.state.selection) != 0 {
		t.Fatal("a different query must clear the selection")
	}
}

func TestPlanBatch(t *testing.T) {
	local := named("a", "local")
	published := named("b", "pub")
	published.Published = true
	syncing := named("c", "sync")
	status := func(cid string) dirclient.RecordStatus {
		if cid == "c" {
			return dirclient.StatusSyncing
		}
		return dirclient.StatusLocal
	}
	all := []*dirclient.RecordSummary{local, published, syncing}

	tests := []struct {
		kind        batchKind
		wantAct     string
		wantSkipped int
	}{
		{batchDelete, "a,b", 1},
		{batchPublish, "a", 2},
		{batchUnpublish, "b", 2},
	}
	for _, tt := range tests {
		act, skipped := planBatch(tt.kind, all, status)
		var cids []string
		for _, r := range act {
			cids = append(cids, r.CID)
		}
		if strings.Join(cids, ",") != tt.wantAct || skipped != tt.wantSkipped {
			t.Errorf("%s: act=%v skipped=%d, want %s skipped=%d", tt.kind.verb(), cids, skipped, tt.wantAct, tt.wantSkipped)
		}
	}
}

func TestBatchConfirmBody(t *testing.T) {
	recs := []*dirclient.RecordSummary{named("a", "one"), named("b", "two")}
	body := batchConfirmBody(batchPublish, recs, 3)
	for _, want := range []string{"Publish 2 records?", "one v1", "two v1", "3 skipped"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	if body := batchConfirmBody(batchDelete, recs, 0); strings.Contains(body, "skipped") {
		t.Errorf("unexpected skipped line:\n%s", body)
	}
}

func TestBatchConfirmBody_TruncatesList(t *testing.T) {
	body := batchConfirmBody(batchDelete, recs8(), 0)
	if !strings.Contains(body, "Delete 8 records?") || !strings.Contains(body, "… and 3 more") {
		t.Fatalf("unexpected body:\n%s", body)
	}
}

func recs8() []*dirclient.RecordSummary {
	out := recs(8)
	for _, r := range out {
		r.Name = "rec-" + r.CID
	}
	return out
}

func TestDeleteEach_ContinuesPastFailures(t *testing.T) {
	recs := []*dirclient.RecordSummary{named("a", "one"), named("b", "two"), named("c", "three")}
	var progress []int
	del := func(_ context.Context, cid string) error {
		if cid == "b" {
			return errors.New("boom")
		}
		return nil
	}
	deleted, failures := deleteEach(context.Background(), recs, del, func(done, _ int) { progress = append(progress, done) })

	if strings.Join(deleted, ",") != "a,c" {
		t.Errorf("deleted = %v, want [a c]", deleted)
	}
	if len(failures) != 1 || failures[0].record.CID != "b" || failures[0].err.Error() != "boom" {
		t.Errorf("failures = %+v", failures)
	}
	if len(progress) != 3 || progress[2] != 3 {
		t.Errorf("progress = %v, want 1,2,3", progress)
	}
}

func TestBatchKindProgressing(t *testing.T) {
	for kind, want := range map[batchKind]string{batchDelete: "deleting", batchPublish: "publishing", batchUnpublish: "unpublishing"} {
		if got := kind.progressing(); got != want {
			t.Errorf("%s.progressing() = %q, want %q", kind.verb(), got, want)
		}
	}
}

func TestBatchFailureText(t *testing.T) {
	text := batchFailureText(batchDelete, 3, []batchFailure{{record: named("b", "two"), err: errors.New("boom")}})
	for _, want := range []string{"Deleted 2 of 3 records", "two v1: boom"} {
		if !strings.Contains(text, want) {
			t.Errorf("text missing %q:\n%s", want, text)
		}
	}
}

func TestRecordsTitle_SelectedCount(t *testing.T) {
	app := selectionApp()
	app.toggleSelected(app.state.page.records[0])
	if title := app.recordsTitle(); !strings.Contains(title, "[1 selected]") {
		t.Fatalf("title = %q, want selected count", title)
	}
}

func TestOptionsBarText_SelectionAndProgress(t *testing.T) {
	got := optionsBarText(viewRecords, 0, barStatus{selected: 2, progress: "deleting 1/2"})
	if !strings.Contains(got, "2 selected") || !strings.Contains(got, "deleting 1/2") {
		t.Fatalf("options bar = %q", got)
	}
}

func TestRecordRowLine(t *testing.T) {
	plain := recordRowLine("", false, 6, 20, "agent", "v1")
	if plain != "  agent   v1" {
		t.Errorf("plain row = %q", plain)
	}

	sel := recordRowLine("", true, 6, 20, "agent", "v1")
	want := ansiBold + ansiGreen + selectedMark + ansiDefaultFg + " " + fmt.Sprintf("%-18s", "agent   v1") + ansiReset
	if sel != want {
		t.Errorf("selected row = %q, want %q", sel, want)
	}

	// On a colored row the line color is re-applied after the green marker.
	pub := recordRowLine("\033[32m", true, 6, 20, "agent", "v1")
	if !strings.HasPrefix(pub, "\033[32m"+ansiBold+ansiGreen+selectedMark+ansiDefaultFg+"\033[32m ") {
		t.Errorf("colored selected row = %q", pub)
	}

	// Plain text is aligned the same whether or not the row is selected.
	if strings.Index(plain, "agent") != rowGutterWidth {
		t.Errorf("text should start after the %d-column gutter: %q", rowGutterWidth, plain)
	}
}

// TestBatchProgress_LateUpdateAfterEnd guards against gocui delivering a
// progress update after the batch's completion callback: it must not leave
// the batch looking busy.
func TestBatchProgress_LateUpdateAfterEnd(t *testing.T) {
	app := &Gui{}
	id := app.beginBatch("deleting 0/2")
	app.updateBatchProgress(id, "deleting 1/2")
	if app.state.batchRunning != id || app.state.batchProgress != "deleting 1/2" {
		t.Fatalf("running batch: running=%d progress=%q", app.state.batchRunning, app.state.batchProgress)
	}

	app.endBatch(id)
	app.updateBatchProgress(id, "deleting 2/2") // arrives late
	if app.state.batchRunning != 0 || app.state.batchProgress != "" {
		t.Fatalf("late update revived a finished batch: running=%d progress=%q", app.state.batchRunning, app.state.batchProgress)
	}

	next := app.beginBatch("publishing 0/1")
	app.endBatch(id) // stale end from the previous batch
	if app.state.batchRunning != next || app.state.batchProgress != "publishing 0/1" {
		t.Fatalf("stale end stopped the next batch: running=%d progress=%q", app.state.batchRunning, app.state.batchProgress)
	}
}
