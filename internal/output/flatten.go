// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"sort"
	"unicode/utf8"
)

const maxFlattenDepth = 3
const ellipsis = "…"

type flatEntry struct {
	Key   string
	Value string
}

// flattenObject flattens a nested object into dot-notation key-value pairs.
// Objects nested beyond maxFlattenDepth levels are serialized as JSON strings.
// Keys are sorted alphabetically for deterministic column order.
func flattenObject(obj map[string]interface{}, prefix string, depth int) []flatEntry {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var entries []flatEntry
	for _, k := range keys {
		v := obj[k]
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch val := v.(type) {
		case map[string]interface{}:
			if depth+1 >= maxFlattenDepth {
				entries = append(entries, flatEntry{Key: key, Value: cellStr(val)})
			} else {
				entries = append(entries, flattenObject(val, key, depth+1)...)
			}
		default:
			entries = append(entries, flatEntry{Key: key, Value: cellStr(v)})
		}
	}
	return entries
}

// collectColumns collects column names from all rows (union set),
// preserving first-occurrence order.
func collectColumns(rows [][]flatEntry) []string {
	seen := map[string]bool{}
	var cols []string
	for _, row := range rows {
		for _, e := range row {
			if !seen[e.Key] {
				seen[e.Key] = true
				cols = append(cols, e.Key)
			}
		}
	}
	return cols
}

// rowMap converts a slice of flatEntry into a map for column lookup.
func rowMap(entries []flatEntry) map[string]string {
	m := make(map[string]string, len(entries))
	for _, e := range entries {
		m[e.Key] = e.Value
	}
	return m
}

// runeWidth returns the display width of a rune.
// CJK characters and some symbols are double-width.
func runeWidth(r rune) int {
	if r == utf8.RuneError {
		return 1
	}
	// CJK Unified Ideographs, CJK Compatibility Ideographs, etc.
	if (r >= 0x1100 && r <= 0x115F) || // Hangul Jamo
		r == 0x2329 || r == 0x232A ||
		(r >= 0x2E80 && r <= 0x303E) || // CJK Radicals, Kangxi, CJK Symbols
		(r >= 0x3040 && r <= 0x33BF) || // Hiragana, Katakana, Bopomofo, etc.
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Unified Ideographs Extension A
		(r >= 0x4E00 && r <= 0xA4CF) || // CJK Unified Ideographs, Yi
		(r >= 0xA960 && r <= 0xA97C) || // Hangul Jamo Extended-A
		(r >= 0xAC00 && r <= 0xD7A3) || // Hangul Syllables
		(r >= 0xF900 && r <= 0xFAFF) || // CJK Compatibility Ideographs
		(r >= 0xFE10 && r <= 0xFE6F) || // CJK Compatibility Forms, Small Forms
		(r >= 0xFF01 && r <= 0xFF60) || // Fullwidth Forms
		(r >= 0xFFE0 && r <= 0xFFE6) || // Fullwidth Signs
		(r >= 0x1F300 && r <= 0x1F9FF) || // Emoji (Miscellaneous Symbols and Pictographs, Emoticons, etc.)
		(r >= 0x20000 && r <= 0x2FFFF) || // CJK Unified Ideographs Extension B-F
		(r >= 0x30000 && r <= 0x3FFFF) { // CJK Unified Ideographs Extension G+
		return 2
	}
	return 1
}

// stringWidth returns the display width of a string.
func stringWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

// truncateToWidth truncates a string to fit within maxWidth display columns.
// If truncated, appends "…".
func truncateToWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if stringWidth(s) <= maxWidth {
		return s
	}

	ellipsisWidth := stringWidth(ellipsis)
	if ellipsisWidth > maxWidth {
		return ""
	}

	w := 0
	cut := 0
	for i, r := range s {
		rw := runeWidth(r)
		if w+rw+ellipsisWidth > maxWidth {
			break
		}
		w += rw
		cut = i + utf8.RuneLen(r)
	}
	if cut == 0 {
		return ellipsis
	}
	return s[:cut] + ellipsis
}

// flattenItem flattens a single item (object or other) into flatEntry pairs.
func flattenItem(item interface{}) []flatEntry {
	if obj, ok := item.(map[string]interface{}); ok {
		return flattenObject(obj, "", 0)
	}
	return []flatEntry{{Key: "value", Value: cellStr(item)}}
}

// prepareRows converts a data value into flattened rows and column names.
// Returns rows (as maps), columns, and whether the data was a list.
func prepareRows(data interface{}) (rows []map[string]string, cols []string, isList bool) {
	items := extractArray(data)
	if items == nil {
		// Single object
		if obj, ok := data.(map[string]interface{}); ok {
			entries := flattenObject(obj, "", 0)
			rm := rowMap(entries)
			flatRows := [][]flatEntry{entries}
			return []map[string]string{rm}, collectColumns(flatRows), false
		}
		return nil, nil, false
	}

	isList = true
	var flatRows [][]flatEntry
	for _, item := range items {
		entries := flattenItem(item)
		flatRows = append(flatRows, entries)
		rows = append(rows, rowMap(entries))
	}
	cols = collectColumns(flatRows)
	return rows, cols, isList
}

// extractArray extracts an array from data, or returns nil.
func extractArray(data interface{}) []interface{} {
	if arr, ok := data.([]interface{}); ok {
		return arr
	}
	return nil
}
