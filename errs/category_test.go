// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

import "testing"

func TestCategoryWireValues(t *testing.T) {
	tests := []struct {
		name string
		got  Category
		want string
	}{
		{"validation", CategoryValidation, "validation"},
		{"authentication", CategoryAuthentication, "authentication"},
		{"authorization", CategoryAuthorization, "authorization"},
		{"config", CategoryConfig, "config"},
		{"network", CategoryNetwork, "network"},
		{"api", CategoryAPI, "api"},
		{"policy", CategoryPolicy, "policy"},
		{"internal", CategoryInternal, "internal"},
		{"confirmation", CategoryConfirmation, "confirmation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.want {
				t.Errorf("category %s = %q, want %q", tt.name, string(tt.got), tt.want)
			}
		})
	}
}
