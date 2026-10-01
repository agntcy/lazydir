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
