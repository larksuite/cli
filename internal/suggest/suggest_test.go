// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package suggest

import (
	"slices"
	"testing"
)

func TestClosest_HallucinatedSharesPrefix(t *testing.T) {
	cmds := []string{
		"+cells-get", "+cells-set", "+cells-search", "+cells-replace",
		"+cells-clear", "+cells-merge", "+csv-get", "+chart-create",
		"+pivot-create", "+sheet-info",
	}
	// "+cells-find" is semantically +cells-search but lexically far; the shared
	// "+cells-" prefix should still surface the right family (incl. +cells-search).
	got := Closest("+cells-find", cmds, 6)
	if len(got) == 0 || len(got) > 6 {
		t.Fatalf("expected 1..6 suggestions, got %v", got)
	}
	if !slices.Contains(got, "+cells-search") {
		t.Errorf("expected +cells-search among suggestions, got %v", got)
	}
	for _, s := range got {
		if len(s) < 7 || s[:7] != "+cells-" {
			t.Errorf("suggestion %q does not share the +cells- prefix", s)
		}
	}
}

func TestClosest_TypoRanksExactNeighborFirst(t *testing.T) {
	got := Closest("+cell-get", []string{"+cells-get", "+cells-set", "+csv-get", "+sheet-info"}, 3)
	if len(got) == 0 || got[0] != "+cells-get" {
		t.Errorf("expected +cells-get first for typo +cell-get, got %v", got)
	}
}

func TestClosest_NoPlausibleMatch(t *testing.T) {
	if got := Closest("+zzzzzz", []string{"+cells-get", "+csv-get"}, 6); len(got) != 0 {
		t.Errorf("expected no suggestions for unrelated input, got %v", got)
	}
}

func TestClosest_CompoundSurfacesTrailingSegment(t *testing.T) {
	// `--sql-file` is welded from two real flags of `apps +db-execute`. Prefix
	// weighting alone surfaced only the leading half (--sql), and --file sits 4
	// edits away — past the budget — so the flag that does what the caller asked
	// for was dropped, pushing callers to inline SQL through the shell.
	flags := []string{"app-id", "as", "dry-run", "environment", "file", "format", "help", "jq", "json", "sql", "yes"}
	got := Closest("sql-file", flags, 3)
	for _, want := range []string{"sql", "file"} {
		if !slices.Contains(got, want) {
			t.Errorf("expected %q among suggestions for sql-file, got %v", want, got)
		}
	}
}

func TestClosest_CompoundKeepsPrefixRankingFirst(t *testing.T) {
	// Segment-exact candidates become plausible, they do not jump the queue:
	// ranking stays prefix-first, so the leading segment still leads.
	flags := []string{"file", "sql"}
	if got := Closest("sql-file", flags, 3); len(got) == 0 || got[0] != "sql" {
		t.Errorf("expected sql ranked first for sql-file, got %v", got)
	}
	if got := Closest("file-sql", flags, 3); len(got) == 0 || got[0] != "file" {
		t.Errorf("expected file ranked first for file-sql, got %v", got)
	}
}

func TestClosest_SegmentRescueDoesNotAdmitUnrelated(t *testing.T) {
	// Only exact segment hits are rescued; a hyphen in the typed name must not
	// turn every candidate into a plausible match.
	if got := Closest("sql-file", []string{"table", "environment"}, 6); len(got) != 0 {
		t.Errorf("expected no suggestions for unrelated candidates, got %v", got)
	}
}

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "abc", 3},
		{"abc", "", 3},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"cell-get", "cells-get", 1},
		{"--query", "--find", 5},
		{"飞书", "飞书", 0}, // rune-aware: multi-byte equal
		{"飞书", "飞s", 1}, // one rune substitution, not byte count
	}
	for _, c := range cases {
		if d := Levenshtein(c.a, c.b); d != c.want {
			t.Errorf("Levenshtein(%q,%q) = %d, want %d", c.a, c.b, d, c.want)
		}
	}
}

func TestSharedPrefixLen(t *testing.T) {
	if got := sharedPrefixLen("+cells-find", "+cells-search"); got != 7 {
		t.Errorf("sharedPrefixLen = %d, want 7", got)
	}
	if got := sharedPrefixLen("abc", "xyz"); got != 0 {
		t.Errorf("sharedPrefixLen = %d, want 0", got)
	}
}
