// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdpolicy

import "testing"

// suggest is unexported, so the test lives in the same package.

func TestSuggestRisk(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"wrtie", "write"},
		{"WRITE", "write"},
		{"reed", "read"},
		{"rad", "read"},
		{"high-rik-write", "high-risk-write"},
		// "highrisk" is genuinely ambiguous between "write" and
		// "high-risk-write" — not testing it.
		{"", ""}, // empty input has no meaningful suggestion; the engine
		// routes the absent case to risk_not_annotated, not risk_invalid.
	}
	for _, c := range cases {
		got := suggestRisk(c.input)
		if got != c.want {
			t.Errorf("suggestRisk(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
