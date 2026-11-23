// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package domaincontract

import "testing"

func TestParseChangedGoPaths(t *testing.T) {
	raw := []byte("M\x00changed.go\x00R100\x00old.go\x00renamed.go\x00C100\x00source.go\x00copied.go\x00A\x00README.md\x00")
	got, err := parseChangedGoPaths(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []changedGoPath{
		{Old: "changed.go", New: "changed.go"},
		{Old: "old.go", New: "renamed.go"},
		{Old: "copied.go", New: "copied.go"},
	}
	if len(got) != len(want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("paths = %#v, want %#v", got, want)
		}
	}
}

func TestParseChangedGoPathsRejectsTruncatedRename(t *testing.T) {
	if _, err := parseChangedGoPaths([]byte("R100\x00old.go\x00")); err == nil {
		t.Fatal("expected truncated rename error")
	}
}

func TestParseAddedLineRanges(t *testing.T) {
	patch := []byte(`diff --git a/x.go b/x.go
index 1111111..2222222 100644
--- a/x.go
+++ b/x.go
@@ -2,0 +3,2 @@
+first
+second
@@ -10 +12 @@
-old
+new
@@ -20 +21,0 @@
-deleted
`)
	got, err := parseAddedLineRanges(patch)
	if err != nil {
		t.Fatal(err)
	}
	want := []addedLineRange{{Start: 3, End: 4}, {Start: 12, End: 12}}
	if len(got) != len(want) {
		t.Fatalf("ranges = %#v, want %#v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("ranges = %#v, want %#v", got, want)
		}
	}
}

func TestParseAddedLineRangesRejectsUnknownHunk(t *testing.T) {
	if _, err := parseAddedLineRanges([]byte("@@@ unsupported @@@\n")); err == nil {
		t.Fatal("expected unsupported hunk error")
	}
}

func TestFirstAddedLineInSpan(t *testing.T) {
	ranges := []addedLineRange{{Start: 5, End: 7}, {Start: 10, End: 10}}
	tests := []struct {
		start, end int
		line       int
		ok         bool
	}{
		{start: 1, end: 4, ok: false},
		{start: 4, end: 6, line: 5, ok: true},
		{start: 6, end: 9, line: 6, ok: true},
		{start: 8, end: 12, line: 10, ok: true},
	}
	for _, tc := range tests {
		line, ok := firstAddedLineInSpan(ranges, tc.start, tc.end)
		if line != tc.line || ok != tc.ok {
			t.Errorf(
				"firstAddedLineInSpan(%d, %d) = (%d, %v), want (%d, %v)",
				tc.start,
				tc.end,
				line,
				ok,
				tc.line,
				tc.ok,
			)
		}
	}
}
