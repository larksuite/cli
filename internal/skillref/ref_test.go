// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillref

import "testing"

func TestParseRoundTrip(t *testing.T) {
	for _, raw := range []string{
		"lark-doc",
		"lark-doc/references/lark-doc-fetch.md",
	} {
		got, err := Parse(raw)
		if err != nil {
			t.Fatalf("Parse(%q): %v", raw, err)
		}
		if got.String() != raw {
			t.Errorf("Parse(%q).String() = %q", raw, got.String())
		}
	}
}

func TestParseRejectsInvalidReferences(t *testing.T) {
	for _, raw := range []string{
		"",
		".",
		"..",
		"lark-doc/",
		"lark-doc/../secret",
		`lark-doc/references\x.md`,
		`lark-doc\references\x.md`,
	} {
		if _, err := Parse(raw); err == nil {
			t.Errorf("Parse(%q) unexpectedly succeeded", raw)
		}
	}
}
