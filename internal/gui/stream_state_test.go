// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import "testing"

// TestMarkStreamingDoesNotRegress guards the fix for the sync-indicator race:
// when every record fits in the first page, OnFirstPage and OnDone fire almost
// together and gocui's async g.Update can run them out of order. If OnDone
// (streamDone) lands first, OnFirstPage must NOT drag the state back to
// streamStreaming — otherwise the records panel stays "streaming" forever and
// the "[⟳ N ago]" indicator (gated on streamDone) never appears.
func TestMarkStreamingDoesNotRegress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		from streamState
		want streamState
	}{
		{"loading advances to streaming", streamLoading, streamStreaming},
		{"done is preserved", streamDone, streamDone},
		{"errored is preserved", streamErrored, streamErrored},
		{"already streaming stays streaming", streamStreaming, streamStreaming},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app := &Gui{}
			app.state.stream = tt.from
			app.markStreaming()
			if app.state.stream != tt.want {
				t.Errorf("markStreaming from %d: got %d, want %d", tt.from, app.state.stream, tt.want)
			}
		})
	}
}
