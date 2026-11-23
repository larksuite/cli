// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package validate

import (
	"testing"
)

func TestRejectControlChars_FiltersControlCharsAndDangerousUnicode(t *testing.T) {
	for _, tt := range []struct {
		name    string
		input   string
		wantErr bool
	}{
		// ── GIVEN: normal text → THEN: allowed ──
		{"plain text", "hello world", false},
		{"with tab", "hello\tworld", false},
		{"with newline", "hello\nworld", false},
		{"unicode text", "你好世界", false},
		{"with symbols", "hello!@#$^&*()", false},
		{"empty", "", false},

		// ── GIVEN: C0 control characters → THEN: rejected ──
		{"null byte", "hello\x00world", true},
		{"bell", "hello\x07world", true},
		{"backspace", "hello\x08world", true},
		{"escape", "hello\x1bworld", true},
		{"carriage return", "hello\rworld", true},
		{"form feed", "hello\x0cworld", true},
		{"vertical tab", "hello\x0bworld", true},
		{"DEL", "hello\x7fworld", true},

		// ── GIVEN: dangerous Unicode characters → THEN: rejected ──
		{"zero width space", "hello\u200Bworld", true},
		{"zero width non-joiner", "hello\u200Cworld", true},
		{"zero width joiner", "hello\u200Dworld", true},
		{"BOM", "hello\uFEFFworld", true},
		{"bidi LRE", "hello\u202Aworld", true},
		{"bidi RLE", "hello\u202Bworld", true},
		{"bidi PDF", "hello\u202Cworld", true},
		{"bidi LRO", "hello\u202Dworld", true},
		{"bidi RLO", "hello\u202Eworld", true},
		{"line separator", "hello\u2028world", true},
		{"paragraph separator", "hello\u2029world", true},
		{"bidi LRI", "hello\u2066world", true},
		{"bidi RLI", "hello\u2067world", true},
		{"bidi FSI", "hello\u2068world", true},
		{"bidi PDI", "hello\u2069world", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// WHEN: RejectControlChars validates the input
			err := RejectControlChars(tt.input, "--test")

			// THEN: error matches expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("RejectControlChars(%q) error = %v, wantErr %v",
					tt.input, err, tt.wantErr)
			}
		})
	}
}
