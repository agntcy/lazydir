// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package localdaemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTailLog_Chunks(t *testing.T) {
	t.Parallel()

	// 2000 lines of ~20 bytes span many tailChunk-sized reads.
	var many strings.Builder
	for i := range 2000 {
		fmt.Fprintf(&many, "line %04d padding\n", i)
	}
	longLine := strings.Repeat("x", 3*tailChunk)

	tests := []struct {
		name    string
		content string
		n       int
		want    string
	}{
		{name: "empty", content: "", n: 10, want: ""},
		{name: "fewer lines than n", content: "a\nb\n", n: 10, want: "a\nb"},
		{name: "no trailing newline", content: "a\nb\nc", n: 2, want: "b\nc"},
		{name: "trailing blank lines", content: "a\nb\nc\n\n\n", n: 2, want: "b\nc"},
		{name: "many chunks", content: many.String(), n: 3, want: "line 1997 padding\nline 1998 padding\nline 1999 padding"},
		{name: "line longer than a chunk", content: "first\n" + longLine + "\nlast\n", n: 2, want: longLine + "\nlast"},
		{name: "zero lines", content: "a\n", n: 0, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "daemon.log")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := TailLog(path, tt.n)
			if err != nil {
				t.Fatalf("TailLog: %v", err)
			}
			if got != tt.want {
				t.Errorf("TailLog(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestTailLog_Missing(t *testing.T) {
	t.Parallel()
	if _, err := TailLog(filepath.Join(t.TempDir(), "nope.log"), 10); err == nil {
		t.Error("TailLog on a missing file returned no error")
	}
}
