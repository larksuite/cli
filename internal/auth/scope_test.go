// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"testing"
)

// TestMissingScopes tests the calculation of missing scopes.
func TestMissingScopes(t *testing.T) {
	tests := []struct {
		name        string
		storedScope string
		required    []string
		expected    []string
	}{
		{
			name:        "all matched",
			storedScope: "a b c",
			required:    []string{"a", "b"},
			expected:    nil,
		},
		{
			name:        "partial missing",
			storedScope: "a b",
			required:    []string{"a", "c"},
			expected:    []string{"c"},
		},
		{
			name:        "all missing",
			storedScope: "a b",
			required:    []string{"x", "y"},
			expected:    []string{"x", "y"},
		},
		{
			name:        "empty storedScope",
			storedScope: "",
			required:    []string{"a"},
			expected:    []string{"a"},
		},
		{
			name:        "empty required",
			storedScope: "a b",
			required:    []string{},
			expected:    nil,
		},
		{
			name:        "extra whitespace in storedScope",
			storedScope: " a  b  c ",
			required:    []string{"b"},
			expected:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MissingScopes(tt.storedScope, tt.required)
			if !sliceEqual(got, tt.expected) {
				t.Errorf("MissingScopes(%q, %v) = %v, want %v", tt.storedScope, tt.required, got, tt.expected)
			}
		})
	}
}

// sliceEqual compares two string slices for equality.
func sliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
