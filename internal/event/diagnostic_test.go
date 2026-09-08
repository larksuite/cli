// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateDiagnostic(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxBytes int
		suffix   string
		want     string
	}{
		{name: "short", input: "short", maxBytes: 5, suffix: "...", want: "short"},
		{name: "ASCII boundary", input: "abcdef", maxBytes: 5, suffix: "...", want: "abcde..."},
		{name: "rune boundary", input: strings.Repeat("x", 4) + "界tail", maxBytes: 5, suffix: "…", want: "xxxx…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateDiagnostic(tt.input, tt.maxBytes, tt.suffix)
			if got != tt.want {
				t.Fatalf("TruncateDiagnostic() = %q, want %q", got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("TruncateDiagnostic() returned invalid UTF-8: %q", got)
			}
		})
	}
}
