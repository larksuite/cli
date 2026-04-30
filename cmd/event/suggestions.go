// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"fmt"
	"sort"
	"strings"

	eventlib "github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/output"
)

const maxSuggestions = 3

// suggestEventKeys returns up to maxSuggestions keys resembling input (substring match beats edit distance).
func suggestEventKeys(input string) []string {
	type match struct {
		key  string
		dist int
	}
	var hits []match
	threshold := max(2, len(input)/5)

	for _, def := range eventlib.ListAll() {
		if strings.Contains(def.Key, input) {
			hits = append(hits, match{def.Key, 0})
			continue
		}
		if d := levenshtein(input, def.Key); d <= threshold {
			hits = append(hits, match{def.Key, d})
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].dist < hits[j].dist })

	n := min(maxSuggestions, len(hits))
	out := make([]string, n)
	for i := range out {
		out[i] = hits[i].key
	}
	return out
}

// formatSuggestions renders keys as a human-readable quoted tail.
func formatSuggestions(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	quoted := make([]string, len(keys))
	for i, k := range keys {
		quoted[i] = fmt.Sprintf("%q", k)
	}
	if len(quoted) == 1 {
		return quoted[0]
	}
	return "one of: " + strings.Join(quoted, ", ")
}

// unknownEventKeyErr builds the shared "unknown EventKey" error with a suggestion tail when available.
func unknownEventKeyErr(key string) error {
	msg := fmt.Sprintf("unknown EventKey: %s", key)
	if guesses := suggestEventKeys(key); len(guesses) > 0 {
		msg += " — did you mean " + formatSuggestions(guesses) + "?"
	}
	return output.ErrWithHint(
		output.ExitValidation, "validation",
		msg,
		"Run 'lark-cli event list' to see available keys.",
	)
}

// levenshtein computes classic edit distance (two-row DP).
func levenshtein(a, b string) int {
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
