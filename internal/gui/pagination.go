// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import "github.com/agntcy/lazydir/internal/dirclient"

// pageState holds the server-side pagination state for the records list.
// It replaces the former fullCache/records/filteredRecords pipeline.
type pageState struct {
	records    []*dirclient.RecordSummary // accumulated across appended pages
	total      uint32                     // total matches for the current query
	totalKnown bool                       // false when CountRecords is unavailable
	offset     uint32                     // next offset to request
	exhausted  bool                       // no further pages
	loading    bool                       // a page fetch is in flight
}

// reset clears all accumulated state for a new query (filter/search change).
func (p *pageState) reset() {
	p.records = nil
	p.total = 0
	p.totalKnown = false
	p.offset = 0
	p.exhausted = false
	p.loading = false
}

// addSummaryPage applies one ListRecordSummaries page: its records are
// appended, the offset advances past them, and the server's total and
// has-more flag replace the previous ones.
func (p *pageState) addSummaryPage(page *dirclient.SummaryPage) {
	p.records = append(p.records, page.Records...)
	p.offset += uint32(len(page.Records)) //nolint:gosec // a page holds at most pageSize records
	p.exhausted = !page.HasMore
	p.total = page.Total
	p.totalKnown = true
}
