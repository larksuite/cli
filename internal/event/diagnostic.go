// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import "unicode/utf8"

// TruncateDiagnostic bounds the retained prefix of s to maxBytes, backing off
// to the previous rune boundary so truncating valid UTF-8 keeps it valid.
func TruncateDiagnostic(s string, maxBytes int, suffix string) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + suffix
}
