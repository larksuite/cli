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
