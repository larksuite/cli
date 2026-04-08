// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package validate

import (
	"testing"

	"github.com/larksuite/cli/internal/charcheck"
)

func TestSanitizeForTerminal_StripsEscapesAndDangerousChars(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input string
		want  string
	}{
		// ── GIVEN: normal text → THEN: unchanged ──
		{"plain text", "hello world", "hello world"},
		{"unicode text", "你好世界", "你好世界"},
		{"empty", "", ""},

		// ── GIVEN: tab and newline → THEN: preserved ──
		{"preserve tab", "col1\tcol2", "col1\tcol2"},
		{"preserve newline", "line1\nline2", "line1\nline2"},

		// ── GIVEN: ANSI CSI sequences → THEN: stripped, text preserved ──
		{"clear screen", "before\x1b[2Jafter", "beforeafter"},
		{"red color", "before\x1b[31mred\x1b[0mafter", "beforeredafter"},
		{"bold", "before\x1b[1mbold\x1b[0mafter", "beforeboldafter"},
		{"cursor move", "before\x1b[10;20Hafter", "beforeafter"},
		{"multiple sequences", "\x1b[31m\x1b[1mhello\x1b[0m", "hello"},

		// ── GIVEN: ANSI OSC sequences → THEN: stripped ──
		{"OSC title change", "before\x1b]0;evil title\x07after", "beforeafter"},
		{"OSC with text", "text\x1b]2;new title\x07more", "textmore"},

		// ── GIVEN: C0 control characters → THEN: stripped ──
		{"null byte", "hello\x00world", "helloworld"},
		{"bell", "hello\x07world", "helloworld"},
		{"backspace", "hello\x08world", "helloworld"},
		{"escape alone", "hello\x1bworld", "helloworld"},
		{"carriage return", "hello\rworld", "helloworld"},
		{"DEL", "hello\x7fworld", "helloworld"},

		// ── GIVEN: dangerous Unicode → THEN: stripped ──
		{"zero width space", "hello\u200Bworld", "helloworld"},
		{"BOM", "hello\uFEFFworld", "helloworld"},
		{"bidi RLO", "hello\u202Eworld", "helloworld"},
		{"bidi LRI", "hello\u2066world", "helloworld"},
		{"line separator", "hello\u2028world", "helloworld"},

		// ── GIVEN: mixed attack payload → THEN: all dangerous content stripped ──
		{"ansi + null + bidi", "\x1b[31m\x00\u202Ehello\x1b[0m", "hello"},
		{"realistic injection", "Status: \x1b[32mOK\x1b[0m (fake)", "Status: OK (fake)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// WHEN: SanitizeForTerminal processes the input
			got := SanitizeForTerminal(tt.input)

			// THEN: output matches expected sanitized result
			if got != tt.want {
				t.Errorf("SanitizeForTerminal(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsDangerousUnicode_IdentifiesAllDangerousRanges(t *testing.T) {
	// ── GIVEN: known dangerous Unicode code points → THEN: returns true ──
	dangerous := []rune{
		0x200B, 0x200C, 0x200D, // zero-width
		0xFEFF,                                 // BOM
		0x202A, 0x202B, 0x202C, 0x202D, 0x202E, // bidi
		0x2028, 0x2029, // separators
		0x2066, 0x2067, 0x2068, 0x2069, // isolates
	}
	for _, r := range dangerous {
		if !charcheck.IsDangerousUnicode(r) {
			t.Errorf("charcheck.IsDangerousUnicode(%U) = false, want true", r)
		}
	}

	// ── GIVEN: safe Unicode code points → THEN: returns false ──
	safe := []rune{'A', '中', '!', ' ', '\t', '\n', 0x200A, 0x2070}
	for _, r := range safe {
		if charcheck.IsDangerousUnicode(r) {
			t.Errorf("charcheck.IsDangerousUnicode(%U) = true, want false", r)
		}
	}
}
