// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"testing"

	"github.com/agntcy/lazydir/internal/dirclient"
)

func rec(cid string) *dirclient.RecordSummary { return &dirclient.RecordSummary{CID: cid} }

func TestPageState_AppendAdvancesOffset(t *testing.T) {
	var p pageState
	p.appendPage([]*dirclient.RecordSummary{rec("a"), rec("b")}, false)
	if p.offset != 2 || len(p.records) != 2 || p.exhausted {
		t.Fatalf("after first page: offset=%d len=%d exhausted=%v", p.offset, len(p.records), p.exhausted)
	}
	p.appendPage([]*dirclient.RecordSummary{rec("c")}, true)
	if p.offset != 3 || len(p.records) != 3 || !p.exhausted {
		t.Fatalf("after second page: offset=%d len=%d exhausted=%v", p.offset, len(p.records), p.exhausted)
	}
}

func TestPageState_ResetClears(t *testing.T) {
	p := pageState{records: []*dirclient.RecordSummary{rec("a")}, offset: 5, exhausted: true, total: 9, totalKnown: true}
	p.reset()
	if len(p.records) != 0 || p.offset != 0 || p.exhausted {
		t.Fatalf("reset did not clear: %+v", p)
	}
	if p.total != 0 || p.totalKnown {
		t.Fatalf("reset did not clear total: %+v", p)
	}
}
