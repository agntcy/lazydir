// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"testing"

	"github.com/agntcy/lazydir/internal/dirclient"
)

func rec(cid string) *dirclient.RecordSummary { return &dirclient.RecordSummary{CID: cid} }

func TestPageState_AddRecordDoesNotMoveOffset(t *testing.T) {
	var p pageState
	p.addRecord(rec("a"))
	p.addRecord(rec("b"))
	if len(p.records) != 2 || p.offset != 0 || p.exhausted {
		t.Fatalf("after addRecord: offset=%d len=%d exhausted=%v", p.offset, len(p.records), p.exhausted)
	}
	if p.records[0].CID != "a" || p.records[1].CID != "b" {
		t.Fatalf("records out of order: %q, %q", p.records[0].CID, p.records[1].CID)
	}
}

func TestPageState_FinishPageAdvancesByRawCount(t *testing.T) {
	var p pageState
	p.addRecord(rec("a"))
	p.addRecord(rec("b"))
	p.finishPage(2, false)
	if p.offset != 2 || len(p.records) != 2 || p.exhausted {
		t.Fatalf("after first page: offset=%d len=%d exhausted=%v", p.offset, len(p.records), p.exhausted)
	}
	// Server sent 3 raw records but one was dropped by extractSummary: the
	// offset must still advance by 3 so the next page does not overlap.
	p.addRecord(rec("c"))
	p.addRecord(rec("d"))
	p.finishPage(3, true)
	if p.offset != 5 || len(p.records) != 4 || !p.exhausted {
		t.Fatalf("after second page: offset=%d len=%d exhausted=%v", p.offset, len(p.records), p.exhausted)
	}
}

func TestPageState_ResetClears(t *testing.T) {
	var p pageState
	p.addRecord(rec("a"))
	p.finishPage(5, true)
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
	for i := range n {
		app.state.page.addRecord(rec(string(rune('a' + i))))
	}
	app.rebuildRecordRows()
	app.state.recordCursor = cursor
	app.state.page.reset()
	return app
}

func TestCompleteQueryPage_EmptyResultClearsStaleRows(t *testing.T) {
	app := staleApp(3, 2)
	app.completeQueryPage(nil, 0, true)
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

func TestCompleteQueryPage_BufferedRefreshKeepsCursor(t *testing.T) {
	const k = 5
	app := staleApp(10, k)
	var buffered []*dirclient.RecordSummary
	for i := range 10 {
		buffered = append(buffered, rec(string(rune('a'+i))))
	}
	app.completeQueryPage(buffered, 10, false)
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
