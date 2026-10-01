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

// addRecord appends one streamed record. The offset is not moved here; it
// advances once per page in finishPage.
func (p *pageState) addRecord(r *dirclient.RecordSummary) {
	p.records = append(p.records, r)
}

// finishPage completes a page fetch. The offset advances by rawCount — the
// number of raw records the server sent — rather than the number of records
// added, so a record dropped during extraction does not make the next page
// start short and duplicate a row.
func (p *pageState) finishPage(rawCount uint32, exhausted bool) {
	p.offset += rawCount
	p.exhausted = exhausted
}
