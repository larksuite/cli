// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package i18n

import "testing"

func TestIsValidLang(t *testing.T) {
	tests := []struct {
		lang     string
		expected bool
	}{
		{"zh", true},
		{"en", true},
		{"ja", true},
		{"ko", true},
		{"invalid", false},
		{"", false},
		{"ZH", false}, // case sensitive
	}
	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			if got := IsValidLang(tt.lang); got != tt.expected {
				t.Errorf("IsValidLang(%q) = %v, want %v", tt.lang, got, tt.expected)
			}
		})
	}
	// Guard against drift between ValidLanguages and IsValidLang.
	for _, lang := range ValidLanguages {
		if !IsValidLang(lang) {
			t.Errorf("IsValidLang(%q) = false, want true", lang)
		}
	}
}

func TestValidLanguages(t *testing.T) {
	if len(ValidLanguages) != 14 {
		t.Errorf("Expected 14 languages, got %d", len(ValidLanguages))
	}
}
