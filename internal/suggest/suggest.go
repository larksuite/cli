// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package suggest provides the shared "did you mean" primitives: a rune-aware
// Levenshtein edit distance and a prefix-weighted Closest ranker. It is the
// single home for these so cmd, cmd/event, and internal/cmdpolicy stop each
// carrying their own copy.
package suggest

import (
	"sort"
	"strings"
)

// Levenshtein computes the classic edit distance between two strings. It is
// rune-aware, so it is correct for multi-byte input.
func Levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)]
}

// Closest returns up to maxN of candidates that plausibly match typed, ranked
// by shared-prefix length (desc) then edit distance (asc), keeping only
// reasonably-close ones.
//
// Shared prefix is weighted first on purpose: hallucinated names are often
// semantically close but lexically far (e.g. "+cells-find" vs "+cells-search",
// "--with-styles" vs nothing close), where the common prefix is the strongest
// signal of intent that raw edit distance misses.
func Closest(typed string, candidates []string, maxN int) []string {
	type scored struct {
		name    string
		contain bool
		prefix  int
		dist    int
	}
	limit := editLimit(typed)
	ranked := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		p := sharedPrefixLen(typed, c)
		d := Levenshtein(typed, c)
		ct := containsSegment(typed, c)
		// Keep only plausible matches: a meaningful shared prefix, an edit
		// distance within budget, or one name containing the other (a missing
		// namespace prefix like "+block-list" vs "+base-block-list"). Drop
		// everything else so the hint stays short.
		if p >= 3 || d <= limit || ct {
			ranked = append(ranked, scored{name: c, contain: ct, prefix: p, dist: d})
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].contain != ranked[j].contain {
			return ranked[i].contain
		}
		if ranked[i].prefix != ranked[j].prefix {
			return ranked[i].prefix > ranked[j].prefix
		}
		if ranked[i].dist != ranked[j].dist {
			return ranked[i].dist < ranked[j].dist
		}
		return ranked[i].name < ranked[j].name
	})
	if maxN <= 0 || maxN > len(ranked) {
		maxN = len(ranked)
	}
	out := make([]string, 0, maxN)
	for _, s := range ranked[:maxN] {
		out = append(out, s.name)
	}
	return out
}

// editLimit allows roughly one third of the typed length in edits (min 2), so
// short names tolerate a couple of typos and longer ones proportionally more.
func editLimit(s string) int {
	if l := len([]rune(s)) / 3; l > 2 {
		return l
	}
	return 2
}

// containsSegment reports whether one name contains the other as a substring
// after stripping the "+"/"--" sigils. It catches hallucinated names that drop
// a namespace prefix (e.g. "+block-list" for "+base-block-list"), which share
// almost no prefix and sit far beyond the edit-distance budget. The shorter
// side must be at least 5 runes so generic fragments like "list" do not match
// half the catalog.
func containsSegment(a, b string) bool {
	a = strings.TrimLeft(a, "+-")
	b = strings.TrimLeft(b, "+-")
	if len([]rune(a)) > len([]rune(b)) {
		a, b = b, a
	}
	return len([]rune(a)) >= 5 && strings.Contains(b, a)
}

func sharedPrefixLen(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	n := 0
	for n < len(ra) && n < len(rb) && ra[n] == rb[n] {
		n++
	}
	return n
}
