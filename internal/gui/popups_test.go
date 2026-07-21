// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import "testing"

// TestConfirmOptionRowOffset guards against the off-by-one that highlighted the
// second option (cancel) instead of the first (confirm) in the confirm popup.
// Options are rendered immediately after the body's terminating newline, so the
// first option lives on row wrappedLineCount(body) — never one row further.
func TestConfirmOptionRowOffset(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		viewW int
		want  int
	}{
		{name: "empty body", body: "", viewW: 40, want: 0},
		{name: "single line", body: "Delete record foo?", viewW: 40, want: 1},
		{name: "two lines", body: "line one\nline two", viewW: 40, want: 2},
		{name: "wrapping line", body: "0123456789", viewW: 5, want: 2},
		{name: "zero width fallback", body: "a\nb\nc", viewW: 0, want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := confirmOptionRowOffset(tt.body, tt.viewW); got != tt.want {
				t.Errorf("confirmOptionRowOffset(%q, %d) = %d, want %d",
					tt.body, tt.viewW, got, tt.want)
			}
		})
	}
}
