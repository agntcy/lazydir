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
	loading    bool                       //nolint:unused // wired up by later tasks (loadNextPage/startQuery)
}

// reset clears all accumulated state for a new query (filter/search change).
func (p *pageState) reset() {
	p.records = nil
	p.total = 0
	p.totalKnown = false
	p.offset = 0
	p.exhausted = false
}

// appendPage adds a fetched page and advances the offset.
func (p *pageState) appendPage(recs []*dirclient.RecordSummary, exhausted bool) {
	p.records = append(p.records, recs...)
	p.offset += uint32(len(recs))
	p.exhausted = exhausted
}
