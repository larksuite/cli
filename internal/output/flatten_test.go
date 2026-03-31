// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"testing"
)

func TestFlattenObjectSimple(t *testing.T) {
	obj := map[string]interface{}{
		"name": "Alice",
		"age":  float64(30),
	}
	entries := flattenObject(obj, "", 0)
	m := rowMap(entries)

	if m["name"] != "Alice" {
		t.Errorf("name = %q, want %q", m["name"], "Alice")
	}
	if m["age"] != "30" {
		t.Errorf("age = %q, want %q", m["age"], "30")
	}
}

func TestFlattenObjectNested(t *testing.T) {
	obj := map[string]interface{}{
		"user": map[string]interface{}{
			"name": "Alice",
			"addr": map[string]interface{}{
				"city": "Beijing",
			},
		},
	}
	entries := flattenObject(obj, "", 0)
	m := rowMap(entries)

	if m["user.name"] != "Alice" {
		t.Errorf("user.name = %q, want %q", m["user.name"], "Alice")
	}
	if m["user.addr.city"] != "Beijing" {
		t.Errorf("user.addr.city = %q, want %q", m["user.addr.city"], "Beijing")
	}
}

func TestFlattenObjectDeepLimit(t *testing.T) {
	// Create depth=4 nesting — should serialize the innermost object as JSON
	obj := map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": map[string]interface{}{
					"d": "deep",
				},
			},
		},
	}
	entries := flattenObject(obj, "", 0)
	m := rowMap(entries)

	// depth 0 → a (map), depth 1 → b (map), depth 2 → c (map), depth 3 ≥ maxFlattenDepth → serialize
	if v, ok := m["a.b.c"]; !ok {
		t.Errorf("expected key a.b.c, got keys: %v", m)
	} else if v != `{"d":"deep"}` {
		t.Errorf("a.b.c = %q, want JSON string", v)
	}
}

func TestFlattenObjectArrayLeaf(t *testing.T) {
	obj := map[string]interface{}{
		"tags": []interface{}{"a", "b"},
	}
	entries := flattenObject(obj, "", 0)
	m := rowMap(entries)

	if m["tags"] != `["a","b"]` {
		t.Errorf("tags = %q, want %q", m["tags"], `["a","b"]`)
	}
}

func TestFlattenObjectNilValue(t *testing.T) {
	obj := map[string]interface{}{
		"empty": nil,
	}
	entries := flattenObject(obj, "", 0)
	m := rowMap(entries)

	if m["empty"] != "" {
		t.Errorf("empty = %q, want %q", m["empty"], "")
	}
}

func TestCollectColumns(t *testing.T) {
	rows := [][]flatEntry{
		{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}},
		{{Key: "b", Value: "3"}, {Key: "c", Value: "4"}},
	}
	cols := collectColumns(rows)

	// Should contain a, b, c (union)
	colSet := map[string]bool{}
	for _, c := range cols {
		colSet[c] = true
	}
	for _, expected := range []string{"a", "b", "c"} {
		if !colSet[expected] {
			t.Errorf("missing column %q in %v", expected, cols)
		}
	}
	if len(cols) != 3 {
		t.Errorf("got %d columns, want 3", len(cols))
	}
}

func TestTruncateToWidth(t *testing.T) {
	tests := []struct {
		input    string
		maxWidth int
		want     string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 4, "hel…"},
		{"hello", 3, "he…"},
		{"hello", 1, "…"},
		{"hello", 0, ""},
		// CJK: each char is width 2
		{"你好世界", 8, "你好世界"},
		{"你好世界", 6, "你好…"},
		{"你好世界", 4, "你…"},
		{"你好世界", 3, "你…"},
		{"你好世界", 2, "…"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncateToWidth(tt.input, tt.maxWidth)
			if got != tt.want {
				t.Errorf("truncateToWidth(%q, %d) = %q, want %q", tt.input, tt.maxWidth, got, tt.want)
			}
			if width := stringWidth(got); width > tt.maxWidth {
				t.Errorf("truncateToWidth(%q, %d) width = %d, want <= %d", tt.input, tt.maxWidth, width, tt.maxWidth)
			}
		})
	}
}

func TestStringWidth(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello", 5},
		{"你好", 4},
		{"ab你好cd", 8},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := stringWidth(tt.input)
			if got != tt.want {
				t.Errorf("stringWidth(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
