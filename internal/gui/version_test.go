// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import "testing"

func TestCompareVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b string
		want int // >0 means a>b, <0 means a<b, 0 means equal
	}{
		{"equal", "1.0.0", "1.0.0", 0},
		{"equal with v prefix", "v1.0.0", "1.0.0", 0},
		{"major greater", "2.0.0", "1.0.0", 1},
		{"major less", "1.0.0", "2.0.0", -1},
		{"minor greater", "1.2.0", "1.1.0", 1},
		{"patch greater", "1.0.2", "1.0.1", 1},
		{"different lengths", "1.0.0.1", "1.0.0", 1},
		{"both prefixed", "v2.1.0", "v1.9.0", 1},
		{"lexicographic fallback", "alpha", "beta", -1},
		{"mixed numeric lexicographic", "1.0.0-rc1", "1.0.0-rc2", -1},
		{"empty strings", "", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := compareVersions(tt.a, tt.b)
			switch {
			case tt.want > 0 && got <= 0:
				t.Errorf("compareVersions(%q, %q) = %d, want >0", tt.a, tt.b, got)
			case tt.want < 0 && got >= 0:
				t.Errorf("compareVersions(%q, %q) = %d, want <0", tt.a, tt.b, got)
			case tt.want == 0 && got != 0:
				t.Errorf("compareVersions(%q, %q) = %d, want 0", tt.a, tt.b, got)
			}
		})
	}
}
