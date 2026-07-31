// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import "testing"

// TestConfirmSelectedLine guards the core invariant behind the highlight fix:
// the selected option lives on buffer line (bodyLines + cursor), computed from
// the buffer content alone — never from a wrapped, view-width-dependent offset.
// This is what SetHighlight uses, so the highlight can't drift onto the wrong
// row when the body wraps or the view is sized late.
func TestConfirmSelectedLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		body   string
		cursor int
		want   int
	}{
		{name: "empty body, first option", body: "", cursor: 0, want: 0},
		{name: "empty body, second option", body: "", cursor: 1, want: 1},
		{name: "single-line body, confirm", body: "Unpublish foo?", cursor: 0, want: 1},
		{name: "single-line body, cancel", body: "Unpublish foo?", cursor: 1, want: 2},
		{name: "long single-line body still one buffer line", body: "Unpublish unix-cli-best-practices 1.0.0?", cursor: 0, want: 1},
		{name: "two-line body, confirm", body: "line one\nline two", cursor: 0, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := confirmSelectedLine(tt.body, tt.cursor); got != tt.want {
				t.Errorf("confirmSelectedLine(%q, %d) = %d, want %d", tt.body, tt.cursor, got, tt.want)
			}
		})
	}
}

// TestConfirmContentWidth checks the option-row padding width: the widest of the
// body lines and the option labels (plus the leading space), so the highlighted
// row spans the popup like a selected row elsewhere.
func TestConfirmContentWidth(t *testing.T) {
	t.Parallel()

	opts := []menuOption{{label: "confirm"}, {label: "cancel"}}

	// Body wider than the options -> body width wins.
	if got := confirmContentWidth("Unpublish something-long?", opts); got != len("Unpublish something-long?") {
		t.Errorf("width = %d, want %d (body width)", got, len("Unpublish something-long?"))
	}
	// Options wider than the body -> option label + leading space wins.
	if got := confirmContentWidth("x?", opts); got != len("confirm")+1 {
		t.Errorf("width = %d, want %d (option width)", got, len("confirm")+1)
	}
}
