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
//
// A hallucinated name is also often a compound welded from real names ("sql-file"
// from "sql" + "file"). Prefix and edit distance both miss the trailing half:
// "file" shares no prefix with "sql-file" and sits 4 edits away, past the budget,
// so the one candidate naming what the caller actually wanted got dropped while
// the leading half survived on prefix alone. Segment-exact candidates are
// therefore always plausible, however far the whole string drifted.
func Closest(typed string, candidates []string, maxN int) []string {
	type scored struct {
		name   string
		prefix int
		dist   int
	}
	limit := editLimit(typed)
	segments := hyphenSegments(typed)
	ranked := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		p := sharedPrefixLen(typed, c)
		d := Levenshtein(typed, c)
		// Keep only plausible matches: a meaningful shared prefix, an edit
		// distance within budget, or an exact hit on one segment of a compound.
		// Drop everything else so the hint stays short.
		if p >= 3 || d <= limit || segments[c] {
			ranked = append(ranked, scored{name: c, prefix: p, dist: d})
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
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

func sharedPrefixLen(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	n := 0
	for n < len(ra) && n < len(rb) && ra[n] == rb[n] {
		n++
	}
	return n
}

// hyphenSegments splits a hyphenated name into its parts and returns them as a
// set, so a candidate that exactly equals one part can be recognized in O(1).
// Single-segment names yield an empty set: without a hyphen there is no compound
// to decompose, and treating the whole string as a "segment" would just restate
// the equality case Closest already handles at distance 0.
func hyphenSegments(typed string) map[string]bool {
	if !strings.Contains(typed, "-") {
		return nil
	}
	out := make(map[string]bool, 2)
	for _, seg := range strings.Split(typed, "-") {
		if seg != "" {
			out[seg] = true
		}
	}
	return out
}
