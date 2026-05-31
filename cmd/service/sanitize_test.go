// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"strings"
	"testing"
)

func TestSanitizeOptionDesc(t *testing.T) {
	cases := map[string]string{
		"":                         "",
		"以 open_id 标识用户":           "以 open_id 标识用户",
		"中文。English second clause": "中文",         // first clause only (。)
		"head；tail":                "head",       // first clause (；)
		"line one\nline two":       "line one",   // first clause (newline)
		"  spaced   out  ":         "spaced out", // whitespace collapsed
		"see [飞书后台](https://x/admin) 详情": "see 飞书后台 详情", // markdown link -> text, url dropped
	}
	for in, want := range cases {
		if got := sanitizeOptionDesc(in); got != want {
			t.Errorf("sanitizeOptionDesc(%q) = %q, want %q", in, got, want)
		}
	}

	// Truncation: a long single clause is cut to 40 runes with an ellipsis,
	// rune-safe (no split mid-character).
	long := strings.Repeat("文", 60)
	got := sanitizeOptionDesc(long)
	if r := []rune(got); len(r) != 40 || !strings.HasSuffix(got, "...") {
		t.Errorf("truncation = %q (%d runes), want 40 runes ending in ...", got, len(r))
	}
}
