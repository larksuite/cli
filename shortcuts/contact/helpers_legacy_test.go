// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import "testing"

func TestPickUserName_PriorityOrder(t *testing.T) {
	tests := []struct {
		desc string
		in   map[string]interface{}
		want string
	}{
		{"name takes precedence", map[string]interface{}{"name": "A", "user_name": "B"}, "A"},
		{"user_name when name empty", map[string]interface{}{"name": "", "user_name": "B"}, "B"},
		{"display_name fallback", map[string]interface{}{"display_name": "C"}, "C"},
		{"employee_name fallback", map[string]interface{}{"employee_name": "D"}, "D"},
		{"cn_name fallback", map[string]interface{}{"cn_name": "E"}, "E"},
		{"non-string values are skipped", map[string]interface{}{"name": 42, "user_name": "F"}, "F"},
		{"nothing matches → empty string", map[string]interface{}{"unknown": "X"}, ""},
		{"empty map → empty string", map[string]interface{}{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if got := pickUserName(tt.in); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		desc string
		in   map[string]interface{}
		keys []string
		want string
	}{
		{"first key wins", map[string]interface{}{"a": "x", "b": "y"}, []string{"a", "b"}, "x"},
		{"falls through empty string", map[string]interface{}{"a": "", "b": "y"}, []string{"a", "b"}, "y"},
		{"non-string values are skipped", map[string]interface{}{"a": 42, "b": "z"}, []string{"a", "b"}, "z"},
		{"all empty / missing → empty string", map[string]interface{}{"a": ""}, []string{"a", "b"}, ""},
		{"no keys requested → empty string", map[string]interface{}{"a": "x"}, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if got := firstNonEmpty(tt.in, tt.keys...); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
