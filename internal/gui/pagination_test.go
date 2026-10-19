// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"testing"

	"github.com/agntcy/lazydir/internal/dirclient"
)

func rec(cid string) *dirclient.RecordSummary { return &dirclient.RecordSummary{CID: cid} }

func TestPageState_ResetClears(t *testing.T) {
	var p pageState
	p.addSummaryPage(&dirclient.SummaryPage{Records: []*dirclient.RecordSummary{rec("a")}})
	p.total, p.totalKnown, p.loading = 9, true, true
	p.reset()
	if len(p.records) != 0 || p.offset != 0 || p.exhausted || p.loading {
		t.Fatalf("reset did not clear: %+v", p)
	}
	if p.total != 0 || p.totalKnown {
		t.Fatalf("reset did not clear total: %+v", p)
	}
}

// staleApp returns a Gui showing n rows from a previous query, then reset for
// a new one (as startQuery does), with the cursor on row cursor.
func staleApp(n, cursor int) *Gui {
	app := &Gui{}
	app.state.page.addSummaryPage(&dirclient.SummaryPage{Records: recs(n)})
	app.rebuildRecordRows()
	app.state.recordCursor = cursor
	app.state.page.reset()
	return app
}

// recs returns n records with single-letter CIDs.
func recs(n int) []*dirclient.RecordSummary {
	out := make([]*dirclient.RecordSummary, 0, n)
	for i := range n {
		out = append(out, rec(string(rune('a'+i))))
	}
	return out
}

func TestFirstPage_EmptyResultClearsStaleRows(t *testing.T) {
	app := staleApp(3, 2)
	app.state.page.addSummaryPage(&dirclient.SummaryPage{})
	app.rebuildRecordRows()
	if n := len(app.state.recordDisplayRows); n != 0 {
		t.Fatalf("expected 0 display rows after an empty result, got %d", n)
	}
	if app.state.recordCursor != 0 {
		t.Fatalf("expected cursor 0, got %d", app.state.recordCursor)
	}
	if !app.state.page.exhausted {
		t.Fatal("expected page exhausted")
	}
}

func TestFirstPage_SilentRefreshKeepsCursor(t *testing.T) {
	const k = 5
	app := staleApp(10, k)
	app.state.page.addSummaryPage(&dirclient.SummaryPage{Records: recs(10), HasMore: true})
	app.rebuildRecordRows()
	if app.state.recordCursor != k {
		t.Fatalf("expected cursor kept at %d, got %d", k, app.state.recordCursor)
	}
	if n := len(app.state.recordDisplayRows); n != 10 {
		t.Fatalf("expected 10 display rows, got %d", n)
	}
	if app.state.page.offset != 10 || app.state.page.exhausted {
		t.Fatalf("unexpected page state: offset=%d exhausted=%v", app.state.page.offset, app.state.page.exhausted)
	}
}

func TestPageState_AddSummaryPage(t *testing.T) {
	var p pageState
	p.addSummaryPage(&dirclient.SummaryPage{Records: []*dirclient.RecordSummary{rec("a"), rec("b")}, HasMore: true, Total: 3})
	if p.offset != 2 || len(p.records) != 2 || p.exhausted || !p.totalKnown || p.total != 3 {
		t.Fatalf("after first page: %+v", p)
	}
	p.addSummaryPage(&dirclient.SummaryPage{Records: []*dirclient.RecordSummary{rec("c")}, Total: 3})
	if p.offset != 3 || len(p.records) != 3 || !p.exhausted || p.records[2].CID != "c" {
		t.Fatalf("after last page: %+v", p)
	}
}

func TestPageState_RemoveRecordShiftsOffsetAndTotal(t *testing.T) {
	var p pageState
	p.addSummaryPage(&dirclient.SummaryPage{Records: recs(3), HasMore: true, Total: 10})
	p.removeRecord("b")
	if p.offset != 2 || p.total != 9 || len(p.records) != 2 {
		t.Fatalf("after removing a loaded record: offset=%d total=%d len=%d", p.offset, p.total, len(p.records))
	}
	p.removeRecord("zz")
	if p.offset != 2 || p.total != 9 {
		t.Fatalf("removing an unloaded record changed paging: offset=%d total=%d", p.offset, p.total)
	}
}
